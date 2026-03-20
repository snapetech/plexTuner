package tuner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/channelreport"
	"github.com/snapetech/iptvtunerr/internal/epgstore"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

// PlexDVRMaxChannels is Plex's per-tuner channel limit when using the wizard; exceeding it causes "failed to save channel lineup".
const PlexDVRMaxChannels = 480

// PlexDVRWizardSafeMax is used in "easy" mode: strip from end so lineup fits when Plex suggests a guide (e.g. Rogers West Canada ~680 ch); keep first N.
const PlexDVRWizardSafeMax = 479

func parseCustomHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return headers
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, ":"); idx > 0 {
			name := strings.TrimSpace(part[:idx])
			value := strings.TrimSpace(part[idx+1:])
			if name != "" && value != "" {
				headers[name] = value
			}
		}
	}
	return headers
}

// NoLineupCap disables the lineup cap (use when syncing lineup into Plex DB programmatically so users get full channel count).
const NoLineupCap = -1

// Server runs the HDHR emulator + XMLTV + stream gateway.
// Handlers are kept so UpdateChannels can refresh the channel list without restart.
type Server struct {
	Addr              string
	BaseURL           string
	TunerCount        int
	LineupMaxChannels int    // max channels in lineup/guide (default PlexDVRMaxChannels); 0 = use PlexDVRMaxChannels
	GuideNumberOffset int    // add offset to exposed GuideNumber values to avoid cross-DVR collisions in Plex clients
	DeviceID          string // HDHomeRun discover.json; set from IPTV_TUNERR_DEVICE_ID
	FriendlyName      string // HDHomeRun discover.json; set from IPTV_TUNERR_FRIENDLY_NAME
	// AppVersion is shown on /ui/ (optional; set from main.Version in cmd).
	AppVersion          string
	StreamBufferBytes   int    // 0 = no buffer; -1 = auto; e.g. 2097152 for 2 MiB
	StreamTranscodeMode string // "off" | "on" | "auto"
	AutopilotStateFile  string // optional JSON file for remembered dna_id+client_class playback decisions
	RecorderStateFile   string // optional JSON file written by catchup-daemon for recorder status/reporting
	Channels            []catalog.LiveChannel
	ProviderUser        string
	ProviderPass        string
	ProviderBaseURL     string
	XMLTVSourceURL      string
	XMLTVTimeout        time.Duration
	XMLTVCacheTTL       time.Duration // 0 = use default 10m
	EpgPruneUnlinked    bool          // when true, guide.xml and /live.m3u only include channels with tvg-id
	FetchCFReject       bool          // abort HLS stream if segment redirected to CF abuse page (passed to Gateway)
	ProviderEPGEnabled  bool
	ProviderEPGTimeout  time.Duration
	ProviderEPGCacheTTL time.Duration
	// ProviderEPGDiskCachePath: optional on-disk cache + conditional GET for provider xmltv.php.
	ProviderEPGDiskCachePath  string
	ProviderEPGIncremental    bool
	ProviderEPGLookaheadHours int
	ProviderEPGBackfillHours  int

	// EpgStore is an optional SQLite file for durable merged EPG (LP-007/008). Nil = disabled.
	EpgStore *epgstore.Store
	// EpgSQLiteRetainPastHours: if > 0, drop SQLite programme rows whose stop is before now-N hours (LP-009).
	EpgSQLiteRetainPastHours int
	// EpgSQLiteVacuumAfterPrune: VACUUM SQLite after retain-past prune removed rows (LP-009).
	EpgSQLiteVacuumAfterPrune bool
	// EpgSQLiteMaxBytes: optional post-sync file size cap (LP-009).
	EpgSQLiteMaxBytes int64
	// EpgSQLiteIncrementalUpsert uses overlap-window upsert instead of full truncate+replace.
	EpgSQLiteIncrementalUpsert bool
	// ProviderEPGURLSuffix is appended to provider xmltv.php URL (optional; e.g. panel-specific date params).
	ProviderEPGURLSuffix string
	// HDHRGuideURL is an optional device guide.xml URL (LP-003); merged after provider + external gap-fill.
	HDHRGuideURL string
	// HDHRGuideTimeout for guide.xml fetch; 0 = default 90s.
	HDHRGuideTimeout time.Duration
	// RuntimeSnapshot is an optional read-only view of effective settings for the operator dashboard.
	RuntimeSnapshot *RuntimeSnapshot

	// health state updated by UpdateChannels; read by /healthz and /readyz.
	healthMu       sync.RWMutex
	healthChannels int
	healthRefresh  time.Time

	hdhr     *HDHR
	gateway  *Gateway
	xmltv    *XMLTV
	m3uServe *M3UServe
}

// RuntimeSnapshot is returned by /debug/runtime.json for the dedicated web UI and operator tooling.
// Secrets are intentionally omitted or reduced to presence booleans/counts.
type RuntimeSnapshot struct {
	GeneratedAt   string                 `json:"generated_at"`
	Version       string                 `json:"version,omitempty"`
	ListenAddress string                 `json:"listen_address,omitempty"`
	BaseURL       string                 `json:"base_url,omitempty"`
	DeviceID      string                 `json:"device_id,omitempty"`
	FriendlyName  string                 `json:"friendly_name,omitempty"`
	Tuner         map[string]interface{} `json:"tuner,omitempty"`
	Guide         map[string]interface{} `json:"guide,omitempty"`
	Provider      map[string]interface{} `json:"provider,omitempty"`
	Recorder      map[string]interface{} `json:"recorder,omitempty"`
	HDHR          map[string]interface{} `json:"hdhr,omitempty"`
	WebUI         map[string]interface{} `json:"webui,omitempty"`
	MediaServers  map[string]interface{} `json:"media_servers,omitempty"`
	URLs          map[string]string      `json:"urls,omitempty"`
}

type OperatorActionResponse struct {
	OK      bool        `json:"ok"`
	Action  string      `json:"action"`
	Message string      `json:"message,omitempty"`
	Detail  interface{} `json:"detail,omitempty"`
}

type OperatorWorkflowReport struct {
	GeneratedAt string                 `json:"generated_at"`
	Name        string                 `json:"name"`
	Summary     map[string]interface{} `json:"summary,omitempty"`
	Steps       []string               `json:"steps,omitempty"`
	Actions     []string               `json:"actions,omitempty"`
}

// UpdateChannels updates the channel list for all handlers so -refresh can serve new lineup without restart.
// Caps at LineupMaxChannels (default PlexDVRMaxChannels) so Plex DVR can save the lineup when using the wizard (Plex fails above ~480).
// When LineupMaxChannels is NoLineupCap, no cap is applied (for programmatic lineup sync; see -register-plex).
func (s *Server) UpdateChannels(live []catalog.LiveChannel) {
	live = applyLineupPreCapFilters(live)
	if s.xmltv != nil {
		live = s.xmltv.applyGuidePolicyToChannels(live, os.Getenv("IPTV_TUNERR_GUIDE_POLICY"))
	}
	live = applyDNAPolicy(live, os.Getenv("IPTV_TUNERR_DNA_POLICY"))
	if s.LineupMaxChannels == NoLineupCap {
		// Full lineup for programmatic sync; do not cap.
	} else {
		max := s.LineupMaxChannels
		if max <= 0 {
			max = PlexDVRMaxChannels
		}
		if len(live) > max {
			log.Printf("Lineup capped at %d channels (Plex DVR limit; catalog has %d; excess stripped from end)", max, len(live))
			live = live[:max]
		}
	}
	live = applyGuideNumberOffset(live, s.GuideNumberOffset)
	s.Channels = live
	s.healthMu.Lock()
	s.healthChannels = len(live)
	s.healthRefresh = time.Now()
	s.healthMu.Unlock()
	if s.hdhr != nil {
		s.hdhr.Channels = live
	}
	if s.gateway != nil {
		s.gateway.Channels = live
	}
	if s.xmltv != nil {
		s.xmltv.Channels = live
		s.xmltv.mu.Lock()
		s.xmltv.cachedMatchReport = nil
		s.xmltv.cachedMatchAliases = ""
		s.xmltv.cachedMatchExp = time.Time{}
		s.xmltv.cachedGuideHealth = nil
		s.xmltv.mu.Unlock()
	}
	if s.m3uServe != nil {
		s.m3uServe.Channels = live
	}
}

func applyGuideNumberOffset(live []catalog.LiveChannel, offset int) []catalog.LiveChannel {
	if offset == 0 || len(live) == 0 {
		return live
	}
	out := make([]catalog.LiveChannel, len(live))
	copy(out, live)
	changed := 0
	for i := range out {
		g := strings.TrimSpace(out[i].GuideNumber)
		if g == "" {
			continue
		}
		n, err := strconv.Atoi(g)
		if err != nil {
			continue
		}
		out[i].GuideNumber = strconv.Itoa(n + offset)
		changed++
	}
	if changed > 0 {
		log.Printf("Guide number offset applied: offset=%d changed=%d/%d channels", offset, changed, len(out))
	}
	return out
}

func applyLineupPreCapFilters(live []catalog.LiveChannel) []catalog.LiveChannel {
	if len(live) == 0 {
		return live
	}
	before := len(live)
	out := live
	if envBool("IPTV_TUNERR_LINEUP_DROP_MUSIC", false) {
		filtered := make([]catalog.LiveChannel, 0, len(out))
		dropped := 0
		for _, ch := range out {
			if looksLikeMusicOrRadioChannel(ch) {
				dropped++
				continue
			}
			filtered = append(filtered, ch)
		}
		if dropped > 0 {
			log.Printf("Lineup pre-cap filter: dropped %d music/radio channels by name heuristic (remaining %d)", dropped, len(filtered))
			out = filtered
		}
	}
	if want := normalizeLineupLangFilter(strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_LANGUAGE"))); want != "" && want != "all" {
		filtered := make([]catalog.LiveChannel, 0, len(out))
		dropped := 0
		for _, ch := range out {
			if liveChannelMatchesLanguage(ch, want) {
				filtered = append(filtered, ch)
				continue
			}
			dropped++
		}
		log.Printf("Lineup pre-cap filter: language=%s kept=%d dropped=%d", want, len(filtered), dropped)
		out = filtered
	}
	if pat := strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX")); pat != "" {
		re, err := regexp.Compile("(?i)" + pat)
		if err != nil {
			log.Printf("Lineup pre-cap exclude regex ignored (compile failed): %v", err)
		} else {
			filtered := make([]catalog.LiveChannel, 0, len(out))
			dropped := 0
			for _, ch := range out {
				target := ch.GuideName + " " + ch.TVGID
				if re.MatchString(target) {
					dropped++
					continue
				}
				filtered = append(filtered, ch)
			}
			if dropped > 0 {
				log.Printf("Lineup pre-cap filter: dropped %d channels by IPTV_TUNERR_LINEUP_EXCLUDE_REGEX (remaining %d)", dropped, len(filtered))
				out = filtered
			}
		}
	}
	if len(out) != before {
		// Continue with optional wizard-shaping reordering before cap.
	}
	out = applyLineupRecipe(out)
	out = applyLineupWizardShape(out)
	out = applyLineupShard(out)
	return out
}

func applyLineupRecipe(live []catalog.LiveChannel) []catalog.LiveChannel {
	return ApplyNamedLineupRecipe(live, os.Getenv("IPTV_TUNERR_LINEUP_RECIPE"))
}

func ApplyNamedLineupRecipe(live []catalog.LiveChannel, recipe string) []catalog.LiveChannel {
	recipe = strings.ToLower(strings.TrimSpace(recipe))
	if recipe == "" || recipe == "off" || recipe == "none" || len(live) == 0 {
		return live
	}
	type ranked struct {
		ch     catalog.LiveChannel
		score  int
		guide  int
		stream int
		idx    int
	}
	rows := make([]ranked, 0, len(live))
	for i, ch := range live {
		rows = append(rows, ranked{
			ch:     ch,
			score:  channelreport.Score(ch),
			guide:  channelreport.GuideConfidence(ch),
			stream: channelreport.StreamResilience(ch),
			idx:    i,
		})
	}

	filtered := rows[:0]
	switch recipe {
	case "high_confidence":
		for _, row := range rows {
			if row.guide >= 40 {
				filtered = append(filtered, row)
			}
		}
		if len(filtered) > 0 {
			rows = filtered
		}
	case "sports_now":
		for _, row := range rows {
			if lineupRecipeSportsLike(row.ch) {
				filtered = append(filtered, row)
			}
		}
		if len(filtered) > 0 {
			rows = filtered
		}
	case "kids_safe":
		for _, row := range rows {
			if lineupRecipeKidsSafe(row.ch) {
				filtered = append(filtered, row)
			}
		}
		if len(filtered) > 0 {
			rows = filtered
		}
	case "locals_first":
		// Reordering only; keep full set.
	case "resilient":
		// Reordering only; keep full set.
	case "balanced", "guide_first":
		// Reordering only; keep full set.
	default:
		log.Printf("Lineup recipe ignored: unknown recipe=%q", recipe)
		return live
	}

	sort.SliceStable(rows, func(i, j int) bool {
		switch recipe {
		case "resilient":
			if rows[i].stream == rows[j].stream {
				if rows[i].score == rows[j].score {
					return rows[i].idx < rows[j].idx
				}
				return rows[i].score > rows[j].score
			}
			return rows[i].stream > rows[j].stream
		case "guide_first":
			if rows[i].guide == rows[j].guide {
				if rows[i].score == rows[j].score {
					return rows[i].idx < rows[j].idx
				}
				return rows[i].score > rows[j].score
			}
			return rows[i].guide > rows[j].guide
		case "locals_first":
			left := lineupRecipeLocalScore(rows[i].ch)
			right := lineupRecipeLocalScore(rows[j].ch)
			if left == right {
				if rows[i].score == rows[j].score {
					return rows[i].idx < rows[j].idx
				}
				return rows[i].score > rows[j].score
			}
			return left > right
		default: // balanced, high_confidence
			if rows[i].score == rows[j].score {
				return rows[i].idx < rows[j].idx
			}
			return rows[i].score > rows[j].score
		}
	})

	out := make([]catalog.LiveChannel, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ch)
	}
	log.Printf("Lineup recipe applied: recipe=%s kept=%d/%d", recipe, len(out), len(live))
	return out
}

func lineupRecipeSportsLike(ch catalog.LiveChannel) bool {
	s := lineupRecipeSearchText(ch)
	for _, term := range []string{
		" espn", " tsn", " sportsnet", " nfl", " nba", " nhl", " mlb", " soccer", " football", " basketball",
		" baseball", " hockey", " fight ", " boxing", " ufc", " racing", " golf", " tennis", " cricket", " sports ",
	} {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}

func lineupRecipeKidsSafe(ch catalog.LiveChannel) bool {
	s := lineupRecipeSearchText(ch)
	for _, blocked := range []string{
		" adult", " xxx", " ppv", " fight ", " horror", " news", " fox news", " cnn", " msnbc",
	} {
		if strings.Contains(s, blocked) {
			return false
		}
	}
	for _, term := range []string{
		" disney", " cartoon", " nick", " nickelodeon", " nick jr", " junior", " kids", " family", " teen",
		" boomerang", " pbs kids", " treehouse", " cbeebies", " ytv", " universal kids",
	} {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}

func lineupRecipeLocalScore(ch catalog.LiveChannel) int {
	return scoreLineupChannelForShape("na_en", strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_REGION_PROFILE"))), ch)
}

func lineupRecipeSearchText(ch catalog.LiveChannel) string {
	return " " + strings.ToLower(strings.TrimSpace(ch.GuideName)+" "+strings.TrimSpace(ch.TVGID)) + " "
}

func normalizeLineupLangFilter(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "", "all", "*", "any":
		return "all"
	case "eng", "english":
		return "en"
	case "arabic":
		return "ar"
	case "french":
		return "fr"
	case "spanish":
		return "es"
	case "portuguese":
		return "pt"
	case "german":
		return "de"
	case "italian":
		return "it"
	case "turkish":
		return "tr"
	case "russian":
		return "ru"
	case "hindi":
		return "hi"
	default:
		return v
	}
}

func liveChannelMatchesLanguage(ch catalog.LiveChannel, want string) bool {
	want = normalizeLineupLangFilter(want)
	if want == "" || want == "all" {
		return true
	}
	guess := inferLiveChannelLanguage(ch)
	if guess == "" {
		// Conservative default for unknowns: keep only in English mode.
		return want == "en"
	}
	return guess == want
}

func inferLiveChannelLanguage(ch catalog.LiveChannel) string {
	s := strings.ToLower(strings.TrimSpace(ch.GuideName + " " + ch.TVGID + " " + ch.ChannelID))
	if s == "" {
		return ""
	}
	// Strong textual/script signals first.
	if looksMostlyNonLatinText(ch.GuideName) || looksMostlyNonLatinText(ch.ChannelID) || looksMostlyNonLatinText(ch.TVGID) {
		if hasAnyToken(s, []string{" arab", ".ar", " ar ", "bein ar", "mbc "}) {
			return "ar"
		}
		if hasAnyToken(s, []string{" persian", " farsi", ".ir"}) {
			return "fa"
		}
		if hasAnyToken(s, []string{"рус", ".ru", " ru "}) {
			return "ru"
		}
		return "nonlatin"
	}
	// Common explicit tags in IPTV lineups.
	switch {
	case hasAnyToken(s, []string{".fr", " fr ", " french", " france "}):
		return "fr"
	case hasAnyToken(s, []string{".es", " es ", " spanish", " espanol", " españa", " spain "}):
		return "es"
	case hasAnyToken(s, []string{".pt", " pt ", " portuguese", " portugal", " brazil", "brasil"}):
		return "pt"
	case hasAnyToken(s, []string{".de", " de ", " german", " germany"}):
		return "de"
	case hasAnyToken(s, []string{".it", " it ", " italian", " italy"}):
		return "it"
	case hasAnyToken(s, []string{".tr", " tr ", " turkish", " turkey"}):
		return "tr"
	case hasAnyToken(s, []string{".pl", " pl ", " polish", " poland"}):
		return "pl"
	case hasAnyToken(s, []string{".nl", " dutch", " netherlands"}):
		return "nl"
	case hasAnyToken(s, []string{".sv", " swedish", " sweden"}):
		return "sv"
	case hasAnyToken(s, []string{".da", " danish", " denmark"}):
		return "da"
	case hasAnyToken(s, []string{".no", " norwegian", " norway"}):
		return "no"
	case hasAnyToken(s, []string{".fi", " finnish", " finland"}):
		return "fi"
	case hasAnyToken(s, []string{".ar", " arabic", " arab "}):
		return "ar"
	case hasAnyToken(s, []string{".hi", " hindi", " india "}):
		return "hi"
	case hasAnyToken(s, []string{".en", " english", ".us", ".ca", ".uk", ".gb", ".au", ".nz", ".ie", ".za"}):
		return "en"
	default:
		return "en"
	}
}

func hasAnyToken(s string, needles []string) bool {
	padded := " " + s + " "
	for _, n := range needles {
		if strings.Contains(padded, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

func applyLineupShard(live []catalog.LiveChannel) []catalog.LiveChannel {
	if len(live) == 0 {
		return live
	}
	skip := envInt("IPTV_TUNERR_LINEUP_SKIP", 0)
	take := envInt("IPTV_TUNERR_LINEUP_TAKE", 0)
	if skip < 0 {
		skip = 0
	}
	if skip > len(live) {
		skip = len(live)
	}
	start := skip
	end := len(live)
	if take > 0 && start+take < end {
		end = start + take
	}
	if start == 0 && end == len(live) {
		return live
	}
	out := make([]catalog.LiveChannel, end-start)
	copy(out, live[start:end])
	log.Printf("Lineup shard applied: skip=%d take=%d selected=%d/%d", skip, take, len(out), len(live))
	return out
}

func applyLineupWizardShape(live []catalog.LiveChannel) []catalog.LiveChannel {
	shape := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_SHAPE")))
	if shape == "" || shape == "off" || shape == "none" {
		return live
	}
	region := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_REGION_PROFILE")))
	type scored struct {
		ch    catalog.LiveChannel
		score int
		idx   int
	}
	scoredCh := make([]scored, 0, len(live))
	for i, ch := range live {
		scoredCh = append(scoredCh, scored{
			ch:    ch,
			score: scoreLineupChannelForShape(shape, region, ch),
			idx:   i,
		})
	}
	sort.SliceStable(scoredCh, func(i, j int) bool {
		if scoredCh[i].score == scoredCh[j].score {
			return scoredCh[i].idx < scoredCh[j].idx
		}
		return scoredCh[i].score > scoredCh[j].score
	})
	out := make([]catalog.LiveChannel, 0, len(live))
	moved := 0
	for i, s := range scoredCh {
		out = append(out, s.ch)
		if s.idx != i {
			moved++
		}
	}
	if moved > 0 {
		log.Printf("Lineup pre-cap shape: shape=%s region=%s reordered %d/%d channels for wizard/provider matching", shape, regionOrDash(region), moved, len(out))
	}
	return out
}

func regionOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func scoreLineupChannelForShape(shape, region string, ch catalog.LiveChannel) int {
	if shape != "na_en" {
		return 0
	}
	name := strings.ToLower(strings.TrimSpace(ch.GuideName))
	tvgid := strings.ToLower(strings.TrimSpace(ch.TVGID))
	s := " " + name + " " + tvgid + " "
	score := 0
	naAffinity := 0

	// Prefer North American English-ish channels.
	switch {
	case strings.HasSuffix(tvgid, ".ca"):
		score += 80
		naAffinity = 2
	case strings.HasSuffix(tvgid, ".us"):
		score += 60
		naAffinity = 1
	case strings.HasSuffix(tvgid, ".mx"):
		score += 20
	case tvgid != "":
		score -= 80
	}

	// Prefer likely local/provider channels for western/prairie Canada style lineups.
	if region == "ca_west" || region == "ca_prairies" {
		for _, n := range []string{
			" regina", " saskatoon", " sask ", " winnipeg", " calgary", " edmonton", " vancouver", " victoria",
			" alberta", " manitoba", " british columbia", " bc ",
		} {
			if strings.Contains(s, n) {
				score += 55
			}
		}
	}

	// Core networks/channels that help provider matching feel local and conventional.
	for _, n := range []string{
		" cbc", " ctv", " global", " citytv", " omni", " ctv2", " noovo", " tva",
		" abc", " cbs", " nbc", " fox", " pbs", " cw",
		" tsn", " sportsnet", " sn ", " cp24", " cnn", " fox news", " msnbc", " weather network",
		" a&e", " history", " discovery", " national geographic", " hgtv", " food", " tlc",
	} {
		if strings.Contains(s, n) {
			score += 25
		}
	}

	// De-prioritize content that often confuses or bloats wizard/provider matching.
	for _, n := range []string{
		" adult", " ppv", " pay per view", " event", " test", " promo", " barker", " shopping",
		" qvc", " tsc ", " shop", " 4k", " uhd", " cam", " xxx",
	} {
		if strings.Contains(s, n) {
			score -= 80
		}
	}

	if looksMostlyNonLatinText(name) {
		score -= 35
	}
	if naAffinity == 0 && tvgid != "" {
		score -= 120
	}

	// Prefer conventional low channel numbers slightly, but don't let numbering dominate.
	if n := leadingGuideNumber(ch.GuideNumber); n > 0 {
		bump := 0
		switch {
		case n <= 99:
			bump = 20
		case n <= 199:
			bump = 12
		case n <= 399:
			bump = 6
		case n >= 1000:
			bump = -6
		}
		// Only trust channel numbering as a positive signal when the channel already
		// looks like part of the target NA provider shape.
		if bump > 0 && naAffinity == 0 {
			bump = 0
		}
		score += bump
	}

	if ch.EPGLinked || tvgid != "" {
		score += 8
	}
	return score
}

func leadingGuideNumber(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 {
			break
		}
	}
	if b.Len() == 0 {
		return 0
	}
	n, err := strconv.Atoi(b.String())
	if err != nil {
		return 0
	}
	return n
}

func looksMostlyNonLatinText(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	letters := 0
	latin := 0
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.In(r, unicode.Latin) {
			latin++
		}
	}
	if letters < 3 {
		return false
	}
	return latin*2 < letters
}

func looksLikeMusicOrRadioChannel(ch catalog.LiveChannel) bool {
	s := strings.ToLower(strings.TrimSpace(ch.GuideName + " " + ch.TVGID))
	if s == "" {
		return false
	}
	needles := []string{
		" stingray ",
		" vevo ",
		" mtv live",
		"music",
		"radio",
		"karaoke",
		"jukebox",
		"djazz",
		"mezzo",
		"trace ",
		"clubbing",
		"hits",
		"cmt",
	}
	padded := " " + s + " "
	for _, n := range needles {
		if strings.Contains(padded, n) {
			return true
		}
	}
	return false
}

// GetStream returns a reader for the given channel.
// This is used by HDHomeRun network mode to get streams for direct TCP delivery.
func (s *Server) GetStream(ctx context.Context, channelID string) (io.ReadCloser, error) {
	// Find the channel
	var ch *catalog.LiveChannel
	for i := range s.Channels {
		if s.Channels[i].ChannelID == channelID {
			ch = &s.Channels[i]
			break
		}
	}
	if ch == nil {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}

	// Use the gateway to get the stream - make HTTP request to ourselves
	// This reuses the existing gateway logic but via HTTP to localhost
	streamURL := fmt.Sprintf("http://127.0.0.1%s/stream/%s", s.Addr, channelID)
	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Prefer the gateway's client (cookies/UA parity); else shared tuned streaming client (not http.DefaultClient).
	client := httpclient.ForStreaming()
	if s.gateway != nil && s.gateway.Client != nil {
		client = s.gateway.Client
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// Run blocks until ctx is cancelled or the server fails to start. On shutdown it stops
// accepting new connections and waits briefly for in-flight requests to finish.
func (s *Server) Run(ctx context.Context) error {
	hdhr := &HDHR{
		BaseURL:      s.BaseURL,
		TunerCount:   s.TunerCount,
		DeviceID:     s.DeviceID,
		FriendlyName: s.FriendlyName,
		Channels:     s.Channels,
	}
	s.hdhr = hdhr
	defaultProfile := defaultProfileFromEnv()
	profileMatrixPath := os.Getenv("IPTV_TUNERR_STREAM_PROFILES_FILE")
	namedProfiles, profileMatrixErr := loadNamedProfilesFile(profileMatrixPath)
	if profileMatrixErr != nil {
		log.Printf("Named stream profiles disabled: load %q failed: %v", profileMatrixPath, profileMatrixErr)
	} else if len(namedProfiles) > 0 {
		log.Printf("Named stream profiles loaded: %d entries from %s", len(namedProfiles), profileMatrixPath)
	}
	overridePath := os.Getenv("IPTV_TUNERR_PROFILE_OVERRIDES_FILE")
	overrides, err := loadProfileOverridesFile(overridePath)
	if err != nil {
		log.Printf("Profile overrides disabled: load %q failed: %v", overridePath, err)
	} else if len(overrides) > 0 {
		log.Printf("Profile overrides loaded: %d entries from %s (default=%s)", len(overrides), overridePath, defaultProfile)
	} else {
		log.Printf("Profile overrides: none (default=%s)", defaultProfile)
	}
	txOverridePath := os.Getenv("IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE")
	txOverrides, txErr := loadTranscodeOverridesFile(txOverridePath)
	if txErr != nil {
		log.Printf("Transcode overrides disabled: load %q failed: %v", txOverridePath, txErr)
	} else if len(txOverrides) > 0 {
		log.Printf("Transcode overrides loaded: %d entries from %s", len(txOverrides), txOverridePath)
	}
	streamMode := strings.TrimSpace(s.StreamTranscodeMode)
	if streamMode == "" {
		// Fallback to process env so runtime overrides still work even if a caller
		// didn't thread config through correctly.
		streamMode = strings.TrimSpace(os.Getenv("IPTV_TUNERR_STREAM_TRANSCODE"))
	}
	gateway := &Gateway{
		Channels:            s.Channels,
		ProviderUser:        s.ProviderUser,
		ProviderPass:        s.ProviderPass,
		TunerCount:          s.TunerCount,
		StreamBufferBytes:   s.StreamBufferBytes,
		StreamTranscodeMode: streamMode,
		TranscodeOverrides:  txOverrides,
		DefaultProfile:      defaultProfile,
		ProfileOverrides:    overrides,
		NamedProfiles:       namedProfiles,
		CustomHeaders:       parseCustomHeaders(os.Getenv("IPTV_TUNERR_UPSTREAM_HEADERS")),
		CustomUserAgent:     strings.TrimSpace(os.Getenv("IPTV_TUNERR_UPSTREAM_USER_AGENT")),
		AddSecFetchHeaders:  envBool("IPTV_TUNERR_UPSTREAM_ADD_SEC_FETCH", false),
		DisableFFmpeg:       getenvBool("IPTV_TUNERR_FFMPEG_DISABLED", false),
		DisableFFmpegDNS:    getenvBool("IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE", false),
		CookieJarFile:       strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE")),
		FetchCFReject:       s.FetchCFReject,
		PlexPMSURL:          strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_URL")),
		PlexPMSToken:        strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_TOKEN")),
		PlexClientAdapt:     strings.EqualFold(strings.TrimSpace(os.Getenv("IPTV_TUNERR_CLIENT_ADAPT")), "1") || strings.EqualFold(strings.TrimSpace(os.Getenv("IPTV_TUNERR_CLIENT_ADAPT")), "true") || strings.EqualFold(strings.TrimSpace(os.Getenv("IPTV_TUNERR_CLIENT_ADAPT")), "yes"),
	}
	if store, err := loadAutopilotStore(strings.TrimSpace(s.AutopilotStateFile)); err != nil {
		log.Printf("Autopilot memory disabled: load %q failed: %v", s.AutopilotStateFile, err)
	} else {
		gateway.Autopilot = store
		if store != nil && strings.TrimSpace(s.AutopilotStateFile) != "" {
			log.Printf("Gateway Autopilot memory enabled: path=%q decisions=%d", s.AutopilotStateFile, len(store.byKey))
		}
	}
	// CF learned store: persists working UA per-host and CF-tagged flags across restarts.
	cfLearnedPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_CF_LEARNED_FILE"))
	if cfLearnedPath == "" {
		// Auto-derive from cookie jar path if set.
		if jar := strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE")); jar != "" {
			dir := strings.TrimSuffix(jar, "/"+filepath.Base(jar))
			if dir == jar {
				dir = filepath.Dir(jar)
			}
			cfLearnedPath = filepath.Join(dir, "cf-learned.json")
		}
	}
	gateway.cfLearnedStore = loadCFLearnedStore(cfLearnedPath)
	// Pre-populate learnedUAByHost from persisted CF learned store (survives restarts).
	for _, status := range gateway.cfLearnedStore.allStatuses() {
		if status.WorkingUA != "" {
			gateway.setLearnedUA(status.Host, status.WorkingUA)
		}
	}
	// Pre-populate learnedUAByHost from channel PreferredUA fields (set by catalog build after CF probe cycling).
	for _, ch := range gateway.Channels {
		if ch.PreferredUA == "" {
			continue
		}
		for _, u := range append([]string{ch.StreamURL}, ch.StreamURLs...) {
			if host := hostFromURL(u); host != "" {
				gateway.setLearnedUA(host, ch.PreferredUA)
			}
		}
	}
	// Per-host UA override: IPTV_TUNERR_HOST_UA=host1:vlc,host2:lavf
	// Lets operators pin a known-good UA per provider without waiting for cycling.
	if hostUARaw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HOST_UA")); hostUARaw != "" {
		count := 0
		for _, pair := range strings.Split(hostUARaw, ",") {
			pair = strings.TrimSpace(pair)
			host, preset, ok := strings.Cut(pair, ":")
			if !ok || strings.TrimSpace(host) == "" || strings.TrimSpace(preset) == "" {
				continue
			}
			host = strings.ToLower(strings.TrimSpace(host))
			// resolveUserAgentPreset is not available here (wrong package), so resolve inline.
			ua := resolveUserAgentPreset(strings.TrimSpace(preset), gateway.DetectedFFmpegUA)
			gateway.setLearnedUA(host, ua)
			count++
		}
		if count > 0 {
			log.Printf("Gateway host UA overrides applied: %d entries from IPTV_TUNERR_HOST_UA", count)
		}
	}
	// Stream attempt audit log: IPTV_TUNERR_STREAM_ATTEMPT_LOG=/path/to/attempts.jsonl
	if logFile := strings.TrimSpace(os.Getenv("IPTV_TUNERR_STREAM_ATTEMPT_LOG")); logFile != "" {
		gateway.StreamAttemptLogFile = logFile
		log.Printf("Gateway stream attempt audit log enabled: path=%q", logFile)
	}
	log.Printf("Gateway stream mode: transcode=%q buffer_bytes=%d", gateway.StreamTranscodeMode, gateway.StreamBufferBytes)
	if gateway.PlexClientAdapt {
		log.Printf("Gateway Plex client adapt enabled: pms_url=%q token_set=%t", gateway.PlexPMSURL, gateway.PlexPMSToken != "")
	}
	if len(gateway.CustomHeaders) > 0 {
		log.Printf("Gateway custom upstream headers enabled: %d headers", len(gateway.CustomHeaders))
	}
	if gateway.CustomUserAgent != "" {
		log.Printf("Gateway custom upstream User-Agent: %s", gateway.CustomUserAgent)
	}
	if gateway.AddSecFetchHeaders {
		log.Printf("Gateway upstream Sec-Fetch headers enabled")
	}
	if gateway.DisableFFmpeg {
		log.Printf("Gateway ffmpeg relay disabled by config")
	}
	if gateway.DisableFFmpegDNS {
		log.Printf("Gateway ffmpeg input DNS rewrite disabled")
	}
	if gateway.CookieJarFile != "" {
		jar, err := newPersistentCookieJar(gateway.CookieJarFile)
		if err != nil {
			log.Printf("Gateway persistent cookie jar disabled: %v", err)
		} else {
			gateway.persistentCookieJar = jar
			gateway.Client = httpclient.ForStreaming()
			gateway.Client.Jar = jar
			log.Printf("Gateway persistent cookie jar enabled: path=%q", gateway.CookieJarFile)
		}
	}
	if gateway.Client == nil {
		gateway.Client = httpclient.ForStreaming()
	}
	if gateway.DetectedFFmpegUA == "" {
		gateway.DetectedFFmpegUA = detectFFmpegLavfUA()
		if gateway.DetectedFFmpegUA != "" {
			log.Printf("Gateway detected ffmpeg Lavf UA: %s", gateway.DetectedFFmpegUA)
		}
	}
	gateway.AutoCFBoot = envBool("IPTV_TUNERR_CF_AUTO_BOOT", false)
	if gateway.AutoCFBoot {
		if gateway.persistentCookieJar == nil {
			log.Printf("Gateway CF auto-boot enabled but no cookie jar configured — clearance cookies won't persist across restarts; set IPTV_TUNERR_COOKIE_JAR_FILE")
		}
		uaCands := uaCycleCandidates(gateway.DetectedFFmpegUA)
		gateway.cfBoot = newCFBootstrapper(gateway.persistentCookieJar, uaCands)
		log.Printf("Gateway CF auto-boot enabled (UA candidates: %d)", len(uaCands))
		// Proactively refresh cf_clearance cookies before they expire.
		gateway.cfBoot.StartFreshnessMonitor(ctx, gateway.Client)
		// Pre-flight: ensure access for each unique provider host in the channel list.
		go func() {
			seen := make(map[string]bool)
			for _, ch := range gateway.Channels {
				urls := ch.StreamURLs
				if len(urls) == 0 && ch.StreamURL != "" {
					urls = []string{ch.StreamURL}
				}
				for _, u := range urls {
					host := hostFromURL(u)
					if host == "" || seen[host] {
						continue
					}
					seen[host] = true
					if ua := gateway.cfBoot.EnsureAccess(ctx, u, gateway.Client); ua != "" {
						gateway.setLearnedUA(host, ua)
					}
				}
			}
		}()
	}
	maybeStartPlexSessionReaper(ctx, gateway.Client)
	s.gateway = gateway
	cacheTTL := s.XMLTVCacheTTL
	if s.ProviderEPGCacheTTL > 0 {
		cacheTTL = s.ProviderEPGCacheTTL
	}
	xmltv := &XMLTV{
		Channels:                   s.Channels,
		EpgPruneUnlinked:           s.EpgPruneUnlinked,
		SourceURL:                  s.XMLTVSourceURL,
		SourceTimeout:              s.XMLTVTimeout,
		CacheTTL:                   cacheTTL,
		ProviderBaseURL:            s.ProviderBaseURL,
		ProviderUser:               s.ProviderUser,
		ProviderPass:               s.ProviderPass,
		ProviderEPGEnabled:         s.ProviderEPGEnabled,
		ProviderEPGTimeout:         s.ProviderEPGTimeout,
		ProviderEPGIncremental:     s.ProviderEPGIncremental,
		ProviderEPGLookaheadHours:  s.ProviderEPGLookaheadHours,
		ProviderEPGBackfillHours:   s.ProviderEPGBackfillHours,
		EpgStore:                   s.EpgStore,
		EpgRetainPastHours:         s.EpgSQLiteRetainPastHours,
		EpgVacuumAfterPrune:        s.EpgSQLiteVacuumAfterPrune,
		EpgMaxBytes:                s.EpgSQLiteMaxBytes,
		EpgSQLiteIncrementalUpsert: s.EpgSQLiteIncrementalUpsert,
		ProviderEPGURLSuffix:       s.ProviderEPGURLSuffix,
		ProviderEPGDiskCachePath:   s.ProviderEPGDiskCachePath,
		HDHRGuideURL:               s.HDHRGuideURL,
		HDHRGuideTimeout:           s.HDHRGuideTimeout,
	}
	s.xmltv = xmltv
	xmltv.StartRefresh(ctx)
	m3uServe := &M3UServe{BaseURL: s.BaseURL, Channels: s.Channels, EpgPruneUnlinked: s.EpgPruneUnlinked}
	s.m3uServe = m3uServe

	addr := s.Addr
	if addr == "" {
		addr = ":5004"
	}

	if envBool("IPTV_TUNERR_SSDP_DISABLED", false) {
		log.Printf("SSDP disabled via IPTV_TUNERR_SSDP_DISABLED")
	} else {
		StartSSDP(ctx, addr, s.BaseURL, s.DeviceID)
	}

	mux := http.NewServeMux()
	mux.Handle("/discover.json", hdhr)
	mux.Handle("/lineup_status.json", hdhr)
	mux.Handle("/lineup.json", hdhr)
	mux.Handle("/device.xml", s.serveDeviceXML())
	mux.Handle("/guide.xml", xmltv)
	mux.Handle("/guide/health.json", s.serveGuideHealth())
	mux.Handle("/guide/policy.json", s.serveGuidePolicy())
	mux.Handle("/guide/doctor.json", s.serveEPGDoctor())
	mux.Handle("/guide/aliases.json", s.serveSuggestedAliasOverrides())
	mux.Handle("/guide/highlights.json", s.serveGuideHighlights())
	mux.Handle("/guide/epg-store.json", s.serveEpgStoreReport())
	mux.Handle("/guide/capsules.json", s.serveCatchupCapsules())
	mux.Handle("/live.m3u", m3uServe)
	mux.Handle("/stream/", gateway)
	mux.Handle("/healthz", s.serveHealth())
	mux.Handle("/readyz", s.serveReady())
	mux.Handle("/ui/guide-preview.json", s.serveOperatorGuidePreviewJSON())
	mux.HandleFunc("/ui/guide", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/guide/", http.StatusSeeOther)
	})
	mux.Handle("/ui/guide/", s.serveOperatorGuidePreviewPage())
	mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/", http.StatusSeeOther)
	})
	mux.Handle("/ui/", s.serveOperatorUI())
	mux.Handle("/channels/report.json", s.serveChannelReport())
	mux.Handle("/channels/leaderboard.json", s.serveChannelLeaderboard())
	mux.Handle("/channels/dna.json", s.serveChannelDNAReport())
	mux.Handle("/autopilot/report.json", s.serveAutopilotReport())
	mux.Handle("/plex/ghost-report.json", s.serveGhostHunterReport())
	mux.Handle("/provider/profile.json", s.serveProviderProfile())
	mux.Handle("/recordings/recorder.json", s.serveCatchupRecorderReport())
	mux.Handle("/debug/stream-attempts.json", s.serveRecentStreamAttempts())
	mux.Handle("/debug/runtime.json", s.serveRuntimeSnapshot())
	mux.Handle("/debug/hls-mux-demo.html", s.serveHlsMuxWebDemo())
	if metricsEnableFromEnv() {
		promRegisterAutopilotMetrics(gateway)
		mux.Handle("/metrics", promhttp.Handler())
	}
	mux.Handle("/ops/actions/mux-seg-decode", s.serveMuxSegDecodeAction())
	mux.Handle("/ops/actions/status.json", s.serveOperatorActionStatus())
	mux.Handle("/ops/workflows/guide-repair.json", s.serveGuideRepairWorkflow())
	mux.Handle("/ops/workflows/stream-investigate.json", s.serveStreamInvestigateWorkflow())
	mux.Handle("/ops/workflows/ops-recovery.json", s.serveOpsRecoveryWorkflow())
	mux.Handle("/ops/actions/guide-refresh", s.serveGuideRefreshAction())
	mux.Handle("/ops/actions/stream-attempts-clear", s.serveStreamAttemptsClearAction())
	mux.Handle("/ops/actions/provider-profile-reset", s.serveProviderProfileResetAction())
	mux.Handle("/ops/actions/autopilot-reset", s.serveAutopilotResetAction())

	srv := &http.Server{Addr: addr, Handler: logRequests(mux)}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Tuner listening on %s (BaseURL %s)", addr, s.BaseURL)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		log.Print("Shutting down tuner ...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Tuner shutdown: %v", err)
		}
		<-serverErr
		return nil
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (w *loggingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *loggingResponseWriter) ResponseStarted() bool {
	return w.status != 0 || w.bytes > 0
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(lw, r)
		status := lw.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Printf(
			"http: %s %s status=%d bytes=%d dur=%s ua=%q remote=%s",
			r.Method, r.URL.Path, status, lw.bytes, time.Since(start).Round(time.Millisecond), r.UserAgent(), r.RemoteAddr,
		)
	})
}

// serveHealth returns an http.Handler for GET /healthz.
// Returns 200 {"status":"ok",...} once channels have been loaded, 503 {"status":"loading"} before.
func (s *Server) serveHealth() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := s.healthStatusPayload()
		if ready, _ := body["source_ready"].(bool); !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		writeJSONStatusBody(w, body)
	})
}

// serveReady returns an http.Handler for GET /readyz.
// Returns 200 {"status":"ready",...} once channels have been loaded, 503 {"status":"not_ready"} before.
func (s *Server) serveReady() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := s.healthStatusPayload()
		ready, _ := body["source_ready"].(bool)
		if ready {
			body["status"] = "ready"
			writeJSONStatusBody(w, body)
			return
		}
		body["status"] = "not_ready"
		body["reason"] = "channels not loaded"
		w.WriteHeader(http.StatusServiceUnavailable)
		writeJSONStatusBody(w, body)
	})
}

func (s *Server) healthStatusPayload() map[string]interface{} {
	s.healthMu.RLock()
	count := s.healthChannels
	lastRefresh := s.healthRefresh
	s.healthMu.RUnlock()

	body := map[string]interface{}{
		"status":       "ok",
		"source_ready": count > 0,
		"channels":     count,
	}
	if count == 0 {
		body["status"] = "loading"
		return body
	}
	body["last_refresh"] = lastRefresh.Format(time.RFC3339)
	return body
}

func writeJSONStatusBody(w http.ResponseWriter, body map[string]interface{}) {
	encoded, err := json.Marshal(body)
	if err != nil {
		http.Error(w, `{"error":"encode status"}`, http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(encoded)
}

// epgStoreReportJSON is returned by GET /guide/epg-store.json when IPTV_TUNERR_EPG_SQLITE_PATH is set.
type epgStoreReportJSON struct {
	SchemaVersion      int              `json:"schema_version"`
	SourceReady        bool             `json:"source_ready"`
	LastSyncUTC        string           `json:"last_sync_utc,omitempty"`
	ProgrammeCount     int              `json:"programme_count"`
	ChannelCount       int              `json:"channel_count"`
	GlobalMaxStopUnix  int64            `json:"global_max_stop_unix"`
	ChannelMaxStopUnix map[string]int64 `json:"channel_max_stop_unix,omitempty"`
	RetainPastHours    int              `json:"retain_past_hours,omitempty"`
	VacuumAfterPrune   bool             `json:"vacuum_after_prune,omitempty"`
	MaxBytes           int64            `json:"max_bytes,omitempty"`
	DbFileBytes        int64            `json:"db_file_bytes,omitempty"`
	DbFileModifiedUTC  string           `json:"db_file_modified_utc,omitempty"`
	// IncrementalUpsert reflects IPTV_TUNERR_EPG_SQLITE_INCREMENTAL_UPSERT (overlap-window sync).
	IncrementalUpsert bool `json:"incremental_upsert,omitempty"`
	// ProviderEPGIncremental reflects IPTV_TUNERR_PROVIDER_EPG_INCREMENTAL (suffix token rendering).
	ProviderEPGIncremental bool `json:"provider_epg_incremental,omitempty"`
}

func (s *Server) serveEpgStoreReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if s.EpgStore == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"epg sqlite disabled (set IPTV_TUNERR_EPG_SQLITE_PATH)"}`))
			return
		}
		prog, ch, err := s.EpgStore.RowCounts()
		if err != nil {
			http.Error(w, `{"error":"epg store stats"}`, http.StatusInternalServerError)
			return
		}
		lastSync, _ := s.EpgStore.MetaLastSyncUTC()
		gmax, err := s.EpgStore.GlobalMaxStopUnix()
		if err != nil {
			http.Error(w, `{"error":"epg store max stop"}`, http.StatusInternalServerError)
			return
		}
		detail := false
		if raw := strings.TrimSpace(r.URL.Query().Get("detail")); raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "yes") {
			detail = true
		}
		rep := epgStoreReportJSON{
			SchemaVersion:          s.EpgStore.SchemaVersion(),
			SourceReady:            prog > 0 || ch > 0,
			LastSyncUTC:            lastSync,
			ProgrammeCount:         prog,
			ChannelCount:           ch,
			GlobalMaxStopUnix:      gmax,
			RetainPastHours:        s.EpgSQLiteRetainPastHours,
			VacuumAfterPrune:       s.EpgSQLiteVacuumAfterPrune,
			MaxBytes:               s.EpgSQLiteMaxBytes,
			IncrementalUpsert:      s.EpgSQLiteIncrementalUpsert,
			ProviderEPGIncremental: s.ProviderEPGIncremental,
		}
		if sz, mod, err := s.EpgStore.DBFileStat(); err == nil {
			rep.DbFileBytes = sz
			rep.DbFileModifiedUTC = mod.UTC().Format(time.RFC3339)
		}
		if detail {
			m, err := s.EpgStore.MaxStopUnixPerChannel()
			if err != nil {
				http.Error(w, `{"error":"epg store per-channel max"}`, http.StatusInternalServerError)
				return
			}
			rep.ChannelMaxStopUnix = m
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode epg store report"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveChannelReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rep := channelreport.Build(s.Channels)
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode channel report"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveChannelLeaderboard() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		limit := 10
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		body, err := json.MarshalIndent(channelreport.BuildLeaderboard(s.Channels, limit), "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode channel leaderboard"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveChannelDNAReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, err := json.MarshalIndent(channeldna.BuildReport(s.Channels), "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode dna report"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveAutopilotReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		limit := 10
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		var rep AutopilotReport
		if s.gateway != nil && s.gateway.Autopilot != nil {
			rep = s.gateway.Autopilot.report(limit)
		} else {
			rep = AutopilotReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode autopilot report"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveGuideHighlights() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.xmltv == nil {
			http.Error(w, `{"error":"xmltv unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		soonWindow := 30 * time.Minute
		if raw := strings.TrimSpace(r.URL.Query().Get("soon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil {
				soonWindow = d
			}
		}
		limit := 12
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		rep, err := s.xmltv.GuideHighlights(time.Now(), soonWindow, limit)
		if err != nil {
			http.Error(w, `{"error":"guide highlights failed"}`, http.StatusBadGateway)
			return
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode guide highlights"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveCatchupCapsules() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.xmltv == nil {
			http.Error(w, `{"error":"xmltv unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		horizon := 3 * time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil {
				horizon = d
			}
		}
		limit := 20
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		policy := strings.TrimSpace(r.URL.Query().Get("policy"))
		if policy == "" {
			policy = strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_GUIDE_POLICY"))
		}
		replayTemplate := strings.TrimSpace(r.URL.Query().Get("replay_template"))
		if replayTemplate == "" {
			replayTemplate = strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE"))
		}
		rep, err := s.xmltv.CatchupCapsulePreview(time.Now(), horizon, limit)
		if err != nil {
			http.Error(w, `{"error":"catchup capsule preview failed"}`, http.StatusBadGateway)
			return
		}
		if policy != "" {
			if gh, ok := s.xmltv.cachedGuideHealthReport(); ok {
				rep = FilterCatchupCapsulesByGuidePolicy(rep, gh, policy)
			}
		}
		rep = ApplyCatchupReplayTemplate(rep, replayTemplate)
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode catchup capsules"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveGuidePolicy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.xmltv == nil {
			http.Error(w, `{"error":"xmltv unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		policy := normalizeGuidePolicy(strings.TrimSpace(r.URL.Query().Get("policy")))
		if policy == "off" {
			if raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_GUIDE_POLICY")); raw != "" {
				policy = normalizeGuidePolicy(raw)
			}
		}
		report, ok := s.xmltv.guidePolicyReport(s.xmltv.Channels, policy)
		if !ok && report.Summary.Policy == "" {
			report.Summary.Policy = policy
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode guide policy"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveGhostHunterReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cfg := NewGhostHunterConfigFromEnv()
		if raw := strings.TrimSpace(r.URL.Query().Get("observe")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil {
				cfg.ObserveWindow = d
			}
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("poll")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil {
				cfg.PollInterval = d
			}
		}
		stop := false
		if raw := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("stop"))); raw != "" {
			stop = raw == "1" || raw == "true" || raw == "yes" || raw == "on"
		}
		rep, err := RunGhostHunter(r.Context(), cfg, stop, nil)
		if err != nil {
			http.Error(w, `{"error":"ghost hunter failed"}`, http.StatusBadGateway)
			return
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode ghost report"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProviderProfile() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.gateway == nil {
			http.Error(w, `{"error":"gateway unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		body, err := json.MarshalIndent(s.gateway.ProviderBehaviorProfile(), "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode provider profile"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveRecentStreamAttempts() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.gateway == nil {
			http.Error(w, `{"error":"gateway unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		rep := s.gateway.RecentStreamAttempts(streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 10))
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode stream attempts"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveOperatorActionStatus() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		detail := map[string]interface{}{
			"guide_refresh": map[string]interface{}{
				"available": s.xmltv != nil,
				"status":    s.xmltv.RefreshStatus(),
			},
			"stream_attempts_clear": map[string]interface{}{
				"available": s.gateway != nil,
			},
			"provider_profile_reset": map[string]interface{}{
				"available": s.gateway != nil,
			},
			"autopilot_reset": map[string]interface{}{
				"available": s.gateway != nil && s.gateway.Autopilot != nil,
			},
			"mux_seg_decode": map[string]interface{}{
				"available":    true,
				"endpoint":     "/ops/actions/mux-seg-decode",
				"method":       "POST",
				"body":         `{"seg_b64":"<base64 of raw seg URL>"}`,
				"localhost_ui": true,
			},
		}
		body, err := json.MarshalIndent(detail, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode operator actions"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveGuideRepairWorkflow() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		gh := map[string]interface{}{}
		if s.xmltv != nil {
			if rep, err := s.xmltv.GuideHealth(time.Now(), ""); err == nil {
				gh["guide_health"] = rep.Summary
			}
		}
		report := OperatorWorkflowReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Name:        "guide_repair",
			Summary:     gh,
			Steps: []string{
				"Inspect guide health and doctor output for stale or placeholder-only channels.",
				"Run a manual guide refresh if the cache or upstream source looks stale.",
				"Check provider EPG incremental/disk-cache settings in runtime snapshot.",
				"Inspect alias and doctor payloads before changing XMLTV matching inputs.",
			},
			Actions: []string{
				"/ops/actions/guide-refresh",
				"/guide/health.json",
				"/guide/doctor.json",
				"/guide/aliases.json",
				"/debug/runtime.json",
			},
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode guide workflow"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveStreamInvestigateWorkflow() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		attempts := StreamAttemptReport{}
		providerProfile := ProviderBehaviorProfile{}
		if s.gateway != nil {
			attempts = s.gateway.RecentStreamAttempts(5)
			providerProfile = s.gateway.ProviderBehaviorProfile()
		}
		report := OperatorWorkflowReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Name:        "stream_investigate",
			Summary: map[string]interface{}{
				"recent_attempt_count": attempts.Count,
				"provider_profile":     providerProfile,
			},
			Steps: []string{
				"Start from recent stream attempts and identify the failing host, profile, and outcome.",
				"Check provider profile penalties, CF hits, and learned tuner limits.",
				"Inspect runtime settings for transcode mode, strip-hosts, and provider blocking policy.",
				"Clear volatile attempt history or provider penalties only when you want a fresh comparison pass.",
			},
			Actions: []string{
				"/ops/actions/stream-attempts-clear",
				"/ops/actions/provider-profile-reset",
				"/ops/actions/autopilot-reset",
				"/debug/stream-attempts.json",
				"/provider/profile.json",
				"/autopilot/report.json",
				"/debug/runtime.json",
			},
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode stream workflow"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveOpsRecoveryWorkflow() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")

		recorderSummary := map[string]interface{}{}
		if stateFile := strings.TrimSpace(firstNonEmptyString(s.RecorderStateFile, os.Getenv("IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE"))); stateFile != "" {
			if rep, err := LoadCatchupRecorderReport(stateFile, 5); err == nil {
				recorderSummary["active_count"] = len(rep.Active)
				recorderSummary["completed_count"] = len(rep.Completed)
				recorderSummary["failed_count"] = len(rep.Failed)
				recorderSummary["interrupted_count"] = rep.InterruptedCount
				recorderSummary["published_count"] = rep.PublishedCount
				recorderSummary["state_file"] = rep.StateFile
			} else {
				recorderSummary["error"] = err.Error()
			}
		} else {
			recorderSummary["state_file"] = ""
		}

		ghostSummary := map[string]interface{}{}
		if rep, err := RunGhostHunter(r.Context(), NewGhostHunterConfigFromEnv(), false, nil); err == nil {
			ghostSummary["session_count"] = rep.SessionCount
			ghostSummary["stale_count"] = rep.StaleCount
			ghostSummary["hidden_grab_suspected"] = rep.HiddenGrabSuspected
			ghostSummary["recommended_action"] = rep.RecommendedAction
		} else {
			ghostSummary["error"] = err.Error()
		}

		autopilotSummary := map[string]interface{}{}
		if s.gateway != nil && s.gateway.Autopilot != nil {
			rep := s.gateway.Autopilot.report(5)
			autopilotSummary["decision_count"] = rep.DecisionCount
			autopilotSummary["hot_channel_count"] = len(rep.HotChannels)
			autopilotSummary["state_file"] = rep.StateFile
		} else {
			autopilotSummary["decision_count"] = 0
		}

		report := OperatorWorkflowReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Name:        "ops_recovery",
			Summary: map[string]interface{}{
				"recorder":  recorderSummary,
				"ghost":     ghostSummary,
				"autopilot": autopilotSummary,
			},
			Steps: []string{
				"Check recorder failures and interrupted items before assuming the recording lane is healthy.",
				"Inspect Ghost Hunter when playback symptoms smell like stale Plex session state rather than upstream failures.",
				"Review Autopilot memory when the gateway keeps preferring a stale profile or host path.",
				"Reset Autopilot memory only after you have captured the current evidence and want a clean learning pass.",
			},
			Actions: []string{
				"/ops/actions/autopilot-reset",
				"/recordings/recorder.json",
				"/plex/ghost-report.json?observe=0s",
				"/autopilot/report.json",
				"/debug/runtime.json",
			},
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode ops workflow"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func writeOperatorActionJSON(w http.ResponseWriter, status int, rep OperatorActionResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body, _ := json.MarshalIndent(rep, "", "  ")
	_, _ = w.Write(body)
}

func (s *Server) serveGuideRefreshAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.xmltv == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "guide_refresh", Message: "xmltv unavailable"})
			return
		}
		if !s.xmltv.TriggerRefresh("operator_action") {
			writeOperatorActionJSON(w, http.StatusConflict, OperatorActionResponse{OK: false, Action: "guide_refresh", Message: "refresh already in progress", Detail: s.xmltv.RefreshStatus()})
			return
		}
		writeOperatorActionJSON(w, http.StatusAccepted, OperatorActionResponse{OK: true, Action: "guide_refresh", Message: "guide refresh started", Detail: s.xmltv.RefreshStatus()})
	})
}

func (s *Server) serveStreamAttemptsClearAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.gateway == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "stream_attempts_clear", Message: "gateway unavailable"})
			return
		}
		n := s.gateway.ClearRecentStreamAttempts()
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "stream_attempts_clear", Message: "recent stream attempts cleared", Detail: map[string]int{"cleared": n}})
	})
}

func (s *Server) serveProviderProfileResetAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.gateway == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "provider_profile_reset", Message: "gateway unavailable"})
			return
		}
		s.gateway.ResetProviderBehaviorProfile()
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "provider_profile_reset", Message: "provider behavior profile reset", Detail: s.gateway.ProviderBehaviorProfile()})
	})
}

func (s *Server) serveAutopilotResetAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.gateway == nil || s.gateway.Autopilot == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "autopilot_reset", Message: "autopilot unavailable"})
			return
		}
		if err := s.gateway.Autopilot.reset(); err != nil {
			writeOperatorActionJSON(w, http.StatusInternalServerError, OperatorActionResponse{OK: false, Action: "autopilot_reset", Message: err.Error()})
			return
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "autopilot_reset", Message: "autopilot memory cleared"})
	})
}

func (s *Server) serveRuntimeSnapshot() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rep := s.RuntimeSnapshot
		if rep == nil {
			rep = &RuntimeSnapshot{
				GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
				Version:      s.AppVersion,
				BaseURL:      s.BaseURL,
				DeviceID:     s.DeviceID,
				FriendlyName: s.FriendlyName,
			}
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode runtime snapshot"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveCatchupRecorderReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stateFile := strings.TrimSpace(s.RecorderStateFile)
		if stateFile == "" {
			stateFile = strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE"))
		}
		if stateFile == "" {
			http.Error(w, `{"error":"recorder state unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		rep, err := LoadCatchupRecorderReport(stateFile, streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 10))
		if err != nil {
			http.Error(w, `{"error":"load recorder report failed"}`, http.StatusBadGateway)
			return
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode recorder report"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveHlsMuxWebDemo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !getenvBool("IPTV_TUNERR_HLS_MUX_WEB_DEMO", false) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		b, err := operatorUIEmbedded.ReadFile("static/hls_mux_demo.html")
		if err != nil {
			http.Error(w, "demo unavailable", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(b)
	})
}

func (s *Server) serveMuxSegDecodeAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		var req struct {
			SegB64 string `json:"seg_b64"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.SegB64))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"invalid base64"}`, http.StatusBadRequest)
			return
		}
		u := strings.TrimSpace(string(raw))
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(map[string]interface{}{
			"redacted_url": safeurl.RedactURL(u),
			"http_ok":      safeurl.IsHTTPOrHTTPS(u),
		})
	})
}

func (s *Server) serveDeviceXML() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceID := s.DeviceID
		if deviceID == "" {
			deviceID = "iptvtunerr01"
		}
		friendlyName := "IPTV Tunerr"
		deviceXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
    <friendlyName>%s</friendlyName>
    <manufacturer>IPTV Tunerr</manufacturer>
    <modelName>IPTV Tunerr</modelName>
    <UDN>uuid:%s</UDN>
  </device>
</root>`, friendlyName, deviceID)
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(deviceXML))
	})
}

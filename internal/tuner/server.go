package tuner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
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
	"github.com/snapetech/iptvtunerr/internal/entitlements"
	"github.com/snapetech/iptvtunerr/internal/epgstore"
	"github.com/snapetech/iptvtunerr/internal/eventhooks"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/plexharvest"
	"github.com/snapetech/iptvtunerr/internal/programming"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
	"github.com/snapetech/iptvtunerr/internal/virtualchannels"
)

// PlexDVRMaxChannels is Plex's per-tuner channel limit when using the wizard; exceeding it causes "failed to save channel lineup".
const PlexDVRMaxChannels = 480

// PlexDVRWizardSafeMax is used in "easy" mode: strip from end so lineup fits when Plex suggests a guide (e.g. Rogers West Canada ~680 ch); keep first N.
const PlexDVRWizardSafeMax = 479

var timeNow = time.Now

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
	AppVersion            string
	StreamBufferBytes     int    // 0 = no buffer; -1 = auto; e.g. 2097152 for 2 MiB
	StreamTranscodeMode   string // "off" | "on" | "auto"
	AutopilotStateFile    string // optional JSON file for remembered dna_id+client_class playback decisions
	RecorderStateFile     string // optional JSON file written by catchup-daemon for recorder status/reporting
	RecordingRulesFile    string // optional JSON file for durable recording rule configuration
	Movies                []catalog.Movie
	Series                []catalog.Series
	Channels              []catalog.LiveChannel
	RawChannels           []catalog.LiveChannel
	ProgrammingRecipeFile string
	ProgrammingRecipe     programming.Recipe
	PlexLineupHarvestFile string
	PlexLineupHarvest     plexharvest.Report
	VirtualChannelsFile   string
	VirtualChannels       virtualchannels.Ruleset
	RecordingRules        RecordingRuleset
	EventHooksFile        string
	EventHooks            *eventhooks.Dispatcher
	XtreamOutputUser      string
	XtreamOutputPass      string
	XtreamUsersFile       string
	XtreamEntitlements    entitlements.Ruleset
	ProviderUser          string
	ProviderPass          string
	ProviderBaseURL       string
	XMLTVSourceURL        string
	XMLTVTimeout          time.Duration
	XMLTVCacheTTL         time.Duration // 0 = use default 10m
	EpgPruneUnlinked      bool          // when true, guide.xml and /live.m3u only include channels with tvg-id
	EpgForceLineupMatch   bool          // when true, guide.xml keeps every lineup row even if prune-unlinked is enabled
	FetchCFReject         bool          // abort HLS stream if segment redirected to CF abuse page (passed to Gateway)
	ProviderEPGEnabled    bool
	ProviderEPGTimeout    time.Duration
	ProviderEPGCacheTTL   time.Duration
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
	Events        map[string]interface{} `json:"events,omitempty"`
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

var runGhostHunterAction = RunGhostHunter
var runGhostHunterRecoveryAction = RunGhostHunterRecoveryHelper
var runChannelDiffHarnessAction = func(ctx context.Context, env map[string]string) (map[string]interface{}, error) {
	return runDiagnosticsHarnessAction(ctx, "channel-diff-harness.sh", ".diag/channel-diff", env)
}
var runStreamCompareHarnessAction = func(ctx context.Context, env map[string]string) (map[string]interface{}, error) {
	return runDiagnosticsHarnessAction(ctx, "stream-compare-harness.sh", ".diag/stream-compare", env)
}

// UpdateChannels updates the channel list for all handlers so -refresh can serve new lineup without restart.
// Caps at LineupMaxChannels (default PlexDVRMaxChannels) so Plex DVR can save the lineup when using the wizard (Plex fails above ~480).
// When LineupMaxChannels is NoLineupCap, no cap is applied (for programmatic lineup sync; see -register-plex).
func (s *Server) UpdateChannels(live []catalog.LiveChannel) {
	live = applyLineupBaseFilters(live)
	if s.xmltv != nil {
		live = s.xmltv.applyGuidePolicyToChannels(live, os.Getenv("IPTV_TUNERR_GUIDE_POLICY"))
	}
	live = applyDNAPolicy(live, os.Getenv("IPTV_TUNERR_DNA_POLICY"))
	s.RawChannels = cloneLiveChannels(live)
	live = s.applyProgrammingRecipe(live)
	live = applyLineupRecipe(live)
	live = applyLineupWizardShape(live)
	live = applyLineupShard(live)
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
	s.setExposedChannels(live)
}

func (s *Server) setExposedChannels(live []catalog.LiveChannel) {
	summary := summarizeLineupIntegrity(live)
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
		if len(live) > 0 {
			s.xmltv.TriggerRefresh("lineup_update")
		}
	}
	if s.m3uServe != nil {
		s.m3uServe.Channels = live
	}
	log.Printf(
		"Lineup updated: channels=%d epg_linked=%d with_tvg=%d with_stream=%d missing_core=%d duplicate_guide_numbers=%d duplicate_channel_ids=%d",
		summary.Total,
		summary.EPGLinked,
		summary.WithTVGID,
		summary.WithStream,
		summary.MissingCoreFields,
		summary.DuplicateGuideNumbers,
		summary.DuplicateChannelIDs,
	)
	if s.EventHooks != nil {
		s.EventHooks.Dispatch("lineup.updated", "server", map[string]interface{}{
			"channels":                summary.Total,
			"epg_linked":              summary.EPGLinked,
			"with_tvg":                summary.WithTVGID,
			"with_stream":             summary.WithStream,
			"missing_core":            summary.MissingCoreFields,
			"duplicate_guide_numbers": summary.DuplicateGuideNumbers,
			"duplicate_channel_ids":   summary.DuplicateChannelIDs,
			"raw_channels":            len(s.RawChannels),
			"programming_recipe_file": strings.TrimSpace(s.ProgrammingRecipeFile),
			"guide_number_offset":     s.GuideNumberOffset,
			"lineup_max_channels":     s.LineupMaxChannels,
		})
	}
}

func (s *Server) reloadProgrammingRecipe() programming.Recipe {
	path := strings.TrimSpace(s.ProgrammingRecipeFile)
	if path == "" {
		s.ProgrammingRecipe = programming.NormalizeRecipe(s.ProgrammingRecipe)
		return s.ProgrammingRecipe
	}
	recipe, err := programming.LoadRecipeFile(path)
	if err != nil {
		log.Printf("Programming recipe disabled: load %q failed: %v", path, err)
		return s.ProgrammingRecipe
	}
	s.ProgrammingRecipe = recipe
	return recipe
}

func (s *Server) applyProgrammingRecipe(live []catalog.LiveChannel) []catalog.LiveChannel {
	recipe := s.reloadProgrammingRecipe()
	return programming.ApplyRecipe(live, recipe)
}

func (s *Server) reloadPlexLineupHarvest() plexharvest.Report {
	path := strings.TrimSpace(s.PlexLineupHarvestFile)
	if path == "" {
		return s.PlexLineupHarvest
	}
	rep, err := plexharvest.LoadReportFile(path)
	if err != nil {
		log.Printf("Plex lineup harvest disabled: load %q failed: %v", path, err)
		return s.PlexLineupHarvest
	}
	s.PlexLineupHarvest = rep
	return rep
}

func (s *Server) savePlexLineupHarvest(rep plexharvest.Report) (plexharvest.Report, error) {
	path := strings.TrimSpace(s.PlexLineupHarvestFile)
	if path == "" {
		s.PlexLineupHarvest = rep
		return rep, nil
	}
	saved, err := plexharvest.SaveReportFile(path, rep)
	if err != nil {
		return plexharvest.Report{}, err
	}
	s.PlexLineupHarvest = saved
	return saved, nil
}

func (s *Server) reloadVirtualChannels() virtualchannels.Ruleset {
	path := strings.TrimSpace(s.VirtualChannelsFile)
	if path == "" {
		s.VirtualChannels = virtualchannels.NormalizeRuleset(s.VirtualChannels)
		return s.VirtualChannels
	}
	set, err := virtualchannels.LoadFile(path)
	if err != nil {
		log.Printf("Virtual channels disabled: load %q failed: %v", path, err)
		return s.VirtualChannels
	}
	s.VirtualChannels = set
	return set
}

func (s *Server) saveVirtualChannels(set virtualchannels.Ruleset) (virtualchannels.Ruleset, error) {
	path := strings.TrimSpace(s.VirtualChannelsFile)
	if path == "" {
		s.VirtualChannels = virtualchannels.NormalizeRuleset(set)
		return s.VirtualChannels, nil
	}
	saved, err := virtualchannels.SaveFile(path, set)
	if err != nil {
		return virtualchannels.Ruleset{}, err
	}
	s.VirtualChannels = saved
	return saved, nil
}

func (s *Server) reloadRecordingRules() RecordingRuleset {
	path := strings.TrimSpace(s.RecordingRulesFile)
	if path == "" {
		s.RecordingRules = normalizeRecordingRuleset(s.RecordingRules)
		return s.RecordingRules
	}
	set, err := loadRecordingRulesFile(path)
	if err != nil {
		log.Printf("Recording rules disabled: load %q failed: %v", path, err)
		return s.RecordingRules
	}
	s.RecordingRules = set
	return set
}

func (s *Server) saveRecordingRules(set RecordingRuleset) (RecordingRuleset, error) {
	path := strings.TrimSpace(s.RecordingRulesFile)
	if path == "" {
		s.RecordingRules = normalizeRecordingRuleset(set)
		return s.RecordingRules, nil
	}
	saved, err := saveRecordingRulesFile(path, set)
	if err != nil {
		return RecordingRuleset{}, err
	}
	s.RecordingRules = saved
	if s.EventHooks != nil {
		s.EventHooks.Dispatch("recording_rules.updated", "server", map[string]interface{}{
			"rule_count": len(saved.Rules),
			"rules_file": path,
		})
	}
	return saved, nil
}

func (s *Server) rebuildCuratedChannelsFromRaw() {
	live := cloneLiveChannels(s.RawChannels)
	live = s.applyProgrammingRecipe(live)
	live = applyLineupRecipe(live)
	live = applyLineupWizardShape(live)
	live = applyLineupShard(live)
	if s.LineupMaxChannels != NoLineupCap {
		max := s.LineupMaxChannels
		if max <= 0 {
			max = PlexDVRMaxChannels
		}
		if len(live) > max {
			live = live[:max]
		}
	}
	live = applyGuideNumberOffset(live, s.GuideNumberOffset)
	s.setExposedChannels(live)
}

func cloneLiveChannels(live []catalog.LiveChannel) []catalog.LiveChannel {
	out := make([]catalog.LiveChannel, len(live))
	copy(out, live)
	return out
}

type lineupIntegritySummary struct {
	Total                 int
	EPGLinked             int
	WithTVGID             int
	WithStream            int
	MissingCoreFields     int
	DuplicateGuideNumbers int
	DuplicateChannelIDs   int
}

func summarizeLineupIntegrity(live []catalog.LiveChannel) lineupIntegritySummary {
	s := lineupIntegritySummary{Total: len(live)}
	guideNumbers := make(map[string]int, len(live))
	channelIDs := make(map[string]int, len(live))
	for _, ch := range live {
		if ch.EPGLinked {
			s.EPGLinked++
		}
		if strings.TrimSpace(ch.TVGID) != "" {
			s.WithTVGID++
		}
		if strings.TrimSpace(ch.StreamURL) != "" || len(ch.StreamURLs) > 0 {
			s.WithStream++
		}
		if strings.TrimSpace(ch.ChannelID) == "" || strings.TrimSpace(ch.GuideNumber) == "" || strings.TrimSpace(ch.GuideName) == "" {
			s.MissingCoreFields++
		}
		if guide := strings.TrimSpace(ch.GuideNumber); guide != "" {
			guideNumbers[guide]++
		}
		if cid := strings.TrimSpace(ch.ChannelID); cid != "" {
			channelIDs[cid]++
		}
	}
	for _, n := range guideNumbers {
		if n > 1 {
			s.DuplicateGuideNumbers++
		}
	}
	for _, n := range channelIDs {
		if n > 1 {
			s.DuplicateChannelIDs++
		}
	}
	return s
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

func applyLineupBaseFilters(live []catalog.LiveChannel) []catalog.LiveChannel {
	if len(live) == 0 {
		return live
	}
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
	return out
}

func applyLineupPreCapFilters(live []catalog.LiveChannel) []catalog.LiveChannel {
	out := applyLineupBaseFilters(live)
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

func (s *Server) xtreamOutputEnabled() bool {
	return (strings.TrimSpace(s.XtreamOutputUser) != "" && strings.TrimSpace(s.XtreamOutputPass) != "") ||
		strings.TrimSpace(s.XtreamUsersFile) != ""
}

func (s *Server) reloadXtreamEntitlements() entitlements.Ruleset {
	path := strings.TrimSpace(s.XtreamUsersFile)
	if path == "" {
		s.XtreamEntitlements = entitlements.NormalizeRuleset(s.XtreamEntitlements)
		return s.XtreamEntitlements
	}
	set, err := entitlements.LoadFile(path)
	if err != nil {
		log.Printf("Xtream entitlements disabled: load %q failed: %v", path, err)
		return s.XtreamEntitlements
	}
	s.XtreamEntitlements = set
	return set
}

func (s *Server) saveXtreamEntitlements(set entitlements.Ruleset) (entitlements.Ruleset, error) {
	path := strings.TrimSpace(s.XtreamUsersFile)
	if path == "" {
		return entitlements.Ruleset{}, fmt.Errorf("xtream users file not configured")
	}
	saved, err := entitlements.SaveFile(path, set)
	if err == nil {
		s.XtreamEntitlements = saved
	}
	return saved, err
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
		EventHooks:          s.EventHooks,
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
	accountLimitPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_STATE_FILE"))
	if accountLimitPath == "" {
		if jar := strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE")); jar != "" {
			dir := strings.TrimSuffix(jar, "/"+filepath.Base(jar))
			if dir == jar {
				dir = filepath.Dir(jar)
			}
			accountLimitPath = filepath.Join(dir, "provider-account-limits.json")
		}
	}
	gateway.accountLimitStore = loadAccountLimitStore(accountLimitPath, providerAccountLimitTTL())
	gateway.restoreProviderAccountLearnedLimits(gateway.accountLimitStore.snapshot())
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
		EpgForceLineupMatch:        s.EpgForceLineupMatch,
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
	if s.xtreamOutputEnabled() {
		mux.Handle("/player_api.php", s.serveXtreamPlayerAPI())
		mux.Handle("/live/", s.serveXtreamLiveProxy())
		mux.Handle("/movie/", s.serveXtreamMovieProxy())
		mux.Handle("/series/", s.serveXtreamSeriesProxy())
	}
	mux.Handle("/entitlements.json", s.serveXtreamEntitlements())
	mux.Handle("/guide.xml", xmltv)
	mux.Handle("/guide/health.json", s.serveGuideHealth())
	mux.Handle("/guide/policy.json", s.serveGuidePolicy())
	mux.Handle("/guide/doctor.json", s.serveEPGDoctor())
	mux.Handle("/guide/aliases.json", s.serveSuggestedAliasOverrides())
	mux.Handle("/guide/lineup-match.json", s.serveGuideLineupMatch())
	mux.Handle("/programming/categories.json", s.serveProgrammingCategories())
	mux.Handle("/programming/browse.json", s.serveProgrammingBrowse())
	mux.Handle("/programming/channels.json", s.serveProgrammingChannels())
	mux.Handle("/programming/channel-detail.json", s.serveProgrammingChannelDetail())
	mux.Handle("/programming/order.json", s.serveProgrammingOrder())
	mux.Handle("/programming/backups.json", s.serveProgrammingBackups())
	mux.Handle("/programming/harvest.json", s.serveProgrammingHarvest())
	mux.Handle("/programming/harvest-import.json", s.serveProgrammingHarvestImport())
	mux.Handle("/programming/harvest-assist.json", s.serveProgrammingHarvestAssist())
	mux.Handle("/programming/recipe.json", s.serveProgrammingRecipe())
	mux.Handle("/programming/preview.json", s.serveProgrammingPreview())
	mux.Handle("/virtual-channels/rules.json", s.serveVirtualChannelRules())
	mux.Handle("/virtual-channels/preview.json", s.serveVirtualChannelPreview())
	mux.Handle("/virtual-channels/schedule.json", s.serveVirtualChannelSchedule())
	mux.Handle("/virtual-channels/channel-detail.json", s.serveVirtualChannelDetail())
	mux.Handle("/virtual-channels/guide.xml", s.serveVirtualChannelGuide())
	mux.Handle("/virtual-channels/live.m3u", s.serveVirtualChannelM3U())
	mux.Handle("/virtual-channels/stream/", s.serveVirtualChannelStream())
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
	mux.Handle("/recordings/rules.json", s.serveRecordingRules())
	mux.Handle("/recordings/rules/preview.json", s.serveRecordingRulePreview())
	mux.Handle("/recordings/history.json", s.serveRecordingHistory())
	mux.Handle("/debug/active-streams.json", s.serveActiveStreamsReport())
	mux.Handle("/debug/shared-relays.json", s.serveSharedRelayReport())
	mux.Handle("/debug/stream-attempts.json", s.serveRecentStreamAttempts())
	mux.Handle("/debug/event-hooks.json", s.serveEventHooksReport())
	mux.Handle("/debug/runtime.json", s.serveRuntimeSnapshot())
	mux.Handle("/debug/hls-mux-demo.html", s.serveHlsMuxWebDemo())
	if metricsEnableFromEnv() {
		promRegisterAutopilotMetrics(gateway)
		promRegisterUpstreamMetrics()
		mux.Handle("/metrics", promhttp.Handler())
	}
	mux.Handle("/ops/actions/mux-seg-decode", s.serveMuxSegDecodeAction())
	mux.Handle("/ops/actions/status.json", s.serveOperatorActionStatus())
	mux.Handle("/ops/workflows/guide-repair.json", s.serveGuideRepairWorkflow())
	mux.Handle("/ops/workflows/stream-investigate.json", s.serveStreamInvestigateWorkflow())
	mux.Handle("/ops/workflows/diagnostics.json", s.serveDiagnosticsWorkflow())
	mux.Handle("/ops/workflows/ops-recovery.json", s.serveOpsRecoveryWorkflow())
	mux.Handle("/ops/actions/guide-refresh", s.serveGuideRefreshAction())
	mux.Handle("/ops/actions/stream-attempts-clear", s.serveStreamAttemptsClearAction())
	mux.Handle("/ops/actions/stream-stop", s.serveStreamStopAction())
	mux.Handle("/ops/actions/provider-profile-reset", s.serveProviderProfileResetAction())
	mux.Handle("/ops/actions/autopilot-reset", s.serveAutopilotResetAction())
	mux.Handle("/ops/actions/ghost-visible-stop", s.serveGhostVisibleStopAction())
	mux.Handle("/ops/actions/ghost-hidden-recover", s.serveGhostHiddenRecoverAction())
	mux.Handle("/ops/actions/evidence-intake-start", s.serveEvidenceIntakeStartAction())
	mux.Handle("/ops/actions/channel-diff-run", s.serveChannelDiffRunAction())
	mux.Handle("/ops/actions/stream-compare-run", s.serveStreamCompareRunAction())

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
	w.ResponseWriter.Header().Set("X-Content-Type-Options", "nosniff")
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
		rep, err := runGhostHunterAction(r.Context(), cfg, stop, nil)
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

func (s *Server) serveSharedRelayReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var rep SharedRelayReport
		if s.gateway != nil {
			rep = s.gateway.SharedRelayReport()
		} else {
			rep = SharedRelayReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode shared relay report"}`, http.StatusInternalServerError)
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
			"active_streams": map[string]interface{}{
				"available": s.gateway != nil,
				"endpoint":  "/debug/active-streams.json",
			},
			"stream_stop": map[string]interface{}{
				"available":    s.gateway != nil,
				"endpoint":     "/ops/actions/stream-stop",
				"method":       "POST",
				"body":         `{"request_id":"r000001"}` + " or " + `{"channel_id":"espn.us"}`,
				"localhost_ui": true,
			},
			"provider_profile_reset": map[string]interface{}{
				"available": s.gateway != nil,
			},
			"autopilot_reset": map[string]interface{}{
				"available": s.gateway != nil && s.gateway.Autopilot != nil,
			},
			"ghost_visible_stop": map[string]interface{}{
				"available": NewGhostHunterConfigFromEnv().GhostHunterReady(),
				"observe":   NewGhostHunterConfigFromEnv().ObserveWindow.String(),
			},
			"ghost_hidden_recover": map[string]interface{}{
				"available":    NewGhostHunterConfigFromEnv().GhostHunterReady(),
				"helper_path":  ghostHunterRecoveryHelperPath(),
				"modes":        []string{"dry-run", "restart"},
				"localhost_ui": true,
			},
			"mux_seg_decode": map[string]interface{}{
				"available":    true,
				"endpoint":     "/ops/actions/mux-seg-decode",
				"method":       "POST",
				"body":         `{"seg_b64":"<base64 of raw seg URL>"}`,
				"localhost_ui": true,
			},
			"evidence_intake_start": map[string]interface{}{
				"available":    true,
				"endpoint":     "/ops/actions/evidence-intake-start",
				"method":       "POST",
				"body":         `{"case_id":"plex-server-vs-laptop"}`,
				"localhost_ui": true,
			},
			"channel_diff_run": map[string]interface{}{
				"available":    true,
				"endpoint":     "/ops/actions/channel-diff-run",
				"method":       "POST",
				"body":         `{"good_channel_id":"325860","bad_channel_id":"325778"}`,
				"localhost_ui": true,
			},
			"stream_compare_run": map[string]interface{}{
				"available":    true,
				"endpoint":     "/ops/actions/stream-compare-run",
				"method":       "POST",
				"body":         `{"channel_id":"325778"}`,
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

func (s *Server) serveDiagnosticsWorkflow() http.Handler {
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
		if s.gateway != nil {
			attempts = s.gateway.RecentStreamAttempts(12)
		}
		good, bad := suggestDiagnosticChannels(attempts)
		report := OperatorWorkflowReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Name:        "diagnostics_capture",
			Summary: map[string]interface{}{
				"recent_attempt_count":      attempts.Count,
				"suggested_good_channel_id": good,
				"suggested_bad_channel_id":  bad,
				"diag_runs":                 latestDiagRuns("channel-diff", "stream-compare", "multi-stream", "evidence"),
			},
			Steps: []string{
				"Choose one known-good and one known-bad channel from recent attempts or the Programming lane preview.",
				"Run a paired channel diff / stream compare capture so the failure becomes a channel-class comparison instead of one anecdote.",
				"Create an evidence bundle and attach PMS logs, Tunerr logs, and pcap for the same time window.",
				"Analyze the bundle with analyze-bundle.py or compare harness outputs before changing provider or playback policy.",
			},
			Actions: []string{
				"/programming/channel-detail.json",
				"/programming/harvest-assist.json",
				"/debug/stream-attempts.json",
				"/ops/actions/channel-diff-run",
				"/ops/actions/stream-compare-run",
				"/ops/actions/evidence-intake-start",
			},
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode diagnostics workflow"}`, http.StatusInternalServerError)
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
		if rep, err := runGhostHunterAction(r.Context(), NewGhostHunterConfigFromEnv(), false, nil); err == nil {
			ghostSummary["session_count"] = rep.SessionCount
			ghostSummary["stale_count"] = rep.StaleCount
			ghostSummary["hidden_grab_suspected"] = rep.HiddenGrabSuspected
			ghostSummary["recommended_action"] = rep.RecommendedAction
			ghostSummary["safe_actions"] = rep.SafeActions
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
				"Stop only visible stale sessions first; use hidden-grab recovery dry-run before any restart action.",
				"Review Autopilot memory when the gateway keeps preferring a stale profile or host path.",
				"Reset Autopilot memory only after you have captured the current evidence and want a clean learning pass.",
			},
			Actions: []string{
				"/ops/actions/ghost-visible-stop",
				"/ops/actions/ghost-hidden-recover?mode=dry-run",
				"/ops/actions/ghost-hidden-recover?mode=restart",
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

func (s *Server) serveStreamStopAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.gateway == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "stream_stop", Message: "gateway unavailable"})
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		var req struct {
			RequestID string `json:"request_id"`
			ChannelID string `json:"channel_id"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "stream_stop", Message: "invalid json"})
			return
		}
		cancelled := s.gateway.cancelActiveStreams(req.RequestID, req.ChannelID)
		if len(cancelled) == 0 {
			writeOperatorActionJSON(w, http.StatusNotFound, OperatorActionResponse{OK: false, Action: "stream_stop", Message: "no matching active streams"})
			return
		}
		if s.EventHooks != nil {
			s.EventHooks.Dispatch("stream.cancelled", "operator", map[string]interface{}{
				"request_id": req.RequestID,
				"channel_id": req.ChannelID,
				"count":      len(cancelled),
			})
		}
		writeOperatorActionJSON(w, http.StatusAccepted, OperatorActionResponse{
			OK:      true,
			Action:  "stream_stop",
			Message: "stream cancellation requested",
			Detail:  map[string]interface{}{"count": len(cancelled), "streams": cancelled},
		})
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

func (s *Server) serveGhostVisibleStopAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		cfg := NewGhostHunterConfigFromEnv()
		if !cfg.GhostHunterReady() {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "ghost_visible_stop", Message: "ghost hunter is not configured"})
			return
		}
		rep, err := runGhostHunterAction(r.Context(), cfg, true, nil)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadGateway, OperatorActionResponse{OK: false, Action: "ghost_visible_stop", Message: "ghost hunter stop failed", Detail: err.Error()})
			return
		}
		msg := "ghost hunter stop pass completed"
		if rep.StaleCount == 0 {
			msg = "ghost hunter found no visible stale sessions to stop"
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "ghost_visible_stop", Message: msg, Detail: rep})
	})
}

func (s *Server) serveGhostHiddenRecoverAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
		if mode == "" {
			mode = "dry-run"
		}
		result, err := runGhostHunterRecoveryAction(r.Context(), mode)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadGateway, OperatorActionResponse{
				OK:      false,
				Action:  "ghost_hidden_recover",
				Message: "ghost hidden-grab helper failed",
				Detail:  map[string]interface{}{"mode": mode, "result": result, "error": err.Error()},
			})
			return
		}
		message := "ghost hidden-grab helper completed"
		if mode == "dry-run" {
			message = "ghost hidden-grab helper dry-run completed"
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{
			OK:      true,
			Action:  "ghost_hidden_recover",
			Message: message,
			Detail:  result,
		})
	})
}

func (s *Server) serveEvidenceIntakeStartAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		defer limited.Close()
		var req struct {
			CaseID string `json:"case_id"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		caseID := strings.TrimSpace(req.CaseID)
		if caseID == "" {
			caseID = "evidence-" + time.Now().UTC().Format("20060102-150405")
		}
		outDir := filepath.Join(repoDiagRoot(), "evidence", caseID)
		if err := createEvidenceIntakeBundle(outDir); err != nil {
			http.Error(w, `{"error":"create evidence bundle failed"}`, http.StatusBadGateway)
			return
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{
			OK:      true,
			Action:  "evidence_intake_start",
			Message: "evidence intake bundle created",
			Detail: map[string]interface{}{
				"case_id":    caseID,
				"output_dir": outDir,
				"next": []string{
					fmt.Sprintf(`python3 scripts/analyze-bundle.py "%s" --output "%s/report.txt"`, outDir, outDir),
				},
			},
		})
	})
}

func (s *Server) serveChannelDiffRunAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		defer limited.Close()
		var req struct {
			GoodChannelID string `json:"good_channel_id"`
			BadChannelID  string `json:"bad_channel_id"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil && err != io.EOF {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "channel_diff_run", Message: "invalid json"})
			return
		}
		env, detail, err := s.buildChannelDiffHarnessEnv(req.GoodChannelID, req.BadChannelID)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "channel_diff_run", Message: err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		runDetail, err := runChannelDiffHarnessAction(ctx, env)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadGateway, OperatorActionResponse{OK: false, Action: "channel_diff_run", Message: "channel diff harness failed", Detail: map[string]interface{}{"request": detail, "error": err.Error()}})
			return
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "channel_diff_run", Message: "channel diff capture completed", Detail: mergeOperatorActionDetail(detail, runDetail)})
	})
}

func (s *Server) serveStreamCompareRunAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		defer limited.Close()
		var req struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil && err != io.EOF {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "stream_compare_run", Message: "invalid json"})
			return
		}
		env, detail, err := s.buildStreamCompareHarnessEnv(req.ChannelID)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "stream_compare_run", Message: err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		runDetail, err := runStreamCompareHarnessAction(ctx, env)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadGateway, OperatorActionResponse{OK: false, Action: "stream_compare_run", Message: "stream compare harness failed", Detail: map[string]interface{}{"request": detail, "error": err.Error()}})
			return
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "stream_compare_run", Message: "stream compare capture completed", Detail: mergeOperatorActionDetail(detail, runDetail)})
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
		if rep.Events == nil {
			rep.Events = map[string]interface{}{}
		}
		rep.Events["webhooks_file"] = strings.TrimSpace(s.EventHooksFile)
		rep.Events["enabled"] = s.EventHooks != nil && s.EventHooks.Enabled()
		if s.EventHooks != nil {
			report := s.EventHooks.Report()
			rep.Events["hook_count"] = report.TotalHooks
			rep.Events["recent_count"] = len(report.Recent)
		} else {
			rep.Events["hook_count"] = 0
			rep.Events["recent_count"] = 0
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode runtime snapshot"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveEventHooksReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		report := eventhooks.Report{
			Enabled:    false,
			ConfigFile: strings.TrimSpace(s.EventHooksFile),
			RecentMax:  64,
		}
		if s.EventHooks != nil {
			report = s.EventHooks.Report()
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode event hooks"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveActiveStreamsReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rep := ActiveStreamsReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if s.gateway != nil {
			rep = s.gateway.ActiveStreamsReport()
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode active streams"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveGuideLineupMatch() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.xmltv == nil {
			http.Error(w, `{"error":"guide unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		rep, err := s.xmltv.GuideLineupMatchReport(streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 25))
		if err != nil {
			http.Error(w, `{"error":"guide lineup match unavailable"}`, http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode guide lineup match"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

type programmingPreviewReport struct {
	GeneratedAt       string                                `json:"generated_at"`
	RecipeFile        string                                `json:"recipe_file,omitempty"`
	RecipeWritable    bool                                  `json:"recipe_writable"`
	HarvestFile       string                                `json:"harvest_file,omitempty"`
	HarvestReady      bool                                  `json:"harvest_ready"`
	HarvestLineups    []plexharvest.SummaryLineup           `json:"harvest_lineups,omitempty"`
	RawChannels       int                                   `json:"raw_channels"`
	CuratedChannels   int                                   `json:"curated_channels"`
	Recipe            programming.Recipe                    `json:"recipe"`
	Inventory         []programming.CategorySummary         `json:"inventory,omitempty"`
	Lineup            []catalog.LiveChannel                 `json:"lineup,omitempty"`
	LineupDescriptors map[string]programming.FeedDescriptor `json:"lineup_descriptors,omitempty"`
	Buckets           map[string]int                        `json:"buckets,omitempty"`
	BackupGroups      []programming.BackupGroup             `json:"backup_groups,omitempty"`
}

type programmingChannelDetailReport struct {
	GeneratedAt        string                          `json:"generated_at"`
	Channel            catalog.LiveChannel             `json:"channel"`
	Descriptor         programming.FeedDescriptor      `json:"descriptor,omitempty"`
	Curated            bool                            `json:"curated"`
	CategoryID         string                          `json:"category_id"`
	CategoryLabel      string                          `json:"category_label"`
	CategorySource     string                          `json:"category_source,omitempty"`
	Bucket             string                          `json:"bucket"`
	ExactBackupGroup   *programming.BackupGroup        `json:"exact_backup_group,omitempty"`
	AlternativeSources []programming.BackupGroupMember `json:"alternative_sources,omitempty"`
	UpcomingProgrammes []CatchupCapsule                `json:"upcoming_programmes,omitempty"`
	SourceReady        bool                            `json:"source_ready"`
}

type programmingBrowseItem struct {
	CategoryID             string                     `json:"category_id"`
	Bucket                 string                     `json:"bucket,omitempty"`
	ChannelID              string                     `json:"channel_id"`
	GuideNumber            string                     `json:"guide_number"`
	GuideName              string                     `json:"guide_name"`
	TVGID                  string                     `json:"tvg_id,omitempty"`
	SourceTag              string                     `json:"source_tag,omitempty"`
	GroupTitle             string                     `json:"group_title,omitempty"`
	Descriptor             programming.FeedDescriptor `json:"descriptor,omitempty"`
	Curated                bool                       `json:"curated"`
	Included               bool                       `json:"included"`
	Excluded               bool                       `json:"excluded"`
	ExactBackupCount       int                        `json:"exact_backup_count"`
	GuideStatus            string                     `json:"guide_status,omitempty"`
	HasGuideProgrammes     bool                       `json:"has_guide_programmes"`
	HasRealGuideProgrammes bool                       `json:"has_real_guide_programmes"`
	NextHourProgrammeCount int                        `json:"next_hour_programme_count"`
	NextHourTitles         []string                   `json:"next_hour_titles,omitempty"`
}

type programmingBrowseReport struct {
	GeneratedAt    string                  `json:"generated_at"`
	CategoryID     string                  `json:"category_id"`
	CategoryLabel  string                  `json:"category_label,omitempty"`
	CategorySource string                  `json:"category_source,omitempty"`
	SourceReady    bool                    `json:"source_ready"`
	Horizon        string                  `json:"horizon"`
	Recipe         programming.Recipe      `json:"recipe"`
	TotalChannels  int                     `json:"total_channels"`
	Items          []programmingBrowseItem `json:"items,omitempty"`
}

type programmingHarvestImportReport struct {
	GeneratedAt          string                `json:"generated_at"`
	HarvestFile          string                `json:"harvest_file,omitempty"`
	LineupTitle          string                `json:"lineup_title,omitempty"`
	FriendlyName         string                `json:"friendly_name,omitempty"`
	Replace              bool                  `json:"replace"`
	CollapseExactBackups bool                  `json:"collapse_exact_backups"`
	HarvestedChannels    int                   `json:"harvested_channels"`
	MatchedChannels      int                   `json:"matched_channels"`
	MatchStrategies      map[string]int        `json:"match_strategies,omitempty"`
	OrderedChannelIDs    []string              `json:"ordered_channel_ids,omitempty"`
	MissingGuideNames    []string              `json:"missing_guide_names,omitempty"`
	Recipe               programming.Recipe    `json:"recipe"`
	MatchedLineup        []catalog.LiveChannel `json:"matched_lineup,omitempty"`
}

type programmingHarvestAssist struct {
	LineupTitle          string         `json:"lineup_title"`
	FriendlyNames        []string       `json:"friendly_names,omitempty"`
	MatchedChannels      int            `json:"matched_channels"`
	OrderedChannelIDs    []string       `json:"ordered_channel_ids,omitempty"`
	MatchStrategies      map[string]int `json:"match_strategies,omitempty"`
	LocalBroadcastHits   int            `json:"local_broadcast_hits"`
	ExactGuideNameHits   int            `json:"exact_guide_name_hits"`
	ExactTVGIDHits       int            `json:"exact_tvg_id_hits"`
	GuideNumberHits      int            `json:"guide_number_hits"`
	Recommended          bool           `json:"recommended"`
	RecommendationReason string         `json:"recommendation_reason,omitempty"`
}

type programmingHarvestAssistReport struct {
	GeneratedAt string                     `json:"generated_at"`
	HarvestFile string                     `json:"harvest_file,omitempty"`
	Assists     []programmingHarvestAssist `json:"assists,omitempty"`
}

type virtualChannelDetailReport struct {
	GeneratedAt string                        `json:"generated_at"`
	Channel     virtualchannels.Channel       `json:"channel"`
	ResolvedNow *virtualchannels.ResolvedSlot `json:"resolved_now,omitempty"`
	Upcoming    []virtualchannels.PreviewSlot `json:"upcoming,omitempty"`
	Schedule    []virtualchannels.PreviewSlot `json:"schedule,omitempty"`
}

type diagRunRef struct {
	Family     string   `json:"family"`
	RunID      string   `json:"run_id"`
	Path       string   `json:"path"`
	Updated    string   `json:"updated"`
	ReportPath string   `json:"report_path,omitempty"`
	Verdict    string   `json:"verdict,omitempty"`
	Summary    []string `json:"summary,omitempty"`
}

func normalizeHarvestGuideName(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.Join(strings.Fields(raw), " ")
	return raw
}

func normalizeHarvestBroadcastStem(raw string) string {
	raw = normalizeHarvestGuideName(raw)
	replacer := strings.NewReplacer(
		" east", "",
		" west", "",
		" hd", "",
		" us", "",
		" usa", "",
		" canada", "",
	)
	raw = replacer.Replace(raw)
	fields := strings.Fields(raw)
	if len(fields) > 1 {
		last := fields[len(fields)-1]
		if len(last) >= 3 && len(last) <= 12 && !strings.ContainsAny(last, "0123456789") {
			fields = fields[:len(fields)-1]
		}
	}
	return strings.Join(fields, " ")
}

func chooseHarvestResult(rep plexharvest.Report, lineupTitle, friendlyName string) (plexharvest.Result, bool) {
	lineupTitle = strings.TrimSpace(lineupTitle)
	friendlyName = strings.TrimSpace(friendlyName)
	best := plexharvest.Result{}
	found := false
	for _, row := range rep.Results {
		if lineupTitle != "" && !strings.EqualFold(strings.TrimSpace(row.LineupTitle), lineupTitle) {
			continue
		}
		if friendlyName != "" && !strings.EqualFold(strings.TrimSpace(row.FriendlyName), friendlyName) {
			continue
		}
		if len(row.Channels) == 0 {
			continue
		}
		if !found || len(row.Channels) > len(best.Channels) || row.ChannelMapRows > best.ChannelMapRows {
			best = row
			found = true
		}
	}
	if found {
		return best, true
	}
	for _, row := range rep.Results {
		if len(row.Channels) == 0 {
			continue
		}
		if !found || len(row.Channels) > len(best.Channels) || row.ChannelMapRows > best.ChannelMapRows {
			best = row
			found = true
		}
	}
	return best, found
}

func harvestCandidateKeys(ch catalog.LiveChannel) []string {
	keys := make([]string, 0, 4)
	if tvg := strings.TrimSpace(ch.TVGID); tvg != "" {
		keys = append(keys, "tvg:"+tvg)
	}
	if name := normalizeHarvestGuideName(ch.GuideName); name != "" {
		keys = append(keys, "name:"+name)
	}
	if num := strings.TrimSpace(ch.GuideNumber); num != "" {
		keys = append(keys, "number:"+num)
	}
	if programming.ClassifyChannel(ch) == programming.BucketLocalBroadcast {
		if stem := normalizeHarvestBroadcastStem(ch.GuideName); stem != "" {
			keys = append(keys, "local_stem:"+stem)
		}
	}
	return keys
}

func harvestLookupKeys(harvested plexharvest.HarvestedChannel) []struct {
	key      string
	strategy string
} {
	keys := make([]struct {
		key      string
		strategy string
	}, 0, 4)
	if tvg := strings.TrimSpace(harvested.TVGID); tvg != "" {
		keys = append(keys, struct {
			key      string
			strategy string
		}{key: "tvg:" + tvg, strategy: "tvg_id_exact"})
	}
	if name := normalizeHarvestGuideName(harvested.GuideName); name != "" {
		keys = append(keys, struct {
			key      string
			strategy string
		}{key: "name:" + name, strategy: "guide_name_exact"})
	}
	if num := strings.TrimSpace(harvested.GuideNumber); num != "" {
		keys = append(keys, struct {
			key      string
			strategy string
		}{key: "number:" + num, strategy: "guide_number_exact"})
	}
	if stem := normalizeHarvestBroadcastStem(harvested.GuideName); stem != "" {
		keys = append(keys, struct {
			key      string
			strategy string
		}{key: "local_stem:" + stem, strategy: "local_broadcast_stem"})
	}
	return keys
}

func buildProgrammingHarvestImport(existing programming.Recipe, raw []catalog.LiveChannel, result plexharvest.Result, replace bool, collapse bool) programmingHarvestImportReport {
	report := programmingHarvestImportReport{
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339),
		LineupTitle:          strings.TrimSpace(result.LineupTitle),
		FriendlyName:         strings.TrimSpace(result.FriendlyName),
		Replace:              replace,
		CollapseExactBackups: collapse,
		HarvestedChannels:    len(result.Channels),
		MatchStrategies:      map[string]int{},
	}
	indexed := map[string][]catalog.LiveChannel{}
	for _, ch := range raw {
		for _, key := range harvestCandidateKeys(ch) {
			indexed[key] = append(indexed[key], ch)
		}
	}
	seen := map[string]struct{}{}
	ordered := make([]string, 0)
	matched := make([]catalog.LiveChannel, 0)
	missing := make([]string, 0)
	for _, harvested := range result.Channels {
		var (
			candidates []catalog.LiveChannel
			matchedVia string
		)
		for _, rule := range harvestLookupKeys(harvested) {
			rows := indexed[rule.key]
			if len(rows) == 0 {
				continue
			}
			candidates = append(candidates, rows...)
			matchedVia = rule.strategy
			break
		}
		if len(candidates) == 0 {
			if name := strings.TrimSpace(harvested.GuideName); name != "" {
				missing = append(missing, name)
			}
			continue
		}
		report.MatchStrategies[matchedVia]++
		sort.SliceStable(candidates, func(i, j int) bool {
			hi := strings.TrimSpace(harvested.GuideNumber)
			ai := strings.TrimSpace(candidates[i].GuideNumber)
			aj := strings.TrimSpace(candidates[j].GuideNumber)
			if ai == hi && aj != hi {
				return true
			}
			if aj == hi && ai != hi {
				return false
			}
			if ai == aj {
				return strings.TrimSpace(candidates[i].GuideName) < strings.TrimSpace(candidates[j].GuideName)
			}
			return ai < aj
		})
		for _, candidate := range candidates {
			channelID := strings.TrimSpace(candidate.ChannelID)
			if _, ok := seen[channelID]; ok {
				continue
			}
			seen[channelID] = struct{}{}
			ordered = append(ordered, channelID)
			matched = append(matched, candidate)
		}
	}
	report.OrderedChannelIDs = append([]string(nil), ordered...)
	report.MatchedChannels = len(ordered)
	report.MatchedLineup = append([]catalog.LiveChannel(nil), matched...)
	report.MissingGuideNames = append([]string(nil), missing...)

	var recipe programming.Recipe
	if replace {
		excluded := make([]string, 0, len(raw))
		for _, ch := range raw {
			channelID := strings.TrimSpace(ch.ChannelID)
			if _, ok := seen[channelID]; ok {
				continue
			}
			excluded = append(excluded, channelID)
		}
		recipe = programming.Recipe{
			IncludedChannelIDs:   append([]string(nil), ordered...),
			ExcludedChannelIDs:   excluded,
			OrderMode:            "custom",
			CustomOrder:          append([]string(nil), ordered...),
			CollapseExactBackups: collapse,
		}
	} else {
		recipe = existing
		recipe.CollapseExactBackups = recipe.CollapseExactBackups || collapse
		recipe.OrderMode = "custom"
		recipe.IncludedChannelIDs = append(append([]string(nil), recipe.IncludedChannelIDs...), ordered...)
		recipe.CustomOrder = append(append([]string(nil), ordered...), recipe.CustomOrder...)
		if len(recipe.ExcludedChannelIDs) > 0 {
			excluded := make([]string, 0, len(recipe.ExcludedChannelIDs))
			for _, id := range recipe.ExcludedChannelIDs {
				if _, ok := seen[strings.TrimSpace(id)]; ok {
					continue
				}
				excluded = append(excluded, id)
			}
			recipe.ExcludedChannelIDs = excluded
		}
	}
	report.Recipe = programming.NormalizeRecipe(recipe)
	return report
}

func buildProgrammingHarvestAssist(raw []catalog.LiveChannel, row plexharvest.SummaryLineup, result plexharvest.Result) programmingHarvestAssist {
	preview := buildProgrammingHarvestImport(programming.Recipe{}, raw, result, true, true)
	assist := programmingHarvestAssist{
		LineupTitle:        strings.TrimSpace(row.LineupTitle),
		FriendlyNames:      append([]string(nil), row.FriendlyNames...),
		MatchedChannels:    preview.MatchedChannels,
		OrderedChannelIDs:  append([]string(nil), preview.OrderedChannelIDs...),
		MatchStrategies:    map[string]int{},
		LocalBroadcastHits: preview.MatchStrategies["local_broadcast_stem"],
		ExactGuideNameHits: preview.MatchStrategies["guide_name_exact"],
		ExactTVGIDHits:     preview.MatchStrategies["tvg_id_exact"],
		GuideNumberHits:    preview.MatchStrategies["guide_number_exact"],
	}
	for key, value := range preview.MatchStrategies {
		assist.MatchStrategies[key] = value
	}
	if assist.LocalBroadcastHits > 0 {
		assist.Recommended = true
		assist.RecommendationReason = fmt.Sprintf("%d local-broadcast lineup row(s) mapped back onto current raw channels.", assist.LocalBroadcastHits)
	} else if assist.ExactTVGIDHits > 0 || assist.ExactGuideNameHits > 0 {
		assist.Recommended = true
		assist.RecommendationReason = "Strong exact guide matches were found for this harvested lineup."
	} else if assist.MatchedChannels > 0 {
		assist.RecommendationReason = "Some rows matched, but this looks weaker as a local-market seed."
	}
	return assist
}

func (s *Server) serveProgrammingCategories() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				http.Error(w, `{"error":"programming recipe file not configured"}`, http.StatusServiceUnavailable)
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var req struct {
				Action      string   `json:"action"`
				CategoryID  string   `json:"category_id"`
				CategoryIDs []string `json:"category_ids"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid programming category json"}`, http.StatusBadRequest)
				return
			}
			ids := append([]string(nil), req.CategoryIDs...)
			if strings.TrimSpace(req.CategoryID) != "" {
				ids = append(ids, strings.TrimSpace(req.CategoryID))
			}
			recipe := programming.UpdateRecipeCategories(s.reloadProgrammingRecipe(), req.Action, ids)
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, recipe)
			if err != nil {
				http.Error(w, `{"error":"save programming recipe failed"}`, http.StatusBadGateway)
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		inventory := programming.BuildCategoryInventory(s.RawChannels)
		resp := map[string]interface{}{
			"generated_at":  time.Now().UTC().Format(time.RFC3339),
			"source_ready":  len(s.RawChannels) > 0,
			"raw_channels":  len(s.RawChannels),
			"categories":    inventory,
			"recipe_file":   strings.TrimSpace(s.ProgrammingRecipeFile),
			"recipe_loaded": s.reloadProgrammingRecipe().Version > 0,
			"recipe":        s.reloadProgrammingRecipe(),
		}
		if categoryID := strings.TrimSpace(r.URL.Query().Get("category")); categoryID != "" {
			resp["members"] = programming.CategoryMembers(s.RawChannels, categoryID)
		}
		body, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode programming categories"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingBrowse() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		categoryID := strings.TrimSpace(r.URL.Query().Get("category"))
		if categoryID == "" {
			http.Error(w, `{"error":"category required"}`, http.StatusBadRequest)
			return
		}
		horizon := time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				horizon = d
			}
		}
		memberLimit := streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 400)
		members := programming.CategoryMembers(s.RawChannels, categoryID)
		if len(members) == 0 {
			body, err := json.MarshalIndent(programmingBrowseReport{
				GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
				CategoryID:    categoryID,
				Horizon:       horizon.String(),
				Recipe:        s.reloadProgrammingRecipe(),
				TotalChannels: 0,
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming browse"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
			return
		}
		if memberLimit > len(members) {
			memberLimit = len(members)
		}
		selected := map[string]struct{}{}
		excluded := map[string]struct{}{}
		recipe := s.reloadProgrammingRecipe()
		for _, id := range recipe.IncludedChannelIDs {
			selected[strings.TrimSpace(id)] = struct{}{}
		}
		for _, id := range recipe.ExcludedChannelIDs {
			excluded[strings.TrimSpace(id)] = struct{}{}
		}
		healthByID := map[string]guidehealth.ChannelHealth{}
		sourceReady := false
		if s.xmltv != nil {
			if rep, err := s.xmltv.GuideHealth(time.Now(), strings.TrimSpace(os.Getenv("IPTV_TUNERR_XMLTV_ALIASES"))); err == nil {
				sourceReady = rep.SourceReady
				for _, row := range rep.Channels {
					healthByID[strings.TrimSpace(row.ChannelID)] = row
				}
			}
		}
		titlesByGuideNumber := map[string][]string{}
		if s.xmltv != nil {
			if preview, err := s.xmltv.CatchupCapsulePreview(time.Now(), horizon, 4096); err == nil {
				sourceReady = sourceReady || preview.SourceReady
				for _, capsule := range preview.Capsules {
					guideNumber := strings.TrimSpace(capsule.GuideNumber)
					if guideNumber == "" {
						continue
					}
					title := strings.TrimSpace(capsule.Title)
					if title == "" {
						continue
					}
					dup := false
					for _, existing := range titlesByGuideNumber[guideNumber] {
						if strings.TrimSpace(existing) == title {
							dup = true
							break
						}
					}
					if !dup {
						titlesByGuideNumber[guideNumber] = append(titlesByGuideNumber[guideNumber], title)
					}
				}
			}
		}
		backupCounts := map[string]int{}
		for _, group := range programming.BuildBackupGroups(s.RawChannels) {
			count := len(group.Members) - 1
			for _, member := range group.Members {
				backupCounts[strings.TrimSpace(member.ChannelID)] = count
			}
		}
		report := programmingBrowseReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			CategoryID:  categoryID,
			SourceReady: sourceReady,
			Horizon:     horizon.String(),
			Recipe:      recipe,
		}
		report.Items = make([]programmingBrowseItem, 0, memberLimit)
		for _, member := range members[:memberLimit] {
			channelID := strings.TrimSpace(member.ChannelID)
			item := programmingBrowseItem{
				CategoryID:       member.CategoryID,
				Bucket:           member.Bucket,
				ChannelID:        member.ChannelID,
				GuideNumber:      member.GuideNumber,
				GuideName:        member.GuideName,
				TVGID:            member.TVGID,
				SourceTag:        member.SourceTag,
				GroupTitle:       member.GroupTitle,
				Descriptor:       member.Descriptor,
				Curated:          containsLiveChannelID(s.Channels, channelID),
				ExactBackupCount: backupCounts[channelID],
			}
			if _, ok := selected[channelID]; ok {
				item.Included = true
			}
			if _, ok := excluded[channelID]; ok {
				item.Excluded = true
			}
			if health, ok := healthByID[channelID]; ok {
				item.GuideStatus = health.Status
				item.HasGuideProgrammes = health.HasProgrammes
				item.HasRealGuideProgrammes = health.HasRealProgrammes
			}
			item.NextHourTitles = append([]string(nil), titlesByGuideNumber[strings.TrimSpace(member.GuideNumber)]...)
			item.NextHourProgrammeCount = len(item.NextHourTitles)
			report.Items = append(report.Items, item)
		}
		report.TotalChannels = len(members)
		if len(report.Items) > 0 {
			report.CategoryLabel = report.Items[0].GroupTitle
			report.CategorySource = report.Items[0].SourceTag
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode programming browse"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingChannels() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			recipe := s.reloadProgrammingRecipe()
			resp := map[string]interface{}{
				"generated_at":      time.Now().UTC().Format(time.RFC3339),
				"recipe_file":       strings.TrimSpace(s.ProgrammingRecipeFile),
				"included_channels": recipe.IncludedChannelIDs,
				"excluded_channels": recipe.ExcludedChannelIDs,
			}
			if categoryID := strings.TrimSpace(r.URL.Query().Get("category")); categoryID != "" {
				resp["members"] = programming.CategoryMembers(s.RawChannels, categoryID)
			}
			body, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming channels"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				http.Error(w, `{"error":"programming recipe file not configured"}`, http.StatusServiceUnavailable)
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var req struct {
				Action     string   `json:"action"`
				ChannelID  string   `json:"channel_id"`
				ChannelIDs []string `json:"channel_ids"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid programming channel json"}`, http.StatusBadRequest)
				return
			}
			ids := append([]string(nil), req.ChannelIDs...)
			if strings.TrimSpace(req.ChannelID) != "" {
				ids = append(ids, strings.TrimSpace(req.ChannelID))
			}
			recipe := programming.UpdateRecipeChannels(s.reloadProgrammingRecipe(), req.Action, ids)
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, recipe)
			if err != nil {
				http.Error(w, `{"error":"save programming recipe failed"}`, http.StatusBadGateway)
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":               true,
				"recipe":           saved,
				"curated_channels": len(s.Channels),
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming channels"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) serveProgrammingOrder() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			recipe := s.reloadProgrammingRecipe()
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at":     time.Now().UTC().Format(time.RFC3339),
				"recipe_file":      strings.TrimSpace(s.ProgrammingRecipeFile),
				"order_mode":       recipe.OrderMode,
				"custom_order":     recipe.CustomOrder,
				"curated_channels": len(s.Channels),
				"collapse_backups": recipe.CollapseExactBackups,
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming order"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				http.Error(w, `{"error":"programming recipe file not configured"}`, http.StatusServiceUnavailable)
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var req struct {
				Action          string   `json:"action"`
				ChannelID       string   `json:"channel_id"`
				ChannelIDs      []string `json:"channel_ids"`
				BeforeChannelID string   `json:"before_channel_id"`
				AfterChannelID  string   `json:"after_channel_id"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid programming order json"}`, http.StatusBadRequest)
				return
			}
			ids := append([]string(nil), req.ChannelIDs...)
			if strings.TrimSpace(req.ChannelID) != "" {
				ids = append(ids, strings.TrimSpace(req.ChannelID))
			}
			recipe := programming.UpdateRecipeOrder(s.reloadProgrammingRecipe(), req.Action, ids, req.BeforeChannelID, req.AfterChannelID)
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, recipe)
			if err != nil {
				http.Error(w, `{"error":"save programming recipe failed"}`, http.StatusBadGateway)
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":               true,
				"recipe":           saved,
				"curated_channels": len(s.Channels),
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming order"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) serveProgrammingBackups() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		recipe := s.reloadProgrammingRecipe()
		preview := programming.ApplyRecipePreview(cloneLiveChannels(s.RawChannels), recipe)
		groups := programming.BuildBackupGroups(preview)
		body, err := json.MarshalIndent(map[string]interface{}{
			"generated_at":     time.Now().UTC().Format(time.RFC3339),
			"recipe_file":      strings.TrimSpace(s.ProgrammingRecipeFile),
			"collapse_enabled": recipe.CollapseExactBackups,
			"raw_channels":     len(s.RawChannels),
			"curated_preview":  len(preview),
			"group_count":      len(groups),
			"groups":           groups,
		}, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode programming backups"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingHarvest() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			rep := s.reloadPlexLineupHarvest()
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at":     time.Now().UTC().Format(time.RFC3339),
				"harvest_file":     strings.TrimSpace(s.PlexLineupHarvestFile),
				"harvest_writable": strings.TrimSpace(s.PlexLineupHarvestFile) != "",
				"report":           rep,
				"lineups":          rep.Lineups,
				"report_ready":     len(rep.Results) > 0 || len(rep.Lineups) > 0,
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming harvest"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.PlexLineupHarvestFile) == "" {
				http.Error(w, `{"error":"plex lineup harvest file not configured"}`, http.StatusServiceUnavailable)
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 1<<20)
			defer limited.Close()
			var rep plexharvest.Report
			if err := json.NewDecoder(limited).Decode(&rep); err != nil {
				http.Error(w, `{"error":"invalid programming harvest json"}`, http.StatusBadRequest)
				return
			}
			saved, err := s.savePlexLineupHarvest(rep)
			if err != nil {
				http.Error(w, `{"error":"save programming harvest failed"}`, http.StatusBadGateway)
				return
			}
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":           true,
				"harvest_file": strings.TrimSpace(s.PlexLineupHarvestFile),
				"report":       saved,
				"lineups":      saved.Lineups,
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming harvest"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) serveProgrammingHarvestImport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rep := s.reloadPlexLineupHarvest()
		switch r.Method {
		case http.MethodGet:
			lineupTitle := strings.TrimSpace(r.URL.Query().Get("lineup_title"))
			friendlyName := strings.TrimSpace(r.URL.Query().Get("friendly_name"))
			replace := true
			if raw := strings.TrimSpace(r.URL.Query().Get("replace")); raw != "" {
				replace = raw != "0" && !strings.EqualFold(raw, "false")
			}
			collapse := false
			if raw := strings.TrimSpace(r.URL.Query().Get("collapse_exact_backups")); raw != "" {
				collapse = raw == "1" || strings.EqualFold(raw, "true")
			}
			result, ok := chooseHarvestResult(rep, lineupTitle, friendlyName)
			if !ok {
				http.Error(w, `{"error":"harvest result not found"}`, http.StatusNotFound)
				return
			}
			report := buildProgrammingHarvestImport(s.reloadProgrammingRecipe(), s.RawChannels, result, replace, collapse)
			report.HarvestFile = strings.TrimSpace(s.PlexLineupHarvestFile)
			body, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming harvest import"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				http.Error(w, `{"error":"programming recipe file not configured"}`, http.StatusServiceUnavailable)
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var req struct {
				LineupTitle          string `json:"lineup_title"`
				FriendlyName         string `json:"friendly_name"`
				Replace              *bool  `json:"replace,omitempty"`
				CollapseExactBackups bool   `json:"collapse_exact_backups"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid programming harvest import json"}`, http.StatusBadRequest)
				return
			}
			replace := true
			if req.Replace != nil {
				replace = *req.Replace
			}
			result, ok := chooseHarvestResult(rep, req.LineupTitle, req.FriendlyName)
			if !ok {
				http.Error(w, `{"error":"harvest result not found"}`, http.StatusNotFound)
				return
			}
			report := buildProgrammingHarvestImport(s.reloadProgrammingRecipe(), s.RawChannels, result, replace, req.CollapseExactBackups)
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, report.Recipe)
			if err != nil {
				http.Error(w, `{"error":"save programming recipe failed"}`, http.StatusBadGateway)
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
			report.Recipe = saved
			report.HarvestFile = strings.TrimSpace(s.PlexLineupHarvestFile)
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":               true,
				"report":           report,
				"curated_channels": len(s.Channels),
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming harvest import"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) serveProgrammingHarvestAssist() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rep := s.reloadPlexLineupHarvest()
		report := programmingHarvestAssistReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			HarvestFile: strings.TrimSpace(s.PlexLineupHarvestFile),
		}
		for _, row := range rep.Lineups {
			result, ok := chooseHarvestResult(rep, row.LineupTitle, "")
			if !ok {
				continue
			}
			report.Assists = append(report.Assists, buildProgrammingHarvestAssist(s.RawChannels, row, result))
		}
		sort.SliceStable(report.Assists, func(i, j int) bool {
			ai := report.Assists[i]
			aj := report.Assists[j]
			if ai.Recommended != aj.Recommended {
				return ai.Recommended
			}
			if ai.LocalBroadcastHits != aj.LocalBroadcastHits {
				return ai.LocalBroadcastHits > aj.LocalBroadcastHits
			}
			if ai.MatchedChannels != aj.MatchedChannels {
				return ai.MatchedChannels > aj.MatchedChannels
			}
			return ai.LineupTitle < aj.LineupTitle
		})
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode programming harvest assist"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveXtreamEntitlements() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			set := s.reloadXtreamEntitlements()
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at": time.Now().UTC().Format(time.RFC3339),
				"users_file":   strings.TrimSpace(s.XtreamUsersFile),
				"enabled":      strings.TrimSpace(s.XtreamUsersFile) != "",
				"rules":        set,
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode xtream entitlements"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.XtreamUsersFile) == "" {
				http.Error(w, `{"error":"xtream users file not configured"}`, http.StatusServiceUnavailable)
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var set entitlements.Ruleset
			if err := json.NewDecoder(limited).Decode(&set); err != nil {
				http.Error(w, `{"error":"invalid xtream entitlements json"}`, http.StatusBadRequest)
				return
			}
			saved, err := s.saveXtreamEntitlements(set)
			if err != nil {
				http.Error(w, `{"error":"save xtream entitlements failed"}`, http.StatusBadGateway)
				return
			}
			body, err := json.MarshalIndent(saved, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode xtream entitlements"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) serveProgrammingChannelDetail() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
		if channelID == "" {
			http.Error(w, `{"error":"channel_id required"}`, http.StatusBadRequest)
			return
		}
		sourceChannels := s.RawChannels
		if len(sourceChannels) == 0 {
			sourceChannels = s.Channels
		}
		var target catalog.LiveChannel
		found := false
		for _, ch := range sourceChannels {
			if strings.TrimSpace(ch.ChannelID) == channelID {
				target = ch
				found = true
				break
			}
		}
		if !found {
			http.Error(w, `{"error":"channel not found"}`, http.StatusNotFound)
			return
		}
		categoryID, categoryLabel, categorySource := programming.CategoryIdentity(target)
		report := programmingChannelDetailReport{
			GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
			Channel:        target,
			Descriptor:     programming.DescribeChannel(target),
			Curated:        containsLiveChannelID(s.Channels, channelID),
			CategoryID:     categoryID,
			CategoryLabel:  categoryLabel,
			CategorySource: categorySource,
			Bucket:         string(programming.ClassifyChannel(target)),
		}
		for _, group := range programming.BuildBackupGroups(sourceChannels) {
			member := false
			for _, row := range group.Members {
				if strings.TrimSpace(row.ChannelID) == channelID {
					member = true
					continue
				}
				report.AlternativeSources = append(report.AlternativeSources, row)
			}
			if member {
				groupCopy := group
				report.ExactBackupGroup = &groupCopy
				break
			}
		}
		horizon := 3 * time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				horizon = d
			}
		}
		limit := 6
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 50 {
				limit = n
			}
		}
		if s.xmltv != nil {
			if preview, err := s.xmltv.CatchupCapsulePreview(time.Now(), horizon, 256); err == nil {
				report.SourceReady = preview.SourceReady
				for _, capsule := range preview.Capsules {
					if strings.TrimSpace(capsule.GuideNumber) != strings.TrimSpace(target.GuideNumber) {
						continue
					}
					report.UpcomingProgrammes = append(report.UpcomingProgrammes, capsule)
					if len(report.UpcomingProgrammes) >= limit {
						break
					}
				}
			}
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode programming channel detail"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingRecipe() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			recipe := s.reloadProgrammingRecipe()
			resp := map[string]interface{}{
				"recipe":          recipe,
				"recipe_file":     strings.TrimSpace(s.ProgrammingRecipeFile),
				"recipe_writable": strings.TrimSpace(s.ProgrammingRecipeFile) != "",
			}
			body, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming recipe"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				http.Error(w, `{"error":"programming recipe file not configured"}`, http.StatusServiceUnavailable)
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var recipe programming.Recipe
			if err := json.NewDecoder(limited).Decode(&recipe); err != nil {
				http.Error(w, `{"error":"invalid programming recipe json"}`, http.StatusBadRequest)
				return
			}
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, recipe)
			if err != nil {
				http.Error(w, `{"error":"save programming recipe failed"}`, http.StatusBadGateway)
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":               true,
				"recipe":           saved,
				"recipe_file":      strings.TrimSpace(s.ProgrammingRecipeFile),
				"curated_channels": len(s.Channels),
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode programming recipe"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func containsLiveChannelID(channels []catalog.LiveChannel, channelID string) bool {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return false
	}
	for _, ch := range channels {
		if strings.TrimSpace(ch.ChannelID) == channelID {
			return true
		}
	}
	return false
}

func (s *Server) serveProgrammingPreview() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		limit := streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 25)
		report := programmingPreviewReport{
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			RecipeFile:      strings.TrimSpace(s.ProgrammingRecipeFile),
			RecipeWritable:  strings.TrimSpace(s.ProgrammingRecipeFile) != "",
			HarvestFile:     strings.TrimSpace(s.PlexLineupHarvestFile),
			RawChannels:     len(s.RawChannels),
			CuratedChannels: len(s.Channels),
			Recipe:          s.reloadProgrammingRecipe(),
			Inventory:       programming.BuildCategoryInventory(s.RawChannels),
		}
		harvest := s.reloadPlexLineupHarvest()
		report.HarvestReady = len(harvest.Results) > 0 || len(harvest.Lineups) > 0
		report.HarvestLineups = append([]plexharvest.SummaryLineup(nil), harvest.Lineups...)
		previewChannels := programming.ApplyRecipePreview(cloneLiveChannels(s.RawChannels), report.Recipe)
		if limit > len(s.Channels) {
			limit = len(s.Channels)
		}
		report.Lineup = append([]catalog.LiveChannel(nil), s.Channels[:limit]...)
		report.LineupDescriptors = programming.DescribeChannels(report.Lineup)
		report.Buckets = make(map[string]int)
		for _, ch := range s.Channels {
			report.Buckets[string(programming.ClassifyChannel(ch))]++
		}
		report.BackupGroups = programming.BuildBackupGroups(previewChannels)
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode programming preview"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveVirtualChannelRules() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			set := s.reloadVirtualChannels()
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at":     time.Now().UTC().Format(time.RFC3339),
				"rules_file":       strings.TrimSpace(s.VirtualChannelsFile),
				"rules_writable":   strings.TrimSpace(s.VirtualChannelsFile) != "",
				"rules":            set,
				"enabled_channels": len(set.Channels),
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode virtual channel rules"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.VirtualChannelsFile) == "" {
				http.Error(w, `{"error":"virtual channels file not configured"}`, http.StatusServiceUnavailable)
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 1<<20)
			defer limited.Close()
			var set virtualchannels.Ruleset
			if err := json.NewDecoder(limited).Decode(&set); err != nil {
				http.Error(w, `{"error":"invalid virtual channels json"}`, http.StatusBadRequest)
				return
			}
			saved, err := s.saveVirtualChannels(set)
			if err != nil {
				http.Error(w, `{"error":"save virtual channels failed"}`, http.StatusBadGateway)
				return
			}
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":         true,
				"rules_file": strings.TrimSpace(s.VirtualChannelsFile),
				"rules":      saved,
			}, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode virtual channel rules"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) serveVirtualChannelPreview() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		perChannel := 4
		if raw := strings.TrimSpace(r.URL.Query().Get("per_channel")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 24 {
				perChannel = n
			}
		}
		report := virtualchannels.BuildPreview(s.reloadVirtualChannels(), s.Movies, s.Series, time.Now(), perChannel)
		body, err := json.MarshalIndent(map[string]interface{}{
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"rules_file":   strings.TrimSpace(s.VirtualChannelsFile),
			"report":       report,
		}, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode virtual channel preview"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveVirtualChannelSchedule() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		horizon := 6 * time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				horizon = d
			}
		}
		report := virtualchannels.BuildSchedule(s.reloadVirtualChannels(), s.Movies, s.Series, timeNow(), horizon)
		body, err := json.MarshalIndent(map[string]interface{}{
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"rules_file":   strings.TrimSpace(s.VirtualChannelsFile),
			"report":       report,
		}, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode virtual channel schedule"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveVirtualChannelDetail() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
		if channelID == "" {
			http.Error(w, `{"error":"channel_id required"}`, http.StatusBadRequest)
			return
		}
		set := s.reloadVirtualChannels()
		var target *virtualchannels.Channel
		for i := range set.Channels {
			if strings.TrimSpace(set.Channels[i].ID) == channelID {
				ch := set.Channels[i]
				target = &ch
				break
			}
		}
		if target == nil {
			http.NotFound(w, r)
			return
		}
		report := virtualChannelDetailReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Channel:     *target,
		}
		if slot, ok := virtualchannels.ResolveCurrentSlot(set, channelID, s.Movies, s.Series, timeNow()); ok {
			report.ResolvedNow = &slot
		}
		perChannel := 4
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 24 {
				perChannel = n
			}
		}
		for _, slot := range virtualchannels.BuildPreview(set, s.Movies, s.Series, timeNow(), perChannel).Slots {
			if strings.TrimSpace(slot.ChannelID) == channelID {
				report.Upcoming = append(report.Upcoming, slot)
			}
		}
		horizon := 6 * time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				horizon = d
			}
		}
		for _, slot := range virtualchannels.BuildSchedule(set, s.Movies, s.Series, timeNow(), horizon).Slots {
			if strings.TrimSpace(slot.ChannelID) == channelID {
				report.Schedule = append(report.Schedule, slot)
			}
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode virtual channel detail"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveVirtualChannelGuide() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		horizon := 6 * time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				horizon = d
			}
		}
		report := virtualchannels.BuildSchedule(s.reloadVirtualChannels(), s.Movies, s.Series, timeNow(), horizon)
		tv := &xmlTVRoot{
			XMLName: xml.Name{Local: "tv"},
			Source:  "IPTV Tunerr (virtual channels)",
		}
		seen := map[string]struct{}{}
		for _, slot := range report.Slots {
			channelID := strings.TrimSpace(slot.ChannelID)
			if _, ok := seen[channelID]; !ok {
				seen[channelID] = struct{}{}
				tv.Channels = append(tv.Channels, xmlChannel{
					ID:      "virtual." + channelID,
					Display: slot.ChannelName,
				})
			}
			tv.Programmes = append(tv.Programmes, xmlProgramme{
				Start:      timeMustParseRFC3339(slot.StartsAtUTC).Format("20060102150405 -0700"),
				Stop:       timeMustParseRFC3339(slot.EndsAtUTC).Format("20060102150405 -0700"),
				Channel:    "virtual." + channelID,
				Title:      xmlValue{Value: slot.ResolvedName},
				SubTitle:   xmlValue{Value: slot.EntryType},
				Desc:       xmlValue{Value: fmt.Sprintf("Synthetic virtual channel slot sourced from %s.", firstNonEmptyString(slot.EntryID, slot.EntryType))},
				Categories: []xmlValue{{Value: "Virtual Channels"}},
			})
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = w.Write([]byte(xml.Header))
		enc := xml.NewEncoder(w)
		enc.Indent("", "  ")
		_ = enc.Encode(tv)
	})
}

func (s *Server) virtualChannelLiveRows() []catalog.LiveChannel {
	rules := s.reloadVirtualChannels()
	if len(rules.Channels) == 0 {
		return nil
	}
	base := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	rows := make([]catalog.LiveChannel, 0, len(rules.Channels))
	for _, ch := range rules.Channels {
		if !ch.Enabled {
			continue
		}
		channelID := "virtual-" + strings.TrimSpace(ch.ID)
		streamURL := base + "/virtual-channels/stream/" + strings.TrimSpace(ch.ID) + ".mp4"
		rows = append(rows, catalog.LiveChannel{
			ChannelID:   channelID,
			DNAID:       channelID,
			GuideNumber: strings.TrimSpace(ch.GuideNumber),
			GuideName:   strings.TrimSpace(ch.Name),
			StreamURL:   streamURL,
			StreamURLs:  []string{streamURL},
			EPGLinked:   false,
			TVGID:       "virtual." + strings.TrimSpace(ch.ID),
			GroupTitle:  firstNonEmptyString(strings.TrimSpace(ch.GroupTitle), "Virtual Channels"),
			SourceTag:   "virtual",
		})
	}
	return rows
}

func (s *Server) serveVirtualChannelM3U() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-mpegURL")
		_, _ = io.WriteString(w, "#EXTM3U\n")
		for _, ch := range s.virtualChannelLiveRows() {
			_, _ = io.WriteString(w, fmt.Sprintf("#EXTINF:-1 tvg-id=\"%s\" tvg-name=\"%s\" group-title=\"%s\",%s\n%s\n",
				strings.TrimSpace(ch.TVGID),
				strings.TrimSpace(ch.GuideName),
				strings.TrimSpace(ch.GroupTitle),
				strings.TrimSpace(ch.GuideName),
				strings.TrimSpace(ch.StreamURL),
			))
		}
	})
}

func (s *Server) serveVirtualChannelStream() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/virtual-channels/stream/")
		if idx := strings.Index(id, "."); idx > 0 {
			id = id[:idx]
		}
		id = strings.TrimSpace(id)
		slot, ok := virtualchannels.ResolveCurrentSlot(s.reloadVirtualChannels(), id, s.Movies, s.Series, timeNow())
		if !ok {
			http.NotFound(w, r)
			return
		}
		sourceURL := strings.TrimSpace(slot.SourceURL)
		if sourceURL == "" {
			http.Error(w, `{"error":"virtual channel slot has no source"}`, http.StatusBadGateway)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, sourceURL, nil)
		if err != nil {
			http.Error(w, "proxy request failed", http.StatusBadGateway)
			return
		}
		if raw := strings.TrimSpace(r.Header.Get("Range")); raw != "" {
			req.Header.Set("Range", raw)
		}
		resp, err := httpclient.ForStreaming().Do(req)
		if err != nil {
			http.Error(w, "proxy request failed", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for _, name := range []string{"Content-Type", "Content-Length", "Accept-Ranges", "Content-Range", "Last-Modified", "ETag"} {
			if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
				w.Header().Set(name, value)
			}
		}
		w.Header().Set("X-IptvTunerr-Virtual-Channel", id)
		w.Header().Set("X-IptvTunerr-Virtual-Entry", strings.TrimSpace(slot.EntryID))
		w.WriteHeader(resp.StatusCode)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = io.Copy(w, resp.Body)
	})
}

func timeMustParseRFC3339(raw string) time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return parsed.UTC()
}

func repoDiagRoot() string {
	return filepath.Clean(".diag")
}

func latestDiagRuns(families ...string) []diagRunRef {
	root := repoDiagRoot()
	refs := make([]diagRunRef, 0, len(families))
	for _, family := range families {
		dir := filepath.Join(root, family)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var best diagRunRef
		var bestTime time.Time
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			mod := info.ModTime().UTC()
			if best.RunID == "" || mod.After(bestTime) {
				bestTime = mod
				best = diagRunRef{
					Family:  family,
					RunID:   entry.Name(),
					Path:    filepath.Join(dir, entry.Name()),
					Updated: mod.Format(time.RFC3339),
				}
				populateDiagRunSummary(&best)
			}
		}
		if best.RunID != "" {
			refs = append(refs, best)
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		return refs[i].Family < refs[j].Family
	})
	return refs
}

func populateDiagRunSummary(ref *diagRunRef) {
	if ref == nil || strings.TrimSpace(ref.Path) == "" {
		return
	}
	reportPath := filepath.Join(ref.Path, "report.json")
	body, err := os.ReadFile(reportPath)
	if err == nil {
		ref.ReportPath = reportPath
		var payload map[string]interface{}
		if json.Unmarshal(body, &payload) == nil {
			ref.Verdict, ref.Summary = summarizeDiagPayload(ref.Family, payload)
			if len(ref.Summary) > 4 {
				ref.Summary = ref.Summary[:4]
			}
			if ref.Verdict != "" || len(ref.Summary) > 0 {
				return
			}
		}
	}
	textPath := filepath.Join(ref.Path, "report.txt")
	body, err = os.ReadFile(textPath)
	if err != nil {
		return
	}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	ref.ReportPath = textPath
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if line == "" {
			continue
		}
		ref.Summary = append(ref.Summary, line)
		if len(ref.Summary) >= 3 {
			break
		}
	}
}

func summarizeDiagPayload(family string, payload map[string]interface{}) (string, []string) {
	switch strings.TrimSpace(family) {
	case "channel-diff":
		findings := stringSliceFromAny(payload["findings"], 3)
		if len(findings) == 0 {
			return "needs_review", nil
		}
		verdict := "channel_class_split"
		for _, item := range findings {
			lower := strings.ToLower(item)
			switch {
			case strings.Contains(lower, "tunerr-path issue"), strings.Contains(lower, "through tunerr"), strings.Contains(lower, "tunerr-only"):
				verdict = "tunerr_split"
			case strings.Contains(lower, "fails direct"), strings.Contains(lower, "upstream/provider/cdn"), strings.Contains(lower, "provider-specific"), strings.Contains(lower, "upstream-only"):
				verdict = "upstream_split"
				return verdict, findings
			}
		}
		return verdict, findings
	case "stream-compare":
		compare, _ := payload["compare"].(map[string]interface{})
		findings := stringSliceFromAny(compare["findings"], 3)
		if len(findings) == 0 {
			return "no_mismatch", nil
		}
		verdict := "mismatch_found"
		for _, item := range findings {
			if strings.Contains(strings.ToLower(item), "no top-level status mismatch") {
				verdict = "needs_lower_level_inspection"
				break
			}
		}
		return verdict, findings
	case "multi-stream":
		synopsis, _ := payload["synopsis"].(map[string]interface{})
		hypotheses := stringSliceFromAny(payload["hypotheses"], 3)
		sustained := intFromAny(synopsis["sustained_reads"])
		premature := intFromAny(synopsis["premature_exits"])
		zero := intFromAny(synopsis["zero_byte_streams"])
		verdict := "needs_review"
		switch {
		case sustained >= 2 && premature == 0 && zero == 0:
			verdict = "stable_parallel_reads"
		case zero > 0:
			verdict = "open_path_failure"
		case premature > 0:
			verdict = "premature_exit"
		}
		return verdict, hypotheses
	case "evidence":
		return "bundle_ready", []string{"Evidence bundle scaffolded; add PMS logs, Tunerr logs, and pcap for the failing window."}
	default:
		return "", nil
	}
}

func stringSliceFromAny(v interface{}, limit int) []string {
	rows, _ := v.([]interface{})
	if len(rows) == 0 {
		return nil
	}
	out := make([]string, 0, min(limit, len(rows)))
	for _, row := range rows {
		text := strings.TrimSpace(fmt.Sprint(row))
		if text == "" {
			continue
		}
		out = append(out, text)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func intFromAny(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func suggestDiagnosticChannels(attempts StreamAttemptReport) (good string, bad string) {
	for _, row := range attempts.Attempts {
		status := strings.ToLower(strings.TrimSpace(row.FinalStatus))
		if good == "" && status != "" &&
			!strings.Contains(status, "fail") &&
			!strings.Contains(status, "reject") &&
			!strings.Contains(status, "error") &&
			!strings.Contains(status, "timeout") &&
			!strings.Contains(status, "http_4") &&
			!strings.Contains(status, "http_5") {
			good = strings.TrimSpace(row.ChannelID)
		}
		if bad == "" && (strings.Contains(status, "fail") ||
			strings.Contains(status, "reject") ||
			strings.Contains(status, "timeout") ||
			strings.Contains(status, "error") ||
			strings.Contains(status, "http_4") ||
			strings.Contains(status, "http_5") ||
			strings.Contains(status, "limited")) {
			bad = strings.TrimSpace(row.ChannelID)
		}
	}
	return strings.TrimSpace(good), strings.TrimSpace(bad)
}

func createEvidenceIntakeBundle(outDir string) error {
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		return fmt.Errorf("evidence output directory required")
	}
	for _, sub := range []string{"bundle", "logs/plex", "logs/tunerr", "pcap", "notes"} {
		if err := os.MkdirAll(filepath.Join(outDir, sub), 0o755); err != nil {
			return err
		}
	}
	notes := fmt.Sprintf(`# Evidence Notes

- Case id: %s
- Created at: %s
- Environment:
  - Working machine:
  - Failing machine:
  - Plex version:
  - Tunerr version/tag:
- Symptom:
  - 
- What changed immediately before the failure:
  - 
- Known differences between working and failing machines:
  - 
- Relevant Plex Preferences.xml differences:
  - 
- Channels tested:
  - working:
  - failing:
- Commands run:
  - 
- Next analysis command:
  - python3 scripts/analyze-bundle.py "%s" --output "%s/report.txt"
`, filepath.Base(outDir), time.Now().UTC().Format(time.RFC3339), outDir, outDir)
	readme := fmt.Sprintf(`Evidence intake bundle for %s

Directory layout:
- bundle/       iptv-tunerr debug-bundle output
- logs/plex/    Plex Media Server logs
- logs/tunerr/  Tunerr stdout/journal logs
- pcap/         packet captures (.pcap / .pcapng)
- notes.md      analyst notes and environment deltas

Recommended next steps:
1. Put the failing-run debug bundle in bundle/
2. Add PMS and Tunerr logs for the same time window
3. Add pcap if available
4. Fill out notes.md with the exact working-vs-failing deltas
5. Run:
   python3 scripts/analyze-bundle.py "%s" --output "%s/report.txt"
`, filepath.Base(outDir), outDir, outDir)
	if err := os.WriteFile(filepath.Join(outDir, "notes.md"), []byte(notes), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "README.txt"), []byte(readme), 0o600); err != nil {
		return err
	}
	return nil
}

func mergeOperatorActionDetail(left, right map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range left {
		out[k] = v
	}
	for k, v := range right {
		out[k] = v
	}
	return out
}

func repoScriptPath(name string) string {
	return filepath.Join(".", "scripts", strings.TrimSpace(name))
}

func runDiagnosticsHarnessAction(ctx context.Context, scriptName, outRoot string, env map[string]string) (map[string]interface{}, error) {
	scriptName = strings.TrimSpace(scriptName)
	if scriptName == "" {
		return nil, fmt.Errorf("script name required")
	}
	scriptPath := repoScriptPath(scriptName)
	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	cmd.Dir = "."
	runEnv := append([]string{}, os.Environ()...)
	for key, value := range env {
		if strings.TrimSpace(key) == "" {
			continue
		}
		runEnv = append(runEnv, key+"="+value)
	}
	cmd.Env = runEnv
	out, err := cmd.CombinedOutput()
	runID := strings.TrimSpace(env["RUN_ID"])
	outDir := ""
	if strings.TrimSpace(outRoot) != "" && runID != "" {
		outDir = filepath.Join(outRoot, runID)
	}
	detail := map[string]interface{}{
		"script":     scriptName,
		"run_id":     runID,
		"output_dir": outDir,
	}
	if reportPath := filepath.Join(outDir, "report.json"); outDir != "" {
		if _, statErr := os.Stat(reportPath); statErr == nil {
			detail["report_path"] = reportPath
		}
		if _, statErr := os.Stat(filepath.Join(outDir, "report.txt")); statErr == nil {
			detail["report_text_path"] = filepath.Join(outDir, "report.txt")
		}
	}
	if len(out) > 0 {
		text := strings.TrimSpace(string(out))
		if len(text) > 1200 {
			text = text[:1200] + "..."
		}
		detail["stdout"] = text
	}
	return detail, err
}

func (s *Server) operatorTunerBaseURL() string {
	if base := strings.TrimSpace(s.BaseURL); base != "" {
		return strings.TrimRight(base, "/")
	}
	addr := strings.TrimSpace(s.Addr)
	if addr == "" {
		addr = ":5004"
	}
	host := "127.0.0.1"
	if strings.HasPrefix(addr, ":") {
		return "http://" + host + addr
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	return "http://" + strings.TrimRight(addr, "/")
}

func (s *Server) channelStreamURL(channelID string) (string, bool) {
	ch, ok := s.findLiveChannel(channelID)
	if !ok {
		return "", false
	}
	if raw := strings.TrimSpace(ch.StreamURL); raw != "" {
		return raw, true
	}
	if len(ch.StreamURLs) > 0 {
		if raw := strings.TrimSpace(ch.StreamURLs[0]); raw != "" {
			return raw, true
		}
	}
	return "", false
}

func (s *Server) diagnosticSuggestedChannels() (string, string) {
	if s.gateway == nil {
		return "", ""
	}
	return suggestDiagnosticChannels(s.gateway.RecentStreamAttempts(12))
}

func (s *Server) buildChannelDiffHarnessEnv(goodID, badID string) (map[string]string, map[string]interface{}, error) {
	goodID = strings.TrimSpace(goodID)
	badID = strings.TrimSpace(badID)
	suggestedGood, suggestedBad := s.diagnosticSuggestedChannels()
	if goodID == "" {
		goodID = suggestedGood
	}
	if badID == "" {
		badID = suggestedBad
	}
	if goodID == "" || badID == "" {
		return nil, nil, fmt.Errorf("good_channel_id and bad_channel_id are required or must be inferable from recent attempts")
	}
	goodURL, ok := s.channelStreamURL(goodID)
	if !ok {
		return nil, nil, fmt.Errorf("no direct source found for good channel %q", goodID)
	}
	badURL, ok := s.channelStreamURL(badID)
	if !ok {
		return nil, nil, fmt.Errorf("no direct source found for bad channel %q", badID)
	}
	runID := "operator-" + time.Now().UTC().Format("20060102-150405")
	env := map[string]string{
		"TUNERR_BASE_URL": s.operatorTunerBaseURL(),
		"GOOD_CHANNEL_ID": goodID,
		"BAD_CHANNEL_ID":  badID,
		"GOOD_DIRECT_URL": goodURL,
		"BAD_DIRECT_URL":  badURL,
		"RUN_ID":          runID,
		"OUT_ROOT":        filepath.Join(repoDiagRoot(), "channel-diff"),
		"RUN_SECONDS":     "8",
		"SEED_SECONDS":    "4",
		"ATTEMPT_LIMIT":   "40",
		"USE_FFPLAY":      "false",
		"USE_TCPDUMP":     "false",
	}
	detail := map[string]interface{}{
		"good_channel_id": goodID,
		"bad_channel_id":  badID,
		"good_direct_url": safeurl.RedactURL(goodURL),
		"bad_direct_url":  safeurl.RedactURL(badURL),
		"run_id":          runID,
	}
	return env, detail, nil
}

func (s *Server) buildStreamCompareHarnessEnv(channelID string) (map[string]string, map[string]interface{}, error) {
	channelID = strings.TrimSpace(channelID)
	_, suggestedBad := s.diagnosticSuggestedChannels()
	if channelID == "" {
		channelID = suggestedBad
	}
	if channelID == "" {
		return nil, nil, fmt.Errorf("channel_id is required or must be inferable from recent attempts")
	}
	directURL, ok := s.channelStreamURL(channelID)
	if !ok {
		return nil, nil, fmt.Errorf("no direct source found for channel %q", channelID)
	}
	runID := "operator-" + time.Now().UTC().Format("20060102-150405")
	env := map[string]string{
		"TUNERR_BASE_URL":   s.operatorTunerBaseURL(),
		"CHANNEL_ID":        channelID,
		"DIRECT_URL":        directURL,
		"RUN_ID":            runID,
		"OUT_ROOT":          filepath.Join(repoDiagRoot(), "stream-compare"),
		"RUN_SECONDS":       "8",
		"USE_FFPLAY":        "false",
		"USE_TCPDUMP":       "false",
		"ANALYZE_MANIFESTS": "true",
	}
	detail := map[string]interface{}{
		"channel_id": channelID,
		"direct_url": safeurl.RedactURL(directURL),
		"run_id":     runID,
	}
	return env, detail, nil
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

func (s *Server) serveRecordingRules() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			body, err := json.MarshalIndent(s.reloadRecordingRules(), "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode recording rules"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 131072)
			var req struct {
				Action  string          `json:"action"`
				RuleID  string          `json:"rule_id"`
				Enabled *bool           `json:"enabled,omitempty"`
				Rule    RecordingRule   `json:"rule"`
				Rules   []RecordingRule `json:"rules"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			rules := s.reloadRecordingRules()
			switch strings.ToLower(strings.TrimSpace(req.Action)) {
			case "", "upsert":
				rules = upsertRecordingRule(rules, req.Rule)
			case "replace":
				rules = normalizeRecordingRuleset(RecordingRuleset{Rules: req.Rules})
			case "delete":
				rules = deleteRecordingRule(rules, req.RuleID)
			case "toggle":
				enabled := true
				if req.Enabled != nil {
					enabled = *req.Enabled
				}
				rules = toggleRecordingRule(rules, req.RuleID, enabled)
			default:
				http.Error(w, `{"error":"unsupported action"}`, http.StatusBadRequest)
				return
			}
			saved, err := s.saveRecordingRules(rules)
			if err != nil {
				http.Error(w, `{"error":"save recording rules failed"}`, http.StatusBadGateway)
				return
			}
			body, err := json.MarshalIndent(saved, "", "  ")
			if err != nil {
				http.Error(w, `{"error":"encode recording rules"}`, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) serveRecordingRulePreview() http.Handler {
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
		limit := 50
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		preview, err := s.xmltv.CatchupCapsulePreview(time.Now(), horizon, limit)
		if err != nil {
			http.Error(w, `{"error":"recording rule preview failed"}`, http.StatusBadGateway)
			return
		}
		body, err := json.MarshalIndent(buildRecordingRulePreview(s.reloadRecordingRules(), preview), "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode recording rule preview"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveRecordingHistory() http.Handler {
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
		report, err := LoadCatchupRecorderReport(stateFile, streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 25))
		if err != nil {
			http.Error(w, `{"error":"load recorder history failed"}`, http.StatusBadGateway)
			return
		}
		body, err := json.MarshalIndent(buildRecordingRuleHistory(s.reloadRecordingRules(), report), "", "  ")
		if err != nil {
			http.Error(w, `{"error":"encode recording history"}`, http.StatusInternalServerError)
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

package tuner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
)

// PlexDVRMaxChannels is Plex's per-tuner channel limit when using the wizard; exceeding it causes "failed to save channel lineup".
const PlexDVRMaxChannels = 480

// PlexDVRWizardSafeMax is used in "easy" mode: strip from end so lineup fits when Plex suggests a guide (e.g. Rogers West Canada ~680 ch); keep first N.
const PlexDVRWizardSafeMax = 479

// NoLineupCap disables the lineup cap (use when syncing lineup into Plex DB programmatically so users get full channel count).
const NoLineupCap = -1

// Server runs the HDHR emulator + XMLTV + stream gateway.
// Handlers are kept so UpdateChannels can refresh the channel list without restart.
type Server struct {
	Addr                string
	BaseURL             string
	TunerCount          int
	LineupMaxChannels   int    // max channels in lineup/guide (default PlexDVRMaxChannels); 0 = use PlexDVRMaxChannels
	GuideNumberOffset   int    // add offset to exposed GuideNumber values to avoid cross-DVR collisions in Plex clients
	DeviceID            string // HDHomeRun discover.json; set from PLEX_TUNER_DEVICE_ID
	FriendlyName        string // HDHomeRun discover.json; set from PLEX_TUNER_FRIENDLY_NAME
	StreamBufferBytes   int    // 0 = no buffer; -1 = auto; e.g. 2097152 for 2 MiB
	StreamTranscodeMode string // "off" | "on" | "auto"
	Channels            []catalog.LiveChannel
	ProviderUser        string
	ProviderPass        string
	XMLTVSourceURL      string
	XMLTVTimeout        time.Duration
	XMLTVCacheTTL       time.Duration // 0 = use default 10m
	EpgPruneUnlinked    bool          // when true, guide.xml and /live.m3u only include channels with tvg-id

	// health state updated by UpdateChannels; read by /healthz.
	healthMu       sync.RWMutex
	healthChannels int
	healthRefresh  time.Time

	hdhr     *HDHR
	gateway  *Gateway
	xmltv    *XMLTV
	m3uServe *M3UServe
}

// UpdateChannels updates the channel list for all handlers so -refresh can serve new lineup without restart.
// Caps at LineupMaxChannels (default PlexDVRMaxChannels) so Plex DVR can save the lineup when using the wizard (Plex fails above ~480).
// When LineupMaxChannels is NoLineupCap, no cap is applied (for programmatic lineup sync; see -register-plex).
func (s *Server) UpdateChannels(live []catalog.LiveChannel) {
	live = applyLineupPreCapFilters(live)
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
	out := live
	if envBool("PLEX_TUNER_LINEUP_DROP_MUSIC", false) {
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
	if want := normalizeLineupLangFilter(strings.TrimSpace(os.Getenv("PLEX_TUNER_LINEUP_LANGUAGE"))); want != "" && want != "all" {
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
	if pat := strings.TrimSpace(os.Getenv("PLEX_TUNER_LINEUP_EXCLUDE_REGEX")); pat != "" {
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
				log.Printf("Lineup pre-cap filter: dropped %d channels by PLEX_TUNER_LINEUP_EXCLUDE_REGEX (remaining %d)", dropped, len(filtered))
				out = filtered
			}
		}
	}
	if cat := strings.TrimSpace(os.Getenv("PLEX_TUNER_LINEUP_CATEGORY")); cat != "" && cat != "all" {
		filtered := make([]catalog.LiveChannel, 0, len(out))
		dropped := 0
		for _, ch := range out {
			if liveChannelMatchesCategory(ch, cat) {
				filtered = append(filtered, ch)
			} else {
				dropped++
			}
		}
		log.Printf("Lineup pre-cap filter: category=%s kept=%d dropped=%d", cat, len(filtered), dropped)
		out = filtered
	}
	out = applyLineupWizardShape(out)
	out = applyLineupShard(out)
	return out
}

// classifyLiveChannel derives coarse content-type and region from a LiveChannel's
// GroupTitle and GuideName. The classification mirrors the external split-m3u.py
// logic so that in-app PLEX_TUNER_LINEUP_CATEGORY filtering produces the same
// buckets as the external splitter.
//
// Content type values: sports, movies, news, kids, music, general
// Region values: ca, us, na (either CA or US), uk, ie, ukie, europe, nordics,
//
//	eusouth, eueast, latam, mena, intl
func classifyLiveChannel(ch catalog.LiveChannel) (contentType, region string) {
	group := strings.ToUpper(strings.TrimSpace(ch.GroupTitle))
	name := strings.ToUpper(strings.TrimSpace(ch.GuideName))
	tvg := strings.ToUpper(strings.TrimSpace(ch.TVGID))
	all := " " + group + " " + name + " " + tvg + " "

	// --- Region from group-title prefix (e.g. "US | ESPN HD" → prefix = "US") ---
	prefix := ""
	if idx := strings.Index(group, "|"); idx > 0 {
		prefix = strings.TrimSpace(group[:idx])
	} else if idx := strings.Index(group, ":"); idx > 0 {
		prefix = strings.TrimSpace(group[:idx])
	}

	switch prefix {
	case "CA", "CAN":
		region = "ca"
	case "US", "USA":
		region = "us"
	case "UK", "GB":
		region = "uk"
	case "IE":
		region = "ukie"
	case "SE", "NO", "DK", "FI":
		region = "nordics"
	case "FR", "DE", "NL", "BE", "CH", "AT":
		region = "europe"
	case "IT", "GR", "CY", "MT":
		region = "eusouth"
	case "ES", "PT":
		region = "europe"
	case "PL", "RU", "SR", "HU", "RO", "CZ", "BG", "AL", "HR", "TR", "MK", "BA", "SI", "UA":
		region = "eueast"
	case "AR", "BR", "MX", "CO", "CL", "PE", "CU":
		region = "latam"
	}

	// Fallback region from tvg-id TLD.
	if region == "" {
		switch {
		case strings.HasSuffix(tvg, ".CA"):
			region = "ca"
		case strings.HasSuffix(tvg, ".US"):
			region = "us"
		case strings.HasSuffix(tvg, ".UK"), strings.HasSuffix(tvg, ".GB"):
			region = "uk"
		}
	}

	// Fallback region from name keywords.
	if region == "" {
		switch {
		case liveHasAny(all, " CBC ", " CTV ", " CITY TV ", " CITYTV ", " OMNI ", " NOOVO ", " TVA "):
			region = "ca"
		case liveHasAny(all, " NBC ", " CBS ", " ABC ", " FOX ", " PBS ", " CW "):
			region = "us"
		case liveHasAny(all, " BBC ", " ITV ", " CHANNEL 4 ", " SKY ONE ", " CHANNEL 5 "):
			region = "uk"
		}
	}

	if region == "" {
		region = "intl"
	}

	// --- Content type ---
	switch {
	case liveHasAny(all,
		" SPORT", "SPORTS", " ESPN", " TSN ", " DAZN ", " BEIN SPORT",
		" NFL ", " NBA ", " NHL ", " MLB ", " UFC ", " WWE ",
		" F1 ", " FORMULA 1 ", " MOTOGP ", " PREMIER LEAGUE ", " BUNDESLIGA ",
		" LALIGA ", " SERIE A ", " CHAMPIONS LEAGUE ", " SPORTSNET ", " SPORTSCENTRE "):
		contentType = "sports"
	case liveHasAny(all,
		" NEWS", " BUSINESS ", " WEATHER NETWORK ", " CNN ", " FOX NEWS ", " MSNBC ",
		" CNBC ", " BLOOMBERG ", " AL JAZEERA ", " FRANCE 24 ", " SKY NEWS ",
		" CP24 ", " LCN ", " BNN "):
		contentType = "news"
	case liveHasAny(all,
		" MOVIE", " MOVIES", " CINEMA", " FILM", " HBO ", " SHOWTIME ", " STARZ ",
		" CINEMAX ", " EPIX ", " MGM+", " SKY CINEMA"):
		contentType = "movies"
	case liveHasAny(all,
		" KIDS", " CARTOON", " ANIME", " DISNEY ", " DISNEY JR", " DISNEY XD",
		" NICKELODEON", " NICK JR", " PBS KIDS", " TREEHOUSE", " FAMILY CHANNEL",
		" TELETOON", " BABY", " BAMBINI"):
		contentType = "kids"
	case liveHasAny(all,
		"MUSIC", " RADIO ", " STINGRAY ", " VEVO ", " MTV ", " MUCHMUSIC ",
		"KARAOKE", "JUKEBOX", " MEZZO ", " CLUBBING "):
		contentType = "music"
	default:
		contentType = "general"
	}
	return contentType, region
}

// liveChannelMatchesCategory returns true when the channel belongs to the
// requested category bucket. Category values match the new DVR bucket names
// (case-insensitive). Multiple comma-separated values are supported.
//
// Supported values:
//
//	canada, us, na, sports, movies, news, kids, music, general,
//	uk, ukie, europe, nordics, eusouth, eueast, latam, mena, intl
func liveChannelMatchesCategory(ch catalog.LiveChannel, want string) bool {
	ctype, region := classifyLiveChannel(ch)
	for _, w := range strings.Split(strings.ToLower(want), ",") {
		w = strings.TrimSpace(w)
		switch w {
		case "canada", "ca":
			if region == "ca" {
				return true
			}
		case "us":
			if region == "us" {
				return true
			}
		case "na":
			if region == "ca" || region == "us" {
				return true
			}
		case "sports":
			if ctype == "sports" {
				return true
			}
		case "movies":
			if ctype == "movies" {
				return true
			}
		case "news":
			if ctype == "news" {
				return true
			}
		case "kids":
			if ctype == "kids" {
				return true
			}
		case "music":
			if ctype == "music" {
				return true
			}
		case "general":
			if ctype == "general" {
				return true
			}
		case "uk":
			if region == "uk" || region == "ukie" {
				return true
			}
		case "ukie":
			if region == "uk" || region == "ukie" {
				return true
			}
		case "europe":
			if region == "europe" || region == "nordics" || region == "eusouth" || region == "eueast" || region == "uk" || region == "ukie" {
				return true
			}
		case "nordics":
			if region == "nordics" {
				return true
			}
		case "eusouth":
			if region == "eusouth" {
				return true
			}
		case "eueast":
			if region == "eueast" {
				return true
			}
		case "latam":
			if region == "latam" {
				return true
			}
		case "mena":
			if region == "mena" {
				return true
			}
		case "intl":
			if region == "intl" {
				return true
			}
		}
	}
	return false
}

func liveHasAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
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
	skip := envInt("PLEX_TUNER_LINEUP_SKIP", 0)
	take := envInt("PLEX_TUNER_LINEUP_TAKE", 0)
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
	shape := strings.ToLower(strings.TrimSpace(os.Getenv("PLEX_TUNER_LINEUP_SHAPE")))
	if shape == "" || shape == "off" || shape == "none" {
		return live
	}
	region := strings.ToLower(strings.TrimSpace(os.Getenv("PLEX_TUNER_LINEUP_REGION_PROFILE")))
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

	// Use the gateway's HTTP client if available, otherwise default client
	client := http.DefaultClient
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
	overridePath := os.Getenv("PLEX_TUNER_PROFILE_OVERRIDES_FILE")
	overrides, err := loadProfileOverridesFile(overridePath)
	if err != nil {
		log.Printf("Profile overrides disabled: load %q failed: %v", overridePath, err)
	} else if len(overrides) > 0 {
		log.Printf("Profile overrides loaded: %d entries from %s (default=%s)", len(overrides), overridePath, defaultProfile)
	} else {
		log.Printf("Profile overrides: none (default=%s)", defaultProfile)
	}
	txOverridePath := os.Getenv("PLEX_TUNER_TRANSCODE_OVERRIDES_FILE")
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
		streamMode = strings.TrimSpace(os.Getenv("PLEX_TUNER_STREAM_TRANSCODE"))
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
		PlexPMSURL:          strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_URL")),
		PlexPMSToken:        strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_TOKEN")),
		PlexClientAdapt:     strings.EqualFold(strings.TrimSpace(os.Getenv("PLEX_TUNER_CLIENT_ADAPT")), "1") || strings.EqualFold(strings.TrimSpace(os.Getenv("PLEX_TUNER_CLIENT_ADAPT")), "true") || strings.EqualFold(strings.TrimSpace(os.Getenv("PLEX_TUNER_CLIENT_ADAPT")), "yes"),
	}
	log.Printf("Gateway stream mode: transcode=%q buffer_bytes=%d", gateway.StreamTranscodeMode, gateway.StreamBufferBytes)
	if gateway.PlexClientAdapt {
		log.Printf("Gateway Plex client adapt enabled: pms_url=%q token_set=%t", gateway.PlexPMSURL, gateway.PlexPMSToken != "")
	}
	if gateway.Client == nil {
		gateway.Client = httpclient.ForStreaming()
	}
	maybeStartPlexSessionReaper(ctx, gateway.Client)
	s.gateway = gateway
	xmltv := &XMLTV{
		Channels:         s.Channels,
		EpgPruneUnlinked: s.EpgPruneUnlinked,
		SourceURL:        s.XMLTVSourceURL,
		SourceTimeout:    s.XMLTVTimeout,
		CacheTTL:         s.XMLTVCacheTTL,
	}
	s.xmltv = xmltv
	m3uServe := &M3UServe{BaseURL: s.BaseURL, Channels: s.Channels, EpgPruneUnlinked: s.EpgPruneUnlinked}
	s.m3uServe = m3uServe

	addr := s.Addr
	if addr == "" {
		addr = ":5004"
	}

	if envBool("PLEX_TUNER_SSDP_DISABLED", false) {
		log.Printf("SSDP disabled via PLEX_TUNER_SSDP_DISABLED")
	} else {
		StartSSDP(ctx, addr, s.BaseURL, s.DeviceID)
	}

	mux := http.NewServeMux()
	mux.Handle("/discover.json", hdhr)
	mux.Handle("/lineup_status.json", hdhr)
	mux.Handle("/lineup.json", hdhr)
	mux.Handle("/device.xml", s.serveDeviceXML())
	mux.Handle("/guide.xml", xmltv)
	mux.Handle("/live.m3u", m3uServe)
	mux.Handle("/stream/", gateway)
	mux.Handle("/healthz", s.serveHealth())

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
		s.healthMu.RLock()
		count := s.healthChannels
		lastRefresh := s.healthRefresh
		s.healthMu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		if count == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"loading"}`))
			return
		}
		body, _ := json.Marshal(map[string]interface{}{
			"status":       "ok",
			"channels":     count,
			"last_refresh": lastRefresh.Format(time.RFC3339),
		})
		_, _ = w.Write(body)
	})
}

func (s *Server) serveDeviceXML() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceID := s.DeviceID
		if deviceID == "" {
			deviceID = "plextuner01"
		}
		friendlyName := "Plex Tuner"
		deviceXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
    <friendlyName>%s</friendlyName>
    <manufacturer>Plex Tuner</manufacturer>
    <modelName>Plex Tuner</modelName>
    <UDN>uuid:%s</UDN>
  </device>
</root>`, friendlyName, deviceID)
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(deviceXML))
	})
}

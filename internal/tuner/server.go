package tuner

import (
	"context"
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
	"github.com/snapetech/iptvtunerr/internal/channelreport"
	"github.com/snapetech/iptvtunerr/internal/entitlements"
	"github.com/snapetech/iptvtunerr/internal/epgstore"
	"github.com/snapetech/iptvtunerr/internal/eventhooks"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/plexharvest"
	"github.com/snapetech/iptvtunerr/internal/programming"
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
	AppVersion                string
	StreamBufferBytes         int    // 0 = no buffer; -1 = auto; e.g. 2097152 for 2 MiB
	StreamTranscodeMode       string // "off" | "on" | "auto"
	AutopilotStateFile        string // optional JSON file for remembered dna_id+client_class playback decisions
	RecorderStateFile         string // optional JSON file written by catchup-daemon for recorder status/reporting
	RecordingRulesFile        string // optional JSON file for durable recording rule configuration
	Movies                    []catalog.Movie
	Series                    []catalog.Series
	Channels                  []catalog.LiveChannel
	RawChannels               []catalog.LiveChannel
	GuidePolicySourceChannels []catalog.LiveChannel
	ProgrammingRecipeFile     string
	ProgrammingRecipe         programming.Recipe
	PlexLineupHarvestFile     string
	PlexLineupHarvest         plexharvest.Report
	VirtualChannelsFile       string
	VirtualRecoveryStateFile  string
	VirtualChannels           virtualchannels.Ruleset
	RecordingRules            RecordingRuleset
	EventHooksFile            string
	EventHooks                *eventhooks.Dispatcher
	XtreamOutputUser          string
	XtreamOutputPass          string
	XtreamUsersFile           string
	XtreamEntitlements        entitlements.Ruleset
	ProviderUser              string
	ProviderPass              string
	ProviderBaseURL           string
	ProviderIdentities        []ProviderIdentity
	XMLTVSourceURL            string
	XMLTVTimeout              time.Duration
	XMLTVCacheTTL             time.Duration // 0 = use default 10m
	XMLTVPlexSafeIDs          bool          // when true, /guide.xml emits Plex-safe stable channel ids instead of raw guide numbers
	EpgPruneUnlinked          bool          // when true, guide.xml and /live.m3u only include channels with tvg-id
	EpgForceLineupMatch       bool          // when true, guide.xml keeps every lineup row even if prune-unlinked is enabled
	FetchCFReject             bool          // abort HLS stream if segment redirected to CF abuse page (passed to Gateway)
	ProviderEPGEnabled        bool
	ProviderEPGTimeout        time.Duration
	ProviderEPGCacheTTL       time.Duration
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
	runtimeMu       sync.RWMutex

	// health state updated by UpdateChannels; read by /healthz and /readyz.
	healthMu       sync.RWMutex
	healthChannels int
	healthRefresh  time.Time

	virtualRecoveryMu     sync.Mutex
	virtualRecoveryEvents []virtualChannelRecoveryEvent
	virtualRecoveryLoaded bool

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

func cloneInterfaceMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneRuntimeSnapshot(src *RuntimeSnapshot) *RuntimeSnapshot {
	if src == nil {
		return nil
	}
	return &RuntimeSnapshot{
		GeneratedAt:   src.GeneratedAt,
		Version:       src.Version,
		ListenAddress: src.ListenAddress,
		BaseURL:       src.BaseURL,
		DeviceID:      src.DeviceID,
		FriendlyName:  src.FriendlyName,
		Tuner:         cloneInterfaceMap(src.Tuner),
		Guide:         cloneInterfaceMap(src.Guide),
		Provider:      cloneInterfaceMap(src.Provider),
		Recorder:      cloneInterfaceMap(src.Recorder),
		HDHR:          cloneInterfaceMap(src.HDHR),
		WebUI:         cloneInterfaceMap(src.WebUI),
		Events:        cloneInterfaceMap(src.Events),
		MediaServers:  cloneInterfaceMap(src.MediaServers),
		URLs:          cloneStringMap(src.URLs),
	}
}

func (s *Server) SetRuntimeSnapshot(snapshot *RuntimeSnapshot) {
	if s == nil {
		return
	}
	s.runtimeMu.Lock()
	s.RuntimeSnapshot = snapshot
	s.runtimeMu.Unlock()
}

func (s *Server) UpdateRuntimeTunerSetting(key string, value interface{}) {
	if s == nil || strings.TrimSpace(key) == "" {
		return
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.RuntimeSnapshot == nil {
		s.RuntimeSnapshot = &RuntimeSnapshot{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Tuner:       map[string]interface{}{},
		}
	}
	if s.RuntimeSnapshot.Tuner == nil {
		s.RuntimeSnapshot.Tuner = map[string]interface{}{}
	}
	s.RuntimeSnapshot.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	s.RuntimeSnapshot.Tuner[key] = value
}

func (s *Server) runtimeSnapshotClone() *RuntimeSnapshot {
	if s == nil {
		return nil
	}
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return cloneRuntimeSnapshot(s.RuntimeSnapshot)
}

func (s *Server) UpdateProviderContext(baseURL, user, pass string, snapshot *RuntimeSnapshot) {
	if s == nil {
		return
	}
	s.ProviderBaseURL = baseURL
	s.ProviderUser = user
	s.ProviderPass = pass
	if s.gateway != nil {
		s.gateway.setProviderCredentials(user, pass)
	}
	if s.xmltv != nil {
		s.xmltv.setProviderIdentity(baseURL, user, pass)
	}
	if snapshot != nil {
		s.SetRuntimeSnapshot(snapshot)
	}
}

// UpdateChannels updates the channel list for all handlers so -refresh can serve new lineup without restart.
// Caps at LineupMaxChannels (default PlexDVRMaxChannels) so Plex DVR can save the lineup when using the wizard (Plex fails above ~480).
// When LineupMaxChannels is NoLineupCap, no cap is applied (for programmatic lineup sync; see -register-plex).
func (s *Server) UpdateChannels(live []catalog.LiveChannel) {
	live = applyLineupBaseFilters(live)
	live = applyDNAPolicy(live, os.Getenv("IPTV_TUNERR_DNA_POLICY"))
	s.RawChannels = cloneLiveChannels(live)
	source, exposed := s.curateChannelsFromRaw(live)
	s.GuidePolicySourceChannels = cloneLiveChannels(source)
	s.setExposedChannels(exposed)
}

func (s *Server) setExposedChannels(live []catalog.LiveChannel) {
	summary := summarizeLineupIntegrity(live)
	if tunerCountAutoConfigured() {
		s.TunerCount = lineupFeedCapacity(live)
		if s.TunerCount < 1 {
			s.TunerCount = 1
		}
	}
	s.Channels = live
	s.healthMu.Lock()
	s.healthChannels = len(live)
	s.healthRefresh = time.Now()
	s.healthMu.Unlock()
	if s.hdhr != nil {
		s.hdhr.Channels = live
		s.hdhr.TunerCount = s.TunerCount
	}
	if s.gateway != nil {
		s.gateway.Channels = live
		s.gateway.TunerCount = s.TunerCount
	}
	if s.xmltv != nil {
		s.xmltv.Channels = live
		s.xmltv.GuideHealthChannels = cloneLiveChannels(s.GuidePolicySourceChannels)
		s.xmltv.mu.Lock()
		s.xmltv.cachedMatchReport = nil
		s.xmltv.cachedMatchAliases = ""
		s.xmltv.cachedMatchExp = time.Time{}
		s.xmltv.cachedGuideHealth = nil
		s.xmltv.cachedCapsulePreview = nil
		s.xmltv.cachedCapsuleHorizon = 0
		s.xmltv.cachedCapsuleExp = time.Time{}
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

func tunerCountAutoConfigured() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("IPTV_TUNERR_TUNER_COUNT")), "auto")
}

func lineupFeedCapacity(live []catalog.LiveChannel) int {
	seen := map[string]struct{}{}
	for _, ch := range live {
		urls := ch.StreamURLs
		if len(urls) == 0 && strings.TrimSpace(ch.StreamURL) != "" {
			urls = []string{ch.StreamURL}
		}
		for _, raw := range urls {
			u := strings.TrimSpace(raw)
			if u == "" {
				continue
			}
			seen[u] = struct{}{}
		}
	}
	return len(seen)
}

func (s *Server) reapplyDeferredGuidePolicyAfterGuideHealthReady() {
	if s == nil || s.xmltv == nil {
		return
	}
	policy := normalizeGuidePolicy(os.Getenv("IPTV_TUNERR_GUIDE_POLICY"))
	if policy == "off" {
		return
	}
	rep, ok := s.xmltv.cachedGuideHealthReport()
	if !ok || !rep.SourceReady {
		return
	}
	live := cloneLiveChannels(s.RawChannels)
	if len(live) == 0 {
		return
	}
	source, filtered := s.curateChannelsFromRaw(live)
	if policy == "placeholder" && rep.Summary.PlaceholderOnlyChannels == 0 && len(s.Channels) < len(filtered) {
		return
	}
	if len(filtered) == len(s.Channels) && len(source) == len(s.GuidePolicySourceChannels) {
		return
	}
	log.Printf("Guide policy cache ready: rebuilding exposed lineup from raw channels; kept=%d/%d", len(filtered), len(source))
	s.GuidePolicySourceChannels = cloneLiveChannels(source)
	s.setExposedChannels(filtered)
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
	source, exposed := s.curateChannelsFromRaw(live)
	s.GuidePolicySourceChannels = cloneLiveChannels(source)
	s.setExposedChannels(exposed)
}

func (s *Server) curateChannelsFromRaw(live []catalog.LiveChannel) ([]catalog.LiveChannel, []catalog.LiveChannel) {
	live = s.applyProgrammingRecipe(live)
	live = applyLineupExcludeRecipe(live)
	live = applyLineupRecipe(live)
	live = applyLineupWizardShape(live)
	live = applyLineupDedupe(live)
	live = applyLineupShard(live)
	live = applyGuideNumberResequence(live)
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
	live = applyLineupProbeFilter(live)
	source := cloneLiveChannels(live)
	if s.xmltv != nil {
		live = s.xmltv.applyGuidePolicyToChannels(live, os.Getenv("IPTV_TUNERR_GUIDE_POLICY"))
	}
	return source, live
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

func applyGuideNumberResequence(live []catalog.LiveChannel) []catalog.LiveChannel {
	if !envBool("IPTV_TUNERR_GUIDE_NUMBER_RESEQUENCE", false) || len(live) == 0 {
		return live
	}
	start := envInt("IPTV_TUNERR_GUIDE_NUMBER_RESEQUENCE_START", 1)
	if start < 1 {
		start = 1
	}
	out := make([]catalog.LiveChannel, len(live))
	copy(out, live)
	for i := range out {
		out[i].GuideNumber = strconv.Itoa(start + i)
	}
	log.Printf("Guide number resequence applied: start=%d changed=%d", start, len(out))
	return out
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

func lineupExcludedChannelIDs() map[string]struct{} {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_EXCLUDE_CHANNEL_IDS"))
	if raw == "" {
		return nil
	}
	out := map[string]struct{}{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func lineupChannelIDMatchesAny(ch catalog.LiveChannel, ids map[string]struct{}) bool {
	if len(ids) == 0 {
		return false
	}
	for _, candidate := range []string{ch.ChannelID, ch.GuideNumber, ch.TVGID} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := ids[candidate]; ok {
			return true
		}
	}
	return false
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
	if envBool("IPTV_TUNERR_LINEUP_DROP_SPORTS", false) {
		filtered := make([]catalog.LiveChannel, 0, len(out))
		dropped := 0
		for _, ch := range out {
			if lineupLooksLikeSportsChannel(ch) {
				dropped++
				continue
			}
			filtered = append(filtered, ch)
		}
		if dropped > 0 {
			log.Printf("Lineup pre-cap filter: dropped %d sports/event channels by name heuristic (remaining %d)", dropped, len(filtered))
			out = filtered
		}
	}
	if excludedIDs := lineupExcludedChannelIDs(); len(excludedIDs) > 0 {
		filtered := make([]catalog.LiveChannel, 0, len(out))
		dropped := 0
		for _, ch := range out {
			if lineupChannelIDMatchesAny(ch, excludedIDs) {
				dropped++
				continue
			}
			filtered = append(filtered, ch)
		}
		if dropped > 0 {
			log.Printf("Lineup pre-cap filter: dropped %d channels by IPTV_TUNERR_LINEUP_EXCLUDE_CHANNEL_IDS (remaining %d)", dropped, len(filtered))
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

func applyLineupExcludeRecipe(live []catalog.LiveChannel) []catalog.LiveChannel {
	recipe := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_EXCLUDE_RECIPE")))
	if recipe == "" || recipe == "off" || recipe == "none" || len(live) == 0 {
		return live
	}
	switch recipe {
	case "sports_now", "sports_na", "kids_safe":
	default:
		log.Printf("Lineup exclude recipe ignored: unknown recipe=%q", recipe)
		return live
	}
	excluded := make(map[string]struct{}, len(live))
	for _, ch := range live {
		if !lineupChannelMatchesExcludeRecipe(ch, recipe) {
			continue
		}
		if id := strings.TrimSpace(ch.ChannelID); id != "" {
			excluded[id] = struct{}{}
		}
	}
	if len(excluded) == 0 {
		return live
	}
	filtered := make([]catalog.LiveChannel, 0, len(live))
	dropped := 0
	for _, ch := range live {
		if _, ok := excluded[strings.TrimSpace(ch.ChannelID)]; ok {
			dropped++
			continue
		}
		filtered = append(filtered, ch)
	}
	if dropped > 0 {
		log.Printf("Lineup pre-cap filter: dropped %d channels by IPTV_TUNERR_LINEUP_EXCLUDE_RECIPE=%s (remaining %d)", dropped, recipe, len(filtered))
	}
	return filtered
}

func lineupChannelMatchesExcludeRecipe(ch catalog.LiveChannel, recipe string) bool {
	switch recipe {
	case "sports_now":
		return lineupRecipeSportsLike(ch)
	case "sports_na":
		return lineupRecipeNorthAmericanSportsScore(ch) > 0
	case "kids_safe":
		return lineupRecipeKidsSafe(ch)
	default:
		return false
	}
}

func applyLineupPreCapFilters(live []catalog.LiveChannel) []catalog.LiveChannel {
	out := applyLineupBaseFilters(live)
	out = applyLineupExcludeRecipe(out)
	out = applyLineupRecipe(out)
	out = applyLineupWizardShape(out)
	out = applyLineupDedupe(out)
	out = applyLineupShard(out)
	out = applyGuideNumberResequence(out)
	return out
}

func applyLineupDedupe(live []catalog.LiveChannel) []catalog.LiveChannel {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_DEDUPE")))
	if mode == "" || mode == "off" || mode == "none" || mode == "false" || len(live) == 0 {
		return live
	}
	switch mode {
	case "stable", "identity", "strong", "true", "1":
	default:
		log.Printf("Lineup dedupe ignored: unknown mode=%q", mode)
		return live
	}

	type selected struct {
		ch        catalog.LiveChannel
		firstIdx  int
		bestScore int
		dups      int
	}
	byKey := map[string]*selected{}

	for i, ch := range live {
		key := lineupDedupeKey(ch)
		if key == "" {
			key = "row:" + strconv.Itoa(i)
		}
		score := lineupDedupeScore(ch)
		cur, ok := byKey[key]
		if !ok {
			byKey[key] = &selected{ch: ch, firstIdx: i, bestScore: score}
			continue
		}
		cur.dups++
		if score > cur.bestScore {
			cur.ch = ch
			cur.bestScore = score
		}
	}

	if len(byKey) == 0 {
		return live
	}
	rows := make([]selected, 0, len(byKey))
	dropped := 0
	for _, row := range byKey {
		rows = append(rows, *row)
		dropped += row.dups
	}
	if dropped == 0 {
		return live
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].firstIdx < rows[j].firstIdx
	})
	out := make([]catalog.LiveChannel, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ch)
	}
	log.Printf("Lineup pre-cap dedupe: mode=%s dropped=%d kept=%d/%d", mode, dropped, len(out), len(live))
	return out
}

func lineupDedupeKey(ch catalog.LiveChannel) string {
	if tvgid := strings.ToLower(strings.TrimSpace(ch.TVGID)); tvgid != "" {
		return "tvg:" + tvgid
	}
	if name := normalizedLineupDedupeName(ch.GuideName); name != "" {
		return "name:" + name
	}
	if dna := strings.ToLower(strings.TrimSpace(ch.DNAID)); dna != "" {
		return "dna:" + dna
	}
	return ""
}

func normalizedLineupDedupeName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return ""
	}
	if idx := strings.Index(s, "|"); idx >= 0 && idx+1 < len(s) {
		s = strings.TrimSpace(s[idx+1:])
	}
	s = strings.NewReplacer(
		"ᴴᴰ", " hd ",
		"ᴿᴬᵂ", " raw ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"-", " ",
		"_", " ",
		"/", " ",
		"+", " ",
	).Replace(s)
	tokens := strings.Fields(s)
	filtered := tokens[:0]
	for _, tok := range tokens {
		switch tok {
		case "hd", "fhd", "uhd", "4k", "raw", "backup", "bk", "hevc", "h265", "h.265", "h264", "h.264":
			continue
		default:
			filtered = append(filtered, tok)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, " ")
}

func lineupDedupeScore(ch catalog.LiveChannel) int {
	score := channelreport.Score(ch)
	if ch.EPGLinked {
		score += 80
	}
	if strings.TrimSpace(ch.TVGID) != "" {
		score += 50
	}
	if strings.TrimSpace(ch.DNAID) != "" {
		score += 10
	}
	if len(ch.StreamURLs) > 1 {
		score += 5 * len(ch.StreamURLs)
	}
	name := strings.ToLower(strings.TrimSpace(ch.GuideName))
	switch {
	case strings.Contains(name, " fhd"):
		score += 8
	case strings.Contains(name, " hd"):
		score += 5
	}
	for _, n := range []string{" raw", " backup", " bk", " sd"} {
		if strings.Contains(name, n) {
			score -= 15
		}
	}
	return score
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
	case "sports_na":
		for _, row := range rows {
			if lineupRecipeNorthAmericanSportsScore(row.ch) > 0 {
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
		case "sports_na":
			left := lineupRecipeNorthAmericanSportsScore(rows[i].ch)
			right := lineupRecipeNorthAmericanSportsScore(rows[j].ch)
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
	carriageByNetwork := map[string]int{}
	for _, row := range rows {
		ch := row.ch
		if recipe == "sports_na" {
			if network := lineupRecipeNorthAmericanSportsCarriageNetwork(ch); network != "" {
				if carriageByNetwork[network] >= 12 {
					continue
				}
				carriageByNetwork[network]++
			}
			ch = normalizeSportsNAEventStreamURL(ch)
		}
		out = append(out, ch)
	}
	log.Printf("Lineup recipe applied: recipe=%s kept=%d/%d", recipe, len(out), len(live))
	return out
}

func normalizeSportsNAEventStreamURL(ch catalog.LiveChannel) catalog.LiveChannel {
	s := lineupRecipeSearchText(ch)
	if !strings.Contains(s, "peacock") {
		return ch
	}
	if fallback, ok := hlsTSFallbackURL(ch.StreamURL); ok {
		ch.StreamURL = fallback
	}
	if len(ch.StreamURLs) > 0 {
		out := make([]string, 0, len(ch.StreamURLs))
		seen := map[string]bool{}
		for _, raw := range ch.StreamURLs {
			u := strings.TrimSpace(raw)
			if fallback, ok := hlsTSFallbackURL(u); ok {
				u = fallback
			}
			if u == "" || seen[u] {
				continue
			}
			seen[u] = true
			out = append(out, u)
		}
		ch.StreamURLs = out
	} else if strings.TrimSpace(ch.StreamURL) != "" {
		ch.StreamURLs = []string{ch.StreamURL}
	}
	return ch
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

func lineupLooksLikeSportsChannel(ch catalog.LiveChannel) bool {
	if lineupRecipeSportsLike(ch) || lineupRecipeNorthAmericanSportsScore(ch) > 0 {
		return true
	}
	s := lineupRecipeSearchText(ch)
	for _, term := range []string{
		" sport ", " sports", " fanduel", " msg ", " yes network", " acc network", " big ten network",
		" stadium", " sportsman", " outdoor channel", " wwe", " pokergo", " fuel tv", " tvg network",
		" flo", "flohockey", "flobaseball", "milb", "hockey", "volleyball", " whl ", " tigres",
		" blades", " raiders", " calgary wranglers", " manitoba moose", " vancouver whitecaps",
		" washington spirit", " soccer", " rugby", " lacrosse", " wrestling", " redzone", " team white",
		" team black", " academy ", " canucks", " oilers", " flames", " jets", "fc|",
	} {
		if strings.Contains(s, term) {
			return true
		}
	}
	name := strings.ToLower(strings.TrimSpace(ch.GuideName))
	return strings.HasPrefix(name, "nhl team|") || strings.HasPrefix(name, "mls ")
}

func lineupRecipeNorthAmericanSportsScore(ch catalog.LiveChannel) int {
	eventScore := lineupRecipeNorthAmericanSportsEventScore(ch)
	carriageScore := lineupRecipeNorthAmericanSportsCarriageScore(ch)
	if !lineupRecipeSportsLike(ch) && eventScore == 0 && carriageScore == 0 {
		return 0
	}
	s := lineupRecipeSearchText(ch)
	if eventScore == 0 {
		for _, blocked := range []string{
			" team|", " sportsnet ppv", " ppv",
		} {
			if strings.Contains(s, blocked) {
				return 0
			}
		}
	}
	score := 0
	tvgid := strings.ToLower(strings.TrimSpace(ch.TVGID))
	switch {
	case strings.HasSuffix(tvgid, ".ca"):
		score += 140
	case strings.HasSuffix(tvgid, ".us"):
		score += 120
	default:
		if eventScore > 0 {
			score += eventScore
		} else if carriageScore > 0 {
			score += carriageScore
		} else if scoreLineupChannelForShape("na_en", strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_REGION_PROFILE"))), ch) < 40 {
			return 0
		} else {
			score += 40
		}
	}
	if tvgid != "" || eventScore > 0 {
		score += carriageScore
	}
	for _, term := range []string{
		" tsn", " sportsnet", " sn ", " espn", " espnews", " espnu", " sec network", " acc network",
		" nfl network", " mlb network", " nba tv", " nhl network", " golf channel", " tennis channel",
		" fox sports", " fs1", " fs2", " cbs sports", " nbc sports", " tnt sports", " bein sports us",
		" sportsman", " outd", " willow", " fubo sports", " msg ", " masn", " sny", " yes network",
		" root sports", " sportsnet one", " sportsnet 360", " sportsnet world", " sportsnet east", " sportsnet west",
		" sportsnet ontario", " sportsnet pacific",
	} {
		if strings.Contains(s, term) {
			score += 40
		}
	}
	for _, blocked := range []string{
		" arab", " bein sports mena", " bein sports fr", " bein sport fr", " supersport", " sky sport it",
		" sky sports uk", " eurosport", " astro", " viaplay", " premier sports", " tigo sports", " fox deportes",
		" tudn", " univision deportes",
	} {
		if strings.Contains(s, blocked) {
			return 0
		}
	}
	return score
}

func lineupRecipeNorthAmericanSportsCarriageScore(ch catalog.LiveChannel) int {
	switch lineupRecipeNorthAmericanSportsCarriageNetwork(ch) {
	case "abc", "nbc":
		return 95
	default:
		return 0
	}
}

func lineupRecipeNorthAmericanSportsCarriageNetwork(ch catalog.LiveChannel) string {
	if !ch.EPGLinked {
		return ""
	}
	s := lineupRecipeSearchText(ch)
	for _, blocked := range []string{
		" abc news", " nbc news", " msnbc", " cnbc",
	} {
		if strings.Contains(s, blocked) {
			return ""
		}
	}
	naSource := strings.Contains(s, " us|") || strings.Contains(s, " ca|") ||
		strings.HasSuffix(strings.ToLower(strings.TrimSpace(ch.TVGID)), ".us") ||
		strings.HasSuffix(strings.ToLower(strings.TrimSpace(ch.TVGID)), ".ca")
	if !naSource {
		return ""
	}
	switch {
	case strings.Contains(s, " abc "):
		return "abc"
	case strings.Contains(s, " nbc ") && !strings.Contains(s, " nbc sports"):
		return "nbc"
	default:
		return ""
	}
}

func lineupRecipeNorthAmericanSportsEventScore(ch catalog.LiveChannel) int {
	s := lineupRecipeSearchText(ch)
	if strings.Contains(s, " no event streaming ") || strings.Contains(s, "#####") || strings.Contains(s, " ended |") || strings.Contains(s, " nfhs ") {
		return 0
	}
	naSource := false
	for _, term := range []string{
		" us|", " us (", " us:", " ca|", " ca (", " ca:", " tsn+", " peacock ",
	} {
		if strings.Contains(s, term) {
			naSource = true
			break
		}
	}
	if !naSource {
		return 0
	}
	nbaContext := strings.Contains(s, " nba") || strings.Contains(s, " wnba") || strings.Contains(s, " basketball")
	eventStyle := strings.Contains(s, " vs ") || strings.Contains(s, " vs. ") || strings.Contains(s, " @ ") || strings.Contains(s, " game ")
	matchup := false
	if eventStyle {
		for _, term := range []string{
			"raptors", "cavaliers", "nuggets", "timberwolves", "knicks", "hawks", "lakers", "rockets",
			"celtics", "76ers", "sixers", "thunder", "suns", "pistons", "bucks", "pacers", "grizzlies",
			"warriors", "clippers", "mavericks", "spurs", "pelicans", "kings", "trail blazers", "jazz",
			"hornets", "wizards", "nets", "bulls", "heat", "magic",
		} {
			if strings.Contains(s, term) {
				matchup = true
				break
			}
		}
	}
	if !nbaContext && !matchup {
		return 0
	}
	if eventTime, ok := lineupRecipeSportsEventTime(ch); ok {
		now := timeNow().UTC()
		if eventTime.After(now.Add(lineupRecipeSportsEventFutureWindow())) || eventTime.Before(now.Add(-lineupRecipeSportsEventPastWindow())) {
			return 0
		}
	}
	score := 105
	if strings.Contains(s, " nba pass ppv") || strings.Contains(s, " league pass") {
		score += 35
	}
	if strings.Contains(s, " tsn+") || strings.Contains(s, "peacock") {
		score += 25
	}
	if eventStyle {
		score += 20
	}
	if strings.Contains(s, " next |") {
		score += 10
	}
	return score
}

var lineupRecipeSportsEventTimeRE = regexp.MustCompile(`\((\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\)`)

func lineupRecipeSportsEventTime(ch catalog.LiveChannel) (time.Time, bool) {
	text := strings.TrimSpace(ch.GuideName)
	m := lineupRecipeSportsEventTimeRE.FindStringSubmatch(text)
	if len(m) != 2 {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

func lineupRecipeSportsEventFutureWindow() time.Duration {
	return lineupRecipeSportsEventWindowFromEnv("IPTV_TUNERR_SPORTS_EVENT_FUTURE_WINDOW", 4*time.Hour)
}

func lineupRecipeSportsEventPastWindow() time.Duration {
	return lineupRecipeSportsEventWindowFromEnv("IPTV_TUNERR_SPORTS_EVENT_PAST_WINDOW", 150*time.Minute)
}

func lineupRecipeSportsEventWindowFromEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	if d, err := time.ParseDuration(raw); err == nil && d >= 0 {
		return d
	}
	if hours, err := strconv.ParseFloat(raw, 64); err == nil && hours >= 0 {
		return time.Duration(hours * float64(time.Hour))
	}
	return 4 * time.Hour
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
	gateway.sharedAccountLeases = newProviderSharedLeaseManager(
		configuredProviderAccountSharedLeaseDir(),
		strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_SHARED_LEASE_OWNER")),
		configuredProviderAccountSharedLeaseTTL(),
	)
	if gateway.sharedAccountLeases != nil {
		log.Printf("Gateway shared provider-account leases enabled: dir=%q ttl=%s owner=%q",
			gateway.sharedAccountLeases.dir,
			gateway.sharedAccountLeases.ttl,
			gateway.sharedAccountLeases.owner,
		)
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
		EpgForceLineupMatch:        s.EpgForceLineupMatch,
		SourceURL:                  s.XMLTVSourceURL,
		SourceTimeout:              s.XMLTVTimeout,
		CacheTTL:                   cacheTTL,
		PlexSafeIDs:                s.XMLTVPlexSafeIDs,
		ProviderBaseURL:            s.ProviderBaseURL,
		ProviderUser:               s.ProviderUser,
		ProviderPass:               s.ProviderPass,
		ProviderIdentities:         append([]ProviderIdentity(nil), s.ProviderIdentities...),
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
	xmltv.OnGuideHealthReady = s.reapplyDeferredGuidePolicyAfterGuideHealthReady
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
		mux.Handle("/get.php", s.serveXtreamM3U())
		mux.Handle("/xmltv.php", s.serveXtreamXMLTV())
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
	mux.Handle("/programming/harvest-request.json", s.serveProgrammingHarvestRequest())
	mux.Handle("/programming/harvest-import.json", s.serveProgrammingHarvestImport())
	mux.Handle("/programming/harvest-assist.json", s.serveProgrammingHarvestAssist())
	mux.Handle("/programming/recipe.json", s.serveProgrammingRecipe())
	mux.Handle("/programming/preview.json", s.serveProgrammingPreview())
	mux.Handle("/virtual-channels/rules.json", s.serveVirtualChannelRules())
	mux.Handle("/virtual-channels/preview.json", s.serveVirtualChannelPreview())
	mux.Handle("/virtual-channels/schedule.json", s.serveVirtualChannelSchedule())
	mux.Handle("/virtual-channels/channel-detail.json", s.serveVirtualChannelDetail())
	mux.Handle("/virtual-channels/report.json", s.serveVirtualChannelReport())
	mux.Handle("/virtual-channels/recovery-report.json", s.serveVirtualChannelRecoveryReport())
	mux.Handle("/virtual-channels/guide.xml", s.serveVirtualChannelGuide())
	mux.Handle("/virtual-channels/live.m3u", s.serveVirtualChannelM3U())
	mux.Handle("/virtual-channels/slate/", s.serveVirtualChannelSlate())
	mux.Handle("/virtual-channels/branded-stream/", s.serveVirtualChannelBrandedStream())
	mux.Handle("/virtual-channels/stream/", s.serveVirtualChannelStream())
	mux.Handle("/guide/highlights.json", s.serveGuideHighlights())
	mux.Handle("/guide/epg-store.json", s.serveEpgStoreReport())
	mux.Handle("/guide/capsules.json", s.serveCatchupCapsules())
	mux.Handle("/live.m3u", m3uServe)
	mux.Handle("/stream/", gateway)
	// Plex can tune activated HDHR channels through /auto/v<guide-number>, not only /stream/<channel-id>.
	mux.Handle("/auto/", gateway)
	mux.Handle("/healthz", s.serveHealth())
	mux.Handle("/readyz", s.serveReady())
	mux.Handle("/ui/guide-preview.json", s.serveOperatorGuidePreviewJSON())
	mux.HandleFunc("/ui/guide", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/guide/", http.StatusTemporaryRedirect)
	})
	mux.Handle("/ui/guide/", s.serveOperatorGuidePreviewPage())
	mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
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
	mux.Handle("/ops/workflows/programming-harvest.json", s.serveProgrammingHarvestWorkflow())
	mux.Handle("/ops/workflows/ops-recovery.json", s.serveOpsRecoveryWorkflow())
	mux.Handle("/ops/actions/guide-refresh", s.serveGuideRefreshAction())
	mux.Handle("/ops/actions/stream-attempts-clear", s.serveStreamAttemptsClearAction())
	mux.Handle("/ops/actions/stream-stop", s.serveStreamStopAction())
	mux.Handle("/ops/actions/provider-profile-reset", s.serveProviderProfileResetAction())
	mux.Handle("/ops/actions/shared-relay-replay", s.serveSharedRelayReplayUpdateAction())
	mux.Handle("/ops/actions/virtual-channel-live-stall", s.serveVirtualChannelLiveStallUpdateAction())
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

func (s *Server) serveGuideLineupMatch() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.xmltv == nil {
			writeServerJSONError(w, http.StatusServiceUnavailable, "guide unavailable")
			return
		}
		rep, err := s.xmltv.GuideLineupMatchReport(streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 25))
		if err != nil {
			writeServerJSONError(w, http.StatusBadGateway, "guide lineup match unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode guide lineup match")
			return
		}
		_, _ = w.Write(body)
	})
}

type virtualChannelDetailReport struct {
	GeneratedAt        string                        `json:"generated_at"`
	Channel            virtualchannels.Channel       `json:"channel"`
	PublishedStreamURL string                        `json:"published_stream_url,omitempty"`
	SlateURL           string                        `json:"slate_url,omitempty"`
	BrandedStreamURL   string                        `json:"branded_stream_url,omitempty"`
	ResolvedNow        *virtualchannels.ResolvedSlot `json:"resolved_now,omitempty"`
	RecentRecovery     []virtualChannelRecoveryEvent `json:"recent_recovery,omitempty"`
	Upcoming           []virtualchannels.PreviewSlot `json:"upcoming,omitempty"`
	Schedule           []virtualchannels.PreviewSlot `json:"schedule,omitempty"`
}

type virtualChannelStationReportRow struct {
	ChannelID          string                        `json:"channel_id"`
	Name               string                        `json:"name"`
	GuideNumber        string                        `json:"guide_number,omitempty"`
	Enabled            bool                          `json:"enabled"`
	StreamMode         string                        `json:"stream_mode,omitempty"`
	LogoURL            string                        `json:"logo_url,omitempty"`
	BugText            string                        `json:"bug_text,omitempty"`
	BugImageURL        string                        `json:"bug_image_url,omitempty"`
	BugPosition        string                        `json:"bug_position,omitempty"`
	BannerText         string                        `json:"banner_text,omitempty"`
	ThemeColor         string                        `json:"theme_color,omitempty"`
	RecoveryMode       string                        `json:"recovery_mode,omitempty"`
	BlackScreenSeconds int                           `json:"black_screen_seconds,omitempty"`
	FallbackEntries    int                           `json:"fallback_entries,omitempty"`
	RecoveryEvents     int                           `json:"recovery_events,omitempty"`
	RecoveryExhausted  bool                          `json:"recovery_exhausted,omitempty"`
	LastRecoveryReason string                        `json:"last_recovery_reason,omitempty"`
	PublishedStreamURL string                        `json:"published_stream_url,omitempty"`
	SlateURL           string                        `json:"slate_url,omitempty"`
	BrandedStreamURL   string                        `json:"branded_stream_url,omitempty"`
	ResolvedNow        *virtualchannels.ResolvedSlot `json:"resolved_now,omitempty"`
	RecentRecovery     []virtualChannelRecoveryEvent `json:"recent_recovery,omitempty"`
}

type virtualChannelStationReport struct {
	GeneratedAt string                           `json:"generated_at"`
	Count       int                              `json:"count"`
	Channels    []virtualChannelStationReportRow `json:"channels,omitempty"`
}

type virtualChannelRecoveryEvent struct {
	DetectedAtUTC   string `json:"detected_at_utc"`
	ChannelID       string `json:"channel_id,omitempty"`
	ChannelName     string `json:"channel_name,omitempty"`
	EntryID         string `json:"entry_id,omitempty"`
	SourceURL       string `json:"source_url,omitempty"`
	FallbackEntryID string `json:"fallback_entry_id,omitempty"`
	FallbackURL     string `json:"fallback_url,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Surface         string `json:"surface,omitempty"`
}

type virtualChannelRecoveryReport struct {
	GeneratedAt string                        `json:"generated_at"`
	ChannelID   string                        `json:"channel_id,omitempty"`
	Events      []virtualChannelRecoveryEvent `json:"events,omitempty"`
}

type virtualChannelRecoverySummary struct {
	EventCount        int
	RecoveryExhausted bool
	LastReason        string
}

type virtualChannelFallbackTarget struct {
	URL     string
	EntryID string
}

type virtualChannelChannelMutationRequest struct {
	Action        string                         `json:"action"`
	ChannelID     string                         `json:"channel_id,omitempty"`
	Name          string                         `json:"name,omitempty"`
	GuideNumber   string                         `json:"guide_number,omitempty"`
	GroupTitle    string                         `json:"group_title,omitempty"`
	Description   string                         `json:"description,omitempty"`
	Enabled       *bool                          `json:"enabled,omitempty"`
	Branding      virtualchannels.Branding       `json:"branding,omitempty"`
	BrandingClear []string                       `json:"branding_clear,omitempty"`
	Recovery      virtualchannels.RecoveryPolicy `json:"recovery,omitempty"`
	RecoveryClear []string                       `json:"recovery_clear,omitempty"`
}

type virtualChannelScheduleMutationRequest struct {
	Action         string                  `json:"action"`
	ChannelID      string                  `json:"channel_id,omitempty"`
	Mode           string                  `json:"mode,omitempty"` // append | replace
	Entry          *virtualchannels.Entry  `json:"entry,omitempty"`
	Entries        []virtualchannels.Entry `json:"entries,omitempty"`
	Slot           *virtualchannels.Slot   `json:"slot,omitempty"`
	Slots          []virtualchannels.Slot  `json:"slots,omitempty"`
	MovieIDs       []string                `json:"movie_ids,omitempty"`
	SeriesID       string                  `json:"series_id,omitempty"`
	EpisodeIDs     []string                `json:"episode_ids,omitempty"`
	DurationMins   int                     `json:"duration_mins,omitempty"`
	RemoveEntryIDs []string                `json:"remove_entry_ids,omitempty"`
	RemoveSlots    []string                `json:"remove_slots,omitempty"`
	DaypartStart   string                  `json:"daypart_start_hhmm,omitempty"`
	DaypartEnd     string                  `json:"daypart_end_hhmm,omitempty"`
	LabelPrefix    string                  `json:"label_prefix,omitempty"`
	Category       string                  `json:"category,omitempty"`
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

func (s *Server) serveXtreamEntitlements() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			set := s.reloadXtreamEntitlements()
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at": time.Now().UTC().Format(time.RFC3339),
				"users_file":   strings.TrimSpace(s.XtreamUsersFile),
				"enabled":      strings.TrimSpace(s.XtreamUsersFile) != "",
				"rules":        set,
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode xtream entitlements")
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.XtreamUsersFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "xtream users file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var set entitlements.Ruleset
			if err := json.NewDecoder(limited).Decode(&set); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid xtream entitlements json")
				return
			}
			saved, err := s.saveXtreamEntitlements(set)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save xtream entitlements failed")
				return
			}
			body, err := json.MarshalIndent(saved, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode xtream entitlements")
				return
			}
			_, _ = w.Write(body)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func timeMustParseRFC3339(raw string) time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return parsed.UTC()
}

func resolveVirtualChannelEntryURL(entry virtualchannels.Entry, movies []catalog.Movie, series []catalog.Series) string {
	if strings.EqualFold(strings.TrimSpace(entry.Type), "movie") {
		for _, movie := range movies {
			if strings.TrimSpace(movie.ID) == strings.TrimSpace(entry.MovieID) {
				return strings.TrimSpace(movie.StreamURL)
			}
		}
		return ""
	}
	for _, show := range series {
		if strings.TrimSpace(show.ID) != strings.TrimSpace(entry.SeriesID) {
			continue
		}
		for _, season := range show.Seasons {
			for _, episode := range season.Episodes {
				if strings.TrimSpace(episode.ID) == strings.TrimSpace(entry.EpisodeID) {
					return strings.TrimSpace(episode.StreamURL)
				}
			}
		}
	}
	return ""
}

func virtualChannelByID(set virtualchannels.Ruleset, channelID string) (virtualchannels.Channel, bool) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return virtualchannels.Channel{}, false
	}
	for _, ch := range set.Channels {
		if strings.TrimSpace(ch.ID) == channelID {
			return ch, true
		}
	}
	return virtualchannels.Channel{}, false
}

func virtualChannelIndex(set virtualchannels.Ruleset, channelID string) int {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return -1
	}
	for i, ch := range set.Channels {
		if strings.TrimSpace(ch.ID) == channelID {
			return i
		}
	}
	return -1
}

func appendEntriesByMode(existing, incoming []virtualchannels.Entry, mode string) []virtualchannels.Entry {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "replace" {
		return append([]virtualchannels.Entry(nil), incoming...)
	}
	out := append([]virtualchannels.Entry(nil), existing...)
	out = append(out, incoming...)
	return out
}

func removeVirtualChannelEntries(entries []virtualchannels.Entry, removeIDs []string) []virtualchannels.Entry {
	if len(entries) == 0 || len(removeIDs) == 0 {
		return append([]virtualchannels.Entry(nil), entries...)
	}
	remove := make(map[string]struct{}, len(removeIDs))
	for _, id := range removeIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			remove[id] = struct{}{}
		}
	}
	out := make([]virtualchannels.Entry, 0, len(entries))
	for _, entry := range entries {
		if _, ok := remove[virtualChannelEntryIdentifier(entry)]; ok {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func virtualChannelEntryIdentifier(entry virtualchannels.Entry) string {
	if strings.EqualFold(strings.TrimSpace(entry.Type), "movie") {
		return strings.TrimSpace(entry.MovieID)
	}
	return strings.TrimSpace(entry.SeriesID) + ":" + strings.TrimSpace(entry.EpisodeID)
}

func appendSlotsByMode(existing, incoming []virtualchannels.Slot, mode string) []virtualchannels.Slot {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "replace" {
		return append([]virtualchannels.Slot(nil), incoming...)
	}
	out := append([]virtualchannels.Slot(nil), existing...)
	out = append(out, incoming...)
	return out
}

func removeVirtualChannelSlots(slots []virtualchannels.Slot, removeStarts []string) []virtualchannels.Slot {
	if len(slots) == 0 || len(removeStarts) == 0 {
		return append([]virtualchannels.Slot(nil), slots...)
	}
	remove := make(map[string]struct{}, len(removeStarts))
	for _, start := range removeStarts {
		start = strings.TrimSpace(start)
		if start != "" {
			remove[start] = struct{}{}
		}
	}
	out := make([]virtualchannels.Slot, 0, len(slots))
	for _, slot := range slots {
		if _, ok := remove[strings.TrimSpace(slot.StartHHMM)]; ok {
			continue
		}
		out = append(out, slot)
	}
	return out
}

func (s *Server) buildEntriesForScheduleMutation(req virtualChannelScheduleMutationRequest) ([]virtualchannels.Entry, error) {
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "append_movies", "fill_daypart":
		if len(req.MovieIDs) > 0 {
			entries := make([]virtualchannels.Entry, 0, len(req.MovieIDs))
			for _, movieID := range req.MovieIDs {
				movieID = strings.TrimSpace(movieID)
				if movieID == "" {
					continue
				}
				entries = append(entries, virtualchannels.Entry{Type: "movie", MovieID: movieID, DurationMins: req.DurationMins})
			}
			if len(entries) > 0 {
				return entries, nil
			}
		}
		if req.Entry != nil {
			return []virtualchannels.Entry{*req.Entry}, nil
		}
		if len(req.Entries) > 0 {
			return append([]virtualchannels.Entry(nil), req.Entries...), nil
		}
		if len(req.EpisodeIDs) > 0 {
			seriesID := strings.TrimSpace(req.SeriesID)
			if seriesID == "" {
				return nil, fmt.Errorf("series_id required")
			}
			entries := make([]virtualchannels.Entry, 0, len(req.EpisodeIDs))
			for _, episodeID := range req.EpisodeIDs {
				episodeID = strings.TrimSpace(episodeID)
				if episodeID == "" {
					continue
				}
				entries = append(entries, virtualchannels.Entry{Type: "episode", SeriesID: seriesID, EpisodeID: episodeID, DurationMins: req.DurationMins})
			}
			if len(entries) > 0 {
				return entries, nil
			}
		}
		return nil, fmt.Errorf("schedule entries required")
	case "fill_movie_category":
		category := strings.ToLower(strings.TrimSpace(req.Category))
		if category == "" {
			return nil, fmt.Errorf("category required")
		}
		entries := make([]virtualchannels.Entry, 0, len(s.Movies))
		for _, movie := range s.Movies {
			if strings.ToLower(strings.TrimSpace(movie.Category)) != category {
				continue
			}
			entries = append(entries, virtualchannels.Entry{Type: "movie", MovieID: strings.TrimSpace(movie.ID), DurationMins: req.DurationMins})
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("no movies found for category")
		}
		return entries, nil
	case "fill_series":
		seriesID := strings.TrimSpace(req.SeriesID)
		if seriesID == "" {
			return nil, fmt.Errorf("series_id required")
		}
		for _, show := range s.Series {
			if strings.TrimSpace(show.ID) != seriesID {
				continue
			}
			entries := make([]virtualchannels.Entry, 0)
			if len(req.EpisodeIDs) > 0 {
				allowed := make(map[string]struct{}, len(req.EpisodeIDs))
				for _, episodeID := range req.EpisodeIDs {
					episodeID = strings.TrimSpace(episodeID)
					if episodeID != "" {
						allowed[episodeID] = struct{}{}
					}
				}
				for _, season := range show.Seasons {
					for _, episode := range season.Episodes {
						if _, ok := allowed[strings.TrimSpace(episode.ID)]; !ok {
							continue
						}
						entries = append(entries, virtualchannels.Entry{Type: "episode", SeriesID: seriesID, EpisodeID: strings.TrimSpace(episode.ID), DurationMins: req.DurationMins})
					}
				}
			} else {
				for _, season := range show.Seasons {
					for _, episode := range season.Episodes {
						entries = append(entries, virtualchannels.Entry{Type: "episode", SeriesID: seriesID, EpisodeID: strings.TrimSpace(episode.ID), DurationMins: req.DurationMins})
					}
				}
			}
			if len(entries) == 0 {
				return nil, fmt.Errorf("no episodes found for series")
			}
			return entries, nil
		}
		return nil, fmt.Errorf("series not found")
	case "append_episodes":
		seriesID := strings.TrimSpace(req.SeriesID)
		if seriesID == "" {
			return nil, fmt.Errorf("series_id required")
		}
		entries := make([]virtualchannels.Entry, 0, len(req.EpisodeIDs))
		for _, episodeID := range req.EpisodeIDs {
			episodeID = strings.TrimSpace(episodeID)
			if episodeID == "" {
				continue
			}
			entries = append(entries, virtualchannels.Entry{Type: "episode", SeriesID: seriesID, EpisodeID: episodeID, DurationMins: req.DurationMins})
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("episode_ids required")
		}
		return entries, nil
	}
	return nil, fmt.Errorf("schedule entries required")
}

func buildDaypartSlots(startHHMM, endHHMM, labelPrefix string, entries []virtualchannels.Entry) ([]virtualchannels.Slot, error) {
	startHHMM = strings.TrimSpace(startHHMM)
	endHHMM = strings.TrimSpace(endHHMM)
	if startHHMM == "" || endHHMM == "" {
		return nil, fmt.Errorf("daypart_start_hhmm and daypart_end_hhmm required")
	}
	startMins, err := hhmmToMinutes(startHHMM)
	if err != nil {
		return nil, fmt.Errorf("invalid daypart_start_hhmm")
	}
	endMins, err := hhmmToMinutes(endHHMM)
	if err != nil {
		return nil, fmt.Errorf("invalid daypart_end_hhmm")
	}
	if endMins <= startMins {
		return nil, fmt.Errorf("daypart_end_hhmm must be after daypart_start_hhmm")
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("schedule entries required")
	}
	labelPrefix = strings.TrimSpace(labelPrefix)
	slots := make([]virtualchannels.Slot, 0, len(entries))
	cursor := startMins
	idx := 0
	for cursor < endMins {
		entry := entries[idx%len(entries)]
		duration := entry.DurationMins
		if duration <= 0 {
			duration = 30
		}
		if cursor+duration > endMins {
			break
		}
		slot := virtualchannels.Slot{
			StartHHMM:    minutesToHHMM(cursor),
			DurationMins: duration,
			Entry:        entry,
		}
		if labelPrefix != "" {
			slot.Label = strings.TrimSpace(labelPrefix + " " + strconv.Itoa(len(slots)+1))
		}
		slots = append(slots, slot)
		cursor += duration
		idx++
	}
	if len(slots) == 0 {
		return nil, fmt.Errorf("daypart window too small for requested entries")
	}
	return slots, nil
}

func mergeDaypartSlots(existing, replacement []virtualchannels.Slot, startHHMM, endHHMM string) []virtualchannels.Slot {
	startMins, errStart := hhmmToMinutes(startHHMM)
	endMins, errEnd := hhmmToMinutes(endHHMM)
	if errStart != nil || errEnd != nil {
		return append([]virtualchannels.Slot(nil), replacement...)
	}
	out := make([]virtualchannels.Slot, 0, len(existing)+len(replacement))
	for _, slot := range existing {
		slotStart, err := hhmmToMinutes(slot.StartHHMM)
		if err != nil {
			out = append(out, slot)
			continue
		}
		if slotStart >= startMins && slotStart < endMins {
			continue
		}
		out = append(out, slot)
	}
	out = append(out, replacement...)
	return out
}

func hhmmToMinutes(raw string) (int, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func minutesToHHMM(total int) string {
	if total < 0 {
		total = 0
	}
	hours := (total / 60) % 24
	mins := total % 60
	return fmt.Sprintf("%02d:%02d", hours, mins)
}

func hasVirtualChannelRecoveryFields(policy virtualchannels.RecoveryPolicy) bool {
	return strings.TrimSpace(policy.Mode) != "" || policy.BlackScreenSeconds != 0 || len(policy.FallbackEntries) > 0
}

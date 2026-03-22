package livetvbundle

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/emby"
	"github.com/snapetech/iptvtunerr/internal/plex"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

const libraryTitleSampleLimit = 50

type Bundle struct {
	GeneratedAt string            `json:"generated_at"`
	Source      string            `json:"source"`
	Tuner       Tuner             `json:"tuner"`
	Guide       Guide             `json:"guide"`
	Lineup      Lineup            `json:"lineup"`
	Libraries   []Library         `json:"libraries,omitempty"`
	Catchup     []Library         `json:"catchup,omitempty"`
	Plex        *PlexBundleSource `json:"plex,omitempty"`
	Notes       []string          `json:"notes,omitempty"`
}

type Tuner struct {
	FriendlyName string `json:"friendly_name,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
	TunerURL     string `json:"tuner_url"`
	TunerCount   int    `json:"tuner_count,omitempty"`
}

type Guide struct {
	XMLTVURL string `json:"xmltv_url"`
}

type Lineup struct {
	Title string `json:"title,omitempty"`
}

type Library struct {
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Locations       []string `json:"locations,omitempty"`
	PlexKey         string   `json:"plex_key,omitempty"`
	SourceItemCount int      `json:"source_item_count,omitempty"`
	SourceTitles    []string `json:"source_titles,omitempty"`
}

type PlexBundleSource struct {
	BaseURL    string `json:"base_url,omitempty"`
	DVRKey     int    `json:"dvr_key,omitempty"`
	DVRUUID    string `json:"dvr_uuid,omitempty"`
	DeviceKey  string `json:"device_key,omitempty"`
	DeviceUUID string `json:"device_uuid,omitempty"`
	DeviceURI  string `json:"device_uri,omitempty"`
	LineupURL  string `json:"lineup_url,omitempty"`
}

type BuildFromPlexOptions struct {
	DVRKeyOverride   int
	TunerURLOverride string
	TunerCount       int
	IncludeLibraries bool
}

type EmbyPlan struct {
	GeneratedAt       string                    `json:"generated_at"`
	Target            string                    `json:"target"`
	BundleSource      string                    `json:"bundle_source"`
	TargetHost        string                    `json:"target_host,omitempty"`
	RecommendedConfig emby.Config               `json:"recommended_config"`
	TunerHost         emby.TunerHostInfo        `json:"tuner_host"`
	ListingProvider   emby.ListingsProviderInfo `json:"listing_provider"`
	Notes             []string                  `json:"notes,omitempty"`
}

type ApplyResult struct {
	AppliedAt         string      `json:"applied_at"`
	Target            string      `json:"target"`
	TargetHost        string      `json:"target_host"`
	StateFile         string      `json:"state_file,omitempty"`
	RecommendedConfig emby.Config `json:"recommended_config"`
	Notes             []string    `json:"notes,omitempty"`
}

type LiveTVDiffEntry struct {
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	ExistingID string `json:"existing_id,omitempty"`
}

type LiveTVDiffResult struct {
	ComparedAt          string            `json:"compared_at"`
	Target              string            `json:"target"`
	TargetHost          string            `json:"target_host"`
	CreateCount         int               `json:"create_count"`
	ReuseCount          int               `json:"reuse_count"`
	ConflictCount       int               `json:"conflict_count"`
	IndexedChannelCount int               `json:"indexed_channel_count"`
	Entries             []LiveTVDiffEntry `json:"entries"`
	Notes               []string          `json:"notes,omitempty"`
}

type RolloutPlan struct {
	GeneratedAt  string     `json:"generated_at"`
	BundleSource string     `json:"bundle_source"`
	Plans        []EmbyPlan `json:"plans"`
	Notes        []string   `json:"notes,omitempty"`
}

type TargetSpec struct {
	Target string
	Host   string
}

type ApplySpec struct {
	Host      string
	Token     string
	StateFile string
}

type RolloutApplyResult struct {
	AppliedAt string        `json:"applied_at"`
	Results   []ApplyResult `json:"results"`
	Notes     []string      `json:"notes,omitempty"`
}

type LiveTVRolloutDiffResult struct {
	ComparedAt string             `json:"compared_at"`
	Results    []LiveTVDiffResult `json:"results"`
	Notes      []string           `json:"notes,omitempty"`
}

type LibraryPlan struct {
	GeneratedAt  string                   `json:"generated_at"`
	Target       string                   `json:"target"`
	BundleSource string                   `json:"bundle_source"`
	TargetHost   string                   `json:"target_host,omitempty"`
	Libraries    []emby.LibraryCreateSpec `json:"libraries"`
	Notes        []string                 `json:"notes,omitempty"`
}

type LibraryApplyLibrary struct {
	Name           string   `json:"name"`
	CollectionType string   `json:"collection_type"`
	Locations      []string `json:"locations,omitempty"`
	ID             string   `json:"id,omitempty"`
	Created        bool     `json:"created"`
}

type LibraryApplyResult struct {
	AppliedAt  string                `json:"applied_at"`
	Target     string                `json:"target"`
	TargetHost string                `json:"target_host"`
	Refresh    bool                  `json:"refresh"`
	Libraries  []LibraryApplyLibrary `json:"libraries"`
	Notes      []string              `json:"notes,omitempty"`
}

type LibraryDiffLibrary struct {
	Name              string   `json:"name"`
	CollectionType    string   `json:"collection_type"`
	DesiredPath       string   `json:"desired_path"`
	SourceItemCount   int      `json:"source_item_count,omitempty"`
	SourceTitles      []string `json:"source_titles,omitempty"`
	Status            string   `json:"status"`
	Reason            string   `json:"reason,omitempty"`
	ExistingID        string   `json:"existing_id,omitempty"`
	ExistingItemCount int      `json:"existing_item_count,omitempty"`
	ExistingTitles    []string `json:"existing_titles,omitempty"`
	ExistingLocations []string `json:"existing_locations,omitempty"`
	ParityStatus      string   `json:"parity_status,omitempty"`
	TitleParityStatus string   `json:"title_parity_status,omitempty"`
	MissingTitles     []string `json:"missing_titles,omitempty"`
}

type LibraryDiffResult struct {
	ComparedAt    string               `json:"compared_at"`
	Target        string               `json:"target"`
	TargetHost    string               `json:"target_host"`
	DesiredCount  int                  `json:"desired_count"`
	PresentCount  int                  `json:"present_count"`
	CreateCount   int                  `json:"create_count"`
	ReuseCount    int                  `json:"reuse_count"`
	ConflictCount int                  `json:"conflict_count"`
	Libraries     []LibraryDiffLibrary `json:"libraries"`
	Notes         []string             `json:"notes,omitempty"`
}

type LibraryRolloutPlan struct {
	GeneratedAt  string        `json:"generated_at"`
	BundleSource string        `json:"bundle_source"`
	Plans        []LibraryPlan `json:"plans"`
	Notes        []string      `json:"notes,omitempty"`
}

type LibraryRolloutApplyResult struct {
	AppliedAt string               `json:"applied_at"`
	Results   []LibraryApplyResult `json:"results"`
	Notes     []string             `json:"notes,omitempty"`
}

type LibraryRolloutDiffResult struct {
	ComparedAt string              `json:"compared_at"`
	Results    []LibraryDiffResult `json:"results"`
	Notes      []string            `json:"notes,omitempty"`
}

type MigrationTargetAudit struct {
	Target             string                  `json:"target"`
	TargetHost         string                  `json:"target_host"`
	Status             string                  `json:"status"`
	StatusReason       string                  `json:"status_reason,omitempty"`
	ReadyToApply       bool                    `json:"ready_to_apply"`
	LiveTVReady        bool                    `json:"live_tv_ready"`
	LibraryReady       bool                    `json:"library_ready"`
	LiveTV             LiveTVDiffResult        `json:"live_tv"`
	Library            *LibraryDiffResult      `json:"library,omitempty"`
	LibraryMode        string                  `json:"library_mode,omitempty"`
	LibraryScan        *emby.LibraryScanStatus `json:"library_scan,omitempty"`
	SyncedLibraries    []string                `json:"synced_libraries,omitempty"`
	LaggingLibraries   []string                `json:"lagging_libraries,omitempty"`
	TitleSyncedLibraries []string              `json:"title_synced_libraries,omitempty"`
	TitleLaggingLibraries []string             `json:"title_lagging_libraries,omitempty"`
	PresentLibraries   []string                `json:"present_libraries,omitempty"`
	MissingLibraries   []string                `json:"missing_libraries,omitempty"`
	PopulatedLibraries []string                `json:"populated_libraries,omitempty"`
	EmptyLibraries     []string                `json:"empty_libraries,omitempty"`
	ConflictCount      int                     `json:"conflict_count"`
}

type MigrationAuditResult struct {
	ComparedAt       string                 `json:"compared_at"`
	Source           string                 `json:"source,omitempty"`
	Status           string                 `json:"status"`
	ReadyToApply     bool                   `json:"ready_to_apply"`
	TargetCount      int                    `json:"target_count"`
	ReadyTargetCount int                    `json:"ready_target_count"`
	ConflictCount    int                    `json:"conflict_count"`
	Results          []MigrationTargetAudit `json:"results"`
	Notes            []string               `json:"notes,omitempty"`
}

func BuildFromPlexAPI(plexBaseURL, plexToken string, opts BuildFromPlexOptions) (*Bundle, error) {
	plexBaseURL = strings.TrimSpace(plexBaseURL)
	plexToken = strings.TrimSpace(plexToken)
	if plexBaseURL == "" {
		return nil, fmt.Errorf("plex base url required")
	}
	if plexToken == "" {
		return nil, fmt.Errorf("plex token required")
	}
	host, err := hostPortFromBaseURL(plexBaseURL)
	if err != nil {
		return nil, err
	}
	dvrs, err := plex.ListDVRsAPI(host, plexToken)
	if err != nil {
		return nil, fmt.Errorf("list plex dvrs: %w", err)
	}
	if len(dvrs) == 0 {
		return nil, fmt.Errorf("no plex dvrs found")
	}
	selected, err := chooseDVR(dvrs, opts.DVRKeyOverride)
	if err != nil {
		return nil, err
	}
	devices, err := plex.ListDevicesAPI(host, plexToken)
	if err != nil {
		return nil, fmt.Errorf("list plex devices: %w", err)
	}
	device, err := matchPlexDevice(devices, selected)
	if err != nil {
		return nil, err
	}
	tunerURL := strings.TrimSpace(opts.TunerURLOverride)
	if tunerURL == "" {
		tunerURL = strings.TrimSpace(device.URI)
	}
	if tunerURL == "" {
		return nil, fmt.Errorf("selected plex dvr has no tuner uri; set an override")
	}
	xmltvURL, err := parsePlexLineupXMLTVURL(selected.LineupURL)
	if err != nil {
		return nil, err
	}
	tunerCount := opts.TunerCount
	notes := []string{
		"Built from Plex DVR/device state; tuner count is inferred only if you supply it.",
	}
	if tunerCount <= 0 {
		tunerCount = 2
		notes = append(notes, "Tuner count defaulted to 2 because Plex does not expose a stable tuner-count field in DVR/device APIs.")
	}
	var libraries []Library
	if opts.IncludeLibraries {
		sections, err := plex.ListLibrarySections(plexBaseURL, plexToken)
		if err != nil {
			return nil, fmt.Errorf("list plex libraries: %w", err)
		}
		libraries = make([]Library, 0, len(sections))
		for _, section := range sections {
			entry := Library{
				Name:    strings.TrimSpace(section.Title),
				Type:    strings.TrimSpace(section.Type),
				PlexKey: strings.TrimSpace(section.Key),
			}
			if entry.PlexKey != "" {
				count, err := plex.GetLibrarySectionItemCount(plexBaseURL, plexToken, entry.PlexKey)
				if err == nil {
					entry.SourceItemCount = count
				} else {
					notes = append(notes, fmt.Sprintf("Skipped source item count for Plex library %q: %v", entry.Name, err))
				}
				titles, err := plex.GetLibrarySectionItemTitles(plexBaseURL, plexToken, entry.PlexKey, libraryTitleSampleLimit)
				if err == nil {
					entry.SourceTitles = titles
				} else {
					notes = append(notes, fmt.Sprintf("Skipped source title sample for Plex library %q: %v", entry.Name, err))
				}
			}
			for _, location := range section.Locations {
				if trimmed := strings.TrimSpace(location); trimmed != "" {
					entry.Locations = append(entry.Locations, trimmed)
				}
			}
			libraries = append(libraries, entry)
		}
		if len(libraries) > 0 {
			notes = append(notes, "Included Plex library sections so shared storage paths can be recreated on Emby/Jellyfin.")
		}
	}
	return &Bundle{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      "plex_api",
		Tuner: Tuner{
			FriendlyName: strings.TrimSpace(device.Name),
			DeviceID:     strings.TrimSpace(device.DeviceID),
			TunerURL:     tunerURL,
			TunerCount:   tunerCount,
		},
		Guide: Guide{
			XMLTVURL: xmltvURL,
		},
		Lineup: Lineup{
			Title: strings.TrimSpace(selected.LineupTitle),
		},
		Libraries: libraries,
		Plex: &PlexBundleSource{
			BaseURL:    plexBaseURL,
			DVRKey:     selected.Key,
			DVRUUID:    strings.TrimSpace(selected.UUID),
			DeviceKey:  strings.TrimSpace(selected.DeviceKey),
			DeviceUUID: strings.TrimSpace(device.UUID),
			DeviceURI:  strings.TrimSpace(device.URI),
			LineupURL:  strings.TrimSpace(selected.LineupURL),
		},
		Notes: notes,
	}, nil
}

func BuildEmbyPlan(bundle Bundle, target, host string) (*EmbyPlan, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	if target != "emby" && target != "jellyfin" {
		return nil, fmt.Errorf("target must be emby or jellyfin")
	}
	if strings.TrimSpace(bundle.Tuner.TunerURL) == "" {
		return nil, fmt.Errorf("bundle tuner_url is required")
	}
	if strings.TrimSpace(bundle.Guide.XMLTVURL) == "" {
		return nil, fmt.Errorf("bundle xmltv_url is required")
	}
	friendlyName := strings.TrimSpace(bundle.Tuner.FriendlyName)
	if friendlyName == "" {
		friendlyName = strings.TrimSpace(bundle.Lineup.Title)
	}
	if friendlyName == "" {
		friendlyName = "IPTV Tunerr"
	}
	tunerCount := bundle.Tuner.TunerCount
	if tunerCount <= 0 {
		tunerCount = 2
	}
	cfg := emby.Config{
		Host:         strings.TrimSpace(host),
		TunerURL:     strings.TrimSpace(bundle.Tuner.TunerURL),
		XMLTVURL:     strings.TrimSpace(bundle.Guide.XMLTVURL),
		FriendlyName: friendlyName,
		TunerCount:   tunerCount,
		ServerType:   target,
	}
	return &EmbyPlan{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		Target:            target,
		BundleSource:      strings.TrimSpace(bundle.Source),
		TargetHost:        strings.TrimSpace(host),
		RecommendedConfig: cfg,
		TunerHost: emby.TunerHostInfo{
			Type:                "hdhomerun",
			Url:                 cfg.TunerURL,
			FriendlyName:        cfg.FriendlyName,
			TunerCount:          cfg.TunerCount,
			ImportFavoritesOnly: false,
			AllowHWTranscoding:  false,
			AllowStreamSharing:  true,
			EnableStreamLooping: false,
			IgnoreDts:           false,
		},
		ListingProvider: emby.ListingsProviderInfo{
			Type:            "xmltv",
			Path:            cfg.XMLTVURL,
			EnableAllTuners: true,
		},
		Notes: []string{
			"This is a pre-registration plan, not a direct Emby/Jellyfin DB dump.",
			"Target host/token stay external because they do not exist in Plex DVR state.",
		},
	}, nil
}

func ConfigFromEmbyPlan(plan EmbyPlan, host, token string) (emby.Config, error) {
	target := strings.ToLower(strings.TrimSpace(plan.Target))
	if target != "emby" && target != "jellyfin" {
		return emby.Config{}, fmt.Errorf("target must be emby or jellyfin")
	}
	cfg := plan.RecommendedConfig
	resolvedHost := firstNonEmptyString(host, cfg.Host, plan.TargetHost)
	resolvedHost = strings.TrimSpace(resolvedHost)
	token = strings.TrimSpace(token)
	cfg.Host = resolvedHost
	cfg.Token = token
	cfg.ServerType = target
	cfg.TunerURL = strings.TrimSpace(cfg.TunerURL)
	cfg.XMLTVURL = strings.TrimSpace(cfg.XMLTVURL)
	cfg.FriendlyName = strings.TrimSpace(cfg.FriendlyName)
	if cfg.Host == "" {
		return emby.Config{}, fmt.Errorf("target host required")
	}
	if cfg.Token == "" {
		return emby.Config{}, fmt.Errorf("target token required")
	}
	if cfg.TunerURL == "" {
		return emby.Config{}, fmt.Errorf("plan tuner url required")
	}
	if cfg.XMLTVURL == "" {
		return emby.Config{}, fmt.Errorf("plan xmltv url required")
	}
	if cfg.TunerCount <= 0 {
		cfg.TunerCount = 2
	}
	return cfg, nil
}

func ApplyEmbyPlan(plan EmbyPlan, host, token, stateFile string) (*ApplyResult, error) {
	cfg, err := ConfigFromEmbyPlan(plan, host, token)
	if err != nil {
		return nil, err
	}
	stateFile = strings.TrimSpace(stateFile)
	if err := emby.FullRegister(cfg, stateFile); err != nil {
		return nil, err
	}
	notes := []string{
		"Applied via built-in Emby/Jellyfin Live TV registration APIs.",
	}
	if stateFile != "" {
		notes = append(notes, "Registration ids were persisted for idempotent re-apply/cleanup.")
	}
	return &ApplyResult{
		AppliedAt:         time.Now().UTC().Format(time.RFC3339),
		Target:            cfg.ServerType,
		TargetHost:        cfg.Host,
		StateFile:         stateFile,
		RecommendedConfig: cfg,
		Notes:             notes,
	}, nil
}

func DiffEmbyPlan(plan EmbyPlan, host, token string) (*LiveTVDiffResult, error) {
	cfg, err := ConfigFromEmbyPlan(plan, host, token)
	if err != nil {
		return nil, err
	}
	channelCount := emby.GetChannelCount(cfg)
	tunerHosts, err := emby.ListTunerHosts(cfg)
	if err != nil {
		return nil, fmt.Errorf("list tuner hosts: %w", err)
	}
	listingProviders, err := emby.ListListingProviders(cfg)
	if err != nil {
		return nil, fmt.Errorf("list listing providers: %w", err)
	}
	entries := []LiveTVDiffEntry{
		diffTunerHost(plan.TunerHost, tunerHosts),
		diffListingProvider(plan.ListingProvider, listingProviders),
	}
	var createCount, reuseCount, conflictCount int
	for _, entry := range entries {
		switch entry.Status {
		case "create":
			createCount++
		case "reuse":
			reuseCount++
		default:
			conflictCount++
		}
	}
	notes := []string{
		"Diff compares the planned tuner host and XMLTV listing provider against the current target server state before apply.",
	}
	if conflictCount > 0 {
		notes = append(notes, "Conflicts should be resolved before apply to avoid duplicate or mismatched Live TV registrations.")
	}
	return &LiveTVDiffResult{
		ComparedAt:          time.Now().UTC().Format(time.RFC3339),
		Target:              cfg.ServerType,
		TargetHost:          cfg.Host,
		CreateCount:         createCount,
		ReuseCount:          reuseCount,
		ConflictCount:       conflictCount,
		IndexedChannelCount: channelCount,
		Entries:             entries,
		Notes:               notes,
	}, nil
}

func BuildRolloutPlan(bundle Bundle, specs []TargetSpec) (*RolloutPlan, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("at least one target required")
	}
	plans := make([]EmbyPlan, 0, len(specs))
	seen := map[string]bool{}
	for _, spec := range specs {
		target := strings.ToLower(strings.TrimSpace(spec.Target))
		if target == "" {
			return nil, fmt.Errorf("target must be emby or jellyfin")
		}
		if seen[target] {
			continue
		}
		plan, err := BuildEmbyPlan(bundle, target, spec.Host)
		if err != nil {
			return nil, err
		}
		plans = append(plans, *plan)
		seen[target] = true
	}
	if len(plans) == 0 {
		return nil, fmt.Errorf("no valid rollout targets")
	}
	return &RolloutPlan{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		BundleSource: strings.TrimSpace(bundle.Source),
		Plans:        plans,
		Notes: []string{
			"One neutral Plex-derived bundle can pre-roll multiple media-server targets without forcing a same-day cutover.",
		},
	}, nil
}

func ApplyRolloutPlan(plan RolloutPlan, apply map[string]ApplySpec) (*RolloutApplyResult, error) {
	if len(plan.Plans) == 0 {
		return nil, fmt.Errorf("rollout plan has no targets")
	}
	results := make([]ApplyResult, 0, len(plan.Plans))
	for _, entry := range plan.Plans {
		spec := apply[strings.ToLower(strings.TrimSpace(entry.Target))]
		res, err := ApplyEmbyPlan(entry, spec.Host, spec.Token, spec.StateFile)
		if err != nil {
			return nil, fmt.Errorf("apply %s: %w", entry.Target, err)
		}
		results = append(results, *res)
	}
	return &RolloutApplyResult{
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Results:   results,
		Notes: []string{
			"Rollout apply intentionally leaves Plex untouched so overlap migration stays possible.",
		},
	}, nil
}

func DiffRolloutPlan(plan RolloutPlan, apply map[string]ApplySpec) (*LiveTVRolloutDiffResult, error) {
	if len(plan.Plans) == 0 {
		return nil, fmt.Errorf("rollout plan has no targets")
	}
	results := make([]LiveTVDiffResult, 0, len(plan.Plans))
	for _, entry := range plan.Plans {
		spec := apply[strings.ToLower(strings.TrimSpace(entry.Target))]
		res, err := DiffEmbyPlan(entry, spec.Host, spec.Token)
		if err != nil {
			return nil, fmt.Errorf("diff %s: %w", entry.Target, err)
		}
		results = append(results, *res)
	}
	return &LiveTVRolloutDiffResult{
		ComparedAt: time.Now().UTC().Format(time.RFC3339),
		Results:    results,
		Notes: []string{
			"Rollout diff compares the same neutral Live TV bundle against multiple non-Plex targets without mutating them.",
		},
	}, nil
}

func BuildLibraryPlan(bundle Bundle, target, host string) (*LibraryPlan, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	if target != "emby" && target != "jellyfin" {
		return nil, fmt.Errorf("target must be emby or jellyfin")
	}
	allLibraries := append([]Library(nil), bundle.Libraries...)
	allLibraries = append(allLibraries, bundle.Catchup...)
	if len(allLibraries) == 0 {
		return nil, fmt.Errorf("bundle has no libraries")
	}
	libraries := make([]emby.LibraryCreateSpec, 0, len(allLibraries))
	seen := map[string]bool{}
	for _, lib := range allLibraries {
		collectionType, ok := plexLibraryTypeToCollectionType(lib.Type)
		if !ok {
			continue
		}
		for _, location := range lib.Locations {
			location = strings.TrimSpace(location)
			if location == "" {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(lib.Name) + "|" + collectionType + "|" + location)
			if seen[key] {
				break
			}
			libraries = append(libraries, emby.LibraryCreateSpec{
				Name:            strings.TrimSpace(lib.Name),
				CollectionType:  collectionType,
				Path:            location,
				SourceItemCount: lib.SourceItemCount,
				SourceTitles:    append([]string(nil), lib.SourceTitles...),
			})
			seen[key] = true
			break
		}
	}
	if len(libraries) == 0 {
		return nil, fmt.Errorf("bundle has no convertible libraries")
	}
	return &LibraryPlan{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Target:       target,
		BundleSource: strings.TrimSpace(bundle.Source),
		TargetHost:   strings.TrimSpace(host),
		Libraries:    libraries,
		Notes: []string{
			"Library conversion preserves names, types, and shared storage paths only.",
			"It does not attempt Plex-to-Emby/Jellyfin metadata DB translation.",
		},
	}, nil
}

func AttachCatchupManifest(bundle Bundle, manifest tuner.CatchupPublishManifest) Bundle {
	out := bundle
	out.Catchup = make([]Library, 0, len(manifest.Libraries))
	for _, lib := range manifest.Libraries {
		path := strings.TrimSpace(lib.Path)
		if path == "" {
			continue
		}
		out.Catchup = append(out.Catchup, Library{
			Name:      strings.TrimSpace(lib.Name),
			Type:      "movie",
			Locations: []string{path},
		})
	}
	if len(out.Catchup) > 0 {
		out.Notes = append(out.Notes, "Attached generated catch-up library definitions from a publish manifest.")
	}
	return out
}

func ApplyLibraryPlan(plan LibraryPlan, host, token string, refresh bool) (*LibraryApplyResult, error) {
	target := strings.ToLower(strings.TrimSpace(plan.Target))
	if target != "emby" && target != "jellyfin" {
		return nil, fmt.Errorf("target must be emby or jellyfin")
	}
	host = strings.TrimSpace(firstNonEmptyString(host, plan.TargetHost))
	token = strings.TrimSpace(token)
	if host == "" {
		return nil, fmt.Errorf("target host required")
	}
	if token == "" {
		return nil, fmt.Errorf("target token required")
	}
	if len(plan.Libraries) == 0 {
		return nil, fmt.Errorf("library plan has no libraries")
	}
	cfg := emby.Config{Host: host, Token: token, ServerType: target}
	results := make([]LibraryApplyLibrary, 0, len(plan.Libraries))
	for _, lib := range plan.Libraries {
		spec := lib
		spec.Refresh = false
		info, created, err := emby.EnsureLibrary(cfg, spec)
		if err != nil {
			return nil, fmt.Errorf("ensure library %q: %w", spec.Name, err)
		}
		entry := LibraryApplyLibrary{
			Name:           spec.Name,
			CollectionType: spec.CollectionType,
			Locations:      []string{spec.Path},
			Created:        created,
		}
		if info != nil {
			entry.ID = strings.TrimSpace(info.ID)
			if len(info.Locations) > 0 {
				entry.Locations = append([]string(nil), info.Locations...)
			}
		}
		results = append(results, entry)
	}
	if refresh {
		if err := emby.RefreshLibraryScan(cfg); err != nil {
			return nil, fmt.Errorf("refresh library scan: %w", err)
		}
	}
	return &LibraryApplyResult{
		AppliedAt:  time.Now().UTC().Format(time.RFC3339),
		Target:     target,
		TargetHost: host,
		Refresh:    refresh,
		Libraries:  results,
		Notes: []string{
			"Applied shared library paths without touching Plex metadata rows, watch-state, or vendor-specific agent settings.",
		},
	}, nil
}

func DiffLibraryPlan(plan LibraryPlan, host, token string) (*LibraryDiffResult, error) {
	target := strings.ToLower(strings.TrimSpace(plan.Target))
	if target != "emby" && target != "jellyfin" {
		return nil, fmt.Errorf("target must be emby or jellyfin")
	}
	host = strings.TrimSpace(firstNonEmptyString(host, plan.TargetHost))
	token = strings.TrimSpace(token)
	if host == "" {
		return nil, fmt.Errorf("target host required")
	}
	if token == "" {
		return nil, fmt.Errorf("target token required")
	}
	if len(plan.Libraries) == 0 {
		return nil, fmt.Errorf("library plan has no libraries")
	}
	cfg := emby.Config{Host: host, Token: token, ServerType: target}
	existing, err := emby.ListLibraries(cfg)
	if err != nil {
		return nil, fmt.Errorf("list target libraries: %w", err)
	}
	results := make([]LibraryDiffLibrary, 0, len(plan.Libraries))
	var createCount, reuseCount, conflictCount int
	for _, spec := range plan.Libraries {
		entry := diffLibraryAgainstExisting(spec, existing)
		if entry.Status == "reuse" && strings.TrimSpace(entry.ExistingID) != "" {
			count, err := emby.GetLibraryItemCount(cfg, entry.ExistingID)
			if err != nil {
				return nil, fmt.Errorf("get item count for library %q: %w", entry.Name, err)
			}
			entry.ExistingItemCount = count
			if entry.SourceItemCount > 0 {
				if entry.ExistingItemCount >= entry.SourceItemCount {
					entry.ParityStatus = "synced"
				} else {
					entry.ParityStatus = "lagging"
				}
			}
			if len(entry.SourceTitles) > 0 {
				titles, err := emby.GetLibraryItemTitles(cfg, entry.ExistingID, len(entry.SourceTitles))
				if err != nil {
					return nil, fmt.Errorf("get title sample for library %q: %w", entry.Name, err)
				}
				entry.ExistingTitles = titles
				entry.MissingTitles = missingNormalizedTitles(entry.SourceTitles, entry.ExistingTitles)
				if len(entry.MissingTitles) == 0 {
					entry.TitleParityStatus = "sample_synced"
				} else {
					entry.TitleParityStatus = "sample_missing"
				}
			}
		}
		switch entry.Status {
		case "create":
			createCount++
		case "reuse":
			reuseCount++
		default:
			conflictCount++
		}
		results = append(results, entry)
	}
	notes := []string{
		"Diff compares bundled desired libraries against the current target server state before apply.",
	}
	if conflictCount > 0 {
		notes = append(notes, "Conflicts must be resolved manually before apply can succeed cleanly.")
	}
	return &LibraryDiffResult{
		ComparedAt:    time.Now().UTC().Format(time.RFC3339),
		Target:        target,
		TargetHost:    host,
		DesiredCount:  len(plan.Libraries),
		PresentCount:  reuseCount,
		CreateCount:   createCount,
		ReuseCount:    reuseCount,
		ConflictCount: conflictCount,
		Libraries:     results,
		Notes:         notes,
	}, nil
}

func BuildLibraryRolloutPlan(bundle Bundle, specs []TargetSpec) (*LibraryRolloutPlan, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("at least one target required")
	}
	plans := make([]LibraryPlan, 0, len(specs))
	seen := map[string]bool{}
	for _, spec := range specs {
		target := strings.ToLower(strings.TrimSpace(spec.Target))
		if target == "" || seen[target] {
			continue
		}
		plan, err := BuildLibraryPlan(bundle, target, spec.Host)
		if err != nil {
			return nil, err
		}
		plans = append(plans, *plan)
		seen[target] = true
	}
	if len(plans) == 0 {
		return nil, fmt.Errorf("no valid rollout targets")
	}
	return &LibraryRolloutPlan{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		BundleSource: strings.TrimSpace(bundle.Source),
		Plans:        plans,
		Notes: []string{
			"One neutral bundle can pre-roll shared library definitions across multiple non-Plex targets.",
		},
	}, nil
}

func ApplyLibraryRolloutPlan(plan LibraryRolloutPlan, apply map[string]ApplySpec, refresh bool) (*LibraryRolloutApplyResult, error) {
	if len(plan.Plans) == 0 {
		return nil, fmt.Errorf("library rollout plan has no targets")
	}
	results := make([]LibraryApplyResult, 0, len(plan.Plans))
	for _, entry := range plan.Plans {
		spec := apply[strings.ToLower(strings.TrimSpace(entry.Target))]
		res, err := ApplyLibraryPlan(entry, spec.Host, spec.Token, refresh)
		if err != nil {
			return nil, fmt.Errorf("apply %s: %w", entry.Target, err)
		}
		results = append(results, *res)
	}
	return &LibraryRolloutApplyResult{
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Results:   results,
		Notes: []string{
			"Library rollout apply intentionally leaves Plex sections and metadata untouched.",
		},
	}, nil
}

func DiffLibraryRolloutPlan(plan LibraryRolloutPlan, apply map[string]ApplySpec) (*LibraryRolloutDiffResult, error) {
	if len(plan.Plans) == 0 {
		return nil, fmt.Errorf("library rollout plan has no targets")
	}
	results := make([]LibraryDiffResult, 0, len(plan.Plans))
	for _, entry := range plan.Plans {
		spec := apply[strings.ToLower(strings.TrimSpace(entry.Target))]
		res, err := DiffLibraryPlan(entry, spec.Host, spec.Token)
		if err != nil {
			return nil, fmt.Errorf("diff %s: %w", entry.Target, err)
		}
		results = append(results, *res)
	}
	return &LibraryRolloutDiffResult{
		ComparedAt: time.Now().UTC().Format(time.RFC3339),
		Results:    results,
		Notes: []string{
			"Rollout diff compares the same neutral bundle against multiple non-Plex targets without mutating them.",
		},
	}, nil
}

func AuditBundleTargets(bundle Bundle, specs []TargetSpec, apply map[string]ApplySpec) (*MigrationAuditResult, error) {
	rollout, err := BuildRolloutPlan(bundle, specs)
	if err != nil {
		return nil, err
	}
	liveDiff, err := DiffRolloutPlan(*rollout, apply)
	if err != nil {
		return nil, err
	}
	results := make([]MigrationTargetAudit, 0, len(liveDiff.Results))
	for _, res := range liveDiff.Results {
		results = append(results, MigrationTargetAudit{
			Target:       res.Target,
			TargetHost:   res.TargetHost,
			LiveTV:       res,
			LiveTVReady:  res.ConflictCount == 0,
			ReadyToApply: res.ConflictCount == 0,
			Status:       auditTargetStatus(res.ConflictCount == 0, true, res.IndexedChannelCount, true),
		})
	}

	allLibraries := append([]Library(nil), bundle.Libraries...)
	allLibraries = append(allLibraries, bundle.Catchup...)
	if len(allLibraries) == 0 {
		var readyCount, conflictCount int
		for i := range results {
			results[i].LibraryMode = "not_in_bundle"
			results[i].LibraryReady = true
			results[i].ConflictCount = results[i].LiveTV.ConflictCount
			results[i].Status = auditTargetStatus(results[i].LiveTVReady, true, results[i].LiveTV.IndexedChannelCount, true)
			results[i].StatusReason = auditTargetReason(results[i])
			if results[i].ReadyToApply {
				readyCount++
			}
			conflictCount += results[i].ConflictCount
		}
		return &MigrationAuditResult{
			ComparedAt:       time.Now().UTC().Format(time.RFC3339),
			Source:           strings.TrimSpace(bundle.Source),
			Status:           auditOverallStatus(conflictCount, readyCount, len(results), results),
			ReadyToApply:     readyCount == len(results),
			TargetCount:      len(results),
			ReadyTargetCount: readyCount,
			ConflictCount:    conflictCount,
			Results:          results,
			Notes: []string{
				"Bundle audit includes Live TV validation for every requested target.",
				"Library audit was skipped because the bundle carries no shared libraries or attached catch-up lanes.",
			},
		}, nil
	}

	libraryRollout, err := BuildLibraryRolloutPlan(bundle, specs)
	if err != nil {
		return nil, err
	}
	libraryDiff, err := DiffLibraryRolloutPlan(*libraryRollout, apply)
	if err != nil {
		return nil, err
	}
	byTarget := map[string]LibraryDiffResult{}
	for _, res := range libraryDiff.Results {
		byTarget[strings.ToLower(strings.TrimSpace(res.Target))] = res
	}
	for i := range results {
		target := strings.ToLower(strings.TrimSpace(results[i].Target))
		if lib, ok := byTarget[target]; ok {
			copyLib := lib
			results[i].Library = &copyLib
			results[i].LibraryMode = "included"
			results[i].LibraryReady = copyLib.ConflictCount == 0
			results[i].ReadyToApply = results[i].LiveTVReady && results[i].LibraryReady
			results[i].ConflictCount = results[i].LiveTV.ConflictCount + copyLib.ConflictCount
			results[i].SyncedLibraries = libraryNamesByParity(copyLib, "synced")
			results[i].LaggingLibraries = libraryNamesByParity(copyLib, "lagging")
			results[i].TitleSyncedLibraries = libraryNamesByTitleParity(copyLib, "sample_synced")
			results[i].TitleLaggingLibraries = libraryNamesByTitleParity(copyLib, "sample_missing")
			results[i].PresentLibraries = libraryNamesByStatus(copyLib, "reuse")
			results[i].MissingLibraries = libraryNamesByStatus(copyLib, "create")
			results[i].PopulatedLibraries = populatedLibraryNames(copyLib, true)
			results[i].EmptyLibraries = populatedLibraryNames(copyLib, false)
			cfg := emby.Config{
				Host:       strings.TrimSpace(firstNonEmptyString(apply[target].Host, results[i].TargetHost)),
				Token:      strings.TrimSpace(apply[target].Token),
				ServerType: target,
			}
			if cfg.Host != "" && cfg.Token != "" {
				scanStatus, err := emby.GetLibraryScanStatus(cfg)
				if err == nil {
					results[i].LibraryScan = scanStatus
				}
			}
			results[i].Status = auditTargetStatus(results[i].LiveTVReady, results[i].LibraryReady, results[i].LiveTV.IndexedChannelCount, copyLib.CreateCount == 0)
			results[i].StatusReason = auditTargetReason(results[i])
		} else {
			results[i].LibraryMode = "not_requested"
			results[i].LibraryReady = true
			results[i].ReadyToApply = results[i].LiveTVReady
			results[i].ConflictCount = results[i].LiveTV.ConflictCount
			results[i].Status = auditTargetStatus(results[i].LiveTVReady, true, results[i].LiveTV.IndexedChannelCount, true)
			results[i].StatusReason = auditTargetReason(results[i])
		}
	}
	var readyCount, conflictCount int
	for _, result := range results {
		if result.ReadyToApply {
			readyCount++
		}
		conflictCount += result.ConflictCount
	}
	return &MigrationAuditResult{
		ComparedAt:       time.Now().UTC().Format(time.RFC3339),
		Source:           strings.TrimSpace(bundle.Source),
		Status:           auditOverallStatus(conflictCount, readyCount, len(results), results),
		ReadyToApply:     readyCount == len(results),
		TargetCount:      len(results),
		ReadyTargetCount: readyCount,
		ConflictCount:    conflictCount,
		Results:          results,
		Notes: []string{
			"Bundle audit combines Live TV registration validation and library/catch-up validation per target.",
		},
	}, nil
}

func chooseDVR(dvrs []plex.DVRInfo, key int) (*plex.DVRInfo, error) {
	if key > 0 {
		for i := range dvrs {
			if dvrs[i].Key == key {
				return &dvrs[i], nil
			}
		}
		return nil, fmt.Errorf("plex dvr key %d not found", key)
	}
	if len(dvrs) == 1 {
		return &dvrs[0], nil
	}
	keys := make([]string, 0, len(dvrs))
	for _, dvr := range dvrs {
		keys = append(keys, fmt.Sprintf("%d:%s", dvr.Key, strings.TrimSpace(dvr.LineupTitle)))
	}
	return nil, fmt.Errorf("multiple plex dvrs found; set -dvr-key (%s)", strings.Join(keys, ", "))
}

func matchPlexDevice(devices []plex.Device, dvr *plex.DVRInfo) (*plex.Device, error) {
	if dvr == nil {
		return nil, fmt.Errorf("plex dvr required")
	}
	for _, want := range dvr.DeviceUUIDs {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		for i := range devices {
			if strings.TrimSpace(devices[i].UUID) == want {
				return &devices[i], nil
			}
		}
	}
	if strings.TrimSpace(dvr.DeviceKey) != "" {
		for i := range devices {
			if strings.TrimSpace(devices[i].Key) == strings.TrimSpace(dvr.DeviceKey) {
				return &devices[i], nil
			}
		}
	}
	return nil, fmt.Errorf("no plex device matched dvr key=%d", dvr.Key)
}

func parsePlexLineupXMLTVURL(lineup string) (string, error) {
	lineup = strings.TrimSpace(lineup)
	if lineup == "" {
		return "", fmt.Errorf("plex lineup url is empty")
	}
	const prefix = "lineup://tv.plex.providers.epg.xmltv/"
	if !strings.HasPrefix(lineup, prefix) {
		return "", fmt.Errorf("unsupported plex lineup url %q", lineup)
	}
	raw := strings.TrimPrefix(lineup, prefix)
	if head, _, ok := strings.Cut(raw, "#"); ok {
		raw = head
	}
	decoded, err := url.QueryUnescape(raw)
	if err == nil && strings.TrimSpace(decoded) != "" {
		raw = decoded
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("plex lineup xmltv url is empty")
	}
	return raw, nil
}

func hostPortFromBaseURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("base url required")
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("base url host required")
	}
	return u.Host, nil
}

func Load(path string) (*Bundle, error) {
	var bundle Bundle
	data, err := osReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, err
	}
	return &bundle, nil
}

func Save(path string, bundle Bundle) error {
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	return osWriteFile(path, data)
}

var osReadFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}

var osWriteFile = func(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func diffTunerHost(want emby.TunerHostInfo, existing []emby.TunerHostInfo) LiveTVDiffEntry {
	wantURL := strings.TrimSpace(want.Url)
	wantName := strings.TrimSpace(want.FriendlyName)
	entry := LiveTVDiffEntry{Kind: "tuner_host", Status: "create"}
	for _, item := range existing {
		itemURL := strings.TrimSpace(item.Url)
		if itemURL != wantURL {
			continue
		}
		entry.ExistingID = strings.TrimSpace(item.Id)
		if strings.TrimSpace(item.Type) != strings.TrimSpace(want.Type) {
			entry.Status = "conflict_type"
			entry.Reason = "tuner host URL already exists with a different tuner type"
			return entry
		}
		if wantName != "" && strings.TrimSpace(item.FriendlyName) != "" && strings.TrimSpace(item.FriendlyName) != wantName {
			entry.Status = "conflict_name"
			entry.Reason = "tuner host URL already exists with a different friendly name"
			return entry
		}
		entry.Status = "reuse"
		entry.Reason = "tuner host already exists with matching URL and type"
		return entry
	}
	return entry
}

func diffListingProvider(want emby.ListingsProviderInfo, existing []emby.ListingsProviderInfo) LiveTVDiffEntry {
	wantPath := strings.TrimSpace(want.Path)
	entry := LiveTVDiffEntry{Kind: "listing_provider", Status: "create"}
	for _, item := range existing {
		itemPath := strings.TrimSpace(item.Path)
		if itemPath != wantPath {
			continue
		}
		entry.ExistingID = strings.TrimSpace(item.Id)
		if strings.TrimSpace(item.Type) != strings.TrimSpace(want.Type) {
			entry.Status = "conflict_type"
			entry.Reason = "listing provider path already exists with a different provider type"
			return entry
		}
		entry.Status = "reuse"
		entry.Reason = "listing provider already exists with matching path and type"
		return entry
	}
	return entry
}

func diffLibraryAgainstExisting(spec emby.LibraryCreateSpec, existing []emby.LibraryInfo) LibraryDiffLibrary {
	wantName := strings.TrimSpace(spec.Name)
	wantType := strings.ToLower(strings.TrimSpace(spec.CollectionType))
	wantPath := filepathClean(strings.TrimSpace(spec.Path))
	entry := LibraryDiffLibrary{
		Name:            wantName,
		CollectionType:  wantType,
		DesiredPath:     wantPath,
		SourceItemCount: spec.SourceItemCount,
		SourceTitles:    append([]string(nil), spec.SourceTitles...),
		Status:          "create",
	}
	var sameName []emby.LibraryInfo
	for _, lib := range existing {
		if strings.TrimSpace(lib.Name) == wantName {
			sameName = append(sameName, lib)
		}
	}
	if len(sameName) == 0 {
		return entry
	}
	for _, lib := range sameName {
		if strings.ToLower(strings.TrimSpace(lib.CollectionType)) != wantType {
			continue
		}
		entry.ExistingID = strings.TrimSpace(lib.ID)
		entry.ExistingLocations = append([]string(nil), lib.Locations...)
		for _, loc := range lib.Locations {
			if filepathClean(strings.TrimSpace(loc)) == wantPath {
				entry.Status = "reuse"
				entry.Reason = "library already exists with matching name, type, and path"
				return entry
			}
		}
		entry.Status = "conflict_path"
		entry.Reason = "library name and type exist but point at different paths"
		return entry
	}
	entry.ExistingID = strings.TrimSpace(sameName[0].ID)
	entry.ExistingLocations = append([]string(nil), sameName[0].Locations...)
	entry.Status = "conflict_type"
	entry.Reason = "library name already exists with a different collection type"
	return entry
}

func filepathClean(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(strings.ReplaceAll(value, `\`, `/`))
}

func auditTargetStatus(liveReady, libraryReady bool, indexedChannelCount int, libraryConverged bool) string {
	if !liveReady || !libraryReady {
		return "blocked_conflicts"
	}
	if indexedChannelCount > 0 && libraryConverged {
		return "converged"
	}
	return "ready_to_apply"
}

func auditOverallStatus(conflictCount, readyCount, total int, results []MigrationTargetAudit) string {
	if conflictCount > 0 {
		return "blocked_conflicts"
	}
	if total > 0 && readyCount == total {
		allConverged := true
		for _, result := range results {
			if result.Status != "converged" {
				allConverged = false
				break
			}
		}
		if allConverged {
			return "converged"
		}
		return "ready_to_apply"
	}
	return "mixed"
}

func auditTargetReason(result MigrationTargetAudit) string {
	if result.ConflictCount > 0 {
		return "definition conflicts must be resolved before apply"
	}
	if result.LibraryMode == "not_in_bundle" && result.LiveTV.IndexedChannelCount == 0 {
		return "no conflicts detected, but the target has not indexed Live TV channels yet"
	}
	if len(result.LaggingLibraries) > 0 && result.LiveTV.IndexedChannelCount > 0 {
		return "live tv is indexed, but some reused libraries still lag the Plex source counts"
	}
	if len(result.LaggingLibraries) > 0 {
		return "no conflicts detected, but some reused libraries still lag the Plex source counts"
	}
	if len(result.TitleLaggingLibraries) > 0 && result.LiveTV.IndexedChannelCount > 0 {
		return "live tv is indexed, but some reused libraries are still missing source sample titles"
	}
	if len(result.TitleLaggingLibraries) > 0 {
		return "no conflicts detected, but some reused libraries are still missing source sample titles"
	}
	if len(result.MissingLibraries) > 0 && result.LiveTV.IndexedChannelCount > 0 {
		return "live tv is indexed, but bundled libraries are still missing"
	}
	if len(result.MissingLibraries) > 0 {
		return "no conflicts detected, but bundled libraries are still missing"
	}
	if result.LiveTV.IndexedChannelCount > 0 {
		return "live tv is indexed and bundled surfaces are already present"
	}
	return "no conflicts detected, but the target has not indexed Live TV channels yet"
}

func libraryNamesByStatus(diff LibraryDiffResult, status string) []string {
	names := make([]string, 0, len(diff.Libraries))
	for _, item := range diff.Libraries {
		if item.Status == status {
			names = append(names, strings.TrimSpace(item.Name))
		}
	}
	return names
}

func populatedLibraryNames(diff LibraryDiffResult, populated bool) []string {
	names := make([]string, 0, len(diff.Libraries))
	for _, item := range diff.Libraries {
		if item.Status != "reuse" {
			continue
		}
		if populated && item.ExistingItemCount > 0 {
			names = append(names, strings.TrimSpace(item.Name))
		}
		if !populated && item.ExistingItemCount == 0 {
			names = append(names, strings.TrimSpace(item.Name))
		}
	}
	return names
}

func libraryNamesByParity(diff LibraryDiffResult, parity string) []string {
	names := make([]string, 0, len(diff.Libraries))
	for _, item := range diff.Libraries {
		if item.ParityStatus == parity {
			names = append(names, strings.TrimSpace(item.Name))
		}
	}
	return names
}

func libraryNamesByTitleParity(diff LibraryDiffResult, parity string) []string {
	names := make([]string, 0, len(diff.Libraries))
	for _, item := range diff.Libraries {
		if item.TitleParityStatus == parity {
			names = append(names, strings.TrimSpace(item.Name))
		}
	}
	return names
}

func missingNormalizedTitles(source, existing []string) []string {
	if len(source) == 0 {
		return nil
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, title := range existing {
		if norm := normalizeSampleTitle(title); norm != "" {
			existingSet[norm] = struct{}{}
		}
	}
	missing := make([]string, 0, len(source))
	for _, title := range source {
		norm := normalizeSampleTitle(title)
		if norm == "" {
			continue
		}
		if _, ok := existingSet[norm]; ok {
			continue
		}
		missing = append(missing, strings.TrimSpace(title))
	}
	return missing
}

func normalizeSampleTitle(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func plexLibraryTypeToCollectionType(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "movie":
		return "movies", true
	case "show":
		return "tvshows", true
	default:
		return "", false
	}
}

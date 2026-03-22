package livetvbundle

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/emby"
	"github.com/snapetech/iptvtunerr/internal/plex"
)

type Bundle struct {
	GeneratedAt string            `json:"generated_at"`
	Source      string            `json:"source"`
	Tuner       Tuner             `json:"tuner"`
	Guide       Guide             `json:"guide"`
	Lineup      Lineup            `json:"lineup"`
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

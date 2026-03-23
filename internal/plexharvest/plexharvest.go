package plexharvest

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/hdhomerun"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/plex"
)

type Target struct {
	BaseURL      string `json:"base_url"`
	Cap          string `json:"cap,omitempty"`
	FriendlyName string `json:"friendly_name"`
	DeviceID     string `json:"device_id"`
}

type Result struct {
	BaseURL        string             `json:"base_url"`
	Cap            string             `json:"cap,omitempty"`
	FriendlyName   string             `json:"friendly_name"`
	DeviceKey      string             `json:"device_key,omitempty"`
	DeviceUUID     string             `json:"device_uuid,omitempty"`
	DVRKey         int                `json:"dvr_key,omitempty"`
	DVRUUID        string             `json:"dvr_uuid,omitempty"`
	LineupTitle    string             `json:"lineup_title,omitempty"`
	LineupURL      string             `json:"lineup_url,omitempty"`
	LineupIDs      []string           `json:"lineup_ids,omitempty"`
	LineupID       string             `json:"lineup_id,omitempty"`
	LineupType     string             `json:"lineup_type,omitempty"`
	LineupSource   string             `json:"lineup_source,omitempty"`
	Channels       []HarvestedChannel `json:"channels,omitempty"`
	ChannelMapRows int                `json:"channelmap_rows,omitempty"`
	ChannelCount   int                `json:"channel_count,omitempty"`
	Activated      int                `json:"activated,omitempty"`
	Error          string             `json:"error,omitempty"`
}

type HarvestedChannel struct {
	ChannelID   string `json:"channel_id,omitempty"`
	GuideNumber string `json:"guide_number,omitempty"`
	GuideName   string `json:"guide_name"`
	TVGID       string `json:"tvg_id,omitempty"`
	GroupTitle  string `json:"group_title,omitempty"`
}

type SummaryLineup struct {
	LineupTitle        string   `json:"lineup_title"`
	Targets            []string `json:"targets,omitempty"`
	FriendlyNames      []string `json:"friendly_names,omitempty"`
	Successes          int      `json:"successes"`
	BestChannelMapRows int      `json:"best_channelmap_rows"`
	BestChannelCount   int      `json:"best_channel_count,omitempty"`
}

type Report struct {
	GeneratedAt string          `json:"generated_at"`
	PlexURL     string          `json:"plex_url"`
	WaitSeconds int             `json:"wait_seconds"`
	Results     []Result        `json:"results"`
	Lineups     []SummaryLineup `json:"lineups,omitempty"`
}

type ProbeRequest struct {
	PlexHost     string
	PlexToken    string
	Targets      []Target
	Wait         time.Duration
	PollInterval time.Duration
	ReloadGuide  bool
	Activate     bool
}

type ProviderProbeRequest struct {
	ProviderBaseURL string
	ProviderVersion string
	PlexToken       string
	Country         string
	PostalCode      string
	Types           []string
	TitleQuery      string
	Limit           int
	IncludeChannels bool
}

type ProviderDefaultLocation struct {
	Country    string
	PostalCode string
}

type providerDefaultCatalog struct {
	Timezones map[string]ProviderDefaultLocation `json:"timezones"`
	Families  map[string]ProviderDefaultLocation `json:"families"`
}

//go:embed provider_tz_defaults.json
var providerTZDefaultsJSON []byte

var (
	providerDefaultsOnce sync.Once
	providerDefaultsData providerDefaultCatalog
)

var (
	registerTunerViaAPI = plex.RegisterTunerViaAPI
	createDVRViaAPI     = plex.CreateDVRViaAPI
	reloadGuideAPI      = plex.ReloadGuideAPI
	getChannelMap       = plex.GetChannelMap
	activateChannelsAPI = plex.ActivateChannelsAPI
	listDVRsAPI         = plex.ListDVRsAPI
	fetchLineupRows     = fetchLineup
)

func providerDefaultCatalogData() providerDefaultCatalog {
	providerDefaultsOnce.Do(func() {
		providerDefaultsData = providerDefaultCatalog{
			Timezones: map[string]ProviderDefaultLocation{},
			Families:  map[string]ProviderDefaultLocation{},
		}
		if err := json.Unmarshal(providerTZDefaultsJSON, &providerDefaultsData); err != nil {
			panic(fmt.Sprintf("plexharvest: invalid provider_tz_defaults.json: %v", err))
		}
		if providerDefaultsData.Timezones == nil {
			providerDefaultsData.Timezones = map[string]ProviderDefaultLocation{}
		}
		if providerDefaultsData.Families == nil {
			providerDefaultsData.Families = map[string]ProviderDefaultLocation{}
		}
	})
	return providerDefaultsData
}

func ExpandTargets(baseURLsCSV, baseTemplate, capsCSV, namePrefix string) []Target {
	prefix := strings.TrimSpace(namePrefix)
	if prefix == "" {
		prefix = "harvest-"
	}
	targets := make([]Target, 0)
	seen := map[string]struct{}{}
	for _, base := range splitCSV(baseURLsCSV) {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		targets = appendTarget(targets, seen, Target{
			BaseURL:      base,
			FriendlyName: targetFriendlyName(prefix, ""),
			DeviceID:     targetDeviceID(prefix, "", len(targets)+1),
		})
	}
	template := strings.TrimSpace(baseTemplate)
	if template == "" {
		return targets
	}
	for _, cap := range splitCSV(capsCSV) {
		base := strings.ReplaceAll(template, "{cap}", strings.TrimSpace(cap))
		targets = appendTarget(targets, seen, Target{
			BaseURL:      base,
			Cap:          strings.TrimSpace(cap),
			FriendlyName: targetFriendlyName(prefix, cap),
			DeviceID:     targetDeviceID(prefix, cap, len(targets)+1),
		})
	}
	return targets
}

func Probe(req ProbeRequest) Report {
	wait := req.Wait
	if wait <= 0 {
		wait = 45 * time.Second
	}
	poll := req.PollInterval
	if poll <= 0 {
		poll = 5 * time.Second
	}
	report := Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		PlexURL:     req.PlexHost,
		WaitSeconds: int(wait / time.Second),
		Results:     make([]Result, 0, len(req.Targets)),
	}
	for _, target := range req.Targets {
		res := Result{
			BaseURL:      strings.TrimSpace(target.BaseURL),
			Cap:          strings.TrimSpace(target.Cap),
			FriendlyName: strings.TrimSpace(target.FriendlyName),
		}
		cfg := plex.PlexAPIConfig{
			BaseURL:      res.BaseURL,
			PlexHost:     req.PlexHost,
			PlexToken:    req.PlexToken,
			FriendlyName: res.FriendlyName,
			DeviceID:     strings.TrimSpace(target.DeviceID),
		}
		deviceInfo, err := registerTunerViaAPI(cfg)
		if err != nil {
			res.Error = "register device: " + err.Error()
			report.Results = append(report.Results, res)
			continue
		}
		res.DeviceKey = strings.TrimSpace(deviceInfo.Key)
		res.DeviceUUID = strings.TrimSpace(deviceInfo.UUID)
		dvrKey, dvrUUID, lineupIDs, err := createDVRViaAPI(cfg, deviceInfo)
		if err != nil {
			res.Error = "create dvr: " + err.Error()
			report.Results = append(report.Results, res)
			continue
		}
		res.DVRKey = dvrKey
		res.DVRUUID = strings.TrimSpace(dvrUUID)
		res.LineupIDs = append([]string(nil), lineupIDs...)
		if req.ReloadGuide {
			if err := reloadGuideAPI(req.PlexHost, req.PlexToken, dvrKey); err != nil {
				res.Error = "reload guide: " + err.Error()
				report.Results = append(report.Results, res)
				continue
			}
		}
		deadline := time.Now().Add(wait)
		for {
			mappings, err := getChannelMap(req.PlexHost, req.PlexToken, deviceInfo.UUID, lineupIDs)
			if err == nil {
				res.ChannelMapRows = len(mappings)
				if req.Activate && len(mappings) > 0 {
					activated, actErr := activateChannelsAPI(cfg, deviceInfo.Key, mappings)
					if actErr != nil {
						res.Error = "activate channelmap: " + actErr.Error()
					} else {
						res.Activated = activated
					}
				}
				if len(mappings) > 0 || time.Now().After(deadline) {
					break
				}
			} else if time.Now().After(deadline) {
				res.Error = "get channelmap: " + err.Error()
				break
			}
			time.Sleep(poll)
		}
		if dvrs, err := listDVRsAPI(req.PlexHost, req.PlexToken); err == nil {
			for _, dvr := range dvrs {
				if dvr.Key == dvrKey {
					res.LineupTitle = strings.TrimSpace(dvr.LineupTitle)
					res.LineupURL = strings.TrimSpace(dvr.LineupURL)
					break
				}
			}
		}
		if channels, err := fetchLineupRows(res.BaseURL); err == nil {
			res.Channels = channels
		}
		report.Results = append(report.Results, res)
	}
	report.Lineups = buildSummary(report.Results)
	return report
}

type providerLineupListResponse struct {
	MediaContainer struct {
		Size   int                   `json:"size"`
		Lineup []providerLineupEntry `json:"Lineup"`
	} `json:"MediaContainer"`
}

type providerLineupEntry struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Source     string `json:"source"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	IsNational bool   `json:"isNational"`
}

type providerLineupChannelsResponse struct {
	MediaContainer struct {
		Size    int                    `json:"size"`
		Title   string                 `json:"title"`
		Channel []providerChannelEntry `json:"Channel"`
	} `json:"MediaContainer"`
}

type providerChannelEntry struct {
	CallSign          string `json:"callSign"`
	AffiliateCallSign string `json:"affiliateCallSign"`
	ID                string `json:"id"`
	Title             string `json:"title"`
	VCN               string `json:"vcn"`
}

func ProbeProviderLineups(req ProviderProbeRequest) Report {
	baseURL := strings.TrimSpace(req.ProviderBaseURL)
	if baseURL == "" {
		baseURL = "https://epg.provider.plex.tv"
	}
	version := strings.TrimSpace(req.ProviderVersion)
	if version == "" {
		version = "5.1"
	}
	report := Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		PlexURL:     baseURL,
		Results:     []Result{},
	}
	country, postalCode := normalizeProviderQuery(req.Country, req.PostalCode)
	if country == "" || postalCode == "" {
		report.Results = append(report.Results, Result{
			Error: fmt.Sprintf("provider lineup query requires country and postal code (got country=%q postal_code=%q)", country, postalCode),
		})
		report.Lineups = buildSummary(report.Results)
		return report
	}
	listings, err := fetchProviderLineups(baseURL, version, req.PlexToken, country, postalCode)
	if err != nil {
		report.Results = append(report.Results, Result{
			LineupTitle: fmt.Sprintf("%s %s", country, postalCode),
			Error:       err.Error(),
		})
		report.Lineups = buildSummary(report.Results)
		return report
	}
	filtered := filterProviderLineups(listings, req.Types, req.TitleQuery, req.Limit)
	for _, lineup := range filtered {
		res := Result{
			LineupID:     strings.TrimSpace(lineup.ID),
			LineupTitle:  strings.TrimSpace(lineup.Title),
			LineupURL:    providerAbsoluteURL(baseURL, lineup.Key),
			LineupType:   strings.TrimSpace(lineup.Type),
			LineupSource: strings.TrimSpace(lineup.Source),
		}
		if req.IncludeChannels {
			channels, count, err := fetchProviderLineupChannels(baseURL, version, req.PlexToken, lineup.Key)
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Channels = channels
				res.ChannelCount = count
			}
		}
		report.Results = append(report.Results, res)
	}
	report.Lineups = buildSummary(report.Results)
	return report
}

func normalizeProviderQuery(country, postalCode string) (string, string) {
	country = strings.ToUpper(strings.TrimSpace(country))
	postalCode = strings.TrimSpace(postalCode)
	return country, postalCode
}

func DefaultProviderLocationFromTZ(zone string) ProviderDefaultLocation {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return ProviderDefaultLocation{}
	}
	catalog := providerDefaultCatalogData()
	if loc, ok := catalog.Timezones[zone]; ok {
		return loc
	}
	if head, _, ok := strings.Cut(zone, "/"); ok {
		if loc, ok := catalog.Families[head]; ok {
			return loc
		}
	}
	return ProviderDefaultLocation{}
}

func LoadReportFile(path string) (Report, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Report{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{}, nil
		}
		return Report{}, err
	}
	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return Report{}, err
	}
	if len(rep.Lineups) == 0 && len(rep.Results) > 0 {
		rep.Lineups = buildSummary(rep.Results)
	}
	return rep, nil
}

func SaveReportFile(path string, rep Report) (Report, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Report{}, fmt.Errorf("lineup harvest file not configured")
	}
	if strings.TrimSpace(rep.GeneratedAt) == "" {
		rep.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if len(rep.Lineups) == 0 && len(rep.Results) > 0 {
		rep.Lineups = buildSummary(rep.Results)
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return Report{}, err
	}
	dir := filepath.Dir(filepath.Clean(path))
	tmp, err := os.CreateTemp(dir, ".plex-lineup-harvest-*.json.tmp")
	if err != nil {
		return Report{}, err
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		if writeErr != nil {
			return Report{}, writeErr
		}
		return Report{}, closeErr
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return Report{}, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return Report{}, err
	}
	return rep, nil
}

func buildSummary(results []Result) []SummaryLineup {
	byTitle := map[string]*SummaryLineup{}
	for _, res := range results {
		if res.Error != "" {
			continue
		}
		title := strings.TrimSpace(res.LineupTitle)
		if title == "" {
			title = "(unknown lineup)"
		}
		row := byTitle[title]
		if row == nil {
			row = &SummaryLineup{LineupTitle: title}
			byTitle[title] = row
		}
		row.Successes++
		if res.ChannelMapRows > row.BestChannelMapRows {
			row.BestChannelMapRows = res.ChannelMapRows
		}
		if res.ChannelCount > row.BestChannelCount {
			row.BestChannelCount = res.ChannelCount
		}
		row.Targets = appendUnique(row.Targets, res.BaseURL)
		row.FriendlyNames = appendUnique(row.FriendlyNames, res.FriendlyName)
	}
	out := make([]SummaryLineup, 0, len(byTitle))
	for _, row := range byTitle {
		sort.Strings(row.Targets)
		sort.Strings(row.FriendlyNames)
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		leftScore := out[i].BestChannelMapRows
		if leftScore == 0 {
			leftScore = out[i].BestChannelCount
		}
		rightScore := out[j].BestChannelMapRows
		if rightScore == 0 {
			rightScore = out[j].BestChannelCount
		}
		if leftScore == rightScore {
			return out[i].LineupTitle < out[j].LineupTitle
		}
		return leftScore > rightScore
	})
	return out
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func appendTarget(targets []Target, seen map[string]struct{}, target Target) []Target {
	target.BaseURL = strings.TrimSpace(target.BaseURL)
	if target.BaseURL == "" {
		return targets
	}
	if _, ok := seen[target.BaseURL]; ok {
		return targets
	}
	seen[target.BaseURL] = struct{}{}
	targets = append(targets, target)
	return targets
}

func targetFriendlyName(prefix, cap string) string {
	if cap == "" {
		return strings.TrimSuffix(prefix, "-")
	}
	return strings.TrimSuffix(prefix, "-") + "-" + strings.TrimSpace(cap)
}

func fetchLineup(baseURL string) ([]HarvestedChannel, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base url required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	doc, err := hdhomerun.FetchLineupJSON(ctx, httpclient.WithTimeout(15*time.Second), baseURL)
	if err != nil {
		return nil, err
	}
	rows := make([]HarvestedChannel, 0, len(doc.Channels))
	for _, ch := range doc.Channels {
		rows = append(rows, HarvestedChannel{
			GuideNumber: strings.TrimSpace(ch.GuideNumber),
			GuideName:   strings.TrimSpace(ch.GuideName),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if strings.TrimSpace(rows[i].GuideNumber) == strings.TrimSpace(rows[j].GuideNumber) {
			return strings.TrimSpace(rows[i].GuideName) < strings.TrimSpace(rows[j].GuideName)
		}
		return strings.TrimSpace(rows[i].GuideNumber) < strings.TrimSpace(rows[j].GuideNumber)
	})
	return rows, nil
}

func targetDeviceID(prefix, cap string, seq int) string {
	base := strings.ToLower(strings.TrimSpace(prefix + cap))
	base = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(base, "")
	if len(base) > 8 {
		base = base[:8]
	}
	if base == "" {
		base = "harvest"
	}
	if len(base) < 8 {
		base = (base + "00000000")[:8]
	}
	n := seq % 100
	if cap != "" {
		if parsed, err := strconv.Atoi(cap); err == nil {
			n = parsed % 100
		}
	}
	return fmt.Sprintf("%s%02d", base[:6], n)
}

func appendUnique(in []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return in
	}
	for _, cur := range in {
		if cur == value {
			return in
		}
	}
	return append(in, value)
}

func providerAbsoluteURL(baseURL, path string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path = strings.TrimSpace(path)
	if path == "" {
		return baseURL
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}

func fetchProviderLineups(baseURL, version, token, country, postalCode string) ([]providerLineupEntry, error) {
	country, postalCode = normalizeProviderQuery(country, postalCode)
	u, err := url.Parse(providerAbsoluteURL(baseURL, "/lineups"))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("country", strings.TrimSpace(country))
	q.Set("postalCode", strings.TrimSpace(postalCode))
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	setProviderHeaders(req, version, token)
	resp, err := httpclient.WithTimeout(20 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider lineup query failed: %s", strings.TrimSpace(string(body)))
	}
	var doc providerLineupListResponse
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	return doc.MediaContainer.Lineup, nil
}

func fetchProviderLineupChannels(baseURL, version, token, path string) ([]HarvestedChannel, int, error) {
	req, err := http.NewRequest(http.MethodGet, providerAbsoluteURL(baseURL, path), nil)
	if err != nil {
		return nil, 0, err
	}
	setProviderHeaders(req, version, token)
	resp, err := httpclient.WithTimeout(20 * time.Second).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("provider lineup channels query failed: %s", strings.TrimSpace(string(body)))
	}
	var doc providerLineupChannelsResponse
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, 0, err
	}
	rows := make([]HarvestedChannel, 0, len(doc.MediaContainer.Channel))
	for _, ch := range doc.MediaContainer.Channel {
		name := strings.TrimSpace(firstNonEmptyProviderString(ch.Title, ch.CallSign, ch.AffiliateCallSign))
		rows = append(rows, HarvestedChannel{
			ChannelID:   strings.TrimSpace(ch.ID),
			GuideNumber: strings.TrimSpace(ch.VCN),
			GuideName:   name,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].GuideNumber == rows[j].GuideNumber {
			return rows[i].GuideName < rows[j].GuideName
		}
		return rows[i].GuideNumber < rows[j].GuideNumber
	})
	return rows, doc.MediaContainer.Size, nil
}

func setProviderHeaders(req *http.Request, version, token string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Provider-Version", strings.TrimSpace(version))
	if strings.TrimSpace(token) != "" {
		req.Header.Set("X-Plex-Token", strings.TrimSpace(token))
	}
}

func filterProviderLineups(in []providerLineupEntry, types []string, titleQuery string, limit int) []providerLineupEntry {
	typeSet := map[string]struct{}{}
	for _, typ := range types {
		typ = strings.ToLower(strings.TrimSpace(typ))
		if typ != "" {
			typeSet[typ] = struct{}{}
		}
	}
	titleQuery = strings.ToLower(strings.TrimSpace(titleQuery))
	out := make([]providerLineupEntry, 0, len(in))
	for _, row := range in {
		if len(typeSet) > 0 {
			if _, ok := typeSet[strings.ToLower(strings.TrimSpace(row.Type))]; !ok {
				continue
			}
		}
		if titleQuery != "" && !strings.Contains(strings.ToLower(strings.TrimSpace(row.Title)), titleQuery) {
			continue
		}
		out = append(out, row)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func firstNonEmptyProviderString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

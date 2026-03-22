package plexharvest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/plex"
)

type Target struct {
	BaseURL      string `json:"base_url"`
	Cap          string `json:"cap,omitempty"`
	FriendlyName string `json:"friendly_name"`
	DeviceID     string `json:"device_id"`
}

type Result struct {
	BaseURL        string   `json:"base_url"`
	Cap            string   `json:"cap,omitempty"`
	FriendlyName   string   `json:"friendly_name"`
	DeviceKey      string   `json:"device_key,omitempty"`
	DeviceUUID     string   `json:"device_uuid,omitempty"`
	DVRKey         int      `json:"dvr_key,omitempty"`
	DVRUUID        string   `json:"dvr_uuid,omitempty"`
	LineupTitle    string   `json:"lineup_title,omitempty"`
	LineupURL      string   `json:"lineup_url,omitempty"`
	LineupIDs      []string `json:"lineup_ids,omitempty"`
	ChannelMapRows int      `json:"channelmap_rows,omitempty"`
	Activated      int      `json:"activated,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type SummaryLineup struct {
	LineupTitle        string   `json:"lineup_title"`
	Targets            []string `json:"targets,omitempty"`
	FriendlyNames      []string `json:"friendly_names,omitempty"`
	Successes          int      `json:"successes"`
	BestChannelMapRows int      `json:"best_channelmap_rows"`
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

var (
	registerTunerViaAPI = plex.RegisterTunerViaAPI
	createDVRViaAPI     = plex.CreateDVRViaAPI
	reloadGuideAPI      = plex.ReloadGuideAPI
	getChannelMap       = plex.GetChannelMap
	activateChannelsAPI = plex.ActivateChannelsAPI
	listDVRsAPI         = plex.ListDVRsAPI
)

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
		report.Results = append(report.Results, res)
	}
	report.Lineups = buildSummary(report.Results)
	return report
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
		if out[i].BestChannelMapRows == out[j].BestChannelMapRows {
			return out[i].LineupTitle < out[j].LineupTitle
		}
		return out[i].BestChannelMapRows > out[j].BestChannelMapRows
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

package plex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// VODBackfillConfig configures a series backfill run.
type VODBackfillConfig struct {
	CatalogIn   string
	CatalogOut  string
	ProgressOut string
	Workers     int
	Timeout     time.Duration
	Limit       int
	RetryFrom   string // path to previous progress JSON; only retry failed SIDs
}

// VODBackfillStats is emitted to stdout as JSON during and after the run.
type VODBackfillStats struct {
	APIBase           string           `json:"api_base"`
	Movies            int              `json:"movies"`
	SeriesTotal       int              `json:"series_total"`
	SeriesProcessing  int              `json:"series_processing"`
	Workers           int              `json:"workers"`
	Done              int              `json:"done"`
	OK                int              `json:"ok"`
	Fail              int              `json:"fail"`
	Eps               int              `json:"eps"`
	Seasons           int              `json:"seasons"`
	StartedAt         int64            `json:"started_at"`
	ElapsedS          float64          `json:"elapsed_s,omitempty"`
	CatalogOut        string           `json:"catalog_out,omitempty"`
	SeriesWithSeasons int              `json:"series_with_seasons,omitempty"`
	Mode              string           `json:"mode,omitempty"`
	FailExamples      []map[string]any `json:"fail_examples,omitempty"`
}

// RunVODBackfill refetches per-series episode info and writes a repaired catalog.
// progressFn is called with each progress snapshot (encoded to JSON by caller).
func RunVODBackfill(cfg VODBackfillConfig, progressFn func(VODBackfillStats)) error {
	raw, err := os.ReadFile(cfg.CatalogIn)
	if err != nil {
		return fmt.Errorf("read catalog: %w", err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return fmt.Errorf("parse catalog: %w", err)
	}

	apiBase, streamBase, user, pw, err := deriveCreds(catalog)
	if err != nil {
		return err
	}

	allSeries := toSlice(catalog["series"])
	series := allSeries
	if cfg.Limit > 0 && len(series) > cfg.Limit {
		series = series[:cfg.Limit]
	}

	var retrySIDs map[string]bool
	if cfg.RetryFrom != "" {
		prev, err := os.ReadFile(cfg.RetryFrom)
		if err != nil {
			return fmt.Errorf("read retry-from: %w", err)
		}
		var p map[string]any
		if err := json.Unmarshal(prev, &p); err != nil {
			return fmt.Errorf("parse retry-from: %w", err)
		}
		retrySIDs = map[string]bool{}
		for _, x := range toSlice(p["fail_examples"]) {
			m, _ := x.(map[string]any)
			if sid, _ := m["sid"].(string); sid != "" {
				retrySIDs[sid] = true
			}
		}
	}

	// Build work list
	type item struct {
		idx int
		row map[string]any
	}
	var items []item
	for i, s := range allSeries {
		m, _ := s.(map[string]any)
		if m == nil {
			continue
		}
		sid := fmt.Sprintf("%v", m["id"])
		if retrySIDs != nil && !retrySIDs[sid] {
			continue
		}
		if cfg.Limit > 0 && len(items) >= cfg.Limit {
			break
		}
		items = append(items, item{i, m})
	}

	start := time.Now()
	stats := VODBackfillStats{
		APIBase:          apiBase,
		Movies:           len(toSlice(catalog["movies"])),
		SeriesTotal:      len(allSeries),
		SeriesProcessing: len(items),
		Workers:          cfg.Workers,
		StartedAt:        start.Unix(),
		Mode: func() string {
			if retrySIDs != nil {
				return "retry"
			}
			return "full"
		}(),
	}

	progressFn(stats)

	type result struct {
		idx  int
		row  map[string]any
		meta map[string]any
	}

	outSeries := make([]any, len(allSeries))
	copy(outSeries, allSeries)

	results := make(chan result, cfg.Workers*2)
	sem := make(chan struct{}, cfg.Workers)

	var wg sync.WaitGroup
	for _, it := range items {
		it := it
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			row, meta := rebuildSeries(it.row, apiBase, streamBase, user, pw, cfg.Timeout)
			results <- result{it.idx, row, meta}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var failExamples []map[string]any
	for r := range results {
		outSeries[r.idx] = r.row
		stats.Done++
		if ok, _ := r.meta["ok"].(bool); ok {
			stats.OK++
			if eps, _ := r.meta["eps"].(int); eps > 0 {
				stats.Eps += eps
			}
			if sn, _ := r.meta["seasons"].(int); sn > 0 {
				stats.Seasons += sn
			}
		} else {
			stats.Fail++
			if len(failExamples) < 50 {
				failExamples = append(failExamples, r.meta)
			}
		}
		if stats.Done%100 == 0 || stats.Done == len(items) {
			snap := stats
			snap.ElapsedS = time.Since(start).Seconds()
			snap.FailExamples = failExamples
			progressFn(snap)
		}
	}

	out := make(map[string]any, len(catalog))
	for k, v := range catalog {
		out[k] = v
	}
	out["series"] = outSeries

	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal output: %w", err)
	}
	if err := os.WriteFile(cfg.CatalogOut, data, 0644); err != nil {
		return fmt.Errorf("write catalog: %w", err)
	}

	seriesWithSeasons := 0
	for _, s := range outSeries {
		m, _ := s.(map[string]any)
		if m == nil {
			continue
		}
		if sl := toSlice(m["seasons"]); len(sl) > 0 {
			seriesWithSeasons++
		}
	}

	stats.ElapsedS = time.Since(start).Seconds()
	stats.CatalogOut = cfg.CatalogOut
	stats.SeriesWithSeasons = seriesWithSeasons
	stats.FailExamples = failExamples
	progressFn(stats)
	return nil
}

func deriveCreds(catalog map[string]any) (apiBase, streamBase, user, pw string, err error) {
	movies := toSlice(catalog["movies"])
	if len(movies) == 0 {
		return "", "", "", "", fmt.Errorf("catalog has no movies; cannot derive provider credentials")
	}
	m, _ := movies[0].(map[string]any)
	if m == nil {
		return "", "", "", "", fmt.Errorf("first movie entry is not a map")
	}
	murl := stringVal(m, "stream_url", "streamURL")
	if murl == "" {
		return "", "", "", "", fmt.Errorf("first movie has no stream_url")
	}
	u, err := url.Parse(murl)
	if err != nil {
		return "", "", "", "", fmt.Errorf("parse movie url: %w", err)
	}
	parts := []string{}
	for _, p := range strings.Split(u.Path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	idx := -1
	for i, p := range parts {
		if p == "movie" {
			idx = i
			break
		}
	}
	if idx < 0 || idx+2 >= len(parts) {
		return "", "", "", "", fmt.Errorf("cannot derive creds from movie url %q", murl)
	}
	user = parts[idx+1]
	pw = parts[idx+2]
	apiBase = u.Scheme + "://" + u.Host
	streamBase = strings.TrimRight(apiBase, "/")
	return apiBase, streamBase, user, pw, nil
}

func rebuildSeries(seriesRow map[string]any, apiBase, streamBase, user, pw string, timeout time.Duration) (map[string]any, map[string]any) {
	sid := fmt.Sprintf("%v", seriesRow["id"])
	if sid == "" || sid == "<nil>" {
		row := clone(seriesRow)
		row["seasons"] = []any{}
		return row, map[string]any{"ok": false, "sid": "", "err": "missing id"}
	}

	q := url.Values{
		"username":  {user},
		"password":  {pw},
		"action":    {"get_series_info"},
		"series_id": {sid},
	}
	u := apiBase + "/player_api.php?" + q.Encode()
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(u)
	if err != nil {
		row := clone(seriesRow)
		row["seasons"] = []any{}
		return row, map[string]any{"ok": false, "sid": sid, "err": err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var info map[string]any
	if err := json.Unmarshal(body, &info); err != nil {
		row := clone(seriesRow)
		row["seasons"] = []any{}
		return row, map[string]any{"ok": false, "sid": sid, "err": err.Error()}
	}

	eps := parseXtreamEpisodes(info["episodes"])
	bySeason := map[int][]map[string]any{}
	for _, ep := range eps {
		epID := fmt.Sprintf("%v", ep["id"])
		sn, _ := toInt(ep["season_num"])
		en, _ := toInt(ep["episode_num"])
		if epID == "" || epID == "<nil>" || sn <= 0 || en <= 0 {
			continue
		}
		ext := "mp4"
		if e, ok := ep["container_extension"].(string); ok && strings.TrimSpace(e) != "" {
			ext = strings.TrimSpace(e)
		}
		streamURL := streamBase + "/series/" + user + "/" + pw + "/" + epID + "." + ext
		bySeason[sn] = append(bySeason[sn], map[string]any{
			"id":          epID,
			"season_num":  sn,
			"episode_num": en,
			"title":       stringVal(ep, "title"),
			"airdate":     stringVal(ep, "releaseDate"),
			"stream_url":  streamURL,
		})
	}

	sns := make([]int, 0, len(bySeason))
	for s := range bySeason {
		sns = append(sns, s)
	}
	sort.Ints(sns)

	totalEps := 0
	var seasons []any
	for _, sn := range sns {
		eps := bySeason[sn]
		sort.Slice(eps, func(i, j int) bool {
			ei, _ := toInt(eps[i]["episode_num"])
			ej, _ := toInt(eps[j]["episode_num"])
			if ei != ej {
				return ei < ej
			}
			return fmt.Sprintf("%v", eps[i]["id"]) < fmt.Sprintf("%v", eps[j]["id"])
		})
		totalEps += len(eps)
		seasons = append(seasons, map[string]any{"number": sn, "episodes": eps})
	}

	row := clone(seriesRow)
	row["seasons"] = seasons
	return row, map[string]any{"ok": true, "sid": sid, "seasons": len(seasons), "eps": totalEps}
}

func parseXtreamEpisodes(v any) []map[string]any {
	var out []map[string]any
	switch val := v.(type) {
	case map[string]any:
		for sk, mv := range val {
			switch mval := mv.(type) {
			case map[string]any:
				out = append(out, mval)
			case []any:
				for _, item := range mval {
					ep, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if ep["season_num"] == nil {
						ep = clone(ep)
						if n, err := strconv.Atoi(sk); err == nil {
							ep["season_num"] = n
						}
					}
					out = append(out, ep)
				}
			}
		}
	case []any:
		for _, item := range val {
			if ep, ok := item.(map[string]any); ok {
				out = append(out, ep)
			}
		}
	}
	return out
}

// helpers
func toSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func clone(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func stringVal(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case float64:
		return int(val), true
	case string:
		n, err := strconv.Atoi(val)
		return n, err == nil
	}
	return 0, false
}

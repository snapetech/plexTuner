package tuner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

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

func sanitizeDiagRunID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return sanitizeFileToken(value)
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
	runID := sanitizeDiagRunID(env["RUN_ID"])
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
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		stateFile := strings.TrimSpace(s.RecorderStateFile)
		if stateFile == "" {
			stateFile = strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE"))
		}
		if stateFile == "" {
			writeServerJSONError(w, http.StatusServiceUnavailable, "recorder state unavailable")
			return
		}
		rep, err := LoadCatchupRecorderReport(stateFile, streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 10))
		if err != nil {
			writeServerJSONError(w, http.StatusBadGateway, "load recorder report failed")
			return
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode recorder report")
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
			if !operatorUIAllowed(w, r) {
				return
			}
			body, err := json.MarshalIndent(s.reloadRecordingRules(), "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode recording rules")
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
				writeServerJSONError(w, http.StatusBadRequest, "invalid json")
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
				writeServerJSONError(w, http.StatusBadRequest, "unsupported action")
				return
			}
			saved, err := s.saveRecordingRules(rules)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save recording rules failed")
				return
			}
			body, err := json.MarshalIndent(saved, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode recording rules")
				return
			}
			_, _ = w.Write(body)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func (s *Server) serveRecordingRulePreview() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if s.xmltv == nil {
			writeServerJSONError(w, http.StatusServiceUnavailable, "xmltv unavailable")
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
			writeServerJSONError(w, http.StatusBadGateway, "recording rule preview failed")
			return
		}
		body, err := json.MarshalIndent(buildRecordingRulePreview(s.reloadRecordingRules(), preview), "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode recording rule preview")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveRecordingHistory() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		stateFile := strings.TrimSpace(s.RecorderStateFile)
		if stateFile == "" {
			stateFile = strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE"))
		}
		if stateFile == "" {
			writeServerJSONError(w, http.StatusServiceUnavailable, "recorder state unavailable")
			return
		}
		report, err := LoadCatchupRecorderReport(stateFile, streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 25))
		if err != nil {
			writeServerJSONError(w, http.StatusBadGateway, "load recorder history failed")
			return
		}
		body, err := json.MarshalIndent(buildRecordingRuleHistory(s.reloadRecordingRules(), report), "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode recording history")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveHlsMuxWebDemo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
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
			writeMethodNotAllowedJSON(w, http.MethodPost)
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
			writeServerJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.SegB64))
		if err != nil {
			writeServerJSONError(w, http.StatusBadRequest, "invalid base64")
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
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		deviceID := s.DeviceID
		if deviceID == "" {
			deviceID = strings.TrimSpace(os.Getenv("IPTV_TUNERR_DEVICE_ID"))
		}
		if deviceID == "" {
			deviceID = strings.TrimSpace(os.Getenv("HOSTNAME"))
		}
		if deviceID == "" {
			deviceID = "iptvtunerr01"
		}
		friendlyName := strings.TrimSpace(s.FriendlyName)
		if friendlyName == "" {
			friendlyName = strings.TrimSpace(os.Getenv("IPTV_TUNERR_FRIENDLY_NAME"))
		}
		if friendlyName == "" {
			friendlyName = strings.TrimSpace(os.Getenv("HOSTNAME"))
		}
		if friendlyName == "" {
			friendlyName = "IPTV Tunerr"
		}
		deviceXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
    <friendlyName>%s</friendlyName>
    <manufacturer>IPTV Tunerr</manufacturer>
    <modelName>IPTV Tunerr</modelName>
    <UDN>uuid:%s</UDN>
  </device>
</root>`, xmlEscapeText(friendlyName), xmlEscapeText(deviceID))
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(deviceXML))
	})
}

package tuner

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestRecordingRulesFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	saved, err := saveRecordingRulesFile(path, RecordingRuleset{
		Rules: []RecordingRule{{
			Name:              "News Now",
			Enabled:           true,
			IncludeLanes:      []string{"news"},
			IncludeChannelIDs: []string{"ch-1", "ch-1"},
			TitleContains:     []string{"Morning", "Morning"},
		}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(saved.Rules) != 1 || saved.Rules[0].ID == "" {
		t.Fatalf("saved=%+v", saved)
	}
	loaded, err := loadRecordingRulesFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Rules[0].ID != saved.Rules[0].ID {
		t.Fatalf("loaded id=%q want %q", loaded.Rules[0].ID, saved.Rules[0].ID)
	}
	if len(loaded.Rules[0].IncludeChannelIDs) != 1 {
		t.Fatalf("dedupe failed: %+v", loaded.Rules[0])
	}
}

func TestServer_recordingRulesEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	s := &Server{RecordingRulesFile: path}
	body := bytes.NewBufferString(`{"action":"upsert","rule":{"name":"Sports Prime","enabled":true,"include_lanes":["sports"],"title_contains":["Live"]}}`)
	req := httptest.NewRequest(http.MethodPost, "/recordings/rules.json", body)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveRecordingRules().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var set RecordingRuleset
	if err := json.Unmarshal(w.Body.Bytes(), &set); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(set.Rules) != 1 || set.Rules[0].Name != "Sports Prime" {
		t.Fatalf("rules=%+v", set.Rules)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Contains(data, []byte("Sports Prime")) {
		t.Fatalf("saved file missing rule: %s", data)
	}
}

func TestServer_recordingRulePreview(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(10 * time.Minute).Format("20060102150405 +0000")
	stop := now.Add(70 * time.Minute).Format("20060102150405 +0000")
	s := &Server{
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "ch-1", GuideNumber: "101", GuideName: "News One"},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>News One</display-name></channel>
  <programme channel="101" start="` + start + `" stop="` + stop + `">
    <title>Morning Live</title>
    <category>News</category>
  </programme>
</tv>`),
		},
		RecordingRules: RecordingRuleset{
			Rules: []RecordingRule{{
				ID:            "news-live",
				Name:          "News Live",
				Enabled:       true,
				TitleContains: []string{"Morning"},
			}},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/recordings/rules/preview.json?horizon=24h&limit=10", nil)
	w := httptest.NewRecorder()
	s.serveRecordingRulePreview().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var report RecordingRulePreviewReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(report.Matches) != 1 || report.Matches[0].MatchCount != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func TestServer_recordingHistory(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "recorder-state.json")
	state := CatchupRecorderState{
		Completed: []CatchupRecorderItem{{
			ChannelID:     "ch-1",
			GuideNumber:   "101",
			Title:         "Morning Live",
			Lane:          "news",
			PublishedPath: filepath.Join(dir, "news.ts"),
		}},
		Failed: []CatchupRecorderItem{{
			ChannelID:   "ch-2",
			GuideNumber: "202",
			Title:       "Late Sports",
			Lane:        "sports",
			Status:      "interrupted",
		}},
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := &Server{
		RecorderStateFile: stateFile,
		RecordingRules: RecordingRuleset{
			Rules: []RecordingRule{{
				ID:                  "news",
				Name:                "News",
				Enabled:             true,
				IncludeLanes:        []string{"news"},
				TitleContains:       []string{"Morning"},
				IncludeGuideNumbers: []string{"101"},
			}},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/recordings/history.json?limit=10", nil)
	w := httptest.NewRecorder()
	s.serveRecordingHistory().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var report RecordingRuleHistoryReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(report.Matches) != 1 || report.Matches[0].CompletedCount != 1 {
		t.Fatalf("matches=%+v", report.Matches)
	}
	if report.Unmatched.FailedCount != 1 || report.Unmatched.InterruptedCount != 1 {
		t.Fatalf("unmatched=%+v", report.Unmatched)
	}
}

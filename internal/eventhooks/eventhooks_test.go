package eventhooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAndDispatch(t *testing.T) {
	delivered := make(chan Event, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var evt Event
		if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		delivered <- evt
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	cfgPath := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(cfgPath, []byte(`{"webhooks":[{"name":"test","url":"`+srv.URL+`","events":["lineup.updated"]}]}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	dispatcher, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load dispatcher: %v", err)
	}
	dispatcher.Dispatch("lineup.updated", "server", map[string]any{"channels": 3})

	select {
	case evt := <-delivered:
		if evt.Name != "lineup.updated" {
			t.Fatalf("unexpected event name %q", evt.Name)
		}
		if evt.Source != "server" {
			t.Fatalf("unexpected source %q", evt.Source)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event delivery")
	}

	report := dispatcher.Report()
	if !report.Enabled {
		t.Fatal("dispatcher report should be enabled")
	}
	if report.TotalHooks != 1 {
		t.Fatalf("report total hooks = %d; want 1", report.TotalHooks)
	}
}

func TestHookMatchesWildcard(t *testing.T) {
	if !hookMatches(Hook{Events: []string{"*"}}, "stream.finished") {
		t.Fatal("wildcard hook should match")
	}
	if hookMatches(Hook{Events: []string{"lineup.updated"}}, "stream.finished") {
		t.Fatal("non-matching hook should not match")
	}
}

func TestReportRedactsWebhookURLsAndHeaders(t *testing.T) {
	dispatcher := &Dispatcher{
		hooks: []Hook{{
			Name:   "secret-hook",
			URL:    "https://user:pass@example.test/hook?token=abc123&ok=1",
			Events: []string{"stream.finished"},
			Headers: map[string]string{
				"Authorization": "Bearer secret-token",
				"X-Api-Key":     "secret-api-key",
				"X-Trace":       "trace-123",
				"X-Callback":    "https://example.test/callback?password=secret",
			},
		}},
		recent: []Delivery{{
			EventName:   "stream.finished",
			EventID:     "evt-000001",
			HookName:    "secret-hook",
			URL:         "https://user:pass@example.test/hook?token=abc123",
			DeliveredAt: "2026-05-14T00:00:00Z",
		}},
	}

	report := dispatcher.Report()
	if len(report.Hooks) != 1 {
		t.Fatalf("hooks=%d", len(report.Hooks))
	}
	hook := report.Hooks[0]
	joined, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	for _, secret := range []string{"user:pass", "abc123", "secret-token", "secret-api-key", "password=secret"} {
		if strings.Contains(string(joined), secret) {
			t.Fatalf("report leaked %q in %s", secret, joined)
		}
	}
	if hook.Headers["Authorization"] != "[REDACTED]" || hook.Headers["X-Api-Key"] != "[REDACTED]" || hook.Headers["X-Callback"] != "[REDACTED]" {
		t.Fatalf("headers not redacted: %#v", hook.Headers)
	}
	if hook.Headers["X-Trace"] != "trace-123" {
		t.Fatalf("non-sensitive header redacted: %#v", hook.Headers)
	}
	if strings.Contains(hook.URL, "?") {
		t.Fatalf("webhook report should not expose raw query strings: %q", hook.URL)
	}
}

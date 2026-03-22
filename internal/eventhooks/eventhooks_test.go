package eventhooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

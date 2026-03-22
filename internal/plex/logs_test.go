package plex

import "testing"

func TestParsePlexLogRequest(t *testing.T) {
	line := `Mar 22 13:17:43 DEBUG - Request: [192.168.1.2:6999] POST /livetv/dvrs/711/channels/3935/tune (13 live)`
	method, path, ok := parsePlexLogRequest(line)
	if !ok {
		t.Fatal("expected match")
	}
	if method != "POST" {
		t.Fatalf("method = %q", method)
	}
	if path != "/livetv/dvrs/711/channels/3935/tune" {
		t.Fatalf("path = %q", path)
	}
	if got := normalizePlexPath(path); got != "/livetv/dvrs/:id/channels/:id/tune" {
		t.Fatalf("normalized path = %q", got)
	}
}

func TestParsePlexLogRequestIgnoresUnrelatedPaths(t *testing.T) {
	line := `Mar 22 13:17:43 DEBUG - Request: [192.168.1.2:6999] GET /library/sections (13 live)`
	if _, _, ok := parsePlexLogRequest(line); ok {
		t.Fatal("expected unrelated path to be ignored")
	}
}

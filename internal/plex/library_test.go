package plex

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetLibrarySectionItemCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/library/sections/42/all" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("X-Plex-Container-Size"); got != "0" {
			t.Fatalf("X-Plex-Container-Size=%q", got)
		}
		if got := r.URL.Query().Get("X-Plex-Token"); got != "token" {
			t.Fatalf("token=%q", got)
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0" totalSize="123"></MediaContainer>`))
	}))
	defer srv.Close()

	count, err := GetLibrarySectionItemCount(srv.URL, "token", "42")
	if err != nil {
		t.Fatalf("GetLibrarySectionItemCount: %v", err)
	}
	if count != 123 {
		t.Fatalf("count=%d", count)
	}
}

func TestGetLibrarySectionItemCountRequiresKey(t *testing.T) {
	_, err := GetLibrarySectionItemCount("http://plex.example", "token", "")
	if err == nil || !strings.Contains(err.Error(), "library section key required") {
		t.Fatalf("err=%v", err)
	}
}

func TestGetLibrarySectionItemTitles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/library/sections/42/all" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("X-Plex-Container-Size"); got != "2" {
			t.Fatalf("X-Plex-Container-Size=%q", got)
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<MediaContainer size="2" totalSize="2">
  <Video title="Zulu" titleSort="Alpha" />
  <Directory title="Bravo" />
</MediaContainer>`))
	}))
	defer srv.Close()

	titles, err := GetLibrarySectionItemTitles(srv.URL, "token", "42", 2)
	if err != nil {
		t.Fatalf("GetLibrarySectionItemTitles: %v", err)
	}
	if len(titles) != 2 || titles[0] != "Alpha" || titles[1] != "Bravo" {
		t.Fatalf("titles=%v", titles)
	}
}

package indexer

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApiGet_retry429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_info":{}}`))
	}))
	defer srv.Close()

	body, err := apiGet(&http.Client{}, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Error("expected body")
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestIndexFromPlayerAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("action") == "get_live_streams" {
			w.Write([]byte(`[
				{"stream_id": 1, "name": "BBC One", "epg_channel_id": "bbc1"},
				{"stream_id": 2, "name": "ITV", "epg_channel_id": 2}
			]`))
			return
		}
		w.Write([]byte(`{"user_info":{"username":"u","password":"p"}}`))
	}))
	defer srv.Close()

	movies, series, live, err := IndexFromPlayerAPI(srv.URL, "u", "p", "m3u8", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 0 || len(series) != 0 {
		t.Errorf("movies=%d series=%d (want 0)", len(movies), len(series))
	}
	if len(live) != 2 {
		t.Fatalf("live channels: %d, want 2", len(live))
	}
	if live[0].GuideName != "BBC One" || live[0].TVGID != "bbc1" {
		t.Errorf("live[0]: name=%q tvgid=%q", live[0].GuideName, live[0].TVGID)
	}
	if live[0].StreamURL != srv.URL+"/live/u/p/1.m3u8" {
		t.Errorf("live[0].StreamURL = %q", live[0].StreamURL)
	}
	if !live[0].EPGLinked {
		t.Error("live[0] should be EPG-linked")
	}
	if live[1].GuideName != "ITV" {
		t.Errorf("live[1].GuideName = %q", live[1].GuideName)
	}
}

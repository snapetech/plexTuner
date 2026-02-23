package indexer

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestApiGet_retry429 verifies that apiGet retries on 429 and eventually succeeds.
func TestApiGet_retry429(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 10 * time.Second
	body, err := apiGet(client, server.URL)
	if err != nil {
		t.Fatalf("apiGet: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts; got %d", attempts)
	}
}

// TestApiGetDecode_retry429 verifies that apiGetDecode retries on 429 and decodes on success.
func TestApiGetDecode_retry429(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"live":1}`))
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 10 * time.Second
	var out struct {
		Live int `json:"live"`
	}
	if err := apiGetDecode(client, server.URL, &out); err != nil {
		t.Fatalf("apiGetDecode: %v", err)
	}
	if out.Live != 1 {
		t.Errorf("out.Live = %d", out.Live)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts; got %d", attempts)
	}
}

// TestIndexFromPlayerAPI_liveOnly mocks the player_api (auth + get_live_streams) and checks live channels.
func TestIndexFromPlayerAPI_liveOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		action := r.URL.Query().Get("action")
		if action == "" {
			// auth (no action)
			w.Write([]byte(`{"user_info":{"username":"u","password":"p"},"server_info":null}`))
			return
		}
		if action == "get_live_streams" {
			w.Write([]byte(`[
			{"stream_id": 1, "name": "Channel One", "epg_channel_id": "1", "stream_icon": "", "category_id": "0"},
			{"stream_id": 2, "name": "Channel Two", "epg_channel_id": "2", "stream_icon": "", "category_id": "0"}
			]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	baseURL := server.URL
	movies, series, live, err := IndexFromPlayerAPI(baseURL, "user", "pass", "m3u8", true, nil, server.Client())
	if err != nil {
		t.Fatalf("IndexFromPlayerAPI: %v", err)
	}
	if len(movies) != 0 || len(series) != 0 {
		t.Errorf("liveOnly: expected no movies/series; got %d movies, %d series", len(movies), len(series))
	}
	if len(live) != 2 {
		t.Fatalf("expected 2 live; got %d", len(live))
	}
	if live[0].GuideName != "Channel One" || live[1].GuideName != "Channel Two" {
		t.Errorf("live = %+v", live)
	}
}

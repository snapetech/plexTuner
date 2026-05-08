package webui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// v2Vods proxies VOD / series data from the tuner's Xtream player API.
// GET /api/v2/vods?kind=movies|series
//
// It reads xtream.user and xtream.pass from kv_settings to authenticate
// against the tuner. If the store is not configured, returns 503.
func (s *Server) v2Vods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	base := strings.TrimRight(strings.TrimSpace(s.tunerBase), "/")
	if base == "" {
		writeError(w, http.StatusServiceUnavailable, "tuner base not configured")
		return
	}

	user, pass := s.vodCreds()
	if user == "" {
		writeError(w, http.StatusUnprocessableEntity,
			"xtream credentials not configured — set xtream.user and xtream.pass in Settings → Provider")
		return
	}

	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	action := "get_vod_streams"
	if kind == "series" {
		action = "get_series"
	} else if kind == "categories" {
		action = "get_vod_categories"
	} else if kind == "series-categories" {
		action = "get_series_categories"
	}

	apiURL := fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=%s",
		base,
		url.QueryEscape(user),
		url.QueryEscape(pass),
		action,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "tuner request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MB cap
	if err != nil {
		writeError(w, http.StatusBadGateway, "read tuner response: "+err.Error())
		return
	}

	// Validate it's JSON before proxying
	if !json.Valid(body) {
		writeError(w, http.StatusBadGateway, "tuner returned non-JSON response (check credentials?)")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) vodCreds() (user, pass string) {
	if s.store == nil {
		return "", ""
	}
	u, _, _ := s.store.GetSetting("xtream.user")
	p, _, _ := s.store.GetSetting("xtream.pass")
	return strings.TrimSpace(u), strings.TrimSpace(p)
}

package webui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/store"
)

// GET /api/v2/stats/active-streams
// Proxies the tuner's active-streams report.
func (s *Server) v2ActiveStreams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	base := strings.TrimRight(s.tunerBase, "/")
	if base == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"in_use":       0,
			"tuner_limit":  0,
			"active":       []any{},
		})
		return
	}
	resp, err := http.Get(base + "/api/debug/active-streams.json")
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("active streams: %v", err))
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// POST /api/v2/stats/stream-stop
// Proxies a force-stop request to the tuner.
func (s *Server) v2StreamStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	base := strings.TrimRight(s.tunerBase, "/")
	if base == "" {
		writeError(w, http.StatusServiceUnavailable, "tuner not configured")
		return
	}
	body, _ := io.ReadAll(r.Body)
	req, err := http.NewRequest(http.MethodPost, base+"/api/ops/actions/stream-stop", strings.NewReader(string(body)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(out)
}

// GET /api/v2/stats/system-events?level=&source=&limit=
func (s *Server) v2SystemEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireStore(w) {
		return
	}
	opts := store.SystemEventListOpts{
		Level:  r.URL.Query().Get("level"),
		Source: r.URL.Query().Get("source"),
		Limit:  qInt(r, "limit", 200),
	}
	events, err := s.store.ListSystemEvents(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// GET /api/v2/connections
// POST /api/v2/connections
func (s *Server) v2Connections(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListEventHooks()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var in store.EventHookInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if in.Name == "" || in.Target == "" {
			writeError(w, http.StatusBadRequest, "name and target required")
			return
		}
		h, err := s.store.CreateEventHook(in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, h)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// PATCH /api/v2/connections/:id
// DELETE /api/v2/connections/:id
func (s *Server) v2ConnectionItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/connections/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var in store.EventHookInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h, err := s.store.UpdateEventHook(id, in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, h)

	case http.MethodDelete:
		if err := s.store.DeleteEventHook(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

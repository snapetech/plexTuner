package webui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/store"
)

// GET /api/v2/recordings?status=
// POST /api/v2/recordings
func (s *Server) v2Recordings(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		list, err := s.store.ListRecordings(status)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var in store.RecordingInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if in.Title == "" || in.StartAt == "" || in.EndAt == "" {
			writeError(w, http.StatusBadRequest, "title, start_at, end_at required")
			return
		}
		rec, err := s.store.CreateRecording(in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, rec)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// PATCH /api/v2/recordings/:id
// DELETE /api/v2/recordings/:id
func (s *Server) v2RecordingItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/recordings/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var body struct {
			Status   string `json:"status"`
			FilePath string `json:"file_path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.UpdateRecordingStatus(id, body.Status, body.FilePath); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		rec, _ := s.store.GetRecording(id)
		writeJSON(w, http.StatusOK, rec)

	case http.MethodDelete:
		if err := s.store.DeleteRecording(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET /api/v2/recording-rules
// POST /api/v2/recording-rules
func (s *Server) v2RecordingRules(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListRecordingRules()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var in store.RecordingRuleInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if in.Title == "" || in.StartTime == "" || in.EndTime == "" {
			writeError(w, http.StatusBadRequest, "title, start_time, end_time required")
			return
		}
		rr, err := s.store.CreateRecordingRule(in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, rr)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// PATCH /api/v2/recording-rules/:id
// DELETE /api/v2/recording-rules/:id
func (s *Server) v2RecordingRuleItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/recording-rules/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var in store.RecordingRuleInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rr, err := s.store.UpdateRecordingRule(id, in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rr)

	case http.MethodDelete:
		if err := s.store.DeleteRecordingRule(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// pathSuffixAfterID is already in apiv2.go — reuse pathSuffix helper.
var _ = strings.TrimPrefix // ensure strings is used

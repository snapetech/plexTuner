package webui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/store"
)

// GET /api/v2/epg-accounts
// POST /api/v2/epg-accounts
func (s *Server) v2EPGAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListEPGAccounts()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		// preview sub-path is served at /api/v2/epg-accounts/preview — unreachable here,
		// but guard against accidental routing.
		if r.URL.Path != "/api/v2/epg-accounts" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		var in store.EPGAccountInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if in.Name == "" {
			writeError(w, http.StatusBadRequest, "name required")
			return
		}
		a, err := s.store.CreateEPGAccount(in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, a)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// Routes for /api/v2/epg-accounts/:id and sub-paths.
func (s *Server) v2EPGAccountItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	// /api/v2/epg-accounts/preview
	if r.URL.Path == "/api/v2/epg-accounts/preview" {
		s.v2EPGPreview(w, r)
		return
	}

	id, ok := pathID(r, "/api/v2/epg-accounts/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	suffix := strings.Trim(pathSuffix(r, "/api/v2/epg-accounts/"), "/")

	switch suffix {
	case "":
		s.v2EPGAccountCRUD(w, r, id)
	case "refresh":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := s.store.TouchEPGAccountRefresh(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) v2EPGAccountCRUD(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case http.MethodGet:
		a, err := s.store.GetEPGAccount(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if a == nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, a)

	case http.MethodPatch:
		var in store.EPGAccountInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		a, err := s.store.UpdateEPGAccount(id, in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, a)

	case http.MethodDelete:
		if err := s.store.DeleteEPGAccount(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// POST /api/v2/epg-accounts/preview
// Accepts a dummy-pattern config JSON and returns rendered example output.
func (s *Server) v2EPGPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Config string `json:"config"`
		Sample string `json:"sample"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Stub: return config as-is until the dummy-pattern renderer is wired up.
	writeJSON(w, http.StatusOK, map[string]string{
		"preview": body.Config,
		"note":    "live preview not yet implemented",
	})
}

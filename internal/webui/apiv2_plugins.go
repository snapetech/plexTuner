package webui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/store"
)

// GET /api/v2/plugins
// POST /api/v2/plugins
func (s *Server) v2Plugins(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListPlugins()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var in store.PluginInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if in.Name == "" || in.Path == "" {
			writeError(w, http.StatusBadRequest, "name and path required")
			return
		}
		p, err := s.store.CreatePlugin(in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// PATCH /api/v2/plugins/:id          (update metadata or toggle enabled)
// DELETE /api/v2/plugins/:id
func (s *Server) v2PluginItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/plugins/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	suffix := strings.Trim(pathSuffix(r, "/api/v2/plugins/"), "/")

	// POST /api/v2/plugins/:id/enable  or  /disable
	if r.Method == http.MethodPost && (suffix == "enable" || suffix == "disable") {
		if err := s.store.SetPluginEnabled(id, suffix == "enable"); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		p, _ := s.store.GetPlugin(id)
		writeJSON(w, http.StatusOK, p)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var in store.PluginInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := s.store.UpdatePlugin(id, in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, p)

	case http.MethodDelete:
		if err := s.store.DeletePlugin(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

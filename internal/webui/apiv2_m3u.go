package webui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/store"
)

// GET /api/v2/m3u-accounts
// POST /api/v2/m3u-accounts
func (s *Server) v2M3UAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListM3UAccounts()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var in store.M3UAccountInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if in.Name == "" {
			writeError(w, http.StatusBadRequest, "name required")
			return
		}
		a, err := s.store.CreateM3UAccount(in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, a)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// Routes for /api/v2/m3u-accounts/:id and sub-paths.
func (s *Server) v2M3UAccountItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/m3u-accounts/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	suffix := strings.Trim(pathSuffix(r, "/api/v2/m3u-accounts/"), "/")

	switch suffix {
	case "":
		s.v2M3UAccountCRUD(w, r, id)
	case "refresh":
		s.v2M3UAccountRefresh(w, r, id)
	case "filters":
		s.v2M3UFilters(w, r, id)
	case "groups":
		s.v2M3UGroups(w, r, id)
	case "profiles":
		s.v2M3UAccountProfiles(w, r, id)
	default:
		// /api/v2/m3u-accounts/:id/groups/:gid
		if strings.HasPrefix(suffix, "groups/") {
			gid, err := parseInt64(strings.TrimPrefix(suffix, "groups/"))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid group id")
				return
			}
			s.v2M3UGroupItem(w, r, id, gid)
			return
		}
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) v2M3UAccountCRUD(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case http.MethodGet:
		a, err := s.store.GetM3UAccount(id)
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
		var in store.M3UAccountInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		a, err := s.store.UpdateM3UAccount(id, in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, a)

	case http.MethodDelete:
		if err := s.store.DeleteM3UAccount(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) v2M3UAccountRefresh(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.store.TouchM3UAccountRefresh(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) v2M3UFilters(w http.ResponseWriter, r *http.Request, accountID int64) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListM3UFilters(accountID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var filters []store.M3UFilter
		if err := json.NewDecoder(r.Body).Decode(&filters); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.ReplaceM3UFilters(accountID, filters); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		list, err := s.store.ListM3UFilters(accountID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) v2M3UGroups(w http.ResponseWriter, r *http.Request, accountID int64) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	list, err := s.store.ListM3UGroups(accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) v2M3UGroupItem(w http.ResponseWriter, r *http.Request, accountID, groupID int64) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var update map[string]any
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdateM3UGroup(groupID, update); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	groups, err := s.store.ListM3UGroups(accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, g := range groups {
		if g.ID == groupID {
			writeJSON(w, http.StatusOK, g)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) v2M3UAccountProfiles(w http.ResponseWriter, r *http.Request, accountID int64) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListM3UAccountProfiles(accountID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var body struct {
			Name       string `json:"name"`
			Username   string `json:"username"`
			Password   string `json:"password"`
			SearchPat  string `json:"search_pat"`
			ReplacePat string `json:"replace_pat"`
			MaxStreams int    `json:"max_streams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := s.store.CreateM3UAccountProfile(
			accountID, body.Name, body.Username, body.Password,
			body.SearchPat, body.ReplacePat, body.MaxStreams)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

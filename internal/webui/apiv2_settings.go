package webui

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/store"
)

// GET /api/v2/settings            → all kv_settings as object
// PATCH /api/v2/settings          → merge patch (only provided keys updated)
func (s *Server) v2Settings(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		m, err := s.store.AllSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, m)

	case http.MethodPatch:
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			writeError(w, http.StatusBadRequest, "read body")
			return
		}
		var patch map[string]string
		if err := json.Unmarshal(body, &patch); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := s.store.PatchSettings(patch); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		m, err := s.store.AllSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, m)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET  /api/v2/stream-profiles
// POST /api/v2/stream-profiles
func (s *Server) v2StreamProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListStreamProfiles()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var in store.StreamProfileInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		p, err := s.store.CreateStreamProfile(in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET    /api/v2/stream-profiles/:id
// PATCH  /api/v2/stream-profiles/:id
// DELETE /api/v2/stream-profiles/:id
func (s *Server) v2StreamProfileItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/stream-profiles/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		p, err := s.store.GetStreamProfile(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if p == nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, p)

	case http.MethodPatch:
		var in store.StreamProfileInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		p, err := s.store.UpdateStreamProfile(id, in)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if p == nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, p)

	case http.MethodDelete:
		if err := s.store.DeleteStreamProfile(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// jarCookie matches the persistentCookieJar JSON storage format.
type jarCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
}

// POST /api/v2/settings/cookie-jar — accepts a Netscape-format cookie file body,
// merges cookies into the persistent jar file (IPTV_TUNERR_COOKIE_JAR_FILE).
func (s *Server) v2CookieJar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	jarPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE"))
	if jarPath == "" {
		writeError(w, http.StatusUnprocessableEntity, "IPTV_TUNERR_COOKIE_JAR_FILE not set")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body")
		return
	}
	cookies, err := parseNetscapeJar(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "parse cookies: "+err.Error())
		return
	}
	if len(cookies) == 0 {
		writeError(w, http.StatusBadRequest, "no cookies found in input")
		return
	}

	// Load existing jar, merge, write back.
	saved := loadJarJSON(jarPath)
	defaultExpiry := time.Now().Add(7 * 24 * time.Hour).Unix()
	for _, c := range cookies {
		if c.Expires == 0 {
			c.Expires = defaultExpiry
		}
		host := strings.TrimPrefix(strings.ToLower(c.Domain), ".")
		if host == "" {
			continue
		}
		if saved[host] == nil {
			saved[host] = make(map[string]*jarCookie)
		}
		key := strings.Join([]string{c.Name, c.Domain, c.Path}, "\x00")
		saved[host][key] = c
	}
	if err := writeJarJSON(jarPath, saved); err != nil {
		writeError(w, http.StatusInternalServerError, "write jar: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"imported": len(cookies)})
}

func parseNetscapeJar(data []byte) ([]*jarCookie, error) {
	var out []*jarCookie
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain := strings.TrimSpace(fields[0])
		path := strings.TrimSpace(fields[2])
		secure := strings.EqualFold(strings.TrimSpace(fields[3]), "true")
		expiry, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		name := strings.TrimSpace(fields[5])
		value := strings.TrimSpace(fields[6])
		if domain == "" || name == "" {
			continue
		}
		out = append(out, &jarCookie{
			Name:    name,
			Value:   value,
			Domain:  strings.TrimPrefix(domain, "."),
			Path:    path,
			Secure:  secure,
			Expires: expiry,
		})
	}
	return out, sc.Err()
}

func loadJarJSON(path string) map[string]map[string]*jarCookie {
	out := make(map[string]map[string]*jarCookie)
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func writeJarJSON(path string, saved map[string]map[string]*jarCookie) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

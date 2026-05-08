package webui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const logosDirEnv = "IPTV_TUNERR_LOGOS_DIR"
const defaultLogosDir = "state/logos"
const maxLogoSize = 2 << 20 // 2 MB

func (s *Server) logosDir() string {
	if d := strings.TrimSpace(os.Getenv(logosDirEnv)); d != "" {
		return d
	}
	return defaultLogosDir
}

// GET /api/v2/logos
// POST /api/v2/logos  (multipart upload)
func (s *Server) v2Logos(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		logos, err := s.store.ListLogos()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Annotate with URL.
		for i := range logos {
			logos[i].URL = fmt.Sprintf("/api/v2/logos/%d/image", logos[i].ID)
		}
		writeJSON(w, http.StatusOK, logos)

	case http.MethodPost:
		if err := r.ParseMultipartForm(maxLogoSize); err != nil {
			writeError(w, http.StatusBadRequest, "multipart parse: "+err.Error())
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "file field required")
			return
		}
		defer file.Close()

		ct := header.Header.Get("Content-Type")
		if ct == "" {
			ct = "image/png"
		}
		safe := filepath.Base(header.Filename)
		if safe == "" || safe == "." {
			writeError(w, http.StatusBadRequest, "invalid filename")
			return
		}
		dir := s.logosDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, "mkdir: "+err.Error())
			return
		}
		dest := filepath.Join(dir, safe)
		f, err := os.Create(dest)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create: "+err.Error())
			return
		}
		n, err := io.Copy(f, file)
		f.Close()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "write: "+err.Error())
			return
		}
		logo, err := s.store.UpsertLogo(safe, ct, n)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		logo.URL = fmt.Sprintf("/api/v2/logos/%d/image", logo.ID)
		writeJSON(w, http.StatusCreated, logo)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// DELETE /api/v2/logos/:id
// GET    /api/v2/logos/:id/image
func (s *Server) v2LogoItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/logos/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	suffix := strings.Trim(pathSuffix(r, "/api/v2/logos/"), "/")

	if suffix == "image" && r.Method == http.MethodGet {
		logo, err := s.store.GetLogo(id)
		if err != nil || logo == nil {
			writeError(w, http.StatusNotFound, "logo not found")
			return
		}
		dest := filepath.Join(s.logosDir(), logo.Filename)
		http.ServeFile(w, r, dest)
		return
	}

	if r.Method == http.MethodDelete {
		logo, _ := s.store.GetLogo(id)
		if logo != nil {
			_ = os.Remove(filepath.Join(s.logosDir(), logo.Filename))
		}
		if err := s.store.DeleteLogo(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

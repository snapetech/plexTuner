package webui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
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

func sanitizeLogoFilename(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if strings.Trim(trimmed, `/\`) == "" {
		return "", false
	}
	name := filepath.Base(trimmed)
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	safe := strings.Trim(b.String(), ".")
	if safe == "" || safe == "." || safe == ".." {
		return "", false
	}
	return safe, true
}

func (s *Server) logoPath(filename string) (string, error) {
	safe, ok := sanitizeLogoFilename(filename)
	if !ok {
		return "", fmt.Errorf("invalid logo filename")
	}
	dir := filepath.Clean(s.logosDir())
	dest := filepath.Join(dir, safe)
	rel, err := filepath.Rel(dir, dest)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", fmt.Errorf("invalid logo path")
	}
	return dest, nil
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
		safe, ok := sanitizeLogoFilename(header.Filename)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid filename")
			return
		}
		dir := s.logosDir()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			writeError(w, http.StatusInternalServerError, "mkdir: "+err.Error())
			return
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			writeError(w, http.StatusInternalServerError, "chmod: "+err.Error())
			return
		}
		dest, err := s.logoPath(safe)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		tmp, err := os.CreateTemp(dir, ".upload-*.tmp")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create: "+err.Error())
			return
		}
		tmpName := tmp.Name()
		n, err := io.Copy(tmp, file)
		closeErr := tmp.Close()
		if err != nil {
			_ = os.Remove(tmpName)
			writeError(w, http.StatusInternalServerError, "write: "+err.Error())
			return
		}
		if closeErr != nil {
			_ = os.Remove(tmpName)
			writeError(w, http.StatusInternalServerError, "close: "+closeErr.Error())
			return
		}
		if err := os.Chmod(tmpName, 0o600); err != nil {
			_ = os.Remove(tmpName)
			writeError(w, http.StatusInternalServerError, "chmod: "+err.Error())
			return
		}
		if err := os.Rename(tmpName, dest); err != nil {
			_ = os.Remove(tmpName)
			writeError(w, http.StatusInternalServerError, "rename: "+err.Error())
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
	suffix := ""
	if rest := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/api/v2/logos/%d", id)); rest != r.URL.Path {
		suffix = strings.Trim(rest, "/")
	}

	if suffix == "image" && r.Method == http.MethodGet {
		logo, err := s.store.GetLogo(id)
		if err != nil || logo == nil {
			writeError(w, http.StatusNotFound, "logo not found")
			return
		}
		dest, err := s.logoPath(logo.Filename)
		if err != nil {
			writeError(w, http.StatusNotFound, "logo not found")
			return
		}
		info, err := os.Lstat(dest)
		if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			writeError(w, http.StatusNotFound, "logo not found")
			return
		}
		http.ServeFile(w, r, dest)
		return
	}

	if r.Method == http.MethodDelete {
		logo, _ := s.store.GetLogo(id)
		if logo != nil {
			if dest, err := s.logoPath(logo.Filename); err == nil {
				_ = os.Remove(dest)
			}
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

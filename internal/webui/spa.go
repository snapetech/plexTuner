package webui

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
)

const spaDistPrefix = "static/dist"

// spaBootstrap is the JSON object injected into the SPA index.html before </head>.
type spaBootstrap struct {
	Version string `json:"version"`
	CSRF    string `json:"csrf"`
	User    string `json:"user"`
	Port    int    `json:"port"`
}

// spaHandler serves the embedded React SPA. Static assets are served directly;
// all other paths fall back to index.html so client-side routing works.
func (s *Server) spaHandler() http.Handler {
	distFS, err := fs.Sub(spaFS, spaDistPrefix)
	if err != nil {
		log.Printf("webui: SPA embed sub-FS error: %v", err)
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPath := r.URL.Path

		// Serve real static assets (Vite hashed filenames, favicon, etc.) directly.
		if urlPath != "/" && urlPath != "" {
			trimmed := strings.TrimPrefix(urlPath, "/")
			if _, ferr := fs.Stat(distFS, trimmed); ferr == nil {
				// Strip cache-busting hints so the file server resolves cleanly.
				if strings.HasPrefix(urlPath, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// All other paths → inject bootstrap and serve index.html.
		s.serveSPAIndex(w, r)
	})
}

func (s *Server) serveSPAIndex(w http.ResponseWriter, r *http.Request) {
	raw, err := spaFS.ReadFile(path.Join(spaDistPrefix, "index.html"))
	if err != nil {
		http.Error(w, "SPA index not found — run `npm run build` in web/", http.StatusServiceUnavailable)
		return
	}

	csrf := ""
	if tok, ok := s.csrfForRequest(r); ok {
		csrf = tok
	}

	boot := spaBootstrap{
		Version: s.Version,
		CSRF:    csrf,
		Port:    s.Port,
		User:    s.sessionUser(r),
	}
	bootJSON, _ := json.Marshal(boot)

	// deck-bootstrap script keeps backward-compat with smoke tests and any tooling
	// that reads the legacy format (csrfToken key, application/json type).
	deckBoot, _ := json.Marshal(map[string]interface{}{
		"csrfToken": csrf,
		"version":   s.Version,
		"user":      boot.User,
		"port":      s.Port,
	})
	inject := `<script id="deck-bootstrap" type="application/json">` + string(deckBoot) + `</script>` +
		`<script>window.__TUNERR_BOOT__=` + string(bootJSON) + `</script>`

	html := bytes.ReplaceAll(raw, []byte("</head>"), []byte(inject+"</head>"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

// csrfForRequest returns the CSRF token for the authenticated session, if any.
func (s *Server) csrfForRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	sess, ok := s.sessions[cookie.Value]
	if !ok {
		return "", false
	}
	return sess.CSRFToken, true
}

// sessionUser returns the username from the active session, or empty string.
func (s *Server) sessionUser(_ *http.Request) string {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return s.settings.AuthUser
}

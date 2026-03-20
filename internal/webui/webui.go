// Package webui serves the operator web dashboard on a dedicated port (default 48879 = 0xBEEF).
// It proxies all /api/* requests to the main tuner server so the SPA never needs to know
// the tuner's port — the webui server is the single origin.
package webui

import (
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"text/template"
	"time"
)

// DefaultPort is 0xBEEF — five digits, spells something, easy to remember.
const DefaultPort = 48879

//go:embed index.html
var indexHTML string

// Server is the web UI HTTP server.
type Server struct {
	// Port to listen on (default DefaultPort).
	Port int
	// TunerAddr is the listen address of the main tuner server, e.g. ":5004" or "0.0.0.0:5004".
	// The proxy target is derived as http://127.0.0.1:<port>.
	TunerAddr string
	// Version string injected into the UI.
	Version string
	// AllowLAN: if false (default), restrict to loopback only.
	AllowLAN bool

	tunerBase string
	tmpl      *template.Template
}

// New constructs a Server. tunerAddr is the main tuner's listen address (e.g. ":5004").
func New(port int, tunerAddr, version string, allowLAN bool) *Server {
	if port == 0 {
		port = DefaultPort
	}
	return &Server{
		Port:      port,
		TunerAddr: tunerAddr,
		Version:   version,
		AllowLAN:  allowLAN,
	}
}

// Start begins listening. Blocks until error. Call in a goroutine.
func (s *Server) Start() error {
	s.tunerBase = proxyBase(s.TunerAddr)
	s.tmpl = template.Must(template.New("ui").Delims("[[", "]]").Parse(indexHTML))

	mux := http.NewServeMux()
	mux.HandleFunc("/api/", s.proxy)
	mux.HandleFunc("/", s.index)

	var handler http.Handler = mux
	if !s.AllowLAN {
		handler = localhostOnly(mux)
	}

	srv := &http.Server{
		Addr:        fmt.Sprintf(":%d", s.Port),
		Handler:     handler,
		ReadTimeout: 60 * time.Second,
	}

	log.Printf("webui: http://127.0.0.1:%d  (0xBEEF)  proxying→%s", s.Port, s.tunerBase)
	return srv.ListenAndServe()
}

// proxy reverse-proxies /api/<path> → <tunerBase>/<path>.
func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	base, _ := url.Parse(s.tunerBase)
	rp := httputil.NewSingleHostReverseProxy(base)
	// Strip the /api prefix before forwarding.
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}
	r.Host = base.Host
	// Suppress X-Forwarded-For noise in tuner logs.
	r.Header.Del("X-Forwarded-For")
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, `{"error":"tuner unreachable"}`, http.StatusBadGateway)
	}
	rp.ServeHTTP(w, r)
}

// index serves the SPA for all non-/api/ paths.
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if err := s.tmpl.Execute(w, map[string]interface{}{
		"Version":   s.Version,
		"Port":      s.Port,
		"TunerBase": s.tunerBase,
	}); err != nil {
		log.Printf("webui: template error: %v", err)
	}
}

// proxyBase converts a listen address like ":5004" or "0.0.0.0:5004" into
// "http://127.0.0.1:5004" suitable as a reverse proxy target.
func proxyBase(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "5004"
	}
	return "http://127.0.0.1:" + port
}

// localhostOnly wraps h to reject non-loopback clients.
func localhostOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !isLoopback(host) {
			http.Error(w, "forbidden: webui is localhost-only (set IPTV_TUNERR_UI_ALLOW_LAN=1)", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

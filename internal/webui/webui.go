// Package webui serves the operator dashboard on a dedicated port (default 48879 = 0xBEEF).
// It proxies all /api/* requests to the main tuner server so the browser only needs one origin.
package webui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultPort is 0xBEEF in decimal.
const DefaultPort = 48879
const defaultTelemetryHistoryLimit = 96

//go:embed index.html
var indexHTML string

// Server is the dedicated web dashboard HTTP server.
type Server struct {
	Port      int
	TunerAddr string
	Version   string
	AllowLAN  bool

	tunerBase string
	tmpl      *template.Template

	telemetryMu      sync.Mutex
	telemetrySamples []DeckTelemetrySample
}

type DeckTelemetrySample struct {
	SampledAt       string  `json:"sampled_at"`
	HealthOK        bool    `json:"health_ok"`
	GuidePercent    float64 `json:"guide_percent"`
	StreamPercent   float64 `json:"stream_percent"`
	RecorderPercent float64 `json:"recorder_percent"`
	OpsPercent      float64 `json:"ops_percent"`
}

type DeckTelemetryReport struct {
	GeneratedAt string                `json:"generated_at"`
	Count       int                   `json:"count"`
	Samples     []DeckTelemetrySample `json:"samples"`
}

// New constructs a dedicated dashboard server.
func New(port int, tunerAddr, version string, allowLAN bool) *Server {
	if port <= 0 {
		port = DefaultPort
	}
	return &Server{
		Port:      port,
		TunerAddr: tunerAddr,
		Version:   version,
		AllowLAN:  allowLAN,
	}
}

// Run starts the dashboard server and shuts it down with ctx.
func (s *Server) Run(ctx context.Context) error {
	s.tunerBase = proxyBase(s.TunerAddr)
	s.tmpl = template.Must(template.New("webui").Delims("[[", "]]").Parse(indexHTML))

	mux := http.NewServeMux()
	mux.HandleFunc("/api/", s.proxy)
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/", http.StatusSeeOther)
	})
	mux.HandleFunc("/deck/telemetry.json", s.telemetry)
	mux.HandleFunc("/", s.index)

	handler := http.Handler(mux)
	if !s.AllowLAN {
		handler = localhostOnly(mux)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("webui: http://127.0.0.1:%d (0xBEEF) proxying -> %s", s.Port, s.tunerBase)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("webui shutdown: %v", err)
		}
		<-serverErr
		return nil
	}
}

func (s *Server) telemetry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch r.Method {
	case http.MethodGet:
		s.writeTelemetry(w)
	case http.MethodPost:
		var sample DeckTelemetrySample
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, `{"error":"read telemetry body"}`, http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &sample); err != nil {
			http.Error(w, `{"error":"invalid telemetry json"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(sample.SampledAt) == "" {
			sample.SampledAt = time.Now().UTC().Format(time.RFC3339)
		}
		s.telemetryMu.Lock()
		s.telemetrySamples = append(s.telemetrySamples, sample)
		if len(s.telemetrySamples) > defaultTelemetryHistoryLimit {
			s.telemetrySamples = append([]DeckTelemetrySample(nil), s.telemetrySamples[len(s.telemetrySamples)-defaultTelemetryHistoryLimit:]...)
		}
		s.telemetryMu.Unlock()
		s.writeTelemetry(w)
	case http.MethodDelete:
		s.telemetryMu.Lock()
		s.telemetrySamples = nil
		s.telemetryMu.Unlock()
		s.writeTelemetry(w)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) writeTelemetry(w http.ResponseWriter) {
	s.telemetryMu.Lock()
	rep := DeckTelemetryReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Count:       len(s.telemetrySamples),
		Samples:     append([]DeckTelemetrySample(nil), s.telemetrySamples...),
	}
	s.telemetryMu.Unlock()
	body, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		http.Error(w, `{"error":"encode telemetry"}`, http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	base, err := url.Parse(s.tunerBase)
	if err != nil {
		http.Error(w, `{"error":"invalid tuner base"}`, http.StatusInternalServerError)
		return
	}
	rp := httputil.NewSingleHostReverseProxy(base)
	origDirector := rp.Director
	rp.Director = func(req *http.Request) {
		origDirector(req)
		req.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		req.URL.RawPath = req.URL.Path
		req.Host = base.Host
		req.Header.Del("X-Forwarded-For")
	}
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"tuner unreachable"}`))
	}
	rp.ServeHTTP(w, r)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tmpl.Execute(w, map[string]interface{}{
		"Version":   fallbackVersion(s.Version),
		"Port":      s.Port,
		"TunerBase": s.tunerBase,
		"Now":       time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		log.Printf("webui template: %v", err)
	}
}

func fallbackVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "dev"
	}
	return v
}

func proxyBase(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "5004"
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + port
}

func localhostOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !isLoopback(host) {
			http.Error(w, "forbidden: webui is localhost-only (set IPTV_TUNERR_WEBUI_ALLOW_LAN=1)", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func isLoopback(host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

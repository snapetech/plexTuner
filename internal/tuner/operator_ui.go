package tuner

import (
	"embed"
	"html"
	"net"
	"net/http"
	"os"
	"strings"
)

//go:embed static/ui/index.html
var operatorUIEmbedded embed.FS

func (s *Server) serveOperatorUI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if os.Getenv("IPTV_TUNERR_UI_DISABLED") == "1" {
			http.NotFound(w, r)
			return
		}
		if os.Getenv("IPTV_TUNERR_UI_ALLOW_LAN") != "1" && !isLoopbackRemote(r) {
			http.Error(w, "operator UI is localhost-only (set IPTV_TUNERR_UI_ALLOW_LAN=1 to allow LAN)", http.StatusForbidden)
			return
		}
		b, err := operatorUIEmbedded.ReadFile("static/ui/index.html")
		if err != nil {
			http.Error(w, "ui unavailable", http.StatusInternalServerError)
			return
		}
		ver := s.AppVersion
		if ver == "" {
			ver = "dev"
		}
		body := strings.Replace(string(b), "{{VERSION}}", html.EscapeString(ver), 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	})
}

func isLoopbackRemote(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

package tuner

import (
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

//go:embed static/ui/index.html static/ui/guide.html static/hls_mux_demo.html
var operatorUIEmbedded embed.FS

// operatorUIAllowed enforces IPTV_TUNERR_UI_DISABLED and localhost-only access (unless IPTV_TUNERR_UI_ALLOW_LAN=1).
func operatorUIAllowed(w http.ResponseWriter, r *http.Request) bool {
	if os.Getenv("IPTV_TUNERR_UI_DISABLED") == "1" {
		http.NotFound(w, r)
		return false
	}
	if os.Getenv("IPTV_TUNERR_UI_ALLOW_LAN") != "1" && !isLoopbackRemote(r) {
		msg := "operator UI is localhost-only (set IPTV_TUNERR_UI_ALLOW_LAN=1 to allow LAN)"
		path := strings.ToLower(strings.TrimSpace(r.URL.Path))
		if strings.HasSuffix(path, ".json") || strings.HasPrefix(path, "/ops/") {
			writeServerJSONError(w, http.StatusForbidden, msg)
			return false
		}
		http.Error(w, msg, http.StatusForbidden)
		return false
	}
	return true
}

func parseOperatorGuidePreviewLimit(r *http.Request, defaultLimit int) int {
	if defaultLimit <= 0 {
		defaultLimit = 50
	}
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultLimit
	}
	if n > 500 {
		return 500
	}
	return n
}

func buildGuidePreviewMeta(s *Server, gp GuidePreview, rowLimit int) string {
	ver := s.AppVersion
	if ver == "" {
		ver = "dev"
	}
	var b strings.Builder
	b.WriteString("Version <strong>")
	b.WriteString(html.EscapeString(ver))
	b.WriteString("</strong>. ")
	if !gp.SourceReady {
		b.WriteString("Merged guide cache is empty.")
		return b.String()
	}
	b.WriteString(fmt.Sprintf("Showing up to <strong>%d</strong> programmes (sorted by start). ", rowLimit))
	b.WriteString(fmt.Sprintf("Channels in guide: <strong>%d</strong>; programmes: <strong>%d</strong>. ", gp.ChannelCount, gp.ProgrammeCount))
	if gp.CacheExpiresAt != "" {
		b.WriteString("Cache expires: <code>")
		b.WriteString(html.EscapeString(gp.CacheExpiresAt))
		b.WriteString("</code>.")
	}
	return b.String()
}

func operatorDeckURL(r *http.Request) string {
	if os.Getenv("IPTV_TUNERR_WEBUI_DISABLED") == "1" {
		return ""
	}
	port := strings.TrimSpace(os.Getenv("IPTV_TUNERR_WEBUI_PORT"))
	if port == "" {
		port = "48879"
	}
	scheme := "http"
	if r != nil && r.TLS != nil {
		scheme = "https"
	}
	host := "127.0.0.1"
	if r != nil {
		if reqHost := strings.TrimSpace(r.Host); reqHost != "" {
			if parsedHost, _, err := net.SplitHostPort(reqHost); err == nil {
				host = parsedHost
			} else {
				host = reqHost
			}
		}
	}
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s://%s:%s/", scheme, host, port)
}

func buildGuidePreviewTable(gp GuidePreview) string {
	if !gp.SourceReady {
		return `<p><em>Guide cache is empty — wait for the next EPG refresh.</em></p>`
	}
	if len(gp.Rows) == 0 {
		return `<p><em>No parseable programmes in the merged guide.</em></p>`
	}
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th>Channel</th><th>Title</th><th>Start (UTC)</th><th>End (UTC)</th></tr></thead><tbody>`)
	for _, row := range gp.Rows {
		ch := html.EscapeString(strings.TrimSpace(row.ChannelName))
		if id := strings.TrimSpace(row.ChannelID); id != "" {
			if ch != "" {
				ch = fmt.Sprintf("%s <small>(%s)</small>", ch, html.EscapeString(id))
			} else {
				ch = html.EscapeString(id)
			}
		}
		if ch == "" {
			ch = "—"
		}
		title := html.EscapeString(strings.TrimSpace(row.Title))
		if t := strings.TrimSpace(row.SubTitle); t != "" {
			title += "<br><small>" + html.EscapeString(t) + "</small>"
		}
		if title == "" {
			title = "—"
		}
		b.WriteString("<tr><td>")
		b.WriteString(ch)
		b.WriteString("</td><td>")
		b.WriteString(title)
		b.WriteString("</td><td><code>")
		b.WriteString(html.EscapeString(row.Start))
		b.WriteString(`</code></td><td><code>`)
		b.WriteString(html.EscapeString(row.Stop))
		b.WriteString("</code></td></tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

func writeOperatorGuidePreviewJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(fmt.Sprintf("{\"error\":%q}\n", msg)))
}

func (s *Server) serveOperatorGuidePreviewPage() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/guide/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.xmltv == nil {
			http.Error(w, "guide preview unavailable", http.StatusServiceUnavailable)
			return
		}
		limit := parseOperatorGuidePreviewLimit(r, 50)
		gp, err := s.xmltv.GuidePreview(limit)
		if err != nil {
			http.Error(w, "guide preview failed", http.StatusBadGateway)
			return
		}
		b, err := operatorUIEmbedded.ReadFile("static/ui/guide.html")
		if err != nil {
			http.Error(w, "guide preview unavailable", http.StatusInternalServerError)
			return
		}
		meta := buildGuidePreviewMeta(s, gp, limit)
		table := buildGuidePreviewTable(gp)
		deckURL := operatorDeckURL(r)
		deckNotice := `<p><strong>Compatibility UI:</strong> this guide preview remains available on the tuner port, but the dedicated Control Deck is now the primary operator surface.</p>`
		if deckURL != "" {
			deckNotice = `<p><strong>Compatibility UI:</strong> this guide preview remains available on the tuner port, but the dedicated <a href="` + html.EscapeString(deckURL) + `">Control Deck</a> is now the primary operator surface.</p>`
		}
		body := string(b)
		body = strings.Replace(body, "{{META}}", meta, 1)
		body = strings.Replace(body, "{{TABLE}}", table, 1)
		body = strings.Replace(body, "{{DECK_NOTICE}}", deckNotice, 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	})
}

func (s *Server) serveOperatorGuidePreviewJSON() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.xmltv == nil {
			writeOperatorGuidePreviewJSONError(w, http.StatusServiceUnavailable, "guide preview unavailable")
			return
		}
		limit := parseOperatorGuidePreviewLimit(r, 50)
		gp, err := s.xmltv.GuidePreview(limit)
		if err != nil {
			writeOperatorGuidePreviewJSONError(w, http.StatusBadGateway, "guide preview failed")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(gp); err != nil {
			return
		}
	})
}

func (s *Server) serveOperatorUI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !operatorUIAllowed(w, r) {
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
		deckURL := operatorDeckURL(r)
		deckNotice := `<p><strong>Compatibility UI:</strong> the dedicated Control Deck is the primary operator surface now; this tuner-port shell is kept for lightweight read-only access.</p>`
		if deckURL != "" {
			deckNotice = `<p><strong>Compatibility UI:</strong> the dedicated <a href="` + html.EscapeString(deckURL) + `">Control Deck</a> is the primary operator surface now; this tuner-port shell is kept for lightweight read-only access.</p>`
		}
		body := strings.Replace(string(b), "{{VERSION}}", html.EscapeString(ver), 1)
		body = strings.Replace(body, "{{DECK_NOTICE}}", deckNotice, 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	})
}

func isLoopbackRemote(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

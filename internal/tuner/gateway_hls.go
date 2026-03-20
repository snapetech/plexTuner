package tuner

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

// hlsQuotedURIAttr matches URI="..." in HLS tag lines (#EXT-X-KEY, #EXT-X-MAP, #EXT-X-STREAM-INF, …).
var hlsQuotedURIAttr = regexp.MustCompile(`(?i)(URI=")([^"]*)(")`)

var errHLSMuxUnsupportedTargetScheme = errors.New("unsupported hls mux target URL scheme")

func hlsMuxCORSEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("IPTV_TUNERR_HLS_MUX_CORS")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// applyHLSMuxCORS sets permissive CORS headers for browser/devtools clients hitting ?mux=hls (off by default).
func applyHLSMuxCORS(w http.ResponseWriter) {
	if !hlsMuxCORSEnabled() {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, If-Range, Content-Type, Accept, Authorization")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges, Cache-Control, Content-Type")
}

// maybeServeHLSMuxOPTIONS handles CORS preflight for HLS mux URLs when IPTV_TUNERR_HLS_MUX_CORS is enabled.
func maybeServeHLSMuxOPTIONS(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	if strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mux"))) != "hls" {
		return false
	}
	if !hlsMuxCORSEnabled() {
		return false
	}
	applyHLSMuxCORS(w)
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
	return true
}

// sleepHLSRefresh sleeps based on playlist EXT-X-TARGETDURATION to avoid hammering upstream (1-10s).
func sleepHLSRefresh(playlistBody []byte) {
	sec := hlsTargetDurationSeconds(playlistBody)
	if sec <= 0 {
		sec = 3
	}
	half := sec / 2
	if half < 1 {
		half = 1
	}
	if half > 10 {
		half = 10
	}
	time.Sleep(time.Duration(half) * time.Second)
}

func (g *Gateway) fetchAndRewritePlaylist(r *http.Request, client *http.Client, playlistURL string) ([]byte, string, error) {
	req, err := g.newUpstreamRequest(r.Context(), r, playlistURL)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New("playlist http status " + strconv.Itoa(resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	effectiveURL := playlistURL
	if resp.Request != nil && resp.Request.URL != nil {
		effectiveURL = resp.Request.URL.String()
	}
	return rewriteHLSPlaylist(body, effectiveURL), effectiveURL, nil
}

func (g *Gateway) fetchAndWriteSegment(
	w http.ResponseWriter,
	bodyOut io.Writer,
	r *http.Request,
	client *http.Client,
	segURL string,
	headerSent bool,
) (int64, error) {
	if bodyOut == nil {
		bodyOut = w
	}
	req, err := g.newUpstreamRequest(r.Context(), r, segURL)
	if err != nil {
		return 0, err
	}
	if debugOpts := streamDebugOptionsFromEnv(); debugOpts.HTTPHeaders {
		for _, line := range debugHeaderLines(req.Header) {
			reqID := gatewayReqIDFromContext(r.Context())
			log.Printf("gateway: req=%s segment-fetch > %s", reqID, line)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		if g.FetchCFReject && strings.Contains(err.Error(), "cloudflare-terms-of-service-abuse.com") {
			return 0, errCFBlock
		}
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Detect CF at segment level — CF sometimes passes the playlist but blocks .ts segments.
		// Peek at the body to check for CF signals, then note the block for bootstrapping.
		preview := make([]byte, 256)
		n, _ := resp.Body.Read(preview)
		if isCFLikeStatus(resp.StatusCode, string(preview[:n])) {
			g.noteUpstreamCFBlock(segURL)
			if g.cfBoot != nil {
				go func() {
					if ua := g.cfBoot.EnsureAccess(r.Context(), segURL, client); ua != "" {
						g.setLearnedUA(hostFromURL(segURL), ua)
					}
				}()
			}
		}
		return 0, errors.New("segment http status " + strconv.Itoa(resp.StatusCode))
	}
	if !headerSent {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusOK)
	}
	n, err := io.Copy(bodyOut, resp.Body)
	return n, err
}

func isHLSResponse(resp *http.Response, upstreamURL string) bool {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "m3u") {
		return true
	}
	return strings.Contains(strings.ToLower(upstreamURL), ".m3u8")
}

func rewriteHLSPlaylist(body []byte, upstreamURL string) []byte {
	base, err := url.Parse(upstreamURL)
	if err != nil || base == nil {
		return body
	}
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(body))
	first := true
	for sc.Scan() {
		if !first {
			out.WriteByte('\n')
		}
		first = false
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			out.WriteString(line)
			continue
		}
		if strings.HasPrefix(trim, "//") {
			out.WriteString(base.Scheme + ":" + trim)
			continue
		}
		ref, perr := url.Parse(trim)
		if perr != nil {
			out.WriteString(line)
			continue
		}
		if ref.IsAbs() {
			out.WriteString(trim)
			continue
		}
		out.WriteString(base.ResolveReference(ref).String())
	}
	if len(body) > 0 && body[len(body)-1] == '\n' {
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// gatewayHLSProxyMediaURL builds a Tunerr /stream URL that proxies an upstream HLS segment or sub-playlist.
// When IPTV_TUNERR_STREAM_PUBLIC_BASE_URL is set (e.g. http://192.168.1.10:5004), media lines use an absolute URL
// so clients that do not resolve relative playlist URLs correctly still work.
func gatewayHLSProxyMediaURL(channelID, resolvedSegURL string) string {
	q := url.QueryEscape(resolvedSegURL)
	rel := "/stream/" + url.PathEscape(channelID) + "?mux=hls&seg=" + q
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("IPTV_TUNERR_STREAM_PUBLIC_BASE_URL")), "/")
	if base == "" {
		return rel
	}
	return base + rel
}

// resolveHLSMediaRef resolves a playlist-relative or absolute URL against the effective playlist URL.
func resolveHLSMediaRef(raw string, base *url.URL) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || base == nil {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return base.Scheme + ":" + raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if ref.IsAbs() {
		return raw
	}
	return base.ResolveReference(ref).String()
}

// rewriteHLSQuotedURIAttrs replaces each URI="…" in an HLS tag line with a Tunerr /stream proxy URL (same as media lines).
func rewriteHLSQuotedURIAttrs(line string, base *url.URL, channelID string) string {
	return hlsQuotedURIAttr.ReplaceAllStringFunc(line, func(full string) string {
		sm := hlsQuotedURIAttr.FindStringSubmatch(full)
		if len(sm) < 4 {
			return full
		}
		prefix, inner, suffix := sm[1], sm[2], sm[3]
		if strings.TrimSpace(inner) == "" {
			return full
		}
		resolved := resolveHLSMediaRef(inner, base)
		if strings.TrimSpace(resolved) == "" {
			return full
		}
		proxied := gatewayHLSProxyMediaURL(channelID, resolved)
		return prefix + proxied + suffix
	})
}

func rewriteHLSPlaylistToGatewayProxy(body []byte, upstreamURL string, channelID string) []byte {
	base, err := url.Parse(upstreamURL)
	if err != nil || base == nil {
		return body
	}
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(body))
	first := true
	for sc.Scan() {
		if !first {
			out.WriteByte('\n')
		}
		first = false
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" {
			out.WriteString(line)
			continue
		}
		if strings.HasPrefix(trim, "#") {
			// Attribute name is usually URI= but some generators use uri=; regex rewrite is case-insensitive.
			if strings.Contains(strings.ToUpper(trim), `URI="`) {
				out.WriteString(rewriteHLSQuotedURIAttrs(line, base, channelID))
			} else {
				out.WriteString(line)
			}
			continue
		}
		ref, perr := url.Parse(trim)
		if perr != nil {
			out.WriteString(line)
			continue
		}
		resolved := trim
		if strings.HasPrefix(trim, "//") {
			resolved = base.Scheme + ":" + trim
		} else if !ref.IsAbs() {
			resolved = base.ResolveReference(ref).String()
		}
		out.WriteString(gatewayHLSProxyMediaURL(channelID, resolved))
	}
	if len(body) > 0 && body[len(body)-1] == '\n' {
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func (g *Gateway) serveHLSMuxTarget(w http.ResponseWriter, r *http.Request, client *http.Client, channelID, targetURL string) error {
	if !safeurl.IsHTTPOrHTTPS(targetURL) {
		return errHLSMuxUnsupportedTargetScheme
	}
	req, err := g.newUpstreamRequest(r.Context(), r, targetURL)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusNotModified:
	default:
		return errors.New("hls target http status " + strconv.Itoa(resp.StatusCode))
	}

	if resp.StatusCode == http.StatusNotModified {
		_, _ = io.Copy(io.Discard, resp.Body)
		for _, h := range []string{"ETag", "Last-Modified", "Cache-Control", "Expires", "Vary"} {
			if v := resp.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		applyHLSMuxCORS(w)
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	effectiveURL := targetURL
	if resp.Request != nil && resp.Request.URL != nil {
		effectiveURL = resp.Request.URL.String()
	}

	if isHLSResponse(resp, effectiveURL) {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("hls playlist unexpected status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		out := rewriteHLSPlaylistToGatewayProxy(body, effectiveURL, channelID)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		applyHLSMuxCORS(w)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		return nil
	}

	for _, h := range []string{"Content-Range", "Accept-Ranges", "Content-Length", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if ct := strings.TrimSpace(resp.Header.Get("Content-Type")); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	w.Header().Set("Cache-Control", "no-store")
	applyHLSMuxCORS(w)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}

func firstHLSMediaLine(body []byte) string {
	lines := hlsMediaLines(body)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func hlsPlaylistLooksUsable(body []byte) bool {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return false
	}
	if !strings.Contains(trimmed, "#EXTM3U") {
		return false
	}
	return len(hlsMediaLines(body)) > 0
}

func hlsMediaLines(body []byte) []string {
	sc := bufio.NewScanner(bytes.NewReader(body))
	var out []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// hlsTargetDurationSeconds parses #EXT-X-TARGETDURATION from playlist body; returns 0 if missing/invalid.
func hlsTargetDurationSeconds(body []byte) int {
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"))
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
			return 0
		}
	}
	return 0
}

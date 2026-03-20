package tuner

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

// hlsQuotedURIAttr matches URI="..." in HLS tag lines (#EXT-X-KEY, #EXT-X-MAP, #EXT-X-STREAM-INF, …).
var hlsQuotedURIAttr = regexp.MustCompile(`(?i)(URI=")([^"]*)(")`)

// hlsQuotedURIAttrSingle matches non-standard URI='...' (some packagers emit single quotes).
var hlsQuotedURIAttrSingle = regexp.MustCompile(`(?i)(URI=')([^']*)(')`)

// extInfSameLineMedia matches non-standard #EXTINF where a segment-like URI appears on the same line as the
// duration (some LL-HLS / packager variants). Conservative: requires a known media extension.
var extInfSameLineMedia = regexp.MustCompile(`^(?i)#EXTINF:([\d.]+),\s*([^",\s][^",\n]*?\.(?:m4s|ts|mp4|mp2t|aac|webvtt|vtt))(?:\?[^",\s]*)?\s*$`)

// extInfMergedByteRangeTail matches a same-line BYTERANGE= tail (non-standard; RFC 8216 bis uses #EXT-X-BYTERANGE).
var extInfMergedByteRangeTail = regexp.MustCompile(`(?i)^byterange=(?:"([^"]+)"|([^\s"]+))\s*$`)

// extInfHead parses #EXTINF duration and optional title (everything after the first comma).
var extInfHead = regexp.MustCompile(`(?i)^#EXTINF:([\d.]+)(?:,(.*))?$`)

// parseExtInfMergedByteRange detects "#EXTINF:...,...,BYTERANGE=..." and returns pieces for spec-style split tags.
func parseExtInfMergedByteRange(line string) (duration, title, byteRange string, ok bool) {
	trim := strings.TrimSpace(line)
	low := strings.ToLower(trim)
	idx := strings.Index(low, ",byterange=")
	if idx < 0 || !strings.HasPrefix(low, "#extinf:") {
		return "", "", "", false
	}
	head := strings.TrimSpace(trim[:idx])
	tail := strings.TrimSpace(trim[idx+1:])
	sm := extInfHead.FindStringSubmatch(head)
	if len(sm) < 2 {
		return "", "", "", false
	}
	duration = sm[1]
	if len(sm) >= 3 {
		title = strings.TrimSpace(sm[2])
	}
	bm := extInfMergedByteRangeTail.FindStringSubmatch(tail)
	if len(bm) < 3 {
		return "", "", "", false
	}
	byteRange = bm[1]
	if byteRange == "" {
		byteRange = bm[2]
	}
	if strings.TrimSpace(byteRange) == "" {
		return "", "", "", false
	}
	return duration, title, byteRange, true
}

var errHLSMuxUnsupportedTargetScheme = errors.New("unsupported hls mux target URL scheme")

var errHLSMuxSegParamTooLarge = errors.New("hls mux seg parameter too large")

var errHLSMuxBlockedPrivateUpstream = errors.New("blocked private upstream host for hls mux")

// hlsMuxDiagnosticHeader is a non-HDHR response header for ?mux=hls tooling (debug / browser clients).
// Values include unsupported_target_scheme and upstream_http_<status> (e.g. upstream_http_404).
const hlsMuxDiagnosticHeader = "X-IptvTunerr-Hls-Mux-Error"

// NativeMuxKindHeader is set on successful Tunerr-native mux responses (rewritten HLS playlist, DASH MPD, **304**, or proxied **seg=** body).
const NativeMuxKindHeader = "X-IptvTunerr-Native-Mux"

func setNativeMuxResponseKind(w http.ResponseWriter, muxKind string) {
	k := strings.TrimSpace(strings.ToLower(muxKind))
	if k == "" {
		k = "hls"
	}
	w.Header().Set(NativeMuxKindHeader, k)
}

// Diagnostic header values (stable tokens for scripts).
const hlsMuxDiagUnsupportedTargetScheme = "unsupported_target_scheme"

const hlsMuxDiagSegParamTooLarge = "seg_param_too_large"

const hlsMuxDiagBlockedPrivateUpstream = "blocked_private_upstream"

const hlsMuxDiagSegRateLimited = "seg_rate_limited"

const hlsMuxDiagRedirectRejected = "redirect_rejected"

func hlsMuxUpstreamErrBodyLimit() int {
	const def = 8192
	v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_UPSTREAM_ERR_BODY_MAX"))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	const hardMax = 1024 * 1024
	if n > hardMax {
		return hardMax
	}
	return n
}

func hlsMuxMaxSegParamBytes() int {
	const def = 262144 // 256 KiB raw seg= value (URL-decoded query value)
	v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_MAX_SEG_PARAM_BYTES"))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	const hardMax = 2 * 1024 * 1024
	if n > hardMax {
		return hardMax
	}
	return n
}

func hlsMuxDenyLiteralPrivateUpstream() bool {
	return getenvBool("IPTV_TUNERR_HLS_MUX_DENY_LITERAL_PRIVATE_UPSTREAM", false)
}

func hlsMuxDenyResolvedPrivateUpstream() bool {
	return getenvBool("IPTV_TUNERR_HLS_MUX_DENY_RESOLVED_PRIVATE_UPSTREAM", false)
}

func muxSegRPSPerIP() float64 {
	v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_RPS_PER_IP"))
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return 0
	}
	if f > 10000 {
		return 10000
	}
	return f
}

// splitHLSLines splits playlist bytes on newlines without bufio.Scanner token limits (long #EXTINF lines).
func splitHLSLines(body []byte) []string {
	norm := bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	parts := bytes.Split(norm, []byte("\n"))
	// A single trailing newline produces an extra empty Split segment; drop trailing empties only
	// (interior blank lines are preserved as empty strings between non-empty parts).
	for len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = string(p)
	}
	return out
}

func clientWantsHLSMuxJSON(r *http.Request) bool {
	if r == nil {
		return false
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	if accept == "" {
		return false
	}
	for _, part := range strings.Split(accept, ",") {
		tok := strings.TrimSpace(strings.Split(part, ";")[0])
		if tok == "application/json" || strings.HasSuffix(tok, "+json") {
			return true
		}
	}
	return strings.Contains(accept, "application/json")
}

// hlsMuxUpstreamHTTPError is returned when the upstream responds with a non-success status for ?mux=hls&seg=.
// The gateway may pass status and body to the client instead of mapping everything to 502.
type hlsMuxUpstreamHTTPError struct {
	Status int
	Body   []byte
}

func (e *hlsMuxUpstreamHTTPError) Error() string {
	return "hls target http status " + strconv.Itoa(e.Status)
}

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
	w.Header().Set("Access-Control-Allow-Headers", "Range, If-Range, Content-Type, Accept, Authorization, X-Request-Id, X-Correlation-Id, X-Trace-Id")
	w.Header().Set("Access-Control-Expose-Headers", strings.Join([]string{
		"Content-Range", "Content-Length", "Accept-Ranges", "Cache-Control", "Content-Type", NativeMuxKindHeader, hlsMuxDiagnosticHeader,
	}, ", "))
}

// respondHLSMuxClientError sends a Tunerr HLS mux client error with diagnostic header and optional JSON (Accept: application/json).
func respondHLSMuxClientError(w http.ResponseWriter, r *http.Request, code int, diag, plaintext string) {
	if r != nil && diag != "" {
		reqID := gatewayReqIDFromContext(r.Context())
		chName := ""
		if v := r.Context().Value(gatewayChannelKey{}); v != nil {
			if ch, ok := v.(*catalog.LiveChannel); ok && ch != nil {
				chName = ch.GuideName
			}
		}
		log.Printf("gateway: req=%s channel=%q hls_mux_diag=%s code=%d %s", reqID, chName, diag, code, plaintext)
	}
	applyHLSMuxCORS(w)
	w.Header().Set(hlsMuxDiagnosticHeader, diag)
	if clientWantsHLSMuxJSON(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": diag, "message": plaintext})
		return
	}
	http.Error(w, plaintext, code)
}

// respondHLSMuxUnsupportedTargetScheme sends 400 with a machine-readable diagnostic header and optional CORS.
func respondHLSMuxUnsupportedTargetScheme(w http.ResponseWriter, r *http.Request) {
	respondHLSMuxClientError(w, r, http.StatusBadRequest, hlsMuxDiagUnsupportedTargetScheme, "unsupported hls mux target URL scheme")
}

// respondHLSMuxUpstreamHTTP forwards an upstream HTTP error to the client (status + small body preview).
func respondHLSMuxUpstreamHTTP(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	diag := "upstream_http_" + strconv.Itoa(status)
	if r != nil {
		reqID := gatewayReqIDFromContext(r.Context())
		chName := ""
		if v := r.Context().Value(gatewayChannelKey{}); v != nil {
			if ch, ok := v.(*catalog.LiveChannel); ok && ch != nil {
				chName = ch.GuideName
			}
		}
		log.Printf("gateway: req=%s channel=%q hls_mux_diag=%s upstream_status=%d", reqID, chName, diag, status)
	}
	applyHLSMuxCORS(w)
	w.Header().Set(hlsMuxDiagnosticHeader, diag)
	if len(body) > 0 {
		if ct := http.DetectContentType(body); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
	}
	w.WriteHeader(status)
	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

// maybeServeHLSMuxOPTIONS handles CORS preflight for HLS mux URLs when IPTV_TUNERR_HLS_MUX_CORS is enabled.
func maybeServeHLSMuxOPTIONS(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	m := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mux")))
	if m != "hls" && m != "dash" {
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

type playlistFetchError struct {
	Status  int
	Preview string
	Limited bool
}

func (e *playlistFetchError) Error() string {
	if e == nil {
		return "playlist fetch failed"
	}
	if strings.TrimSpace(e.Preview) != "" {
		return fmt.Sprintf("playlist http status %d: %s", e.Status, e.Preview)
	}
	return "playlist http status " + strconv.Itoa(e.Status)
}

func hlsPlaylistRetryLimit() int {
	n := getenvInt("IPTV_TUNERR_HLS_PLAYLIST_RETRY_LIMIT", 2)
	if n < 0 {
		return 0
	}
	if n > 4 {
		return 4
	}
	return n
}

func hlsPlaylistRetryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 3 {
		attempt = 3
	}
	baseMs := getenvInt("IPTV_TUNERR_HLS_PLAYLIST_RETRY_BACKOFF_MS", 1000)
	if baseMs < 1 {
		baseMs = 1
	}
	if baseMs > 10000 {
		baseMs = 10000
	}
	return time.Duration(baseMs*(1<<(attempt-1))) * time.Millisecond
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (g *Gateway) fetchAndRewritePlaylist(r *http.Request, client *http.Client, playlistURL string) ([]byte, string, error) {
	reqID := ""
	ctx := context.Background()
	if r != nil {
		reqID = gatewayReqIDFromContext(r.Context())
		ctx = r.Context()
	}
	retries := hlsPlaylistRetryLimit()
	for attempt := 0; ; attempt++ {
		req, err := g.newUpstreamRequest(r.Context(), r, playlistURL)
		if err != nil {
			return nil, "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}
		if resp.StatusCode != http.StatusOK {
			preview := readUpstreamErrorPreview(resp)
			logPreview := sanitizeUpstreamPreviewForLog(preview)
			resp.Body.Close()
			limited := isUpstreamConcurrencyLimit(resp.StatusCode, preview)
			if limited {
				g.noteUpstreamConcurrencySignal(resp.StatusCode, preview)
				if learned := g.learnUpstreamConcurrencyLimit(preview); learned > 0 {
					log.Printf("gateway: req=%s playlist concurrency learned limit=%d status=%d url=%s body=%q",
						reqID, learned, resp.StatusCode, safeurl.RedactURL(playlistURL), logPreview)
				}
			}
			if limited && attempt < retries {
				backoff := hlsPlaylistRetryBackoff(attempt + 1)
				log.Printf("gateway: req=%s playlist refresh concurrency-limited status=%d url=%s backoff=%s retry=%d/%d body=%q",
					reqID, resp.StatusCode, safeurl.RedactURL(playlistURL), backoff, attempt+1, retries, logPreview)
				if err := sleepWithContext(ctx, backoff); err != nil {
					return nil, "", err
				}
				continue
			}
			return nil, "", &playlistFetchError{Status: resp.StatusCode, Preview: preview, Limited: limited}
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, "", err
		}
		effectiveURL := playlistURL
		if resp.Request != nil && resp.Request.URL != nil {
			effectiveURL = resp.Request.URL.String()
		}
		return rewriteHLSPlaylist(body, effectiveURL), effectiveURL, nil
	}
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
	lines := splitHLSLines(body)
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
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

// gatewayNativeMuxProxyURL builds /stream?mux=<kind>&seg= for Tunerr-native HLS or DASH proxies.
func gatewayNativeMuxProxyURL(channelID, resolvedSegURL, muxKind string) string {
	muxKind = strings.TrimSpace(strings.ToLower(muxKind))
	if muxKind == "" {
		muxKind = "hls"
	}
	q := url.QueryEscape(resolvedSegURL)
	rel := "/stream/" + url.PathEscape(channelID) + "?mux=" + url.QueryEscape(muxKind) + "&seg=" + q
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("IPTV_TUNERR_STREAM_PUBLIC_BASE_URL")), "/")
	if base == "" {
		return rel
	}
	return base + rel
}

// gatewayHLSProxyMediaURL builds a Tunerr /stream URL that proxies an upstream HLS segment or sub-playlist.
// When IPTV_TUNERR_STREAM_PUBLIC_BASE_URL is set (e.g. http://192.168.1.10:5004), media lines use an absolute URL
// so clients that do not resolve relative playlist URLs correctly still work.
func gatewayHLSProxyMediaURL(channelID, resolvedSegURL string) string {
	return gatewayNativeMuxProxyURL(channelID, resolvedSegURL, "hls")
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

// rewriteHLSQuotedURIAttrs replaces each URI="…" or URI='…' in an HLS tag line with a Tunerr /stream proxy URL (same as media lines).
func rewriteHLSQuotedURIAttrs(line string, base *url.URL, channelID string) string {
	rewrite := func(re *regexp.Regexp, s string) string {
		return re.ReplaceAllStringFunc(s, func(full string) string {
			sm := re.FindStringSubmatch(full)
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
	line = rewrite(hlsQuotedURIAttr, line)
	line = rewrite(hlsQuotedURIAttrSingle, line)
	return line
}

func rewriteHLSPlaylistToGatewayProxy(body []byte, upstreamURL string, channelID string) []byte {
	body = stripLeadingUTF8BOM(body)
	base, err := url.Parse(upstreamURL)
	if err != nil || base == nil {
		return body
	}
	var out bytes.Buffer
	lines := splitHLSLines(body)
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		trim := strings.TrimSpace(line)
		if trim == "" {
			out.WriteString(line)
			continue
		}
		if strings.HasPrefix(trim, "#") {
			if dur, extTitle, br, ok := parseExtInfMergedByteRange(trim); ok {
				out.WriteString("#EXTINF:" + dur)
				if extTitle != "" {
					out.WriteString("," + extTitle)
				} else {
					out.WriteString(",")
				}
				out.WriteByte('\n')
				out.WriteString("#EXT-X-BYTERANGE:" + br)
				continue
			}
			if sm := extInfSameLineMedia.FindStringSubmatch(trim); len(sm) == 3 {
				resolved := resolveHLSMediaRef(sm[2], base)
				if strings.TrimSpace(resolved) != "" {
					proxied := gatewayHLSProxyMediaURL(channelID, resolved)
					out.WriteString(strings.Replace(trim, sm[2], proxied, 1))
					continue
				}
			}
			// Attribute name is usually URI= but some generators use uri=; regex rewrite is case-insensitive.
			// Covers #EXT-X-PART, #EXT-X-PRELOAD-HINT, #EXT-X-RENDITION-REPORT, keys, maps, variants, etc.
			up := strings.ToUpper(trim)
			if strings.Contains(up, `URI="`) || strings.Contains(up, `URI='`) {
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
		out.WriteString(gatewayNativeMuxProxyURL(channelID, resolved, "hls"))
	}
	if len(body) > 0 && body[len(body)-1] == '\n' {
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func (g *Gateway) serveHLSMuxTarget(w http.ResponseWriter, r *http.Request, client *http.Client, channelID, targetURL string) error {
	return g.serveNativeMuxTarget(w, r, client, channelID, targetURL, "hls")
}

func muxAccessLogJSON(mux, channelID, target string, d time.Duration) string {
	m := map[string]any{
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
		"mux":        mux,
		"channel_id": channelID,
		"dur_ms":     d.Milliseconds(),
		"target":     safeurl.RedactURL(target),
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func appendMuxSegAccessLogLine(path, line string) {
	if path == "" || line == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.WriteString(f, line+"\n")
}

func (g *Gateway) serveNativeMuxTarget(w http.ResponseWriter, r *http.Request, client *http.Client, channelID, targetURL, muxKind string) error {
	start := time.Now()
	if !safeurl.IsHTTPOrHTTPS(targetURL) {
		return errHLSMuxUnsupportedTargetScheme
	}
	muxKind = strings.TrimSpace(strings.ToLower(muxKind))
	if muxKind == "" {
		muxKind = "hls"
	}
	upstreamMethod := http.MethodGet
	if r != nil && r.Method == http.MethodHead {
		upstreamMethod = http.MethodHead
	}
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	req, err := g.newUpstreamRequestMethod(ctx, r, targetURL, upstreamMethod)
	if err != nil {
		return err
	}
	segClient := muxSegHTTPClient(client, ctx)
	resp, err := segClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusNotModified:
	default:
		lim := int64(hlsMuxUpstreamErrBodyLimit())
		preview, rerr := io.ReadAll(io.LimitReader(resp.Body, lim))
		if rerr != nil {
			promNoteMuxManifestOutcome(muxKind, "read_error", channelID, time.Since(start))
			return rerr
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		promNoteMuxManifestOutcome(muxKind, "upstream_http", channelID, time.Since(start))
		return &hlsMuxUpstreamHTTPError{Status: resp.StatusCode, Body: preview}
	}

	if resp.StatusCode == http.StatusNotModified {
		_, _ = io.Copy(io.Discard, resp.Body)
		for _, h := range []string{"ETag", "Last-Modified", "Cache-Control", "Expires", "Vary"} {
			if v := resp.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		setNativeMuxResponseKind(w, muxKind)
		applyHLSMuxCORS(w)
		w.WriteHeader(http.StatusNotModified)
		promNoteMuxManifestOutcome(muxKind, "not_modified", channelID, time.Since(start))
		return nil
	}

	effectiveURL := targetURL
	if resp.Request != nil && resp.Request.URL != nil {
		effectiveURL = resp.Request.URL.String()
	}

	// HEAD has no body; never treat as a fetch-and-rewrite manifest.
	if isHLSResponse(resp, effectiveURL) && muxKind == "hls" && (r == nil || r.Method != http.MethodHead) {
		if resp.StatusCode != http.StatusOK {
			lim := int64(hlsMuxUpstreamErrBodyLimit())
			preview, rerr := io.ReadAll(io.LimitReader(resp.Body, lim))
			if rerr != nil {
				promNoteMuxManifestOutcome(muxKind, "read_error", channelID, time.Since(start))
				return rerr
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			promNoteMuxManifestOutcome(muxKind, "upstream_http", channelID, time.Since(start))
			return &hlsMuxUpstreamHTTPError{Status: resp.StatusCode, Body: preview}
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			promNoteMuxManifestOutcome(muxKind, "read_error", channelID, time.Since(start))
			return err
		}
		out := rewriteHLSPlaylistToGatewayProxy(body, effectiveURL, channelID)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		setNativeMuxResponseKind(w, muxKind)
		applyHLSMuxCORS(w)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		promNoteMuxManifestOutcome(muxKind, "playlist_proxy", channelID, time.Since(start))
		return nil
	}

	if isDASHMPDResponse(resp, effectiveURL) && muxKind == "dash" && (r == nil || r.Method != http.MethodHead) {
		if resp.StatusCode != http.StatusOK {
			lim := int64(hlsMuxUpstreamErrBodyLimit())
			preview, rerr := io.ReadAll(io.LimitReader(resp.Body, lim))
			if rerr != nil {
				promNoteMuxManifestOutcome(muxKind, "read_error", channelID, time.Since(start))
				return rerr
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			promNoteMuxManifestOutcome(muxKind, "upstream_http", channelID, time.Since(start))
			return &hlsMuxUpstreamHTTPError{Status: resp.StatusCode, Body: preview}
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			promNoteMuxManifestOutcome(muxKind, "read_error", channelID, time.Since(start))
			return err
		}
		out := rewriteDASHManifestToGatewayProxy(body, effectiveURL, channelID)
		ct := strings.TrimSpace(resp.Header.Get("Content-Type"))
		if ct == "" {
			ct = "application/dash+xml"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "no-store")
		setNativeMuxResponseKind(w, muxKind)
		applyHLSMuxCORS(w)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		promNoteMuxManifestOutcome(muxKind, "mpd_proxy", channelID, time.Since(start))
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
	setNativeMuxResponseKind(w, muxKind)
	applyHLSMuxCORS(w)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	promNoteMuxManifestOutcome(muxKind, "binary_relay", channelID, time.Since(start))
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
	var out []string
	for _, raw := range splitHLSLines(body) {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// hlsTargetDurationSeconds parses #EXT-X-TARGETDURATION from playlist body; returns 0 if missing/invalid.
func hlsTargetDurationSeconds(body []byte) int {
	for _, raw := range splitHLSLines(body) {
		line := strings.TrimSpace(raw)
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

package tuner

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

// defaultLavfUA is the fallback Lavf User-Agent when ffmpeg is not installed or detection fails.
// Matches the libavformat version shipped with ffmpeg 7.1 (2024).
const defaultLavfUA = "Lavf/61.7.100"

// detectFFmpegLavfUA runs ffprobe (or ffmpeg) to read the libavformat version and returns
// a User-Agent string in the form "Lavf/X.Y.Z". Returns "" if detection fails.
func detectFFmpegLavfUA() string {
	for _, bin := range []string{"ffprobe", "ffmpeg"} {
		out, err := exec.Command(bin, "-version").Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "libavformat") {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "libavformat"))
			// Take the part before "/" (build version vs ident version)
			if idx := strings.Index(rest, "/"); idx >= 0 {
				rest = strings.TrimSpace(rest[:idx])
			}
			// Remove all spaces: "61.  7.100" → "61.7.100"
			ver := strings.ReplaceAll(rest, " ", "")
			ver = strings.Trim(ver, ".")
			if ver == "" {
				continue
			}
			valid := true
			for _, ch := range ver {
				if ch != '.' && (ch < '0' || ch > '9') {
					valid = false
					break
				}
			}
			if valid && strings.Contains(ver, ".") {
				return "Lavf/" + ver
			}
		}
	}
	return ""
}

// resolveUserAgentPreset maps well-known preset names to canonical User-Agent strings.
// detectedLavfUA is the auto-detected value from the installed ffmpeg, used for the
// "lavf"/"ffmpeg" preset so the Go HTTP client sends the same UA as the ffmpeg subprocess.
// If detectedLavfUA is empty, defaultLavfUA is used for those presets.
func resolveUserAgentPreset(raw, detectedLavfUA string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "lavf", "ffmpeg", "libavformat":
		if detectedLavfUA != "" {
			return detectedLavfUA
		}
		return defaultLavfUA
	case "vlc":
		return "VLC/3.0.21 LibVLC/3.0.21"
	case "kodi":
		return "Kodi/21.0 (X11; Linux x86_64) App_Bitness/64 Version/21.0-Git:20240205-a9cf89e8fd"
	case "firefox":
		return "Mozilla/5.0 (Windows NT 10.0; rv:109.0) Gecko/20100101 Firefox/115.0"
	}
	return raw
}

// Range/If-Range: HLS byte-range segments (#EXT-X-BYTERANGE) and partial fetches through ?mux=hls&seg=.
// If-None-Match / If-Modified-Since: conditional GET for cached segments (304 Not Modified).
var forwardedUpstreamHeaderNames = []string{
	"Cookie", "Referer", "Origin",
	"Range", "If-Range", "If-None-Match", "If-Modified-Since",
	"X-Request-Id", "X-Correlation-Id", "X-Trace-Id",
}

func cloneClientWithCookieJar(src *http.Client) *http.Client {
	if src == nil {
		src = httpclient.ForStreaming()
	}
	out := *src
	if out.Jar != nil {
		return &out
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return &out
	}
	out.Jar = jar
	return &out
}

func pickPreferredResolvedIP(ips []string) string {
	var first string
	for _, raw := range ips {
		ip := strings.TrimSpace(raw)
		if ip == "" {
			continue
		}
		if first == "" {
			first = ip
		}
		parsed := net.ParseIP(ip)
		if parsed != nil && parsed.To4() != nil {
			return ip
		}
	}
	return first
}

func appendFFmpegHeaderLine(lines []string, name, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return lines
	}
	value = strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
	return append(lines, name+": "+value)
}

func (g *Gateway) customHeaderValue(name string) (string, bool) {
	if g == nil {
		return "", false
	}
	for k, v := range g.CustomHeaders {
		if strings.EqualFold(strings.TrimSpace(k), name) {
			v = strings.TrimSpace(v)
			if v != "" {
				return v, true
			}
		}
	}
	return "", false
}

func gatewayChannelFromContext(ctx context.Context) *catalog.LiveChannel {
	if ctx == nil {
		return nil
	}
	ch, _ := ctx.Value(gatewayChannelKey{}).(*catalog.LiveChannel)
	return ch
}

func streamAuthForURL(ch *catalog.LiveChannel, rawURL string) (catalog.StreamAuth, bool) {
	if ch == nil || strings.TrimSpace(rawURL) == "" {
		return catalog.StreamAuth{}, false
	}
	var best catalog.StreamAuth
	bestLen := -1
	for _, rule := range ch.StreamAuths {
		prefix := strings.TrimSpace(rule.Prefix)
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(rawURL, prefix) && len(prefix) > bestLen {
			best = rule
			bestLen = len(prefix)
		}
	}
	return best, bestLen >= 0
}

func (g *Gateway) authForURL(ctx context.Context, rawURL string) (string, string) {
	if rule, ok := streamAuthForURL(gatewayChannelFromContext(ctx), rawURL); ok {
		return rule.User, rule.Pass
	}
	if user, pass, _, ok := xtreamPathCredentials(rawURL); ok {
		return user, pass
	}
	return g.ProviderUser, g.ProviderPass
}

func (g *Gateway) cookieHeaderForURL(rawURL string) string {
	if g == nil || g.Client == nil || g.Client.Jar == nil || strings.TrimSpace(rawURL) == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return ""
	}
	cookies := g.Client.Jar.Cookies(u)
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c == nil || strings.TrimSpace(c.Name) == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func (g *Gateway) ffmpegCookiesOptionForURL(rawURL string) string {
	if g == nil || g.Client == nil || g.Client.Jar == nil || strings.TrimSpace(rawURL) == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return ""
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return ""
	}
	cookies := g.Client.Jar.Cookies(u)
	if len(cookies) == 0 {
		return ""
	}
	path := strings.TrimSpace(u.EscapedPath())
	if path == "" {
		path = "/"
	}
	lines := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c == nil || strings.TrimSpace(c.Name) == "" {
			continue
		}
		line := c.Name + "=" + c.Value + "; path=" + path + "; domain=" + host + ";"
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (g *Gateway) effectiveUpstreamUserAgent(incoming *http.Request) string {
	return g.effectiveUpstreamUserAgentForURL("", incoming)
}

// effectiveUpstreamUserAgentForURL resolves the User-Agent to use for a request to rawURL.
// Priority: CustomHeaders["User-Agent"] > learned-per-host > CustomUserAgent preset > incoming > default.
func (g *Gateway) effectiveUpstreamUserAgentForURL(rawURL string, incoming *http.Request) string {
	if ua, ok := g.customHeaderValue("User-Agent"); ok {
		return resolveUserAgentPreset(ua, g.DetectedFFmpegUA)
	}
	// Per-host learned UA (set by UA cycling after a CF hit).
	if rawURL != "" {
		if host := hostFromURL(rawURL); host != "" {
			if learned := g.getLearnedUA(host); learned != "" {
				return learned
			}
		}
	}
	if g.CustomUserAgent != "" {
		return resolveUserAgentPreset(g.CustomUserAgent, g.DetectedFFmpegUA)
	}
	if incoming != nil && strings.TrimSpace(incoming.UserAgent()) != "" {
		return strings.TrimSpace(incoming.UserAgent())
	}
	return "IptvTunerr/1.0"
}

func (g *Gateway) effectiveUpstreamReferer(incoming *http.Request) string {
	if v, ok := g.customHeaderValue("Referer"); ok {
		return v
	}
	if incoming != nil {
		return strings.TrimSpace(incoming.Header.Get("Referer"))
	}
	return ""
}

func (g *Gateway) applyUpstreamRequestHeaders(req *http.Request, incoming *http.Request) {
	if req == nil {
		return
	}
	if incoming != nil {
		for _, name := range forwardedUpstreamHeaderNames {
			for _, value := range incoming.Header.Values(name) {
				if strings.TrimSpace(value) != "" {
					req.Header.Add(name, value)
				}
			}
		}
		if g.ProviderUser == "" && g.ProviderPass == "" {
			for _, value := range incoming.Header.Values("Authorization") {
				if strings.TrimSpace(value) != "" {
					req.Header.Add("Authorization", value)
				}
			}
		}
	}
	authUser, authPass := g.authForURL(req.Context(), req.URL.String())
	if authUser != "" || authPass != "" {
		req.SetBasicAuth(authUser, authPass)
	}
	if host, ok := g.customHeaderValue("Host"); ok {
		req.Host = host
	}
	ua := g.effectiveUpstreamUserAgentForURL(req.URL.String(), incoming)
	req.Header.Set("User-Agent", ua)
	// Apply full browser header profile when using a browser UA (helps CF Bot Management scoring).
	for name, value := range browserHeadersForUA(ua) {
		if req.Header.Get(name) == "" {
			req.Header.Set(name, value)
		}
	}
	if site, ok := g.customHeaderValue("Sec-Fetch-Site"); ok {
		req.Header.Set("Sec-Fetch-Site", site)
	} else if g.AddSecFetchHeaders {
		req.Header.Set("Sec-Fetch-Site", "cross-site")
	}
	if mode, ok := g.customHeaderValue("Sec-Fetch-Mode"); ok {
		req.Header.Set("Sec-Fetch-Mode", mode)
	} else if g.AddSecFetchHeaders {
		req.Header.Set("Sec-Fetch-Mode", "cors")
	}
	for name, value := range g.CustomHeaders {
		switch {
		case strings.EqualFold(name, "Host"),
			strings.EqualFold(name, "User-Agent"),
			strings.EqualFold(name, "Sec-Fetch-Site"),
			strings.EqualFold(name, "Sec-Fetch-Mode"):
			continue
		}
		req.Header.Set(name, value)
	}
}

func (g *Gateway) newUpstreamRequest(ctx context.Context, incoming *http.Request, rawURL string) (*http.Request, error) {
	return g.newUpstreamRequestMethod(ctx, incoming, rawURL, http.MethodGet)
}

// newUpstreamRequestMethod builds an upstream GET or HEAD. Only GET and HEAD are accepted; any other method is treated as GET.
func (g *Gateway) newUpstreamRequestMethod(ctx context.Context, incoming *http.Request, rawURL, method string) (*http.Request, error) {
	switch method {
	case http.MethodHead:
		// ok
	default:
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	g.applyUpstreamRequestHeaders(req, incoming)
	return req, nil
}

func (g *Gateway) ffmpegInputHeaderBlock(incoming *http.Request, rawURL, hostOverride string) string {
	lines := make([]string, 0, 8)
	if host, ok := g.customHeaderValue("Host"); ok {
		lines = appendFFmpegHeaderLine(lines, "Host", host)
	} else if hostOverride != "" {
		lines = appendFFmpegHeaderLine(lines, "Host", hostOverride)
	}
	if incoming != nil {
		for _, name := range forwardedUpstreamHeaderNames {
			for _, value := range incoming.Header.Values(name) {
				lines = appendFFmpegHeaderLine(lines, name, value)
			}
		}
		if g.ProviderUser == "" && g.ProviderPass == "" {
			for _, value := range incoming.Header.Values("Authorization") {
				lines = appendFFmpegHeaderLine(lines, "Authorization", value)
			}
		}
	}
	authUser, authPass := g.ProviderUser, g.ProviderPass
	if incoming != nil {
		authUser, authPass = g.authForURL(incoming.Context(), rawURL)
	}
	if authUser != "" || authPass != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(authUser + ":" + authPass))
		lines = appendFFmpegHeaderLine(lines, "Authorization", "Basic "+auth)
	}
	if cookieHeader := g.cookieHeaderForURL(rawURL); cookieHeader != "" {
		lines = appendFFmpegHeaderLine(lines, "Cookie", cookieHeader)
	}
	lines = appendFFmpegHeaderLine(lines, "User-Agent", g.effectiveUpstreamUserAgentForURL(rawURL, incoming))
	if site, ok := g.customHeaderValue("Sec-Fetch-Site"); ok {
		lines = appendFFmpegHeaderLine(lines, "Sec-Fetch-Site", site)
	} else if g.AddSecFetchHeaders {
		lines = appendFFmpegHeaderLine(lines, "Sec-Fetch-Site", "cross-site")
	}
	if mode, ok := g.customHeaderValue("Sec-Fetch-Mode"); ok {
		lines = appendFFmpegHeaderLine(lines, "Sec-Fetch-Mode", mode)
	} else if g.AddSecFetchHeaders {
		lines = appendFFmpegHeaderLine(lines, "Sec-Fetch-Mode", "cors")
	}
	for name, value := range g.CustomHeaders {
		switch {
		case strings.EqualFold(name, "Host"),
			strings.EqualFold(name, "User-Agent"),
			strings.EqualFold(name, "Sec-Fetch-Site"),
			strings.EqualFold(name, "Sec-Fetch-Mode"):
			continue
		}
		lines = appendFFmpegHeaderLine(lines, name, value)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func readUpstreamErrorPreview(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	const limit = 256
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	text = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(text)
	if len(text) > 160 {
		text = text[:160]
	}
	return text
}

func sanitizeUpstreamPreviewForLog(preview string) string {
	preview = strings.TrimSpace(preview)
	if preview == "" {
		return ""
	}
	fields := strings.Fields(preview)
	for i, field := range fields {
		if safeurl.IsHTTPOrHTTPS(field) {
			fields[i] = safeurl.RedactURL(field)
			continue
		}
		if safeurl.HasSensitive(field) {
			fields[i] = "<redacted>"
		}
	}
	out := strings.Join(fields, " ")
	if len(out) > 160 {
		out = out[:160]
	}
	return out
}

func isUpstreamConcurrencyLimit(status int, preview string) bool {
	switch status {
	case http.StatusLocked, http.StatusTooManyRequests, 458, 509:
		return true
	}
	if preview == "" {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(preview))
	return strings.Contains(s, "max connections") ||
		strings.Contains(s, "maximum connections") ||
		strings.Contains(s, "too many connections") ||
		strings.Contains(s, "connection limit") ||
		strings.Contains(s, "concurrent")
}

var upstreamConcurrencyLimitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:max(?:imum)?|limit|allowed)[^0-9]{0,24}(\d{1,2})[^0-9]{0,12}(?:connections?|streams?|devices?|sessions?)`),
	regexp.MustCompile(`(?i)(?:max(?:imum)?)[^0-9]{0,12}(?:connections?|streams?|devices?|sessions?)[^0-9]{0,24}(\d{1,2})`),
	regexp.MustCompile(`(?i)(\d{1,2})[^0-9]{0,12}(?:connections?|streams?|devices?|sessions?)[^0-9]{0,24}(?:max(?:imum)?|limit|allowed|only)`),
}

func parseUpstreamConcurrencyLimit(preview string) int {
	if preview == "" {
		return 0
	}
	for _, re := range upstreamConcurrencyLimitPatterns {
		m := re.FindStringSubmatch(preview)
		if len(m) < 2 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(m[1]))
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}

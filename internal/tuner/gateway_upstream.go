package tuner

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

var forwardedUpstreamHeaderNames = []string{"Cookie", "Referer", "Origin"}

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
	if ua, ok := g.customHeaderValue("User-Agent"); ok {
		req.Header.Set("User-Agent", ua)
	} else if g.CustomUserAgent != "" {
		req.Header.Set("User-Agent", g.CustomUserAgent)
	} else {
		req.Header.Set("User-Agent", "IptvTunerr/1.0")
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
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
	if ua, ok := g.customHeaderValue("User-Agent"); ok {
		lines = appendFFmpegHeaderLine(lines, "User-Agent", ua)
	} else if g.CustomUserAgent != "" {
		lines = appendFFmpegHeaderLine(lines, "User-Agent", g.CustomUserAgent)
	} else if incoming != nil && incoming.UserAgent() != "" {
		lines = appendFFmpegHeaderLine(lines, "User-Agent", incoming.UserAgent())
	} else {
		lines = appendFFmpegHeaderLine(lines, "User-Agent", "IptvTunerr/1.0")
	}
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

func isUpstreamConcurrencyLimit(status int, preview string) bool {
	switch status {
	case http.StatusLocked, http.StatusTooManyRequests, 458:
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

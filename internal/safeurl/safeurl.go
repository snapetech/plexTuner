package safeurl

import (
	"net"
	"net/url"
	"path"
	"strings"
)

// HTTPURLHostIsLiteralBlockedPrivate reports whether rawURL's host is an IP address that is
// loopback, private (RFC 1918), link-local, or unspecified. Hostnames are not resolved.
// Used for optional HLS mux hardening when operators enable deny-private mode.
func HTTPURLHostIsLiteralBlockedPrivate(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return false
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// IsHTTPOrHTTPS reports whether s parses as an http(s) URL.
func IsHTTPOrHTTPS(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u == nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

// RedactURL returns a URL string safe for logging (redacts query and userinfo).
func RedactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "(invalid url)"
	}
	u.RawQuery = ""
	u.User = nil
	u.Path = redactSensitivePath(u.Path)
	return u.String()
}

// RedactQuery redacts only the query portion, keeping path.
func RedactQuery(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	if u.RawQuery == "" {
		return s
	}
	u.RawQuery = "[redacted]"
	return u.String()
}

// HasSensitive returns true if the string looks like it contains credentials.
func HasSensitive(s string) bool {
	if strings.Contains(s, "password=") || strings.Contains(s, "token=") {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	parts := xtreamPathParts(u.Path)
	return len(parts) >= 3
}

func xtreamPathParts(p string) []string {
	clean := path.Clean("/" + strings.TrimSpace(p))
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	if len(parts) < 3 {
		return nil
	}
	switch strings.ToLower(parts[0]) {
	case "live", "movie", "series", "timeshift":
		return parts
	default:
		return nil
	}
}

func redactSensitivePath(p string) string {
	parts := xtreamPathParts(p)
	if len(parts) < 3 {
		return p
	}
	parts[1] = "redacted"
	parts[2] = "redacted"
	return "/" + strings.Join(parts, "/")
}

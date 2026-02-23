package safeurl

import (
	"net/url"
	"strings"
)

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
	return strings.Contains(s, "password=") || strings.Contains(s, "token=")
}

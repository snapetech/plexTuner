package safeurl

import "net/url"

// IsHTTPOrHTTPS returns true if u is a valid URL with scheme http or https.
// Used to reject file://, ftp://, and other schemes that could lead to SSRF or local file access.
func IsHTTPOrHTTPS(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	s := parsed.Scheme
	return s == "http" || s == "https"
}

// RedactURL returns a copy of u with userinfo (username/password) and sensitive query params
// stripped so it is safe to log or include in error messages.
func RedactURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return "[invalid url]"
	}
	if parsed.User != nil {
		parsed.User = nil
	}
	q := parsed.Query()
	for _, key := range []string{"username", "password", "token", "key", "api_key", "apikey"} {
		if q.Has(key) {
			q.Set(key, "[redacted]")
		}
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

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

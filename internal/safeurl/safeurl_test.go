package safeurl

import "testing"

func TestIsHTTPOrHTTPS(t *testing.T) {
	tests := []struct {
		url   string
		allow bool
	}{
		{"http://example.com/", true},
		{"https://example.com/path", true},
		{"HTTP://x", true},
		{"HTTPS://x", true},
		{"file:///etc/passwd", false},
		{"ftp://example.com", false},
		{"", false},
		{"not-a-url", false},
		{"javascript:alert(1)", false},
	}
	for _, tt := range tests {
		got := IsHTTPOrHTTPS(tt.url)
		if got != tt.allow {
			t.Errorf("IsHTTPOrHTTPS(%q) = %v, want %v", tt.url, got, tt.allow)
		}
	}
}

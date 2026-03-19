package safeurl

import "testing"

func TestIsHTTPOrHTTPS(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"http://example.com/stream", true},
		{"https://example.com/stream", true},
		{"HTTP://example.com", true},  // case-insensitive
		{"HTTPS://example.com", true}, // case-insensitive
		{"file:///etc/passwd", false},
		{"ftp://example.com/file", false},
		{"data:text/plain,hello", false},
		{"rtsp://example.com/stream", false},
		{"", false},
		{"not-a-url", false},
		{"//example.com/path", false}, // protocol-relative (no scheme)
		{"javascript:alert(1)", false},
	}
	for _, c := range cases {
		got := IsHTTPOrHTTPS(c.input)
		if got != c.want {
			t.Errorf("IsHTTPOrHTTPS(%q) = %v; want %v", c.input, got, c.want)
		}
	}
}

func TestRedactURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			"http://user:pass@example.com/path?token=secret&foo=bar",
			"http://example.com/path",
		},
		{
			"https://example.com/api?key=abc123",
			"https://example.com/api",
		},
		{
			"http://example.com/plain",
			"http://example.com/plain",
		},
		{
			"http://example.com/live/user123/pass456/789.m3u8",
			"http://example.com/live/redacted/redacted/789.m3u8",
		},
		{
			"https://example.com/timeshift/user123/pass456/60/2026-03-19:10-00/789.ts?token=abc",
			"https://example.com/timeshift/redacted/redacted/60/2026-03-19:10-00/789.ts",
		},
		{
			"not valid url ://",
			"(invalid url)",
		},
	}
	for _, c := range cases {
		got := RedactURL(c.input)
		if got != c.want {
			t.Errorf("RedactURL(%q) = %q; want %q", c.input, got, c.want)
		}
	}
}

func TestRedactQuery(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			"http://example.com/path?username=user&password=pass",
			"http://example.com/path?[redacted]",
		},
		{
			"http://example.com/plain",
			"http://example.com/plain",
		},
	}
	for _, c := range cases {
		got := RedactQuery(c.input)
		if got != c.want {
			t.Errorf("RedactQuery(%q) = %q; want %q", c.input, got, c.want)
		}
	}
}

func TestHasSensitive(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"http://host/get.php?username=user&password=pass", true},
		{"http://host/api?token=abc123", true},
		{"http://host/live/user/pass/1234.m3u8", true},
		{"http://host/stream/1234", false},
		{"", false},
	}
	for _, c := range cases {
		got := HasSensitive(c.input)
		if got != c.want {
			t.Errorf("HasSensitive(%q) = %v; want %v", c.input, got, c.want)
		}
	}
}

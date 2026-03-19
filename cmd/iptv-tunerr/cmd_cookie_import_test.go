package main

import (
	"strings"
	"testing"
)

func TestParseCookieString_basic(t *testing.T) {
	cookies := parseCookieString("cf_clearance=abc123; _ga=GA1.2.456", "provider.example.com", "/", false, 0)
	if len(cookies) != 2 {
		t.Fatalf("want 2 cookies, got %d", len(cookies))
	}
	if cookies[0].Name != "cf_clearance" || cookies[0].Value != "abc123" {
		t.Errorf("cookie[0] = {%s=%s}, want cf_clearance=abc123", cookies[0].Name, cookies[0].Value)
	}
	if cookies[1].Name != "_ga" || cookies[1].Value != "GA1.2.456" {
		t.Errorf("cookie[1] = {%s=%s}, want _ga=GA1.2.456", cookies[1].Name, cookies[1].Value)
	}
	for _, c := range cookies {
		if c.Domain != "provider.example.com" {
			t.Errorf("domain = %q, want provider.example.com", c.Domain)
		}
	}
}

func TestParseCookieString_empty(t *testing.T) {
	if got := parseCookieString("", "example.com", "/", false, 0); len(got) != 0 {
		t.Errorf("empty string: got %d cookies", len(got))
	}
}

func TestParseNetscapeCookies_basic(t *testing.T) {
	raw := `# Netscape HTTP Cookie File
.example.com	TRUE	/	FALSE	1735689600	cf_clearance	token123
.example.com	TRUE	/	TRUE	0	session_id	xyz
`
	cookies, err := parseNetscapeCookies(strings.NewReader(raw), 9999999)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(cookies) != 2 {
		t.Fatalf("want 2 cookies, got %d", len(cookies))
	}
	if cookies[0].Name != "cf_clearance" || cookies[0].Value != "token123" {
		t.Errorf("cookie[0] = {%s=%s}", cookies[0].Name, cookies[0].Value)
	}
	if cookies[0].Expires != 1735689600 {
		t.Errorf("expires = %d, want 1735689600", cookies[0].Expires)
	}
	// session cookie (expiry=0 in file) should use defaultExpiry
	if cookies[1].Expires != 9999999 {
		t.Errorf("session cookie expires = %d, want 9999999 (defaultExpiry)", cookies[1].Expires)
	}
	if cookies[1].Secure != true {
		t.Errorf("session_id should be Secure")
	}
}

func TestParseNetscapeCookies_skipComments(t *testing.T) {
	raw := `# comment line
# another comment
.example.com	TRUE	/	FALSE	0	name	value
`
	cookies, err := parseNetscapeCookies(strings.NewReader(raw), 0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(cookies) != 1 {
		t.Errorf("want 1 cookie, got %d", len(cookies))
	}
}

func TestStoreCookie_merges(t *testing.T) {
	saved := make(map[string]map[string]*httpCookieJSON)
	c1 := &httpCookieJSON{Name: "cf_clearance", Value: "old", Domain: "example.com", Path: "/"}
	c2 := &httpCookieJSON{Name: "cf_clearance", Value: "new", Domain: "example.com", Path: "/"}
	storeCookie(saved, c1)
	storeCookie(saved, c2)
	host := "example.com"
	if len(saved[host]) != 1 {
		t.Errorf("expected 1 cookie after overwrite, got %d", len(saved[host]))
	}
	for _, v := range saved[host] {
		if v.Value != "new" {
			t.Errorf("value = %q, want new", v.Value)
		}
	}
}

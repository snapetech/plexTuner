package main

import "testing"

func TestParseCSV_TrimAndDropEmpty(t *testing.T) {
	got := parseCSV(" alpha, ,beta ,, gamma ")
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("len(parseCSV)=%d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseCSV[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestParseCSV_BlankReturnsNil(t *testing.T) {
	if got := parseCSV("   "); got != nil {
		t.Fatalf("parseCSV(blank)=%v want nil", got)
	}
}

func TestHostPortFromBaseURL_ReturnsHostPort(t *testing.T) {
	got, err := hostPortFromBaseURL(" https://example.com:8443/base/path?x=1 ")
	if err != nil {
		t.Fatalf("hostPortFromBaseURL: %v", err)
	}
	if got != "example.com:8443" {
		t.Fatalf("hostPortFromBaseURL=%q want %q", got, "example.com:8443")
	}
}

func TestHostPortFromBaseURL_RejectsMissingHost(t *testing.T) {
	if _, err := hostPortFromBaseURL("/not-a-url"); err == nil {
		t.Fatal("expected missing host error")
	}
}

package guideinput

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestProviderXMLTVURL(t *testing.T) {
	got := ProviderXMLTVURL("https://example.test/base/", "user name", "p@ss")
	want := "https://example.test/base/xmltv.php?username=user+name&password=p%40ss"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLoadGuideData_LocalAndHTTP(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	dir := t.TempDir()
	t.Setenv("IPTV_TUNERR_GUIDE_INPUT_ROOTS", dir)
	path := filepath.Join(dir, "guide.xml")
	const body = "<tv></tv>"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGuideData(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("local got %q", got)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	t.Setenv("IPTV_TUNERR_XMLTV_URL", srv.URL)

	got, err = LoadGuideData(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("http got %q", got)
	}
}

func TestLoadOptionalMatchReport(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><tv><channel id="id.1"><display-name>Alpha</display-name></channel></tv>`))
	}))
	defer srv.Close()
	t.Setenv("IPTV_TUNERR_XMLTV_URL", srv.URL)

	live := []catalog.LiveChannel{{
		ChannelID:   "ch1",
		GuideNumber: "1",
		GuideName:   "Alpha",
	}}
	rep, err := LoadOptionalMatchReport(live, srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep == nil || rep.TotalChannels != 1 || rep.Matched != 1 {
		t.Fatalf("unexpected report: %#v", rep)
	}
}

func TestLoadGuideData_RemoteRequiresConfiguredURL(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	const body = "<tv></tv>"
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer allowed.Close()
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer blocked.Close()

	t.Setenv("IPTV_TUNERR_XMLTV_URL", allowed.URL)

	if _, err := LoadGuideData(blocked.URL); err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("expected allowlist error, got %v", err)
	}

	t.Setenv("IPTV_TUNERR_GUIDE_INPUT_ALLOWED_URLS", blocked.URL)
	got, err := LoadGuideData(blocked.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("http got %q", got)
	}
}

func TestLookupAllowedRemoteGuideRefReturnsAllowlistedTarget(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not used", http.StatusNotImplemented)
	}))
	defer allowed.Close()
	t.Setenv("IPTV_TUNERR_XMLTV_URL", allowed.URL)

	remote, ok, err := lookupAllowedRemoteGuideRef(allowed.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected remote ref to be allowed")
	}
	if remote.URL() != allowed.URL {
		t.Fatalf("remote URL=%q want %q", remote.URL(), allowed.URL)
	}
}

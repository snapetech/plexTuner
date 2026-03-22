package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/provider"
)

func TestNormalizeCatalogProviderBase(t *testing.T) {
	if got := normalizeCatalogProviderBase("  http://example.test///  "); got != "http://example.test" {
		t.Fatalf("normalizeCatalogProviderBase=%q", got)
	}
}

func TestCatalogProviderIdentityKey_NormalizesBase(t *testing.T) {
	got := catalogProviderIdentityKey("  http://example.test///  ", "u", "p")
	want := "http://example.test|u|p"
	if got != want {
		t.Fatalf("catalogProviderIdentityKey=%q want %q", got, want)
	}
}

func TestPrioritizeWinningProvider_NormalizesEquivalentBases(t *testing.T) {
	ranked := []provider.EntryResult{
		{Entry: provider.Entry{BaseURL: "http://provider.test", User: "u", Pass: "p"}},
		{Entry: provider.Entry{BaseURL: "  http://provider.test///  ", User: "u", Pass: "p"}},
		{Entry: provider.Entry{BaseURL: "http://backup.test", User: "u2", Pass: "p2"}},
	}

	got := prioritizeWinningProvider(ranked, provider.EntryResult{
		Entry: provider.Entry{BaseURL: "  http://provider.test///  ", User: "u", Pass: "p"},
	})
	if len(got) != 2 {
		t.Fatalf("len(got)=%d want 2", len(got))
	}
	if got[0].Entry.BaseURL != "http://provider.test" {
		t.Fatalf("winner first=%q want canonical first matching entry", got[0].Entry.BaseURL)
	}
	if got[1].Entry.BaseURL != "http://backup.test" {
		t.Fatalf("duplicate equivalent provider should not remain in output order: %+v", got)
	}
}

func TestStreamVariantsFromRankedEntries_NormalizesBase(t *testing.T) {
	variants := streamVariantsFromRankedEntries("http://origin.test/live/u/p/1001.m3u8", []provider.EntryResult{
		{Entry: provider.Entry{BaseURL: "  http://provider.test///  ", User: "u", Pass: "p"}},
	})
	if len(variants) != 1 {
		t.Fatalf("len(variants)=%d want 1", len(variants))
	}
	if got := variants[0].URL; got != "http://provider.test/live/u/p/1001.m3u8" {
		t.Fatalf("variant url=%q", got)
	}
}

func TestStreamURLsFromRankedBases_NormalizesBase(t *testing.T) {
	got := streamURLsFromRankedBases("http://origin.test/live/u/p/1001.m3u8", []string{"  http://provider.test///  "})
	if len(got) != 1 {
		t.Fatalf("len(got)=%d want 1", len(got))
	}
	if got[0] != "http://provider.test/live/u/p/1001.m3u8" {
		t.Fatalf("stream url=%q", got[0])
	}
}

func TestLoadProviderXMLTVChannelsForRepairFallsBackToDiskCache(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_URL", "")
	t.Setenv("IPTV_TUNERR_PROVIDER_URLS", "")
	t.Setenv("IPTV_TUNERR_PROVIDER_USER", "")
	t.Setenv("IPTV_TUNERR_PROVIDER_PASS", "")
	t.Setenv("IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX", "")

	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "provider-epg.xml")
	body := `<?xml version="1.0" encoding="UTF-8"?><tv><channel id="ch1"><display-name>Ch1</display-name></channel></tv>`
	if err := os.WriteFile(cacheFile, []byte(body), 0644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("hijack unsupported")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_, _ = fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Length: 999999\r\n\r\n<tv>")
		_ = conn.Close()
	}))
	defer srv.Close()

	ref := fmt.Sprintf("%s/xmltv.php?username=user&password=pass", srv.URL)
	chans, err := loadProviderXMLTVChannelsForRepair(&config.Config{ProviderEPGDiskCachePath: cacheFile}, ref, []string{ref})
	if err != nil {
		t.Fatalf("loadProviderXMLTVChannelsForRepair: %v", err)
	}
	if len(chans) != 1 || chans[0].ID != "ch1" {
		t.Fatalf("unexpected channels: %+v", chans)
	}
}

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeOne_ok(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("#EXTM3U"))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := ProbeOne(ctx, srv.URL, nil)
	if r.Status != StatusOK {
		t.Errorf("Status: %s", r.Status)
	}
	if r.StatusCode != 200 {
		t.Errorf("StatusCode: %d", r.StatusCode)
	}
}

func TestProbeOne_badStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ctx := context.Background()
	r := ProbeOne(ctx, srv.URL, nil)
	if r.Status != StatusBadStatus {
		t.Errorf("Status: %s", r.Status)
	}
	if r.StatusCode != 404 {
		t.Errorf("StatusCode: %d", r.StatusCode)
	}
}

func TestProbeOne_cloudflare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.WriteHeader(503)
		w.Write([]byte("Checking your browser"))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := ProbeOne(ctx, srv.URL, nil)
	if r.Status != StatusCloudflare {
		t.Errorf("Status: %s", r.Status)
	}
}

func TestProbeAll_sort(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer badSrv.Close()

	urls := []string{badSrv.URL, okSrv.URL}
	ctx := context.Background()
	results := ProbeAll(ctx, urls, nil)
	if len(results) != 2 {
		t.Fatalf("len(results)=%d", len(results))
	}
	// OK should be first (sorted by OK then latency)
	if results[0].Status != StatusOK {
		t.Errorf("first result Status: %s", results[0].Status)
	}
	if results[1].Status != StatusBadStatus {
		t.Errorf("second result Status: %s", results[1].Status)
	}
}

func TestBestM3UURL(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer badSrv.Close()

	ctx := context.Background()
	urls := []string{badSrv.URL, okSrv.URL}
	best := BestM3UURL(ctx, urls, nil)
	if best != okSrv.URL {
		t.Errorf("BestM3UURL = %q", best)
	}
}

func TestBestM3UURL_noneOk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	ctx := context.Background()
	best := BestM3UURL(ctx, []string{srv.URL}, nil)
	if best != "" {
		t.Errorf("BestM3UURL = %q, want empty", best)
	}
}

func TestProbePlayerAPI_ok(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_info":{"username":"u","password":"p"}}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	r := ProbePlayerAPI(ctx, srv.URL, "u", "p", nil)
	if r.Status != StatusOK {
		t.Errorf("Status: %s", r.Status)
	}
	if r.URL != srv.URL {
		t.Errorf("URL: %q", r.URL)
	}
}

func TestProbePlayerAPI_badStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	ctx := context.Background()
	r := ProbePlayerAPI(ctx, srv.URL, "u", "p", nil)
	if r.Status != StatusBadStatus {
		t.Errorf("Status: %s", r.Status)
	}
}

func TestFirstWorkingPlayerAPI(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer bad.Close()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_info":{}}`))
	}))
	defer ok.Close()

	ctx := context.Background()
	urls := []string{bad.URL, ok.URL}
	got := FirstWorkingPlayerAPI(ctx, urls, "u", "p", nil)
	if got != ok.URL {
		t.Errorf("FirstWorkingPlayerAPI = %q, want %q", got, ok.URL)
	}
}

// cfPlayerAPISrv returns an httptest.Server that responds as a Cloudflare-proxied 503.
func cfPlayerAPISrv() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.WriteHeader(503)
		w.Write([]byte("Checking your browser before accessing the site."))
	}))
}

func TestRankedPlayerAPI_blockCF_skipsCloudflare(t *testing.T) {
	cf := cfPlayerAPISrv()
	defer cf.Close()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_info":{}}`))
	}))
	defer ok.Close()

	var warned string
	opts := ProbeOptions{
		BlockCloudflare: true,
		Logger:          func(f string, a ...any) { warned += f },
	}
	ctx := context.Background()
	ranked := RankedPlayerAPI(ctx, []string{cf.URL, ok.URL}, "u", "p", nil, opts)
	if len(ranked) != 1 {
		t.Fatalf("want 1 ranked URL (non-CF), got %d: %v", len(ranked), ranked)
	}
	if ranked[0] != ok.URL {
		t.Errorf("ranked[0] = %q, want %q", ranked[0], ok.URL)
	}
	if warned == "" {
		t.Error("expected a warning log about CF URL, got none")
	}
}

func TestRankedPlayerAPI_blockCF_allCFReturnsEmpty(t *testing.T) {
	cf1 := cfPlayerAPISrv()
	defer cf1.Close()
	cf2 := cfPlayerAPISrv()
	defer cf2.Close()

	var alerts []string
	opts := ProbeOptions{
		BlockCloudflare: true,
		Logger:          func(f string, a ...any) { alerts = append(alerts, f) },
	}
	ctx := context.Background()
	ranked := RankedPlayerAPI(ctx, []string{cf1.URL, cf2.URL}, "u", "p", nil, opts)
	if len(ranked) != 0 {
		t.Errorf("expected empty result when all URLs are CF, got %v", ranked)
	}
	found := false
	for _, a := range alerts {
		if a == "%s" || len(a) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected alert log when all URLs are Cloudflare-proxied")
	}
}

func TestRankedPlayerAPI_blockCF_offAllowsCF(t *testing.T) {
	// With BlockCloudflare=false (default), CF URLs are treated as bad-status and excluded
	// from the OK-only result, but no warning is emitted.
	cf := cfPlayerAPISrv()
	defer cf.Close()

	var warned string
	opts := ProbeOptions{BlockCloudflare: false, Logger: func(f string, _ ...any) { warned += f }}
	ctx := context.Background()
	ranked := RankedPlayerAPI(ctx, []string{cf.URL}, "u", "p", nil, opts)
	// CF host probe returns StatusCloudflare which is not StatusOK, so ranked should be empty.
	if len(ranked) != 0 {
		t.Errorf("expected empty (CF host returns non-OK), got %v", ranked)
	}
	// No warning should be emitted when blocking is off.
	if warned != "" {
		t.Errorf("unexpected warning when BlockCloudflare=false: %q", warned)
	}
}

func TestRankedPlayerAPI_allRankedBestFirst(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_info":{}}`))
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_info":{}}`))
	}))
	defer fast.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer bad.Close()

	ctx := context.Background()
	urls := []string{bad.URL, slow.URL, fast.URL}
	ranked := RankedPlayerAPI(ctx, urls, "u", "p", nil)
	// Only OK bases are returned (bad returns 503)
	if len(ranked) != 2 {
		t.Fatalf("RankedPlayerAPI: want 2 OK bases, got %d", len(ranked))
	}
	// Best first (fast), then slow
	if ranked[0] != fast.URL {
		t.Errorf("ranked[0] = %q, want fast %q", ranked[0], fast.URL)
	}
	if ranked[1] != slow.URL {
		t.Errorf("ranked[1] = %q, want slow %q", ranked[1], slow.URL)
	}
}

// RankedEntries tests

func TestRankedEntries_multipleProvidersRankedBestFirst(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_info":{}}`))
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_info":{}}`))
	}))
	defer fast.Close()

	entries := []Entry{
		{BaseURL: slow.URL, User: "uA", Pass: "pA"},
		{BaseURL: fast.URL, User: "uB", Pass: "pB"},
	}
	ctx := context.Background()
	ranked := RankedEntries(ctx, entries, nil)
	if len(ranked) != 2 {
		t.Fatalf("want 2 ranked, got %d", len(ranked))
	}
	if ranked[0].Entry.BaseURL != fast.URL {
		t.Errorf("ranked[0] URL = %q, want fast %q", ranked[0].Entry.BaseURL, fast.URL)
	}
	if ranked[0].Entry.User != "uB" {
		t.Errorf("ranked[0] User = %q, want uB", ranked[0].Entry.User)
	}
}

func TestRankedEntries_blockCF_separateCreds(t *testing.T) {
	cf := cfPlayerAPISrv()
	defer cf.Close()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_info":{}}`))
	}))
	defer ok.Close()

	var warnLog string
	entries := []Entry{
		{BaseURL: cf.URL, User: "cfUser", Pass: "cfPass"},
		{BaseURL: ok.URL, User: "realUser", Pass: "realPass"},
	}
	ctx := context.Background()
	ranked := RankedEntries(ctx, entries, nil, ProbeOptions{
		BlockCloudflare: true,
		Logger:          func(f string, a ...any) { warnLog += f },
	})
	if len(ranked) != 1 {
		t.Fatalf("want 1 (CF filtered), got %d", len(ranked))
	}
	if ranked[0].Entry.User != "realUser" {
		t.Errorf("winning entry user = %q, want realUser", ranked[0].Entry.User)
	}
	if warnLog == "" {
		t.Error("expected WARNING log for CF-blocked entry")
	}
}

func TestRankedEntries_allCF_blockReturnsEmpty(t *testing.T) {
	cf := cfPlayerAPISrv()
	defer cf.Close()

	entries := []Entry{{BaseURL: cf.URL, User: "u", Pass: "p"}}
	ctx := context.Background()
	ranked := RankedEntries(ctx, entries, nil, ProbeOptions{BlockCloudflare: true})
	if len(ranked) != 0 {
		t.Errorf("expected empty when all CF, got %d", len(ranked))
	}
}

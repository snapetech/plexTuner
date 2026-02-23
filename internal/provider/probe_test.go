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
	if len(ranked) != 3 {
		t.Fatalf("RankedPlayerAPI: want 3 bases, got %d", len(ranked))
	}
	// Best first (fast), then slow, then bad (non-OK)
	if ranked[0] != fast.URL {
		t.Errorf("ranked[0] = %q, want fast %q", ranked[0], fast.URL)
	}
	if ranked[1] != slow.URL {
		t.Errorf("ranked[1] = %q, want slow %q", ranked[1], slow.URL)
	}
	if ranked[2] != bad.URL {
		t.Errorf("ranked[2] = %q, want bad %q", ranked[2], bad.URL)
	}
}

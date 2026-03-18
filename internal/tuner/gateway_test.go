package tuner

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestGateway_stream_backwardCompat(t *testing.T) {
	// Channel with only StreamURL (no StreamURLs) uses StreamURL
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: up.URL},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("code: %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("body: %q", w.Body.String())
	}
}

func TestGateway_stream_primaryThenBackup(t *testing.T) {
	// Primary returns 500, backup returns 200 -> use backup
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backup"))
	}))
	defer backup.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: primary.URL, StreamURLs: []string{primary.URL, backup.URL}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("code: %d", w.Code)
	}
	if w.Body.String() != "backup" {
		t.Errorf("body: %q", w.Body.String())
	}
}

func TestGateway_stream_noURL(t *testing.T) {
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1"},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("code: %d", w.Code)
	}
}

func TestGateway_stream_allFail(t *testing.T) {
	// Both URLs return 503
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{srv.URL + "/a", srv.URL + "/b"}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("code: %d", w.Code)
	}
}

func TestGateway_stream_upstreamConcurrencyLimitReturnsAllTunersInUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "max connections reached", 458)
	}))
	defer srv.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: srv.URL},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code: %d", w.Code)
	}
	if got := w.Header().Get("X-HDHomeRun-Error"); got != "805" {
		t.Fatalf("X-HDHomeRun-Error=%q", got)
	}
	if !strings.Contains(w.Body.String(), "All tuners in use") {
		t.Fatalf("body=%q", w.Body.String())
	}
}

func TestParseUpstreamConcurrencyLimit(t *testing.T) {
	cases := []struct {
		preview string
		want    int
	}{
		{preview: "max connections reached: 1", want: 1},
		{preview: "Only 2 streams allowed on this account", want: 2},
		{preview: "connection limit hit", want: 0},
	}
	for _, tc := range cases {
		if got := parseUpstreamConcurrencyLimit(tc.preview); got != tc.want {
			t.Fatalf("preview=%q got=%d want=%d", tc.preview, got, tc.want)
		}
	}
}

func TestGateway_learnsUpstreamConcurrencyLimitAndRejectsLocally(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "maximum 1 connections allowed", 423)
	}))
	defer srv.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: srv.URL},
		},
		TunerCount: 4,
	}

	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("first code: %d", w.Code)
	}
	if g.learnedUpstreamLimit != 1 {
		t.Fatalf("learnedUpstreamLimit=%d", g.learnedUpstreamLimit)
	}

	g.mu.Lock()
	g.inUse = 1
	g.mu.Unlock()

	req2 := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("second code: %d", w2.Code)
	}
	if hits != 1 {
		t.Fatalf("upstream hits=%d want=1", hits)
	}
}

func TestGateway_stream_primaryOK(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("primary"))
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("backup"))
	}))
	defer backup.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{primary.URL, backup.URL}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "primary" {
		t.Errorf("code: %d body: %q", w.Code, w.Body.String())
	}
}

func TestGateway_notFound(t *testing.T) {
	g := &Gateway{Channels: nil, TunerCount: 2}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("code: %d", w.Code)
	}
	req2 := httptest.NewRequest(http.MethodGet, "http://local/other", nil)
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Errorf("path other code: %d", w2.Code)
	}
}

func TestGateway_autoPath_fallsBackToGuideNumber(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{
				ChannelID:   "eurosport1.de",
				GuideNumber: "112",
				GuideName:   "DE: EURO SPORT 1",
				StreamURL:   up.URL,
			},
		},
		TunerCount: 2,
	}

	req := httptest.NewRequest(http.MethodGet, "http://local/auto/v112", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("body: %q", w.Body.String())
	}
}

func TestGateway_requestAdaptation_unknownDefaultsWebsafe(t *testing.T) {
	g := &Gateway{PlexClientAdapt: true}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test", nil)

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected transcode override for unknown client")
	}
	if !transcode {
		t.Fatalf("expected unknown client to default to websafe transcode")
	}
	if profile != profilePlexSafe {
		t.Fatalf("profile=%q want %q", profile, profilePlexSafe)
	}
	if reason != "unknown-client-websafe" {
		t.Fatalf("reason=%q", reason)
	}
	if clientClass != "unknown" {
		t.Fatalf("clientClass=%q want unknown", clientClass)
	}
}

func TestGateway_requestAdaptation_resolvedNonWebGetsFull(t *testing.T) {
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer size="1"><Video title="Live TV"><Session id="sid-1"/><Player machineIdentifier="cid-1" product="Plex for Roku" platform="Roku"/></Video></MediaContainer>`))
	}))
	defer pms.Close()

	g := &Gateway{
		PlexClientAdapt: true,
		PlexPMSURL:      pms.URL,
		PlexPMSToken:    "tok",
		Client:          pms.Client(),
	}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test", nil)
	req.Header.Set("X-Plex-Session-Identifier", "sid-1")

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected override for resolved non-web client")
	}
	if transcode {
		t.Fatalf("expected resolved non-web client to force full mode (transcode off)")
	}
	if profile != "" {
		t.Fatalf("profile=%q want empty", profile)
	}
	if reason != "resolved-nonweb-client" {
		t.Fatalf("reason=%q", reason)
	}
	if clientClass != "native" {
		t.Fatalf("clientClass=%q want native", clientClass)
	}
}

func TestGateway_requestAdaptation_resolvedWebGetsWebsafe(t *testing.T) {
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer size="1"><Video title="Live TV"><Session id="sid-web"/><Player machineIdentifier="cid-web" product="Plex Web" platform="Firefox"/></Video></MediaContainer>`))
	}))
	defer pms.Close()

	g := &Gateway{
		PlexClientAdapt: true,
		PlexPMSURL:      pms.URL,
		PlexPMSToken:    "tok",
		Client:          pms.Client(),
	}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test", nil)
	req.Header.Set("X-Plex-Session-Identifier", "sid-web")

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected override for resolved web client")
	}
	if !transcode {
		t.Fatalf("expected resolved web client to use websafe transcode")
	}
	if profile != profilePlexSafe {
		t.Fatalf("profile=%q want %q", profile, profilePlexSafe)
	}
	if reason != "resolved-web-client" {
		t.Fatalf("reason=%q", reason)
	}
	if clientClass != "web" {
		t.Fatalf("clientClass=%q want web", clientClass)
	}
}

func TestGateway_requestAdaptation_internalFetcherGetsWebsafe(t *testing.T) {
	// When Plex matches the internal fetcher (Lavf/PMS) session instead of the browser,
	// we treat it as websafe so Chrome and other browsers still get MP3 audio.
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer size="1"><Video title="Live TV"><Session id="sid-lavf"/><Player machineIdentifier="cid-lavf" product="Lavf" platform="Plex Media Server"/></Video></MediaContainer>`))
	}))
	defer pms.Close()

	g := &Gateway{
		PlexClientAdapt: true,
		PlexPMSURL:      pms.URL,
		PlexPMSToken:    "tok",
		Client:          pms.Client(),
	}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test", nil)
	req.Header.Set("X-Plex-Session-Identifier", "sid-lavf")

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected override for internal fetcher")
	}
	if !transcode {
		t.Fatalf("expected internal fetcher to get websafe transcode so browsers get audio")
	}
	if profile != profilePlexSafe {
		t.Fatalf("profile=%q want %q", profile, profilePlexSafe)
	}
	if reason != "internal-fetcher-websafe" {
		t.Fatalf("reason=%q want internal-fetcher-websafe", reason)
	}
	if clientClass != "internal" {
		t.Fatalf("clientClass=%q want internal", clientClass)
	}
}

func TestGateway_requestAdaptation_autopilotMemoryWins(t *testing.T) {
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer size="1"><Video title="Live TV"><Session id="sid-web"/><Player machineIdentifier="cid-web" product="Plex Web" platform="Firefox"/></Video></MediaContainer>`))
	}))
	defer pms.Close()

	store := &autopilotStore{
		byKey: map[string]autopilotDecision{
			autopilotKey("dna:test", "web"): {
				DNAID:       "dna:test",
				ClientClass: "web",
				Profile:     profileDashFast,
				Transcode:   true,
				Reason:      "resolved-web-client",
				Hits:        3,
			},
		},
	}
	g := &Gateway{
		PlexClientAdapt: true,
		PlexPMSURL:      pms.URL,
		PlexPMSToken:    "tok",
		Client:          pms.Client(),
		Autopilot:       store,
	}
	ch := &catalog.LiveChannel{GuideName: "Test", DNAID: "dna:test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test", nil)
	req.Header.Set("X-Plex-Session-Identifier", "sid-web")

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected autopilot override")
	}
	if !transcode {
		t.Fatalf("expected remembered transcode=true")
	}
	if profile != profileDashFast {
		t.Fatalf("profile=%q want %q", profile, profileDashFast)
	}
	if reason != "autopilot-memory" {
		t.Fatalf("reason=%q want autopilot-memory", reason)
	}
	if clientClass != "web" {
		t.Fatalf("clientClass=%q want web", clientClass)
	}
}

func TestGateway_stream_emptyBodyTriesNext(t *testing.T) {
	// 200 with ContentLength 0 (e.g. dead CDN) should try next URL
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
	}))
	defer empty.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("backup"))
	}))
	defer backup.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{empty.URL, backup.URL}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "backup" {
		t.Errorf("code: %d body: %q (expected 200 backup)", w.Code, w.Body.String())
	}
}

func TestGateway_stream_rejectsNonHTTP(t *testing.T) {
	// SSRF: file:// and other schemes must be rejected; client gets 502
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{"file:///etc/passwd"}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("code: %d (want 502 Bad Gateway for rejected scheme)", w.Code)
	}
}

func TestGateway_stream_rewritesHLSRelativeURLs(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/path/playlist.m3u8":
			w.Header().Set("Content-Type", "application/x-mpegurl")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:10,\n/seg-a.ts\n#EXTINF:10,\nseg-b.ts?x=1\n"))
		case "/seg-a.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-A"))
		case "/path/seg-b.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-B"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: up.URL + "/path/playlist.m3u8"},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	body := w.Body.String()
	if w.Header().Get("Content-Type") != "video/mp2t" {
		t.Fatalf("content-type: %q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, "SEG-A") || !strings.Contains(body, "SEG-B") {
		t.Fatalf("segments not relayed: %q", body)
	}
}

func TestAdaptiveWriter_passthrough(t *testing.T) {
	var out bytes.Buffer
	aw := newAdaptiveWriter(&out)
	data := []byte("hello")
	if _, err := aw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := aw.Flush(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Errorf("got %q", out.Bytes())
	}
}

func TestAdaptiveWriter_growsOnSlowFlush(t *testing.T) {
	// Slow writer: each Write blocks for longer than adaptiveSlowFlushMs.
	slow := &slowWriter{delay: 150 * time.Millisecond}
	aw := newAdaptiveWriter(slow)
	chunk := make([]byte, adaptiveBufferMin)
	for i := range chunk {
		chunk[i] = byte(i & 0xff)
	}
	// Fill past initial target so we trigger a flush; the flush will be "slow" so target grows.
	for i := 0; i < 3; i++ {
		if _, err := aw.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := aw.Flush(); err != nil {
		t.Fatal(err)
	}
	if aw.targetSize <= adaptiveBufferInitial {
		t.Errorf("expected target to grow after slow flush; got targetSize=%d", aw.targetSize)
	}
}

func TestStreamWriter_adaptive(t *testing.T) {
	var out bytes.Buffer
	sw, flush := streamWriter(&mockResponseWriter{w: &out}, -1)
	if _, err := sw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	flush()
	if out.String() != "x" {
		t.Errorf("adaptive flush: got %q", out.String())
	}
}

// slowWriter delays before each Write.
type slowWriter struct {
	delay time.Duration
	w     bytes.Buffer
}

func (s *slowWriter) Write(p []byte) (n int, err error) {
	time.Sleep(s.delay)
	return s.w.Write(p)
}

// mockResponseWriter implements http.ResponseWriter for streamWriter (only Write matters).
type mockResponseWriter struct {
	w io.Writer
}

func (m *mockResponseWriter) Header() http.Header         { return nil }
func (m *mockResponseWriter) WriteHeader(int)             {}
func (m *mockResponseWriter) Write(p []byte) (int, error) { return m.w.Write(p) }

func TestGateway_ffmpegInputHeaderBlock_includesForwardedHeaders(t *testing.T) {
	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream", nil)
	req.Header.Set("Cookie", "session=abc123")
	req.Header.Set("Referer", "https://referer.example")
	req.Header.Set("Origin", "https://origin.example")
	req.Header.Set("Authorization", "Bearer upstream-token")
	block := g.ffmpegInputHeaderBlock(req, "cdn.example")
	if !strings.Contains(block, "Host: cdn.example") {
		t.Fatalf("missing host header in block: %q", block)
	}
	if !strings.Contains(block, "Cookie: session=abc123") {
		t.Fatalf("missing cookie header in block: %q", block)
	}
	if !strings.Contains(block, "Referer: https://referer.example") {
		t.Fatalf("missing referer header in block: %q", block)
	}
	if !strings.Contains(block, "Origin: https://origin.example") {
		t.Fatalf("missing origin header in block: %q", block)
	}
	if !strings.Contains(block, "Authorization: Bearer upstream-token") {
		t.Fatalf("missing auth header in block: %q", block)
	}
}

func TestPickPreferredResolvedIPPrefersIPv4(t *testing.T) {
	if got := pickPreferredResolvedIP([]string{"2606:4700::1", "198.51.100.5"}); got != "198.51.100.5" {
		t.Fatalf("got %q, want IPv4", got)
	}
}

func TestPickPreferredResolvedIPFallsBack(t *testing.T) {
	if got := pickPreferredResolvedIP([]string{"2606:4700::1", "::"}); got != "2606:4700::1" {
		t.Fatalf("got %q, want first entry", got)
	}
}

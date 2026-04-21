package tuner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/eventhooks"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
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

func TestRewriteHLSPlaylistToGatewayProxy_stripsUTF8BOM(t *testing.T) {
	in := append([]byte{0xEF, 0xBB, 0xBF}, []byte("#EXTM3U\n#EXTINF:4,\nseg-1.ts\n")...)
	out := rewriteHLSPlaylistToGatewayProxy(in, "http://up.example/live/index.m3u8", "bom")
	s := string(out)
	if strings.HasPrefix(s, "\ufeff") {
		t.Fatalf("BOM should be stripped from rewrite output: %q", s)
	}
	if len(out) >= 3 && out[0] == 0xEF && out[1] == 0xBB && out[2] == 0xBF {
		t.Fatalf("raw UTF-8 BOM bytes should not prefix output")
	}
	if !strings.Contains(s, "/stream/bom?mux=hls&seg=") {
		t.Fatalf("expected proxied segment: %q", s)
	}
}

func TestRewriteDASHManifestToGatewayProxy_stripsUTF8BOM(t *testing.T) {
	in := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`<MPD><Period><SegmentURL media="https://cdn.example/x.mp4"/></Period></MPD>`)...)
	out := rewriteDASHManifestToGatewayProxy(in, "http://up.example/m.mpd", "c")
	s := string(out)
	if strings.HasPrefix(s, "\ufeff") {
		t.Fatalf("BOM should be stripped: %q", s)
	}
	if !strings.Contains(s, "mux=dash&seg=") {
		t.Fatalf("expected dash proxy: %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy(t *testing.T) {
	in := []byte("#EXTM3U\n#EXTINF:4,\nseg-1.ts\n")
	out := rewriteHLSPlaylistToGatewayProxy(in, "http://up.example/live/index.m3u8", "abc")
	s := string(out)
	if !strings.Contains(s, "/stream/abc?mux=hls&seg=") {
		t.Fatalf("missing gateway proxy url: %q", s)
	}
	if !strings.Contains(s, "http%3A%2F%2Fup.example%2Flive%2Fseg-1.ts") {
		t.Fatalf("missing escaped target url: %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_extXPartURI(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-PART:URI=\"https://cdn.example/part1.m4s\",DURATION=0.5\n"
	out := rewriteHLSPlaylistToGatewayProxy([]byte(in), "http://up.example/ll.m3u8", "x")
	s := string(out)
	if !strings.Contains(s, `URI="/stream/x?mux=hls&seg=`) || !strings.Contains(s, "cdn.example") {
		t.Fatalf("expected EXT-X-PART URI rewrite: %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_extXMapURISingleQuoted(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-MAP:URI='https://cdn.example/init.mp4'\n"
	out := rewriteHLSPlaylistToGatewayProxy([]byte(in), "http://up.example/live.m3u8", "q")
	s := string(out)
	if !strings.Contains(s, `URI='`) || !strings.Contains(s, "/stream/q?mux=hls&seg=") {
		t.Fatalf("expected single-quoted URI= rewrite: %q", s)
	}
	if strings.Contains(s, "https://cdn.example/init.mp4") {
		t.Fatalf("raw upstream init URL should not remain: %q", s)
	}
}

func TestRewriteDASHManifestToGatewayProxy(t *testing.T) {
	in := []byte(`<MPD><Period><AdaptationSet><Representation><SegmentTemplate media="https://cdn.example/seg-$Number$.m4s" initialization="https://cdn.example/init.mp4"/></Representation></AdaptationSet></Period></MPD>`)
	out := rewriteDASHManifestToGatewayProxy(in, "http://up.example/master.mpd", "ch9")
	s := string(out)
	if !strings.Contains(s, "mux=dash&seg=") {
		t.Fatalf("expected dash mux proxy: %q", s)
	}
	if !strings.Contains(s, "$Number$") {
		t.Fatalf("expected SegmentTemplate $Number$ preserved for player substitution: %q", s)
	}
}

func TestRewriteDASHManifestToGatewayProxy_singleQuotedMedia(t *testing.T) {
	in := []byte(`<MPD><Period><SegmentURL media='https://cdn.example/clip.mp4'/></Period></MPD>`)
	out := rewriteDASHManifestToGatewayProxy(in, "http://up.example/m.mpd", "cid")
	s := string(out)
	if !strings.Contains(s, `media="`) || !strings.Contains(s, "mux=dash&seg=") {
		t.Fatalf("expected single-quoted media rewritten to double-quoted proxy URL: %q", s)
	}
	if strings.Contains(s, `'https://cdn.example/clip.mp4'`) {
		t.Fatalf("raw single-quoted upstream URL should not remain: %q", s)
	}
}

func TestDashSegQueryEscape_paddedNumberInTemplate(t *testing.T) {
	in := "https://cdn.example/v-$Number%05d$.m4s"
	q := dashSegQueryEscape(in)
	if strings.Contains(q, "%2505") {
		t.Fatalf("printf width token broken (double %%25): %q", q)
	}
	if !strings.Contains(q, "%05") && !strings.Contains(q, "05d") {
		t.Fatalf("expected restorable %%05d-style width in escaped form: %q", q)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_extInfSameLineMedia(t *testing.T) {
	in := []byte("#EXTM3U\n#EXTINF:4.0,relative/part.m4s\n")
	out := rewriteHLSPlaylistToGatewayProxy(in, "http://up.example/live/m.m3u8", "z")
	s := string(out)
	if !strings.Contains(s, "/stream/z?mux=hls&seg=") {
		t.Fatalf("expected same-line media rewritten: %q", s)
	}
	if strings.Contains(s, "relative/part.m4s") {
		t.Fatalf("raw relative path should not remain: %q", s)
	}
}

func TestParseExtInfMergedByteRange(t *testing.T) {
	d, title, br, ok := parseExtInfMergedByteRange(`#EXTINF:9.009,BYTERANGE="128@448"`)
	if !ok || d != "9.009" || title != "" || br != "128@448" {
		t.Fatalf("quoted: ok=%v d=%q title=%q br=%q", ok, d, title, br)
	}
	d, title, br, ok = parseExtInfMergedByteRange(`#EXTINF:10.0,My Title,BYTERANGE=2048@0`)
	if !ok || d != "10.0" || title != "My Title" || br != "2048@0" {
		t.Fatalf("with title: ok=%v d=%q title=%q br=%q", ok, d, title, br)
	}
	_, _, _, ok = parseExtInfMergedByteRange(`#EXTINF:4,\n`)
	if ok {
		t.Fatal("expected false for normal EXTINF")
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_extInfMergedByteRange(t *testing.T) {
	in := "#EXTM3U\n#EXTINF:9.009,BYTERANGE=\"128@448\"\nseg.ts\n"
	out := rewriteHLSPlaylistToGatewayProxy([]byte(in), "http://up.example/live/index.m3u8", "cid")
	s := string(out)
	if !strings.Contains(s, "#EXTINF:9.009,") || !strings.Contains(s, "#EXT-X-BYTERANGE:128@448") {
		t.Fatalf("expected split EXTINF + EXT-X-BYTERANGE: %q", s)
	}
	for _, line := range strings.Split(s, "\n") {
		tline := strings.TrimSpace(line)
		if strings.HasPrefix(tline, "#EXTINF:") && strings.Contains(strings.ToUpper(tline), "BYTERANGE") {
			t.Fatalf("EXTINF line still merged: %q", s)
		}
	}
	if !strings.Contains(s, "/stream/cid?mux=hls&seg=") {
		t.Fatalf("expected proxied segment URL: %q", s)
	}
}

func TestRewriteDASHManifestToGatewayProxy_expandSegmentTemplate(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE", "true")
	t.Setenv("IPTV_TUNERR_HLS_MUX_DASH_EXPAND_MAX_SEGMENTS", "100")
	mpd := `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" mediaPresentationDuration="PT12S">
<Period duration="PT12S"><AdaptationSet><Representation id="v1" bandwidth="1">
<SegmentTemplate timescale="1" duration="6" startNumber="1" media="https://cdn.example/a-$Number$.m4s" initialization="https://cdn.example/init.mp4"/>
</Representation></AdaptationSet></Period></MPD>`
	out := rewriteDASHManifestToGatewayProxy([]byte(mpd), "http://up.example/m.mpd", "ch1")
	s := string(out)
	if !strings.Contains(s, "<SegmentList") || strings.Contains(s, "$Number$") {
		t.Fatalf("expected expanded SegmentList without $Number$: %q", s)
	}
	if !strings.Contains(s, "mux=dash&seg=") {
		t.Fatalf("expected dash mux proxies: %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_preloadHintUri(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-PRELOAD-HINT:TYPE=PART,URI=\"https://cdn.example/hint.m4s\"\n"
	out := rewriteHLSPlaylistToGatewayProxy([]byte(in), "http://up.example/x.m3u8", "a")
	s := string(out)
	if !strings.Contains(s, `URI="/stream/a?mux=hls&seg=`) {
		t.Fatalf("expected PRELOAD-HINT URI rewrite: %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_renditionReportUri(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-RENDITION-REPORT:URI=\"https://cdn.example/rep.m3u8\",LAST-MSN=1\n"
	out := rewriteHLSPlaylistToGatewayProxy([]byte(in), "http://up.example/x.m3u8", "b")
	s := string(out)
	if !strings.Contains(s, `URI="/stream/b?mux=hls&seg=`) {
		t.Fatalf("expected RENDITION-REPORT URI rewrite: %q", s)
	}
}

func TestRewriteDASHManifestToGatewayProxy_relativeWithBaseURL(t *testing.T) {
	mpd := `<MPD><Period><AdaptationSet><Representation>` +
		`<BaseURL>https://cdn.example/video/</BaseURL>` +
		`<SegmentTemplate media="seg1.m4s" initialization="init.mp4"/>` +
		`</Representation></AdaptationSet></Period></MPD>`
	out := rewriteDASHManifestToGatewayProxy([]byte(mpd), "http://up.example/path/master.mpd", "c1")
	s := string(out)
	if !strings.Contains(s, "mux=dash&seg=") {
		t.Fatalf("expected proxies: %q", s)
	}
	if strings.Contains(s, `media="seg1.m4s"`) {
		t.Fatalf("relative media should be rewritten: %q", s)
	}
	if strings.Contains(s, `initialization="init.mp4"`) {
		t.Fatalf("relative init should be rewritten: %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_matchesGolden(t *testing.T) {
	in := []byte("#EXTM3U\n#EXTINF:4,\nseg-1.ts\n")
	got := rewriteHLSPlaylistToGatewayProxy(in, "http://up.example/live/index.m3u8", "abc")
	want, err := os.ReadFile("testdata/hls_mux_small_playlist.golden")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch\ngot:\n%q\nwant:\n%q", got, want)
	}
}

// TestRewriteHLSPlaylistToGatewayProxy_streamCompareCaptureGolden locks a stream-compare harness snapshot:
// upstream playlist (testdata/stream_compare_hls_mux_capture_upstream.m3u8) rewrites to the expected
// Tunerr playlist (testdata/stream_compare_hls_mux_capture_tunerr_expected.m3u8). Regenerate expected when
// mux URL shape intentionally changes. Source: .diag/stream-compare/synthetic-mux-capture/ (not committed).
func TestRewriteHLSPlaylistToGatewayProxy_streamCompareCaptureGolden(t *testing.T) {
	t.Setenv("IPTV_TUNERR_STREAM_PUBLIC_BASE_URL", "http://127.0.0.1:18080")
	upstream, err := os.ReadFile("testdata/stream_compare_hls_mux_capture_upstream.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/stream_compare_hls_mux_capture_tunerr_expected.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	got := rewriteHLSPlaylistToGatewayProxy(upstream, "http://127.0.0.1:18080/direct.m3u8", "demo")
	if !bytes.Equal(got, want) {
		t.Fatalf("stream-compare golden mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestRewriteDASHManifestToGatewayProxy_streamCompareCaptureGolden mirrors the HLS stream-compare promotion:
// synthetic upstream MPD (testdata/stream_compare_dash_mux_capture_upstream.mpd) is expanded with
// IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE (uniform SegmentTemplate → SegmentList), then URLs are
// rewritten to Tunerr proxies. Regenerate expected if expand output or proxy URL shape changes.
func TestRewriteDASHManifestToGatewayProxy_streamCompareCaptureGolden(t *testing.T) {
	t.Setenv("IPTV_TUNERR_STREAM_PUBLIC_BASE_URL", "http://127.0.0.1:18080")
	t.Setenv("IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE", "1")
	upstream, err := os.ReadFile("testdata/stream_compare_dash_mux_capture_upstream.mpd")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/stream_compare_dash_mux_capture_tunerr_expected.mpd")
	if err != nil {
		t.Fatal(err)
	}
	got := rewriteDASHManifestToGatewayProxy(upstream, "http://127.0.0.1:18080/direct.mpd", "demo")
	if !bytes.Equal(got, want) {
		t.Fatalf("stream-compare DASH golden mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestGateway_hlsMuxSeg_redirectToBlockedPrivateUpstream(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_DENY_LITERAL_PRIVATE_UPSTREAM", "1")
	redir := "http://127.0.0.1:9/secret.ts"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redir, http.StatusFound)
	}))
	defer up.Close()
	g := &Gateway{
		Channels:   []catalog.LiveChannel{{GuideNumber: "0", GuideName: "C", StreamURL: "http://x/x.m3u8"}},
		TunerCount: 2,
	}
	seg := url.QueryEscape(up.URL + "/start.ts")
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+seg, nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d body=%q diag=%q", w.Code, w.Body.String(), w.Header().Get("X-IptvTunerr-Hls-Mux-Error"))
	}
}

func TestGateway_hlsMuxSeg_chunkedUpstreamPassthrough(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("no flusher")
		}
		_, _ = w.Write([]byte("aa"))
		fl.Flush()
		_, _ = w.Write([]byte("bb"))
		fl.Flush()
	}))
	defer up.Close()
	g := &Gateway{
		Channels:   []catalog.LiveChannel{{GuideNumber: "0", GuideName: "C", StreamURL: "http://x/x.m3u8"}},
		TunerCount: 2,
	}
	seg := url.QueryEscape(up.URL + "/blob.bin")
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+seg, nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "aabb" {
		t.Fatalf("body=%q", got)
	}
}

func TestGateway_effectiveHLSMuxSegLimit_autopilotBonus(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS", "1")
	t.Setenv("IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_MIN_HITS", "2")
	t.Setenv("IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS_PER_STEP", "5")
	t.Setenv("IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS_CAP", "100")
	t.Setenv("IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER", "1")
	st := &autopilotStore{byKey: map[string]autopilotDecision{}}
	st.byKey[autopilotKey("dna:x", "web")] = autopilotDecision{DNAID: "dna:x", ClientClass: "web", Hits: 5}
	g := &Gateway{TunerCount: 2, Autopilot: st}
	ch := &catalog.LiveChannel{DNAID: "dna:x"}
	g.mu.Lock()
	lim := g.effectiveHLSMuxSegLimitLocked(ch)
	g.mu.Unlock()
	// base 2 + (5-2+1)*5 = 22
	if lim < 22 {
		t.Fatalf("expected autopilot bonus, limit=%d", lim)
	}
}

func TestGateway_effectiveHLSMuxSegLimit_adaptiveBonus(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO", "1")
	t.Setenv("IPTV_TUNERR_HLS_MUX_SEG_AUTO_BONUS_PER_HIT", "10")
	t.Setenv("IPTV_TUNERR_HLS_MUX_SEG_AUTO_BONUS_CAP", "100")
	t.Setenv("IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER", "1")
	g := &Gateway{TunerCount: 2}
	now := time.Now()
	g.muxSegAutoRejectAt = []time.Time{now, now, now}
	g.mu.Lock()
	base := g.effectiveHLSMuxSegLimitLocked(nil)
	g.mu.Unlock()
	// 2 tuners * 1 slot + min(3*10, 100) = 32
	if base < 30 {
		t.Fatalf("expected adaptive bonus, limit=%d", base)
	}
}

func TestGateway_dashMuxSeg_proxiesBinary(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/init.m4s" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ftyp"))
	}))
	defer up.Close()
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: "http://placeholder/x.mpd"},
		},
		TunerCount: 2,
	}
	seg := url.QueryEscape(up.URL + "/init.m4s")
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=dash&seg="+seg, nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get(NativeMuxKindHeader); got != "dash" {
		t.Fatalf("%s=%q want dash", NativeMuxKindHeader, got)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("ftyp")) {
		t.Fatalf("body=%q", w.Body.String())
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_usesPublicBaseURLWhenConfigured(t *testing.T) {
	t.Setenv("IPTV_TUNERR_STREAM_PUBLIC_BASE_URL", "http://deck.example:5004/")
	in := []byte("#EXTM3U\n#EXTINF:4,\nseg-1.ts\n")
	out := rewriteHLSPlaylistToGatewayProxy(in, "http://up.example/live/index.m3u8", "abc")
	s := string(out)
	if !strings.Contains(s, "http://deck.example:5004/stream/abc?mux=hls&seg=") {
		t.Fatalf("missing absolute proxy url: %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_rewritesExtXKeyAndStreamInfURI(t *testing.T) {
	in := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=1280000,URI="low/index.m3u8"
#EXT-X-KEY:METHOD=AES-128,URI="https://keys.example.com/secret.key"
#EXT-X-MAP:URI="init.mp4"
#EXTINF:4,
seg.ts
`
	out := rewriteHLSPlaylistToGatewayProxy([]byte(in), "http://up.example/live/master.m3u8", "abc")
	s := string(out)
	if !strings.Contains(s, `URI="/stream/abc?mux=hls&seg=`) {
		t.Fatalf("missing proxied URI= on tag lines: %q", s)
	}
	if !strings.Contains(s, "http%3A%2F%2Fup.example%2Flive%2Flow%2Findex.m3u8") {
		t.Fatalf("missing variant stream rewrite: %q", s)
	}
	if !strings.Contains(s, "https%3A%2F%2Fkeys.example.com%2Fsecret.key") {
		t.Fatalf("missing AES key rewrite: %q", s)
	}
	if !strings.Contains(s, "http%3A%2F%2Fup.example%2Flive%2Finit.mp4") {
		t.Fatalf("missing map rewrite: %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_sampleAESAndSessionKey(t *testing.T) {
	in := `#EXTM3U
#EXT-X-SESSION-KEY:METHOD=SAMPLE-AES,URI="https://keys.example/session.key"
#EXT-X-KEY:METHOD=SAMPLE-AES,KEYFORMAT="identity",URI="rel/key.bin"
#EXT-X-MEDIA:TYPE=CLOSED-CAPTIONS,GROUP-ID="cc",NAME="EN",URI="tracks/cc.m3u8"
#EXTINF:4,
seg.ts
`
	out := rewriteHLSPlaylistToGatewayProxy([]byte(in), "http://up.example/live/master.m3u8", "ch1")
	s := string(out)
	if !strings.Contains(s, "METHOD=SAMPLE-AES") {
		t.Fatalf("expected SAMPLE-AES retained: %q", s)
	}
	if !strings.Contains(s, "EXT-X-SESSION-KEY") {
		t.Fatalf("expected EXT-X-SESSION-KEY retained: %q", s)
	}
	if !strings.Contains(s, "https%3A%2F%2Fkeys.example%2Fsession.key") {
		t.Fatalf("missing session-key URI rewrite: %q", s)
	}
	if !strings.Contains(s, "http%3A%2F%2Fup.example%2Flive%2Frel%2Fkey.bin") {
		t.Fatalf("missing relative SAMPLE-AES key rewrite: %q", s)
	}
	if !strings.Contains(s, "tracks%2Fcc.m3u8") {
		t.Fatalf("missing EXT-X-MEDIA URI rewrite: %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_preservesEmptyKeyURI(t *testing.T) {
	in := `#EXTM3U
#EXT-X-KEY:METHOD=NONE
#EXT-X-KEY:METHOD=AES-128,URI=""
#EXTINF:4,
s.ts
`
	out := rewriteHLSPlaylistToGatewayProxy([]byte(in), "http://up.example/a.m3u8", "x")
	s := string(out)
	if !strings.Contains(s, `METHOD=AES-128,URI=""`) {
		t.Fatalf("expected empty URI= left unchanged (no bogus proxy): %q", s)
	}
}

func TestRewriteHLSPlaylistToGatewayProxy_lowercaseURIAttribute(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,NAME=\"a\",uri=\"http://cdn.example/track.m3u8\"\n"
	out := rewriteHLSPlaylistToGatewayProxy([]byte(in), "http://up.example/live/master.m3u8", "z")
	s := string(out)
	if !strings.Contains(s, `/stream/z?mux=hls&seg=`) || !strings.Contains(s, "cdn.example") {
		t.Fatalf("expected lowercase uri= attribute rewritten: %q", s)
	}
}

func TestGateway_serveHLSMuxTarget_forwardsRangeAndPartialContent(t *testing.T) {
	var gotRange string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Content-Range", "bytes 0-3/99")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("1234"))
	}))
	defer up.Close()

	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/ch1?mux=hls&seg="+url.QueryEscape(up.URL+"/seg.ts"), nil)
	req.Header.Set("Range", "bytes=0-3")
	w := httptest.NewRecorder()
	if err := g.serveHLSMuxTarget(w, req, up.Client(), "ch1", up.URL+"/seg.ts"); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusPartialContent {
		t.Fatalf("code=%d want 206", w.Code)
	}
	if gotRange != "bytes=0-3" {
		t.Fatalf("upstream Range header: got %q", gotRange)
	}
	if w.Header().Get("Content-Range") == "" {
		t.Fatal("expected Content-Range on downstream response")
	}
	if got := w.Body.String(); got != "1234" {
		t.Fatalf("body=%q", got)
	}
}

func TestGateway_serveHLSMuxTarget_forwardsNotModified(t *testing.T) {
	var gotINM string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotINM = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Cache-Control", "max-age=60")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer up.Close()

	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/ch1?mux=hls&seg="+url.QueryEscape(up.URL+"/seg.ts"), nil)
	req.Header.Set("If-None-Match", `"abc"`)
	w := httptest.NewRecorder()
	if err := g.serveHLSMuxTarget(w, req, up.Client(), "ch1", up.URL+"/seg.ts"); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusNotModified {
		t.Fatalf("code=%d want 304", w.Code)
	}
	if gotINM != `"abc"` {
		t.Fatalf("If-None-Match upstream: got %q", gotINM)
	}
	if w.Header().Get("ETag") == "" {
		t.Fatal("expected ETag on client response")
	}
}

func TestGateway_hlsMuxSeg_rejectsWhenAtConcurrencyLimit(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT", "1")
	block := make(chan struct{})
	var firstEntered sync.WaitGroup
	firstEntered.Add(1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstEntered.Done()
		<-block
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: up.URL + "/x"},
		},
		TunerCount: 2,
	}
	seg := url.QueryEscape(up.URL + "/seg.ts")
	done := make(chan struct{})
	go func() {
		req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+seg, nil)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, req)
		close(done)
	}()
	firstEntered.Wait()
	req2 := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+seg, nil)
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d body=%q", w2.Code, w2.Body.String())
	}
	close(block)
	<-done
}

func TestGateway_ProviderBehaviorProfile_hlsMuxSegLimit(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT", "17")
	g := &Gateway{TunerCount: 2}
	p := g.ProviderBehaviorProfile()
	if p.HlsMuxSegLimit != 17 {
		t.Fatalf("HlsMuxSegLimit=%d want 17", p.HlsMuxSegLimit)
	}
}

func TestGateway_ProviderBehaviorProfile_includesIntelligenceAutopilot(t *testing.T) {
	st := &autopilotStore{
		byKey: map[string]autopilotDecision{
			autopilotKey("dna:z", "web"): {
				DNAID: "dna:z", ClientClass: "web", Hits: 9, Profile: "heavy",
				Transcode: true, PreferredHost: "cdn.example", UpdatedAt: "2026-03-20T00:00:00Z",
			},
		},
	}
	st.path = "/tmp/autopilot-test.json"
	g := &Gateway{TunerCount: 2, Autopilot: st}
	p := g.ProviderBehaviorProfile()
	if !p.Intelligence.Autopilot.Enabled {
		t.Fatal("expected intelligence.autopilot.enabled")
	}
	if p.Intelligence.Autopilot.DecisionCount != 1 {
		t.Fatalf("decision_count=%d", p.Intelligence.Autopilot.DecisionCount)
	}
	if len(p.Intelligence.Autopilot.HotChannels) != 1 {
		t.Fatalf("hot=%v", p.Intelligence.Autopilot.HotChannels)
	}
	if p.Intelligence.Autopilot.HotChannels[0].DNAID != "dna:z" || p.Intelligence.Autopilot.HotChannels[0].Hits != 9 {
		t.Fatalf("%+v", p.Intelligence.Autopilot.HotChannels[0])
	}
	g2 := &Gateway{TunerCount: 2}
	p2 := g2.ProviderBehaviorProfile()
	if p2.Intelligence.Autopilot.Enabled {
		t.Fatal("expected no intelligence.autopilot without store")
	}
}

func TestGateway_newUpstreamRequest_forwardsCorrelationHeaders(t *testing.T) {
	g := &Gateway{}
	in := httptest.NewRequest(http.MethodGet, "/", nil)
	in.Header.Set("X-Request-Id", "req-1")
	in.Header.Set("X-Correlation-Id", "corr-2")
	in.Header.Set("X-Trace-Id", "trace-3")
	req, err := g.newUpstreamRequest(context.Background(), in, "http://upstream.example/seg.ts")
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("X-Request-Id"); got != "req-1" {
		t.Fatalf("X-Request-Id: got %q", got)
	}
	if got := req.Header.Get("X-Correlation-Id"); got != "corr-2" {
		t.Fatalf("X-Correlation-Id: got %q", got)
	}
	if got := req.Header.Get("X-Trace-Id"); got != "trace-3" {
		t.Fatalf("X-Trace-Id: got %q", got)
	}
}

func TestGateway_serveHLSMuxTarget_forwardsHEAD(t *testing.T) {
	var gotMethod string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Content-Length", "999")
		w.WriteHeader(http.StatusOK)
		// HEAD must not include a body
		_, _ = w.Write([]byte("nope"))
	}))
	defer up.Close()

	g := &Gateway{}
	req := httptest.NewRequest(http.MethodHead, "http://local/stream/ch1?mux=hls&seg="+url.QueryEscape(up.URL+"/seg.ts"), nil)
	w := httptest.NewRecorder()
	if err := g.serveHLSMuxTarget(w, req, up.Client(), "ch1", up.URL+"/seg.ts"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodHead {
		t.Fatalf("upstream method: got %q want HEAD", gotMethod)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("client body should be empty for HEAD, got len=%d", w.Body.Len())
	}
}

func TestGateway_hlsMuxSeg_unsupportedScheme_returnsJSONWhenAccepted(t *testing.T) {
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: "http://up.example/live.m3u8"},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+url.QueryEscape("skd://key-server/example"), nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%q", w.Code, w.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v body=%q", err, w.Body.String())
	}
	if payload["error"] != hlsMuxDiagUnsupportedTargetScheme {
		t.Fatalf("error field: %#v", payload)
	}
}

func TestGateway_hlsMuxSeg_paramTooLarge_returnsBadRequest(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_MAX_SEG_PARAM_BYTES", "12")
	longTarget := "http://zz.example/seg.ts" // len > 12
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: "http://up.example/live.m3u8"},
		},
		TunerCount: 2,
	}
	seg := url.QueryEscape(longTarget)
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+seg, nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Result().Header.Get(hlsMuxDiagnosticHeader); got != hlsMuxDiagSegParamTooLarge {
		t.Fatalf("diag header: got %q", got)
	}
	p := g.ProviderBehaviorProfile()
	if p.HlsMuxSegErrParam != 1 {
		t.Fatalf("HlsMuxSegErrParam=%d want 1", p.HlsMuxSegErrParam)
	}
}

func TestGateway_hlsMuxSeg_literalPrivateBlocked_returnsForbidden(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_DENY_LITERAL_PRIVATE_UPSTREAM", "true")
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: "http://up.example/live.m3u8"},
		},
		TunerCount: 2,
	}
	seg := url.QueryEscape("http://192.168.1.50/seg.ts")
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+seg, nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Result().Header.Get(hlsMuxDiagnosticHeader); got != hlsMuxDiagBlockedPrivateUpstream {
		t.Fatalf("diag: got %q", got)
	}
	p := g.ProviderBehaviorProfile()
	if p.HlsMuxSegErrPrivate != 1 {
		t.Fatalf("HlsMuxSegErrPrivate=%d want 1", p.HlsMuxSegErrPrivate)
	}
}

func TestGateway_hlsMuxSeg_successIncrementsProfileCounter(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ts"))
	}))
	defer up.Close()
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: "http://placeholder/x.m3u8"},
		},
		TunerCount: 2,
	}
	seg := url.QueryEscape(up.URL + "/seg.ts")
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+seg, nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%q", w.Code, w.Body.String())
	}
	p := g.ProviderBehaviorProfile()
	if p.HlsMuxSegSuccess != 1 {
		t.Fatalf("HlsMuxSegSuccess=%d want 1", p.HlsMuxSegSuccess)
	}
}

func TestGateway_hlsMuxSeg_unsupportedScheme_returnsBadRequest(t *testing.T) {
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: "http://up.example/live.m3u8"},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+url.QueryEscape("skd://key-server/example"), nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%q", w.Code, w.Body.String())
	}
	resp := w.Result()
	if got := resp.Header.Get(hlsMuxDiagnosticHeader); got != hlsMuxDiagUnsupportedTargetScheme {
		t.Fatalf("diagnostic header: got %q want %q", got, hlsMuxDiagUnsupportedTargetScheme)
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "unsupported hls mux target url scheme") {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}
}

func TestGateway_hlsMuxSeg_unsupportedScheme_setsCORSExposeWhenEnabled(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_CORS", "1")
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: "http://up.example/live.m3u8"},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+url.QueryEscape("skd://x"), nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	resp := w.Result()
	exp := resp.Header.Get("Access-Control-Expose-Headers")
	if !strings.Contains(exp, hlsMuxDiagnosticHeader) {
		t.Fatalf("Expose-Headers should list diagnostic header: %q", exp)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected CORS allow-origin on error response")
	}
}

func TestServeHLSMuxTarget_unsupportedScheme(t *testing.T) {
	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	err := g.serveHLSMuxTarget(w, req, httpclient.Default(), "ch", "skd://example/key")
	if !errors.Is(err, errHLSMuxUnsupportedTargetScheme) {
		t.Fatalf("want errHLSMuxUnsupportedTargetScheme, got %v", err)
	}
}

func TestServeHLSMuxTarget_returnsUpstreamHTTPError(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("denied"))
	}))
	defer up.Close()
	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	err := g.serveHLSMuxTarget(w, req, up.Client(), "ch", up.URL+"/seg.ts")
	var upErr *hlsMuxUpstreamHTTPError
	if !errors.As(err, &upErr) || upErr.Status != http.StatusForbidden || string(upErr.Body) != "denied" {
		t.Fatalf("unexpected err=%v upErr=%+v", err, upErr)
	}
}

func TestGateway_hlsMuxSeg_upstreamHTTP_passedThrough(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("gone"))
	}))
	defer up.Close()
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: "http://x/a.m3u8"},
		},
		TunerCount: 2,
	}
	seg := url.QueryEscape(up.URL + "/missing.ts")
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls&seg="+seg, nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%q", w.Code, w.Body.String())
	}
	resp := w.Result()
	if got := resp.Header.Get(hlsMuxDiagnosticHeader); got != "upstream_http_404" {
		t.Fatalf("diagnostic header: got %q", got)
	}
	if w.Body.String() != "gone" {
		t.Fatalf("body: %q", w.Body.String())
	}
	p := g.ProviderBehaviorProfile()
	if p.LastHLSMuxOutcome != "upstream_http_404" {
		t.Fatalf("LastHLSMuxOutcome=%q want upstream_http_404", p.LastHLSMuxOutcome)
	}
	if p.LastHLSMuxURL == "" {
		t.Fatal("expected LastHLSMuxURL")
	}
	if p.LastHLSMuxAt == "" {
		t.Fatal("expected LastHLSMuxAt")
	}
}

func TestGateway_dashMuxSeg_successUpdatesRecentOutcome(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("dash"))
	}))
	defer up.Close()
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: "http://x/a.mpd"},
		},
		TunerCount: 2,
	}
	seg := url.QueryEscape(up.URL + "/seg.m4s")
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=dash&seg="+seg, nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%q", w.Code, w.Body.String())
	}
	p := g.ProviderBehaviorProfile()
	if p.LastDashMuxOutcome != "success" {
		t.Fatalf("LastDashMuxOutcome=%q want success", p.LastDashMuxOutcome)
	}
	if p.LastDashMuxURL == "" {
		t.Fatal("expected LastDashMuxURL")
	}
	if p.LastDashMuxAt == "" {
		t.Fatal("expected LastDashMuxAt")
	}
}

func TestMaybeServeHLSMuxOPTIONS(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		t.Setenv("IPTV_TUNERR_HLS_MUX_CORS", "1")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodOptions, "http://x/stream/1?mux=hls", nil)
		if !maybeServeHLSMuxOPTIONS(w, r) {
			t.Fatal("expected handled")
		}
		if w.Code != http.StatusNoContent {
			t.Fatalf("code=%d want 204", w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("missing cors: %v", w.Header())
		}
	})
	t.Run("wrong_method", func(t *testing.T) {
		t.Setenv("IPTV_TUNERR_HLS_MUX_CORS", "1")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "http://x/stream/1?mux=hls", nil)
		if maybeServeHLSMuxOPTIONS(w, r) {
			t.Fatal("expected not handled")
		}
	})
	t.Run("disabled", func(t *testing.T) {
		t.Setenv("IPTV_TUNERR_HLS_MUX_CORS", "0")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodOptions, "http://x/stream/1?mux=hls", nil)
		if maybeServeHLSMuxOPTIONS(w, r) {
			t.Fatal("expected not handled when disabled")
		}
	})
	t.Run("wrong_mux", func(t *testing.T) {
		t.Setenv("IPTV_TUNERR_HLS_MUX_CORS", "1")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodOptions, "http://x/stream/1?mux=fmp4", nil)
		if maybeServeHLSMuxOPTIONS(w, r) {
			t.Fatal("expected not handled")
		}
	})
}

func TestGateway_stream_requiresGetOrHead(t *testing.T) {
	g := &Gateway{
		Channels: []catalog.LiveChannel{{
			ChannelID:   "1",
			GuideNumber: "1",
			GuideName:   "One",
			StreamURLs:  []string{"http://example.invalid/live.m3u8"},
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "http://x/stream/1", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("allow=%q", got)
	}
}

func TestGateway_stream_hlsMuxMethodAllowIncludesOptionsWhenCORSEnabled(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_CORS", "1")
	g := &Gateway{
		Channels: []catalog.LiveChannel{{
			ChannelID:   "1",
			GuideNumber: "1",
			GuideName:   "One",
			StreamURLs:  []string{"http://example.invalid/live.m3u8"},
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "http://x/stream/1?mux=hls", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD, OPTIONS" {
		t.Fatalf("allow=%q", got)
	}
}

func TestGateway_serveHLSMuxTarget_CORSWhenConfigured(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_CORS", "true")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("x"))
	}))
	defer up.Close()
	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/ch1?mux=hls&seg="+url.QueryEscape(up.URL+"/seg.ts"), nil)
	w := httptest.NewRecorder()
	if err := g.serveHLSMuxTarget(w, req, up.Client(), "ch1", up.URL+"/seg.ts"); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected CORS on seg response")
	}
}

func TestGateway_stream_hlsMux_CORSHeadersWhenConfigured(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HLS_MUX_CORS", "on")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:4,\nseg.ts\n"))
	}))
	defer up.Close()
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: up.URL + "/live.m3u8"},
		},
		TunerCount:    2,
		DisableFFmpeg: true,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected CORS on playlist")
	}
}

func TestGateway_stream_hlsMux_returnsRewrittenPlaylist(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:4,\nseg.ts\n"))
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: up.URL + "/live.m3u8"},
		},
		TunerCount:    2,
		DisableFFmpeg: true,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=hls", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "mpegurl") || !strings.Contains(ct, "application/") {
		t.Fatalf("content-type=%q", ct)
	}
	if got := w.Header().Get(NativeMuxKindHeader); got != "hls" {
		t.Fatalf("%s=%q want hls", NativeMuxKindHeader, got)
	}
	body := w.Body.String()
	if !strings.Contains(body, "/stream/0?mux=hls&seg=") {
		t.Fatalf("missing proxy line: %q", body)
	}
}

func TestGateway_stream_dashMux_returnsRewrittenManifest(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/dash+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011"><Period><SegmentURL media="https://cdn.example/seg.m4s"/></Period></MPD>`))
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "0", GuideName: "Ch", StreamURL: up.URL + "/manifest.mpd"},
		},
		TunerCount:    2,
		DisableFFmpeg: true,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0?mux=dash", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get(NativeMuxKindHeader); got != "dash" {
		t.Fatalf("%s=%q want dash", NativeMuxKindHeader, got)
	}
	body := w.Body.String()
	if !strings.Contains(body, "mux=dash&seg=") || !strings.Contains(body, "cdn.example") {
		t.Fatalf("missing proxy rewrite: %q", body)
	}
}

func TestGateway_stream_invalidHLSPlaylistFallsBackToBackup(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>blocked</html>"))
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backup"))
	}))
	defer backup.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{primary.URL + "/stream.m3u8", backup.URL}},
		},
		TunerCount:    2,
		DisableFFmpeg: true,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	if w.Body.String() != "backup" {
		t.Fatalf("body: %q", w.Body.String())
	}
}

func TestGateway_stream_invalidHLSPlaylistFallsBackToTSVariant(t *testing.T) {
	var tsHits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/live/event.m3u8":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.WriteHeader(http.StatusOK)
		case "/live/event.ts":
			tsHits.Add(1)
			w.Header().Set("Content-Type", "video/mp2t")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("fallback-ts"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Peacock Event", StreamURL: up.URL + "/live/event.m3u8?token=keep"},
		},
		TunerCount:    2,
		DisableFFmpeg: true,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d body=%q", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "fallback-ts" {
		t.Fatalf("body: %q", got)
	}
	if got := tsHits.Load(); got != 1 {
		t.Fatalf("ts hits=%d want 1", got)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "video/mp2t") {
		t.Fatalf("content-type=%q", ct)
	}
}

func TestGateway_stream_peacockTSZeroBytesFailsBeforeResponse(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "US (Peacock 031) | Game 3: Pistons vs. Magic", StreamURL: up.URL + "/live/event.ts"},
		},
		TunerCount:    2,
		DisableFFmpeg: false,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "All upstreams failed") {
		t.Fatalf("body=%q", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "video/mp2t") {
		t.Fatalf("unexpected committed stream body: %q", w.Body.String())
	}
}

func TestHLSTSFallbackURL(t *testing.T) {
	got, ok := hlsTSFallbackURL("http://provider.example/live/u/p/1910860.m3u8?token=abc#frag")
	if !ok {
		t.Fatal("expected fallback URL")
	}
	want := "http://provider.example/live/u/p/1910860.ts?token=abc#frag"
	if got != want {
		t.Fatalf("fallback=%q want %q", got, want)
	}
	if got, ok := hlsTSFallbackURL("http://provider.example/live/u/p/1910860.ts"); ok || got != "" {
		t.Fatalf("unexpected fallback=%q ok=%t", got, ok)
	}
}

func TestShouldBypassFFmpegForRawMPEGTS_Peacock(t *testing.T) {
	ch := &catalog.LiveChannel{GuideName: "US (Peacock 017) | Game 2: Rockets vs. Lakers"}
	if !shouldBypassFFmpegForRawMPEGTS(ch, "http://provider.example/live/u/p/1910860.ts?token=abc") {
		t.Fatal("expected Peacock TS to bypass ffmpeg")
	}
	if shouldBypassFFmpegForRawMPEGTS(ch, "http://provider.example/live/u/p/1910860.m3u8") {
		t.Fatal("did not expect Peacock HLS to bypass ffmpeg")
	}
	if shouldBypassFFmpegForRawMPEGTS(&catalog.LiveChannel{GuideName: "US| ESPN HD"}, "http://provider.example/live/u/p/1.ts") {
		t.Fatal("did not expect non-Peacock TS to bypass ffmpeg")
	}
}

func TestGateway_applyUpstreamRequestHeaders_omitsAuthForPeacockTS(t *testing.T) {
	ch := &catalog.LiveChannel{GuideName: "US (Peacock 017) | Game 2: Rockets vs. Lakers"}
	g := &Gateway{ProviderUser: "u1", ProviderPass: "p1"}
	req := httptest.NewRequest(http.MethodGet, "http://provider.example/live/u1/p1/1910860.ts", nil)
	req = req.WithContext(context.WithValue(req.Context(), gatewayChannelKey{}, ch))
	g.applyUpstreamRequestHeaders(req, nil)
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization=%q want empty", got)
	}
}

func TestGateway_stream_prefersAutopilotRememberedURL(t *testing.T) {
	hits := []string{}
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, "primary")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("primary"))
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, "backup")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backup"))
	}))
	defer backup.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", DNAID: "dna:test", StreamURL: primary.URL, StreamURLs: []string{primary.URL, backup.URL}},
		},
		TunerCount: 2,
		Autopilot: &autopilotStore{
			byKey: map[string]autopilotDecision{
				autopilotKey("dna:test", "unknown"): {
					DNAID:         "dna:test",
					ClientClass:   "unknown",
					PreferredURL:  backup.URL,
					PreferredHost: autopilotURLHost(backup.URL),
					Hits:          4,
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	if w.Body.String() != "backup" {
		t.Fatalf("body: %q", w.Body.String())
	}
	if len(hits) == 0 || hits[0] != "backup" {
		t.Fatalf("hit order=%v want backup first", hits)
	}
}

func TestGateway_stream_prefersAutopilotRememberedURL_normalizedTrailingSlash(t *testing.T) {
	hits := []string{}
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, "primary")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("primary"))
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, "backup")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backup"))
	}))
	defer backup.Close()

	rememberAs := strings.TrimRight(backup.URL, "/") + "/"
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", DNAID: "dna:slash", StreamURL: primary.URL, StreamURLs: []string{primary.URL, backup.URL}},
		},
		TunerCount: 2,
		Autopilot: &autopilotStore{
			byKey: map[string]autopilotDecision{
				autopilotKey("dna:slash", "unknown"): {
					DNAID:         "dna:slash",
					ClientClass:   "unknown",
					PreferredURL:  rememberAs,
					PreferredHost: autopilotURLHost(backup.URL),
					Hits:          2,
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	if w.Body.String() != "backup" {
		t.Fatalf("body: %q", w.Body.String())
	}
	if len(hits) == 0 || hits[0] != "backup" {
		t.Fatalf("hit order=%v want backup first", hits)
	}
}

func TestGateway_reorderStreamURLs_autopilotConsensusHost(t *testing.T) {
	t.Setenv("IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST", "true")
	t.Setenv("IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_DNA", "2")
	t.Setenv("IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_HIT_SUM", "8")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE", "false")

	st := &autopilotStore{byKey: map[string]autopilotDecision{
		autopilotKey("dna:one", "web"): {DNAID: "dna:one", ClientClass: "web", PreferredHost: "cdn.example.com", Hits: 5},
		autopilotKey("dna:two", "web"): {DNAID: "dna:two", ClientClass: "web", PreferredHost: "cdn.example.com", Hits: 5},
	}}
	g := &Gateway{TunerCount: 2, Autopilot: st}
	ch := &catalog.LiveChannel{DNAID: "dna:new", GuideNumber: "1", GuideName: "Ch", StreamURLs: []string{
		"https://bad.example.com/a",
		"https://cdn.example.com/b",
	}}
	got := g.reorderStreamURLs(ch, "web", ch.StreamURLs)
	if len(got) != 2 || got[0] != "https://cdn.example.com/b" {
		t.Fatalf("got %v want consensus host first", got)
	}
}

func TestGateway_reorderStreamURLs_autopilotGlobalPreferredHosts(t *testing.T) {
	t.Setenv("IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST", "false")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE", "false")
	t.Setenv("IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS", "cdn.good.example")
	t.Cleanup(func() { _ = os.Unsetenv("IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS") })

	g := &Gateway{TunerCount: 2, Autopilot: &autopilotStore{byKey: map[string]autopilotDecision{}}}
	ch := &catalog.LiveChannel{DNAID: "dna:new", GuideNumber: "1", GuideName: "Ch", StreamURLs: []string{
		"https://zbad.example.com/a",
		"https://cdn.good.example/b",
	}}
	got := g.reorderStreamURLs(ch, "web", ch.StreamURLs)
	if len(got) != 2 || got[0] != "https://cdn.good.example/b" {
		t.Fatalf("got %v want global preferred host first", got)
	}
}

func TestGateway_reorderStreamURLs_autopilotHostPolicyBlockedHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host-policy.json")
	if err := os.WriteFile(path, []byte(`{"global_blocked_hosts":["bad.example"]}`), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	t.Setenv("IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE", path)
	g := &Gateway{}
	ch := &catalog.LiveChannel{
		GuideName:   "Test",
		GuideNumber: "100",
		StreamURLs:  []string{"http://bad.example/live/1.m3u8", "http://good.example/live/1.m3u8"},
	}
	got := g.reorderStreamURLs(ch, "web", ch.StreamURLs)
	if len(got) != 1 || got[0] != "http://good.example/live/1.m3u8" {
		t.Fatalf("got %v want blocked host removed", got)
	}
}

func TestGateway_reorderStreamURLs_autopilotHostPolicyMergesEnvAndFilePreferredHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host-policy.json")
	if err := os.WriteFile(path, []byte(`{"global_preferred_hosts":["cdn.file.example"]}`), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	t.Setenv("IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE", path)
	t.Setenv("IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS", "cdn.env.example")
	g := &Gateway{}
	ch := &catalog.LiveChannel{
		GuideName:   "Test",
		GuideNumber: "100",
		StreamURLs:  []string{"http://plain.example/live/1.m3u8", "http://cdn.env.example/live/1.m3u8", "http://cdn.file.example/live/1.m3u8"},
	}
	got := g.reorderStreamURLs(ch, "web", ch.StreamURLs)
	if len(got) != 3 || got[0] != "http://cdn.env.example/live/1.m3u8" {
		t.Fatalf("got %v want env preferred host first", got)
	}
}

func TestGateway_reorderStreamURLs_autopilotMemoryBeatsGlobalPreferredHosts(t *testing.T) {
	t.Setenv("IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST", "false")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE", "false")
	t.Setenv("IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS", "cdn.good.example")
	t.Cleanup(func() { _ = os.Unsetenv("IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS") })

	st := &autopilotStore{byKey: map[string]autopilotDecision{
		autopilotKey("dna:x", "web"): {DNAID: "dna:x", ClientClass: "web", PreferredHost: "zbad.example.com", Hits: 3},
	}}
	g := &Gateway{TunerCount: 2, Autopilot: st}
	ch := &catalog.LiveChannel{DNAID: "dna:x", GuideNumber: "1", StreamURLs: []string{
		"https://zbad.example.com/a",
		"https://cdn.good.example/b",
	}}
	got := g.reorderStreamURLs(ch, "web", ch.StreamURLs)
	if len(got) != 2 || got[0] != "https://zbad.example.com/a" {
		t.Fatalf("got %v want per-DNA autopilot memory first", got)
	}
}

func TestGateway_stream_penalizedHostFallsBehindHealthyHost(t *testing.T) {
	hits := []string{}
	penalized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, "penalized")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer penalized.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, "healthy")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer healthy.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{penalized.URL, healthy.URL}},
		},
		TunerCount: 2,
	}

	req1 := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w1 := httptest.NewRecorder()
	g.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first code: %d", w1.Code)
	}
	if len(hits) < 2 || hits[0] != "penalized" || hits[1] != "healthy" {
		t.Fatalf("first hit order=%v", hits)
	}

	hits = nil
	req2 := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second code: %d", w2.Code)
	}
	if len(hits) == 0 || hits[0] != "healthy" {
		t.Fatalf("second hit order=%v want healthy first", hits)
	}
	prof := g.ProviderBehaviorProfile()
	if len(prof.PenalizedHosts) != 1 || prof.PenalizedHosts[0].Host == "" {
		t.Fatalf("penalized_hosts=%+v", prof.PenalizedHosts)
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
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if len(g.recentAttempts) != 1 {
		t.Fatalf("recentAttempts=%d", len(g.recentAttempts))
	}
	if got := g.recentAttempts[0].FinalStatus; got != "upstream_concurrency_limited" {
		t.Fatalf("final status=%q", got)
	}
}

func TestGateway_stream_localTunerLimitRecordsAttemptStatus(t *testing.T) {
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: "http://provider.example/live/1.m3u8"},
		},
		TunerCount: 1,
		inUse:      1,
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
	if len(g.recentAttempts) != 1 {
		t.Fatalf("recentAttempts=%d", len(g.recentAttempts))
	}
	if got := g.recentAttempts[0].FinalStatus; got != "all_tuners_in_use" {
		t.Fatalf("final status=%q", got)
	}
}

func TestGateway_reorderStreamURLsByAccountLoad_prefersFreeAccount(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "1")
	ch := &catalog.LiveChannel{
		StreamURLs: []string{
			"http://provider.example/live/u1/p1/1001.m3u8",
			"http://provider.example/live/u2/p2/1001.m3u8",
		},
		StreamAuths: []catalog.StreamAuth{
			{Prefix: "http://provider.example/live/u1/p1/", User: "u1", Pass: "p1"},
			{Prefix: "http://provider.example/live/u2/p2/", User: "u2", Pass: "p2"},
		},
	}
	g := &Gateway{ProviderUser: "u1", ProviderPass: "p1"}
	identity, ok := providerAccountIdentityForURL(g, ch, ch.StreamURLs[0])
	if !ok {
		t.Fatal("expected account identity")
	}
	g.accountLeases = map[string]int{identity.Key: 1}
	got := g.reorderStreamURLsByAccountLoad(ch, ch.StreamURLs)
	if len(got) != 2 || got[0] != ch.StreamURLs[1] {
		t.Fatalf("reordered urls=%v", got)
	}
}

func TestGateway_reorderStreamURLsByAccountLoad_prefersFreeXtreamPathAccountWithoutStreamAuths(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "1")
	ch := &catalog.LiveChannel{
		StreamURLs: []string{
			"http://provider.example/live/u1/p1/1001.m3u8",
			"http://provider.example/live/u2/p2/1001.m3u8",
		},
	}
	g := &Gateway{ProviderUser: "fallback", ProviderPass: "fallback"}
	identity, ok := providerAccountIdentityForURL(g, ch, ch.StreamURLs[0])
	if !ok {
		t.Fatal("expected account identity from Xtream path")
	}
	g.accountLeases = map[string]int{identity.Key: 1}
	got := g.reorderStreamURLsByAccountLoad(ch, ch.StreamURLs)
	if len(got) != 2 || got[0] != ch.StreamURLs[1] {
		t.Fatalf("reordered urls=%v", got)
	}
}

func TestGateway_authForURL_fallsBackToXtreamPathCredentials(t *testing.T) {
	g := &Gateway{ProviderUser: "fallback", ProviderPass: "fallback"}
	ch := &catalog.LiveChannel{}
	ctx := context.WithValue(context.Background(), gatewayChannelKey{}, ch)
	user, pass := g.authForURL(ctx, "http://provider.example/live/u2/p2/1001.m3u8")
	if user != "u2" || pass != "p2" {
		t.Fatalf("auth = %q/%q; want u2/p2", user, pass)
	}
}

func TestGateway_stream_rollsAcrossThreeXtreamPathAccounts(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "1")
	release := make(chan struct{})
	seen := make(chan string, 4)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("ok"))
			f.Flush()
		}
		<-release
	}))
	defer up.Close()

	ch := catalog.LiveChannel{
		ChannelID:   "100",
		GuideNumber: "100",
		GuideName:   "Test",
		StreamURLs: []string{
			up.URL + "/live/u1/p1/1001.ts",
			up.URL + "/live/u2/p2/1001.ts",
			up.URL + "/live/u3/p3/1001.ts",
		},
	}
	g := &Gateway{
		Channels:     []catalog.LiveChannel{ch},
		TunerCount:   4,
		ProviderUser: "fallback",
		ProviderPass: "fallback",
	}

	type result struct {
		code int
		body string
	}
	run := func() <-chan result {
		done := make(chan result, 1)
		go func() {
			req := httptest.NewRequest(http.MethodGet, "http://local/stream/100", nil)
			w := httptest.NewRecorder()
			g.ServeHTTP(w, req)
			done <- result{code: w.Code, body: w.Body.String()}
		}()
		return done
	}

	done1 := run()
	if got := <-seen; got != "/live/u1/p1/1001.ts" {
		t.Fatalf("first upstream path = %q; want u1", got)
	}
	done2 := run()
	if got := <-seen; got != "/live/u2/p2/1001.ts" {
		t.Fatalf("second upstream path = %q; want u2", got)
	}
	done3 := run()
	if got := <-seen; got != "/live/u3/p3/1001.ts" {
		t.Fatalf("third upstream path = %q; want u3", got)
	}

	req4 := httptest.NewRequest(http.MethodGet, "http://local/stream/100", nil)
	w4 := httptest.NewRecorder()
	g.ServeHTTP(w4, req4)
	if w4.Code != http.StatusServiceUnavailable {
		t.Fatalf("fourth code=%d body=%q; want 503", w4.Code, w4.Body.String())
	}

	close(release)
	for i, done := range []<-chan result{done1, done2, done3} {
		res := <-done
		if res.code != http.StatusOK {
			t.Fatalf("request %d code=%d body=%q", i+1, res.code, res.body)
		}
	}
}

func TestGateway_stream_twoChannelsPreferDifferentXtreamPathAccounts(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "1")
	release := make(chan struct{})
	seen := make(chan string, 4)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("ok"))
			f.Flush()
		}
		<-release
	}))
	defer up.Close()

	channelA := catalog.LiveChannel{
		ChannelID:   "100",
		GuideNumber: "100",
		GuideName:   "Channel A",
		StreamURLs: []string{
			up.URL + "/live/u1/p1/1001.ts",
			up.URL + "/live/u2/p2/1001.ts",
			up.URL + "/live/u3/p3/1001.ts",
		},
	}
	channelB := catalog.LiveChannel{
		ChannelID:   "200",
		GuideNumber: "200",
		GuideName:   "Channel B",
		StreamURLs: []string{
			up.URL + "/live/u1/p1/2001.ts",
			up.URL + "/live/u2/p2/2001.ts",
			up.URL + "/live/u3/p3/2001.ts",
		},
	}
	g := &Gateway{
		Channels:     []catalog.LiveChannel{channelA, channelB},
		TunerCount:   4,
		ProviderUser: "fallback",
		ProviderPass: "fallback",
	}

	type result struct {
		code int
		body string
	}
	run := func(channelID string) <-chan result {
		done := make(chan result, 1)
		go func() {
			req := httptest.NewRequest(http.MethodGet, "http://local/stream/"+channelID, nil)
			w := httptest.NewRecorder()
			g.ServeHTTP(w, req)
			done <- result{code: w.Code, body: w.Body.String()}
		}()
		return done
	}

	done1 := run("100")
	if got := <-seen; got != "/live/u1/p1/1001.ts" {
		t.Fatalf("first upstream path = %q; want u1 channelA", got)
	}
	done2 := run("200")
	if got := <-seen; got != "/live/u2/p2/2001.ts" {
		t.Fatalf("second upstream path = %q; want u2 channelB", got)
	}

	close(release)
	for i, done := range []<-chan result{done1, done2} {
		res := <-done
		if res.code != http.StatusOK {
			t.Fatalf("request %d code=%d body=%q", i+1, res.code, res.body)
		}
	}
}

func TestGateway_stream_threeChannelsUseThreeXtreamPathAccounts(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "1")
	release := make(chan struct{})
	seen := make(chan string, 6)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("ok"))
			f.Flush()
		}
		<-release
	}))
	defer up.Close()

	makeChannel := func(id, suffix string) catalog.LiveChannel {
		return catalog.LiveChannel{
			ChannelID:   id,
			GuideNumber: id,
			GuideName:   "Channel " + id,
			StreamURLs: []string{
				up.URL + "/live/u1/p1/" + suffix + ".ts",
				up.URL + "/live/u2/p2/" + suffix + ".ts",
				up.URL + "/live/u3/p3/" + suffix + ".ts",
			},
		}
	}
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			makeChannel("100", "1001"),
			makeChannel("200", "2001"),
			makeChannel("300", "3001"),
		},
		TunerCount:   4,
		ProviderUser: "fallback",
		ProviderPass: "fallback",
	}

	type result struct {
		code int
		body string
	}
	run := func(channelID string) <-chan result {
		done := make(chan result, 1)
		go func() {
			req := httptest.NewRequest(http.MethodGet, "http://local/stream/"+channelID, nil)
			w := httptest.NewRecorder()
			g.ServeHTTP(w, req)
			done <- result{code: w.Code, body: w.Body.String()}
		}()
		return done
	}

	done1 := run("100")
	if got := <-seen; got != "/live/u1/p1/1001.ts" {
		t.Fatalf("first upstream path = %q; want u1 channel100", got)
	}
	done2 := run("200")
	if got := <-seen; got != "/live/u2/p2/2001.ts" {
		t.Fatalf("second upstream path = %q; want u2 channel200", got)
	}
	done3 := run("300")
	if got := <-seen; got != "/live/u3/p3/3001.ts" {
		t.Fatalf("third upstream path = %q; want u3 channel300", got)
	}

	close(release)
	for i, done := range []<-chan result{done1, done2, done3} {
		res := <-done
		if res.code != http.StatusOK {
			t.Fatalf("request %d code=%d body=%q", i+1, res.code, res.body)
		}
	}
}

func TestGateway_sharedRelaySessionFanout(t *testing.T) {
	g := &Gateway{}
	sess := g.createSharedRelaySession("ch1", "r000001")
	if sess == nil {
		t.Fatal("expected shared relay session")
	}
	reader, ok := g.attachSharedRelaySession(sharedHLSGoRelayKey("ch1"), "r000002")
	if !ok {
		t.Fatal("expected subscriber attach")
	}
	defer reader.Close()
	done := make(chan []byte, 1)
	errs := make(chan error, 1)
	go func() {
		buf, err := io.ReadAll(reader)
		if err != nil {
			errs <- err
			return
		}
		done <- buf
	}()
	writer := &sharedRelayFanoutWriter{base: io.Discard, session: sess}
	if _, err := writer.Write([]byte("segment-bytes")); err != nil {
		t.Fatalf("write: %v", err)
	}
	g.closeSharedRelaySession(sharedHLSGoRelayKey("ch1"), sess)
	select {
	case err := <-errs:
		t.Fatalf("read: %v", err)
	case buf := <-done:
		if string(buf) != "segment-bytes" {
			t.Fatalf("got %q", buf)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for subscriber bytes")
	}
}

func TestGateway_sharedRelaySessionLateSubscriberGetsReplay(t *testing.T) {
	t.Setenv("IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES", "64")
	g := &Gateway{}
	sess := g.createSharedRelaySession("ch1", "r000001")
	if sess == nil {
		t.Fatal("expected shared relay session")
	}
	writer := &sharedRelayFanoutWriter{base: io.Discard, session: sess}
	if _, err := writer.Write([]byte("prefix-")); err != nil {
		t.Fatalf("write prefix: %v", err)
	}
	reader, ok := g.attachSharedRelaySession(sharedHLSGoRelayKey("ch1"), "r000002")
	if !ok {
		t.Fatal("expected late subscriber attach")
	}
	defer reader.Close()
	done := make(chan []byte, 1)
	go func() {
		buf, _ := io.ReadAll(reader)
		done <- buf
	}()
	if _, err := writer.Write([]byte("suffix")); err != nil {
		t.Fatalf("write suffix: %v", err)
	}
	g.closeSharedRelaySession(sharedHLSGoRelayKey("ch1"), sess)
	select {
	case buf := <-done:
		if got := string(buf); got != "prefix-suffix" {
			t.Fatalf("got %q want prefix-suffix", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for replay subscriber bytes")
	}
}

func TestGateway_attachSharedRelaySessionSkipsIdleZeroReplaySession(t *testing.T) {
	t.Setenv("IPTV_TUNERR_SHARED_RELAY_ATTACH_IDLE_TIMEOUT_MS", "50")
	g := &Gateway{}
	sess := g.createSharedRelaySession("ch1", "r000001")
	if sess == nil {
		t.Fatal("expected shared relay session")
	}
	sess.mu.Lock()
	sess.StartedAt = time.Now().Add(-time.Second)
	sess.mu.Unlock()
	reader, ok := g.attachSharedRelaySession(sharedHLSGoRelayKey("ch1"), "r000002")
	if ok {
		if reader != nil {
			_ = reader.Close()
		}
		t.Fatal("expected idle zero-replay shared relay attach to be skipped")
	}
	g.closeSharedRelaySession(sharedHLSGoRelayKey("ch1"), sess)
}

func TestGateway_attachSharedRelaySessionAllowsRecentZeroReplaySession(t *testing.T) {
	t.Setenv("IPTV_TUNERR_SHARED_RELAY_ATTACH_IDLE_TIMEOUT_MS", "1000")
	g := &Gateway{}
	sess := g.createSharedRelaySession("ch1", "r000001")
	if sess == nil {
		t.Fatal("expected shared relay session")
	}
	reader, ok := g.attachSharedRelaySession(sharedHLSGoRelayKey("ch1"), "r000002")
	if !ok || reader == nil {
		t.Fatal("expected recent zero-replay shared relay attach")
	}
	_ = reader.Close()
	g.closeSharedRelaySession(sharedHLSGoRelayKey("ch1"), sess)
}

func TestConfiguredProviderAccountSharedLeaseTTLDefaultIsShort(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_SHARED_LEASE_TTL", "")
	if got := configuredProviderAccountSharedLeaseTTL(); got != 2*time.Minute {
		t.Fatalf("default shared lease ttl = %s, want 2m", got)
	}
}

func TestProviderSharedLeaseSnapshotRemovesExpiredFiles(t *testing.T) {
	dir := t.TempDir()
	mgr := newProviderSharedLeaseManager(dir, "pod-a", time.Minute)
	identity := providerAccountLease{
		Key:   "provider.example|u|p|http://provider.example/live/u/p/",
		Label: "provider.example/u",
		Host:  "provider.example",
	}
	lease, _, acquired, err := mgr.acquire(identity, 2)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if !acquired || lease == nil {
		t.Fatalf("acquired=%v lease=%#v", acquired, lease)
	}
	lease.stopOnce.Do(func() {
		if lease.stopCh != nil {
			close(lease.stopCh)
		}
	})
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(lease.Path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if got := mgr.snapshot(); len(got) != 0 {
		t.Fatalf("snapshot=%#v, want expired lease ignored", got)
	}
	if _, err := os.Stat(lease.Path); !os.IsNotExist(err) {
		t.Fatalf("expired lease file still exists err=%v", err)
	}
}

func TestGateway_tryServeSharedRelay(t *testing.T) {
	g := &Gateway{}
	sess := g.createSharedRelaySession("ch1", "r000001")
	if sess == nil {
		t.Fatal("expected shared relay session")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(10 * time.Millisecond)
		sess.fanout([]byte("ts-bytes"))
		g.closeSharedRelaySession(sharedHLSGoRelayKey("ch1"), sess)
	}()
	req := httptest.NewRequest(http.MethodGet, "/stream/ch1", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	ok := g.tryServeSharedRelay(w, req, &catalog.LiveChannel{ChannelID: "ch1", GuideName: "One", GuideNumber: "101"}, "ch1", "r000002", time.Now())
	if !ok {
		t.Fatal("expected shared relay attach")
	}
	<-done
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body != "ts-bytes" {
		t.Fatalf("body=%q want ts-bytes", body)
	}
	if got := w.Header().Get("X-IptvTunerr-Shared-Upstream"); got != "hls_go" {
		t.Fatalf("header=%q", got)
	}
}

func TestGateway_stream_sameChannelFFmpegRelayReusesExistingSession(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
	script := `#!/bin/sh
printf 'shared-'
sleep 1
printf 'ffmpeg'
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_FIRST_BYTES_TIMEOUT_MS", "2000")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:2.0,\nsegment-1.ts\n"))
	}))
	defer up.Close()

	g := &Gateway{
		TunerCount: 1,
		Channels: []catalog.LiveChannel{{
			ChannelID:   "ff1",
			GuideNumber: "401",
			GuideName:   "Shared FFmpeg",
			StreamURLs:  []string{up.URL + "/live.m3u8"},
		}},
	}

	firstDone := make(chan struct{})
	firstRec := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodGet, "http://local/stream/ff1", nil)
	go func() {
		defer close(firstDone)
		g.ServeHTTP(firstRec, firstReq)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rep := g.SharedRelayReport()
		if rep.Count == 1 && len(rep.Relays) == 1 && rep.Relays[0].SharedUpstream == "hls_ffmpeg" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	rep := g.SharedRelayReport()
	if rep.Count != 1 || len(rep.Relays) != 1 || rep.Relays[0].SharedUpstream != "hls_ffmpeg" {
		t.Fatalf("expected live shared ffmpeg relay, got %#v", rep)
	}

	secondRec := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodGet, "http://local/stream/ff1", nil)
	g.ServeHTTP(secondRec, secondReq)

	<-firstDone
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%q", firstRec.Code, firstRec.Body.String())
	}
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second status=%d body=%q", secondRec.Code, secondRec.Body.String())
	}
	if got := secondRec.Header().Get("X-IptvTunerr-Shared-Upstream"); got != "hls_ffmpeg" {
		t.Fatalf("second shared header=%q", got)
	}
	if got := secondRec.Header().Get("Content-Type"); got != "video/mp2t" {
		t.Fatalf("second content-type=%q", got)
	}
	if secondRec.Body.Len() == 0 {
		t.Fatal("expected second shared viewer to receive bytes")
	}
}

func TestGateway_stream_sameChannelFFmpegFMP4ReusesExistingSession(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
	script := `#!/bin/sh
printf 'shared-'
sleep 1
printf 'fmp4'
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_FIRST_BYTES_TIMEOUT_MS", "2000")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:2.0,\nsegment-1.ts\n"))
	}))
	defer up.Close()

	enable := true
	g := &Gateway{
		TunerCount: 1,
		Channels: []catalog.LiveChannel{{
			ChannelID:   "ff2",
			GuideNumber: "402",
			GuideName:   "Shared FFmpeg fMP4",
			StreamURLs:  []string{up.URL + "/live.m3u8"},
		}},
		NamedProfiles: map[string]NamedStreamProfile{
			"mobile-fmp4": {
				BaseProfile: profileLowBitrate,
				Transcode:   &enable,
				OutputMux:   streamMuxFMP4,
			},
		},
	}

	firstDone := make(chan struct{})
	firstRec := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodGet, "http://local/stream/ff2?profile=mobile-fmp4", nil)
	go func() {
		defer close(firstDone)
		g.ServeHTTP(firstRec, firstReq)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rep := g.SharedRelayReport()
		if rep.Count == 1 && len(rep.Relays) == 1 && rep.Relays[0].SharedUpstream == "hls_ffmpeg" && rep.Relays[0].ContentType == "video/mp4" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	rep := g.SharedRelayReport()
	if rep.Count != 1 || len(rep.Relays) != 1 || rep.Relays[0].SharedUpstream != "hls_ffmpeg" || rep.Relays[0].ContentType != "video/mp4" {
		t.Fatalf("expected live shared ffmpeg fmp4 relay, got %#v", rep)
	}

	secondRec := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodGet, "http://local/stream/ff2?profile=mobile-fmp4", nil)
	g.ServeHTTP(secondRec, secondReq)

	<-firstDone
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%q", firstRec.Code, firstRec.Body.String())
	}
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second status=%d body=%q", secondRec.Code, secondRec.Body.String())
	}
	if got := secondRec.Header().Get("X-IptvTunerr-Shared-Upstream"); got != "hls_ffmpeg" {
		t.Fatalf("second shared header=%q", got)
	}
	if got := secondRec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("second content-type=%q", got)
	}
	if secondRec.Body.Len() == 0 {
		t.Fatal("expected second shared fmp4 viewer to receive bytes")
	}
}

func TestGateway_stream_providerAccountLimitRejectsLocally(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "1")
	ch := catalog.LiveChannel{
		GuideNumber: "1",
		GuideName:   "Ch1",
		StreamURLs:  []string{"http://provider.example/live/u1/p1/1001.m3u8"},
		StreamAuths: []catalog.StreamAuth{{Prefix: "http://provider.example/live/u1/p1/", User: "u1", Pass: "p1"}},
	}
	g := &Gateway{
		Channels:     []catalog.LiveChannel{ch},
		TunerCount:   4,
		ProviderUser: "u1",
		ProviderPass: "p1",
	}
	identity, ok := providerAccountIdentityForURL(g, &ch, ch.StreamURLs[0])
	if !ok {
		t.Fatal("expected account identity")
	}
	g.accountLeases = map[string]int{identity.Key: 1}

	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d", w.Code)
	}
	if got := g.recentAttempts[0].FinalStatus; got != "provider_accounts_in_use" {
		t.Fatalf("final status=%q", got)
	}
}

func TestGateway_stream_successReleasesProviderAccountLease(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ch := catalog.LiveChannel{
		GuideNumber: "1",
		GuideName:   "Ch1",
		StreamURLs:  []string{srv.URL},
		StreamAuths: []catalog.StreamAuth{{Prefix: srv.URL, User: "u1", Pass: "p1"}},
	}
	g := &Gateway{
		Channels:     []catalog.LiveChannel{ch},
		TunerCount:   2,
		ProviderUser: "u1",
		ProviderPass: "p1",
	}

	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
	}
	if leases := g.providerAccountLeases(); len(leases) != 0 {
		t.Fatalf("expected provider account leases to be released, got %#v", leases)
	}
}

func TestGateway_sharedProviderAccountLeaseBlocksSecondGateway(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "1")
	sharedDir := t.TempDir()
	mgrA := newProviderSharedLeaseManager(sharedDir, "pod-a", time.Hour)
	mgrB := newProviderSharedLeaseManager(sharedDir, "pod-b", time.Hour)
	ch := &catalog.LiveChannel{
		StreamURLs:  []string{"http://provider.example/live/u1/p1/1001.m3u8"},
		StreamAuths: []catalog.StreamAuth{{Prefix: "http://provider.example/live/u1/p1/", User: "u1", Pass: "p1"}},
	}
	gA := &Gateway{ProviderUser: "u1", ProviderPass: "p1", sharedAccountLeases: mgrA}
	gB := &Gateway{ProviderUser: "u1", ProviderPass: "p1", sharedAccountLeases: mgrB}

	identityA, leaseA, heldA, okA := gA.tryAcquireProviderAccountLease(ch, ch.StreamURLs[0])
	if !heldA || !okA {
		t.Fatalf("first acquire held=%v ok=%v identity=%#v", heldA, okA, identityA)
	}
	defer gA.releaseProviderAccountLease(leaseA)

	identityB, _, heldB, okB := gB.tryAcquireProviderAccountLease(ch, ch.StreamURLs[0])
	if !heldB || okB {
		t.Fatalf("second acquire held=%v ok=%v identity=%#v", heldB, okB, identityB)
	}
	if got := gB.providerAccountLeaseCount(identityB.Key); got != 1 {
		t.Fatalf("shared lease count=%d want 1", got)
	}
}

func TestGateway_stream_learnsProviderAccountLimit(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_LIMIT", "0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "maximum 1 connections allowed", 423)
	}))
	defer srv.Close()

	ch := catalog.LiveChannel{
		GuideNumber: "1",
		GuideName:   "Ch1",
		StreamURLs:  []string{srv.URL},
		StreamAuths: []catalog.StreamAuth{{Prefix: srv.URL, User: "u1", Pass: "p1"}},
	}
	g := &Gateway{
		Channels:     []catalog.LiveChannel{ch},
		TunerCount:   4,
		ProviderUser: "u1",
		ProviderPass: "p1",
	}

	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d", w.Code)
	}
	limits := g.providerAccountLearnedLimits()
	if len(limits) != 1 {
		t.Fatalf("learned account limits=%#v", limits)
	}
	if limits[0].LearnedLimit != 1 || limits[0].SignalCount != 1 {
		t.Fatalf("limit state=%#v", limits[0])
	}
	if got := g.effectiveProviderAccountLimitForKey(&ch, providerAccountMustKey(t, g, &ch, srv.URL)); got != 1 {
		t.Fatalf("effective account limit=%d want 1", got)
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

func TestIsUpstreamConcurrencyLimit_509(t *testing.T) {
	if !isUpstreamConcurrencyLimit(509, "") {
		t.Fatal("expected 509 to count as upstream concurrency limit")
	}
}

func TestGateway_shouldPreferGoRelayForHLS(t *testing.T) {
	g := &Gateway{TunerCount: 4, learnedUpstreamLimit: 2}
	if !g.shouldPreferGoRelayForHLS("http://provider.example/live/1.m3u8", false) {
		t.Fatal("expected learned lower upstream limit to prefer go relay")
	}

	t.Setenv("IPTV_TUNERR_HLS_RELAY_PREFER_GO_ON_PROVIDER_PRESSURE", "false")
	if g.shouldPreferGoRelayForHLS("http://provider.example/live/1.m3u8", false) {
		t.Fatal("expected provider-pressure preference to be disable-able")
	}

	t.Setenv("IPTV_TUNERR_HLS_RELAY_PREFER_GO", "true")
	if !g.shouldPreferGoRelayForHLS("http://provider.example/live/1.m3u8", false) {
		t.Fatal("expected explicit go-relay preference override")
	}
	if !g.shouldPreferGoRelayForHLS("http://provider.example/live/1.m3u8", true) {
		t.Fatal("expected explicit go-relay preference override to apply to transcode HLS too")
	}
	t.Setenv("IPTV_TUNERR_HLS_RELAY_PREFER_GO", "false")
	if g.shouldPreferGoRelayForHLS("http://provider.example/live/1.m3u8", true) {
		t.Fatal("expected provider-pressure heuristics to remain remux-only when transcode is requested")
	}
}

func TestGateway_shouldPreferGoRelayForHLS_hostPenalty(t *testing.T) {
	t.Run("penalized_host", func(t *testing.T) {
		g := &Gateway{TunerCount: 4}
		g.noteUpstreamFailure("http://provider.example/live/1.m3u8", 0, "ffmpeg_hls_failed")
		if !g.shouldPreferGoRelayForHLS("http://provider.example/live/2.m3u8", false) {
			t.Fatal("expected host penalty to prefer go relay")
		}
	})
	t.Run("autotune_off_no_penalty_signal", func(t *testing.T) {
		t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE", "false")
		g := &Gateway{TunerCount: 4}
		g.noteUpstreamFailure("http://provider.example/live/1.m3u8", 0, "ffmpeg_hls_failed")
		if g.shouldPreferGoRelayForHLS("http://provider.example/live/2.m3u8", false) {
			t.Fatal("expected no host-penalty go-relay when autotune is off")
		}
	})
	t.Run("remux_penalty_survives_playlist_success", func(t *testing.T) {
		g := &Gateway{TunerCount: 4}
		g.noteHLSRemuxFailure("http://provider.example/live/1.m3u8")
		g.noteUpstreamSuccess("http://provider.example/live/1.m3u8")
		if !g.shouldPreferGoRelayForHLS("http://provider.example/live/2.m3u8", false) {
			t.Fatal("expected remux-specific penalty to survive generic playlist success")
		}
		g.noteHLSRemuxSuccess("http://provider.example/live/1.m3u8")
		if g.shouldPreferGoRelayForHLS("http://provider.example/live/2.m3u8", false) {
			t.Fatal("expected remux-specific penalty to clear after remux success")
		}
	})
}

func TestGateway_filterQuarantinedUpstreams_prefersHealthyAlternatives(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE", "true")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_AFTER", "3")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_SEC", "900")
	g := &Gateway{TunerCount: 4}
	g.noteUpstreamFailure("http://bad.example/live/1.m3u8", 502, "http_status")
	g.noteUpstreamFailure("http://bad.example/live/1.m3u8", 502, "http_status")
	g.noteUpstreamFailure("http://bad.example/live/1.m3u8", 502, "http_status")
	got := g.filterQuarantinedUpstreams([]string{
		"http://bad.example/live/1.m3u8",
		"http://good.example/live/1.m3u8",
	})
	if len(got) != 1 || got[0] != "http://good.example/live/1.m3u8" {
		t.Fatalf("got %v", got)
	}
}

func TestGateway_filterQuarantinedUpstreams_keepsOnlyChoice(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE", "true")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_AFTER", "1")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_SEC", "900")
	g := &Gateway{TunerCount: 4}
	g.noteUpstreamFailure("http://only.example/live/1.m3u8", 502, "http_status")
	got := g.filterQuarantinedUpstreams([]string{"http://only.example/live/1.m3u8"})
	if len(got) != 1 || got[0] != "http://only.example/live/1.m3u8" {
		t.Fatalf("got %v", got)
	}
}

func TestGateway_filterQuarantinedUpstreams_respectsAutotuneOff(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE", "false")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE", "true")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_AFTER", "1")
	g := &Gateway{TunerCount: 4}
	g.noteUpstreamFailure("http://bad.example/live/1.m3u8", 502, "http_status")
	got := g.filterQuarantinedUpstreams([]string{
		"http://bad.example/live/1.m3u8",
		"http://good.example/live/1.m3u8",
	})
	if len(got) != 2 {
		t.Fatalf("got %v want both URLs when autotune is off", got)
	}
}

func TestGateway_filterQuarantinedUpstreams_dropsMultipleQuarantined(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE", "true")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_AFTER", "1")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_SEC", "900")
	g := &Gateway{TunerCount: 4}
	for _, u := range []string{
		"http://bad-a.example/live/1.m3u8",
		"http://bad-b.example/live/1.m3u8",
	} {
		g.noteUpstreamFailure(u, 502, "http_status")
	}
	got := g.filterQuarantinedUpstreams([]string{
		"http://bad-a.example/live/1.m3u8",
		"http://bad-b.example/live/1.m3u8",
		"http://ok.example/live/1.m3u8",
	})
	if len(got) != 1 || got[0] != "http://ok.example/live/1.m3u8" {
		t.Fatalf("got %v", got)
	}
}

func TestGateway_stream_skipsQuarantinedPrimaryUsesBackup(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE", "true")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE", "true")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_AFTER", "3")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_SEC", "900")

	var primaryHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&primaryHits, 1)
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
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{primary.URL, backup.URL}},
		},
		TunerCount: 2,
	}
	for i := 0; i < 3; i++ {
		g.noteUpstreamFailure(primary.URL, 502, "http_status")
	}

	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	if w.Body.String() != "backup" {
		t.Fatalf("body: %q", w.Body.String())
	}
	if atomic.LoadInt32(&primaryHits) != 0 {
		t.Fatalf("primary upstream was hit %d times; want 0 when quarantined", primaryHits)
	}
}

func TestGateway_learnsUpstreamConcurrencyLimitAndRejectsLocally(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_LIMIT", "0")
	hits := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	prof := g.ProviderBehaviorProfile()
	if prof.ConcurrencySignalsSeen != 1 {
		t.Fatalf("concurrency_signals_seen=%d want 1", prof.ConcurrencySignalsSeen)
	}
	if prof.LastConcurrencyStatus != 423 {
		t.Fatalf("last_concurrency_status=%d want 423", prof.LastConcurrencyStatus)
	}
	if prof.EffectiveTunerLimit != 1 {
		t.Fatalf("effective_tuner_limit=%d want 1", prof.EffectiveTunerLimit)
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

func TestGateway_providerAccountLimitAutoUsesLineupFeedCapacity(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "auto")
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: "http://example.test/live/u/p/1.m3u8", StreamURLs: []string{"http://example.test/live/u/p/1.m3u8", "http://backup.test/live/u/p/1.m3u8"}},
			{GuideNumber: "2", GuideName: "Ch2", StreamURL: "http://example.test/live/u/p/2.m3u8", StreamURLs: []string{"http://example.test/live/u/p/2.m3u8"}},
		},
		TunerCount: 3,
	}
	prof := g.ProviderBehaviorProfile()
	if prof.AccountPoolLimit != 3 {
		t.Fatalf("account_pool_limit=%d want 3", prof.AccountPoolLimit)
	}
	if !prof.AccountPoolConfigured {
		t.Fatal("expected account pool to be configured")
	}
}

func TestGateway_stream_concurrencyLimitRetriesThenFallback(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_LIMIT", "2")
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_BACKOFF_MS", "1")

	var hits int32
	var backupHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "max connections reached", 423)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&backupHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backup"))
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
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if w.Body.String() != "backup" {
		t.Fatalf("body=%q", w.Body.String())
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("primary hits=%d want=3", got)
	}
	if got := atomic.LoadInt32(&backupHits); got != 1 {
		t.Fatalf("backup hits=%d want=1", got)
	}
}

func TestGateway_stream_nonConcurrencyErrorsStillFallbackImmediately(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_LIMIT", "2")
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_BACKOFF_MS", "1")

	var primaryHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&primaryHits, 1)
		http.Error(w, "server error", 500)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backup"))
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
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if got := atomic.LoadInt32(&primaryHits); got != 1 {
		t.Fatalf("primary hits=%d want=1", got)
	}
}

func TestGateway_ProviderBehaviorProfile_quarantinedHosts(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE", "true")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_AFTER", "2")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_SEC", "900")
	g := &Gateway{TunerCount: 4}
	g.noteUpstreamFailure("http://bad.example/live/1.m3u8", 502, "http_status")
	g.noteUpstreamFailure("http://bad.example/live/1.m3u8", 502, "http_status")
	p := g.ProviderBehaviorProfile()
	if !p.AutoHostQuarantine {
		t.Fatal("expected auto_host_quarantine")
	}
	if len(p.QuarantinedHosts) != 1 || p.QuarantinedHosts[0].Host != "bad.example" {
		t.Fatalf("quarantined_hosts=%#v", p.QuarantinedHosts)
	}
	if p.QuarantinedHosts[0].QuarantinedUntil == "" {
		t.Fatalf("missing quarantined_until: %#v", p.QuarantinedHosts[0])
	}
}

func TestGateway_ProviderBehaviorProfile_upstreamQuarantineSkipsTotal(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE", "true")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_AFTER", "3")
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_SEC", "900")
	g := &Gateway{TunerCount: 4}
	for i := 0; i < 3; i++ {
		g.noteUpstreamFailure("http://bad.example/live/1.m3u8", 502, "http_status")
	}
	g.filterQuarantinedUpstreams([]string{
		"http://bad.example/live/1.m3u8",
		"http://good.example/live/1.m3u8",
	})
	p := g.ProviderBehaviorProfile()
	if p.UpstreamQuarantineSkipsTotal != 1 {
		t.Fatalf("upstream_quarantine_skips_total=%d want 1", p.UpstreamQuarantineSkipsTotal)
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

func TestGateway_requestAdaptation_unknownPolicyDirect(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PLEX_UNKNOWN_CLIENT_POLICY", "direct")
	g := &Gateway{PlexClientAdapt: true}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test", nil)

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected direct override for unknown client")
	}
	if transcode {
		t.Fatalf("expected unknown client direct policy to keep transcode off")
	}
	if profile != "" {
		t.Fatalf("profile=%q want empty", profile)
	}
	if reason != "unknown-client-websafe" {
		t.Fatalf("reason=%q", reason)
	}
	if clientClass != "unknown" {
		t.Fatalf("clientClass=%q want unknown", clientClass)
	}
}

func TestGateway_requestAdaptation_stickyFallbackWebsafe(t *testing.T) {
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK", "true")
	key := "ch1" + adaptStickyKeySep + "sid-a" + adaptStickyKeySep + "-"
	g := &Gateway{
		PlexClientAdapt: true,
		adaptStickyUntil: map[string]time.Time{
			key: time.Now().Add(time.Hour),
		},
	}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/ch1", nil)
	req.Header.Set("X-Plex-Session-Identifier", "sid-a")

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "ch1")
	if !hasOverride || !transcode || profile != profilePlexSafe || reason != "sticky-fallback-websafe" {
		t.Fatalf("override=%v trans=%v profile=%q reason=%q", hasOverride, transcode, profile, reason)
	}
	if clientClass != "unknown" {
		t.Fatalf("clientClass=%q want unknown without PMS", clientClass)
	}
}

func TestGateway_requestAdaptation_forceWebsafeProfileOverride(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FORCE_WEBSAFE", "true")
	t.Setenv("IPTV_TUNERR_FORCE_WEBSAFE_PROFILE", "plexsafehq")
	g := &Gateway{PlexClientAdapt: true}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test", nil)

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride || !transcode {
		t.Fatalf("override=%v trans=%v", hasOverride, transcode)
	}
	if profile != profilePlexSafeHQ {
		t.Fatalf("profile=%q want %q", profile, profilePlexSafeHQ)
	}
	if reason != "force-websafe" {
		t.Fatalf("reason=%q", reason)
	}
	if clientClass != "manual" {
		t.Fatalf("clientClass=%q want manual", clientClass)
	}
}

func TestGateway_adaptSticky_noteAndHonor(t *testing.T) {
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK", "true")
	g := &Gateway{PlexClientAdapt: true}
	h := plexForwardedHints{SessionIdentifier: "s1", ClientIdentifier: "c1"}
	g.noteAdaptStickyFallback("ch1", h)
	if !g.shouldAdaptStickyWebsafe("ch1", h) {
		t.Fatal("expected sticky active after note")
	}
}

func TestGateway_adaptSticky_skipsWithoutPlexHints(t *testing.T) {
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK", "true")
	g := &Gateway{PlexClientAdapt: true}
	g.noteAdaptStickyFallback("ch1", plexForwardedHints{})
	g.adaptStickyMu.Lock()
	n := len(g.adaptStickyUntil)
	g.adaptStickyMu.Unlock()
	if n != 0 {
		t.Fatalf("expected no sticky without session/client id, got %d entries", n)
	}
}

func TestGateway_adaptSticky_unknownInternalFetcherKeyedByChannel(t *testing.T) {
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK", "true")
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_UNKNOWN_INTERNAL_STICKY_FALLBACK", "true")
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_UNKNOWN_INTERNAL_GLOBAL_FALLBACK", "false")
	g := &Gateway{PlexClientAdapt: true}
	g.noteAdaptStickyFallbackForRequest("ch1", plexForwardedHints{}, "Lavf/60.16.100")
	if !g.shouldAdaptStickyWebsafeForRequest("ch1", plexForwardedHints{}, "Lavf/60.16.100") {
		t.Fatal("expected sticky for unknown internal fetcher")
	}
	if g.shouldAdaptStickyWebsafeForRequest("ch2", plexForwardedHints{}, "Lavf/60.16.100") {
		t.Fatal("did not expect sticky on different channel")
	}
}

func TestGateway_adaptSticky_unknownInternalFetcherDisabled(t *testing.T) {
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK", "true")
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_UNKNOWN_INTERNAL_STICKY_FALLBACK", "false")
	g := &Gateway{PlexClientAdapt: true}
	g.noteAdaptStickyFallbackForRequest("ch1", plexForwardedHints{}, "Lavf/60.16.100")
	if g.shouldAdaptStickyWebsafeForRequest("ch1", plexForwardedHints{}, "Lavf/60.16.100") {
		t.Fatal("did not expect sticky when unknown internal fallback disabled")
	}
}

func TestGateway_adaptSticky_unknownInternalFetcherGlobal(t *testing.T) {
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK", "true")
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_UNKNOWN_INTERNAL_STICKY_FALLBACK", "true")
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_UNKNOWN_INTERNAL_GLOBAL_FALLBACK", "true")
	g := &Gateway{PlexClientAdapt: true}
	g.noteAdaptStickyFallbackForRequest("ch1", plexForwardedHints{}, "Lavf/60.16.100")
	if !g.shouldAdaptStickyWebsafeForRequest("ch2", plexForwardedHints{}, "Lavf/60.16.100") {
		t.Fatal("expected sticky on different channel when global unknown-internal fallback enabled")
	}
}

func TestGateway_adaptSticky_respectsMasterSwitch(t *testing.T) {
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK", "false")
	g := &Gateway{PlexClientAdapt: true}
	h := plexForwardedHints{SessionIdentifier: "s1"}
	g.noteAdaptStickyFallback("ch1", h)
	if g.shouldAdaptStickyWebsafe("ch1", h) {
		t.Fatal("did not expect sticky when fallback disabled")
	}
}

func TestGateway_adaptSticky_requiresClientAdapt(t *testing.T) {
	t.Setenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK", "true")
	g := &Gateway{
		PlexClientAdapt: false,
		adaptStickyUntil: map[string]time.Time{
			"x" + adaptStickyKeySep + "s" + adaptStickyKeySep + "c": time.Now().Add(time.Hour),
		},
	}
	if g.shouldAdaptStickyWebsafe("x", plexForwardedHints{SessionIdentifier: "s", ClientIdentifier: "c"}) {
		t.Fatal("expected no sticky when CLIENT_ADAPT-equivalent off")
	}
}

func TestGateway_requestAdaptation_queryProfilePMSXcode(t *testing.T) {
	g := &Gateway{PlexClientAdapt: true}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test?profile=pmsxcode", nil)
	hasOverride, transcode, profile, reason, _ := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride || !transcode || profile != profilePMSXcode || reason != "query-profile" {
		t.Fatalf("override=%v trans=%v profile=%q reason=%q", hasOverride, transcode, profile, reason)
	}
}

func TestGateway_requestAdaptation_queryProfileHDHRAlias(t *testing.T) {
	g := &Gateway{PlexClientAdapt: true}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test?profile=internet360", nil)
	_, transcode, profile, reason, _ := g.requestAdaptation(context.Background(), req, ch, "test")
	if !transcode || profile != profileAACCFR || reason != "query-profile" {
		t.Fatalf("trans=%v profile=%q reason=%q", transcode, profile, reason)
	}
}

func TestGateway_requestAdaptation_queryProfileNamedProfile(t *testing.T) {
	enable := true
	g := &Gateway{
		PlexClientAdapt: true,
		NamedProfiles: map[string]NamedStreamProfile{
			"mobile-fmp4": {
				BaseProfile: profileLowBitrate,
				Transcode:   &enable,
				OutputMux:   streamMuxFMP4,
			},
		},
	}
	ch := &catalog.LiveChannel{GuideName: "Test"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/test?profile=mobile-fmp4", nil)
	hasOverride, transcode, profile, reason, _ := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride || !transcode || profile != "mobile-fmp4" || reason != "query-profile" {
		t.Fatalf("override=%v trans=%v profile=%q reason=%q", hasOverride, transcode, profile, reason)
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

func TestGateway_requestAdaptation_resolvedWebUsesWebProfileOverride(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FORCE_WEBSAFE_PROFILE", "plexsafehq")
	t.Setenv("IPTV_TUNERR_PLEX_WEB_CLIENT_PROFILE", "copyvideomp3")
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
	if !hasOverride || !transcode {
		t.Fatalf("override=%v transcode=%v", hasOverride, transcode)
	}
	if profile != profileCopyVideoMP3 {
		t.Fatalf("profile=%q want %q", profile, profileCopyVideoMP3)
	}
	if reason != "resolved-web-client" || clientClass != "web" {
		t.Fatalf("reason=%q clientClass=%q", reason, clientClass)
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

func TestGateway_requestAdaptation_internalFetcherUsesInternalProfileOverride(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FORCE_WEBSAFE_PROFILE", "copyvideomp3")
	t.Setenv("IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE", "plexsafehq")
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
	if !hasOverride || !transcode {
		t.Fatalf("override=%v transcode=%v", hasOverride, transcode)
	}
	if profile != profilePlexSafeHQ {
		t.Fatalf("profile=%q want %q", profile, profilePlexSafeHQ)
	}
	if reason != "internal-fetcher-websafe" || clientClass != "internal" {
		t.Fatalf("reason=%q clientClass=%q", reason, clientClass)
	}
}

func TestGateway_requestAdaptation_internalFetcherPolicyDirect(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY", "direct")
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
	if transcode {
		t.Fatalf("expected internal fetcher direct policy to keep transcode off")
	}
	if profile != "" {
		t.Fatalf("profile=%q want empty", profile)
	}
	if reason != "internal-fetcher-websafe" {
		t.Fatalf("reason=%q want internal-fetcher-websafe", reason)
	}
	if clientClass != "internal" {
		t.Fatalf("clientClass=%q want internal", clientClass)
	}
}

func TestGateway_requestAdaptation_unknownInternalFetcherInfersSingleWebSession(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FORCE_WEBSAFE_PROFILE", "copyvideomp3")
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer size="2"><Video title="Live TV"><Session id="sid-lavf"/><Player machineIdentifier="cid-lavf" product="Lavf" platform="Plex Media Server"/></Video><Video title="Live TV"><Session id="sid-web"/><Player machineIdentifier="cid-web" product="Plex Web" platform="Firefox"/></Video></MediaContainer>`))
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
	req.Header.Set("User-Agent", "Lavf/60.16.100")

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected override for inferred web session")
	}
	if !transcode {
		t.Fatalf("expected inferred web session to use fallback profile")
	}
	if profile != profileCopyVideoMP3 {
		t.Fatalf("profile=%q want %q", profile, profileCopyVideoMP3)
	}
	if reason != "resolved-web-client" {
		t.Fatalf("reason=%q want resolved-web-client", reason)
	}
	if clientClass != "web" {
		t.Fatalf("clientClass=%q want web", clientClass)
	}
}

func TestGateway_requestAdaptation_unknownInternalFetcherInfersSingleNativeSession(t *testing.T) {
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer size="2"><Video title="Live TV"><Session id="sid-lavf"/><Player machineIdentifier="cid-lavf" product="Lavf" platform="Plex Media Server"/></Video><Video title="Live TV"><Session id="sid-tv"/><Player machineIdentifier="cid-tv" product="Plex for LG" platform="LG"/></Video></MediaContainer>`))
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
	req.Header.Set("User-Agent", "Lavf/60.16.100")

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected override for inferred native session")
	}
	if transcode {
		t.Fatalf("expected inferred native session to stay direct")
	}
	if profile != "" {
		t.Fatalf("profile=%q want empty", profile)
	}
	if reason != "resolved-nonweb-client" {
		t.Fatalf("reason=%q want resolved-nonweb-client", reason)
	}
	if clientClass != "native" {
		t.Fatalf("clientClass=%q want native", clientClass)
	}
}

func TestGateway_requestAdaptation_unknownInternalFetcherAmbiguousFallsBackToInternalPolicy(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FORCE_WEBSAFE_PROFILE", "copyvideomp3")
	t.Setenv("IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE", "plexsafehq")
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer size="3"><Video title="Live TV"><Session id="sid-lavf"/><Player machineIdentifier="cid-lavf" product="Lavf" platform="Plex Media Server"/></Video><Video title="Live TV"><Session id="sid-web"/><Player machineIdentifier="cid-web" product="Plex Web" platform="Firefox"/></Video><Video title="Live TV"><Session id="sid-tv"/><Player machineIdentifier="cid-tv" product="Plex for LG" platform="LG"/></Video></MediaContainer>`))
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
	req.Header.Set("User-Agent", "Lavf/60.16.100")

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected override for ambiguous internal fetcher")
	}
	if !transcode {
		t.Fatalf("expected ambiguous internal fetcher to use internal fetcher policy")
	}
	if profile != profilePlexSafeHQ {
		t.Fatalf("profile=%q want %q", profile, profilePlexSafeHQ)
	}
	if reason != "ambiguous-internal-fetcher-websafe" {
		t.Fatalf("reason=%q want ambiguous-internal-fetcher-websafe", reason)
	}
	if clientClass != "internal" {
		t.Fatalf("clientClass=%q want internal", clientClass)
	}
}

func TestGateway_requestAdaptation_unknownInternalFetcherAmbiguousPolicyDirect(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY", "direct")
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer size="3"><Video title="Live TV"><Session id="sid-lavf"/><Player machineIdentifier="cid-lavf" product="Lavf" platform="Plex Media Server"/></Video><Video title="Live TV"><Session id="sid-web"/><Player machineIdentifier="cid-web" product="Plex Web" platform="Firefox"/></Video><Video title="Live TV"><Session id="sid-tv"/><Player machineIdentifier="cid-tv" product="Plex for LG" platform="LG"/></Video></MediaContainer>`))
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
	req.Header.Set("User-Agent", "Lavf/60.16.100")

	hasOverride, transcode, profile, reason, clientClass := g.requestAdaptation(context.Background(), req, ch, "test")
	if !hasOverride {
		t.Fatalf("expected override for ambiguous internal fetcher")
	}
	if transcode {
		t.Fatalf("expected ambiguous internal fetcher direct policy to keep transcode off")
	}
	if profile != "" {
		t.Fatalf("profile=%q want empty", profile)
	}
	if reason != "ambiguous-internal-fetcher-websafe" {
		t.Fatalf("reason=%q want ambiguous-internal-fetcher-websafe", reason)
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

func TestGateway_hotStartConfigFromAutopilotHits(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HOT_START_ENABLED", "true")
	t.Setenv("IPTV_TUNERR_HOT_START_MIN_HITS", "3")
	g := &Gateway{
		Autopilot: &autopilotStore{
			byKey: map[string]autopilotDecision{
				autopilotKey("dna:test", "web"): {
					DNAID:       "dna:test",
					ClientClass: "web",
					Hits:        4,
				},
			},
		},
	}
	cfg := g.hotStartConfig(&catalog.LiveChannel{DNAID: "dna:test", ChannelID: "1001", GuideNumber: "101"}, "web")
	if !cfg.Enabled {
		t.Fatal("expected hot-start enabled")
	}
	if cfg.Reason != "autopilot_hits" {
		t.Fatalf("reason=%q want autopilot_hits", cfg.Reason)
	}
}

func TestGateway_hotStartConfigFromFavoriteList(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HOT_START_ENABLED", "true")
	t.Setenv("IPTV_TUNERR_HOT_START_CHANNELS", "1001,fox news")
	g := &Gateway{}
	cfg := g.hotStartConfig(&catalog.LiveChannel{ChannelID: "1001", GuideName: "FOX News"}, "web")
	if !cfg.Enabled {
		t.Fatal("expected favorite hot-start enabled")
	}
	if cfg.Reason != "favorite" {
		t.Fatalf("reason=%q want favorite", cfg.Reason)
	}
}

func TestGateway_shouldAutoEnableHLSReconnect(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE", "true")
	_ = os.Unsetenv("IPTV_TUNERR_FFMPEG_HLS_RECONNECT")

	g := &Gateway{}
	if g.shouldAutoEnableHLSReconnect() {
		t.Fatalf("expected auto reconnect off before failures")
	}
	g.noteHLSPlaylistFailure("http://provider.example/live/a.m3u8")
	if !g.shouldAutoEnableHLSReconnect() {
		t.Fatalf("expected auto reconnect on after hls playlist failure")
	}
	prof := g.ProviderBehaviorProfile()
	if !prof.AutoHLSReconnect {
		t.Fatalf("expected provider profile auto_hls_reconnect=true")
	}
	if prof.HLSPlaylistFailures != 1 {
		t.Fatalf("hls_playlist_failures=%d want 1", prof.HLSPlaylistFailures)
	}
}

func TestGateway_shouldAutoEnableHLSReconnectRespectsExplicitEnv(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE", "true")
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_RECONNECT", "false")

	g := &Gateway{}
	g.noteHLSSegmentFailure("http://provider.example/live/seg.ts")
	if g.shouldAutoEnableHLSReconnect() {
		t.Fatalf("expected explicit env to disable auto reconnect override")
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
	prevTimeout := hlsRelayNoProgressTimeout
	prevRefreshSleep := hlsRelayRefreshSleep
	hlsRelayNoProgressTimeout = 50 * time.Millisecond
	hlsRelayRefreshSleep = func([]byte) {}
	t.Cleanup(func() {
		hlsRelayNoProgressTimeout = prevTimeout
		hlsRelayRefreshSleep = prevRefreshSleep
	})

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
	block := g.ffmpegInputHeaderBlock(req, "http://cdn.example/live/u/p/1.m3u8", "cdn.example")
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

func TestParseCustomHeaders(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		expect map[string]string
	}{
		{
			name:   "empty",
			raw:    "",
			expect: map[string]string{},
		},
		{
			name:   "single header",
			raw:    "Referer: http://example.com",
			expect: map[string]string{"Referer": "http://example.com"},
		},
		{
			name:   "multiple headers",
			raw:    "Referer: http://example.com,Origin: http://example.com,X-Custom: value",
			expect: map[string]string{"Referer": "http://example.com", "Origin": "http://example.com", "X-Custom": "value"},
		},
		{
			name:   "with spaces",
			raw:    "  Referer :  http://example.com  ,  Origin :  http://example.com  ",
			expect: map[string]string{"Referer": "http://example.com", "Origin": "http://example.com"},
		},
		{
			name:   "value with colon",
			raw:    "Authorization: Bearer token:with:colons",
			expect: map[string]string{"Authorization": "Bearer token:with:colons"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCustomHeaders(tt.raw)
			if len(got) != len(tt.expect) {
				t.Fatalf("parseCustomHeaders(%q): got %d headers, want %d", tt.raw, len(got), len(tt.expect))
			}
			for k, v := range tt.expect {
				if got[k] != v {
					t.Errorf("parseCustomHeaders(%q)[%q] = %q, want %q", tt.raw, k, got[k], v)
				}
			}
		})
	}
}

func TestGateway_applyUpstreamRequestHeaders_customHeaders(t *testing.T) {
	g := &Gateway{
		CustomHeaders: map[string]string{
			"Referer":  "http://provider.example.com",
			"Origin":   "http://provider.example.com",
			"X-Custom": "custom-value",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/segment.ts", nil)
	g.applyUpstreamRequestHeaders(req, nil)
	if req.Header.Get("Referer") != "http://provider.example.com" {
		t.Errorf("Referer = %q, want http://provider.example.com", req.Header.Get("Referer"))
	}
	if req.Header.Get("Origin") != "http://provider.example.com" {
		t.Errorf("Origin = %q, want http://provider.example.com", req.Header.Get("Origin"))
	}
	if req.Header.Get("X-Custom") != "custom-value" {
		t.Errorf("X-Custom = %q, want custom-value", req.Header.Get("X-Custom"))
	}
	if req.Header.Get("User-Agent") != "IptvTunerr/1.0" {
		t.Errorf("User-Agent = %q, want IptvTunerr/1.0", req.Header.Get("User-Agent"))
	}
}

func TestGateway_applyUpstreamRequestHeaders_customUserAgent(t *testing.T) {
	g := &Gateway{
		CustomUserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/segment.ts", nil)
	g.applyUpstreamRequestHeaders(req, nil)
	if req.Header.Get("User-Agent") != "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36" {
		t.Errorf("User-Agent = %q, want custom UA", req.Header.Get("User-Agent"))
	}
}

func TestGateway_effectiveUpstreamUserAgent_prefersIncomingWhenUnset(t *testing.T) {
	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/segment.ts", nil)
	req.Header.Set("User-Agent", "ffplay/7.1")
	if got := g.effectiveUpstreamUserAgent(req); got != "ffplay/7.1" {
		t.Fatalf("effectiveUpstreamUserAgent=%q want ffplay/7.1", got)
	}
}

func TestGateway_effectiveUpstreamReferer_prefersCustomHeader(t *testing.T) {
	g := &Gateway{
		CustomHeaders: map[string]string{"Referer": "https://custom.example/"},
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/segment.ts", nil)
	req.Header.Set("Referer", "https://incoming.example/")
	if got := g.effectiveUpstreamReferer(req); got != "https://custom.example/" {
		t.Fatalf("effectiveUpstreamReferer=%q want custom referer", got)
	}
}

func TestGateway_applyUpstreamRequestHeaders_customUserAgentAndHeaders(t *testing.T) {
	g := &Gateway{
		CustomUserAgent: "CustomUA/1.0",
		CustomHeaders: map[string]string{
			"Referer": "http://example.com",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/segment.ts", nil)
	g.applyUpstreamRequestHeaders(req, nil)
	if req.Header.Get("User-Agent") != "CustomUA/1.0" {
		t.Errorf("User-Agent = %q, want CustomUA/1.0", req.Header.Get("User-Agent"))
	}
	if req.Header.Get("Referer") != "http://example.com" {
		t.Errorf("Referer = %q, want http://example.com", req.Header.Get("Referer"))
	}
}

func TestGateway_applyUpstreamRequestHeaders_customHostOverride(t *testing.T) {
	g := &Gateway{
		CustomHeaders: map[string]string{
			"Host": "cdn.example",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "http://origin.example/segment.ts", nil)
	g.applyUpstreamRequestHeaders(req, nil)
	if req.Host != "cdn.example" {
		t.Fatalf("Host = %q, want cdn.example", req.Host)
	}
}

func TestGateway_ffmpegInputHeaderBlock_includesCustomHeaders(t *testing.T) {
	g := &Gateway{
		CustomHeaders: map[string]string{
			"Referer":  "http://provider.example.com",
			"Origin":   "http://provider.example.com",
			"X-Custom": "custom-value",
		},
	}
	block := g.ffmpegInputHeaderBlock(nil, "http://cdn.example/live/u/p/1.m3u8", "cdn.example")
	if !strings.Contains(block, "Referer: http://provider.example.com") {
		t.Fatalf("missing custom Referer in block: %q", block)
	}
	if !strings.Contains(block, "Origin: http://provider.example.com") {
		t.Fatalf("missing custom Origin in block: %q", block)
	}
	if !strings.Contains(block, "X-Custom: custom-value") {
		t.Fatalf("missing X-Custom in block: %q", block)
	}
	if !strings.Contains(block, "User-Agent: IptvTunerr/1.0") {
		t.Fatalf("missing User-Agent in block: %q", block)
	}
}

func TestGateway_ffmpegInputHeaderBlock_customHostOverridesResolvedHost(t *testing.T) {
	g := &Gateway{
		CustomHeaders: map[string]string{
			"Host": "edge.example",
		},
	}
	block := g.ffmpegInputHeaderBlock(nil, "http://resolved.example/live/u/p/1.m3u8", "resolved.example")
	if !strings.Contains(block, "Host: edge.example") {
		t.Fatalf("missing custom Host in block: %q", block)
	}
	if strings.Contains(block, "Host: resolved.example") {
		t.Fatalf("resolved host should be overridden: %q", block)
	}
}

func TestHLSPlaylistCrossHostRefs(t *testing.T) {
	body := []byte(`#EXTM3U
#EXT-X-KEY:METHOD=AES-128,URI="https://keys.example.com/key.bin"
#EXT-X-MAP:URI='//init.example.com/init.mp4'
#EXTINF:2,
seg-1.ts
#EXTINF:2,https://media.example.net/seg-2.ts
#EXT-X-STREAM-INF:BANDWIDTH=1000000,URI="https://variants.example.org/low/index.m3u8"
`)
	got := hlsPlaylistCrossHostRefs(body, "http://playlist.example.com/live/start.m3u8")
	want := []string{"init.example.com", "keys.example.com", "media.example.net", "variants.example.org"}
	if len(got) != len(want) {
		t.Fatalf("cross-host refs len=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cross-host refs[%d]=%q want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestHLSPlaylistCrossHostRefs_IgnoresSameHostRelativeRefs(t *testing.T) {
	body := []byte(`#EXTM3U
#EXT-X-KEY:METHOD=AES-128,URI="key.bin"
#EXT-X-MAP:URI='//playlist.example.com/init.mp4'
#EXTINF:2,
seg-1.ts
`)
	got := hlsPlaylistCrossHostRefs(body, "http://playlist.example.com/live/start.m3u8")
	if len(got) != 0 {
		t.Fatalf("expected no cross-host refs, got %v", got)
	}
}

func TestGateway_relaySuccessfulHLSUpstream_crossHostPlaylistPrefersGoBeforeFFmpegFailure(t *testing.T) {
	if _, err := resolveFFmpegPath(); err != nil {
		t.Skip("ffmpeg not installed; skipping cross-host HLS relay integration test")
	}
	t.Setenv("IPTV_TUNERR_FFMPEG_DISABLED", "0")
	t.Setenv("IPTV_TUNERR_HLS_RELAY_ALLOW_FFMPEG_CROSS_HOST", "0")

	prevTimeout := hlsRelayNoProgressTimeout
	prevRefreshSleep := hlsRelayRefreshSleep
	hlsRelayNoProgressTimeout = 250 * time.Millisecond
	hlsRelayRefreshSleep = func([]byte) {}
	t.Cleanup(func() {
		hlsRelayNoProgressTimeout = prevTimeout
		hlsRelayRefreshSleep = prevRefreshSleep
	})

	var badSegmentHost atomic.Int32
	goodSegment := make(chan struct{})
	var goodSegmentOnce sync.Once

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\n"+srv.URL+"/seg.ts\n")
		case "/seg.ts":
			if strings.HasPrefix(r.Host, "localhost:") {
				badSegmentHost.Add(1)
				http.Error(w, "wrong host override", http.StatusForbidden)
				return
			}
			expectedReferer := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1) + "/playlist.m3u8"
			if got := r.Header.Get("Referer"); got != expectedReferer {
				http.Error(w, "missing playlist referer", http.StatusForbidden)
				return
			}
			expectedOrigin := strings.TrimSuffix(expectedReferer, "/playlist.m3u8")
			if got := r.Header.Get("Origin"); got != expectedOrigin {
				http.Error(w, "missing playlist origin", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("segment-bytes"))
			goodSegmentOnce.Do(func() { close(goodSegment) })
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	playlistURL := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1) + "/playlist.m3u8"
	client := srv.Client()
	resp, err := client.Get(playlistURL)
	if err != nil {
		t.Fatalf("initial playlist GET: %v", err)
	}

	channel := &catalog.LiveChannel{GuideName: "Ch1", GuideNumber: "101", TVGID: "tvg.1"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/1", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	attempt := newStreamAttemptBuilder("req-cross-host", req, "1", channel.GuideName, 1)
	attemptIdx := attempt.addUpstream(1, playlistURL, nil, false, false, false, false)

	type result struct {
		status string
		mode   string
		ok     bool
	}
	done := make(chan result, 1)
	go func() {
		status, mode, _, ok := (&Gateway{TunerCount: 2}).relaySuccessfulHLSUpstream(
			rec, req, channel, "1", playlistURL, playlistURL, time.Now(), attempt, attemptIdx, client, resp,
			false, "", "", "", "",
		)
		done <- result{status: status, mode: mode, ok: ok}
	}()

	select {
	case <-goodSegment:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for successful segment fetch")
	}

	res := <-done
	if !res.ok {
		t.Fatalf("relaySuccessfulHLSUpstream ok=false status=%q mode=%q", res.status, res.mode)
	}
	if res.mode != "hls_go" {
		t.Fatalf("final mode=%q want hls_go", res.mode)
	}
	if bad := badSegmentHost.Load(); bad != 0 {
		t.Fatalf("ffmpeg attempted cross-host segment with stale Host header badSegmentHost=%d", bad)
	}
}

func TestGateway_relayHLSWithFFmpeg_nonTranscodeFirstBytesTimeout(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
	script := `#!/bin/sh
set -eu
exec sleep 30
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_FIRST_BYTES_TIMEOUT_MS", "100")

	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/1", nil)
	rec := httptest.NewRecorder()
	start := time.Now()
	err := g.relayHLSWithFFmpeg(
		rec,
		req,
		ffmpegPath,
		"http://provider.example/live/1.m3u8",
		"Ch1",
		"1",
		"101",
		"tvg.1",
		start,
		false,
		0,
		"",
		hotStartConfig{},
		streamMuxMPEGTS,
		nil,
	)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "first-bytes-timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("expected fast failover timeout, got elapsed=%s", elapsed)
	}
}

func TestGateway_relayHLSWithFFmpeg_nonTranscodeRequireGoodStartTimeout(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
	script := `#!/bin/sh
set -eu
exec sleep 30
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("IPTV_TUNERR_HLS_REMUX_REQUIRE_GOOD_START", "true")
	t.Setenv("IPTV_TUNERR_WEBSAFE_STARTUP_TIMEOUT_MS", "100")
	t.Setenv("IPTV_TUNERR_WEBSAFE_STARTUP_MIN_BYTES", "1024")
	t.Setenv("IPTV_TUNERR_WEBSAFE_STARTUP_MAX_BYTES", "4096")

	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/1", nil)
	rec := httptest.NewRecorder()
	start := time.Now()
	err := g.relayHLSWithFFmpeg(
		rec,
		req,
		ffmpegPath,
		"http://provider.example/live/1.m3u8",
		"Ch1",
		"1",
		"101",
		"tvg.1",
		start,
		false,
		0,
		"",
		hotStartConfig{},
		streamMuxMPEGTS,
		nil,
	)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "startup-gate-timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("expected fast failover timeout, got elapsed=%s", elapsed)
	}
}

func TestGateway_stream_hlsDeadRemuxFallsBackQuickly(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
	script := `#!/bin/sh
set -eu
exec sleep 30
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	prevTimeout := hlsRelayNoProgressTimeout
	prevRefreshSleep := hlsRelayRefreshSleep
	hlsRelayNoProgressTimeout = 250 * time.Millisecond
	hlsRelayRefreshSleep = func([]byte) {}
	t.Cleanup(func() {
		hlsRelayNoProgressTimeout = prevTimeout
		hlsRelayRefreshSleep = prevRefreshSleep
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\nseg.ts\n")
		case "/seg.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("segment-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_FIRST_BYTES_TIMEOUT_MS", "100")
	t.Setenv("IPTV_TUNERR_FFMPEG_DISABLED", "0")
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_LIMIT", "0")

	g := &Gateway{
		Channels: []catalog.LiveChannel{{
			ChannelID:   "1",
			GuideNumber: "101",
			GuideName:   "Ch1",
			TVGID:       "tvg.1",
			StreamURL:   upstream.URL + "/playlist.m3u8",
		}},
		TunerCount: 2,
	}

	req := httptest.NewRequest(http.MethodGet, "/stream/1", nil)
	w := httptest.NewRecorder()
	start := time.Now()
	g.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); !strings.Contains(got, "segment-bytes") {
		t.Fatalf("body=%q want segment bytes after go-relay fallback", got)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected quick fallback, got elapsed=%s", elapsed)
	}
}

func TestGateway_stream_hlsStallAfterProgressFallsBackToBackup(t *testing.T) {
	prevTimeout := hlsRelayNoProgressTimeout
	prevRefreshSleep := hlsRelayRefreshSleep
	hlsRelayNoProgressTimeout = 50 * time.Millisecond
	hlsRelayRefreshSleep = func([]byte) {}
	t.Cleanup(func() {
		hlsRelayNoProgressTimeout = prevTimeout
		hlsRelayRefreshSleep = prevRefreshSleep
	})

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = io.WriteString(w, `#EXTM3U
#EXT-X-TARGETDURATION:1
#EXTINF:1,
seg.ts
`)
		case "/seg.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("PRIMARY-SEGMENT"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer primary.Close()

	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("BACKUP-STREAM"))
	}))
	defer backup.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{{
			ChannelID:   "1",
			GuideNumber: "101",
			GuideName:   "Ch1",
			TVGID:       "tvg.1",
			StreamURLs:  []string{primary.URL + "/playlist.m3u8", backup.URL},
		}},
		TunerCount:    2,
		DisableFFmpeg: true,
	}

	req := httptest.NewRequest(http.MethodGet, "/stream/1", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "PRIMARY-SEGMENT") {
		t.Fatalf("body=%q want primary segment bytes before failover", body)
	}
	if !strings.Contains(body, "BACKUP-STREAM") {
		t.Fatalf("body=%q want backup bytes after HLS stall failover", body)
	}
}

func TestGateway_stream_hlsStallAfterProgressRetriesSameUpstream(t *testing.T) {
	prevTimeout := hlsRelayNoProgressTimeout
	prevRefreshSleep := hlsRelayRefreshSleep
	hlsRelayNoProgressTimeout = 50 * time.Millisecond
	hlsRelayRefreshSleep = func([]byte) {}
	t.Cleanup(func() {
		hlsRelayNoProgressTimeout = prevTimeout
		hlsRelayRefreshSleep = prevRefreshSleep
	})
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_LIMIT", "1")

	var playlistHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
			hit := playlistHits.Add(1)
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			if hit == 1 {
				_, _ = io.WriteString(w, `#EXTM3U
#EXT-X-TARGETDURATION:1
#EXTINF:1,
seg-a.ts
`)
				return
			}
			_, _ = io.WriteString(w, `#EXTM3U
#EXT-X-TARGETDURATION:1
#EXTINF:1,
seg-b.ts
`)
		case "/seg-a.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-A"))
		case "/seg-b.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-B"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{{
			ChannelID:   "1",
			GuideNumber: "101",
			GuideName:   "Ch1",
			TVGID:       "tvg.1",
			StreamURL:   upstream.URL + "/playlist.m3u8",
		}},
		TunerCount:    2,
		DisableFFmpeg: true,
	}

	req := httptest.NewRequest(http.MethodGet, "/stream/1", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "SEG-A") || !strings.Contains(body, "SEG-B") {
		t.Fatalf("body=%q want bytes from both same-upstream attempts", body)
	}
	if got := playlistHits.Load(); got < 2 {
		t.Fatalf("playlist hits=%d want retry on same upstream", got)
	}
}

func TestGateway_stream_hlsStallAfterProgressDoesNotDowngradeToAllUpstreamsFailedAfterDeadBackup(t *testing.T) {
	prevTimeout := hlsRelayNoProgressTimeout
	prevRefreshSleep := hlsRelayRefreshSleep
	hlsRelayNoProgressTimeout = 50 * time.Millisecond
	hlsRelayRefreshSleep = func([]byte) {}
	t.Cleanup(func() {
		hlsRelayNoProgressTimeout = prevTimeout
		hlsRelayRefreshSleep = prevRefreshSleep
	})
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_LIMIT", "1")

	var playlistHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
			hit := playlistHits.Add(1)
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			if hit == 1 {
				_, _ = io.WriteString(w, `#EXTM3U
#EXT-X-TARGETDURATION:1
#EXTINF:1,
seg-a.ts
`)
				return
			}
			_, _ = io.WriteString(w, `#EXTM3U
#EXT-X-TARGETDURATION:1
#EXTINF:1,
seg-b.ts
`)
		case "/seg-a.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-A"))
		case "/seg-b.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-B"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{{
			ChannelID:   "1",
			GuideNumber: "101",
			GuideName:   "Ch1",
			TVGID:       "tvg.1",
			StreamURLs:  []string{upstream.URL + "/playlist.m3u8", "http://does-not-resolve.invalid/backup.m3u8"},
		}},
		TunerCount:    2,
		DisableFFmpeg: true,
	}

	req := httptest.NewRequest(http.MethodGet, "/stream/1", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "SEG-A") || !strings.Contains(body, "SEG-B") {
		t.Fatalf("body=%q want streamed segment bytes from primary retries", body)
	}
	if len(g.recentAttempts) != 1 {
		t.Fatalf("recentAttempts=%d want 1", len(g.recentAttempts))
	}
	if got := g.recentAttempts[0].FinalStatus; got != "stream_ended_after_progress" {
		t.Fatalf("final status=%q want stream_ended_after_progress", got)
	}
}

func TestGateway_stream_hlsStallAfterProgressRetriesPrimaryBeforeDeadBackup(t *testing.T) {
	prevTimeout := hlsRelayNoProgressTimeout
	prevRefreshSleep := hlsRelayRefreshSleep
	hlsRelayNoProgressTimeout = 50 * time.Millisecond
	hlsRelayRefreshSleep = func([]byte) {}
	t.Cleanup(func() {
		hlsRelayNoProgressTimeout = prevTimeout
		hlsRelayRefreshSleep = prevRefreshSleep
	})
	t.Setenv("IPTV_TUNERR_UPSTREAM_RETRY_LIMIT", "1")

	var playlistHits atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
			hit := playlistHits.Add(1)
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			if hit == 1 {
				_, _ = io.WriteString(w, `#EXTM3U
#EXT-X-TARGETDURATION:1
#EXTINF:1,
seg-a.ts
`)
				return
			}
			_, _ = io.WriteString(w, `#EXTM3U
#EXT-X-TARGETDURATION:1
#EXTINF:1,
seg-b.ts
`)
		case "/seg-a.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-A"))
		case "/seg-b.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-B"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer primary.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{{
			ChannelID:   "1",
			GuideNumber: "101",
			GuideName:   "Ch1",
			TVGID:       "tvg.1",
			StreamURLs:  []string{primary.URL + "/playlist.m3u8", "http://does-not-resolve.invalid/backup.m3u8"},
		}},
		TunerCount:    2,
		DisableFFmpeg: true,
	}

	req := httptest.NewRequest(http.MethodGet, "/stream/1", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "SEG-A") || !strings.Contains(body, "SEG-B") {
		t.Fatalf("body=%q want bytes from both recovered primary attempts", body)
	}
	if hits := playlistHits.Load(); hits < 2 {
		t.Fatalf("playlist hits=%d want >=2", hits)
	}
}

func TestGateway_fetchAndRewritePlaylist_usesRedirectedURLAsBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start.m3u8":
			http.Redirect(w, r, "/nested/playlist.m3u8", http.StatusFound)
		case "/nested/playlist.m3u8":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("#EXTM3U\nsegment.ts\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	g := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/1", nil)
	body, effectiveURL, err := g.fetchAndRewritePlaylist(req, srv.Client(), srv.URL+"/start.m3u8")
	if err != nil {
		t.Fatalf("fetchAndRewritePlaylist: %v", err)
	}
	if effectiveURL != srv.URL+"/nested/playlist.m3u8" {
		t.Fatalf("effectiveURL = %q, want redirected playlist URL", effectiveURL)
	}
	if got := string(body); !strings.Contains(got, srv.URL+"/nested/segment.ts") {
		t.Fatalf("rewritten playlist = %q, want redirected base URL", got)
	}
}

func TestGateway_fetchAndRewritePlaylist_retriesConcurrencyLimit(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			http.Error(w, "maximum 1 connections allowed", 509)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#EXTM3U\nsegment.ts\n"))
	}))
	defer srv.Close()

	t.Setenv("IPTV_TUNERR_HLS_PLAYLIST_RETRY_LIMIT", "1")
	t.Setenv("IPTV_TUNERR_HLS_PLAYLIST_RETRY_BACKOFF_MS", "1")
	g := &Gateway{TunerCount: 4}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/1", nil)
	body, effectiveURL, err := g.fetchAndRewritePlaylist(req, srv.Client(), srv.URL+"/playlist.m3u8")
	if err != nil {
		t.Fatalf("fetchAndRewritePlaylist retry: %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits=%d want 2", hits)
	}
	if effectiveURL != srv.URL+"/playlist.m3u8" {
		t.Fatalf("effectiveURL=%q", effectiveURL)
	}
	if got := string(body); !strings.Contains(got, srv.URL+"/segment.ts") {
		t.Fatalf("rewritten playlist=%q", got)
	}
	if g.learnedUpstreamLimit != 1 {
		t.Fatalf("learnedUpstreamLimit=%d want 1", g.learnedUpstreamLimit)
	}
}

func TestGateway_fetchAndRewritePlaylist_learnsProviderAccountLimitFromContext(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			http.Error(w, "maximum 1 connections allowed", 509)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:1\nseg.ts\n"))
	}))
	defer srv.Close()

	t.Setenv("IPTV_TUNERR_HLS_PLAYLIST_RETRY_LIMIT", "1")
	t.Setenv("IPTV_TUNERR_HLS_PLAYLIST_RETRY_BACKOFF_MS", "1")
	ch := &catalog.LiveChannel{
		GuideNumber: "1",
		GuideName:   "Ch1",
		StreamAuths: []catalog.StreamAuth{{Prefix: srv.URL, User: "u1", Pass: "p1"}},
	}
	g := &Gateway{TunerCount: 4, ProviderUser: "u1", ProviderPass: "p1"}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/1", nil)
	req = req.WithContext(context.WithValue(req.Context(), gatewayChannelKey{}, ch))
	_, _, err := g.fetchAndRewritePlaylist(req, srv.Client(), srv.URL+"/playlist.m3u8")
	if err != nil {
		t.Fatalf("fetchAndRewritePlaylist retry: %v", err)
	}
	limits := g.providerAccountLearnedLimits()
	if len(limits) != 1 {
		t.Fatalf("learned account limits=%#v", limits)
	}
	if limits[0].LearnedLimit != 1 || limits[0].SignalCount != 1 {
		t.Fatalf("limit state=%#v", limits[0])
	}
}

func providerAccountMustKey(t *testing.T, g *Gateway, ch *catalog.LiveChannel, rawURL string) string {
	t.Helper()
	identity, ok := providerAccountIdentityForURL(g, ch, rawURL)
	if !ok {
		t.Fatal("expected provider account identity")
	}
	return identity.Key
}

func TestCopyStreamResponseHeaders_StripsSetCookie(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Content-Type":      []string{"video/mp2t"},
			"Set-Cookie":        []string{"session=secret"},
			"Transfer-Encoding": []string{"chunked"},
			"Content-Length":    []string{"123"},
		},
	}
	w := httptest.NewRecorder()
	copyStreamResponseHeaders(w, resp)
	if got := w.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("Set-Cookie leaked: %q", got)
	}
	if got := w.Header().Get("Transfer-Encoding"); got != "" {
		t.Fatalf("Transfer-Encoding leaked: %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length leaked: %q", got)
	}
	if got := w.Header().Get("Content-Type"); got != "video/mp2t" {
		t.Fatalf("Content-Type=%q want video/mp2t", got)
	}
}

func TestGateway_relayHLSAsTS_survivesPlaylistConcurrencyRetry(t *testing.T) {
	var playlistHits atomic.Int32
	thirdPlaylistOK := make(chan struct{})
	var thirdOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
			h := playlistHits.Add(1)
			if h == 2 {
				http.Error(w, "maximum 1 connections allowed", 509)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:1\nseg.ts\n"))
			if h >= 3 {
				thirdOnce.Do(func() { close(thirdPlaylistOK) })
			}
		case "/seg.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("segment-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("IPTV_TUNERR_HLS_PLAYLIST_RETRY_LIMIT", "1")
	t.Setenv("IPTV_TUNERR_HLS_PLAYLIST_RETRY_BACKOFF_MS", "1")
	g := &Gateway{TunerCount: 4}
	baseReq := httptest.NewRequest(http.MethodGet, "http://local/stream/1", nil)
	initial, effectiveURL, err := g.fetchAndRewritePlaylist(baseReq, srv.Client(), srv.URL+"/playlist.m3u8")
	if err != nil {
		t.Fatalf("initial playlist fetch: %v", err)
	}

	ctx, cancel := context.WithCancel(baseReq.Context())
	defer cancel()
	req := baseReq.WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan error, 1)
	go func() {
		done <- g.relayHLSAsTS(rec, req, srv.Client(), effectiveURL, initial, "Ch1", "1", "101", "tvg.1", time.Now(), false, "", 0, nil, false)
	}()

	select {
	case <-thirdPlaylistOK:
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout waiting for successful playlist fetch after 509 retry (playlistHits=%d)", playlistHits.Load())
	}

	if got := rec.Body.Len(); got == 0 {
		t.Fatal("expected relay to write bytes before cancel")
	}

	cancel()

	if err := <-done; err != nil {
		t.Fatalf("relayHLSAsTS: %v", err)
	}
	if g.learnedUpstreamLimit != 1 {
		t.Fatalf("learnedUpstreamLimit=%d want 1", g.learnedUpstreamLimit)
	}
	if n := playlistHits.Load(); n < 3 {
		t.Fatalf("playlistHits=%d want at least 3 (initial + retry path)", n)
	}
}

func TestPersistentCookieJarPersistsNewCookies(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cookies.json"
	jar, err := newPersistentCookieJar(path)
	if err != nil {
		t.Fatalf("newPersistentCookieJar: %v", err)
	}
	u, err := url.Parse("https://cdn.example/playlist.m3u8")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	jar.SetCookies(u, []*http.Cookie{{
		Name:    "cf_clearance",
		Value:   "token123",
		Domain:  "cdn.example",
		Path:    "/",
		Secure:  true,
		Expires: time.Now().Add(time.Hour).Round(time.Second),
	}})
	if err := jar.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := newPersistentCookieJar(path)
	if err != nil {
		t.Fatalf("newPersistentCookieJar(load): %v", err)
	}
	got := loaded.Cookies(u)
	if len(got) != 1 || got[0].Name != "cf_clearance" || got[0].Value != "token123" {
		t.Fatalf("loaded cookies = %#v, want cf_clearance token", got)
	}
}

func TestPersistentCookieJarRemovesExpiredCookies(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/cookies.json"
	jar, err := newPersistentCookieJar(path)
	if err != nil {
		t.Fatalf("newPersistentCookieJar: %v", err)
	}
	u, err := url.Parse("https://cdn.example/playlist.m3u8")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	jar.SetCookies(u, []*http.Cookie{{
		Name:    "expired",
		Value:   "gone",
		Domain:  "cdn.example",
		Path:    "/",
		Secure:  true,
		Expires: time.Now().Add(-time.Minute),
	}})
	if err := jar.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "expired") {
		t.Fatalf("cookie file still contains expired cookie: %s", string(data))
	}
}

func TestGateway_applyUpstreamRequestHeaders_usesPerStreamAuth(t *testing.T) {
	ch := &catalog.LiveChannel{
		StreamAuths: []catalog.StreamAuth{{
			Prefix: "http://provider2.example/live/u2/p2/",
			User:   "u2",
			Pass:   "p2",
		}},
	}
	g := &Gateway{ProviderUser: "u1", ProviderPass: "p1"}
	req := httptest.NewRequest(http.MethodGet, "http://provider2.example/live/u2/p2/1001.m3u8", nil)
	req = req.WithContext(context.WithValue(req.Context(), gatewayChannelKey{}, ch))
	g.applyUpstreamRequestHeaders(req, nil)
	user, pass, ok := req.BasicAuth()
	if !ok || user != "u2" || pass != "p2" {
		t.Fatalf("BasicAuth = %q/%q ok=%t, want u2/p2", user, pass, ok)
	}
}

func TestGateway_ffmpegInputHeaderBlock_usesPerStreamAuthAndCookies(t *testing.T) {
	jar, err := newPersistentCookieJar("")
	if err != nil {
		t.Fatalf("newPersistentCookieJar: %v", err)
	}
	playlistURL := "http://provider2.example/live/u2/p2/1001.m3u8"
	u, err := url.Parse(playlistURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	jar.SetCookies(u, []*http.Cookie{{Name: "cf_clearance", Value: "token123", Path: "/"}})
	ch := &catalog.LiveChannel{
		StreamAuths: []catalog.StreamAuth{{
			Prefix: "http://provider2.example/live/u2/p2/",
			User:   "u2",
			Pass:   "p2",
		}},
	}
	g := &Gateway{
		Client:       &http.Client{Jar: jar},
		ProviderUser: "u1",
		ProviderPass: "p1",
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/1", nil)
	req = req.WithContext(context.WithValue(req.Context(), gatewayChannelKey{}, ch))
	block := g.ffmpegInputHeaderBlock(req, playlistURL, "provider2.example")
	if !strings.Contains(block, "Authorization: Basic dTI6cDI=") {
		t.Fatalf("missing per-stream auth in block: %q", block)
	}
	if !strings.Contains(block, "Cookie: cf_clearance=token123") {
		t.Fatalf("missing cookie jar cookies in block: %q", block)
	}
}

func TestGateway_ffmpegCookiesOptionForURL(t *testing.T) {
	jar, err := newPersistentCookieJar("")
	if err != nil {
		t.Fatalf("newPersistentCookieJar: %v", err)
	}
	playlistURL := "http://provider2.example/live/u2/p2/1001.m3u8"
	u, err := url.Parse(playlistURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	jar.SetCookies(u, []*http.Cookie{{Name: "cf_clearance", Value: "token123", Path: "/"}})
	g := &Gateway{Client: &http.Client{Jar: jar}}
	got := g.ffmpegCookiesOptionForURL(playlistURL)
	if !strings.Contains(got, "cf_clearance=token123;") {
		t.Fatalf("cookies option missing token: %q", got)
	}
	if !strings.Contains(got, "domain=provider2.example;") {
		t.Fatalf("cookies option missing domain: %q", got)
	}
}

func TestGateway_streamRejectEmitsWebhookEvent(t *testing.T) {
	delivered := make(chan eventhooks.Event, 4)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var evt eventhooks.Event
		if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
			t.Fatalf("decode webhook event: %v", err)
		}
		delivered <- evt
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()

	cfgPath := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(cfgPath, []byte(`{"webhooks":[{"name":"test","url":"`+webhook.URL+`","events":["stream.requested","stream.rejected","stream.finished"]}]}`), 0o644); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}
	dispatcher, err := eventhooks.Load(cfgPath)
	if err != nil {
		t.Fatalf("load dispatcher: %v", err)
	}
	g := &Gateway{
		TunerCount: 2,
		inUse:      2,
		EventHooks: dispatcher,
		Channels: []catalog.LiveChannel{{
			ChannelID:   "100",
			GuideNumber: "100",
			GuideName:   "Test",
			StreamURL:   "http://example.com/live.m3u8",
		}},
	}
	req := httptest.NewRequest(http.MethodGet, "/stream/100", nil)
	rr := httptest.NewRecorder()
	g.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", rr.Code)
	}
	seenRejected := false
	timeout := time.After(2 * time.Second)
	for !seenRejected {
		select {
		case evt := <-delivered:
			if evt.Name == "stream.rejected" {
				seenRejected = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for stream.rejected webhook")
		}
	}
}

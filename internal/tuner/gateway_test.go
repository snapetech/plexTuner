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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
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

func TestGateway_shouldPreferGoRelayForHLSRemux(t *testing.T) {
	g := &Gateway{TunerCount: 4, learnedUpstreamLimit: 2}
	if !g.shouldPreferGoRelayForHLSRemux("http://provider.example/live/1.m3u8") {
		t.Fatal("expected learned lower upstream limit to prefer go relay")
	}

	t.Setenv("IPTV_TUNERR_HLS_RELAY_PREFER_GO_ON_PROVIDER_PRESSURE", "false")
	if g.shouldPreferGoRelayForHLSRemux("http://provider.example/live/1.m3u8") {
		t.Fatal("expected provider-pressure preference to be disable-able")
	}

	t.Setenv("IPTV_TUNERR_HLS_RELAY_PREFER_GO", "true")
	if !g.shouldPreferGoRelayForHLSRemux("http://provider.example/live/1.m3u8") {
		t.Fatal("expected explicit go-relay preference override")
	}
}

func TestGateway_shouldPreferGoRelayForHLSRemux_hostPenalty(t *testing.T) {
	t.Run("penalized_host", func(t *testing.T) {
		g := &Gateway{TunerCount: 4}
		g.noteUpstreamFailure("http://provider.example/live/1.m3u8", 0, "ffmpeg_hls_failed")
		if !g.shouldPreferGoRelayForHLSRemux("http://provider.example/live/2.m3u8") {
			t.Fatal("expected host penalty to prefer go relay")
		}
	})
	t.Run("autotune_off_no_penalty_signal", func(t *testing.T) {
		t.Setenv("IPTV_TUNERR_PROVIDER_AUTOTUNE", "false")
		g := &Gateway{TunerCount: 4}
		g.noteUpstreamFailure("http://provider.example/live/1.m3u8", 0, "ffmpeg_hls_failed")
		if g.shouldPreferGoRelayForHLSRemux("http://provider.example/live/2.m3u8") {
			t.Fatal("expected no host-penalty go-relay when autotune is off")
		}
	})
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
			"Referer":  "http://smarter8k.ru",
			"Origin":   "http://smarter8k.ru",
			"X-Custom": "custom-value",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/segment.ts", nil)
	g.applyUpstreamRequestHeaders(req, nil)
	if req.Header.Get("Referer") != "http://smarter8k.ru" {
		t.Errorf("Referer = %q, want http://smarter8k.ru", req.Header.Get("Referer"))
	}
	if req.Header.Get("Origin") != "http://smarter8k.ru" {
		t.Errorf("Origin = %q, want http://smarter8k.ru", req.Header.Get("Origin"))
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
			"Referer":  "http://smarter8k.ru",
			"Origin":   "http://smarter8k.ru",
			"X-Custom": "custom-value",
		},
	}
	block := g.ffmpegInputHeaderBlock(nil, "http://cdn.example/live/u/p/1.m3u8", "cdn.example")
	if !strings.Contains(block, "Referer: http://smarter8k.ru") {
		t.Fatalf("missing custom Referer in block: %q", block)
	}
	if !strings.Contains(block, "Origin: http://smarter8k.ru") {
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
		done <- g.relayHLSAsTS(rec, req, srv.Client(), effectiveURL, initial, "Ch1", "1", "101", "tvg.1", time.Now(), false, "", 0, false)
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

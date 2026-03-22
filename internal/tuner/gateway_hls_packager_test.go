package tuner

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestGateway_ffmpegPackagedHLS_namedProfileServesPlaylistAndSegment(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
	script := `#!/bin/sh
set -eu
playlist=""
segpat=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "seg" ]; then
    segpat="$arg"
    prev=""
    continue
  fi
  if [ "$arg" = "-hls_segment_filename" ]; then
    prev="seg"
    continue
  fi
  playlist="$arg"
done
mkdir -p "$(dirname "$playlist")"
printf '#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.0,\nseg-000001.ts\n' > "$playlist"
segfile=$(printf "$segpat" 1)
printf 'segment-bytes' > "$segfile"
sleep 30
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_STARTUP_TIMEOUT_MS", "2000")
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_FILE_WAIT_TIMEOUT_MS", "2000")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:2.0,\nupstream-1.ts\n"))
	}))
	defer up.Close()

	enable := true
	g := &Gateway{
		TunerCount: 1,
		Channels: []catalog.LiveChannel{{
			ChannelID:   "c1",
			GuideNumber: "1",
			GuideName:   "Packaged",
			StreamURLs:  []string{up.URL + "/live.m3u8"},
		}, {
			ChannelID:   "c2",
			GuideNumber: "2",
			GuideName:   "Second",
			StreamURLs:  []string{up.URL + "/other.m3u8"},
		}},
		NamedProfiles: map[string]NamedStreamProfile{
			"web-hls": {
				BaseProfile: profileDashFast,
				Transcode:   &enable,
				OutputMux:   streamMuxHLS,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/c1?profile=web-hls", nil)
	g.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "mpegurl") {
		t.Fatalf("content-type=%q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "mux="+streamMuxHLSPackager) || !strings.Contains(body, "sid=") || !strings.Contains(body, "file=") {
		t.Fatalf("expected packaged playlist URLs, got %q", body)
	}
	u, err := url.Parse(strings.TrimSpace(lastNonEmptyLine(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer g.unregisterHLSPackagerSession(u.Query().Get("sid"), "test_cleanup")

	recSeg := httptest.NewRecorder()
	reqSeg := httptest.NewRequest(http.MethodGet, "http://local"+u.String(), nil)
	g.ServeHTTP(recSeg, reqSeg)
	if recSeg.Code != http.StatusOK {
		t.Fatalf("segment status=%d body=%q", recSeg.Code, recSeg.Body.String())
	}
	if got := recSeg.Body.String(); got != "segment-bytes" {
		t.Fatalf("segment body=%q", got)
	}

	recSecond := httptest.NewRecorder()
	reqSecond := httptest.NewRequest(http.MethodGet, "http://local/stream/c2", nil)
	g.ServeHTTP(recSecond, reqSecond)
	if recSecond.Code != http.StatusServiceUnavailable {
		t.Fatalf("second status=%d body=%q", recSecond.Code, recSecond.Body.String())
	}
	if !strings.Contains(recSecond.Body.String(), "All tuners in use") {
		t.Fatalf("second body=%q", recSecond.Body.String())
	}
}

func TestGateway_ffmpegPackagedHLS_targetRequiresGetOrHead(t *testing.T) {
	g := &Gateway{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://local/stream/c1?mux="+streamMuxHLSPackager+"&sid=test&file=index.m3u8", nil)
	handled := g.maybeServeFFmpegPackagedHLSTarget(rec, req, "c1")
	if !handled {
		t.Fatal("expected packaged target request to be handled")
	}
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("allow=%q", got)
	}
}

func TestGateway_ffmpegPackagedHLS_sameProfileReusesExistingSession(t *testing.T) {
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
	script := `#!/bin/sh
set -eu
playlist=""
segpat=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "seg" ]; then
    segpat="$arg"
    prev=""
    continue
  fi
  if [ "$arg" = "-hls_segment_filename" ]; then
    prev="seg"
    continue
  fi
  playlist="$arg"
done
mkdir -p "$(dirname "$playlist")"
printf '#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.0,\nseg-000001.ts\n' > "$playlist"
segfile=$(printf "$segpat" 1)
printf 'segment-bytes' > "$segfile"
sleep 30
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_STARTUP_TIMEOUT_MS", "2000")
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_FILE_WAIT_TIMEOUT_MS", "2000")

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:2.0,\nupstream-1.ts\n"))
	}))
	defer up.Close()

	enable := true
	g := &Gateway{
		TunerCount: 1,
		Channels: []catalog.LiveChannel{{
			ChannelID:   "c1",
			GuideNumber: "1",
			GuideName:   "Packaged",
			StreamURLs:  []string{up.URL + "/live.m3u8"},
		}, {
			ChannelID:   "c2",
			GuideNumber: "2",
			GuideName:   "Other",
			StreamURLs:  []string{up.URL + "/other.m3u8"},
		}},
		NamedProfiles: map[string]NamedStreamProfile{
			"web-hls": {
				BaseProfile: profileDashFast,
				Transcode:   &enable,
				OutputMux:   streamMuxHLS,
			},
			"web-hls-alt": {
				BaseProfile: profileLowBitrate,
				Transcode:   &enable,
				OutputMux:   streamMuxHLS,
			},
		},
	}

	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "http://local/stream/c1?profile=web-hls", nil)
	g.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%q", rec1.Code, rec1.Body.String())
	}
	u1, err := url.Parse(strings.TrimSpace(lastNonEmptyLine(rec1.Body.String())))
	if err != nil {
		t.Fatal(err)
	}
	sid1 := u1.Query().Get("sid")
	if sid1 == "" {
		t.Fatalf("missing sid in %q", rec1.Body.String())
	}
	defer g.unregisterHLSPackagerSession(sid1, "test_cleanup")

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://local/stream/c1?profile=web-hls", nil)
	g.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second status=%d body=%q", rec2.Code, rec2.Body.String())
	}
	if got := rec2.Header().Get("X-IptvTunerr-Shared-Upstream"); got != "ffmpeg_hls_packager" {
		t.Fatalf("shared header=%q", got)
	}
	u2, err := url.Parse(strings.TrimSpace(lastNonEmptyLine(rec2.Body.String())))
	if err != nil {
		t.Fatal(err)
	}
	if sid2 := u2.Query().Get("sid"); sid2 != sid1 {
		t.Fatalf("reused sid=%q want %q", sid2, sid1)
	}

	recAlt := httptest.NewRecorder()
	reqAlt := httptest.NewRequest(http.MethodGet, "http://local/stream/c1?profile=web-hls-alt", nil)
	g.ServeHTTP(recAlt, reqAlt)
	if recAlt.Code != http.StatusServiceUnavailable {
		t.Fatalf("alt status=%d body=%q", recAlt.Code, recAlt.Body.String())
	}
	if !strings.Contains(recAlt.Body.String(), "All tuners in use") {
		t.Fatalf("alt body=%q", recAlt.Body.String())
	}

	recOther := httptest.NewRecorder()
	reqOther := httptest.NewRequest(http.MethodGet, "http://local/stream/c2", nil)
	g.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusServiceUnavailable {
		t.Fatalf("other status=%d body=%q", recOther.Code, recOther.Body.String())
	}
	if !strings.Contains(recOther.Body.String(), "All tuners in use") {
		t.Fatalf("other body=%q", recOther.Body.String())
	}
}

func TestGateway_lookupReusableHLSPackagerSessionDropsExitedSession(t *testing.T) {
	g := &Gateway{
		hlsPackagerSessions:      map[string]*ffmpegHLSPackagerSession{},
		hlsPackagerSessionsByKey: map[string]*ffmpegHLSPackagerSession{},
		hlsPackagerInUse:         1,
	}
	profile := resolvedStreamProfile{
		Name:           "shared-hls",
		BaseProfile:    profileDashFast,
		ForceTranscode: true,
		OutputMux:      streamMuxHLS,
	}
	sess := &ffmpegHLSPackagerSession{
		id:         "sid-1",
		reuseKey:   hlsPackagerReuseKey("c1", profile),
		channelID:  "c1",
		tunerHeld:  true,
		createdAt:  time.Now(),
		lastAccess: time.Now(),
		exited:     true,
	}
	g.hlsPackagerSessions[sess.id] = sess
	g.hlsPackagerSessionsByKey[sess.reuseKey] = sess

	got := g.lookupReusableHLSPackagerSession(sess.reuseKey)
	if got != nil {
		t.Fatalf("expected nil reusable session, got %#v", got)
	}
	if len(g.hlsPackagerSessions) != 0 {
		t.Fatalf("session map still populated: %#v", g.hlsPackagerSessions)
	}
	if len(g.hlsPackagerSessionsByKey) != 0 {
		t.Fatalf("session-by-key map still populated: %#v", g.hlsPackagerSessionsByKey)
	}
	if g.hlsPackagerInUse != 0 {
		t.Fatalf("hlsPackagerInUse=%d want 0", g.hlsPackagerInUse)
	}
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

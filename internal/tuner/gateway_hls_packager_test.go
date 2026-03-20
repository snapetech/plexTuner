package tuner

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

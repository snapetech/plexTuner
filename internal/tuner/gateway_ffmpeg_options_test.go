package tuner

import "testing"

func TestFFmpegHLSHTTPPersistentEnabled_defaultOff(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_HTTP_PERSISTENT", "")
	if ffmpegHLSHTTPPersistentEnabled() {
		t.Fatal("expected http_persistent to default off")
	}
}

func TestFFmpegHLSHTTPPersistentEnabled_explicitOn(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_HTTP_PERSISTENT", "true")
	if !ffmpegHLSHTTPPersistentEnabled() {
		t.Fatal("expected http_persistent to honor explicit enable")
	}
}

func TestFFmpegHLSLiveStartIndex_defaultOff(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_LIVE_START_INDEX", "")
	if got := ffmpegHLSLiveStartIndex(); got != 0 {
		t.Fatalf("live_start_index=%d want 0", got)
	}
}

func TestFFmpegHLSLiveStartIndex_explicitOverride(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FFMPEG_HLS_LIVE_START_INDEX", "-3")
	if got := ffmpegHLSLiveStartIndex(); got != -3 {
		t.Fatalf("live_start_index=%d want -3", got)
	}
}

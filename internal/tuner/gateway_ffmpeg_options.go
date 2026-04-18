package tuner

// Some ffmpeg/libavformat builds do not support the `-http_persistent` input
// option at all. Keep it opt-in so HLS playback does not fail hard on those
// runtimes.
func ffmpegHLSHTTPPersistentEnabled() bool {
	return getenvBool("IPTV_TUNERR_FFMPEG_HLS_HTTP_PERSISTENT", false)
}

// Keep live-start seeking opt-in too: some ffmpeg builds reject the option,
// and a failed remux startup delays fallback enough to break real clients.
func ffmpegHLSLiveStartIndex() int {
	return getenvInt("IPTV_TUNERR_FFMPEG_HLS_LIVE_START_INDEX", 0)
}

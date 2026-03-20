package tuner

import (
	"testing"
)

func FuzzRewriteHLSPlaylistToGatewayProxy(f *testing.F) {
	f.Add([]byte("#EXTM3U\n#EXTINF:4,\nseg.ts\n"), "http://up.example/live.m3u8", "ch1")
	f.Add([]byte("#EXTM3U\n#EXTINF:9.009,BYTERANGE=\"128@448\"\nseg.ts\n"), "http://up.example/live.m3u8", "ch1")
	f.Fuzz(func(t *testing.T, body []byte, up, ch string) {
		if len(body) > 1<<18 {
			t.Skip()
		}
		if len(up) > 2048 || len(ch) > 256 {
			t.Skip()
		}
		_ = rewriteHLSPlaylistToGatewayProxy(body, up, ch)
	})
}

func FuzzRewriteDASHManifestToGatewayProxy(f *testing.F) {
	f.Add([]byte(`<MPD><Period><BaseURL>https://cdn.example/init/</BaseURL></Period></MPD>`), "http://up.example/manifest.mpd", "z")
	f.Add([]byte(`<MPD mediaPresentationDuration="PT6S"><Period duration="PT6S"><Representation id="a" bandwidth="1"><SegmentTemplate timescale="1" duration="3" startNumber="1" media="https://x/s-$Number$.m4s"/></Representation></Period></MPD>`), "http://up.example/manifest.mpd", "z")
	f.Fuzz(func(t *testing.T, body []byte, up, ch string) {
		if len(body) > 1<<18 {
			t.Skip()
		}
		if len(up) > 2048 || len(ch) > 256 {
			t.Skip()
		}
		_ = rewriteDASHManifestToGatewayProxy(body, up, ch)
	})
}

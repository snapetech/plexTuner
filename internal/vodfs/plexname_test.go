//go:build linux
// +build linux

package vodfs

import "testing"

func TestVODFileExtPreservesKnownDirectExtensions(t *testing.T) {
	cases := map[string]string{
		"http://x.example/a/movie.mkv":   ".mkv",
		"http://x.example/a/movie.mp4":   ".mp4",
		"http://x.example/a/movie.webm":  ".webm",
		"http://x.example/a/stream.m3u8": ".mp4",
		"http://x.example/a/noext":       ".mp4",
		"not a url":                      ".mp4",
	}
	for in, want := range cases {
		if got := VODFileExt(in); got != want {
			t.Fatalf("VODFileExt(%q)=%q want %q", in, got, want)
		}
	}
}

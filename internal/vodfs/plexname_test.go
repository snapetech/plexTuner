//go:build linux
// +build linux

package vodfs

import "testing"

func TestVODNamesIncludeLivePrefix(t *testing.T) {
	if got := MovieFileNameForStream("Face/Off", 1997, "http://x.example/a/movie.mkv"); got != "Live: Face - Off (1997).mkv" {
		t.Fatalf("movie filename missing prefix/sanitization: %q", got)
	}
	if got := EpisodeFileNameForStream("Scrubs", 2001, 1, 2, "My Job", "http://x.example/a/ep.mkv"); got != "Live: Scrubs (2001) - s01e02 - My Job.mkv" {
		t.Fatalf("episode filename missing prefix: %q", got)
	}
	if got := MovieDirName("Live: Already", 0); got != "Live: Already" {
		t.Fatalf("double prefix should not be added: %q", got)
	}
}

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

package tuner

import (
	"strings"
	"testing"
)

func TestNormalizeProfileName_HDHRStyleAliases(t *testing.T) {
	cases := map[string]string{
		"native":         profileDefault,
		"heavy":          profileDefault,
		"internet":       profileDashFast,
		"internet360":    profileAACCFR,
		"mobile":         profileLowBitrate,
		"cell":           profileLowBitrate,
		"Internet-1080":  profileDashFast,
		"INTERNET480":    profileAACCFR,
		"pms-xcode":      profilePMSXcode,
		"unknown-custom": profileDefault,
	}
	for in, want := range cases {
		if got := normalizeProfileName(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestBuildFFmpegStreamOutputArgs_fmp4(t *testing.T) {
	args := buildFFmpegStreamOutputArgs(true, profileDefault, streamMuxFMP4)
	s := strings.Join(args, " ")
	if !strings.Contains(s, "-f mp4") || !strings.Contains(s, "frag_keyframe") {
		t.Fatalf("expected fragmented mp4 mux: %s", s)
	}
	argsTS := buildFFmpegStreamOutputArgs(true, profileDefault, streamMuxMPEGTS)
	if !strings.Contains(strings.Join(argsTS, " "), "-f mpegts") {
		t.Fatalf("expected mpegts: %v", argsTS)
	}
}

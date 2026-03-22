package tuner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeProfileName_HDHRStyleAliases(t *testing.T) {
	cases := map[string]string{
		"native":         profileDefault,
		"heavy":          profileDefault,
		"plexsafehq":     profilePlexSafeHQ,
		"plex-safe-hq":   profilePlexSafeHQ,
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

func TestBuildFFmpegStreamCodecArgs_plexsafeHQ(t *testing.T) {
	args := buildFFmpegStreamCodecArgs(true, profilePlexSafeHQ, streamMuxMPEGTS)
	s := strings.Join(args, " ")
	for _, needle := range []string{
		"setsar=1",
		"-crf 18",
		"-maxrate 16000k",
		"-bufsize 32000k",
		"-b:a 192k",
		"-muxrate 18000000",
	} {
		if !strings.Contains(s, needle) {
			t.Fatalf("expected %q in %s", needle, s)
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
	if got := normalizeStreamOutputMuxName("hls"); got != streamMuxHLS {
		t.Fatalf("expected hls output mux, got %q", got)
	}
}

func TestLoadNamedProfilesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	body := `{
		"mobile-fmp4": {"base_profile":"lowbitrate","transcode":true,"output_mux":"fmp4"},
		"copy-ts": {"base_profile":"dashfast","transcode":false,"output_mux":"mpegts"},
		"web-hls": {"base_profile":"dashfast","transcode":true,"output_mux":"hls"}
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := loadNamedProfilesFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["mobile-fmp4"].BaseProfile != profileLowBitrate {
		t.Fatalf("BaseProfile=%q", got["mobile-fmp4"].BaseProfile)
	}
	if got["mobile-fmp4"].Transcode == nil || !*got["mobile-fmp4"].Transcode {
		t.Fatalf("expected transcode true: %#v", got["mobile-fmp4"])
	}
	if got["mobile-fmp4"].OutputMux != streamMuxFMP4 {
		t.Fatalf("OutputMux=%q", got["mobile-fmp4"].OutputMux)
	}
	if got["copy-ts"].Transcode == nil || *got["copy-ts"].Transcode {
		t.Fatalf("expected transcode false: %#v", got["copy-ts"])
	}
	if got["web-hls"].OutputMux != streamMuxHLS {
		t.Fatalf("OutputMux=%q", got["web-hls"].OutputMux)
	}
}

func TestPreferredOutputMuxForProfile_namedProfile(t *testing.T) {
	enable := true
	g := &Gateway{
		NamedProfiles: map[string]NamedStreamProfile{
			"mobile-fmp4": {
				BaseProfile: profileLowBitrate,
				Transcode:   &enable,
				OutputMux:   streamMuxFMP4,
			},
			"web-hls": {
				BaseProfile: profileDashFast,
				Transcode:   &enable,
				OutputMux:   streamMuxHLS,
			},
		},
	}
	if got := g.preferredOutputMuxForProfile("mobile-fmp4", "", true); got != streamMuxFMP4 {
		t.Fatalf("preferred mux=%q want %q", got, streamMuxFMP4)
	}
	if got := g.preferredOutputMuxForProfile("mobile-fmp4", "mpegts", true); got != streamMuxMPEGTS {
		t.Fatalf("request mux should win, got %q", got)
	}
	if got := g.preferredOutputMuxForProfile("web-hls", "", true); got != streamMuxHLS {
		t.Fatalf("preferred mux=%q want %q", got, streamMuxHLS)
	}
}

func TestLoadNamedProfilesFile_emptyPath(t *testing.T) {
	got, err := loadNamedProfilesFile("")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("want nil map, got %#v", got)
	}
}

func TestLoadNamedProfilesFile_invalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := loadNamedProfilesFile(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadNamedProfilesFile_omittedBaseProfileUsesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	body := `{"custom": {"transcode": false}}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := loadNamedProfilesFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spec := got["custom"]
	if spec.BaseProfile != profileDefault {
		t.Fatalf("BaseProfile=%q want %q", spec.BaseProfile, profileDefault)
	}
	if spec.Transcode == nil || *spec.Transcode {
		t.Fatalf("want transcode false: %#v", spec)
	}
}

func TestResolveProfileSelection_namedProfileNilTranscodeDefaultsTrue(t *testing.T) {
	g := &Gateway{
		NamedProfiles: map[string]NamedStreamProfile{
			"panel-a": {BaseProfile: profileAACCFR},
		},
	}
	sel := g.resolveProfileSelection("panel-a")
	if !sel.Known || sel.Name != "panel-a" || sel.BaseProfile != profileAACCFR || !sel.ForceTranscode {
		t.Fatalf("unexpected: %#v", sel)
	}
}

func TestResolveProfileSelection_namedProfileExplicitRemux(t *testing.T) {
	off := false
	g := &Gateway{
		NamedProfiles: map[string]NamedStreamProfile{
			"panel-b": {BaseProfile: profileDashFast, Transcode: &off, OutputMux: streamMuxMPEGTS},
		},
	}
	sel := g.resolveProfileSelection("panel-b")
	if !sel.Known || sel.ForceTranscode || sel.BaseProfile != profileDashFast {
		t.Fatalf("unexpected: %#v", sel)
	}
}

func TestResolveProfileSelection_builtinInternet360(t *testing.T) {
	sel := (&Gateway{}).resolveProfileSelection("internet360")
	if !sel.Known || sel.BaseProfile != profileAACCFR || sel.Name != profileAACCFR {
		t.Fatalf("unexpected: %#v", sel)
	}
}

func TestResolveProfileSelection_unknownCustomName(t *testing.T) {
	sel := (&Gateway{NamedProfiles: map[string]NamedStreamProfile{}}).resolveProfileSelection("totally-unknown-panel-label")
	if sel.Known || sel.BaseProfile != profileDefault || sel.Name != "totally-unknown-panel-label" {
		t.Fatalf("unexpected: %#v", sel)
	}
}

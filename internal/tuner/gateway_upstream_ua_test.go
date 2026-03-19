package tuner

import (
	"strings"
	"testing"
)

func TestResolveUserAgentPreset_lavf(t *testing.T) {
	for _, name := range []string{"lavf", "ffmpeg", "FFMPEG", "Lavf", "libavformat"} {
		got := resolveUserAgentPreset(name, "")
		if !strings.HasPrefix(got, "Lavf/") {
			t.Errorf("resolveUserAgentPreset(%q) = %q, want Lavf/...", name, got)
		}
	}
}

func TestResolveUserAgentPreset_lavfWithDetected(t *testing.T) {
	detected := "Lavf/61.99.1"
	got := resolveUserAgentPreset("lavf", detected)
	if got != detected {
		t.Errorf("got %q, want %q", got, detected)
	}
}

func TestResolveUserAgentPreset_vlc(t *testing.T) {
	got := resolveUserAgentPreset("vlc", "")
	if !strings.Contains(got, "VLC") {
		t.Errorf("resolveUserAgentPreset(vlc) = %q, want VLC UA", got)
	}
}

func TestResolveUserAgentPreset_firefox(t *testing.T) {
	got := resolveUserAgentPreset("firefox", "")
	if !strings.Contains(got, "Firefox") {
		t.Errorf("resolveUserAgentPreset(firefox) = %q, want Firefox UA", got)
	}
}

func TestResolveUserAgentPreset_kodi(t *testing.T) {
	got := resolveUserAgentPreset("kodi", "")
	if !strings.Contains(got, "Kodi") {
		t.Errorf("resolveUserAgentPreset(kodi) = %q, want Kodi UA", got)
	}
}

func TestResolveUserAgentPreset_passthrough(t *testing.T) {
	custom := "MyCustomApp/2.0"
	got := resolveUserAgentPreset(custom, "Lavf/61.7.100")
	if got != custom {
		t.Errorf("resolveUserAgentPreset(%q) = %q, want passthrough", custom, got)
	}
}

func TestDetectFFmpegLavfUA_format(t *testing.T) {
	// detectFFmpegLavfUA should return "" or a "Lavf/X.Y.Z" string — never garbage.
	got := detectFFmpegLavfUA()
	if got == "" {
		t.Skip("ffprobe/ffmpeg not installed; skipping UA detection test")
	}
	if !strings.HasPrefix(got, "Lavf/") {
		t.Errorf("detectFFmpegLavfUA() = %q, want Lavf/X.Y.Z", got)
	}
	ver := strings.TrimPrefix(got, "Lavf/")
	if !strings.Contains(ver, ".") {
		t.Errorf("version %q has no dot separator", ver)
	}
}

func TestEffectiveUpstreamUserAgent_lavfPreset(t *testing.T) {
	g := &Gateway{
		CustomUserAgent:  "lavf",
		DetectedFFmpegUA: "Lavf/61.7.100",
	}
	got := g.effectiveUpstreamUserAgent(nil)
	if got != "Lavf/61.7.100" {
		t.Errorf("effectiveUpstreamUserAgent = %q, want Lavf/61.7.100", got)
	}
}

func TestEffectiveUpstreamUserAgent_lavfFallback(t *testing.T) {
	g := &Gateway{
		CustomUserAgent:  "lavf",
		DetectedFFmpegUA: "", // detection failed, should use default
	}
	got := g.effectiveUpstreamUserAgent(nil)
	if !strings.HasPrefix(got, "Lavf/") {
		t.Errorf("effectiveUpstreamUserAgent = %q, want Lavf/... fallback", got)
	}
}

func TestEffectiveUpstreamUserAgent_default(t *testing.T) {
	g := &Gateway{}
	got := g.effectiveUpstreamUserAgent(nil)
	if got != "IptvTunerr/1.0" {
		t.Errorf("effectiveUpstreamUserAgent = %q, want IptvTunerr/1.0", got)
	}
}

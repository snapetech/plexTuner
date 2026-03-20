package tuner

import (
	"context"
	"testing"
)

func TestEffectiveTranscodeForChannelMeta_overrideWhenOff(t *testing.T) {
	g := &Gateway{
		StreamTranscodeMode: "off",
		TranscodeOverrides:  map[string]bool{"ch1": true},
	}
	if !g.effectiveTranscodeForChannelMeta(context.Background(), "ch1", "", "", "http://example/stream") {
		t.Fatal("override file should force transcode when global mode is off")
	}
}

func TestEffectiveTranscodeForChannelMeta_overrideWhenOn(t *testing.T) {
	g := &Gateway{
		StreamTranscodeMode: "on",
		TranscodeOverrides:  map[string]bool{"ch1": false},
	}
	if g.effectiveTranscodeForChannelMeta(context.Background(), "ch1", "", "", "http://example/stream") {
		t.Fatal("override file should force remux when global mode is on")
	}
}

func TestEffectiveTranscodeForChannelMeta_guideNumberKey(t *testing.T) {
	g := &Gateway{
		StreamTranscodeMode: "off",
		TranscodeOverrides:  map[string]bool{"501": true},
	}
	if !g.effectiveTranscodeForChannelMeta(context.Background(), "other", "501", "", "http://x") {
		t.Fatal("expected guide_number key to match override map")
	}
}

func TestEffectiveTranscodeForChannelMeta_tvgIDKey(t *testing.T) {
	g := &Gateway{
		StreamTranscodeMode: "off",
		TranscodeOverrides:  map[string]bool{"i0.uk": true},
	}
	if !g.effectiveTranscodeForChannelMeta(context.Background(), "x", "", "i0.uk", "http://x") {
		t.Fatal("expected tvg_id key to match override map")
	}
}

func TestEffectiveTranscodeForChannelMeta_autoCachedNoMatchRemux(t *testing.T) {
	g := &Gateway{
		StreamTranscodeMode: "auto_cached",
		TranscodeOverrides:  map[string]bool{"other": true},
	}
	if g.effectiveTranscodeForChannelMeta(context.Background(), "ch1", "", "", "http://x") {
		t.Fatal("auto_cached without key should remux")
	}
}

func TestEffectiveTranscodeForChannelMeta_autoCachedMatch(t *testing.T) {
	g := &Gateway{
		StreamTranscodeMode: "auto_cached",
		TranscodeOverrides:  map[string]bool{"ch1": true},
	}
	if !g.effectiveTranscodeForChannelMeta(context.Background(), "ch1", "", "", "http://x") {
		t.Fatal("auto_cached with key should use file value")
	}
}

func TestEffectiveTranscodeForChannelMeta_noOverrideUsesBase(t *testing.T) {
	g := &Gateway{
		StreamTranscodeMode: "off",
		TranscodeOverrides:  nil,
	}
	if g.effectiveTranscodeForChannelMeta(context.Background(), "ch1", "", "", "http://x") {
		t.Fatal("off without override should remux")
	}
}

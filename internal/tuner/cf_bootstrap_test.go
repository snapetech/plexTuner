package tuner

import "testing"

func TestAllowRealBrowserCFFallback_DefaultDisabled(t *testing.T) {
	t.Setenv("DISPLAY", ":99")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("IPTV_TUNERR_CF_REAL_BROWSER_FALLBACK", "")

	if allowRealBrowserCFFallback() {
		t.Fatal("expected real-browser CF fallback to stay disabled by default")
	}
}

func TestAllowRealBrowserCFFallback_RequiresDisplayAndOptIn(t *testing.T) {
	t.Setenv("DISPLAY", ":99")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("IPTV_TUNERR_CF_REAL_BROWSER_FALLBACK", "true")

	if !allowRealBrowserCFFallback() {
		t.Fatal("expected real-browser CF fallback to enable only with display and explicit opt-in")
	}
}

package plexlabelproxy

import (
	"strings"
	"testing"
)

func TestLiveProviderIdentFromPath(t *testing.T) {
	cases := map[string]string{
		"/tv.plex.providers.epg.xmltv:135":           "tv.plex.providers.epg.xmltv:135",
		"/tv.plex.providers.epg.xmltv:135/grid":      "tv.plex.providers.epg.xmltv:135",
		"/tv.plex.providers.epg.xmltv:7/lineups/dvr": "tv.plex.providers.epg.xmltv:7",
		"/media/providers":                           "",
		"/identity":                                  "",
		"/tv.plex.providers.something.else:1":        "",
	}
	for in, want := range cases {
		if got := LiveProviderIdentFromPath(in); got != want {
			t.Errorf("path=%q got=%q want=%q", in, got, want)
		}
	}
}

func TestRewriteMediaProvidersXML_RewritesLiveProviders(t *testing.T) {
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="3" friendlyName="plexKube">
  <MediaProvider identifier="com.plexapp.plugins.library" friendlyName="plexKube" title="Library"/>
  <MediaProvider identifier="tv.plex.providers.epg.xmltv:135" friendlyName="plexKube" sourceTitle="plexKube" title="Live TV &amp; DVR">
    <Feature type="content">
      <Directory id="tv.plex.providers.epg.xmltv:135" title="Live TV &amp; DVR"/>
      <Directory key="/tv.plex.providers.epg.xmltv:135/watchnow" title="Guide"/>
    </Feature>
  </MediaProvider>
  <MediaProvider identifier="tv.plex.providers.epg.xmltv:136" friendlyName="plexKube" title="Live TV &amp; DVR"/>
</MediaContainer>`)
	labels := map[string]string{
		"tv.plex.providers.epg.xmltv:135": "newsus",
		"tv.plex.providers.epg.xmltv:136": "sports",
	}
	out, err := rewriteMediaProvidersXML(in, labels)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	// Library provider untouched.
	if !strings.Contains(s, `identifier="com.plexapp.plugins.library"`) || !strings.Contains(s, `friendlyName="plexKube"`) {
		t.Errorf("library provider should retain plexKube friendlyName, body=%s", s)
	}
	// LiveTV providers rewritten.
	if !strings.Contains(s, `identifier="tv.plex.providers.epg.xmltv:135"`) {
		t.Fatalf("missing first LiveTV provider, body=%s", s)
	}
	if !contains(s, `friendlyName="newsus"`) {
		t.Errorf("LiveTV 135 friendlyName not rewritten: %s", s)
	}
	if !contains(s, `sourceTitle="newsus"`) {
		t.Errorf("LiveTV 135 sourceTitle not rewritten: %s", s)
	}
	if !contains(s, `friendlyName="sports"`) {
		t.Errorf("LiveTV 136 friendlyName not rewritten: %s", s)
	}
	// Directory child for the watchnow path becomes "<label> Guide".
	if !contains(s, `title="newsus Guide"`) {
		t.Errorf("watchnow directory not retitled: %s", s)
	}
}

func TestRewriteMediaProvidersXML_NoMatch_ReturnsUnchanged(t *testing.T) {
	in := []byte(`<MediaContainer><MediaProvider identifier="com.plexapp.plugins.library" friendlyName="plexKube"/></MediaContainer>`)
	out, err := rewriteMediaProvidersXML(in, map[string]string{"tv.plex.providers.epg.xmltv:999": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(in) {
		t.Fatalf("expected byte-equal passthrough, got %s", out)
	}
}

func TestRewriteMediaProvidersXML_EmptyLabels_NoOp(t *testing.T) {
	in := []byte(`<MediaContainer><MediaProvider identifier="tv.plex.providers.epg.xmltv:1" friendlyName="plexKube"/></MediaContainer>`)
	out, err := rewriteMediaProvidersXML(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(in) {
		t.Fatalf("nil labels should pass through unchanged, got %s", out)
	}
}

func TestRewriteProviderScopedXML_RewritesRootContainer(t *testing.T) {
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer friendlyName="plexKube" title1="Live TV &amp; DVR" title2="Guide">
  <Directory id="tv.plex.providers.epg.xmltv:135" title="Live TV &amp; DVR"/>
  <Directory key="/tv.plex.providers.epg.xmltv:135/watchnow" title="Guide"/>
</MediaContainer>`)
	out, err := rewriteProviderScopedXML(in, "tv.plex.providers.epg.xmltv:135", "newsus")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !contains(s, `friendlyName="newsus"`) {
		t.Errorf("root friendlyName not rewritten: %s", s)
	}
	if !contains(s, `title1="newsus"`) || !contains(s, `title2="newsus"`) {
		t.Errorf("title1/title2 not rewritten: %s", s)
	}
	if !contains(s, `title="newsus Guide"`) {
		t.Errorf("watchnow title not rewritten: %s", s)
	}
}

func TestRewriteProviderScopedXML_PreservesNonGenericTitle(t *testing.T) {
	in := []byte(`<MediaContainer friendlyName="plexKube" title1="Custom Show Title"/>`)
	out, err := rewriteProviderScopedXML(in, "tv.plex.providers.epg.xmltv:1", "newsus")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !contains(s, `title1="Custom Show Title"`) {
		t.Errorf("non-generic title1 should be preserved, got %s", s)
	}
	if !contains(s, `friendlyName="newsus"`) {
		t.Errorf("friendlyName should still be rewritten: %s", s)
	}
}

func TestRewriteRootIdentityXML_OnlyTouchesFriendlyName(t *testing.T) {
	in := []byte(`<MediaContainer machineIdentifier="abc" friendlyName="plexKube" version="1.43.0"/>`)
	out, err := rewriteRootIdentityXML(in, "newsus")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !contains(s, `machineIdentifier="abc"`) {
		t.Errorf("machineIdentifier must be preserved: %s", s)
	}
	if !contains(s, `friendlyName="newsus"`) {
		t.Errorf("friendlyName not rewritten: %s", s)
	}
	if !contains(s, `version="1.43.0"`) {
		t.Errorf("version must be preserved: %s", s)
	}
}

func TestRewriteRootIdentityXML_NoFriendlyNameAttr_NoOp(t *testing.T) {
	in := []byte(`<MediaContainer machineIdentifier="abc"/>`)
	out, err := rewriteRootIdentityXML(in, "newsus")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) == "" || !contains(string(out), `machineIdentifier="abc"`) {
		t.Fatalf("expected passthrough preserving root, got %q", out)
	}
}

func TestIsGenericTitle(t *testing.T) {
	for _, s := range []string{"", "Plex Library", "Live TV & DVR", "Guide", "  Guide  "} {
		if !isGenericTitle(s) {
			t.Errorf("%q should be generic", s)
		}
	}
	for _, s := range []string{"newsus", "My Sports Tab", "ESPN Pack"} {
		if isGenericTitle(s) {
			t.Errorf("%q should not be generic", s)
		}
	}
}

// contains is strings.Contains shorthand kept local for test readability.
func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

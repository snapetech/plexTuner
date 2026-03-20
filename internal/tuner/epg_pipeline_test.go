package tuner

import "testing"

func TestProviderXMLTVEPGURL_suffix(t *testing.T) {
	got := providerXMLTVEPGURL("http://example.com:8080/", "user", "pass", "foo=1&bar=2")
	want := "http://example.com:8080/xmltv.php?username=user&password=pass&foo=1&bar=2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got2 := providerXMLTVEPGURL("http://example.com", "u", "p", "&x=y")
	if want2 := "http://example.com/xmltv.php?username=u&password=p&x=y"; got2 != want2 {
		t.Fatalf("got %q want %q", got2, want2)
	}
}

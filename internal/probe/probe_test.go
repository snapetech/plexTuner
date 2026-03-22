package probe

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestProbe_classifyByURLPath(t *testing.T) {
	cases := []struct {
		url  string
		want StreamType
	}{
		{"http://x/a.m3u8", StreamHLS},
		{"https://cdn/x.ts", StreamHLS},
		{"http://x/movie.mp4", StreamDirectMP4},
		{"http://x/V.M4V", StreamDirectMP4},
		{"http://x/a.mkv", StreamDirectFile},
		{"http://x/a.webm", StreamDirectFile},
		{"http://x/a.avi", StreamDirectFile},
		{"http://x/a.mov", StreamDirectFile},
	}
	for _, tc := range cases {
		got, err := Probe(tc.url, nil)
		if err != nil {
			t.Fatalf("%q: unexpected err: %v", tc.url, err)
		}
		if got != tc.want {
			t.Errorf("%q: got %q want %q", tc.url, got, tc.want)
		}
	}
}

func TestProbe_invalidURL(t *testing.T) {
	_, err := Probe(" ://bad", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestProbe_http_contentType_mpegurl(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not read for type"))
	}))
	defer ts.Close()
	got, err := Probe(ts.URL+"/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != StreamHLS {
		t.Fatalf("got %q", got)
	}
}

func TestProbe_http_contentType_octetStream(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "x")
	}))
	defer ts.Close()
	got, err := Probe(ts.URL+"/bin", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != StreamDirectFile {
		t.Fatalf("got %q", got)
	}
}

func TestProbe_http_sniff_m3u8(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Avoid application/octet-stream — Probe classifies that as direct_file before sniffing the body.
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "#EXTM3U\n#EXTINF\n")
	}))
	defer ts.Close()
	got, err := Probe(ts.URL+"/playlist", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != StreamHLS {
		t.Fatalf("got %q", got)
	}
}

func TestProbe_http_sniff_mp4_ftyp(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		// Box size (4) + "ftyp" + brand — Probe requires len(body)>=12 to sniff MP4.
		w.Write([]byte{0, 0, 0, 0x20, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'})
	}))
	defer ts.Close()
	got, err := Probe(ts.URL+"/media", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != StreamDirectMP4 {
		t.Fatalf("got %q", got)
	}
}

func TestProbe_http_sniff_matroska(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0})
	}))
	defer ts.Close()
	got, err := Probe(ts.URL+"/mkv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != StreamDirectFile {
		t.Fatalf("got %q", got)
	}
}

func TestProbe_http_unknown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "not media")
	}))
	defer ts.Close()
	_, err := Probe(ts.URL+"/x", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown stream type") {
		t.Fatalf("want unknown stream type, got %v", err)
	}
}

func TestProbe_redirect_classifiesFinalPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/real.m3u8", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	got, err := Probe(ts.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != StreamHLS {
		t.Fatalf("got %q", got)
	}
}

func TestLineup_withBaseURL(t *testing.T) {
	ch := []catalog.LiveChannel{{
		GuideNumber: "1",
		GuideName:   "One",
		StreamURL:   "http://up/s.m3u8",
	}}
	items := Lineup(ch, "http://tuner:5004")
	if len(items) != 1 {
		t.Fatal(len(items))
	}
	if items[0].GuideNumber != "1" || items[0].GuideName != "One" {
		t.Fatalf("%+v", items[0])
	}
	if !strings.HasPrefix(items[0].URL, "http://tuner:5004/stream?url=") {
		t.Fatalf("URL: %q", items[0].URL)
	}
	if !strings.Contains(items[0].URL, "http%3A%2F%2Fup%2Fs.m3u8") {
		t.Fatalf("escaped upstream missing: %q", items[0].URL)
	}
}

func TestLineup_withBaseURL_NormalizesWhitespaceAndTrailingSlashes(t *testing.T) {
	ch := []catalog.LiveChannel{{
		GuideNumber: "1",
		GuideName:   "One",
		StreamURL:   "http://up/s.m3u8",
	}}
	items := Lineup(ch, "  http://tuner:5004///  ")
	if len(items) != 1 {
		t.Fatal(len(items))
	}
	if got := items[0].URL; got != "http://tuner:5004/stream?url=http%3A%2F%2Fup%2Fs.m3u8" {
		t.Fatalf("URL: %q", got)
	}
}

func TestLineup_noBaseURL(t *testing.T) {
	ch := []catalog.LiveChannel{{StreamURL: "http://direct/x"}}
	items := Lineup(ch, "")
	if items[0].URL != "http://direct/x" {
		t.Fatalf("%q", items[0].URL)
	}
}

func TestLineupHandler(t *testing.T) {
	h := LineupHandler(func() []LineupItem {
		return []LineupItem{{GuideNumber: "2", GuideName: "B", URL: "u"}}
	})

	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/lineup.json", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	var got []LineupItem
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].GuideNumber != "2" {
		t.Fatalf("%+v", got)
	}
}

func TestLineupHandler_requiresGetOrHead(t *testing.T) {
	h := LineupHandler(func() []LineupItem {
		return []LineupItem{{GuideNumber: "2", GuideName: "B", URL: "u"}}
	})

	req := httptest.NewRequest(http.MethodPost, "/lineup.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code %d", w.Code)
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("allow %q", got)
	}
}

func TestDiscoveryHandler(t *testing.T) {
	h := DiscoveryHandler("dev1", "Friendly")

	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/device.xml", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "dev1") || !strings.Contains(body, "Friendly") {
		t.Fatalf("body: %s", body)
	}
}

func TestDiscoveryHandler_requiresGetOrHead(t *testing.T) {
	h := DiscoveryHandler("dev1", "Friendly")

	req := httptest.NewRequest(http.MethodPost, "/device.xml", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code %d", w.Code)
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("allow %q", got)
	}
}

func TestDiscoveryHandlerEscapesXML(t *testing.T) {
	h := DiscoveryHandler("dev&1<2>", "AT&T <Friendly>")

	req := httptest.NewRequest(http.MethodGet, "/device.xml", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<DeviceID>dev&amp;1&lt;2&gt;</DeviceID>") {
		t.Fatalf("body: %s", body)
	}
	if !strings.Contains(body, "<FriendlyName>AT&amp;T &lt;Friendly&gt;</FriendlyName>") {
		t.Fatalf("body: %s", body)
	}
}

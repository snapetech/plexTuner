package tuner

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestXMLTV_serve(t *testing.T) {
	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	dec := xml.NewDecoder(w.Body)
	var tv struct {
		Channels []struct {
			ID      string `xml:"id,attr"`
			Display string `xml:"display-name"`
		} `xml:"channel"`
	}
	if err := dec.Decode(&tv); err != nil {
		t.Fatal(err)
	}
	if len(tv.Channels) != 1 || tv.Channels[0].ID != "1" || tv.Channels[0].Display != "Ch1" {
		t.Errorf("channels: %+v", tv.Channels)
	}
}

func TestXMLTV_404(t *testing.T) {
	x := &XMLTV{}
	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestXMLTV_epgPruneUnlinked(t *testing.T) {
	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "With TVG", TVGID: "id1"},
			{GuideNumber: "2", GuideName: "No TVG", TVGID: ""},
			{GuideNumber: "3", GuideName: "With TVG 2", TVGID: "id3"},
		},
		EpgPruneUnlinked: true,
	}
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	dec := xml.NewDecoder(w.Body)
	var tv struct {
		Channels []struct {
			ID      string `xml:"id,attr"`
			Display string `xml:"display-name"`
		} `xml:"channel"`
	}
	if err := dec.Decode(&tv); err != nil {
		t.Fatal(err)
	}
	if len(tv.Channels) != 2 {
		t.Fatalf("EpgPruneUnlinked should include only 2 channels with TVGID; got %d", len(tv.Channels))
	}
	ids := make(map[string]string)
	for _, ch := range tv.Channels {
		ids[ch.ID] = ch.Display
	}
	if ids["1"] != "With TVG" || ids["3"] != "With TVG 2" {
		t.Errorf("channels: %+v", tv.Channels)
	}
}

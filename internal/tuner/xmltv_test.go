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

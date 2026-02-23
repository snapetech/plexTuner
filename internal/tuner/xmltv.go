package tuner

import (
	"encoding/xml"
	"net/http"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

// XMLTV generates a minimal XMLTV guide so Plex can import guide data.
// Programmes can be extended later (e.g. from EPG URL or placeholder).
type XMLTV struct {
	Channels         []catalog.LiveChannel
	EpgPruneUnlinked bool // when true, only include channels with TVGID set
}

func (x *XMLTV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/guide.xml" {
		http.NotFound(w, r)
		return
	}
	channels := x.Channels
	if channels == nil {
		channels = []catalog.LiveChannel{}
	}
	if x.EpgPruneUnlinked {
		filtered := make([]catalog.LiveChannel, 0, len(channels))
		for _, c := range channels {
			if c.TVGID != "" {
				filtered = append(filtered, c)
			}
		}
		channels = filtered
	}

	now := time.Now()
	start := now.Add(-24 * time.Hour).Format("20060102150405")
	stop := now.Add(7 * 24 * time.Hour).Format("20060102150405")

	tv := &xmlTVRoot{
		XMLName: xml.Name{Local: "tv"},
		Source:  "Plex Tuner",
	}
	for _, c := range channels {
		tv.Channels = append(tv.Channels, xmlChannel{
			ID:      c.GuideNumber,
			Display: c.GuideName,
		})
		tv.Programmes = append(tv.Programmes, xmlProgramme{
			Start:   start,
			Stop:    stop,
			Channel: c.GuideNumber,
			Title:   xmlValue{Value: c.GuideName},
		})
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	enc.Encode(tv)
}

type xmlTVRoot struct {
	XMLName    xml.Name       `xml:"tv"`
	Source     string         `xml:"source-info-name,attr,omitempty"`
	Channels   []xmlChannel   `xml:"channel"`
	Programmes []xmlProgramme `xml:"programme"`
}

type xmlChannel struct {
	ID      string `xml:"id,attr"`
	Display string `xml:"display-name"`
}

type xmlProgramme struct {
	Start   string   `xml:"start,attr"`
	Stop    string   `xml:"stop,attr"`
	Channel string   `xml:"channel,attr"`
	Title   xmlValue `xml:"title"`
}

type xmlValue struct {
	Value string `xml:",chardata"`
}

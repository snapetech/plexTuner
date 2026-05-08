package webui

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// guideXMLTV is a minimal XMLTV unmarshal target.
type guideXMLTV struct {
	XMLName    xml.Name          `xml:"tv"`
	Channels   []guideXMLChannel `xml:"channel"`
	Programmes []guideXMLProg    `xml:"programme"`
}

type guideXMLChannel struct {
	ID          string `xml:"id,attr"`
	DisplayName string `xml:"display-name"`
	Icon        struct {
		Src string `xml:"src,attr"`
	} `xml:"icon"`
}

type guideXMLProg struct {
	Start    string   `xml:"start,attr"`
	Stop     string   `xml:"stop,attr"`
	Channel  string   `xml:"channel,attr"`
	Title    xmlText  `xml:"title"`
	SubTitle xmlText  `xml:"sub-title"`
	Desc     xmlText  `xml:"desc"`
	Category []xmlText `xml:"category"`
	Icon     struct {
		Src string `xml:"src,attr"`
	} `xml:"icon"`
}

type xmlText struct {
	Value string `xml:",chardata"`
}

// GuideProgramme is the JSON shape returned to the frontend.
type GuideProgramme struct {
	Title      string   `json:"title"`
	SubTitle   string   `json:"sub_title,omitempty"`
	Desc       string   `json:"desc,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Start      string   `json:"start"`
	Stop       string   `json:"stop"`
	StartUnix  int64    `json:"start_unix"`
	StopUnix   int64    `json:"stop_unix"`
	Icon       string   `json:"icon,omitempty"`
}

// GuideChannel is the JSON shape for one channel row in the grid.
type GuideChannel struct {
	EpgID      string           `json:"epg_id"`
	Name       string           `json:"name"`
	Icon       string           `json:"icon,omitempty"`
	Programmes []GuideProgramme `json:"programmes"`
}

// GuideGridResponse is the top-level JSON response.
type GuideGridResponse struct {
	From     string         `json:"from"`
	To       string         `json:"to"`
	Channels []GuideChannel `json:"channels"`
}

var xmltvLayouts = []string{
	"20060102150405 -0700",
	"20060102150405 +0000",
	"20060102150405",
}

func unmarshalXMLTV(data []byte, out *guideXMLTV) error {
	return xml.Unmarshal(data, out)
}

func parseXMLTVTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range xmltvLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// GET /api/v2/guide?from=RFC3339&to=RFC3339
// Fetches the tuner's guide.xml and returns a grid-friendly JSON response.
func (s *Server) v2Guide(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now()
	from := now
	to := now.Add(3 * time.Hour)

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	grid, err := s.fetchGuideGrid(from, to)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("guide fetch: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, grid)
}

func (s *Server) fetchGuideGrid(from, to time.Time) (*GuideGridResponse, error) {
	base := strings.TrimRight(s.tunerBase, "/")
	if base == "" {
		return &GuideGridResponse{
			From:     from.Format(time.RFC3339),
			To:       to.Format(time.RFC3339),
			Channels: []GuideChannel{},
		}, nil
	}

	resp, err := http.Get(base + "/api/guide.xml")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("guide.xml returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var xmltv guideXMLTV
	if err := xml.Unmarshal(body, &xmltv); err != nil {
		return nil, fmt.Errorf("parse guide.xml: %w", err)
	}

	// Build channel map.
	chanMap := make(map[string]*GuideChannel, len(xmltv.Channels))
	chanOrder := make([]string, 0, len(xmltv.Channels))
	for _, ch := range xmltv.Channels {
		gc := &GuideChannel{
			EpgID:      ch.ID,
			Name:       ch.DisplayName,
			Icon:       ch.Icon.Src,
			Programmes: []GuideProgramme{},
		}
		chanMap[ch.ID] = gc
		chanOrder = append(chanOrder, ch.ID)
	}

	fromUnix := from.Unix()
	toUnix := to.Unix()

	// Filter programmes that overlap [from, to].
	for _, p := range xmltv.Programmes {
		startT, ok1 := parseXMLTVTime(p.Start)
		stopT, ok2 := parseXMLTVTime(p.Stop)
		if !ok1 || !ok2 {
			continue
		}
		// Overlaps if start < to AND stop > from.
		if startT.Unix() >= toUnix || stopT.Unix() <= fromUnix {
			continue
		}
		ch, ok := chanMap[p.Channel]
		if !ok {
			continue
		}
		cats := make([]string, 0, len(p.Category))
		for _, c := range p.Category {
			if v := strings.TrimSpace(c.Value); v != "" {
				cats = append(cats, v)
			}
		}
		ch.Programmes = append(ch.Programmes, GuideProgramme{
			Title:      p.Title.Value,
			SubTitle:   p.SubTitle.Value,
			Desc:       p.Desc.Value,
			Categories: cats,
			Start:      startT.UTC().Format(time.RFC3339),
			Stop:       stopT.UTC().Format(time.RFC3339),
			StartUnix:  startT.Unix(),
			StopUnix:   stopT.Unix(),
			Icon:       p.Icon.Src,
		})
	}

	// Only return channels that have programmes in the window.
	out := make([]GuideChannel, 0, len(chanOrder))
	for _, id := range chanOrder {
		ch := chanMap[id]
		if len(ch.Programmes) > 0 {
			out = append(out, *ch)
		}
	}

	return &GuideGridResponse{
		From:     from.UTC().Format(time.RFC3339),
		To:       to.UTC().Format(time.RFC3339),
		Channels: out,
	}, nil
}

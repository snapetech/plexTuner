package tuner

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

// XMLTV serves /guide.xml. By default it emits a minimal placeholder XMLTV.
// When SourceURL is set, it fetches that XMLTV feed, filters to channels present
// in the live catalog, and remaps programme channel IDs to local guide numbers.
// The remapped result is cached for CacheTTL (default 10m) to avoid hammering
// the upstream on every Plex metadata refresh.
type XMLTV struct {
	Channels         []catalog.LiveChannel
	EpgPruneUnlinked bool // when true, only include channels with TVGID set
	SourceURL        string
	SourceTimeout    time.Duration
	Client           *http.Client
	CacheTTL         time.Duration // 0 = use default 10m; only used when SourceURL is set

	mu        sync.RWMutex
	cachedXML []byte
	cacheExp  time.Time
}

func (x *XMLTV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/guide.xml" {
		http.NotFound(w, r)
		return
	}
	channels := x.filteredChannels()
	if x.SourceURL != "" {
		if err := x.serveExternalXMLTV(w, channels); err == nil {
			return
		} else {
			log.Printf("xmltv: external source failed (%s); falling back to placeholder guide", err)
		}
	}
	x.servePlaceholderXMLTV(w, channels)
}

func (x *XMLTV) filteredChannels() []catalog.LiveChannel {
	channels := x.Channels
	if channels == nil {
		channels = []catalog.LiveChannel{}
	}
	if !x.EpgPruneUnlinked {
		return channels
	}
	filtered := make([]catalog.LiveChannel, 0, len(channels))
	for _, c := range channels {
		if strings.TrimSpace(c.TVGID) != "" {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func (x *XMLTV) serveExternalXMLTV(w http.ResponseWriter, channels []catalog.LiveChannel) error {
	if len(channels) == 0 {
		return errors.New("no live channels available")
	}

	ttl := x.CacheTTL
	if ttl == 0 {
		ttl = 10 * time.Minute
	}

	// Fast path: cache hit under read lock.
	x.mu.RLock()
	if len(x.cachedXML) > 0 && time.Now().Before(x.cacheExp) {
		data := x.cachedXML
		x.mu.RUnlock()
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, err := w.Write(data)
		return err
	}
	x.mu.RUnlock()

	// Cache miss — acquire write lock, double-check, then fetch.
	x.mu.Lock()
	defer x.mu.Unlock()
	if len(x.cachedXML) > 0 && time.Now().Before(x.cacheExp) {
		// Another goroutine populated the cache while we waited for the lock.
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, err := w.Write(x.cachedXML)
		return err
	}

	data, err := x.fetchExternalXMLTV(channels)
	if err != nil {
		return err
	}
	x.cachedXML = data
	x.cacheExp = time.Now().Add(ttl)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, err = w.Write(data)
	return err
}

// fetchExternalXMLTV performs the upstream HTTP fetch and remaps channel IDs.
// Called under the XMLTV write lock; returns the full remapped XML bytes.
func (x *XMLTV) fetchExternalXMLTV(channels []catalog.LiveChannel) ([]byte, error) {
	timeout := x.SourceTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	client := x.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	req, err := http.NewRequest(http.MethodGet, x.SourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	var buf bytes.Buffer
	if err := writeRemappedXMLTV(&buf, resp.Body, channels); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (x *XMLTV) servePlaceholderXMLTV(w http.ResponseWriter, channels []catalog.LiveChannel) {
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
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(tv)
}

func writeRemappedXMLTV(dst io.Writer, src io.Reader, channels []catalog.LiveChannel) error {
	type channelRef struct {
		GuideNumber string
		GuideName   string
		TVGID       string
	}
	byTVGID := make(map[string]channelRef, len(channels))
	ordered := make([]channelRef, 0, len(channels))
	for _, c := range channels {
		tvgID := strings.TrimSpace(c.TVGID)
		if tvgID == "" {
			continue
		}
		ref := channelRef{
			GuideNumber: strings.TrimSpace(c.GuideNumber),
			GuideName:   strings.TrimSpace(c.GuideName),
			TVGID:       tvgID,
		}
		if ref.GuideNumber == "" {
			continue
		}
		if _, exists := byTVGID[tvgID]; exists {
			continue
		}
		byTVGID[tvgID] = ref
		ordered = append(ordered, ref)
	}
	if len(byTVGID) == 0 {
		return errors.New("no TVGID-linked channels to remap")
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].GuideNumber == ordered[j].GuideNumber {
			return ordered[i].GuideName < ordered[j].GuideName
		}
		return ordered[i].GuideNumber < ordered[j].GuideNumber
	})

	dec := xml.NewDecoder(src)
	enc := xml.NewEncoder(dst)
	_, _ = io.WriteString(dst, xml.Header)

	var wroteRoot bool
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local != "tv" {
				// Skip everything until we find the root <tv>.
				_ = dec.Skip()
				continue
			}
			root := t
			if !hasXMLAttr(root.Attr, "source-info-name") {
				root.Attr = append(root.Attr, xml.Attr{Name: xml.Name{Local: "source-info-name"}, Value: "Plex Tuner (external XMLTV remap)"})
			}
			if err := enc.EncodeToken(root); err != nil {
				return err
			}
			for _, c := range ordered {
				node := xmlChannel{ID: c.GuideNumber, Display: c.GuideName}
				if err := enc.EncodeElement(node, xml.StartElement{Name: xml.Name{Local: "channel"}}); err != nil {
					return err
				}
			}
			wroteRoot = true
			// Consume the rest of the XMLTV document, copying only remapped programme nodes.
			for {
				subTok, subErr := dec.Token()
				if subErr != nil {
					if errors.Is(subErr, io.EOF) {
						break
					}
					return subErr
				}
				switch s := subTok.(type) {
				case xml.StartElement:
					switch s.Name.Local {
					case "channel":
						_ = dec.Skip()
					case "programme":
						var node xmlRawNode
						if err := dec.DecodeElement(&node, &s); err != nil {
							return err
						}
						srcID := strings.TrimSpace(xmlAttr(node.Attrs, "channel"))
						ref, ok := byTVGID[srcID]
						if !ok {
							continue
						}
						node.XMLName = xml.Name{Local: "programme"}
						node.Attrs = setXMLAttr(node.Attrs, "channel", ref.GuideNumber)
						if err := enc.EncodeElement(node, xml.StartElement{Name: xml.Name{Local: "programme"}}); err != nil {
							return err
						}
					default:
						_ = dec.Skip()
					}
				case xml.EndElement:
					if s.Name.Local == "tv" {
						if err := enc.EncodeToken(s); err != nil {
							return err
						}
						if err := enc.Flush(); err != nil {
							return err
						}
						return nil
					}
				}
			}
		}
	}
	if !wroteRoot {
		return errors.New("xmltv root <tv> not found")
	}
	return enc.Flush()
}

func hasXMLAttr(attrs []xml.Attr, key string) bool {
	for _, a := range attrs {
		if a.Name.Local == key {
			return true
		}
	}
	return false
}

func xmlAttr(attrs []xml.Attr, key string) string {
	for _, a := range attrs {
		if a.Name.Local == key {
			return a.Value
		}
	}
	return ""
}

func setXMLAttr(attrs []xml.Attr, key, value string) []xml.Attr {
	for i := range attrs {
		if attrs[i].Name.Local == key {
			attrs[i].Value = value
			return attrs
		}
	}
	return append(attrs, xml.Attr{Name: xml.Name{Local: key}, Value: value})
}

type xmlRawNode struct {
	XMLName  xml.Name   `xml:""`
	Attrs    []xml.Attr `xml:",any,attr"`
	InnerXML string     `xml:",innerxml"`
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

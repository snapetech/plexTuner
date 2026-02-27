package plex

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	_liveProviderRe     = regexp.MustCompile(`^tv\.plex\.providers\.epg\.xmltv:(\d+)$`)
	_liveProviderPathRe = regexp.MustCompile(`^/tv\.plex\.providers\.epg\.xmltv:(\d+)(?:/|$)`)
	_hopHeaders         = map[string]bool{
		"connection": true, "keep-alive": true, "proxy-authenticate": true,
		"proxy-authorization": true, "te": true, "trailers": true,
		"transfer-encoding": true, "upgrade": true,
	}
)

// LabelProxyConfig configures the /media/providers label rewriting proxy.
type LabelProxyConfig struct {
	Listen         string // host:port to listen on
	Upstream       string // Plex PMS URL
	Token          string
	StripPrefix    string
	RefreshSeconds int
}

type labelCache struct {
	cfg         LabelProxyConfig
	mu          sync.Mutex
	lastRefresh time.Time
	labels      map[string]string
}

func newLabelCache(cfg LabelProxyConfig) *labelCache {
	return &labelCache{cfg: cfg, labels: map[string]string{}}
}

func (c *labelCache) get() map[string]string {
	c.mu.Lock()
	age := time.Since(c.lastRefresh)
	cached := c.labels
	c.mu.Unlock()
	if age < time.Duration(c.cfg.RefreshSeconds)*time.Second && len(cached) > 0 {
		return cached
	}
	mp, err := fetchDVRLabelMap(c.cfg.Upstream, c.cfg.Token, c.cfg.StripPrefix)
	if err != nil {
		log.Printf("label_proxy: refresh DVR labels: %v", err)
		return cached
	}
	c.mu.Lock()
	c.labels = mp
	c.lastRefresh = time.Now()
	c.mu.Unlock()
	log.Printf("label_proxy: refreshed %d DVR labels", len(mp))
	return mp
}

func fetchDVRLabelMap(plexURL, token, stripPrefix string) (map[string]string, error) {
	u := strings.TrimRight(plexURL, "/") + "/livetv/dvrs?X-Plex-Token=" + url.QueryEscape(token)
	resp, err := http.Get(u) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET /livetv/dvrs: %d", resp.StatusCode)
	}

	var mc struct {
		XMLName xml.Name `xml:"MediaContainer"`
		DVRs    []struct {
			Key         string `xml:"key,attr"`
			ID          string `xml:"id,attr"`
			LineupTitle string `xml:"lineupTitle,attr"`
			Title       string `xml:"title,attr"`
			Lineup      string `xml:"lineup,attr"`
		} `xml:"Dvr"`
	}
	if err := xml.Unmarshal(body, &mc); err != nil {
		return nil, fmt.Errorf("parse /livetv/dvrs: %w", err)
	}

	out := map[string]string{}
	for _, dvr := range mc.DVRs {
		id := dvr.Key
		if id == "" {
			id = dvr.ID
		}
		if id == "" {
			continue
		}
		label := dvr.LineupTitle
		if label == "" {
			label = dvr.Title
		}
		if label == "" {
			if idx := strings.LastIndex(dvr.Lineup, "#"); idx >= 0 {
				label = dvr.Lineup[idx+1:]
			}
		}
		if label == "" {
			continue
		}
		if stripPrefix != "" && strings.HasPrefix(label, stripPrefix) {
			label = label[len(stripPrefix):]
		}
		out["tv.plex.providers.epg.xmltv:"+id] = label
	}
	return out, nil
}

// RewriteMediaProviders rewrites /media/providers XML replacing Live TV provider labels.
func RewriteMediaProviders(xmlBytes []byte, labels map[string]string) []byte {
	// Use a simple string-rewrite approach via re-marshalling.
	var root xmlMediaContainer
	if err := xml.Unmarshal(xmlBytes, &root); err != nil {
		return xmlBytes
	}
	changed := false
	for i, mp := range root.MediaProviders {
		ident := mp.Identifier
		if !_liveProviderRe.MatchString(ident) {
			continue
		}
		label, ok := labels[ident]
		if !ok {
			continue
		}
		root.MediaProviders[i].FriendlyName = label
		root.MediaProviders[i].SourceTitle = label
		root.MediaProviders[i].Title = label
		for j, feat := range mp.Features {
			if feat.Type != "content" {
				continue
			}
			for k, dir := range feat.Directories {
				if dir.ID == ident {
					root.MediaProviders[i].Features[j].Directories[k].Title = label
				} else if dir.Key == "/"+ident+"/watchnow" && dir.Title == "Guide" {
					root.MediaProviders[i].Features[j].Directories[k].Title = label + " Guide"
				}
			}
		}
		changed = true
	}
	if !changed {
		return xmlBytes
	}
	out, err := xml.Marshal(&root)
	if err != nil {
		return xmlBytes
	}
	return append([]byte(xml.Header), out...)
}

type xmlMediaContainer struct {
	XMLName        xml.Name           `xml:"MediaContainer"`
	Attrs          []xml.Attr         `xml:",any,attr"`
	MediaProviders []xmlMediaProvider `xml:"MediaProvider"`
	Other          []xmlRawElem       `xml:",any"`
}

type xmlMediaProvider struct {
	XMLName      xml.Name     `xml:"MediaProvider"`
	Identifier   string       `xml:"identifier,attr"`
	FriendlyName string       `xml:"friendlyName,attr"`
	SourceTitle  string       `xml:"sourceTitle,attr"`
	Title        string       `xml:"title,attr"`
	Attrs        []xml.Attr   `xml:",any,attr"`
	Features     []xmlFeature `xml:"Feature"`
	Other        []xmlRawElem `xml:",any"`
}

type xmlFeature struct {
	XMLName     xml.Name       `xml:"Feature"`
	Type        string         `xml:"type,attr"`
	Attrs       []xml.Attr     `xml:",any,attr"`
	Directories []xmlDirectory `xml:"Directory"`
	Other       []xmlRawElem   `xml:",any"`
}

type xmlDirectory struct {
	XMLName xml.Name   `xml:"Directory"`
	ID      string     `xml:"id,attr"`
	Key     string     `xml:"key,attr"`
	Title   string     `xml:"title,attr"`
	Attrs   []xml.Attr `xml:",any,attr"`
}

type xmlRawElem struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Inner   []byte     `xml:",innerxml"`
}

// RunLabelProxy starts the HTTP reverse proxy listening on cfg.Listen.
// Blocks until the server errors or the process exits.
func RunLabelProxy(cfg LabelProxyConfig) error {
	cache := newLabelCache(cfg)
	cache.get() // initial fetch

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		proxyRequest(w, r, cfg.Upstream, cache)
	})
	log.Printf("label_proxy: listening on %s -> %s", cfg.Listen, cfg.Upstream)
	return http.ListenAndServe(cfg.Listen, mux)
}

func proxyRequest(w http.ResponseWriter, r *http.Request, upstream string, cache *labelCache) {
	pu, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, "bad upstream", 502)
		return
	}
	target := *pu
	target.Path = strings.TrimRight(target.Path, "/") + r.URL.Path
	target.RawQuery = r.URL.RawQuery

	outReq, err := http.NewRequest(r.Method, target.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	for k, vs := range r.Header {
		if _hopHeaders[strings.ToLower(k)] || strings.ToLower(k) == "host" {
			continue
		}
		for _, v := range vs {
			outReq.Header.Add(k, v)
		}
	}
	outReq.Header.Set("Host", pu.Host)

	resp, err := http.DefaultTransport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	isMediaProviders := r.URL.Path == "/media/providers"
	isProviderScoped := _liveProviderPathRe.MatchString(r.URL.Path)
	ct := resp.Header.Get("Content-Type")
	ce := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if (isMediaProviders || isProviderScoped) && resp.StatusCode == 200 &&
		(strings.Contains(strings.ToLower(ct), "xml") || bytes.HasPrefix(bytes.TrimSpace(respBody), []byte("<?xml"))) {
		raw := respBody
		if ce == "gzip" {
			if d, err := gzip.NewReader(bytes.NewReader(respBody)); err == nil {
				raw, _ = io.ReadAll(d)
				d.Close()
			}
		}
		labels := cache.get()
		if isMediaProviders {
			raw = RewriteMediaProviders(raw, labels)
		}
		if isProviderScoped {
			raw = rewriteProviderScopedXML(r.URL.Path, raw, labels)
		}
		if ce == "gzip" {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			_, _ = gz.Write(raw)
			_ = gz.Close()
			respBody = buf.Bytes()
		} else {
			respBody = raw
		}
	}

	for k, vs := range resp.Header {
		lk := strings.ToLower(k)
		if _hopHeaders[lk] || lk == "content-length" {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(respBody)))
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = w.Write(respBody)
	}
}

func rewriteProviderScopedXML(path string, xmlBytes []byte, labels map[string]string) []byte {
	m := _liveProviderPathRe.FindStringSubmatch(path)
	if m == nil {
		return xmlBytes
	}
	ident := "tv.plex.providers.epg.xmltv:" + m[1]
	label, ok := labels[ident]
	if !ok {
		return xmlBytes
	}

	var mc struct {
		XMLName xml.Name   `xml:"MediaContainer"`
		Attrs   []xml.Attr `xml:",any,attr"`
		Inner   []byte     `xml:",innerxml"`
	}
	if err := xml.Unmarshal(xmlBytes, &mc); err != nil {
		return xmlBytes
	}

	changed := false
	for i, a := range mc.Attrs {
		switch a.Name.Local {
		case "title", "title1":
			if a.Value == "Plex Library" || a.Value == "Live TV & DVR" || a.Value == "Guide" || a.Value == "" {
				mc.Attrs[i].Value = label
				changed = true
			}
		case "title2":
			if a.Value == "" || a.Value == "Guide" || a.Value == "Live TV & DVR" {
				mc.Attrs[i].Value = label
				changed = true
			}
		case "friendlyName":
			mc.Attrs[i].Value = label
			changed = true
		}
	}
	if !changed {
		return xmlBytes
	}
	out, err := xml.Marshal(&mc)
	if err != nil {
		return xmlBytes
	}
	return append([]byte(xml.Header), out...)
}

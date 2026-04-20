package plexlabelproxy

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

// LabelSource resolves Live TV provider identifiers (e.g.
// "tv.plex.providers.epg.xmltv:135") to the human-friendly tab label that
// should be substituted in proxied responses.
type LabelSource interface {
	Get() map[string]string
}

// LabelMapCache fetches DVR lineup titles from PMS /livetv/dvrs and exposes
// them keyed by Live TV provider identifier. Refreshes lazily with a TTL to
// avoid hammering PMS.
type LabelMapCache struct {
	upstream    string
	token       string
	stripPrefix string
	ttl         time.Duration
	client      *http.Client

	mu          sync.Mutex
	cached      map[string]string
	lastRefresh time.Time

	// refreshMu serializes refresh attempts so concurrent Get() callers on a
	// stale/cold cache don't fan out N parallel /livetv/dvrs fetches.
	refreshMu sync.Mutex
}

// NewLabelMapCache constructs a cache backed by upstream PMS. ttl <= 0 falls
// back to 30s. client may be nil to use the default httpclient.
func NewLabelMapCache(upstream, token, stripPrefix string, ttl time.Duration, client *http.Client) *LabelMapCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if client == nil {
		client = httpclient.WithTimeout(15 * time.Second)
	}
	return &LabelMapCache{
		upstream:    strings.TrimRight(strings.TrimSpace(upstream), "/"),
		token:       strings.TrimSpace(token),
		stripPrefix: stripPrefix,
		ttl:         ttl,
		client:      client,
	}
}

// Get returns a copy of the current label map, refreshing first if the cached
// snapshot is empty or older than the TTL. On refresh error the previous
// snapshot is returned (or an empty map on cold start).
func (c *LabelMapCache) Get() map[string]string {
	c.mu.Lock()
	fresh := time.Since(c.lastRefresh) < c.ttl && c.cached != nil
	c.mu.Unlock()
	if !fresh {
		_ = c.Refresh()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.cached))
	for k, v := range c.cached {
		out[k] = v
	}
	return out
}

// Refresh fetches /livetv/dvrs and rebuilds the cache. Concurrent callers
// serialize on refreshMu so PMS sees one in-flight fetch at a time; callers
// that arrive after a fresh refresh return immediately without re-fetching.
func (c *LabelMapCache) Refresh() error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	c.mu.Lock()
	if c.cached != nil && time.Since(c.lastRefresh) < c.ttl {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	mapped, err := FetchDVRLabelMap(c.client, c.upstream, c.token, c.stripPrefix)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.cached = mapped
	c.lastRefresh = time.Now()
	c.mu.Unlock()
	return nil
}

// FetchDVRLabelMap performs one /livetv/dvrs GET against upstream and parses
// the response into a map of LiveTV provider identifier -> label.
//
// Label sourcing precedence per DVR:
//  1. lineupTitle attr
//  2. title attr
//  3. fragment of the lineup attr after the final '#'
//
// stripPrefix, when non-empty, is removed from the head of every label
// (e.g. "iptvtunerr-newsus" -> "newsus" with prefix "iptvtunerr-").
func FetchDVRLabelMap(client *http.Client, upstream, token, stripPrefix string) (map[string]string, error) {
	if client == nil {
		client = httpclient.WithTimeout(15 * time.Second)
	}
	upstream = strings.TrimRight(strings.TrimSpace(upstream), "/")
	if upstream == "" {
		return nil, fmt.Errorf("upstream is empty")
	}
	q := url.Values{}
	if t := strings.TrimSpace(token); t != "" {
		q.Set("X-Plex-Token", t)
	}
	reqURL := upstream + "/livetv/dvrs"
	if encoded := q.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/xml")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode != http.StatusOK {
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return nil, fmt.Errorf("/livetv/dvrs returned %d: %s", resp.StatusCode, preview)
	}
	return parseDVRLabelMap(body, stripPrefix)
}

// parseDVRLabelMap is the pure-data parsing half of FetchDVRLabelMap, separated
// for unit testing.
func parseDVRLabelMap(body []byte, stripPrefix string) (map[string]string, error) {
	type dvrAttrs struct {
		Key         string `xml:"key,attr"`
		ID          string `xml:"id,attr"`
		LineupTitle string `xml:"lineupTitle,attr"`
		Title       string `xml:"title,attr"`
		Lineup      string `xml:"lineup,attr"`
	}
	var mc struct {
		Dvr []dvrAttrs `xml:"Dvr"`
	}
	if err := xml.Unmarshal(body, &mc); err != nil {
		return nil, fmt.Errorf("parse /livetv/dvrs xml: %w", err)
	}
	out := make(map[string]string, len(mc.Dvr))
	for _, d := range mc.Dvr {
		id := strings.TrimSpace(d.Key)
		if id == "" {
			id = strings.TrimSpace(d.ID)
		}
		if id == "" {
			continue
		}
		label := pickLabel(d.LineupTitle, d.Title, d.Lineup)
		if label == "" {
			continue
		}
		if stripPrefix != "" && strings.HasPrefix(label, stripPrefix) {
			label = label[len(stripPrefix):]
		}
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		out["tv.plex.providers.epg.xmltv:"+id] = label
	}
	return out, nil
}

// pickLabel applies the (lineupTitle, title, lineup#fragment) precedence chain.
func pickLabel(lineupTitle, title, lineup string) string {
	if v := strings.TrimSpace(lineupTitle); v != "" {
		return v
	}
	if v := strings.TrimSpace(title); v != "" {
		return v
	}
	if i := strings.LastIndex(lineup, "#"); i >= 0 && i+1 < len(lineup) {
		// Fragment is URL-encoded in the lineup URI (we encode it that way
		// when registering); decode best-effort.
		frag := lineup[i+1:]
		if dec, err := url.QueryUnescape(frag); err == nil {
			return strings.TrimSpace(dec)
		}
		return strings.TrimSpace(frag)
	}
	return ""
}

// staticLabels is a LabelSource backed by a fixed map; used by tests and by
// callers wiring known label tables (e.g. from CLI flags).
type staticLabels map[string]string

func (s staticLabels) Get() map[string]string {
	out := make(map[string]string, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

// StaticLabelSource wraps a concrete map as a LabelSource.
func StaticLabelSource(m map[string]string) LabelSource { return staticLabels(m) }

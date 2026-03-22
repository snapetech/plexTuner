package hdhomerun

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

// GuideXMLStats summarizes a fetched XMLTV document from a physical HDHomeRun (or compatible).
type GuideXMLStats struct {
	Bytes       int
	ChannelTags int
	Programmes  int
}

// GuideURLFromBase returns the conventional guide.xml URL for a device base URL.
func GuideURLFromBase(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if b == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(b), "/guide.xml") {
		return b
	}
	return b + "/guide.xml"
}

// FetchGuideXML downloads /guide.xml from a SiliconDust-style device HTTP interface.
// Not all models or firmware builds expose this; callers should handle 404.
func FetchGuideXML(ctx context.Context, client *http.Client, baseURL string) ([]byte, error) {
	if client == nil {
		client = httpclient.WithTimeout(60 * time.Second)
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("hdhomerun: guide base url required")
	}
	u := GuideURLFromBase(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hdhomerun: guide.xml %s: %s", resp.Status, strings.TrimSpace(string(body[:min(256, len(body))])))
	}
	return body, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// AnalyzeGuideXMLStats counts XMLTV-style <channel> and <programme> start elements (namespace-local name).
func AnalyzeGuideXMLStats(raw []byte) (*GuideXMLStats, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var ch, pr int
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "channel":
			ch++
		case "programme":
			pr++
		}
	}
	return &GuideXMLStats{Bytes: len(raw), ChannelTags: ch, Programmes: pr}, nil
}

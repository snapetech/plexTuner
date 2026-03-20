package tuner

import (
	"fmt"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

// EnrichCatchupCapsulesRecordURLs fills RecordSourceURLs and PreferredStreamUA from the live channel map.
// channels is keyed by GuideNumber (XMLTV channel id) matching CatchupCapsule.ChannelID.
func EnrichCatchupCapsulesRecordURLs(preview *CatchupCapsulePreview, channels []catalog.LiveChannel, streamBaseURL string) {
	if preview == nil {
		return
	}
	by := make(map[string]catalog.LiveChannel, len(channels))
	for _, ch := range channels {
		k := strings.TrimSpace(ch.GuideNumber)
		if k == "" {
			continue
		}
		by[k] = ch
	}
	for i := range preview.Capsules {
		c := &preview.Capsules[i]
		ch := by[strings.TrimSpace(c.ChannelID)]
		c.RecordSourceURLs = BuildRecordURLsForCapsule(ch, *c, streamBaseURL)
		if ua := strings.TrimSpace(ch.PreferredUA); ua != "" {
			c.PreferredStreamUA = ua
		}
	}
}

// BuildRecordURLsForCapsule returns an ordered list: Tunerr relay URL when possible, then catalog StreamURL / StreamURLs (deduped).
// When ReplayURL is set on the capsule, only that URL is used.
func BuildRecordURLsForCapsule(ch catalog.LiveChannel, capsule CatchupCapsule, streamBaseURL string) []string {
	if replay := strings.TrimSpace(capsule.ReplayURL); replay != "" {
		return []string{replay}
	}
	streamBaseURL = strings.TrimRight(strings.TrimSpace(streamBaseURL), "/")
	var out []string
	if streamBaseURL != "" && strings.TrimSpace(capsule.ChannelID) != "" {
		out = append(out, streamBaseURL+"/stream/"+strings.TrimSpace(capsule.ChannelID))
	}
	if u := strings.TrimSpace(ch.StreamURL); u != "" {
		out = append(out, u)
	}
	for _, u := range ch.StreamURLs {
		if u = strings.TrimSpace(u); u != "" {
			out = append(out, u)
		}
	}
	return dedupeURLStrings(out)
}

func dedupeURLStrings(urls []string) []string {
	seen := make(map[string]struct{}, len(urls))
	var out []string
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

// ResolveCatchupRecordSourceURLList returns ordered capture URLs for resilient recording.
func ResolveCatchupRecordSourceURLList(capsule CatchupCapsule, streamBaseURL string) ([]string, error) {
	if replay := strings.TrimSpace(capsule.ReplayURL); replay != "" {
		return []string{replay}, nil
	}
	if len(capsule.RecordSourceURLs) > 0 {
		u := dedupeURLStrings(capsule.RecordSourceURLs)
		if len(u) == 0 {
			return nil, fmt.Errorf("empty record_source_urls for capsule %s", capsule.CapsuleID)
		}
		return u, nil
	}
	one, err := ResolveCatchupRecordSourceURL(capsule, streamBaseURL)
	if err != nil {
		return nil, err
	}
	return []string{one}, nil
}

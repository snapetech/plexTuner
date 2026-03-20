package tuner

import (
	"fmt"
	"net/url"
	"sort"
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

// ApplyRecordURLDeprioritizeHosts reorders each capsule's RecordSourceURLs so URLs whose host matches
// penalizedHosts (comma-separated hostnames, case-insensitive) move after non-matching URLs.
// The first URL (typically the Tunerr relay) is always kept first. Intended for IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS.
func ApplyRecordURLDeprioritizeHosts(preview *CatchupCapsulePreview, hostsCSV string) {
	if preview == nil {
		return
	}
	penalized := ParseHostPenaltySet(hostsCSV)
	if len(penalized) == 0 {
		return
	}
	for i := range preview.Capsules {
		preview.Capsules[i].RecordSourceURLs = DeprioritizeRecordSourceURLs(preview.Capsules[i].RecordSourceURLs, penalized)
	}
}

// ParseHostPenaltySet parses a comma-separated hostname list into a lookup set (lowercased).
func ParseHostPenaltySet(csv string) map[string]struct{} {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil
	}
	out := make(map[string]struct{})
	for _, p := range strings.Split(csv, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out[p] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// DeprioritizeRecordSourceURLs moves URLs whose host matches penalized to the end (stable). The first URL stays first.
func DeprioritizeRecordSourceURLs(urls []string, penalized map[string]struct{}) []string {
	if len(urls) <= 1 || len(penalized) == 0 {
		return urls
	}
	head := urls[0]
	tail := append([]string(nil), urls[1:]...)
	sort.SliceStable(tail, func(i, j int) bool {
		pi := urlHostPenalized(tail[i], penalized)
		pj := urlHostPenalized(tail[j], penalized)
		if pi == pj {
			return false
		}
		return !pi && pj
	})
	return append([]string{head}, tail...)
}

func urlHostPenalized(rawURL string, penalized map[string]struct{}) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if _, ok := penalized[h]; ok {
		return true
	}
	for ph := range penalized {
		if h == ph {
			return true
		}
		if strings.HasSuffix(h, "."+ph) {
			return true
		}
	}
	return false
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

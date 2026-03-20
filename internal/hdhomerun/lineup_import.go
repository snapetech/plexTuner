package hdhomerun

import (
	"fmt"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

// LiveChannelsFromLineupDoc maps hardware lineup.json entries to catalog channels (LP-002).
// GuideNumber is used as TVGID so device guide.xml <programme channel="…"> can match when identical.
func LiveChannelsFromLineupDoc(doc *LineupDoc, idPrefix string) []catalog.LiveChannel {
	if doc == nil {
		return nil
	}
	prefix := strings.TrimSpace(idPrefix)
	if prefix == "" {
		prefix = "hdhr"
	}
	out := make([]catalog.LiveChannel, 0, len(doc.Channels))
	seen := make(map[string]struct{}, len(doc.Channels))
	for _, it := range doc.Channels {
		gn := strings.TrimSpace(it.GuideNumber)
		url := strings.TrimSpace(it.URL)
		name := strings.TrimSpace(it.GuideName)
		if gn == "" || url == "" {
			continue
		}
		cid := fmt.Sprintf("%s:%s", prefix, gn)
		if _, ok := seen[cid]; ok {
			continue
		}
		seen[cid] = struct{}{}
		out = append(out, catalog.LiveChannel{
			ChannelID:   cid,
			GuideNumber: gn,
			GuideName:   name,
			StreamURL:   url,
			StreamURLs:  []string{url},
			EPGLinked:   true,
			TVGID:       gn,
			GroupTitle:  "HDHomeRun",
			SourceTag:   prefix,
		})
	}
	return out
}

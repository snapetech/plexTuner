package tuner

import (
	"strconv"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func (g *Gateway) lookupChannel(channelID string) *catalog.LiveChannel {
	for i := range g.Channels {
		if g.Channels[i].ChannelID == channelID {
			return &g.Channels[i]
		}
	}
	if idx, err := strconv.Atoi(channelID); err == nil && idx >= 0 && idx < len(g.Channels) {
		return &g.Channels[idx]
	}
	for i := range g.Channels {
		if g.Channels[i].GuideNumber == channelID {
			return &g.Channels[i]
		}
	}
	return nil
}

func streamURLsForChannel(channel *catalog.LiveChannel) []string {
	if channel == nil {
		return nil
	}
	if len(channel.StreamURLs) > 0 {
		return channel.StreamURLs
	}
	if channel.StreamURL != "" {
		return []string{channel.StreamURL}
	}
	return nil
}

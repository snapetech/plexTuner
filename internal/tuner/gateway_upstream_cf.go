package tuner

import (
	"context"
	"log"
	"net/http"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

// tryRecoverCFUpstream runs UA cycling and optional CF bootstrap after a CF-like non-OK
// upstream response (body already consumed and closed by caller).
// On success returns a new *http.Response with StatusOK and an open body for the stream path;
// caller must close it. Returns nil if recovery failed.
func (g *Gateway) tryRecoverCFUpstream(
	ctx context.Context,
	r *http.Request,
	streamURL string,
	client *http.Client,
	origStatus int,
	channel *catalog.LiveChannel,
	channelID string,
	urlIndex, urlTotal int,
) *http.Response {
	if cycled, ua := g.tryCFUACycle(ctx, r, streamURL, client, origStatus); cycled != nil {
		log.Printf("gateway: channel=%q id=%s upstream[%d/%d] CF-cycle succeeded ua=%q url=%s",
			channel.GuideName, channelID, urlIndex, urlTotal, ua, safeurl.RedactURL(streamURL))
		return cycled
	}
	if g.cfBoot != nil && !hasCFClearanceInJar(g.persistentCookieJar, streamURL) {
		workingUA := g.cfBoot.EnsureAccess(ctx, streamURL, client)
		if workingUA != "" {
			g.setLearnedUA(hostFromURL(streamURL), workingUA)
		}
		if retried, _ := g.tryCFUACycle(ctx, r, streamURL, client, origStatus); retried != nil {
			return retried
		}
	}
	return nil
}

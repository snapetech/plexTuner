package tuner

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

const muxSegMaxRedirects = 10

var errMuxRedirectPolicy = errors.New("mux seg redirect rejected")

// muxSegHTTPClient returns a client that validates every redirect hop with the same private/DNS policy
// as the initial seg= URL. Uses a cloned Transport from the parent (or default streaming transport).
func muxSegHTTPClient(parent *http.Client, baseCtx context.Context) *http.Client {
	if parent == nil {
		parent = httpclient.ForStreaming()
	}
	var t *http.Transport
	if tr, ok := parent.Transport.(*http.Transport); ok && tr != nil {
		t = tr.Clone()
	} else {
		if d, ok := httpclient.ForStreaming().Transport.(*http.Transport); ok && d != nil {
			t = d.Clone()
		} else {
			t = &http.Transport{}
		}
	}
	denyLit := hlsMuxDenyLiteralPrivateUpstream()
	denyRes := hlsMuxDenyResolvedPrivateUpstream()
	c := &http.Client{
		Transport: t,
		Timeout:   parent.Timeout,
		Jar:       parent.Jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= muxSegMaxRedirects {
				return fmt.Errorf("%w: too many redirects", errMuxRedirectPolicy)
			}
			if req.URL == nil {
				return fmt.Errorf("%w: missing redirect url", errMuxRedirectPolicy)
			}
			raw := req.URL.String()
			if err := safeurl.ValidateMuxSegTarget(baseCtx, raw, denyLit, denyRes); err != nil {
				return fmt.Errorf("%w: %v", errMuxRedirectPolicy, err)
			}
			return nil
		},
	}
	return c
}

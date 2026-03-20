package httpclient

import (
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/andybalholm/brotli"
)

func httpAcceptBrotliFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_ACCEPT_BROTLI")))
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

// TransportWithOptionalBrotli wraps base so responses with Content-Encoding: br are decompressed
// when IPTV_TUNERR_HTTP_ACCEPT_BROTLI is set at request time. Otherwise requests pass through unchanged.
func TransportWithOptionalBrotli(base *http.Transport) http.RoundTripper {
	if base == nil {
		return nil
	}
	return &brotliRoundTripper{base: base}
}

type brotliRoundTripper struct {
	base http.RoundTripper
}

func (b *brotliRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if b.base == nil {
		b.base = http.DefaultTransport
	}
	if !httpAcceptBrotliFromEnv() {
		return b.base.RoundTrip(req)
	}
	req2 := req.Clone(req.Context())
	ae := req2.Header.Get("Accept-Encoding")
	low := strings.ToLower(ae)
	if ae == "" {
		req2.Header.Set("Accept-Encoding", "gzip, deflate, br")
	} else if !strings.Contains(low, "br") {
		req2.Header.Set("Accept-Encoding", ae+", br")
	}
	resp, err := b.base.RoundTrip(req2)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	ce := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if ce != "br" {
		return resp, nil
	}
	resp.Header.Del("Content-Encoding")
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	br := brotli.NewReader(resp.Body)
	resp.Body = &brotliReadCloser{br: br, underlying: resp.Body}
	return resp, nil
}

type brotliReadCloser struct {
	br         io.Reader
	underlying io.ReadCloser
}

func (r *brotliReadCloser) Read(p []byte) (int, error) {
	return r.br.Read(p)
}

func (r *brotliReadCloser) Close() error {
	return r.underlying.Close()
}

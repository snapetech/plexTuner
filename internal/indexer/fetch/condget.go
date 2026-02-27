package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/plextuner/plex-tuner/internal/httpclient"
)

// ErrNotModified is returned by ConditionalGet when the server responds 304.
var ErrNotModified = errors.New("fetch: 304 not modified")

// GetResult carries the response body and the cache-validator headers from a
// successful (200) ConditionalGet call.
type GetResult struct {
	Body         []byte
	ETag         string
	LastModified string
	// ContentHash is sha256(Body[:16]) stored in state so we can detect
	// provider-side changes even when ETag/LM are absent.
	ContentHash string
}

// ConditionalGet issues a GET with If-None-Match / If-Modified-Since if prior
// etag / lastModified are non-empty. Returns ErrNotModified on 304. On 200,
// reads the full body and captures ETag/Last-Modified from the response.
//
// The caller is responsible for connecting this to a FetchState to persist the
// returned ETag and LastModified between runs.
func ConditionalGet(ctx context.Context, client *http.Client, url, etag, lastModified string) (*GetResult, error) {
	if client == nil {
		client = httpclient.Default()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("condget: build request: %w", err)
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := httpclient.DoWithRetry(ctx, client, req, httpclient.ProviderRetryPolicy)
	if err != nil {
		return nil, fmt.Errorf("condget %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, ErrNotModified
	}
	if resp.StatusCode != http.StatusOK {
		if ok, cfErr := IsCFResponse(resp); ok {
			resp.Body.Close()
			return nil, cfErr
		}
		resp.Body.Close()
		return nil, fmt.Errorf("condget %s: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("condget %s: read body: %w", url, err)
	}

	return &GetResult{
		Body:         body,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		ContentHash:  ContentHash(body),
	}, nil
}

// ConditionalGetStream is like ConditionalGet but returns a streaming reader
// instead of buffering the full body. Use for large M3U files where you want
// to stream-parse while downloading. ETag and LastModified are read from
// response headers immediately; the body must be read and closed by the caller.
//
// Returns (nil reader, nil error, ErrNotModified) on 304.
func ConditionalGetStream(ctx context.Context, client *http.Client, url, etag, lastModified string) (
	body io.ReadCloser, result *GetResult, err error) {
	if client == nil {
		client = httpclient.Default()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("condget-stream: build request: %w", err)
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := httpclient.DoWithRetry(ctx, client, req, httpclient.ProviderRetryPolicy)
	if err != nil {
		return nil, nil, fmt.Errorf("condget-stream %s: %w", url, err)
	}

	if resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return nil, nil, ErrNotModified
	}
	if resp.StatusCode != http.StatusOK {
		if ok, cfErr := IsCFResponse(resp); ok {
			resp.Body.Close()
			return nil, nil, cfErr
		}
		resp.Body.Close()
		return nil, nil, fmt.Errorf("condget-stream %s: unexpected status %d", url, resp.StatusCode)
	}

	meta := &GetResult{
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	// Wrap the body in a content-hashing tee so we can record the hash after
	// the caller drains it without buffering.
	hr := newHashReader(resp.Body)
	return &hashReadCloser{hr: hr, meta: meta}, meta, nil
}

// hashReadCloser wraps a hashReader and exposes Close().
// On Close it finalises the hash and writes it into meta.ContentHash.
type hashReadCloser struct {
	hr   *hashReader
	meta *GetResult
}

func (h *hashReadCloser) Read(p []byte) (int, error) { return h.hr.Read(p) }
func (h *hashReadCloser) Close() error {
	h.meta.ContentHash = h.hr.Hex()
	return nil
}

// hashReader tees reads through a running sha256 hash.
type hashReader struct {
	r   io.ReadCloser
	buf bytes.Buffer
}

func newHashReader(r io.ReadCloser) *hashReader { return &hashReader{r: r} }

func (h *hashReader) Read(p []byte) (int, error) {
	n, err := h.r.Read(p)
	if n > 0 {
		h.buf.Write(p[:n])
	}
	return n, err
}

func (h *hashReader) Hex() string { return ContentHash(h.buf.Bytes()) }

// RangeRequest fetches a byte range of a URL. Used to resume a partially
// downloaded large file. Returns (nil, ErrNotModified) on 304.
func RangeRequest(ctx context.Context, client *http.Client, url string, offset int64, etag string) (io.ReadCloser, error) {
	if client == nil {
		client = httpclient.Default()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	if etag != "" {
		req.Header.Set("If-Range", etag)
	}

	// Range requests can't use the standard retry policy as-is since the
	// offset must match; use a shorter timeout.
	shortClient := *client
	shortClient.Timeout = 90 * time.Second
	resp, err := shortClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return nil, ErrNotModified
	}
	// 206 Partial Content = server honoured the range.
	// 200 OK = server ignored Range header and sent full body; caller must handle.
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("range request %s offset %d: status %d", url, offset, resp.StatusCode)
	}
	return resp.Body, nil
}

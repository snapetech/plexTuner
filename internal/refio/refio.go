package refio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

const userAgent = "IptvTunerr/1.0"

type LocalFileRef string

type RemoteHTTPRef struct {
	raw string
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelReadCloser) Close() error {
	err := r.ReadCloser.Close()
	if r.cancel != nil {
		r.cancel()
	}
	return err
}

func PrepareLocalFileRef(raw string) (LocalFileRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty ref")
	}
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("invalid local ref")
	}
	path := filepath.Clean(raw)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("local ref is a directory")
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("local ref must be a regular file")
	}
	return LocalFileRef(path), nil
}

func OpenLocalFile(ref LocalFileRef) (io.ReadCloser, error) {
	path := strings.TrimSpace(string(ref))
	if path == "" {
		return nil, fmt.Errorf("empty ref")
	}
	return os.Open(path)
}

func ReadLocalFile(ref LocalFileRef) ([]byte, error) {
	r, err := OpenLocalFile(ref)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func PrepareRemoteHTTPRef(ctx context.Context, raw string) (RemoteHTTPRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RemoteHTTPRef{}, fmt.Errorf("empty ref")
	}
	if !safeurl.IsHTTPOrHTTPS(raw) {
		return RemoteHTTPRef{}, fmt.Errorf("unsupported ref scheme")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return RemoteHTTPRef{}, fmt.Errorf("invalid remote ref")
	}
	if strings.TrimSpace(u.Host) == "" {
		return RemoteHTTPRef{}, fmt.Errorf("remote ref missing host")
	}
	if !allowPrivateHTTPRefs() {
		if safeurl.HTTPURLHostIsLiteralBlockedPrivate(raw) {
			return RemoteHTTPRef{}, fmt.Errorf("remote ref uses blocked private host")
		}
		blocked, err := safeurl.HTTPURLHostResolvesToBlockedPrivate(ctx, raw)
		if err == nil && blocked {
			return RemoteHTTPRef{}, fmt.Errorf("remote ref resolves to blocked private host")
		}
	}
	return RemoteHTTPRef{raw: u.String()}, nil
}

func OpenRemoteHTTP(ctx context.Context, ref RemoteHTTPRef, timeout time.Duration) (io.ReadCloser, error) {
	raw := strings.TrimSpace(ref.raw)
	if raw == "" {
		return nil, fmt.Errorf("empty ref")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	client := httpclient.Default()
	if timeout > 0 {
		client = httpclient.WithTimeout(timeout)
	}
	resp, err := client.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	if cancel != nil {
		return &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}, nil
	}
	return resp.Body, nil
}

func ReadRemoteHTTP(ctx context.Context, ref RemoteHTTPRef, timeout time.Duration) ([]byte, error) {
	r, err := OpenRemoteHTTP(ctx, ref, timeout)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func allowPrivateHTTPRefs() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

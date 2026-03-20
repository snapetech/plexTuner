package refio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

const userAgent = "IptvTunerr/1.0"

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

func Open(ref string, timeout time.Duration) (io.ReadCloser, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty ref")
	}
	if !safeurl.IsHTTPOrHTTPS(ref) {
		path, err := sanitizeLocalRef(ref)
		if err != nil {
			return nil, err
		}
		return os.Open(path)
	}
	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}
	if err := validateRemoteRef(ctx, ref); err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
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

func ReadAll(ref string, timeout time.Duration) ([]byte, error) {
	r, err := Open(ref, timeout)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func sanitizeLocalRef(ref string) (string, error) {
	if strings.ContainsRune(ref, 0) {
		return "", fmt.Errorf("invalid local ref")
	}
	path := filepath.Clean(ref)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("local ref is a directory")
	}
	if info.Mode()&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeIrregular|os.ModeCharDevice) != 0 {
		return "", fmt.Errorf("local ref must be a regular file")
	}
	return path, nil
}

func validateRemoteRef(ctx context.Context, ref string) error {
	if !safeurl.IsHTTPOrHTTPS(ref) {
		return fmt.Errorf("unsupported ref scheme")
	}
	if !allowPrivateHTTPRefs() {
		if safeurl.HTTPURLHostIsLiteralBlockedPrivate(ref) {
			return fmt.Errorf("remote ref uses blocked private host")
		}
		blocked, err := safeurl.HTTPURLHostResolvesToBlockedPrivate(ctx, ref)
		if err == nil && blocked {
			return fmt.Errorf("remote ref resolves to blocked private host")
		}
	}
	return nil
}

func allowPrivateHTTPRefs() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

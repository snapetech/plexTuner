package refio

import (
	"context"
	"fmt"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type LocalFileRef string

type RemoteHTTPRef struct {
	raw string
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

func (r RemoteHTTPRef) URL() string {
	return strings.TrimSpace(r.raw)
}

func (r LocalFileRef) Path() string {
	return strings.TrimSpace(string(r))
}

func allowPrivateHTTPRefs() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

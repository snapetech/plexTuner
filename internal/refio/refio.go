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
	path, err := resolveGuideInputPath(raw)
	if err != nil {
		return "", err
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

func (r RemoteHTTPRef) Hostname() string {
	u, err := url.Parse(strings.TrimSpace(r.raw))
	if err != nil || u == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Hostname()))
}

func resolveGuideInputPath(raw string) (string, error) {
	absPath, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", err
	}
	allowedRoots, err := guideInputRoots()
	if err != nil {
		return "", err
	}
	for _, root := range allowedRoots {
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return absPath, nil
		}
	}
	return "", fmt.Errorf("local ref outside allowed guide roots")
}

func guideInputRoots() ([]string, error) {
	roots := []string{}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	roots = append(roots, cwd)
	for _, part := range strings.Split(os.Getenv("IPTV_TUNERR_GUIDE_INPUT_ROOTS"), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		abs, err := filepath.Abs(part)
		if err != nil {
			continue
		}
		roots = append(roots, abs)
	}
	return roots, nil
}

func allowPrivateHTTPRefs() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CheckProvider fetches the M3U URL (HEAD or GET). Returns nil if OK, error with message if not.
func CheckProvider(ctx context.Context, m3uURL string) error {
	if m3uURL == "" {
		return fmt.Errorf("no M3U URL configured")
	}
	// Some providers don't support HEAD; use GET and close body immediately.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m3uURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("provider unreachable: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// CheckEndpoints hits discover, lineup, guide at baseURL and returns the first error or nil.
func CheckEndpoints(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	for _, path := range []string{"/discover.json", "/lineup.json", "/guide.xml"} {
		url := baseURL + path
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
		}
	}
	return nil
}

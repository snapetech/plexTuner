package refio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

const userAgent = "IptvTunerr/1.0"

func Open(ref string, timeout time.Duration) (io.ReadCloser, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty ref")
	}
	if !strings.HasPrefix(ref, "http://") && !strings.HasPrefix(ref, "https://") {
		return os.Open(ref)
	}
	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	client := httpclient.Default()
	if timeout > 0 {
		client = httpclient.WithTimeout(timeout)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("http %d", resp.StatusCode)
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

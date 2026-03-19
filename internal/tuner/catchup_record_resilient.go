package tuner

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
)

// ResilientRecordOptions configures multi-attempt capture with optional HTTP Range resume.
type ResilientRecordOptions struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	ResumePartial  bool // append using Range after transient mid-stream failures when possible
}

// RecordCaptureMetrics summarizes capture attempts for observability.
type RecordCaptureMetrics struct {
	HTTPAttempts     int   `json:"http_attempts"`
	TransientRetries int   `json:"transient_retries"`
	BytesResumed     int64 `json:"bytes_resumed"`
}

// RecordCatchupCapsuleResilient captures a capsule with transient retries and optional Range resume on the same spool file.
func RecordCatchupCapsuleResilient(ctx context.Context, capsule CatchupCapsule, streamBaseURL, outDir string, client *http.Client, opts ResilientRecordOptions) (CatchupRecordedItem, RecordCaptureMetrics, error) {
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		return CatchupRecordedItem{}, RecordCaptureMetrics{}, fmt.Errorf("output directory required")
	}
	if client == nil {
		client = httpclient.ForStreaming()
	}
	opts = normalizeResilientRecordOptions(opts)
	sourceURL, err := ResolveCatchupRecordSourceURL(capsule, streamBaseURL)
	if err != nil {
		return CatchupRecordedItem{}, RecordCaptureMetrics{}, err
	}
	laneDir := filepath.Join(outDir, firstNonEmptyString(capsule.Lane, "general"))
	if err := os.MkdirAll(laneDir, 0o755); err != nil {
		return CatchupRecordedItem{}, RecordCaptureMetrics{}, err
	}
	spoolPath, finalPath := CatchupRecordArtifactPaths(capsule, outDir)

	var metrics RecordCaptureMetrics
	var errs []string
	var lastErr error

	for attempt := 0; attempt < opts.MaxAttempts; attempt++ {
		if attempt > 0 {
			d := BackoffAfterRecordError(lastErr, attempt-1, opts.InitialBackoff, opts.MaxBackoff)
			t := time.NewTimer(d)
			select {
			case <-t.C:
			case <-ctx.Done():
				if !t.Stop() {
					<-t.C
				}
				return CatchupRecordedItem{}, metrics, ctx.Err()
			}
		}

		if attempt == 0 {
			_ = os.Remove(spoolPath)
		}

		offset := int64(0)
		if attempt > 0 && opts.ResumePartial {
			if st, err := os.Stat(spoolPath); err == nil && st.Size() > 0 {
				offset = st.Size()
			}
		} else if attempt > 0 && !opts.ResumePartial {
			_ = os.Remove(spoolPath)
		}

		metrics.HTTPAttempts++
		resumed, rAfter, err := spoolCopyFromHTTP(ctx, client, sourceURL, capsule.CapsuleID, spoolPath, offset)
		_ = rAfter
		if err == nil {
			metrics.BytesResumed += resumed
			metrics.TransientRetries = attempt
			if err := ctx.Err(); err != nil {
				return CatchupRecordedItem{}, metrics, err
			}
			_ = os.Remove(finalPath)
			if err := os.Rename(spoolPath, finalPath); err != nil {
				return CatchupRecordedItem{}, metrics, err
			}
			var total int64
			if st, err := os.Stat(finalPath); err == nil {
				total = st.Size()
			}
			return CatchupRecordedItem{
				CapsuleID:  capsule.CapsuleID,
				Lane:       capsule.Lane,
				Title:      capsule.Title,
				ChannelID:  capsule.ChannelID,
				OutputPath: finalPath,
				SourceURL:  sourceURL,
				Bytes:      total,
			}, metrics, nil
		}
		lastErr = err
		errs = append(errs, fmt.Sprintf("attempt %d: %v", attempt+1, err))
		if attempt+1 >= opts.MaxAttempts || !IsTransientRecordError(err) {
			return CatchupRecordedItem{}, metrics, fmt.Errorf("%s", strings.Join(errs, " | "))
		}
		if !opts.ResumePartial {
			_ = os.Remove(spoolPath)
		}
	}
	return CatchupRecordedItem{}, metrics, fmt.Errorf("%s", strings.Join(errs, " | "))
}

func normalizeResilientRecordOptions(o ResilientRecordOptions) ResilientRecordOptions {
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 1
	}
	if o.InitialBackoff <= 0 {
		o.InitialBackoff = 5 * time.Second
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = 2 * time.Minute
	}
	if o.MaxBackoff < o.InitialBackoff {
		o.MaxBackoff = o.InitialBackoff
	}
	return o
}

// spoolCopyFromHTTP performs one GET (optionally with Range) into spoolPath. resumedBytes counts bytes appended from a 206 response.
func spoolCopyFromHTTP(ctx context.Context, client *http.Client, url string, capsuleID string, spoolPath string, offset int64) (resumedBytes int64, retryAfter string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	retryAfter = resp.Header.Get("Retry-After")
	switch resp.StatusCode {
	case http.StatusOK:
		if offset > 0 {
			// Server ignored Range; replace spool with this full body.
			f, err := os.Create(spoolPath)
			if err != nil {
				return 0, retryAfter, err
			}
			_, copyErr := io.Copy(f, resp.Body)
			if closeErr := f.Close(); closeErr != nil && copyErr == nil {
				copyErr = closeErr
			}
			if copyErr != nil {
				return 0, retryAfter, copyErr
			}
			if err := ctx.Err(); err != nil {
				return 0, retryAfter, err
			}
			return 0, retryAfter, nil
		}
		f, err := os.Create(spoolPath)
		if err != nil {
			return 0, retryAfter, err
		}
		_, copyErr := io.Copy(f, resp.Body)
		if closeErr := f.Close(); closeErr != nil && copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return 0, retryAfter, copyErr
		}
		if err := ctx.Err(); err != nil {
			return 0, retryAfter, err
		}
		return 0, retryAfter, nil

	case http.StatusPartialContent:
		f, err := os.OpenFile(spoolPath, os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			return 0, retryAfter, err
		}
		st, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return 0, retryAfter, err
		}
		if st.Size() != offset {
			_ = f.Close()
			return 0, retryAfter, fmt.Errorf("spool size mismatch: have %d want %d", st.Size(), offset)
		}
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			_ = f.Close()
			return 0, retryAfter, err
		}
		n, copyErr := io.Copy(f, resp.Body)
		if closeErr := f.Close(); closeErr != nil && copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return 0, retryAfter, copyErr
		}
		if err := ctx.Err(); err != nil {
			return 0, retryAfter, err
		}
		return n, retryAfter, nil

	default:
		return 0, retryAfter, newRecordHTTPStatusError(capsuleID, resp)
	}
}

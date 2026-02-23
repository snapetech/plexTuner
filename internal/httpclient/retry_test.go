package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	max := 60 * time.Second
	tests := []struct {
		name string
		s    string
		max  time.Duration
		want time.Duration
	}{
		{"empty", "", max, 1 * time.Second},
		{"seconds 5", "5", max, 5 * time.Second},
		{"seconds 0", "0", max, 0},
		{"seconds over cap", "120", max, max},
		{"whitespace", "  10  ", max, 10 * time.Second},
		{"invalid fallback", "x", max, 1 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRetryAfter(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("parseRetryAfter(%q, %v) = %v, want %v", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestDoWithRetry_429Then200(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	policy := DefaultRetryPolicy
	policy.Max429Wait = time.Second
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := DoWithRetry(ctx, client, req, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestDoWithRetry_4xxNoRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := DoWithRetry(ctx, nil, req, DefaultRetryPolicy)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

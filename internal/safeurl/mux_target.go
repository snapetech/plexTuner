package safeurl

import (
	"context"
	"fmt"
	"time"
)

// ValidateMuxSegTarget applies the same logical checks used for initial seg= URLs: scheme, optional
// literal-private block, optional DNS-resolved private block. DNS errors fail open (same as gateway).
func ValidateMuxSegTarget(ctx context.Context, rawURL string, denyLiteralPrivate, denyResolvedPrivate bool) error {
	if !IsHTTPOrHTTPS(rawURL) {
		return fmt.Errorf("mux target: unsupported URL scheme")
	}
	if denyLiteralPrivate && HTTPURLHostIsLiteralBlockedPrivate(rawURL) {
		return fmt.Errorf("mux target: blocked literal private host")
	}
	if denyResolvedPrivate {
		cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		blocked, err := HTTPURLHostResolvesToBlockedPrivate(cctx, rawURL)
		if err != nil {
			return nil // fail-open on DNS errors (match gateway seg= path)
		}
		if blocked {
			return fmt.Errorf("mux target: blocked resolved private host")
		}
	}
	return nil
}

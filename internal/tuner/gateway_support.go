package tuner

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func gatewayReqIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(gatewayReqIDKey{}).(string); ok {
		return v
	}
	return ""
}

func gatewayReqIDField(ctx context.Context) string {
	if id := gatewayReqIDFromContext(ctx); id != "" {
		return " req=" + id
	}
	return ""
}

func getenvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getenvFloat(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func getenvBool(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func mpegTSFlagsWithOptionalInitialDiscontinuity() string {
	flags := []string{"resend_headers", "pat_pmt_at_frames"}
	if getenvBool("IPTV_TUNERR_MPEGTS_INITIAL_DISCONTINUITY", true) {
		flags = append(flags, "initial_discontinuity")
	}
	return "+" + strings.Join(flags, "+")
}

func isClientDisconnectWriteError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "use of closed network connection")
}

func channelIDFromRequestPath(path string) (string, bool) {
	if strings.HasPrefix(path, "/stream/") {
		return strings.TrimPrefix(path, "/stream/"), true
	}
	if strings.HasPrefix(path, "/auto/") {
		rest := strings.TrimPrefix(path, "/auto/")
		if strings.HasPrefix(rest, "v") {
			rest = strings.TrimPrefix(rest, "v")
		}
		return rest, true
	}
	return "", false
}

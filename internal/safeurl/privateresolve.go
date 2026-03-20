package safeurl

import (
	"context"
	"net"
	"net/url"
	"strings"
)

func ipIsBlockedPrivate(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// HTTPURLHostResolvesToBlockedPrivate resolves the URL host (A/AAAA). If the host is a literal IP,
// it uses the same rules as HTTPURLHostIsLiteralBlockedPrivate. For hostnames, if any resolved address
// is loopback, private, link-local, or unspecified, it returns true. Returns false on DNS failure
// (fail-open for availability; callers may choose to treat errors strictly).
func HTTPURLHostResolvesToBlockedPrivate(ctx context.Context, raw string) (blocked bool, err error) {
	u, perr := url.Parse(raw)
	if perr != nil || u == nil {
		return false, perr
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return false, nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return ipIsBlockedPrivate(ip), nil
	}
	r := net.DefaultResolver
	addrs, err := r.LookupIPAddr(ctx, host)
	if err != nil {
		return false, err
	}
	for _, a := range addrs {
		if ipIsBlockedPrivate(a.IP) {
			return true, nil
		}
	}
	return false, nil
}

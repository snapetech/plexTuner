package tuner

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

type providerAccountLease struct {
	Key   string `json:"-"`
	Label string `json:"label"`
	Host  string `json:"host,omitempty"`
	InUse int    `json:"in_use"`
}

func configuredProviderAccountLimit() int {
	if _, ok := os.LookupEnv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT"); !ok {
		return 0
	}
	n := getenvInt("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", 0)
	if n < 0 {
		n = 0
	}
	return n
}

func providerAccountIdentityForURL(g *Gateway, ch *catalog.LiveChannel, rawURL string) (providerAccountLease, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return providerAccountLease{}, false
	}
	host := upstreamURLAuthority(rawURL)
	user := strings.TrimSpace(g.ProviderUser)
	pass := strings.TrimSpace(g.ProviderPass)
	prefix := ""
	if rule, ok := streamAuthForURL(ch, rawURL); ok {
		user = strings.TrimSpace(rule.User)
		pass = strings.TrimSpace(rule.Pass)
		prefix = strings.TrimSpace(rule.Prefix)
		if prefixHost := upstreamURLAuthority(prefix); prefixHost != "" {
			host = prefixHost
		}
	}
	if host == "" {
		if parsed, err := url.Parse(rawURL); err == nil && parsed != nil {
			host = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		}
	}
	labelUser := "anonymous"
	if user != "" {
		labelUser = maskProviderAccountUser(user)
	}
	key := fmt.Sprintf("%s|%s|%s|%s", strings.ToLower(host), user, pass, prefix)
	return providerAccountLease{
		Key:   key,
		Label: strings.Trim(host+"/"+labelUser, "/"),
		Host:  strings.ToLower(strings.TrimSpace(host)),
	}, true
}

func maskProviderAccountUser(user string) string {
	user = strings.TrimSpace(user)
	if user == "" {
		return "anonymous"
	}
	if len(user) <= 4 {
		return strings.Repeat("*", len(user))
	}
	return user[:4] + strings.Repeat("*", len(user)-4)
}

func (g *Gateway) effectiveProviderAccountLimit(ch *catalog.LiveChannel) int {
	if limit := configuredProviderAccountLimit(); limit > 0 {
		return limit
	}
	if ch == nil {
		return 0
	}
	keys := map[string]struct{}{}
	for _, rawURL := range streamURLsForChannel(ch) {
		identity, ok := providerAccountIdentityForURL(g, ch, rawURL)
		if !ok || identity.Key == "" {
			continue
		}
		keys[identity.Key] = struct{}{}
	}
	if len(keys) > 1 {
		return 1
	}
	return 0
}

func (g *Gateway) providerAccountLeaseCount(key string) int {
	if g == nil || strings.TrimSpace(key) == "" {
		return 0
	}
	g.accountLeaseMu.Lock()
	defer g.accountLeaseMu.Unlock()
	return g.accountLeases[key]
}

func (g *Gateway) tryAcquireProviderAccountLease(ch *catalog.LiveChannel, rawURL string) (providerAccountLease, bool, bool) {
	identity, ok := providerAccountIdentityForURL(g, ch, rawURL)
	if !ok || identity.Key == "" {
		return providerAccountLease{}, false, false
	}
	limit := g.effectiveProviderAccountLimit(ch)
	if limit <= 0 {
		return identity, false, true
	}
	g.accountLeaseMu.Lock()
	defer g.accountLeaseMu.Unlock()
	if g.accountLeases == nil {
		g.accountLeases = map[string]int{}
	}
	inUse := g.accountLeases[identity.Key]
	identity.InUse = inUse
	if inUse >= limit {
		return identity, true, false
	}
	g.accountLeases[identity.Key] = inUse + 1
	identity.InUse = inUse + 1
	return identity, true, true
}

func (g *Gateway) releaseProviderAccountLease(key string) {
	if g == nil || strings.TrimSpace(key) == "" {
		return
	}
	g.accountLeaseMu.Lock()
	defer g.accountLeaseMu.Unlock()
	if g.accountLeases == nil {
		return
	}
	if n := g.accountLeases[key]; n > 1 {
		g.accountLeases[key] = n - 1
	} else {
		delete(g.accountLeases, key)
	}
}

func (g *Gateway) reorderStreamURLsByAccountLoad(ch *catalog.LiveChannel, urls []string) []string {
	if len(urls) < 2 {
		return urls
	}
	limit := g.effectiveProviderAccountLimit(ch)
	out := append([]string(nil), urls...)
	sort.SliceStable(out, func(i, j int) bool {
		leftID, leftOK := providerAccountIdentityForURL(g, ch, out[i])
		rightID, rightOK := providerAccountIdentityForURL(g, ch, out[j])
		leftLoad, rightLoad := 0, 0
		leftSat, rightSat := false, false
		if leftOK {
			leftLoad = g.providerAccountLeaseCount(leftID.Key)
			leftSat = limit > 0 && leftLoad >= limit
		}
		if rightOK {
			rightLoad = g.providerAccountLeaseCount(rightID.Key)
			rightSat = limit > 0 && rightLoad >= limit
		}
		if leftSat != rightSat {
			return !leftSat
		}
		if leftLoad != rightLoad {
			return leftLoad < rightLoad
		}
		return i < j
	})
	return out
}

func (g *Gateway) providerAccountPoolExhausted(ch *catalog.LiveChannel, urls []string) bool {
	limit := g.effectiveProviderAccountLimit(ch)
	if limit <= 0 || len(urls) == 0 {
		return false
	}
	seen := map[string]struct{}{}
	saturated := 0
	for _, rawURL := range urls {
		identity, ok := providerAccountIdentityForURL(g, ch, rawURL)
		if !ok || identity.Key == "" {
			continue
		}
		if _, ok := seen[identity.Key]; ok {
			continue
		}
		seen[identity.Key] = struct{}{}
		if g.providerAccountLeaseCount(identity.Key) >= limit {
			saturated++
		}
	}
	return len(seen) > 0 && saturated == len(seen)
}

func (g *Gateway) providerAccountLeases() []providerAccountLease {
	if g == nil {
		return nil
	}
	g.accountLeaseMu.Lock()
	defer g.accountLeaseMu.Unlock()
	if len(g.accountLeases) == 0 {
		return nil
	}
	out := make([]providerAccountLease, 0, len(g.accountLeases))
	for key, inUse := range g.accountLeases {
		host, label := parseProviderAccountKey(key)
		out = append(out, providerAccountLease{
			Key:   key,
			Label: label,
			Host:  host,
			InUse: inUse,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].InUse == out[j].InUse {
			return out[i].Label < out[j].Label
		}
		return out[i].InUse > out[j].InUse
	})
	return out
}

func parseProviderAccountKey(key string) (host, label string) {
	parts := strings.SplitN(key, "|", 4)
	if len(parts) >= 2 {
		host = strings.TrimSpace(parts[0])
		user := strings.TrimSpace(parts[1])
		if user == "" {
			user = "anonymous"
		} else {
			user = maskProviderAccountUser(user)
		}
		label = strings.Trim(host+"/"+user, "/")
	}
	if label == "" {
		label = "provider-account"
	}
	return host, label
}

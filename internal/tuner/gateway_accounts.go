package tuner

import (
	"fmt"
	"net/url"
	"os"
	"path"
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
	} else if pathUser, pathPass, pathPrefix, ok := xtreamPathCredentials(rawURL); ok {
		user = pathUser
		pass = pathPass
		prefix = pathPrefix
		if prefixHost := upstreamURLAuthority(pathPrefix); prefixHost != "" {
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

func xtreamPathCredentials(rawURL string) (user, pass, prefix string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u == nil {
		return "", "", "", false
	}
	clean := path.Clean("/" + strings.TrimSpace(u.Path))
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	if len(parts) < 3 {
		return "", "", "", false
	}
	switch strings.ToLower(parts[0]) {
	case "live", "movie", "series", "timeshift":
	default:
		return "", "", "", false
	}
	user = strings.TrimSpace(parts[1])
	pass = strings.TrimSpace(parts[2])
	if user == "" || pass == "" {
		return "", "", "", false
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = "/" + strings.Join(parts[:3], "/") + "/"
	return user, pass, u.String(), true
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
	return g.effectiveProviderAccountLimitForKey(ch, "")
}

func (g *Gateway) defaultProviderAccountLimit(ch *catalog.LiveChannel) int {
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

func (g *Gateway) learnedProviderAccountLimit(key string) int {
	if g == nil || strings.TrimSpace(key) == "" {
		return 0
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	if g.learnedAccountLimits == nil {
		return 0
	}
	return g.learnedAccountLimits[key]
}

func (g *Gateway) effectiveProviderAccountLimitForKey(ch *catalog.LiveChannel, key string) int {
	configured := configuredProviderAccountLimit()
	learned := g.learnedProviderAccountLimit(key)
	switch {
	case configured > 0 && learned > 0 && learned < configured:
		return learned
	case configured > 0:
		return configured
	case learned > 0:
		return learned
	default:
		return g.defaultProviderAccountLimit(ch)
	}
}

func (g *Gateway) learnProviderAccountLimit(ch *catalog.LiveChannel, rawURL, preview string) int {
	if g == nil {
		return 0
	}
	identity, ok := providerAccountIdentityForURL(g, ch, rawURL)
	if !ok || identity.Key == "" {
		return 0
	}
	learned := parseUpstreamConcurrencyLimit(preview)
	if learned <= 0 {
		learned = 1
	}
	if learned < 1 {
		return 0
	}
	configured := configuredProviderAccountLimit()
	if configured > 0 && learned > configured {
		learned = configured
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	if g.learnedAccountLimits == nil {
		g.learnedAccountLimits = map[string]int{}
	}
	if g.accountConcurrencySignals == nil {
		g.accountConcurrencySignals = map[string]int{}
	}
	g.accountConcurrencySignals[identity.Key]++
	if cur := g.learnedAccountLimits[identity.Key]; cur > 0 && cur <= learned {
		return 0
	}
	g.learnedAccountLimits[identity.Key] = learned
	store := g.accountLimitStore
	signals := g.accountConcurrencySignals[identity.Key]
	if store != nil {
		go store.set(identity.Key, learned, signals)
	}
	return learned
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
	limit := g.effectiveProviderAccountLimitForKey(ch, identity.Key)
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
	out := append([]string(nil), urls...)
	sort.SliceStable(out, func(i, j int) bool {
		leftID, leftOK := providerAccountIdentityForURL(g, ch, out[i])
		rightID, rightOK := providerAccountIdentityForURL(g, ch, out[j])
		leftLoad, rightLoad := 0, 0
		leftSat, rightSat := false, false
		if leftOK {
			leftLoad = g.providerAccountLeaseCount(leftID.Key)
			leftLimit := g.effectiveProviderAccountLimitForKey(ch, leftID.Key)
			leftSat = leftLimit > 0 && leftLoad >= leftLimit
		}
		if rightOK {
			rightLoad = g.providerAccountLeaseCount(rightID.Key)
			rightLimit := g.effectiveProviderAccountLimitForKey(ch, rightID.Key)
			rightSat = rightLimit > 0 && rightLoad >= rightLimit
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
	if len(urls) == 0 {
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
		limit := g.effectiveProviderAccountLimitForKey(ch, identity.Key)
		if limit > 0 && g.providerAccountLeaseCount(identity.Key) >= limit {
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

type providerAccountLimitState struct {
	Label        string `json:"label"`
	Host         string `json:"host,omitempty"`
	LearnedLimit int    `json:"learned_limit"`
	SignalCount  int    `json:"signal_count,omitempty"`
	InUse        int    `json:"in_use,omitempty"`
}

func (g *Gateway) providerAccountLearnedLimits() []providerAccountLimitState {
	if g == nil {
		return nil
	}
	g.providerStateMu.Lock()
	limits := make(map[string]int, len(g.learnedAccountLimits))
	for k, v := range g.learnedAccountLimits {
		limits[k] = v
	}
	signals := make(map[string]int, len(g.accountConcurrencySignals))
	for k, v := range g.accountConcurrencySignals {
		signals[k] = v
	}
	g.providerStateMu.Unlock()
	if len(limits) == 0 {
		return nil
	}
	out := make([]providerAccountLimitState, 0, len(limits))
	for key, learned := range limits {
		host, label := parseProviderAccountKey(key)
		out = append(out, providerAccountLimitState{
			Label:        label,
			Host:         host,
			LearnedLimit: learned,
			SignalCount:  signals[key],
			InUse:        g.providerAccountLeaseCount(key),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LearnedLimit == out[j].LearnedLimit {
			return out[i].Label < out[j].Label
		}
		return out[i].LearnedLimit < out[j].LearnedLimit
	})
	return out
}

func (g *Gateway) restoreProviderAccountLearnedLimits(snapshot map[string]providerAccountLimitPersisted) {
	if g == nil || len(snapshot) == 0 {
		return
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	if g.learnedAccountLimits == nil {
		g.learnedAccountLimits = map[string]int{}
	}
	if g.accountConcurrencySignals == nil {
		g.accountConcurrencySignals = map[string]int{}
	}
	for key, entry := range snapshot {
		if strings.TrimSpace(key) == "" || entry.LearnedLimit <= 0 {
			continue
		}
		g.learnedAccountLimits[key] = entry.LearnedLimit
		if entry.SignalCount > 0 {
			g.accountConcurrencySignals[key] = entry.SignalCount
		}
	}
}

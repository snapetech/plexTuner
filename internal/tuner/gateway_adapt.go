package tuner

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

type plexForwardedHints struct {
	SessionIdentifier string
	ClientIdentifier  string
	Product           string
	Platform          string
	Device            string
	Raw               map[string]string
}

type plexResolvedClient struct {
	SessionIdentifier string
	ClientIdentifier  string
	Product           string
	Platform          string
	Title             string
}

func (h plexForwardedHints) empty() bool {
	return h.SessionIdentifier == "" && h.ClientIdentifier == "" && h.Product == "" && h.Platform == "" && h.Device == ""
}

func (h plexForwardedHints) summary() string {
	parts := []string{}
	if h.SessionIdentifier != "" {
		parts = append(parts, "sid=present")
	}
	if h.ClientIdentifier != "" {
		parts = append(parts, "cid=present")
	}
	if h.Product != "" {
		parts = append(parts, "product=present")
	}
	if h.Platform != "" {
		parts = append(parts, "platform=present")
	}
	if h.Device != "" {
		parts = append(parts, "device=present")
	}
	if len(h.Raw) > 0 {
		parts = append(parts, `raw=`+strconv.Itoa(len(h.Raw)))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

func plexRequestHints(r *http.Request) plexForwardedHints {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(r.Header.Get(k)); v != "" {
				return v
			}
			if v := strings.TrimSpace(r.URL.Query().Get(k)); v != "" {
				return v
			}
			lk := strings.ToLower(k)
			if v := strings.TrimSpace(r.Header.Get(lk)); v != "" {
				return v
			}
			if v := strings.TrimSpace(r.URL.Query().Get(lk)); v != "" {
				return v
			}
		}
		return ""
	}
	raw := map[string]string{}
	for k, vals := range r.Header {
		kl := strings.ToLower(k)
		if !strings.HasPrefix(kl, "x-plex-") {
			continue
		}
		if len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
			raw[k] = strings.TrimSpace(vals[0])
		}
	}
	for k, vals := range r.URL.Query() {
		kl := strings.ToLower(k)
		if !strings.Contains(kl, "plex") && !strings.Contains(kl, "session") && !strings.Contains(kl, "client") {
			continue
		}
		if len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
			raw["q:"+k] = strings.TrimSpace(vals[0])
		}
	}
	return plexForwardedHints{
		SessionIdentifier: get("X-Plex-Session-Identifier", "session", "sessionId", "session_id"),
		ClientIdentifier:  get("X-Plex-Client-Identifier", "X-Plex-Target-Client-Identifier", "clientIdentifier", "client_id"),
		Product:           get("X-Plex-Product"),
		Platform:          get("X-Plex-Platform", "X-Plex-Client-Platform"),
		Device:            get("X-Plex-Device", "X-Plex-Device-Name"),
		Raw:               raw,
	}
}

func xmlStartAttr(start xml.StartElement, name string) string {
	for _, a := range start.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (g *Gateway) resolvePlexClient(ctx context.Context, hints plexForwardedHints) (*plexResolvedClient, error) {
	if g == nil || !g.PlexClientAdapt {
		return nil, nil
	}
	if strings.TrimSpace(g.PlexPMSURL) == "" || strings.TrimSpace(g.PlexPMSToken) == "" {
		return nil, nil
	}
	if hints.SessionIdentifier == "" && hints.ClientIdentifier == "" {
		return nil, nil
	}
	base := strings.TrimRight(strings.TrimSpace(g.PlexPMSURL), "/")
	u := base + "/status/sessions?X-Plex-Token=" + url.QueryEscape(strings.TrimSpace(g.PlexPMSToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "IptvTunerr/1.0")
	client := g.Client
	if client == nil {
		client = httpclient.ForStreaming()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("pms /status/sessions status=" + strconv.Itoa(resp.StatusCode))
	}
	dec := xml.NewDecoder(resp.Body)
	type candidate struct {
		title    string
		player   plexResolvedClient
		session  string
		clientID string
	}
	var stack []string
	var cur *candidate
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			switch t.Name.Local {
			case "Video", "Track", "Photo", "Metadata":
				if cur == nil {
					cur = &candidate{title: xmlStartAttr(t, "title")}
				}
			case "Player":
				if cur != nil {
					cur.player.ClientIdentifier = xmlStartAttr(t, "machineIdentifier")
					cur.player.Product = xmlStartAttr(t, "product")
					cur.player.Platform = xmlStartAttr(t, "platform")
					if cur.player.Platform == "" {
						cur.player.Platform = xmlStartAttr(t, "platformTitle")
					}
				}
			case "Session":
				if cur != nil {
					cur.session = xmlStartAttr(t, "id")
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if cur != nil && (t.Name.Local == "Video" || t.Name.Local == "Track" || t.Name.Local == "Photo" || t.Name.Local == "Metadata") {
				matchSID := hints.SessionIdentifier != "" && cur.session != "" && cur.session == hints.SessionIdentifier
				matchCID := hints.ClientIdentifier != "" && cur.player.ClientIdentifier != "" && cur.player.ClientIdentifier == hints.ClientIdentifier
				if matchSID || matchCID {
					out := cur.player
					out.SessionIdentifier = cur.session
					out.Title = cur.title
					if out.ClientIdentifier == "" {
						out.ClientIdentifier = hints.ClientIdentifier
					}
					if out.SessionIdentifier == "" {
						out.SessionIdentifier = hints.SessionIdentifier
					}
					return &out, nil
				}
				cur = nil
			}
		}
	}
	return nil, nil
}

func looksLikePlexWeb(s string) bool {
	v := strings.ToLower(strings.TrimSpace(s))
	return strings.Contains(v, "plex web") || strings.Contains(v, "web") || strings.Contains(v, "browser") || strings.Contains(v, "firefox") || strings.Contains(v, "chrome") || strings.Contains(v, "safari")
}

func looksLikePlexInternalFetcher(product, platform string) bool {
	p := strings.ToLower(strings.TrimSpace(product))
	pl := strings.ToLower(strings.TrimSpace(platform))
	if strings.Contains(p, "lavf") || strings.Contains(pl, "lavf") {
		return true
	}
	if strings.Contains(p, "plex media server") || strings.Contains(pl, "plex media server") {
		return true
	}
	if strings.Contains(p, "segmenter") || strings.Contains(p, "ffmpeg") {
		return true
	}
	return false
}

func plexClientClass(info *plexResolvedClient) string {
	if info == nil {
		return "unknown"
	}
	if looksLikePlexWeb(info.Product) || looksLikePlexWeb(info.Platform) {
		return "web"
	}
	if looksLikePlexInternalFetcher(info.Product, info.Platform) {
		return "internal"
	}
	return "native"
}

func (g *Gateway) requestAdaptation(ctx context.Context, r *http.Request, channel *catalog.LiveChannel, channelID string) (bool, bool, string, string, string) {
	hints := plexRequestHints(r)
	log.Printf("gateway: channel=%q id=%s plex-hints %s", channel.GuideName, channelID, hints.summary())
	explicitProfileRaw := strings.TrimSpace(r.URL.Query().Get("profile"))
	if explicitProfileRaw != "" {
		resolved := g.resolveProfileSelection(explicitProfileRaw)
		if resolved.Known {
			return true, resolved.ForceTranscode, resolved.Name, "query-profile", "manual"
		}
		return true, false, resolved.Name, "query-profile", "manual"
	}
	if getenvBool("IPTV_TUNERR_FORCE_WEBSAFE", false) {
		return true, true, profilePlexSafe, "force-websafe", "manual"
	}
	if g.shouldAdaptStickyWebsafe(channelID, hints) {
		cc := g.stickyFallbackClientClass(ctx, hints)
		return true, true, profilePlexSafe, "sticky-fallback-websafe", cc
	}
	if !g.PlexClientAdapt {
		return false, false, "", "adapt-disabled", "unknown"
	}
	info, err := g.resolvePlexClient(ctx, hints)
	if err != nil {
		log.Printf("gateway: channel=%q id=%s plex-client-resolve err=%v", channel.GuideName, channelID, err)
		return true, true, profilePlexSafe, "resolve-error-websafe", "unknown"
	}
	clientClass := plexClientClass(info)
	if row, ok := g.lookupAutopilotDecision(channel, clientClass); ok {
		return true, row.Transcode, normalizeProfileName(row.Profile), "autopilot-memory", clientClass
	}
	if info == nil {
		return true, true, profilePlexSafe, "unknown-client-websafe", clientClass
	}
	log.Printf("gateway: channel=%q id=%s plex-client-resolved class=%s sid=%t cid=%t product=%t platform=%t title=%t",
		channel.GuideName, channelID, clientClass,
		info.SessionIdentifier != "",
		info.ClientIdentifier != "",
		info.Product != "",
		info.Platform != "",
		info.Title != "")
	if looksLikePlexWeb(info.Product) || looksLikePlexWeb(info.Platform) {
		return true, true, profilePlexSafe, "resolved-web-client", clientClass
	}
	if looksLikePlexInternalFetcher(info.Product, info.Platform) {
		return true, true, profilePlexSafe, "internal-fetcher-websafe", clientClass
	}
	return true, false, "", "resolved-nonweb-client", clientClass
}

func (g *Gateway) lookupAutopilotDecision(channel *catalog.LiveChannel, clientClass string) (autopilotDecision, bool) {
	if g == nil || g.Autopilot == nil || channel == nil {
		return autopilotDecision{}, false
	}
	row, ok := g.Autopilot.get(channel.DNAID, clientClass)
	if !ok {
		return autopilotDecision{}, false
	}
	if row.FailureStreak >= getenvInt("IPTV_TUNERR_AUTOPILOT_MAX_FAILURE_STREAK", 2) {
		return autopilotDecision{}, false
	}
	return row, true
}

func autopilotURLHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Hostname()))
}

// streamURLsSemanticallyEqual reports whether two stream URLs are the same for Autopilot
// preference matching after common catalog vs memory drift:
//   - HTTP(S) default ports (:80 / :443) when omitted vs explicit
//   - a single trailing slash on the path (including root)
//   - scheme and hostname ASCII case
//
// It does not fold path segment case, userinfo, or fragments; query strings must match
// exactly (RawQuery). Intentionally conservative so distinct CDN path spellings are not
// collapsed. Unit: TestStreamURLsSemanticallyEqual; integration: TestGateway_stream_prefersAutopilotRememberedURL_normalizedTrailingSlash.
func streamURLsSemanticallyEqual(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return true
	}
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	if err1 != nil || err2 != nil {
		return false
	}
	if !strings.EqualFold(ua.Scheme, ub.Scheme) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(ua.Hostname()), strings.TrimSpace(ub.Hostname())) {
		return false
	}
	ap := ua.Port()
	bp := ub.Port()
	if ap == "" {
		ap = defaultPortForScheme(ua.Scheme)
	}
	if bp == "" {
		bp = defaultPortForScheme(ub.Scheme)
	}
	if ap != bp {
		return false
	}
	pa := strings.TrimSuffix(ua.EscapedPath(), "/")
	pb := strings.TrimSuffix(ub.EscapedPath(), "/")
	if pa == "" {
		pa = "/"
	}
	if pb == "" {
		pb = "/"
	}
	if pa != pb {
		return false
	}
	return ua.RawQuery == ub.RawQuery
}

func defaultPortForScheme(scheme string) string {
	if strings.EqualFold(scheme, "https") {
		return "443"
	}
	return "80"
}

func upstreamURLAuthority(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Host))
}

func autopilotConsensusHostEnabled() bool {
	return getenvBool("IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST", false)
}

// autopilotConsensusPreferredURL picks a URL whose host matches Autopilot consensus (multi-DNA agreement)
// when IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST is enabled. Skips hosts with autotune penalty.
func (g *Gateway) autopilotConsensusPreferredURL(urls []string) string {
	if g == nil || g.Autopilot == nil || len(urls) == 0 || !autopilotConsensusHostEnabled() {
		return ""
	}
	minDNA := getenvInt("IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_DNA", 3)
	minSum := getenvInt("IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_HIT_SUM", 15)
	host, _, _ := g.Autopilot.consensusPreferredHost(minDNA, minSum)
	if host == "" {
		return ""
	}
	for _, candidate := range urls {
		if strings.TrimSpace(strings.ToLower(autopilotURLHost(candidate))) != host {
			continue
		}
		if g.hostPenalty(upstreamURLAuthority(candidate)) > 0 {
			continue
		}
		return candidate
	}
	return ""
}

func (g *Gateway) autopilotPreferredStreamURL(channel *catalog.LiveChannel, clientClass string, urls []string) string {
	if row, ok := g.lookupAutopilotDecision(channel, clientClass); ok {
		wantURL := strings.TrimSpace(row.PreferredURL)
		wantHost := strings.TrimSpace(row.PreferredHost)
		for _, candidate := range urls {
			if wantURL != "" && (candidate == wantURL || streamURLsSemanticallyEqual(candidate, wantURL)) {
				return candidate
			}
		}
		for _, candidate := range urls {
			if wantHost != "" && strings.EqualFold(autopilotURLHost(candidate), wantHost) {
				return candidate
			}
		}
	}
	if u := pickFirstURLMatchingGlobalPreferredHosts(urls, parseAutopilotGlobalPreferredHosts()); u != "" {
		return u
	}
	return g.autopilotConsensusPreferredURL(urls)
}

// parseAutopilotGlobalPreferredHosts returns hostnames from IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS
// (comma-separated). When non-empty, reorderStreamURLs may prefer a catalog URL whose host matches
// any entry (case-insensitive) after per-channel Autopilot memory and before consensus host — LTV
// provider-level policy without touching the JSON state file.
func parseAutopilotGlobalPreferredHosts() []string {
	return parseAutopilotHostPolicy().PreferredHosts
}

func pickFirstURLMatchingGlobalPreferredHosts(urls []string, preferredHosts []string) string {
	if len(preferredHosts) == 0 || len(urls) == 0 {
		return ""
	}
	set := make(map[string]struct{}, len(preferredHosts))
	for _, h := range preferredHosts {
		set[strings.ToLower(h)] = struct{}{}
	}
	for _, u := range urls {
		ch := strings.ToLower(strings.TrimSpace(autopilotURLHost(u)))
		if ch == "" {
			continue
		}
		if _, ok := set[ch]; ok {
			return u
		}
	}
	return ""
}

func (g *Gateway) reorderStreamURLs(channel *catalog.LiveChannel, clientClass string, urls []string) []string {
	if len(urls) < 2 {
		return urls
	}
	policy := parseAutopilotHostPolicy()
	urls = filterAutopilotBlockedHosts(urls, policy.BlockedHosts)
	preferred := g.autopilotPreferredStreamURL(channel, clientClass, urls)
	if preferred == "" {
		out := append([]string(nil), urls...)
		sort.SliceStable(out, func(i, j int) bool {
			left := g.hostPenalty(upstreamURLAuthority(out[i]))
			right := g.hostPenalty(upstreamURLAuthority(out[j]))
			if left == right {
				return i < j
			}
			return left < right
		})
		return g.reorderStreamURLsByAccountLoad(channel, out)
	}
	out := make([]string, 0, len(urls))
	out = append(out, preferred)
	rest := make([]string, 0, len(urls)-1)
	for _, candidate := range urls {
		if candidate == preferred {
			continue
		}
		rest = append(rest, candidate)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		left := g.hostPenalty(upstreamURLAuthority(rest[i]))
		right := g.hostPenalty(upstreamURLAuthority(rest[j]))
		if left == right {
			return i < j
		}
		return left < right
	})
	out = append(out, rest...)
	return g.reorderStreamURLsByAccountLoad(channel, out)
}

func (g *Gateway) rememberAutopilotDecision(channel *catalog.LiveChannel, clientClass string, transcode bool, profile, reason, preferredURL string) {
	if g == nil || g.Autopilot == nil || channel == nil {
		return
	}
	if strings.TrimSpace(channel.DNAID) == "" || strings.TrimSpace(clientClass) == "" {
		return
	}
	g.Autopilot.put(autopilotDecision{
		DNAID:         channel.DNAID,
		ClientClass:   clientClass,
		Profile:       normalizeProfileName(profile),
		Transcode:     transcode,
		Reason:        reason,
		PreferredURL:  strings.TrimSpace(preferredURL),
		PreferredHost: autopilotURLHost(preferredURL),
	})
}

func (g *Gateway) rememberAutopilotFailure(channel *catalog.LiveChannel, clientClass string) {
	if g == nil || g.Autopilot == nil || channel == nil {
		return
	}
	if strings.TrimSpace(channel.DNAID) == "" || strings.TrimSpace(clientClass) == "" {
		return
	}
	g.Autopilot.fail(channel.DNAID, clientClass)
}

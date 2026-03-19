package tuner

import (
	"os"
	"sort"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

type hostFailureStat struct {
	Host       string
	Failures   int
	LastStatus int
	LastKind   string
	LastURL    string
	LastAt     time.Time
}

type ProviderHostPenalty struct {
	Host       string `json:"host"`
	Failures   int    `json:"failures"`
	LastStatus int    `json:"last_status,omitempty"`
	LastKind   string `json:"last_kind,omitempty"`
	LastURL    string `json:"last_url,omitempty"`
	LastAt     string `json:"last_at,omitempty"`
}

type ProviderBehaviorProfile struct {
	ConfiguredTunerLimit   int                   `json:"configured_tuner_limit"`
	LearnedTunerLimit      int                   `json:"learned_tuner_limit"`
	EffectiveTunerLimit    int                   `json:"effective_tuner_limit"`
	BasicAuthConfigured    bool                  `json:"basic_auth_configured"`
	ForwardedHeaders       []string              `json:"forwarded_headers"`
	FFMPEGHLSReconnect     bool                  `json:"ffmpeg_hls_reconnect"`
	FetchCFReject          bool                  `json:"fetch_cf_reject"`
	ConcurrencySignalsSeen int                   `json:"concurrency_signals_seen"`
	LastConcurrencyStatus  int                   `json:"last_concurrency_status,omitempty"`
	LastConcurrencyBody    string                `json:"last_concurrency_body,omitempty"`
	LastConcurrencyAt      string                `json:"last_concurrency_at,omitempty"`
	CFBlockHits            int                   `json:"cf_block_hits"`
	LastCFBlockAt          string                `json:"last_cf_block_at,omitempty"`
	LastCFBlockURL         string                `json:"last_cf_block_url,omitempty"`
	ProviderAutotune       bool                  `json:"provider_autotune"`
	AutoHLSReconnect       bool                  `json:"auto_hls_reconnect"`
	HLSPlaylistFailures    int                   `json:"hls_playlist_failures"`
	LastHLSPlaylistAt      string                `json:"last_hls_playlist_at,omitempty"`
	LastHLSPlaylistURL     string                `json:"last_hls_playlist_url,omitempty"`
	HLSSegmentFailures     int                   `json:"hls_segment_failures"`
	LastHLSSegmentAt       string                `json:"last_hls_segment_at,omitempty"`
	LastHLSSegmentURL      string                `json:"last_hls_segment_url,omitempty"`
	PenalizedHosts         []ProviderHostPenalty `json:"penalized_hosts,omitempty"`
}

func (g *Gateway) noteHLSSegmentFailure(segURL string) {
	if g == nil {
		return
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	g.hlsSegmentFailures++
	g.lastHLSSegmentAt = time.Now().UTC()
	g.lastHLSSegmentURL = safeurl.RedactURL(segURL)
}

func providerAutotuneEnabled() bool {
	return envBool("IPTV_TUNERR_PROVIDER_AUTOTUNE", true)
}

func (g *Gateway) noteUpstreamCFBlock(rawURL string) {
	if g == nil {
		return
	}
	g.providerStateMu.Lock()
	g.cfBlockHits++
	g.lastCFBlockAt = time.Now().UTC()
	g.lastCFBlockURL = safeurl.RedactURL(rawURL)
	g.providerStateMu.Unlock()
	// CF blocks also feed into the host penalty system so autotune can deprioritize
	// CF-blocking hosts in catalog ordering and prefer non-CF backup URLs.
	g.noteUpstreamFailure(rawURL, 403, "cf_block")
	// Mark host as CF-tagged in the learned store so it's known across restarts.
	if g.cfLearnedStore != nil {
		if host := hostFromURL(rawURL); host != "" {
			go g.cfLearnedStore.markCFTagged(host)
		}
	}
}

func (g *Gateway) noteUpstreamFailure(rawURL string, status int, kind string) {
	if g == nil || !providerAutotuneEnabled() {
		return
	}
	host := upstreamURLAuthority(rawURL)
	if host == "" {
		return
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	if g.hostFailures == nil {
		g.hostFailures = map[string]hostFailureStat{}
	}
	row := g.hostFailures[host]
	row.Host = host
	row.Failures++
	row.LastStatus = status
	row.LastKind = strings.TrimSpace(kind)
	row.LastURL = safeurl.RedactURL(rawURL)
	row.LastAt = time.Now().UTC()
	g.hostFailures[host] = row
}

func (g *Gateway) noteUpstreamSuccess(rawURL string) {
	if g == nil || !providerAutotuneEnabled() {
		return
	}
	host := upstreamURLAuthority(rawURL)
	if host == "" {
		return
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	if g.hostFailures == nil {
		return
	}
	delete(g.hostFailures, host)
}

func (g *Gateway) hostPenalty(host string) int {
	if g == nil || !providerAutotuneEnabled() {
		return 0
	}
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return 0
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	return g.hostFailures[host].Failures
}

func (g *Gateway) penalizedHostsLocked() []ProviderHostPenalty {
	if len(g.hostFailures) == 0 {
		return nil
	}
	out := make([]ProviderHostPenalty, 0, len(g.hostFailures))
	for _, row := range g.hostFailures {
		item := ProviderHostPenalty{
			Host:       row.Host,
			Failures:   row.Failures,
			LastStatus: row.LastStatus,
			LastKind:   row.LastKind,
			LastURL:    row.LastURL,
		}
		if !row.LastAt.IsZero() {
			item.LastAt = row.LastAt.Format(time.RFC3339)
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Failures == out[j].Failures {
			return out[i].Host < out[j].Host
		}
		return out[i].Failures > out[j].Failures
	})
	return out
}

func (g *Gateway) shouldAutoEnableHLSReconnect() bool {
	if !providerAutotuneEnabled() {
		return false
	}
	if _, ok := os.LookupEnv("IPTV_TUNERR_FFMPEG_HLS_RECONNECT"); ok {
		return false
	}
	if g == nil {
		return false
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	return g.hlsPlaylistFailures > 0 || g.hlsSegmentFailures > 0
}

func (g *Gateway) ProviderBehaviorProfile() ProviderBehaviorProfile {
	if g == nil {
		return ProviderBehaviorProfile{}
	}
	g.mu.Lock()
	configured := g.configuredTunerLimit()
	learned := g.learnedUpstreamLimit
	effective := g.effectiveTunerLimitLocked()
	g.mu.Unlock()

	g.providerStateMu.Lock()
	concurrencyHits := g.concurrencyHits
	lastConcurrencyCode := g.lastConcurrencyCode
	lastConcurrencyBody := g.lastConcurrencyBody
	lastConcurrencyAt := g.lastConcurrencyAt
	cfBlockHits := g.cfBlockHits
	lastCFBlockAt := g.lastCFBlockAt
	lastCFBlockURL := g.lastCFBlockURL
	hlsPlaylistFailures := g.hlsPlaylistFailures
	lastHLSPlaylistAt := g.lastHLSPlaylistAt
	lastHLSPlaylistURL := g.lastHLSPlaylistURL
	hlsSegmentFailures := g.hlsSegmentFailures
	lastHLSSegmentAt := g.lastHLSSegmentAt
	lastHLSSegmentURL := g.lastHLSSegmentURL
	penalizedHosts := g.penalizedHostsLocked()
	g.providerStateMu.Unlock()

	prof := ProviderBehaviorProfile{
		ConfiguredTunerLimit:   configured,
		LearnedTunerLimit:      learned,
		EffectiveTunerLimit:    effective,
		BasicAuthConfigured:    strings.TrimSpace(g.ProviderUser) != "" || strings.TrimSpace(g.ProviderPass) != "",
		ForwardedHeaders:       append([]string(nil), forwardedUpstreamHeaderNames...),
		FFMPEGHLSReconnect:     getenvBool("IPTV_TUNERR_FFMPEG_HLS_RECONNECT", false),
		FetchCFReject:          g.FetchCFReject,
		ConcurrencySignalsSeen: concurrencyHits,
		LastConcurrencyStatus:  lastConcurrencyCode,
		LastConcurrencyBody:    lastConcurrencyBody,
		CFBlockHits:            cfBlockHits,
		LastCFBlockURL:         lastCFBlockURL,
		ProviderAutotune:       providerAutotuneEnabled(),
		AutoHLSReconnect:       g.shouldAutoEnableHLSReconnect(),
		HLSPlaylistFailures:    hlsPlaylistFailures,
		LastHLSPlaylistURL:     lastHLSPlaylistURL,
		HLSSegmentFailures:     hlsSegmentFailures,
		LastHLSSegmentURL:      lastHLSSegmentURL,
		PenalizedHosts:         penalizedHosts,
	}
	if !lastConcurrencyAt.IsZero() {
		prof.LastConcurrencyAt = lastConcurrencyAt.Format(time.RFC3339)
	}
	if !lastCFBlockAt.IsZero() {
		prof.LastCFBlockAt = lastCFBlockAt.Format(time.RFC3339)
	}
	if !lastHLSPlaylistAt.IsZero() {
		prof.LastHLSPlaylistAt = lastHLSPlaylistAt.Format(time.RFC3339)
	}
	if !lastHLSSegmentAt.IsZero() {
		prof.LastHLSSegmentAt = lastHLSSegmentAt.Format(time.RFC3339)
	}
	return prof
}

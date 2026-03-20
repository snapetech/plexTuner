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
	ConfiguredTunerLimit       int                   `json:"configured_tuner_limit"`
	LearnedTunerLimit          int                   `json:"learned_tuner_limit"`
	EffectiveTunerLimit        int                   `json:"effective_tuner_limit"`
	BasicAuthConfigured        bool                  `json:"basic_auth_configured"`
	ForwardedHeaders           []string              `json:"forwarded_headers"`
	FFMPEGHLSReconnect         bool                  `json:"ffmpeg_hls_reconnect"`
	FetchCFReject              bool                  `json:"fetch_cf_reject"`
	ConcurrencySignalsSeen     int                   `json:"concurrency_signals_seen"`
	LastConcurrencyStatus      int                   `json:"last_concurrency_status,omitempty"`
	LastConcurrencyBody        string                `json:"last_concurrency_body,omitempty"`
	LastConcurrencyAt          string                `json:"last_concurrency_at,omitempty"`
	CFBlockHits                int                   `json:"cf_block_hits"`
	LastCFBlockAt              string                `json:"last_cf_block_at,omitempty"`
	LastCFBlockURL             string                `json:"last_cf_block_url,omitempty"`
	ProviderAutotune           bool                  `json:"provider_autotune"`
	AutoHLSReconnect           bool                  `json:"auto_hls_reconnect"`
	HLSPlaylistFailures        int                   `json:"hls_playlist_failures"`
	LastHLSPlaylistAt          string                `json:"last_hls_playlist_at,omitempty"`
	LastHLSPlaylistURL         string                `json:"last_hls_playlist_url,omitempty"`
	HLSSegmentFailures         int                   `json:"hls_segment_failures"`
	LastHLSSegmentAt           string                `json:"last_hls_segment_at,omitempty"`
	LastHLSSegmentURL          string                `json:"last_hls_segment_url,omitempty"`
	HlsMuxSegInUse             int                   `json:"hls_mux_seg_in_use"`
	HlsMuxSegLimit             int                   `json:"hls_mux_seg_limit"`
	HlsMuxSegSuccess           uint64                `json:"hls_mux_seg_success"`
	HlsMuxSegErrScheme         uint64                `json:"hls_mux_seg_err_scheme"`
	HlsMuxSegErrPrivate        uint64                `json:"hls_mux_seg_err_private"`
	HlsMuxSegErrParam          uint64                `json:"hls_mux_seg_err_param"`
	HlsMuxSegUpstreamHTTPErrs  uint64                `json:"hls_mux_seg_upstream_http_errs"`
	HlsMuxSeg502               uint64                `json:"hls_mux_seg_502"`
	HlsMuxSeg503LimitHits      uint64                `json:"hls_mux_seg_503_limit_hits"`
	HlsMuxSegRateLimited       uint64                `json:"hls_mux_seg_rate_limited"`
	DashMuxSegSuccess          uint64                `json:"dash_mux_seg_success"`
	DashMuxSegErrScheme        uint64                `json:"dash_mux_seg_err_scheme"`
	DashMuxSegErrPrivate       uint64                `json:"dash_mux_seg_err_private"`
	DashMuxSegErrParam         uint64                `json:"dash_mux_seg_err_param"`
	DashMuxSegUpstreamHTTPErrs uint64                `json:"dash_mux_seg_upstream_http_errs"`
	DashMuxSeg502              uint64                `json:"dash_mux_seg_502"`
	DashMuxSeg503LimitHits     uint64                `json:"dash_mux_seg_503_limit_hits"`
	DashMuxSegRateLimited      uint64                `json:"dash_mux_seg_rate_limited"`
	PenalizedHosts             []ProviderHostPenalty `json:"penalized_hosts,omitempty"`
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
	hlsMuxSegInUse := g.hlsMuxSegInUse
	hlsMuxSegLimit := g.effectiveHLSMuxSegLimitLocked(nil)
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
		ConfiguredTunerLimit:       configured,
		LearnedTunerLimit:          learned,
		EffectiveTunerLimit:        effective,
		BasicAuthConfigured:        strings.TrimSpace(g.ProviderUser) != "" || strings.TrimSpace(g.ProviderPass) != "",
		ForwardedHeaders:           append([]string(nil), forwardedUpstreamHeaderNames...),
		FFMPEGHLSReconnect:         getenvBool("IPTV_TUNERR_FFMPEG_HLS_RECONNECT", false),
		FetchCFReject:              g.FetchCFReject,
		ConcurrencySignalsSeen:     concurrencyHits,
		LastConcurrencyStatus:      lastConcurrencyCode,
		LastConcurrencyBody:        lastConcurrencyBody,
		CFBlockHits:                cfBlockHits,
		LastCFBlockURL:             lastCFBlockURL,
		ProviderAutotune:           providerAutotuneEnabled(),
		AutoHLSReconnect:           g.shouldAutoEnableHLSReconnect(),
		HLSPlaylistFailures:        hlsPlaylistFailures,
		LastHLSPlaylistURL:         lastHLSPlaylistURL,
		HLSSegmentFailures:         hlsSegmentFailures,
		LastHLSSegmentURL:          lastHLSSegmentURL,
		HlsMuxSegInUse:             hlsMuxSegInUse,
		HlsMuxSegLimit:             hlsMuxSegLimit,
		HlsMuxSegSuccess:           g.hlsMuxSegSuccess.Load(),
		HlsMuxSegErrScheme:         g.hlsMuxSegErrScheme.Load(),
		HlsMuxSegErrPrivate:        g.hlsMuxSegErrPrivate.Load(),
		HlsMuxSegErrParam:          g.hlsMuxSegErrParam.Load(),
		HlsMuxSegUpstreamHTTPErrs:  g.hlsMuxSegUpstreamHTTPErrs.Load(),
		HlsMuxSeg502:               g.hlsMuxSeg502Fail.Load(),
		HlsMuxSeg503LimitHits:      g.hlsMuxSeg503LimitHits.Load(),
		HlsMuxSegRateLimited:       g.hlsMuxSegRateLimited.Load(),
		DashMuxSegSuccess:          g.dashMuxSegSuccess.Load(),
		DashMuxSegErrScheme:        g.dashMuxSegErrScheme.Load(),
		DashMuxSegErrPrivate:       g.dashMuxSegErrPrivate.Load(),
		DashMuxSegErrParam:         g.dashMuxSegErrParam.Load(),
		DashMuxSegUpstreamHTTPErrs: g.dashMuxSegUpstreamHTTPErrs.Load(),
		DashMuxSeg502:              g.dashMuxSeg502Fail.Load(),
		DashMuxSeg503LimitHits:     g.dashMuxSeg503LimitHits.Load(),
		DashMuxSegRateLimited:      g.dashMuxSegRateLimited.Load(),
		PenalizedHosts:             penalizedHosts,
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

func (g *Gateway) ResetProviderBehaviorProfile() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.learnedUpstreamLimit = 0
	g.mu.Unlock()

	g.providerStateMu.Lock()
	g.concurrencyHits = 0
	g.lastConcurrencyAt = time.Time{}
	g.lastConcurrencyBody = ""
	g.lastConcurrencyCode = 0
	g.cfBlockHits = 0
	g.lastCFBlockAt = time.Time{}
	g.lastCFBlockURL = ""
	g.hlsPlaylistFailures = 0
	g.lastHLSPlaylistAt = time.Time{}
	g.lastHLSPlaylistURL = ""
	g.hlsSegmentFailures = 0
	g.lastHLSSegmentAt = time.Time{}
	g.lastHLSSegmentURL = ""
	g.hostFailures = nil
	g.providerStateMu.Unlock()
}

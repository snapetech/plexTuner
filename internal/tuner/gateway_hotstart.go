package tuner

import (
	"os"
	"strconv"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

type hotStartConfig struct {
	Enabled          bool
	Reason           string
	StartupMinBytes  int
	StartupTimeoutMs int
	BootstrapSeconds float64
	ProgramKeepalive bool
}

func (g *Gateway) hotStartConfig(channel *catalog.LiveChannel, clientClass string) hotStartConfig {
	cfg := hotStartConfig{}
	if !getenvBool("IPTV_TUNERR_HOT_START_ENABLED", true) || channel == nil {
		return cfg
	}
	reason := g.hotStartReason(channel, clientClass)
	if reason == "" {
		return cfg
	}
	cfg.Enabled = true
	cfg.Reason = reason
	cfg.StartupMinBytes = getenvInt("IPTV_TUNERR_HOT_START_MIN_BYTES", 24576)
	cfg.StartupTimeoutMs = getenvInt("IPTV_TUNERR_HOT_START_TIMEOUT_MS", 15000)
	cfg.BootstrapSeconds = getenvFloat("IPTV_TUNERR_HOT_START_BOOTSTRAP_SECONDS", 2.0)
	cfg.ProgramKeepalive = getenvBool("IPTV_TUNERR_HOT_START_PROGRAM_KEEPALIVE", true)
	return cfg
}

func (g *Gateway) hotStartReason(channel *catalog.LiveChannel, clientClass string) string {
	if channel == nil {
		return ""
	}
	if matchesHotStartFavorite(channel) {
		return "favorite"
	}
	if matchesHotStartGroupTitle(channel) {
		return "group_title"
	}
	minHits := getenvInt("IPTV_TUNERR_HOT_START_MIN_HITS", 3)
	if g != nil && g.Autopilot != nil {
		if _, ok := g.Autopilot.hotDecision(channel.DNAID, clientClass, minHits); ok {
			return "autopilot_hits"
		}
	}
	return ""
}

func matchesHotStartFavorite(ch *catalog.LiveChannel) bool {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HOT_START_CHANNELS"))
	if raw == "" || ch == nil {
		return false
	}
	candidates := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			candidates[part] = true
		}
	}
	for _, key := range []string{
		strings.ToLower(strings.TrimSpace(ch.ChannelID)),
		strings.ToLower(strings.TrimSpace(ch.DNAID)),
		strings.ToLower(strings.TrimSpace(ch.GuideNumber)),
		strings.ToLower(strings.TrimSpace(ch.GuideName)),
	} {
		if key != "" && candidates[key] {
			return true
		}
	}
	return false
}

// matchesHotStartGroupTitle returns true when IPTV_TUNERR_HOT_START_GROUP_TITLES is set and any
// comma-separated substring matches channel GroupTitle (case-insensitive substring match).
// Empty GroupTitle never matches. Whole categories can be marked hot without listing each channel id.
func matchesHotStartGroupTitle(ch *catalog.LiveChannel) bool {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HOT_START_GROUP_TITLES"))
	if raw == "" || ch == nil {
		return false
	}
	gt := strings.TrimSpace(ch.GroupTitle)
	if gt == "" {
		return false
	}
	gtLower := strings.ToLower(gt)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(gtLower, strings.ToLower(part)) {
			return true
		}
	}
	return false
}

func applyHotStartOverrides(startupMin, startupTimeoutMs int, bootstrapSec float64, enableProgramKeepalive bool, cfg hotStartConfig) (int, int, float64, bool) {
	if !cfg.Enabled {
		return startupMin, startupTimeoutMs, bootstrapSec, enableProgramKeepalive
	}
	if cfg.StartupMinBytes > 0 && (startupMin <= 0 || cfg.StartupMinBytes < startupMin) {
		startupMin = cfg.StartupMinBytes
	}
	if cfg.StartupTimeoutMs > 0 && (startupTimeoutMs <= 0 || cfg.StartupTimeoutMs < startupTimeoutMs) {
		startupTimeoutMs = cfg.StartupTimeoutMs
	}
	if cfg.BootstrapSeconds > bootstrapSec {
		bootstrapSec = cfg.BootstrapSeconds
	}
	if cfg.ProgramKeepalive {
		enableProgramKeepalive = true
	}
	return startupMin, startupTimeoutMs, bootstrapSec, enableProgramKeepalive
}

func hotStartSummary(cfg hotStartConfig) string {
	if !cfg.Enabled {
		return "off"
	}
	return cfg.Reason + " min=" + strconv.Itoa(cfg.StartupMinBytes) + " timeout_ms=" + strconv.Itoa(cfg.StartupTimeoutMs)
}

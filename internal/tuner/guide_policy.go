package tuner

import (
	"log"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
)

func normalizeGuidePolicy(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "", "off", "none":
		return "off"
	case "healthy", "good", "real":
		return "healthy"
	case "strict":
		return "strict"
	default:
		return "off"
	}
}

func (x *XMLTV) cachedGuideHealthReport() (guidehealth.Report, bool) {
	if x == nil {
		return guidehealth.Report{}, false
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.cachedGuideHealth == nil {
		return guidehealth.Report{}, false
	}
	rep := *x.cachedGuideHealth
	return rep, true
}

func filterChannelsByGuidePolicy(live []catalog.LiveChannel, rep guidehealth.Report, policy string) ([]catalog.LiveChannel, int) {
	policy = normalizeGuidePolicy(policy)
	if policy == "off" || len(live) == 0 || len(rep.Channels) == 0 {
		return live, 0
	}
	byChannelID := make(map[string]guidehealth.ChannelHealth, len(rep.Channels))
	for _, row := range rep.Channels {
		byChannelID[row.ChannelID] = row
	}
	out := make([]catalog.LiveChannel, 0, len(live))
	dropped := 0
	for _, ch := range live {
		row, ok := byChannelID[ch.ChannelID]
		if !ok {
			out = append(out, ch)
			continue
		}
		if guidePolicyKeeps(row, policy) {
			out = append(out, ch)
			continue
		}
		dropped++
	}
	return out, dropped
}

func guidePolicyKeeps(row guidehealth.ChannelHealth, policy string) bool {
	switch normalizeGuidePolicy(policy) {
	case "healthy":
		return row.HasRealProgrammes
	case "strict":
		return row.HasRealProgrammes && strings.TrimSpace(row.TVGID) != ""
	default:
		return true
	}
}

func (x *XMLTV) applyGuidePolicyToChannels(live []catalog.LiveChannel, policy string) []catalog.LiveChannel {
	policy = normalizeGuidePolicy(policy)
	if policy == "off" || len(live) == 0 {
		return live
	}
	rep, ok := x.cachedGuideHealthReport()
	if !ok || !rep.SourceReady {
		log.Printf("Guide policy deferred: policy=%s cached guide-health unavailable", policy)
		return live
	}
	filtered, dropped := filterChannelsByGuidePolicy(live, rep, policy)
	if dropped > 0 {
		log.Printf("Guide policy applied: policy=%s kept=%d/%d", policy, len(filtered), len(live))
	}
	return filtered
}

func FilterCatchupCapsulesByGuidePolicy(rep CatchupCapsulePreview, health guidehealth.Report, policy string) CatchupCapsulePreview {
	policy = normalizeGuidePolicy(policy)
	if policy == "off" || len(rep.Capsules) == 0 || len(health.Channels) == 0 {
		return rep
	}
	byChannelID := make(map[string]guidehealth.ChannelHealth, len(health.Channels))
	byGuideNumber := make(map[string]guidehealth.ChannelHealth, len(health.Channels))
	for _, row := range health.Channels {
		byChannelID[row.ChannelID] = row
		byGuideNumber[row.GuideNumber] = row
	}
	out := rep
	out.Capsules = make([]CatchupCapsule, 0, len(rep.Capsules))
	for _, capsule := range rep.Capsules {
		row, ok := byChannelID[capsule.ChannelID]
		if !ok {
			row, ok = byGuideNumber[capsule.ChannelID]
		}
		if ok && !guidePolicyKeeps(row, policy) {
			continue
		}
		out.Capsules = append(out.Capsules, capsule)
	}
	return out
}

func buildGuideHealthForChannels(live []catalog.LiveChannel, guideXML []byte, now time.Time) (guidehealth.Report, error) {
	return guidehealth.Build(live, guideXML, nil, now)
}

func BuildGuideHealthForPolicy(live []catalog.LiveChannel, guideXML []byte, now time.Time) (guidehealth.Report, error) {
	return buildGuideHealthForChannels(live, guideXML, now)
}

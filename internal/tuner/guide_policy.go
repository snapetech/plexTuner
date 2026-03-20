package tuner

import (
	"log"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
)

type GuidePolicySummary struct {
	Policy                  string `json:"policy"`
	SourceReady             bool   `json:"source_ready"`
	Deferred                bool   `json:"deferred,omitempty"`
	TotalChannels           int    `json:"total_channels"`
	HealthyChannels         int    `json:"healthy_channels"`
	PlaceholderOnlyChannels int    `json:"placeholder_only_channels"`
	NoProgrammeChannels     int    `json:"no_programme_channels"`
	KeptChannels            int    `json:"kept_channels"`
	DroppedChannels         int    `json:"dropped_channels"`
	DroppedPlaceholderOnly  int    `json:"dropped_placeholder_only"`
	DroppedNoProgramme      int    `json:"dropped_no_programme"`
	DroppedMissingTVGID     int    `json:"dropped_missing_tvg_id,omitempty"`
}

type GuidePolicyDecision struct {
	ChannelID         string `json:"channel_id"`
	GuideNumber       string `json:"guide_number"`
	GuideName         string `json:"guide_name"`
	TVGID             string `json:"tvg_id,omitempty"`
	Status            string `json:"status"`
	HasRealProgrammes bool   `json:"has_real_programmes"`
	PlaceholderOnly   bool   `json:"placeholder_only"`
	Keep              bool   `json:"keep"`
	DropReason        string `json:"drop_reason,omitempty"`
}

type GuidePolicyReport struct {
	GeneratedAt string                `json:"generated_at"`
	Summary     GuidePolicySummary    `json:"summary"`
	Channels    []GuidePolicyDecision `json:"channels,omitempty"`
}

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

func guidePolicyDropReason(row guidehealth.ChannelHealth, policy string) string {
	switch normalizeGuidePolicy(policy) {
	case "healthy":
		if !row.HasRealProgrammes {
			if row.PlaceholderOnly {
				return "placeholder_only"
			}
			return "no_programme"
		}
	case "strict":
		if !row.HasRealProgrammes {
			if row.PlaceholderOnly {
				return "placeholder_only"
			}
			return "no_programme"
		}
		if strings.TrimSpace(row.TVGID) == "" {
			return "missing_tvg_id"
		}
	}
	return ""
}

func buildGuidePolicyReport(live []catalog.LiveChannel, rep guidehealth.Report, policy string) GuidePolicyReport {
	policy = normalizeGuidePolicy(policy)
	out := GuidePolicyReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Summary: GuidePolicySummary{
			Policy:        policy,
			SourceReady:   rep.SourceReady,
			TotalChannels: len(live),
		},
		Channels: make([]GuidePolicyDecision, 0, len(live)),
	}
	if policy == "off" || len(live) == 0 || len(rep.Channels) == 0 {
		out.Summary.KeptChannels = len(live)
		return out
	}
	byChannelID := make(map[string]guidehealth.ChannelHealth, len(rep.Channels))
	for _, row := range rep.Channels {
		byChannelID[row.ChannelID] = row
	}
	for _, ch := range live {
		row, ok := byChannelID[ch.ChannelID]
		decision := GuidePolicyDecision{
			ChannelID:   ch.ChannelID,
			GuideNumber: ch.GuideNumber,
			GuideName:   ch.GuideName,
			TVGID:       ch.TVGID,
			Keep:        true,
		}
		if ok {
			decision.Status = row.Status
			decision.HasRealProgrammes = row.HasRealProgrammes
			decision.PlaceholderOnly = row.PlaceholderOnly
			if row.HasRealProgrammes {
				out.Summary.HealthyChannels++
			}
			if row.PlaceholderOnly {
				out.Summary.PlaceholderOnlyChannels++
			}
			if !row.HasProgrammes {
				out.Summary.NoProgrammeChannels++
			}
			if reason := guidePolicyDropReason(row, policy); reason != "" {
				decision.Keep = false
				decision.DropReason = reason
				out.Summary.DroppedChannels++
				switch reason {
				case "placeholder_only":
					out.Summary.DroppedPlaceholderOnly++
				case "no_programme":
					out.Summary.DroppedNoProgramme++
				case "missing_tvg_id":
					out.Summary.DroppedMissingTVGID++
				}
			} else {
				out.Summary.KeptChannels++
			}
		} else {
			out.Summary.KeptChannels++
		}
		out.Channels = append(out.Channels, decision)
	}
	return out
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
	return guidePolicyDropReason(row, policy) == ""
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
	report := buildGuidePolicyReport(live, rep, policy)
	filtered, dropped := filterChannelsByGuidePolicy(live, rep, policy)
	if dropped > 0 || report.Summary.DroppedMissingTVGID > 0 {
		log.Printf(
			"Guide policy applied: policy=%s kept=%d/%d dropped=%d placeholder_only=%d no_programme=%d missing_tvg_id=%d",
			policy,
			len(filtered),
			len(live),
			report.Summary.DroppedChannels,
			report.Summary.DroppedPlaceholderOnly,
			report.Summary.DroppedNoProgramme,
			report.Summary.DroppedMissingTVGID,
		)
	}
	return filtered
}

func (x *XMLTV) guidePolicyReport(live []catalog.LiveChannel, policy string) (GuidePolicyReport, bool) {
	policy = normalizeGuidePolicy(policy)
	if policy == "off" {
		return GuidePolicyReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Summary: GuidePolicySummary{
				Policy:        policy,
				SourceReady:   true,
				TotalChannels: len(live),
				KeptChannels:  len(live),
			},
		}, true
	}
	rep, ok := x.cachedGuideHealthReport()
	if !ok {
		return GuidePolicyReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Summary: GuidePolicySummary{
				Policy:        policy,
				Deferred:      true,
				TotalChannels: len(live),
			},
		}, false
	}
	report := buildGuidePolicyReport(live, rep, policy)
	if !rep.SourceReady {
		report.Summary.Deferred = true
	}
	return report, rep.SourceReady
}

func FilterCatchupCapsulesByGuidePolicy(rep CatchupCapsulePreview, health guidehealth.Report, policy string) CatchupCapsulePreview {
	policy = normalizeGuidePolicy(policy)
	if policy == "off" || len(rep.Capsules) == 0 || len(health.Channels) == 0 {
		rep.GuidePolicy = &GuidePolicySummary{
			Policy:        policy,
			SourceReady:   health.SourceReady,
			KeptChannels:  len(rep.Capsules),
			TotalChannels: len(rep.Capsules),
		}
		return rep
	}
	byChannelID := make(map[string]guidehealth.ChannelHealth, len(health.Channels))
	byGuideNumber := make(map[string]guidehealth.ChannelHealth, len(health.Channels))
	summary := GuidePolicySummary{
		Policy:        policy,
		SourceReady:   health.SourceReady,
		TotalChannels: len(rep.Capsules),
	}
	for _, row := range health.Channels {
		byChannelID[row.ChannelID] = row
		byGuideNumber[row.GuideNumber] = row
		if row.HasRealProgrammes {
			summary.HealthyChannels++
		}
		if row.PlaceholderOnly {
			summary.PlaceholderOnlyChannels++
		}
		if !row.HasProgrammes {
			summary.NoProgrammeChannels++
		}
	}
	out := rep
	out.Capsules = make([]CatchupCapsule, 0, len(rep.Capsules))
	for _, capsule := range rep.Capsules {
		row, ok := byChannelID[capsule.ChannelID]
		if !ok {
			row, ok = byGuideNumber[capsule.ChannelID]
		}
		if ok {
			if reason := guidePolicyDropReason(row, policy); reason != "" {
				summary.DroppedChannels++
				switch reason {
				case "placeholder_only":
					summary.DroppedPlaceholderOnly++
				case "no_programme":
					summary.DroppedNoProgramme++
				case "missing_tvg_id":
					summary.DroppedMissingTVGID++
				}
				continue
			}
		}
		summary.KeptChannels++
		out.Capsules = append(out.Capsules, capsule)
	}
	out.GuidePolicy = &summary
	return out
}

func buildGuideHealthForChannels(live []catalog.LiveChannel, guideXML []byte, now time.Time) (guidehealth.Report, error) {
	return guidehealth.Build(live, guideXML, nil, now)
}

func BuildGuideHealthForPolicy(live []catalog.LiveChannel, guideXML []byte, now time.Time) (guidehealth.Report, error) {
	return buildGuideHealthForChannels(live, guideXML, now)
}

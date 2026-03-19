package tuner

import (
	"log"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/channelreport"
)

func normalizeDNAPolicy(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "", "off", "none":
		return "off"
	case "prefer_best", "best":
		return "prefer_best"
	case "prefer_resilient", "resilient":
		return "prefer_resilient"
	default:
		return "off"
	}
}

func applyDNAPolicy(live []catalog.LiveChannel, policy string) []catalog.LiveChannel {
	policy = normalizeDNAPolicy(policy)
	if policy == "off" || len(live) == 0 {
		return live
	}
	bestByDNA := make(map[string]int, len(live))
	for i, ch := range live {
		dna := strings.TrimSpace(ch.DNAID)
		if dna == "" {
			dna = channeldna.Compute(ch)
		}
		if prev, ok := bestByDNA[dna]; ok {
			if dnaPolicyBetter(live[i], live[prev], policy) {
				bestByDNA[dna] = i
			}
			continue
		}
		bestByDNA[dna] = i
	}
	out := make([]catalog.LiveChannel, 0, len(bestByDNA))
	dropped := 0
	for i, ch := range live {
		dna := strings.TrimSpace(ch.DNAID)
		if dna == "" {
			dna = channeldna.Compute(ch)
		}
		if bestByDNA[dna] != i {
			dropped++
			continue
		}
		out = append(out, ch)
	}
	if dropped > 0 {
		log.Printf("DNA policy applied: policy=%s kept=%d/%d", policy, len(out), len(live))
	}
	return out
}

func ApplyDNAPolicyForRegistration(live []catalog.LiveChannel, policy string) []catalog.LiveChannel {
	return applyDNAPolicy(live, policy)
}

func dnaPolicyBetter(left, right catalog.LiveChannel, policy string) bool {
	leftScore := channelreport.Score(left)
	rightScore := channelreport.Score(right)
	leftGuide := channelreport.GuideConfidence(left)
	rightGuide := channelreport.GuideConfidence(right)
	leftStream := channelreport.StreamResilience(left)
	rightStream := channelreport.StreamResilience(right)
	switch normalizeDNAPolicy(policy) {
	case "prefer_resilient":
		if leftStream != rightStream {
			return leftStream > rightStream
		}
		if leftScore != rightScore {
			return leftScore > rightScore
		}
	default:
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if leftStream != rightStream {
			return leftStream > rightStream
		}
	}
	if leftGuide != rightGuide {
		return leftGuide > rightGuide
	}
	if len(left.StreamURLs) != len(right.StreamURLs) {
		return len(left.StreamURLs) > len(right.StreamURLs)
	}
	if strings.TrimSpace(left.TVGID) != strings.TrimSpace(right.TVGID) {
		return strings.TrimSpace(left.TVGID) != ""
	}
	return strings.ToLower(strings.TrimSpace(left.GuideName)) < strings.ToLower(strings.TrimSpace(right.GuideName))
}

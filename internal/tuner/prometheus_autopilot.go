package tuner

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var promAutopilotOnce sync.Once

// promRegisterAutopilotMetrics registers GaugeFuncs that reflect current Autopilot consensus
// (same thresholds as /autopilot/report.json). No-op if g is nil. Safe to call once per process.
func promRegisterAutopilotMetrics(g *Gateway) {
	if g == nil {
		return
	}
	promAutopilotOnce.Do(func() {
		prometheus.MustRegister(
			prometheus.NewGaugeFunc(
				prometheus.GaugeOpts{
					Namespace: "iptv_tunerr",
					Subsystem: "autopilot",
					Name:      "consensus_dna_count",
					Help:      "Distinct DNA IDs in the winning consensus preferred_host bucket (0 if none qualifies).",
				},
				func() float64 {
					return float64(promAutopilotConsensusSnapshot(g).dna)
				},
			),
			prometheus.NewGaugeFunc(
				prometheus.GaugeOpts{
					Namespace: "iptv_tunerr",
					Subsystem: "autopilot",
					Name:      "consensus_hit_sum",
					Help:      "Sum of Autopilot Hits rows for the winning consensus host (0 if none).",
				},
				func() float64 {
					return float64(promAutopilotConsensusSnapshot(g).hitSum)
				},
			),
			prometheus.NewGaugeFunc(
				prometheus.GaugeOpts{
					Namespace: "iptv_tunerr",
					Subsystem: "autopilot",
					Name:      "consensus_runtime_enabled",
					Help:      "1 if IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST is enabled, else 0.",
				},
				func() float64 {
					if autopilotConsensusHostEnabled() {
						return 1
					}
					return 0
				},
			),
		)
	})
}

type promAutopilotConsensus struct {
	dna    int
	hitSum int
}

func promAutopilotConsensusSnapshot(g *Gateway) promAutopilotConsensus {
	out := promAutopilotConsensus{}
	if g == nil || g.Autopilot == nil {
		return out
	}
	md := getenvInt("IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_DNA", 3)
	ms := getenvInt("IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_HIT_SUM", 15)
	_, dna, sum := g.Autopilot.consensusPreferredHost(md, ms)
	out.dna = dna
	out.hitSum = sum
	return out
}

package tuner

import (
	"testing"
)

func TestPromRegisterAutopilotMetrics_nilGateway(t *testing.T) {
	promRegisterAutopilotMetrics(nil)
}

func TestPromAutopilotConsensusSnapshot(t *testing.T) {
	t.Setenv("IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_DNA", "2")
	t.Setenv("IPTV_TUNERR_AUTOPILOT_CONSENSUS_MIN_HIT_SUM", "8")
	store := &autopilotStore{byKey: map[string]autopilotDecision{
		autopilotKey("dna:one", "web"): {DNAID: "dna:one", ClientClass: "web", PreferredHost: "cdn.example.com", Hits: 5},
		autopilotKey("dna:two", "web"): {DNAID: "dna:two", ClientClass: "web", PreferredHost: "cdn.example.com", Hits: 5},
	}}
	g := &Gateway{TunerCount: 2, Autopilot: store}
	s := promAutopilotConsensusSnapshot(g)
	if s.dna != 2 || s.hitSum != 10 {
		t.Fatalf("got dna=%d hitSum=%d", s.dna, s.hitSum)
	}
}

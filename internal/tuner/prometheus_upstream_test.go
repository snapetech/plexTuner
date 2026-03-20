package tuner

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPromNoteUpstreamQuarantineSkips_noopWhenMetricsDisabled(t *testing.T) {
	t.Setenv("IPTV_TUNERR_METRICS_ENABLE", "false")
	promNoteUpstreamQuarantineSkips(10)
}

func TestPromNoteUpstreamQuarantineSkips_increments(t *testing.T) {
	t.Setenv("IPTV_TUNERR_METRICS_ENABLE", "true")
	promNoteUpstreamQuarantineSkips(2)
	promRegisterUpstreamMetrics()
	if got := testutil.ToFloat64(promUpstreamQuarantineSkips); got != 2 {
		t.Fatalf("counter=%v want 2", got)
	}
}

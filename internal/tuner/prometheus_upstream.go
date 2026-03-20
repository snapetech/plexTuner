package tuner

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var promUpstreamOnce sync.Once

var promUpstreamQuarantineSkips prometheus.Counter

// promRegisterUpstreamMetrics registers upstream-related counters (no-op after first call).
func promRegisterUpstreamMetrics() {
	promUpstreamOnce.Do(func() {
		promUpstreamQuarantineSkips = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "iptv_tunerr",
			Subsystem: "upstream",
			Name:      "quarantine_skips_total",
			Help:      "Upstream URLs dropped by host quarantine filtering when at least one non-quarantined backup remained.",
		})
		prometheus.MustRegister(promUpstreamQuarantineSkips)
	})
}

func promNoteUpstreamQuarantineSkips(n int) {
	if !metricsEnableFromEnv() || n <= 0 {
		return
	}
	promRegisterUpstreamMetrics()
	promUpstreamQuarantineSkips.Add(float64(n))
}

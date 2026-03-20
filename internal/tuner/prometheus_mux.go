package tuner

import (
	"os"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var promMuxOnce sync.Once

var promMuxOutcomes *prometheus.CounterVec

func metricsEnableFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_METRICS_ENABLE")))
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

func promNoteMuxSegOutcome(mux, outcome string) {
	if !metricsEnableFromEnv() {
		return
	}
	promMuxOnce.Do(func() {
		promMuxOutcomes = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "iptv_tunerr",
			Subsystem: "mux",
			Name:      "seg_outcomes_total",
			Help:      "Outcomes for ?mux=hls|dash&seg= segment proxies",
		}, []string{"mux", "outcome"})
		prometheus.MustRegister(promMuxOutcomes)
	})
	promMuxOutcomes.WithLabelValues(mux, outcome).Inc()
}

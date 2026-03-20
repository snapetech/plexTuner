package tuner

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PromNoMuxSegHistogram pass to promNoteMuxSegOutcome to skip duration histogram observation.
const PromNoMuxSegHistogram = time.Duration(-1)
const PromNoMuxManifestHistogram = time.Duration(-1)

var promMuxOnce sync.Once

var (
	promMuxOutcomes         *prometheus.CounterVec
	promMuxDur              *prometheus.HistogramVec
	promMuxManifestOutcomes *prometheus.CounterVec
	promMuxManifestDur      *prometheus.HistogramVec
	promMuxChLabels         bool
)

func metricsEnableFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_METRICS_ENABLE")))
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

func metricsMuxChannelLabelsFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_METRICS_MUX_CHANNEL_LABELS")))
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

func promMuxChannelLabel(channelID string) string {
	if promMuxChLabels && strings.TrimSpace(channelID) != "" {
		return channelID
	}
	return "_aggregate"
}

func promRegisterMuxMetrics() {
	promMuxOnce.Do(func() {
		promMuxChLabels = metricsMuxChannelLabelsFromEnv()
		outcomeLabels := []string{"mux", "outcome"}
		if promMuxChLabels {
			outcomeLabels = []string{"mux", "outcome", "channel_id"}
		}
		promMuxOutcomes = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "iptv_tunerr",
			Subsystem: "mux",
			Name:      "seg_outcomes_total",
			Help:      "Outcomes for ?mux=hls|dash&seg= segment proxies",
		}, outcomeLabels)

		histLabels := []string{"mux"}
		if promMuxChLabels {
			histLabels = []string{"mux", "channel_id"}
		}
		promMuxDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "iptv_tunerr",
			Subsystem: "mux",
			Name:      "seg_request_duration_seconds",
			Help:      "Wall time for completed ?mux=hls|dash&seg= upstream relay (success or HTTP error pass-through)",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
		}, histLabels)
		promMuxManifestOutcomes = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "iptv_tunerr",
			Subsystem: "mux",
			Name:      "manifest_outcomes_total",
			Help:      "Outcomes for native mux manifest handling (entry playlists/MPDs and nested manifest targets)",
		}, outcomeLabels)
		promMuxManifestDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "iptv_tunerr",
			Subsystem: "mux",
			Name:      "manifest_request_duration_seconds",
			Help:      "Wall time for native mux manifest handling (playlist/MPD rewrite, 304, or upstream manifest HTTP errors)",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
		}, histLabels)
		prometheus.MustRegister(promMuxOutcomes, promMuxDur, promMuxManifestOutcomes, promMuxManifestDur)
	})
}

func promNoteMuxSegOutcome(mux, outcome, channelID string, elapsed time.Duration) {
	if !metricsEnableFromEnv() {
		return
	}
	promRegisterMuxMetrics()
	ch := promMuxChannelLabel(channelID)
	if promMuxChLabels {
		promMuxOutcomes.WithLabelValues(mux, outcome, ch).Inc()
	} else {
		promMuxOutcomes.WithLabelValues(mux, outcome).Inc()
	}
	if elapsed >= 0 {
		if promMuxChLabels {
			promMuxDur.WithLabelValues(mux, ch).Observe(elapsed.Seconds())
		} else {
			promMuxDur.WithLabelValues(mux).Observe(elapsed.Seconds())
		}
	}
}

func promNoteMuxManifestOutcome(mux, outcome, channelID string, elapsed time.Duration) {
	if !metricsEnableFromEnv() {
		return
	}
	promRegisterMuxMetrics()
	ch := promMuxChannelLabel(channelID)
	if promMuxChLabels {
		promMuxManifestOutcomes.WithLabelValues(mux, outcome, ch).Inc()
	} else {
		promMuxManifestOutcomes.WithLabelValues(mux, outcome).Inc()
	}
	if elapsed >= 0 {
		if promMuxChLabels {
			promMuxManifestDur.WithLabelValues(mux, ch).Observe(elapsed.Seconds())
		} else {
			promMuxManifestDur.WithLabelValues(mux).Observe(elapsed.Seconds())
		}
	}
}

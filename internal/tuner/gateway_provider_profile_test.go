package tuner

import (
	"reflect"
	"testing"
)

func TestRemediationHintsForProfile_empty(t *testing.T) {
	h := remediationHintsForProfile(ProviderBehaviorProfile{})
	if len(h) != 0 {
		t.Fatalf("want empty, got %#v", h)
	}
}

func TestRemediationHintsForProfile_cfFetchReject(t *testing.T) {
	h := remediationHintsForProfile(ProviderBehaviorProfile{
		CFBlockHits:   1,
		FetchCFReject: false,
	})
	want := []ProviderRemediationHint{{
		Code:     "fetch_cf_reject",
		Severity: "warn",
		Message:  "Cloudflare-block responses were seen; rejecting Cloudflare-proxied provider URLs at ingest can avoid bad upstreams.",
		Env:      "IPTV_TUNERR_FETCH_CF_REJECT",
	}}
	if !reflect.DeepEqual(h, want) {
		t.Fatalf("got %#v want %#v", h, want)
	}
}

func TestRemediationHintsForProfile_cfNoHintWhenRejectEnabled(t *testing.T) {
	h := remediationHintsForProfile(ProviderBehaviorProfile{
		CFBlockHits:   3,
		FetchCFReject: true,
	})
	if len(h) != 0 {
		t.Fatalf("want empty, got %#v", h)
	}
}

func TestRemediationHintsForProfile_penalizedHosts(t *testing.T) {
	h := remediationHintsForProfile(ProviderBehaviorProfile{
		PenalizedHosts: []ProviderHostPenalty{
			{Host: "a", Failures: 5},
			{Host: "b", Failures: 5},
			{Host: "c", Failures: 5},
		},
	})
	if len(h) != 1 || h[0].Code != "upstream_host_churn" {
		t.Fatalf("got %#v", h)
	}
}

func TestRemediationHintsForProfile_penalizedFailuresSum(t *testing.T) {
	h := remediationHintsForProfile(ProviderBehaviorProfile{
		PenalizedHosts: []ProviderHostPenalty{
			{Host: "only", Failures: 20},
		},
	})
	if len(h) != 1 || h[0].Code != "upstream_host_churn" {
		t.Fatalf("got %#v", h)
	}
}

func TestRemediationHintsForProfile_sortedCodes(t *testing.T) {
	h := remediationHintsForProfile(ProviderBehaviorProfile{
		CFBlockHits:            1,
		FetchCFReject:          false,
		ConcurrencySignalsSeen: 1,
	})
	if len(h) != 2 {
		t.Fatalf("got %#v", h)
	}
	if h[0].Code != "concurrency_limit" || h[1].Code != "fetch_cf_reject" {
		t.Fatalf("want concurrency then fetch_cf_reject, got %#v", h)
	}
}

func TestRemediationHintsForProfile_hostQuarantineActive(t *testing.T) {
	h := remediationHintsForProfile(ProviderBehaviorProfile{
		AutoHostQuarantine: true,
		QuarantinedHosts:   []ProviderHostPenalty{{Host: "bad.example", Failures: 4, QuarantinedUntil: "2026-03-20T12:00:00Z"}},
	})
	if len(h) != 1 || h[0].Code != "host_quarantine_active" {
		t.Fatalf("got %#v", h)
	}
}

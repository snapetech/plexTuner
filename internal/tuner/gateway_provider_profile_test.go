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

func TestRemediationHintsForProfile_accountPoolActive(t *testing.T) {
	h := remediationHintsForProfile(ProviderBehaviorProfile{
		AccountPoolLimit: 1,
		AccountLeases:    []providerAccountLease{{Label: "provider/u1", InUse: 1}},
	})
	if len(h) != 1 || h[0].Code != "account_pool_active" {
		t.Fatalf("got %#v", h)
	}
}

func TestProviderBehaviorProfile_includesAccountPoolState(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT", "2")
	g := &Gateway{
		ProviderUser: "demo",
		ProviderPass: "pass",
		accountLeases: map[string]int{
			"provider.example|demo|pass|http://provider.example/live/demo/pass/": 1,
		},
	}
	prof := g.ProviderBehaviorProfile()
	if !prof.AccountPoolConfigured || prof.AccountPoolLimit != 2 {
		t.Fatalf("account pool config = %#v", prof)
	}
	if len(prof.AccountLeases) != 1 || prof.AccountLeases[0].InUse != 1 {
		t.Fatalf("account leases = %#v", prof.AccountLeases)
	}
	if prof.AccountLeases[0].Host != "provider.example" {
		t.Fatalf("lease host = %#v", prof.AccountLeases[0])
	}
}

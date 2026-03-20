package httpclient

import (
	"testing"
	"time"
)

func TestParseSharedTransportEnv(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		wantPerH int
		wantIdle time.Duration
		wantMaxI int
	}{
		{
			name:     "defaults",
			env:      nil,
			wantPerH: MaxIdleConnsPerHost,
			wantIdle: DefaultIdleConnTimeout,
			wantMaxI: defaultMaxIdleConns,
		},
		{
			name: "all overrides",
			env: map[string]string{
				"IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST": "32",
				"IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC":   "120",
				"IPTV_TUNERR_HTTP_MAX_IDLE_CONNS":          "200",
			},
			wantPerH: 32,
			wantIdle: 120 * time.Second,
			wantMaxI: 200,
		},
		{
			name: "ignore zero and invalid",
			env: map[string]string{
				"IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST": "0",
				"IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC":   "-1",
				"IPTV_TUNERR_HTTP_MAX_IDLE_CONNS":          "nope",
			},
			wantPerH: MaxIdleConnsPerHost,
			wantIdle: DefaultIdleConnTimeout,
			wantMaxI: defaultMaxIdleConns,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			perH, idle, maxI := parseSharedTransportEnv()
			if perH != tc.wantPerH || idle != tc.wantIdle || maxI != tc.wantMaxI {
				t.Fatalf("got perHost=%d idle=%v maxIdle=%d want perHost=%d idle=%v maxIdle=%d",
					perH, idle, maxI, tc.wantPerH, tc.wantIdle, tc.wantMaxI)
			}
		})
	}
}

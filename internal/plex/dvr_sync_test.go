package plex

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- helpers ----------------------------------------------------------------

func xmlResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// serverFor builds a fake Plex server from a mux map and returns its host:port.
func serverFor(mux http.Handler) (host string, close func()) {
	ts := httptest.NewServer(mux)
	host = strings.TrimPrefix(ts.URL, "http://")
	return host, ts.Close
}

// --- InstancesFromSupervisorConfig ------------------------------------------

func TestInstancesFromSupervisorConfig_basic(t *testing.T) {
	cfg := map[string]any{
		"instances": []map[string]any{
			{
				"name": "sports",
				"env": map[string]string{
					"PLEX_TUNER_BASE_URL":      "http://sports:5004",
					"PLEX_TUNER_DEVICE_ID":     "SPORTS01",
					"PLEX_TUNER_FRIENDLY_NAME": "Sports",
				},
			},
			{
				"name": "hdhr-wizard",
				"env": map[string]string{
					"PLEX_TUNER_BASE_URL":          "http://hdhr:5004",
					"PLEX_TUNER_DEVICE_ID":         "HDHR01",
					"PLEX_TUNER_HDHR_NETWORK_MODE": "true",
				},
			},
			{
				"name": "canada",
				"env": map[string]string{
					"PLEX_TUNER_BASE_URL":  "http://canada:5004",
					"PLEX_TUNER_DEVICE_ID": "CANADA01",
				},
			},
			{
				"name": "nodevid",
				"env": map[string]string{
					"PLEX_TUNER_BASE_URL": "http://nodevid:5004",
				},
			},
		},
	}
	raw, _ := json.Marshal(cfg)
	insts, err := InstancesFromSupervisorConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// hdhr-wizard and nodevid should be skipped.
	if len(insts) != 2 {
		t.Fatalf("expected 2 instances, got %d: %+v", len(insts), insts)
	}
	if insts[0].DeviceID != "SPORTS01" {
		t.Errorf("inst[0].DeviceID = %q", insts[0].DeviceID)
	}
	if insts[1].DeviceID != "CANADA01" {
		t.Errorf("inst[1].DeviceID = %q", insts[1].DeviceID)
	}
}

func TestInstancesFromSupervisorConfig_invalid(t *testing.T) {
	_, err := InstancesFromSupervisorConfig([]byte("not-json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- lineupIDsForDVR --------------------------------------------------------

func TestLineupIDsForDVR(t *testing.T) {
	lineup := "lineup://tv.plex.providers.epg.xmltv/http%3A%2F%2Fsports%3A5004%2Fguide.xml#Sports"
	mux := http.NewServeMux()
	mux.HandleFunc("/livetv/dvrs/42", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, `<MediaContainer><Dvr lineup="`+lineup+`"/></MediaContainer>`)
	})
	host, closeFn := serverFor(mux)
	defer closeFn()

	ids := lineupIDsForDVR(host, "tok", 42)
	if len(ids) != 1 || ids[0] != lineup {
		t.Errorf("lineupIDsForDVR = %v, want [%s]", ids, lineup)
	}
}

func TestLineupIDsForDVR_empty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/livetv/dvrs/99", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, `<MediaContainer><Dvr lineup=""/></MediaContainer>`)
	})
	host, closeFn := serverFor(mux)
	defer closeFn()

	ids := lineupIDsForDVR(host, "tok", 99)
	if len(ids) != 0 {
		t.Errorf("expected empty lineup IDs, got %v", ids)
	}
}

// --- ReconcileDVRs dry-run --------------------------------------------------

func TestReconcileDVRs_dryRun_create(t *testing.T) {
	// Plex reports no existing devices or DVRs.
	mux := http.NewServeMux()
	mux.HandleFunc("/media/grabbers/devices", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, `<MediaContainer></MediaContainer>`)
	})
	mux.HandleFunc("/livetv/dvrs", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, `<MediaContainer></MediaContainer>`)
	})
	host, closeFn := serverFor(mux)
	defer closeFn()

	results := ReconcileDVRs(context.Background(), DVRSyncConfig{
		PlexHost: host,
		Token:    "tok",
		Instances: []DVRSyncInstance{
			{Name: "sports", BaseURL: "http://sports:5004", DeviceID: "SPORTS01"},
		},
		DryRun:            true,
		GuideWaitDuration: 1 * time.Millisecond,
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != "created" {
		t.Errorf("action = %q, want 'created'", results[0].Action)
	}
	if results[0].Err != nil {
		t.Errorf("unexpected error: %v", results[0].Err)
	}
}

func TestReconcileDVRs_dryRun_refreshed(t *testing.T) {
	// Plex already has a device and DVR for this instance.
	type xmlDvr struct {
		Key       string `xml:"key,attr"`
		DeviceKey string `xml:"deviceKey,attr"`
	}
	type xmlDev struct {
		Key      string `xml:"key,attr"`
		UUID     string `xml:"uuid,attr"`
		URI      string `xml:"uri,attr"`
		DeviceID string `xml:"deviceId,attr"`
	}
	devXML, _ := xml.Marshal(struct {
		XMLName xml.Name `xml:"MediaContainer"`
		Device  xmlDev   `xml:"Device"`
	}{Device: xmlDev{Key: "dev1", UUID: "u1", URI: "http://sports:5004", DeviceID: "SPORTS01"}})
	dvrXML, _ := xml.Marshal(struct {
		XMLName xml.Name `xml:"MediaContainer"`
		Dvr     xmlDvr   `xml:"Dvr"`
	}{Dvr: xmlDvr{Key: "7", DeviceKey: "dev1"}})

	mux := http.NewServeMux()
	mux.HandleFunc("/media/grabbers/devices", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, string(devXML))
	})
	mux.HandleFunc("/livetv/dvrs", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, string(dvrXML))
	})
	host, closeFn := serverFor(mux)
	defer closeFn()

	results := ReconcileDVRs(context.Background(), DVRSyncConfig{
		PlexHost: host,
		Token:    "tok",
		Instances: []DVRSyncInstance{
			{Name: "sports", BaseURL: "http://sports:5004", DeviceID: "SPORTS01"},
		},
		DryRun:            true,
		GuideWaitDuration: 1 * time.Millisecond,
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != "refreshed" {
		t.Errorf("action = %q, want 'refreshed'", results[0].Action)
	}
}

func TestReconcileDVRs_dryRun_deleteUnknown(t *testing.T) {
	// Plex has a stale injected DVR not in desired set, plus desired DVR.
	type xmlDev struct {
		Key      string `xml:"key,attr"`
		UUID     string `xml:"uuid,attr"`
		URI      string `xml:"uri,attr"`
		DeviceID string `xml:"deviceId,attr"`
		Make     string `xml:"make,attr"`
	}
	type xmlDvr struct {
		Key       string `xml:"key,attr"`
		DeviceKey string `xml:"deviceKey,attr"`
	}
	type container struct {
		XMLName xml.Name `xml:"MediaContainer"`
		Devices []xmlDev `xml:"Device"`
		Dvrs    []xmlDvr `xml:"Dvr"`
	}

	devXML, _ := xml.Marshal(container{Devices: []xmlDev{
		{Key: "dev-wanted", UUID: "u1", URI: "http://sports:5004", DeviceID: "SPORTS01"},
		{Key: "dev-stale", UUID: "u2", URI: "http://old:5004", DeviceID: "OLD01", Make: "Unknown"},
	}})
	dvrXML, _ := xml.Marshal(container{Dvrs: []xmlDvr{
		{Key: "7", DeviceKey: "dev-wanted"},
		{Key: "8", DeviceKey: "dev-stale"},
	}})

	mux := http.NewServeMux()
	mux.HandleFunc("/media/grabbers/devices", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, string(devXML))
	})
	mux.HandleFunc("/livetv/dvrs", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, string(dvrXML))
	})
	host, closeFn := serverFor(mux)
	defer closeFn()

	results := ReconcileDVRs(context.Background(), DVRSyncConfig{
		PlexHost: host,
		Token:    "tok",
		Instances: []DVRSyncInstance{
			{Name: "sports", BaseURL: "http://sports:5004", DeviceID: "SPORTS01"},
		},
		DeleteUnknown:     true,
		DryRun:            true,
		GuideWaitDuration: 1 * time.Millisecond,
	})

	actions := map[string]int{}
	for _, r := range results {
		actions[r.Action]++
	}
	if actions["refreshed"] != 1 {
		t.Errorf("expected 1 refreshed, got %d", actions["refreshed"])
	}
	if actions["deleted"] != 1 {
		t.Errorf("expected 1 deleted, got %d", actions["deleted"])
	}
}

func TestReconcileDVRs_contextCancelled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/media/grabbers/devices", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, `<MediaContainer></MediaContainer>`)
	})
	mux.HandleFunc("/livetv/dvrs", func(w http.ResponseWriter, r *http.Request) {
		xmlResponse(w, `<MediaContainer></MediaContainer>`)
	})
	host, closeFn := serverFor(mux)
	defer closeFn()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := ReconcileDVRs(ctx, DVRSyncConfig{
		PlexHost: host,
		Token:    "tok",
		Instances: []DVRSyncInstance{
			{Name: "sports", BaseURL: "http://sports:5004", DeviceID: "SPORTS01"},
		},
		DryRun:            true,
		GuideWaitDuration: 1 * time.Millisecond,
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

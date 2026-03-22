package tuner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunGhostHunterClassifiesStaleSessionAfterObservation(t *testing.T) {
	var calls atomic.Int32
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		n := calls.Add(1)
		maxOffset := "10.0"
		transTS := "100"
		if n >= 2 {
			maxOffset = "10.0"
			transTS = "100"
		}
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="1">
  <Video title="Live A" key="/livetv/sessions/live-1">
    <Player address="192.168.1.10" product="Plex Web" platform="Chrome" device="Linux" machineIdentifier="m1" state="playing"/>
    <Session id="sess-1"/>
    <TranscodeSession key="/transcode/sessions/tx-1" timeStamp="` + transTS + `" maxOffsetAvailable="` + maxOffset + `" minOffsetAvailable="1.0"/>
  </Video>
</MediaContainer>`))
	}))
	defer pms.Close()

	rep, err := RunGhostHunter(context.Background(), GhostHunterConfig{
		PMSURL:        pms.URL,
		Token:         "tok",
		PollInterval:  20 * time.Millisecond,
		ObserveWindow: 50 * time.Millisecond,
		IdleTimeout:   10 * time.Millisecond,
		RenewLease:    10 * time.Millisecond,
		HardLease:     time.Minute,
	}, false, pms.Client())
	if err != nil {
		t.Fatalf("RunGhostHunter: %v", err)
	}
	if rep.SessionCount != 1 {
		t.Fatalf("session_count=%d want 1", rep.SessionCount)
	}
	if rep.StaleCount != 1 {
		t.Fatalf("stale_count=%d want 1", rep.StaleCount)
	}
	if rep.Sessions[0].Status != "stale" {
		t.Fatalf("status=%q want stale", rep.Sessions[0].Status)
	}
	if rep.RecoveryCommand == "" {
		t.Fatalf("expected recovery command for stale visible sessions")
	}
	if len(rep.SafeActions) == 0 {
		t.Fatalf("expected safe actions for stale visible sessions")
	}
}

func TestServer_ghostHunterReport(t *testing.T) {
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="1">
  <Video title="Live A" key="/livetv/sessions/live-1">
    <Player address="192.168.1.10" product="Plex Web" platform="Chrome" device="Linux" machineIdentifier="m1" state="playing"/>
    <Session id="sess-1"/>
    <TranscodeSession key="/transcode/sessions/tx-1" timeStamp="123.4" maxOffsetAvailable="17.5" minOffsetAvailable="1.0"/>
  </Video>
</MediaContainer>`))
	}))
	defer pms.Close()

	t.Setenv("IPTV_TUNERR_PMS_URL", pms.URL)
	t.Setenv("IPTV_TUNERR_PMS_TOKEN", "tok")
	t.Setenv("IPTV_TUNERR_PLEX_SESSION_REAPER_IDLE_S", "15")
	t.Setenv("IPTV_TUNERR_PLEX_SESSION_REAPER_RENEW_LEASE_S", "20")
	t.Setenv("IPTV_TUNERR_PLEX_SESSION_REAPER_HARD_LEASE_S", "1800")
	t.Setenv("IPTV_TUNERR_PLEX_SESSION_REAPER_POLL_S", "1")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/plex/ghost-report.json?observe=0s", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveGhostHunterReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body GhostHunterReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.SessionCount != 1 {
		t.Fatalf("session_count=%d want 1", body.SessionCount)
	}
	if len(body.Notes) == 0 {
		t.Fatalf("expected snapshot note")
	}
}

func TestRunGhostHunterEscalatesWhenNoVisibleSessions(t *testing.T) {
	pms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><MediaContainer size="0"></MediaContainer>`))
	}))
	defer pms.Close()

	rep, err := RunGhostHunter(context.Background(), GhostHunterConfig{
		PMSURL:        pms.URL,
		Token:         "tok",
		PollInterval:  20 * time.Millisecond,
		ObserveWindow: 40 * time.Millisecond,
		IdleTimeout:   10 * time.Second,
		RenewLease:    20 * time.Second,
		HardLease:     time.Minute,
	}, false, pms.Client())
	if err != nil {
		t.Fatalf("RunGhostHunter: %v", err)
	}
	if !rep.HiddenGrabSuspected {
		t.Fatalf("expected hidden_grab_suspected=true")
	}
	if rep.RecoveryCommand == "" {
		t.Fatalf("expected recovery command")
	}
	if rep.Runbook == "" {
		t.Fatalf("expected runbook")
	}
	if len(rep.SafeActions) == 0 {
		t.Fatalf("expected safe actions")
	}
}

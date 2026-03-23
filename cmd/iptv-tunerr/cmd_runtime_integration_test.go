package main

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
)

func TestRuntimeCommandHelperProcess(t *testing.T) {
	if os.Getenv("IPTV_TUNERR_HELPER_PROCESS") != "1" {
		return
	}

	cfg := &config.Config{
		CatalogPath:  os.Getenv("IPTV_TUNERR_HELPER_CATALOG"),
		TunerCount:   1,
		DeviceID:     "test-device",
		FriendlyName: "Test Tuner",
	}
	if m3uURL := os.Getenv("IPTV_TUNERR_HELPER_M3U_URL"); m3uURL != "" {
		cfg.M3UURL = m3uURL
	}

	switch os.Getenv("IPTV_TUNERR_HELPER_MODE") {
	case "handle-serve":
		addr := os.Getenv("IPTV_TUNERR_HELPER_ADDR")
		baseURL := os.Getenv("IPTV_TUNERR_HELPER_BASEURL")
		handleServe(cfg, cfg.CatalogPath, addr, baseURL, cfg.DeviceID, cfg.FriendlyName, "full")
	case "handle-run-success":
		addr := os.Getenv("IPTV_TUNERR_HELPER_ADDR")
		baseURL := os.Getenv("IPTV_TUNERR_HELPER_BASEURL")
		handleRun(cfg, cfg.CatalogPath, addr, baseURL, cfg.DeviceID, cfg.FriendlyName, 0, true, true, "", true, 0, "", "full", false, false, 0, 0, "", "")
	case "handle-run-refresh":
		addr := os.Getenv("IPTV_TUNERR_HELPER_ADDR")
		baseURL := os.Getenv("IPTV_TUNERR_HELPER_BASEURL")
		handleRun(cfg, cfg.CatalogPath, addr, baseURL, cfg.DeviceID, cfg.FriendlyName, 0, false, true, "", true, 0, "", "full", false, false, 0, 0, "", "")
	case "handle-run-missing":
		addr := os.Getenv("IPTV_TUNERR_HELPER_ADDR")
		baseURL := os.Getenv("IPTV_TUNERR_HELPER_BASEURL")
		handleRun(cfg, cfg.CatalogPath, addr, baseURL, cfg.DeviceID, cfg.FriendlyName, 0, true, true, "", true, 0, "", "full", false, false, 0, 0, "", "")
	}
	os.Exit(0)
}

func writeTestCatalog(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalog.json")
	c := catalog.New()
	c.ReplaceWithLive(
		[]catalog.Movie{{ID: "m1", Title: "Movie 1", Year: 2024, StreamURL: "http://example.com/movie1.mp4"}},
		nil,
		[]catalog.LiveChannel{{ChannelID: "c1", GuideNumber: "101", GuideName: "Channel 1", StreamURL: "http://example.com/live1.m3u8"}},
	)
	if err := c.Save(path); err != nil {
		t.Fatalf("save catalog: %v", err)
	}
	return path
}

func reserveLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func TestHandleServe_ServesDiscoverJSON(t *testing.T) {
	catalogPath := writeTestCatalog(t)
	addr := reserveLocalAddr(t)
	baseURL := "http://" + addr

	cmd := exec.Command(os.Args[0], "-test.run=TestRuntimeCommandHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=handle-serve",
		"IPTV_TUNERR_HELPER_CATALOG="+catalogPath,
		"IPTV_TUNERR_HELPER_ADDR="+addr,
		"IPTV_TUNERR_HELPER_BASEURL="+baseURL,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_, _ = cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	var resp *http.Response
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		r, err := client.Get(baseURL + "/discover.json")
		if err == nil {
			resp = r
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if resp == nil {
		t.Fatal("handleServe helper never became reachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
}

func TestHandleRun_RegisterOnlySuccess(t *testing.T) {
	catalogPath := writeTestCatalog(t)
	addr := reserveLocalAddr(t)
	baseURL := "http://" + addr

	cmd := exec.Command(os.Args[0], "-test.run=TestRuntimeCommandHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=handle-run-success",
		"IPTV_TUNERR_HELPER_CATALOG="+catalogPath,
		"IPTV_TUNERR_HELPER_ADDR="+addr,
		"IPTV_TUNERR_HELPER_BASEURL="+baseURL,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("handleRun registerOnly success: %v", err)
	}
}

func TestHandleRun_RefreshesCatalogFromDirectM3U(t *testing.T) {
	m3u := "#EXTM3U\n#EXTINF:-1 tvg-id=\"cbc.ca\",CBC\nhttp://example.com/cbc.m3u8\n"
	m3uHTTP := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(m3u))
	})
	testSrv := &http.Server{Handler: m3uHTTP}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen m3u: %v", err)
	}
	go func() { _ = testSrv.Serve(ln) }()
	defer func() { _ = testSrv.Close() }()

	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	addr := reserveLocalAddr(t)
	baseURL := "http://" + addr

	cmd := exec.Command(os.Args[0], "-test.run=TestRuntimeCommandHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=handle-run-refresh",
		"IPTV_TUNERR_HELPER_CATALOG="+catalogPath,
		"IPTV_TUNERR_HELPER_ADDR="+addr,
		"IPTV_TUNERR_HELPER_BASEURL="+baseURL,
		"IPTV_TUNERR_HELPER_M3U_URL=http://"+ln.Addr().String(),
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("handleRun refresh: %v", err)
	}
	c := catalog.New()
	if err := c.Load(catalogPath); err != nil {
		t.Fatalf("load refreshed catalog: %v", err)
	}
	live := c.SnapshotLive()
	if len(live) != 1 || live[0].TVGID != "cbc.ca" {
		t.Fatalf("unexpected refreshed live=%+v", live)
	}
}

func TestHandleRun_MissingCatalogExits(t *testing.T) {
	addr := reserveLocalAddr(t)
	baseURL := "http://" + addr
	cmd := exec.Command(os.Args[0], "-test.run=TestRuntimeCommandHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=handle-run-missing",
		"IPTV_TUNERR_HELPER_CATALOG="+filepath.Join(t.TempDir(), "missing-catalog.json"),
		"IPTV_TUNERR_HELPER_ADDR="+addr,
		"IPTV_TUNERR_HELPER_BASEURL="+baseURL,
	)
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit error, got %v", err)
	}
}

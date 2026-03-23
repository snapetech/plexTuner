package main

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
)

func TestVODCommandHelperProcess(t *testing.T) {
	if os.Getenv("IPTV_TUNERR_HELPER_PROCESS") != "1" {
		return
	}
	switch os.Getenv("IPTV_TUNERR_HELPER_MODE") {
	case "vod-webdav-serve":
		cfg := &config.Config{CatalogPath: os.Getenv("IPTV_TUNERR_HELPER_CATALOG")}
		handleVODWebDAV(cfg, os.Getenv("IPTV_TUNERR_HELPER_CATALOG"), os.Getenv("IPTV_TUNERR_HELPER_ADDR"), "")
	case "vod-webdav-missing":
		cfg := &config.Config{}
		handleVODWebDAV(cfg, os.Getenv("IPTV_TUNERR_HELPER_CATALOG"), os.Getenv("IPTV_TUNERR_HELPER_ADDR"), "")
	case "mount-missing":
		cfg := &config.Config{}
		handleMount(cfg, os.Getenv("IPTV_TUNERR_HELPER_CATALOG"), os.Getenv("IPTV_TUNERR_HELPER_MOUNT"), "", false)
	}
	os.Exit(0)
}

func TestHandleVODWebDAV_ServesReadOnlyDAV(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	c := catalog.New()
	c.ReplaceWithLive(
		[]catalog.Movie{{ID: "m1", Title: "Movie 1", Year: 2024, StreamURL: "http://example.com/movie1.mp4"}},
		[]catalog.Series{{ID: "s1", Title: "Series 1", Year: 2024, Seasons: []catalog.Season{{Number: 1, Episodes: []catalog.Episode{{ID: "e1", SeasonNum: 1, EpisodeNum: 1, Title: "Pilot", StreamURL: "http://example.com/e1.mp4"}}}}}},
		nil,
	)
	if err := c.Save(catalogPath); err != nil {
		t.Fatalf("save catalog: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cmd := exec.Command(os.Args[0], "-test.run=TestVODCommandHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=vod-webdav-serve",
		"IPTV_TUNERR_HELPER_CATALOG="+catalogPath,
		"IPTV_TUNERR_HELPER_ADDR="+addr,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	var resp *http.Response
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		req, err := http.NewRequest(http.MethodOptions, "http://"+addr+"/", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if resp == nil {
		t.Fatal("VOD WebDAV helper never became reachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("DAV"); got == "" {
		t.Fatal("missing DAV header")
	}
	if got := resp.Header.Get("Allow"); got == "" {
		t.Fatal("missing Allow header")
	}
}

func TestHandleVODWebDAV_MissingCatalogExits(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestVODCommandHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=vod-webdav-missing",
		"IPTV_TUNERR_HELPER_CATALOG="+filepath.Join(t.TempDir(), "missing-catalog.json"),
		"IPTV_TUNERR_HELPER_ADDR=127.0.0.1:0",
	)
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit error, got %v", err)
	}
}

func TestHandleMount_MissingCatalogExits(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestVODCommandHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=mount-missing",
		"IPTV_TUNERR_HELPER_CATALOG="+filepath.Join(t.TempDir(), "missing-catalog.json"),
		"IPTV_TUNERR_HELPER_MOUNT="+filepath.Join(t.TempDir(), "mnt"),
	)
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit error, got %v", err)
	}
}

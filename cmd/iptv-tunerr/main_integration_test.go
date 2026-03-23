package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestMainHelperProcess(t *testing.T) {
	if os.Getenv("IPTV_TUNERR_HELPER_PROCESS") != "1" {
		return
	}
	mode := os.Getenv("IPTV_TUNERR_HELPER_MODE")
	var args []string
	switch mode {
	case "main-run":
		args = []string{
			"iptv-tunerr",
			"run",
			"-catalog", os.Getenv("IPTV_TUNERR_HELPER_CATALOG"),
			"-addr", os.Getenv("IPTV_TUNERR_HELPER_ADDR"),
			"-base-url", os.Getenv("IPTV_TUNERR_HELPER_BASEURL"),
			"-register-only",
			"-skip-health",
		}
		if os.Getenv("IPTV_TUNERR_HELPER_SKIP_INDEX") == "1" {
			args = append(args, "-skip-index")
		}
	case "main-index":
		args = []string{
			"iptv-tunerr",
			"index",
			"-catalog", os.Getenv("IPTV_TUNERR_HELPER_CATALOG"),
		}
	case "main-version":
		args = []string{"iptv-tunerr", "version"}
	case "main-help":
		args = []string{"iptv-tunerr", "--help"}
	case "main-noargs":
		args = []string{"iptv-tunerr"}
	case "main-unknown":
		args = []string{"iptv-tunerr", "definitely-not-a-real-command"}
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
	os.Args = args
	main()
	os.Exit(0)
}

func TestMain_RunCommandDispatchesSuccessfully(t *testing.T) {
	catalogPath := writeTestCatalog(t)
	addr := reserveLocalAddr(t)
	baseURL := "http://" + addr

	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=main-run",
		"IPTV_TUNERR_HELPER_CATALOG="+catalogPath,
		"IPTV_TUNERR_HELPER_ADDR="+addr,
		"IPTV_TUNERR_HELPER_BASEURL="+baseURL,
		"IPTV_TUNERR_HELPER_SKIP_INDEX=1",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("main run dispatch: %v", err)
	}
}

func TestMain_RunCommandRefreshesFromM3UEnv(t *testing.T) {
	m3uSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"fox.us\",FOX\nhttp://example.com/fox.m3u8\n"))
	}))
	defer m3uSrv.Close()
	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	addr := reserveLocalAddr(t)
	baseURL := "http://" + addr

	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=main-run",
		"IPTV_TUNERR_HELPER_CATALOG="+catalogPath,
		"IPTV_TUNERR_HELPER_ADDR="+addr,
		"IPTV_TUNERR_HELPER_BASEURL="+baseURL,
		"IPTV_TUNERR_M3U_URL="+m3uSrv.URL,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("main run refresh from m3u env: %v", err)
	}
	c := catalog.New()
	if err := c.Load(catalogPath); err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	live := c.SnapshotLive()
	if len(live) != 1 || live[0].TVGID != "fox.us" {
		t.Fatalf("unexpected live=%+v", live)
	}
}

func TestMain_IndexCommandDispatchesSuccessfully(t *testing.T) {
	m3uSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"cnn.us\",CNN\nhttp://example.com/cnn.m3u8\n"))
	}))
	defer m3uSrv.Close()

	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=main-index",
		"IPTV_TUNERR_HELPER_CATALOG="+catalogPath,
		"IPTV_TUNERR_M3U_URL="+m3uSrv.URL,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("main index dispatch: %v", err)
	}
	c := catalog.New()
	if err := c.Load(catalogPath); err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	live := c.SnapshotLive()
	if len(live) != 1 || live[0].TVGID != "cnn.us" {
		t.Fatalf("unexpected live=%+v", live)
	}
}

func TestMain_VersionCommand(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=main-version",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("main version: %v output=%s", err, string(out))
	}
	if string(out) == "" {
		t.Fatal("expected version output")
	}
}

func TestMain_HelpExitsZero(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=main-help",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("main help: %v output=%s", err, string(out))
	}
	if len(out) == 0 {
		t.Fatal("expected help output")
	}
}

func TestMain_NoArgsExitsNonZero(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=main-noargs",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if len(out) == 0 {
		t.Fatal("expected usage output")
	}
}

func TestMain_UnknownCommandExitsNonZero(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
	cmd.Env = append(os.Environ(),
		"IPTV_TUNERR_HELPER_PROCESS=1",
		"IPTV_TUNERR_HELPER_MODE=main-unknown",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if len(out) == 0 {
		t.Fatal("expected unknown-command output")
	}
}

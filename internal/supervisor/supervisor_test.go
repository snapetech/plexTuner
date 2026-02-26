package supervisor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAndMergeEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "multi.json")
	if err := os.WriteFile(p, []byte(`{
  "restart": true,
  "restartDelay": "3s",
  "instances": [
    {
      "name": "newsus",
      "args": ["run","-mode=easy","-addr=:5004","-catalog=/data/newsus/catalog.json"],
      "env": {"PLEX_TUNER_BASE_URL":"http://plextuner-newsus:5004","TZ":"UTC"}
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("LoadConfig err=%v", err)
	}
	if len(cfg.Instances) != 1 || cfg.Instances[0].Name != "newsus" {
		t.Fatalf("unexpected instances: %+v", cfg.Instances)
	}
	if got := cfg.RestartDelay.Duration(0).String(); got != "3s" {
		t.Fatalf("restartDelay=%s want 3s", got)
	}
	env := mergedEnv([]string{"A=1", "TZ=America/Chicago"}, map[string]string{"TZ": "UTC", "B": "2"})
	want := map[string]string{"A": "1", "TZ": "UTC", "B": "2"}
	for _, kv := range env {
		k, v, ok := splitEnvKV(kv)
		if !ok {
			continue
		}
		if wantV, ok := want[k]; ok && v != wantV {
			t.Fatalf("%s=%s want %s", k, v, wantV)
		}
	}
}

func TestLoadConfigRejectsDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dup.json")
	if err := os.WriteFile(p, []byte(`{"instances":[{"name":"x","args":["run"]},{"name":"x","args":["run"]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(p); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestMergedEnvStripsParentPlexReaperEnvForChildren(t *testing.T) {
	base := []string{
		"A=1",
		"PLEX_TUNER_PLEX_SESSION_REAPER=1",
		"PLEX_TUNER_PLEX_SESSION_REAPER_IDLE_S=15",
		"PLEX_TUNER_PMS_URL=http://plex:32400",
		"PLEX_TUNER_PMS_TOKEN=secret",
		"TZ=UTC",
	}
	out := mergedEnv(base, map[string]string{
		"PLEX_TUNER_BASE_URL": "http://child:5004",
		"TZ":                  "America/Regina",
	})
	got := map[string]string{}
	for _, kv := range out {
		k, v, ok := splitEnvKV(kv)
		if ok {
			got[k] = v
		}
	}
	if _, ok := got["PLEX_TUNER_PLEX_SESSION_REAPER"]; ok {
		t.Fatalf("reaper env should not be inherited by children: %+v", got)
	}
	if _, ok := got["PLEX_TUNER_PMS_URL"]; ok {
		t.Fatalf("pms url should not be inherited by children: %+v", got)
	}
	if got["A"] != "1" || got["PLEX_TUNER_BASE_URL"] != "http://child:5004" || got["TZ"] != "America/Regina" {
		t.Fatalf("unexpected merged env: %+v", got)
	}
}

func splitEnvKV(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

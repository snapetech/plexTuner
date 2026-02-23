package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestM3UURLOrBuild(t *testing.T) {
	os.Clearenv()
	os.Setenv("PLEX_TUNER_PROVIDER_URL", "http://host")
	os.Setenv("PLEX_TUNER_PROVIDER_USER", "u")
	os.Setenv("PLEX_TUNER_PROVIDER_PASS", "p")
	c := Load()
	got := c.M3UURLOrBuild()
	want := "http://host/get.php?username=u&password=p&type=m3u_plus&output=ts"
	if got != want {
		t.Errorf("M3UURLOrBuild() = %q, want %q", got, want)
	}
}

func TestM3UURLOrBuild_preferM3UURL(t *testing.T) {
	os.Clearenv()
	os.Setenv("PLEX_TUNER_M3U_URL", "http://custom/m3u")
	os.Setenv("PLEX_TUNER_PROVIDER_URL", "http://host")
	c := Load()
	got := c.M3UURLOrBuild()
	if got != "http://custom/m3u" {
		t.Errorf("should prefer M3U_URL; got %q", got)
	}
}

func TestM3UURLOrBuild_emptyWithoutCreds(t *testing.T) {
	os.Clearenv()
	c := Load()
	got := c.M3UURLOrBuild()
	if got != "" {
		t.Errorf("no creds should give empty; got %q", got)
	}
}

func TestM3UURLsOrBuild_single(t *testing.T) {
	os.Clearenv()
	os.Setenv("PLEX_TUNER_PROVIDER_URL", "http://host")
	os.Setenv("PLEX_TUNER_PROVIDER_USER", "u")
	os.Setenv("PLEX_TUNER_PROVIDER_PASS", "p")
	c := Load()
	urls := c.M3UURLsOrBuild()
	if len(urls) != 1 {
		t.Fatalf("M3UURLsOrBuild() len = %d, want 1", len(urls))
	}
	want := "http://host/get.php?username=u&password=p&type=m3u_plus&output=ts"
	if urls[0] != want {
		t.Errorf("M3UURLsOrBuild()[0] = %q, want %q", urls[0], want)
	}
}

func TestM3UURLsOrBuild_multiple(t *testing.T) {
	os.Clearenv()
	os.Setenv("PLEX_TUNER_PROVIDER_URLS", "http://a.com, http://b.com ")
	os.Setenv("PLEX_TUNER_PROVIDER_USER", "u")
	os.Setenv("PLEX_TUNER_PROVIDER_PASS", "p")
	c := Load()
	urls := c.M3UURLsOrBuild()
	if len(urls) != 2 {
		t.Fatalf("M3UURLsOrBuild() len = %d, want 2", len(urls))
	}
	if urls[0] != "http://a.com/get.php?username=u&password=p&type=m3u_plus&output=ts" {
		t.Errorf("first URL: %q", urls[0])
	}
	if urls[1] != "http://b.com/get.php?username=u&password=p&type=m3u_plus&output=ts" {
		t.Errorf("second URL: %q", urls[1])
	}
}

func TestM3UURLsOrBuild_preferM3UURL(t *testing.T) {
	os.Clearenv()
	os.Setenv("PLEX_TUNER_M3U_URL", "http://custom/m3u")
	os.Setenv("PLEX_TUNER_PROVIDER_URLS", "http://a.com,http://b.com")
	c := Load()
	urls := c.M3UURLsOrBuild()
	if len(urls) != 1 || urls[0] != "http://custom/m3u" {
		t.Errorf("M3U_URL should be sole entry; got %v", urls)
	}
}

func TestProviderURLs(t *testing.T) {
	os.Clearenv()
	os.Setenv("PLEX_TUNER_PROVIDER_URLS", "http://x.com, http://y.com")
	c := Load()
	got := c.ProviderURLs()
	if len(got) != 2 || got[0] != "http://x.com" || got[1] != "http://y.com" {
		t.Errorf("ProviderURLs() = %v", got)
	}
	os.Clearenv()
	os.Setenv("PLEX_TUNER_PROVIDER_URL", "http://single")
	c = Load()
	got = c.ProviderURLs()
	if len(got) != 1 || got[0] != "http://single" {
		t.Errorf("ProviderURLs() fallback = %v", got)
	}
}

// When only user/pass are set, ProviderURLs returns DefaultProviderHosts (inline with xtream-to-m3u.js).
func TestProviderURLs_defaultHostsWhenUserPassOnly(t *testing.T) {
	os.Clearenv()
	os.Setenv("PLEX_TUNER_PROVIDER_USER", "u")
	os.Setenv("PLEX_TUNER_PROVIDER_PASS", "p")
	c := Load()
	got := c.ProviderURLs()
	if len(got) != len(DefaultProviderHosts) {
		t.Errorf("ProviderURLs() len = %d, want %d (DefaultProviderHosts)", len(got), len(DefaultProviderHosts))
	}
	if len(got) > 0 && got[0] != DefaultProviderHosts[0] {
		t.Errorf("ProviderURLs()[0] = %q, want %q", got[0], DefaultProviderHosts[0])
	}
}

func TestLiveEPGOnly(t *testing.T) {
	os.Clearenv()
	c := Load()
	if c.LiveEPGOnly {
		t.Error("LiveEPGOnly should default false")
	}
	os.Setenv("PLEX_TUNER_LIVE_EPG_ONLY", "1")
	c = Load()
	if !c.LiveEPGOnly {
		t.Error("LiveEPGOnly should be true for 1")
	}
	os.Setenv("PLEX_TUNER_LIVE_EPG_ONLY", "true")
	c = Load()
	if !c.LiveEPGOnly {
		t.Error("LiveEPGOnly should be true for true")
	}
	os.Setenv("PLEX_TUNER_LIVE_EPG_ONLY", "no")
	c = Load()
	if c.LiveEPGOnly {
		t.Error("LiveEPGOnly should be false for no")
	}
}

func TestLiveOnly(t *testing.T) {
	os.Clearenv()
	c := Load()
	if c.LiveOnly {
		t.Error("LiveOnly should default false")
	}
	os.Setenv("PLEX_TUNER_LIVE_ONLY", "1")
	c = Load()
	if !c.LiveOnly {
		t.Error("LiveOnly should be true for 1")
	}
}

// Subscription file: Load fills ProviderUser/ProviderPass from file when env is empty.
func TestLoad_subscriptionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub.txt")
	if err := os.WriteFile(path, []byte("Username: myuser\nPassword: mypass\n"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Clearenv()
	os.Setenv("PLEX_TUNER_SUBSCRIPTION_FILE", path)
	c := Load()
	if c.ProviderUser != "myuser" || c.ProviderPass != "mypass" {
		t.Errorf("Load from subscription file: user=%q pass=%q", c.ProviderUser, c.ProviderPass)
	}
}

func TestLoad_subscriptionFile_missingPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub.txt")
	if err := os.WriteFile(path, []byte("Username: u\n"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Clearenv()
	os.Setenv("PLEX_TUNER_SUBSCRIPTION_FILE", path)
	c := Load()
	if c.ProviderUser != "" || c.ProviderPass != "" {
		t.Errorf("missing Password in file should leave creds empty; got user=%q pass=%q", c.ProviderUser, c.ProviderPass)
	}
}

func TestLoad_subscriptionFile_envOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub.txt")
	if err := os.WriteFile(path, []byte("Username: fileuser\nPassword: filepass\n"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Clearenv()
	os.Setenv("PLEX_TUNER_SUBSCRIPTION_FILE", path)
	os.Setenv("PLEX_TUNER_PROVIDER_USER", "envuser")
	c := Load()
	if c.ProviderUser != "envuser" {
		t.Errorf("env user should override; got %q", c.ProviderUser)
	}
	if c.ProviderPass != "filepass" {
		t.Errorf("pass should come from file when env pass empty; got %q", c.ProviderPass)
	}
}

func TestEpgPruneUnlinked(t *testing.T) {
	os.Clearenv()
	c := Load()
	if c.EpgPruneUnlinked {
		t.Error("EpgPruneUnlinked should default false")
	}
	os.Setenv("PLEX_TUNER_EPG_PRUNE_UNLINKED", "1")
	c = Load()
	if !c.EpgPruneUnlinked {
		t.Error("EpgPruneUnlinked should be true for 1")
	}
}

func TestSmoketestEnv(t *testing.T) {
	os.Clearenv()
	c := Load()
	if c.SmoketestEnabled {
		t.Error("SmoketestEnabled should default false")
	}
	if c.SmoketestTimeout != 8*time.Second {
		t.Errorf("SmoketestTimeout default: got %v", c.SmoketestTimeout)
	}
	if c.SmoketestConcurrency != 10 {
		t.Errorf("SmoketestConcurrency default: got %d", c.SmoketestConcurrency)
	}
	os.Setenv("PLEX_TUNER_SMOKETEST_ENABLED", "true")
	os.Setenv("PLEX_TUNER_SMOKETEST_TIMEOUT", "3s")
	os.Setenv("PLEX_TUNER_SMOKETEST_CONCURRENCY", "4")
	c = Load()
	if !c.SmoketestEnabled {
		t.Error("SmoketestEnabled should be true")
	}
	if c.SmoketestTimeout != 3*time.Second {
		t.Errorf("SmoketestTimeout: got %v", c.SmoketestTimeout)
	}
	if c.SmoketestConcurrency != 4 {
		t.Errorf("SmoketestConcurrency: got %d", c.SmoketestConcurrency)
	}
}

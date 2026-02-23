package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds tuner + VODFS + provider settings.
// Load from env and/or config file (future).
type Config struct {
	// Provider (M3U / Xtream)
	ProviderBaseURL string // e.g. http://provider:8080
	ProviderUser    string
	ProviderPass    string
	M3UURL          string // optional: full M3U URL if different from base

	// Paths
	MountPoint  string // e.g. /mnt/vodfs
	CacheDir    string // e.g. /var/cache/plextuner
	CatalogPath string // e.g. /var/lib/plextuner/catalog.json

	// Live tuner
	TunerCount int
	BaseURL    string // e.g. http://192.168.1.10:5004 for Plex to use
	DeviceID   string // HDHomeRun discover.json DeviceID (stable; some Plex versions are picky about format)
	// Stream: buffering absorbs brief upstream stalls; transcoding re-encodes (libx264/aac) for compatibility.
	StreamBufferBytes   int    // -1 = auto (default; adaptive when transcoding). 0 = no buffer. >0 = fixed bytes.
	StreamTranscodeMode string // "off" | "on" | "auto". auto = probe stream (ffprobe) and transcode only when codec not Plex-friendly.
	XMLTVURL            string // optional external XMLTV source to proxy/remap into /guide.xml
	XMLTVTimeout        time.Duration
	LiveEPGOnly         bool // if true, only include channels with tvg-id (EPG-linked) in catalog
	LiveOnly            bool // if true, only fetch live channels from API (skip VOD and series; faster)
	// EPG prune: when true, guide.xml and M3U export only include channels with tvg-id set (reduces noise).
	EpgPruneUnlinked bool
	// Stream smoketest: when true, at index time probe each channel's primary URL and drop failures.
	SmoketestEnabled     bool
	SmoketestTimeout     time.Duration
	SmoketestConcurrency int
	SmoketestMaxChannels int           // 0 = all; else sample up to N random channels to cap runtime
	SmoketestMaxDuration time.Duration // hard cap total smoketest runtime (e.g. 5m); 0 = 5m default
}

// Load reads config from environment. Call LoadEnvFile(".env") before Load() to use a .env file.
// If ProviderUser or ProviderPass are empty, Load tries PLEX_TUNER_SUBSCRIPTION_FILE (or default path) with "Username:" / "Password:" lines.
func Load() *Config {
	c := &Config{
		ProviderBaseURL:      os.Getenv("PLEX_TUNER_PROVIDER_URL"),
		ProviderUser:         os.Getenv("PLEX_TUNER_PROVIDER_USER"),
		ProviderPass:         os.Getenv("PLEX_TUNER_PROVIDER_PASS"),
		M3UURL:               os.Getenv("PLEX_TUNER_M3U_URL"),
		MountPoint:           getEnv("PLEX_TUNER_MOUNT", "/mnt/vodfs"),
		CacheDir:             getEnv("PLEX_TUNER_CACHE", "/var/cache/plextuner"),
		CatalogPath:          getEnv("PLEX_TUNER_CATALOG", "./catalog.json"),
		TunerCount:           getEnvInt("PLEX_TUNER_TUNER_COUNT", 2),
		BaseURL:              os.Getenv("PLEX_TUNER_BASE_URL"),
		DeviceID:             getEnv("PLEX_TUNER_DEVICE_ID", "plextuner01"),
		StreamBufferBytes:    getEnvIntOrAuto("PLEX_TUNER_STREAM_BUFFER_BYTES", -1),
		StreamTranscodeMode:  getEnvTranscodeMode("PLEX_TUNER_STREAM_TRANSCODE", "off"),
		XMLTVURL:             os.Getenv("PLEX_TUNER_XMLTV_URL"),
		XMLTVTimeout:         getEnvDuration("PLEX_TUNER_XMLTV_TIMEOUT", 45*time.Second),
		LiveEPGOnly:          getEnvBool("PLEX_TUNER_LIVE_EPG_ONLY", false),
		LiveOnly:             getEnvBool("PLEX_TUNER_LIVE_ONLY", false),
		EpgPruneUnlinked:     getEnvBool("PLEX_TUNER_EPG_PRUNE_UNLINKED", false),
		SmoketestEnabled:     getEnvBool("PLEX_TUNER_SMOKETEST_ENABLED", false),
		SmoketestTimeout:     getEnvDuration("PLEX_TUNER_SMOKETEST_TIMEOUT", 8*time.Second),
		SmoketestConcurrency: getEnvInt("PLEX_TUNER_SMOKETEST_CONCURRENCY", 10),
		SmoketestMaxChannels: getEnvInt("PLEX_TUNER_SMOKETEST_MAX_CHANNELS", 0),
		SmoketestMaxDuration: getEnvDuration("PLEX_TUNER_SMOKETEST_MAX_DURATION", 5*time.Minute),
	}
	if c.TunerCount <= 0 {
		c.TunerCount = 2
	}
	if c.SmoketestConcurrency <= 0 {
		c.SmoketestConcurrency = 10
	}
	if c.SmoketestMaxDuration <= 0 {
		c.SmoketestMaxDuration = 5 * time.Minute
	}
	if c.XMLTVTimeout <= 0 {
		c.XMLTVTimeout = 45 * time.Second
	}
	// Subscription file fallback (same pattern as k3s update-iptv-m3u.sh / iptv.subscription.2026.txt)
	if c.ProviderUser == "" || c.ProviderPass == "" {
		if user, pass, err := readSubscriptionFile(getEnv("PLEX_TUNER_SUBSCRIPTION_FILE", "")); err == nil {
			if c.ProviderUser == "" {
				c.ProviderUser = user
			}
			if c.ProviderPass == "" {
				c.ProviderPass = pass
			}
		}
	}
	return c
}

// readSubscriptionFile reads "Username: x" and "Password: x" from path. path may be empty to try default.
func readSubscriptionFile(path string) (user, pass string, err error) {
	if path == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return "", "", os.ErrNotExist
		}
		path = strings.TrimSuffix(home, "/") + "/Documents/iptv.subscription.2026.txt"
	}
	path = filepath.Clean(path)
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "Username:") {
			user = strings.TrimSpace(strings.TrimPrefix(line, "Username:"))
		} else if strings.HasPrefix(line, "Password:") {
			pass = strings.TrimSpace(strings.TrimPrefix(line, "Password:"))
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if user == "" || pass == "" {
		return "", "", fmt.Errorf("subscription file: missing Username or Password")
	}
	return user, pass, nil
}

// M3UURLOrBuild returns M3UURL if set, otherwise builds from ProviderBaseURL + user + pass.
func (c *Config) M3UURLOrBuild() string {
	urls := c.M3UURLsOrBuild()
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

// M3UURLsOrBuild returns a list of M3U URLs to probe: single PLEX_TUNER_M3U_URL if set,
// otherwise one URL per PLEX_TUNER_PROVIDER_URLS (or single ProviderBaseURL) with get.php.
func (c *Config) M3UURLsOrBuild() []string {
	if c.M3UURL != "" {
		return []string{c.M3UURL}
	}
	user, pass := c.ProviderUser, c.ProviderPass
	if user == "" || pass == "" {
		return nil
	}
	urls := c.ProviderURLs()
	if len(urls) == 0 {
		return nil
	}
	out := make([]string, 0, len(urls))
	for _, base := range urls {
		base = strings.TrimSuffix(base, "/")
		out = append(out, base+"/get.php?username="+url.QueryEscape(user)+"&password="+url.QueryEscape(pass)+"&type=m3u_plus&output=ts")
	}
	return out
}

// ProviderURLs returns all base URLs to try (PLEX_TUNER_PROVIDER_URLS comma-separated, or single PLEX_TUNER_PROVIDER_URL).
// Requires explicit URL(s); no default host list.
func (c *Config) ProviderURLs() []string {
	s := os.Getenv("PLEX_TUNER_PROVIDER_URLS")
	if s != "" {
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if c.ProviderBaseURL != "" {
		return []string{c.ProviderBaseURL}
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		n, _ := strconv.Atoi(v)
		return n
	}
	return defaultVal
}

// getEnvIntOrAuto returns -1 if env is "auto" or "-1", otherwise like getEnvInt.
func getEnvIntOrAuto(key string, defaultVal int) int {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "auto" || v == "-1" {
		return -1
	}
	if v != "" {
		n, _ := strconv.Atoi(v)
		return n
	}
	return defaultVal
}

// getEnvTranscodeMode returns "off", "on", or "auto" from PLEX_TUNER_STREAM_TRANSCODE.
func getEnvTranscodeMode(key string, defaultVal string) string {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "auto" {
		return "auto"
	}
	if v == "true" || v == "1" || v == "yes" {
		return "on"
	}
	if v == "false" || v == "0" || v == "no" || v == "" {
		return "off"
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}

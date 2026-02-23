package config

import (
	"net/url"
	"os"
	"strconv"
	"strings"
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
	TunerCount   int
	BaseURL      string // e.g. http://192.168.1.10:5004 for Plex to use
	LiveEPGOnly  bool   // if true, only include channels with tvg-id (EPG-linked) in catalog
	LiveOnly     bool   // if true, only fetch live channels from API (skip VOD and series; faster)
}

// Load reads config from environment. Call LoadEnvFile(".env") before Load() to use a .env file.
func Load() *Config {
	c := &Config{
		ProviderBaseURL: os.Getenv("PLEX_TUNER_PROVIDER_URL"),
		ProviderUser:    os.Getenv("PLEX_TUNER_PROVIDER_USER"),
		ProviderPass:    os.Getenv("PLEX_TUNER_PROVIDER_PASS"),
		M3UURL:          os.Getenv("PLEX_TUNER_M3U_URL"),
		MountPoint:      getEnv("PLEX_TUNER_MOUNT", "/mnt/vodfs"),
		CacheDir:        getEnv("PLEX_TUNER_CACHE", "/var/cache/plextuner"),
		CatalogPath:     getEnv("PLEX_TUNER_CATALOG", "./catalog.json"),
		TunerCount:     getEnvInt("PLEX_TUNER_TUNER_COUNT", 2),
		BaseURL:        os.Getenv("PLEX_TUNER_BASE_URL"),
		LiveEPGOnly:    getEnvBool("PLEX_TUNER_LIVE_EPG_ONLY", false),
		LiveOnly:       getEnvBool("PLEX_TUNER_LIVE_ONLY", false),
	}
	if c.TunerCount <= 0 {
		c.TunerCount = 2
	}
	return c
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

// DefaultProviderHosts is the same host list as xtream-to-m3u.js (Documents/code/k3s/plex/scripts):
// try all in parallel, first success wins. get.php often returns 884/Cloudflare; player_api.php often works.
var DefaultProviderHosts = []string{
	"http://pod17546.cdngold.me",
	"http://cf.supergaminghub.xyz",
	"http://cf.business-cdn-8k.ru",
	"http://cf.gaminghub8k.xyz",
	"http://cf.like-cdn.com",
	"http://pro.apps-cdn.net",
}

// ProviderURLs returns all base URLs to try (PLEX_TUNER_PROVIDER_URLS comma-separated, or single PLEX_TUNER_PROVIDER_URL, or DefaultProviderHosts when only creds are set).
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
	if c.ProviderUser != "" && c.ProviderPass != "" {
		return append([]string(nil), DefaultProviderHosts...)
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

func getEnvBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	return defaultVal
}

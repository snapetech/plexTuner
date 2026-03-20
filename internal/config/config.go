package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ProviderEntry is a single Xtream/M3U provider with its own base URL and credentials.
// Provider 1 is loaded from IPTV_TUNERR_PROVIDER_URL / _USER / _PASS.
// Providers 2+ are loaded from IPTV_TUNERR_PROVIDER_URL_2, _USER_2, _PASS_2, etc.
// All entries are available via Config.ProviderEntries().
type ProviderEntry struct {
	BaseURL string
	User    string
	Pass    string
}

// Config holds tuner + VODFS + provider settings.
// Load from env and/or config file (future).
type Config struct {
	// Provider (M3U / Xtream)
	ProviderBaseURL string // e.g. http://provider:8080
	ProviderUser    string
	ProviderPass    string
	M3UURL          string // optional: full M3U URL if different from base

	// Paths
	MountPoint      string // e.g. /mnt/vodfs
	CacheDir        string // e.g. /var/cache/iptvtunerr
	CatalogPath     string // e.g. /var/lib/iptvtunerr/catalog.json
	VODFSAllowOther bool   // Linux only: mount VODFS with FUSE allow_other (needed for some Plex/k8s hostPath setups)

	// Live tuner
	TunerCount        int
	LineupMaxChannels int    // max channels in lineup/guide (Plex DVR limit 480). 0 = use default 480.
	GuideNumberOffset int    // add offset to exposed GuideNumber values (lineup/guide) to avoid cross-DVR key collisions
	BaseURL           string // e.g. http://192.168.1.10:5004 for Plex to use
	DeviceID          string // HDHomeRun discover.json DeviceID (stable; some Plex versions are picky about format)
	FriendlyName      string // HDHomeRun discover.json FriendlyName (shown in Plex Live TV tuner list)
	// Stream: buffering absorbs brief upstream stalls; transcoding re-encodes (libx264/aac) for compatibility.
	StreamBufferBytes   int    // -1 = auto (default; adaptive when transcoding). 0 = no buffer. >0 = fixed bytes.
	StreamTranscodeMode string // "off" | "on" | "auto". auto = probe stream (ffprobe) and transcode only when codec not Plex-friendly.
	AutopilotStateFile  string // optional JSON state file for remembered channel/client playback decisions
	XMLTVURL            string // optional external XMLTV source to proxy/remap into /guide.xml
	XMLTVAliases        string // optional file path or http(s) URL for deterministic XMLTV alias overrides
	XMLTVMatchEnable    bool   // when true, repair/assign TVGIDs during catalog build from XMLTV channel metadata
	XMLTVTimeout        time.Duration
	LiveEPGOnly         bool // if true, only include channels with tvg-id (EPG-linked) in catalog
	LiveOnly            bool // if true, only fetch live channels from API (skip VOD and series; faster)
	// EPG prune: when true, guide.xml and M3U export only include channels with tvg-id set (reduces noise).
	EpgPruneUnlinked bool
	// Provider ingest policy: when true, reject any provider URL that is Cloudflare-proxied.
	// The ranker will skip CF URLs and try alternates; if all URLs are CF-proxied, ingest is
	// blocked with an alert log. Off by default. Enable with IPTV_TUNERR_BLOCK_CF_PROVIDERS=true.
	BlockCFProviders bool
	// when true, abort an HLS stream immediately if a segment fetch is redirected to
	// the Cloudflare abuse page (cloudflare-terms-of-service-abuse.com).
	// Prevents the 12-second stall timeout that results in 0-byte streams from CF-blocked CDNs.
	FetchCFReject bool
	// StripStreamHosts is a comma-separated list of hostnames (e.g. "cdngold.me,othercf.net")
	// whose stream URLs are removed from the catalog at index time.
	// A channel whose every StreamURL matches a blocked host is dropped entirely.
	// Suffix-matching: "cdngold.me" also matches "pod17546.cdngold.me".
	// Enable with IPTV_TUNERR_STRIP_STREAM_HOSTS=cdngold.me
	StripStreamHosts []string

	// Stream smoketest: when true, at index time probe each channel's primary URL and drop failures.
	SmoketestEnabled     bool
	SmoketestTimeout     time.Duration
	SmoketestConcurrency int
	SmoketestMaxChannels int           // 0 = all; else sample up to N random channels to cap runtime
	SmoketestMaxDuration time.Duration // hard cap total smoketest runtime (e.g. 5m); 0 = 5m default
	// Smoketest cache: persist probe results across runs to avoid re-probing fresh entries.
	SmoketestCacheFile string        // path to JSON cache; "" = disabled
	SmoketestCacheTTL  time.Duration // how long a probe result is considered fresh (default 4h)
	// XMLTV cache: cache the external XMLTV feed to avoid hammering the upstream on every /guide.xml request.
	XMLTVCacheTTL time.Duration // 0 = use default 10m
	// HDHomeRun network mode: native HDHomeRun protocol (UDP+TCP) instead of HTTP-only.
	HDHREnabled      bool
	HDHRDeviceID     uint32
	HDHRTunerCount   int
	HDHRDiscoverPort int
	HDHRControlPort  int
	HDHRFriendlyName string

	// Emby registration: IPTV_TUNERR_EMBY_HOST / IPTV_TUNERR_EMBY_TOKEN
	EmbyHost  string
	EmbyToken string

	// Jellyfin registration: IPTV_TUNERR_JELLYFIN_HOST / IPTV_TUNERR_JELLYFIN_TOKEN
	JellyfinHost  string
	JellyfinToken string

	// Provider EPG: fetch xmltv.php from the Xtream provider for richer guide data.
	// Layered priority: Placeholder < External XMLTV < Provider XMLTV.
	ProviderEPGEnabled  bool
	ProviderEPGTimeout  time.Duration
	ProviderEPGCacheTTL time.Duration

	// Free public sources: supplement or enrich the paid catalog with public M3U feeds
	// (e.g. iptv-org/iptv). No credentials required. Never redistributed — fetched at index time.
	//
	// IPTV_TUNERR_FREE_SOURCES        comma-separated M3U URLs
	// IPTV_TUNERR_FREE_SOURCE_MODE    supplement | merge | full (default: supplement)
	// IPTV_TUNERR_FREE_SOURCE_SMOKETEST   probe channels before adding (default: false)
	// IPTV_TUNERR_FREE_SOURCE_REQUIRE_TVGID  only include channels with a tvg-id (default: true)
	// IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES  comma-separated ISO-3166-1 alpha-2 codes (e.g. us,gb,ca)
	// IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_CATEGORIES comma-separated categories (e.g. news,sports,movies)
	// IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_ALL        fetch iptv-org index.m3u (all channels, ~40k)
	FreeSources                 []string
	FreeSourceMode              string // "supplement" | "merge" | "full"
	FreeSourceSmoketest         bool
	FreeSourceRequireTVGID      bool
	FreeSourceIptvOrgCountries  []string
	FreeSourceIptvOrgCategories []string
	FreeSourceIptvOrgAll        bool
}

// Load reads config from environment. Call LoadEnvFile(".env") before Load() to use a .env file.
// If ProviderUser or ProviderPass are empty, Load tries IPTV_TUNERR_SUBSCRIPTION_FILE (or default path) with "Username:" / "Password:" lines.
func Load() *Config {
	c := &Config{
		ProviderBaseURL:             getEnvURL("IPTV_TUNERR_PROVIDER_URL"),
		ProviderUser:                os.Getenv("IPTV_TUNERR_PROVIDER_USER"),
		ProviderPass:                os.Getenv("IPTV_TUNERR_PROVIDER_PASS"),
		M3UURL:                      getEnvURL("IPTV_TUNERR_M3U_URL"),
		MountPoint:                  getEnv("IPTV_TUNERR_MOUNT", "/mnt/vodfs"),
		CacheDir:                    getEnv("IPTV_TUNERR_CACHE", "/var/cache/iptvtunerr"),
		CatalogPath:                 getEnv("IPTV_TUNERR_CATALOG", "./catalog.json"),
		VODFSAllowOther:             getEnvBool("IPTV_TUNERR_VODFS_ALLOW_OTHER", false),
		TunerCount:                  getEnvInt("IPTV_TUNERR_TUNER_COUNT", 2),
		LineupMaxChannels:           getEnvInt("IPTV_TUNERR_LINEUP_MAX_CHANNELS", 480),
		GuideNumberOffset:           getEnvInt("IPTV_TUNERR_GUIDE_NUMBER_OFFSET", 0),
		BaseURL:                     os.Getenv("IPTV_TUNERR_BASE_URL"),
		DeviceID:                    getEnv("IPTV_TUNERR_DEVICE_ID", "iptvtunerr01"),
		FriendlyName:                os.Getenv("IPTV_TUNERR_FRIENDLY_NAME"),
		StreamBufferBytes:           getEnvIntOrAuto("IPTV_TUNERR_STREAM_BUFFER_BYTES", -1),
		StreamTranscodeMode:         getEnvTranscodeMode("IPTV_TUNERR_STREAM_TRANSCODE", "off"),
		AutopilotStateFile:          os.Getenv("IPTV_TUNERR_AUTOPILOT_STATE_FILE"),
		XMLTVURL:                    getEnvURL("IPTV_TUNERR_XMLTV_URL"),
		XMLTVAliases:                os.Getenv("IPTV_TUNERR_XMLTV_ALIASES"),
		XMLTVMatchEnable:            getEnvBool("IPTV_TUNERR_XMLTV_MATCH_ENABLE", true),
		XMLTVTimeout:                getEnvDuration("IPTV_TUNERR_XMLTV_TIMEOUT", 45*time.Second),
		LiveEPGOnly:                 getEnvBool("IPTV_TUNERR_LIVE_EPG_ONLY", false),
		LiveOnly:                    getEnvBool("IPTV_TUNERR_LIVE_ONLY", false),
		EpgPruneUnlinked:            getEnvBool("IPTV_TUNERR_EPG_PRUNE_UNLINKED", false),
		BlockCFProviders:            getEnvBool("IPTV_TUNERR_BLOCK_CF_PROVIDERS", false),
		FetchCFReject:               getEnvBool("IPTV_TUNERR_FETCH_CF_REJECT", false),
		StripStreamHosts:            getEnvHosts("IPTV_TUNERR_STRIP_STREAM_HOSTS"),
		SmoketestEnabled:            getEnvBool("IPTV_TUNERR_SMOKETEST_ENABLED", false),
		SmoketestTimeout:            getEnvDuration("IPTV_TUNERR_SMOKETEST_TIMEOUT", 8*time.Second),
		SmoketestConcurrency:        getEnvInt("IPTV_TUNERR_SMOKETEST_CONCURRENCY", 10),
		SmoketestMaxChannels:        getEnvInt("IPTV_TUNERR_SMOKETEST_MAX_CHANNELS", 0),
		SmoketestMaxDuration:        getEnvDuration("IPTV_TUNERR_SMOKETEST_MAX_DURATION", 5*time.Minute),
		SmoketestCacheFile:          os.Getenv("IPTV_TUNERR_SMOKETEST_CACHE_FILE"),
		SmoketestCacheTTL:           getEnvDuration("IPTV_TUNERR_SMOKETEST_CACHE_TTL", 4*time.Hour),
		XMLTVCacheTTL:               getEnvDuration("IPTV_TUNERR_XMLTV_CACHE_TTL", 10*time.Minute),
		HDHREnabled:                 getEnvBool("IPTV_TUNERR_HDHR_NETWORK_MODE", false),
		HDHRDeviceID:                getEnvUint32("IPTV_TUNERR_HDHR_DEVICE_ID", 0x12345678),
		HDHRTunerCount:              getEnvInt("IPTV_TUNERR_HDHR_TUNER_COUNT", 2),
		HDHRDiscoverPort:            getEnvInt("IPTV_TUNERR_HDHR_DISCOVER_PORT", 65001),
		HDHRControlPort:             getEnvInt("IPTV_TUNERR_HDHR_CONTROL_PORT", 65001),
		HDHRFriendlyName:            os.Getenv("IPTV_TUNERR_HDHR_FRIENDLY_NAME"),
		EmbyHost:                    getEnvURL("IPTV_TUNERR_EMBY_HOST"),
		EmbyToken:                   os.Getenv("IPTV_TUNERR_EMBY_TOKEN"),
		JellyfinHost:                getEnvURL("IPTV_TUNERR_JELLYFIN_HOST"),
		JellyfinToken:               os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"),
		ProviderEPGEnabled:          getEnvBool("IPTV_TUNERR_PROVIDER_EPG_ENABLED", true),
		ProviderEPGTimeout:          getEnvDuration("IPTV_TUNERR_PROVIDER_EPG_TIMEOUT", 90*time.Second),
		ProviderEPGCacheTTL:         getEnvDuration("IPTV_TUNERR_PROVIDER_EPG_CACHE_TTL", 10*time.Minute),
		FreeSources:                 getEnvCSV("IPTV_TUNERR_FREE_SOURCES"),
		FreeSourceMode:              getEnvFreeSourceMode("IPTV_TUNERR_FREE_SOURCE_MODE", "supplement"),
		FreeSourceSmoketest:         getEnvBool("IPTV_TUNERR_FREE_SOURCE_SMOKETEST", false),
		FreeSourceRequireTVGID:      getEnvBool("IPTV_TUNERR_FREE_SOURCE_REQUIRE_TVGID", true),
		FreeSourceIptvOrgCountries:  getEnvCSV("IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES"),
		FreeSourceIptvOrgCategories: getEnvCSV("IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_CATEGORIES"),
		FreeSourceIptvOrgAll:        getEnvBool("IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_ALL", false),
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
		if user, pass, err := readSubscriptionFile(getEnv("IPTV_TUNERR_SUBSCRIPTION_FILE", "")); err == nil {
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

func getEnvURL(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return ""
	}
	// Some shell/secret workflows persist URLs with escaped ampersands (e.g. "\&"),
	// which breaks url parsing/fetches when consumed as a literal env value.
	return strings.ReplaceAll(v, `\&`, `&`)
}

// readSubscriptionFile reads "Username: x" and "Password: x" from path. path may be empty to try default.
// When path is empty, globs ~/Documents/iptv.subscription.*.txt and uses the alphabetically last match
// (i.e. highest year), so the file keeps working across year-end renewals.
func readSubscriptionFile(path string) (user, pass string, err error) {
	if path == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return "", "", os.ErrNotExist
		}
		pattern := filepath.Join(home, "Documents", "iptv.subscription.*.txt")
		matches, globErr := filepath.Glob(pattern)
		if globErr != nil || len(matches) == 0 {
			return "", "", os.ErrNotExist
		}
		sort.Strings(matches)
		path = matches[len(matches)-1]
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

// M3UURLsOrBuild returns a list of M3U URLs to probe.
// Sources, in order:
//  1. IPTV_TUNERR_M3U_URL plus numbered IPTV_TUNERR_M3U_URL_2/_3/... entries if present
//  2. otherwise one get.php URL per IPTV_TUNERR_PROVIDER_URLS (or single ProviderBaseURL) with primary creds
func (c *Config) M3UURLsOrBuild() []string {
	var direct []string
	if c.M3UURL != "" {
		direct = append(direct, c.M3UURL)
	}
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("_%d", n)
		u := getEnvURL("IPTV_TUNERR_M3U_URL" + suffix)
		if u == "" {
			break
		}
		direct = append(direct, u)
	}
	if len(direct) > 0 {
		return direct
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

// ProviderURLs returns all base URLs to try (IPTV_TUNERR_PROVIDER_URLS comma-separated, or single IPTV_TUNERR_PROVIDER_URL).
// Requires explicit URL(s); no default host list.
func (c *Config) ProviderURLs() []string {
	s := os.Getenv("IPTV_TUNERR_PROVIDER_URLS")
	if s != "" {
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.ReplaceAll(strings.TrimSpace(p), `\&`, `&`)
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

// ProviderEntries returns all configured providers in priority order.
// Provider 1 comes from IPTV_TUNERR_PROVIDER_URL(S) / _USER / _PASS (already loaded into Config fields).
// Providers 2..N come from IPTV_TUNERR_PROVIDER_URL_2/_USER_2/_PASS_2, _URL_3/_USER_3/_PASS_3, etc.
// Scanning stops at the first missing _URL_N. Entries with no BaseURL are skipped.
func (c *Config) ProviderEntries() []ProviderEntry {
	var out []ProviderEntry
	// Entry 1: from the primary fields (IPTV_TUNERR_PROVIDER_URL(S) already handled by ProviderURLs).
	for _, base := range c.ProviderURLs() {
		if base != "" {
			out = append(out, ProviderEntry{BaseURL: base, User: c.ProviderUser, Pass: c.ProviderPass})
		}
	}
	// Entries 2..N: IPTV_TUNERR_PROVIDER_URL_N / _USER_N / _PASS_N
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("_%d", n)
		base := getEnvURL("IPTV_TUNERR_PROVIDER_URL" + suffix)
		if base == "" {
			break
		}
		user := os.Getenv("IPTV_TUNERR_PROVIDER_USER" + suffix)
		pass := os.Getenv("IPTV_TUNERR_PROVIDER_PASS" + suffix)
		// Fall back to primary creds if per-entry creds are not set.
		if user == "" {
			user = c.ProviderUser
		}
		if pass == "" {
			pass = c.ProviderPass
		}
		out = append(out, ProviderEntry{BaseURL: base, User: user, Pass: pass})
	}
	return out
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

// getEnvTranscodeMode returns "off", "on", "auto", or "auto_cached" from IPTV_TUNERR_STREAM_TRANSCODE.
func getEnvTranscodeMode(key string, defaultVal string) string {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "auto" {
		return "auto"
	}
	if v == "auto_cached" || v == "cached_auto" {
		return "auto_cached"
	}
	if v == "true" || v == "1" || v == "yes" || v == "on" {
		return "on"
	}
	if v == "false" || v == "0" || v == "no" || v == "off" || v == "" {
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

func getEnvUint32(key string, defaultVal uint32) uint32 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	// base 0: auto-detect (handles "0x" hex prefix as well as decimal)
	n, err := strconv.ParseUint(v, 0, 32)
	if err != nil {
		return defaultVal
	}
	return uint32(n)
}

// getEnvCSV parses a comma-separated list of strings from an env var. Empty entries are dropped.
func getEnvCSV(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// getEnvFreeSourceMode returns one of "supplement", "merge", or "full". Defaults to defaultVal on unknown input.
func getEnvFreeSourceMode(key, defaultVal string) string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "supplement", "merge", "full":
		return v
	default:
		return defaultVal
	}
}

// getEnvHosts parses a comma-separated list of hostnames from an env var.
// Values are lowercased and empty entries are dropped.
func getEnvHosts(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

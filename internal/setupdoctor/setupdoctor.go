package setupdoctor

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/config"
)

type Issue struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Report struct {
	Ready          bool     `json:"ready"`
	Mode           string   `json:"mode"`
	BaseURL        string   `json:"base_url,omitempty"`
	GuideURL       string   `json:"guide_url,omitempty"`
	DeckURL        string   `json:"deck_url,omitempty"`
	ProviderHosts  int      `json:"provider_hosts"`
	Checks         []Issue  `json:"checks"`
	NextSteps      []string `json:"next_steps,omitempty"`
	Summary        string   `json:"summary"`
	MinimalEnvHint string   `json:"minimal_env_hint,omitempty"`
}

func Build(cfg *config.Config, mode, baseURLOverride string) Report {
	mode = normalizeMode(mode)
	report := Report{
		Mode:           mode,
		MinimalEnvHint: ".env.minimal.example",
	}
	providerEntries := cfg.ProviderEntries()
	report.ProviderHosts = len(providerEntries)

	baseURL := strings.TrimSpace(baseURLOverride)
	if baseURL == "" {
		baseURL = strings.TrimSpace(cfg.BaseURL)
	}
	if baseURL != "" {
		report.BaseURL = strings.TrimRight(baseURL, "/")
		report.GuideURL = report.BaseURL + "/guide.xml"
	}
	report.DeckURL = deckURL(report.BaseURL, cfg)

	m3uURL := strings.TrimSpace(cfg.M3UURL)
	providerURLCount := 0
	providerCredCount := 0
	for _, entry := range providerEntries {
		if strings.TrimSpace(entry.BaseURL) != "" {
			providerURLCount++
		}
		if strings.TrimSpace(entry.BaseURL) != "" && strings.TrimSpace(entry.User) != "" && strings.TrimSpace(entry.Pass) != "" {
			providerCredCount++
		}
	}

	switch {
	case m3uURL != "":
		report.Checks = append(report.Checks, Issue{
			Level:   "pass",
			Code:    "source",
			Message: "Direct M3U source is configured.",
		})
	case providerURLCount == 0:
		report.Checks = append(report.Checks, Issue{
			Level:   "fail",
			Code:    "source",
			Message: "No IPTV source is configured.",
			Hint:    "Set IPTV_TUNERR_M3U_URL, or set IPTV_TUNERR_PROVIDER_URL plus IPTV_TUNERR_PROVIDER_USER and IPTV_TUNERR_PROVIDER_PASS.",
		})
	case providerCredCount == 0:
		report.Checks = append(report.Checks, Issue{
			Level:   "fail",
			Code:    "provider_credentials",
			Message: "Provider host is configured, but credentials are missing.",
			Hint:    "Set IPTV_TUNERR_PROVIDER_USER and IPTV_TUNERR_PROVIDER_PASS, or use IPTV_TUNERR_SUBSCRIPTION_FILE.",
		})
	default:
		msg := "Provider host and credentials are configured."
		if providerCredCount > 1 {
			msg = fmt.Sprintf("%d provider hosts with credentials are configured for probe/failover.", providerCredCount)
		}
		report.Checks = append(report.Checks, Issue{
			Level:   "pass",
			Code:    "provider_credentials",
			Message: msg,
		})
	}

	switch {
	case report.BaseURL == "":
		report.Checks = append(report.Checks, Issue{
			Level:   "fail",
			Code:    "base_url",
			Message: "IPTV_TUNERR_BASE_URL is not set.",
			Hint:    "Set it to the URL your media server will use, for example http://192.168.1.10:5004.",
		})
	default:
		u, err := url.Parse(report.BaseURL)
		switch {
		case err != nil || u.Scheme == "" || u.Host == "":
			report.Checks = append(report.Checks, Issue{
				Level:   "fail",
				Code:    "base_url",
				Message: "IPTV_TUNERR_BASE_URL is not a valid absolute URL.",
				Hint:    "Use a full URL such as http://192.168.1.10:5004.",
			})
		case HostLooksLocalOnly(u.Hostname()):
			report.Checks = append(report.Checks, Issue{
				Level:   "warn",
				Code:    "base_url_local_only",
				Message: "Base URL points at localhost or another local-only address.",
				Hint:    "That is fine only when the media server runs on the same machine. Otherwise use the LAN IP or DNS name of this host.",
			})
		default:
			report.Checks = append(report.Checks, Issue{
				Level:   "pass",
				Code:    "base_url",
				Message: "Base URL looks reachable for another machine on the network.",
			})
		}
	}

	catalogPath := strings.TrimSpace(cfg.CatalogPath)
	switch {
	case catalogPath == "":
		report.Checks = append(report.Checks, Issue{
			Level:   "warn",
			Code:    "catalog_path",
			Message: "Catalog path is empty; runtime will fall back to defaults.",
		})
	case !filepath.IsAbs(catalogPath):
		report.Checks = append(report.Checks, Issue{
			Level:   "warn",
			Code:    "catalog_path_relative",
			Message: fmt.Sprintf("Catalog path is relative: %s", catalogPath),
			Hint:    "Relative paths are fine for ad-hoc local runs, but absolute paths are safer for systemd, Docker, and Kubernetes.",
		})
	default:
		report.Checks = append(report.Checks, Issue{
			Level:   "pass",
			Code:    "catalog_path",
			Message: fmt.Sprintf("Catalog path is pinned: %s", catalogPath),
		})
	}

	plexHost := strings.TrimSpace(firstNonEmptyEnv("IPTV_TUNERR_PMS_URL", "PLEX_HOST"))
	plexToken := strings.TrimSpace(firstNonEmptyEnv("IPTV_TUNERR_PMS_TOKEN", "PLEX_TOKEN"))
	if mode == "full" {
		switch {
		case plexHost == "" && plexToken == "":
			report.Checks = append(report.Checks, Issue{Level: "warn", Code: "plex_api", Message: "Plex API zero-touch registration is not configured.", Hint: "Set IPTV_TUNERR_PMS_URL or PLEX_HOST plus IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN, then use -register-plex=api for automatic DVR creation/reuse."})
		case plexHost == "":
			report.Checks = append(report.Checks, Issue{Level: "warn", Code: "plex_api", Message: "Plex token is set, but Plex host is missing.", Hint: "Set IPTV_TUNERR_PMS_URL or PLEX_HOST so Tunerr can call Plex automatically."})
		case plexToken == "":
			report.Checks = append(report.Checks, Issue{Level: "warn", Code: "plex_api", Message: "Plex host is set, but Plex token is missing.", Hint: "Set IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN so Tunerr can create or reuse the DVR automatically."})
		default:
			report.Checks = append(report.Checks, Issue{Level: "pass", Code: "plex_api", Message: "Plex API zero-touch registration looks configured."})
		}
	}

	if cfg.LineupMaxChannels > 480 && mode == "easy" {
		report.Checks = append(report.Checks, Issue{
			Level:   "warn",
			Code:    "lineup_cap",
			Message: fmt.Sprintf("Lineup cap is %d, which exceeds the usual Plex wizard-safe range.", cfg.LineupMaxChannels),
			Hint:    "For the simple Plex wizard path, keep IPTV_TUNERR_LINEUP_MAX_CHANNELS at 480 or below and run with -mode=easy.",
		})
	} else if mode == "easy" {
		report.Checks = append(report.Checks, Issue{
			Level:   "pass",
			Code:    "lineup_cap",
			Message: "Simple first-run mode is selected.",
		})
	} else {
		report.Checks = append(report.Checks, Issue{
			Level:   "pass",
			Code:    "mode",
			Message: "Full mode is selected for advanced or headless setups.",
		})
	}

	switch {
	case strings.EqualFold(strings.TrimSpace(cfg.WebUIUser), "admin") && strings.TrimSpace(cfg.WebUIPass) == "admin":
		report.Checks = append(report.Checks, Issue{
			Level:   "warn",
			Code:    "webui_default_credentials",
			Message: "Web UI is pinned to admin/admin.",
			Hint:    "Set IPTV_TUNERR_WEBUI_USER and IPTV_TUNERR_WEBUI_PASS to something real, or leave the password unset so Tunerr generates one at startup.",
		})
	case strings.EqualFold(strings.TrimSpace(cfg.WebUIUser), "admin") && strings.TrimSpace(cfg.WebUIPass) == "change-me":
		report.Checks = append(report.Checks, Issue{
			Level:   "warn",
			Code:    "webui_default_credentials",
			Message: "Web UI password is still set to the placeholder value change-me.",
			Hint:    "Set IPTV_TUNERR_WEBUI_PASS to a real value, or leave it unset so Tunerr generates one at startup.",
		})
	default:
		msg := fmt.Sprintf("Deck will be available at %s", report.DeckURL)
		if !cfg.WebUIAllowLAN {
			msg += " (localhost-only by default)."
		} else {
			msg += "."
		}
		report.Checks = append(report.Checks, Issue{
			Level:   "pass",
			Code:    "webui",
			Message: msg,
		})
	}

	hasFail := false
	for _, check := range report.Checks {
		if check.Level == "fail" {
			hasFail = true
			break
		}
	}
	report.Ready = !hasFail
	if report.Ready {
		report.Summary = "Ready for a first real run."
		report.NextSteps = append(report.NextSteps,
			"cp .env.minimal.example .env   # if you have not already created a smaller first-run env file",
			"iptv-tunerr probe",
			fmt.Sprintf("iptv-tunerr run -mode=%s", mode),
		)
		if report.BaseURL != "" {
			report.NextSteps = append(report.NextSteps,
				"Connect Plex/Emby/Jellyfin to the tuner URL: "+report.BaseURL,
				"Use the XMLTV guide URL: "+report.GuideURL,
			)
		}
		if mode == "full" {
			if plexHost != "" && plexToken != "" {
				report.NextSteps = append(report.NextSteps, "Run zero-touch Plex registration: iptv-tunerr run -mode=full -register-plex=api")
			} else {
				report.NextSteps = append(report.NextSteps, "Set PLEX_HOST and PLEX_TOKEN (or IPTV_TUNERR_PMS_URL and IPTV_TUNERR_PMS_TOKEN) before using -register-plex=api.")
			}
		}
		if report.DeckURL != "" {
			report.NextSteps = append(report.NextSteps, "Open the Control Deck: "+report.DeckURL)
		}
	} else {
		report.Summary = "Not ready yet; fix the failing checks first."
		report.NextSteps = append(report.NextSteps,
			"Copy .env.minimal.example to .env if you want the smallest working template.",
			"Fill in source settings and IPTV_TUNERR_BASE_URL.",
			"Run iptv-tunerr setup-doctor again until all FAIL rows are gone.",
		)
	}

	return report
}

func NormalizeMode(mode string) string {
	return normalizeMode(mode)
}

func normalizeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "full" {
		return "easy"
	}
	return "full"
}

func HostLooksLocalOnly(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return true
	}
	switch host {
	case "localhost", "0.0.0.0", "::1", "[::1]":
		return true
	}
	if strings.HasPrefix(host, "127.") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsUnspecified()
}

func deckURL(baseURL string, cfg *config.Config) string {
	port := cfg.WebUIPort
	if port <= 0 {
		port = 48879
	}
	if cfg.WebUIAllowLAN && baseURL != "" {
		if u, err := url.Parse(strings.TrimSpace(baseURL)); err == nil && u.Hostname() != "" {
			host := u.Hostname()
			if strings.Contains(host, ":") {
				host = "[" + host + "]"
			}
			return fmt.Sprintf("http://%s:%d/", host, port)
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%d/", port)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func getenv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

package tuner

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// cfBootstrapper orchestrates automatic Cloudflare clearance for provider hosts.
// Resolution sequence (stops at first success):
//
//  1. Existing jar has valid cf_clearance → skip entirely
//  2. UA cycling: try each UA preset against the provider URL → sets learnedUA on Gateway
//  3. Browser cookie theft: read Chrome/Firefox profiles on the local machine → import cf_clearance
//  4. Headless Chromium: launch invisibly with a temp profile, extract clearance → import
//  5. Real browser: xdg-open/open the URL, poll until cf_clearance appears → import
//     (only if DISPLAY/WAYLAND_DISPLAY is set; otherwise skipped)
//
// The bootstrapper caches results per-host and only re-runs after cfBootTTL.
type cfBootstrapper struct {
	jar          *persistentCookieJar
	uaCandidates []string

	mu     sync.Mutex
	byHost map[string]*cfBootState
}

type cfBootState struct {
	workingUA string
	resolved  bool
	lastAt    time.Time
}

const cfBootTTL = 12 * time.Hour

func newCFBootstrapper(jar *persistentCookieJar, uaCandidates []string) *cfBootstrapper {
	return &cfBootstrapper{
		jar:          jar,
		uaCandidates: uaCandidates,
		byHost:       make(map[string]*cfBootState),
	}
}

// EnsureAccess checks and, if needed, bootstraps CF clearance for the given provider URL.
// Returns the working User-Agent to use (may be empty if cookies solved it), or "" on failure.
// Thread-safe: only one resolution runs per host at a time.
func (c *cfBootstrapper) EnsureAccess(ctx context.Context, rawURL string, client *http.Client) string {
	host := hostFromURL(rawURL)
	if host == "" {
		return ""
	}

	c.mu.Lock()
	state := c.byHost[host]
	if state != nil && state.resolved && time.Since(state.lastAt) < cfBootTTL {
		ua := state.workingUA
		c.mu.Unlock()
		return ua
	}
	if state == nil {
		state = &cfBootState{}
		c.byHost[host] = state
	}
	c.mu.Unlock()

	log.Printf("cf-bootstrap: starting access check for %s", host)

	// Step 1: does a plain probe with our first UA candidate succeed without any CF response?
	if ua := c.tryCycleThenProbe(ctx, rawURL, client); ua != "" {
		log.Printf("cf-bootstrap: %s accessible via UA %q — no cookie required", host, ua)
		c.markResolved(host, ua)
		return ua
	}

	// Step 2: try stealing existing browser cookies.
	if c.tryBrowserCookies(ctx, rawURL, client, host) {
		log.Printf("cf-bootstrap: %s — imported browser cf_clearance cookie", host)
		c.markResolved(host, "")
		return ""
	}

	// Step 3: headless Chromium (completely silent, no user interaction).
	if c.tryHeadlessChrome(ctx, rawURL, host) {
		log.Printf("cf-bootstrap: %s — obtained cf_clearance via headless Chrome", host)
		c.markResolved(host, "")
		return ""
	}

	// Step 4: open real browser and wait for the user to clear the challenge.
	if hasDisplay() {
		log.Printf("cf-bootstrap: %s — opening browser for CF challenge (waiting up to 60s)...", host)
		if c.tryRealBrowser(ctx, rawURL, host) {
			log.Printf("cf-bootstrap: %s — cf_clearance obtained via real browser", host)
			c.markResolved(host, "")
			return ""
		}
	}

	log.Printf("cf-bootstrap: %s — could not obtain CF clearance automatically", host)
	return ""
}

func (c *cfBootstrapper) markResolved(host, ua string) {
	c.mu.Lock()
	c.byHost[host] = &cfBootState{workingUA: ua, resolved: true, lastAt: time.Now()}
	c.mu.Unlock()
}

// tryCycleThenProbe attempts the rawURL with each UA candidate, returns the first one that
// returns HTTP 200. A non-CF non-200 response is not retried (wrong auth etc).
// Full browser header profile is applied alongside browser UAs to maximize CF bypass rate.
func (c *cfBootstrapper) tryCycleThenProbe(ctx context.Context, rawURL string, client *http.Client) string {
	for _, ua := range c.uaCandidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", ua)
		for name, value := range browserHeadersForUA(ua) {
			req.Header.Set(name, value)
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		status := resp.StatusCode
		preview := make([]byte, 256)
		n, _ := resp.Body.Read(preview)
		resp.Body.Close()
		if status == http.StatusOK {
			return ua
		}
		if !isCFLikeStatus(status, string(preview[:n])) {
			break // non-CF error (e.g. 401 wrong creds) — stop cycling
		}
	}
	return ""
}

// StartFreshnessMonitor runs a background goroutine that proactively refreshes CF clearance
// for known CF-tagged hosts before their cf_clearance cookie expires.
// This prevents mid-session failures when CF clearance TTLs expire (commonly 10min–12h).
func (c *cfBootstrapper) StartFreshnessMonitor(ctx context.Context, client *http.Client) {
	if c == nil {
		return
	}
	go func() {
		t := time.NewTicker(30 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.mu.Lock()
				hosts := make([]string, 0, len(c.byHost))
				for host, st := range c.byHost {
					if st != nil && st.resolved {
						hosts = append(hosts, host)
					}
				}
				c.mu.Unlock()
				for _, host := range hosts {
					if c.jar == nil {
						continue
					}
					u, err := url.Parse("https://" + host + "/")
					if err != nil || u == nil {
						continue
					}
					for _, ck := range c.jar.Cookies(u) {
						if ck.Name != "cf_clearance" {
							continue
						}
						if !ck.Expires.IsZero() && time.Until(ck.Expires) < time.Hour {
							log.Printf("cf-bootstrap: cf_clearance for %s expires in %v; refreshing proactively", host, time.Until(ck.Expires).Round(time.Minute))
							probeURL := "https://" + host + "/"
							go func(h, u string) {
								_ = c.EnsureAccess(ctx, u, client)
							}(host, probeURL)
						}
					}
				}
			}
		}
	}()
}

// tryBrowserCookies reads cf_clearance cookies from Chrome/Firefox profiles on the local machine,
// imports them into the jar, and verifies they work against the provider URL.
func (c *cfBootstrapper) tryBrowserCookies(ctx context.Context, rawURL string, client *http.Client, host string) bool {
	cookies := BrowserCookiesForHost(host)
	if len(cookies) == 0 {
		return false
	}
	// Only bother if we found a cf_clearance.
	hasClearance := false
	for _, ck := range cookies {
		if ck.Name == "cf_clearance" {
			hasClearance = true
			break
		}
	}
	if !hasClearance {
		return false
	}
	// Import into our jar.
	if c.jar != nil {
		u, err := url.Parse(rawURL)
		if err == nil {
			c.jar.SetCookies(u, cookies)
			_ = c.jar.Save()
		}
	}
	// Verify with a probe using the cookies.
	return c.probeWithJar(ctx, rawURL, client)
}

// probeWithJar does a GET to rawURL using the current jar cookies; returns true on HTTP 200.
func (c *cfBootstrapper) probeWithJar(ctx context.Context, rawURL string, client *http.Client) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	if c.jar != nil {
		if u, err := url.Parse(rawURL); err == nil {
			for _, ck := range c.jar.Cookies(u) {
				req.AddCookie(ck)
			}
		}
	}
	req.Header.Set("User-Agent", defaultLavfUA)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// tryHeadlessChrome launches Chromium/Chrome headlessly, navigates to rawURL so CF can solve
// its challenge, then extracts the resulting cf_clearance cookie into the jar.
// Returns true if a valid cf_clearance was obtained.
func (c *cfBootstrapper) tryHeadlessChrome(ctx context.Context, rawURL, host string) bool {
	bin := findChromeBin()
	if bin == "" {
		return false
	}
	dir, err := os.MkdirTemp("", "tunerr-cfboot-*")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)

	// --disable-blink-features=AutomationControlled hides the webdriver flag from CF.
	// --virtual-time-budget fast-forwards JavaScript timers so CF's ~3s delay passes instantly.
	// --password-store=basic avoids keyring prompts in the temp profile.
	chromeUA := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	args := []string{
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-blink-features=AutomationControlled",
		"--password-store=basic",
		"--use-mock-keychain",
		"--user-agent=" + chromeUA,
		"--user-data-dir=" + dir,
		"--run-all-compositor-stages-before-draw",
		"--virtual-time-budget=8000",
		rawURL,
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, bin, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	_ = cmd.Run() // ignore exit code; Chrome exits non-zero in headless sometimes

	// Read cookies from temp profile.
	cookieDB := filepath.Join(dir, "Default", "Cookies")
	if _, err := os.Stat(cookieDB); err != nil {
		return false
	}
	cookies, err := readChromeCookies(cookieDB, host)
	if err != nil || len(cookies) == 0 {
		return false
	}
	hasClearance := false
	for _, ck := range cookies {
		if ck.Name == "cf_clearance" {
			hasClearance = true
			break
		}
	}
	if !hasClearance {
		return false
	}
	if c.jar != nil {
		u, err := url.Parse(rawURL)
		if err == nil {
			c.jar.SetCookies(u, cookies)
			_ = c.jar.Save()
		}
	}
	return true
}

// tryRealBrowser opens rawURL in the user's default browser (xdg-open / open) and polls
// their Chrome/Firefox profiles for a cf_clearance cookie for up to 60 seconds.
// Returns true if cf_clearance was found and imported.
func (c *cfBootstrapper) tryRealBrowser(ctx context.Context, rawURL, host string) bool {
	var openCmd string
	switch runtime.GOOS {
	case "linux":
		openCmd = "xdg-open"
	case "darwin":
		openCmd = "open"
	default:
		return false
	}
	if _, err := exec.LookPath(openCmd); err != nil {
		return false
	}
	_ = exec.CommandContext(ctx, openCmd, rawURL).Start()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(3 * time.Second):
		}
		cookies := BrowserCookiesForHost(host)
		for _, ck := range cookies {
			if ck.Name == "cf_clearance" {
				if c.jar != nil {
					u, _ := url.Parse(rawURL)
					if u != nil {
						c.jar.SetCookies(u, cookies)
						_ = c.jar.Save()
					}
				}
				return true
			}
		}
	}
	return false
}

// findChromeBin returns the path to a Chrome/Chromium binary, or "" if none found.
func findChromeBin() string {
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "google-chrome-beta"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// hasDisplay returns true if a graphical display is available (needed for real-browser fallback).
func hasDisplay() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// hostFromURL extracts the bare hostname from a URL string.
func hostFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Hostname()))
}

// hasCFClearanceInJar returns true if the jar has a non-expired cf_clearance for the host.
func hasCFClearanceInJar(jar *persistentCookieJar, rawURL string) bool {
	if jar == nil {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	for _, ck := range jar.Cookies(u) {
		if ck.Name == "cf_clearance" && (ck.Expires.IsZero() || ck.Expires.After(time.Now())) {
			return true
		}
	}
	return false
}

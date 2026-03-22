package tuner

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

// PrepareCloudflareAwareClient returns an HTTP client and preferred User-Agent list
// suitable for catalog/probe fetches against CF-protected provider endpoints such as get.php.
//
// Behavior:
// - if IPTV_TUNERR_COOKIE_JAR_FILE is set, load that persistent jar into the client
// - if IPTV_TUNERR_CF_AUTO_BOOT=true, actively try the existing CF bootstrap flow
// - if bootstrap discovers a working UA, return it first so callers can prefer it
func PrepareCloudflareAwareClient(ctx context.Context, rawURL string, base *http.Client, detectedLavfUA string) (*http.Client, []string, error) {
	client := base
	if client == nil {
		client = httpclient.Default()
	}
	persistentJar, changed, err := ensurePersistentCookieJar(client, rawURL)
	if err != nil {
		return client, nil, err
	}
	if changed {
		clone := *client
		clone.Jar = persistentJar
		client = &clone
	}

	if !envBool("IPTV_TUNERR_CF_AUTO_BOOT", false) {
		return client, nil, nil
	}

	boot := newCFBootstrapper(persistentJar, uaCycleCandidates(detectedLavfUA))
	workingUA := strings.TrimSpace(boot.EnsureAccess(ctx, rawURL, client))
	if workingUA == "" {
		return client, nil, nil
	}
	return client, []string{workingUA}, nil
}

func ensurePersistentCookieJar(client *http.Client, rawURL string) (*persistentCookieJar, bool, error) {
	if client != nil {
		if jar, ok := client.Jar.(*persistentCookieJar); ok {
			return jar, false, nil
		}
	}
	jarPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE"))
	jar, err := newPersistentCookieJar(jarPath)
	if err != nil {
		return nil, false, err
	}
	if client == nil || client.Jar == nil {
		return jar, true, nil
	}
	importCookiesForRawURL(jar, client.Jar, rawURL)
	return jar, true, nil
}

func importCookiesForRawURL(dst *persistentCookieJar, src http.CookieJar, rawURL string) {
	if dst == nil || src == nil {
		return
	}
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || target == nil || strings.TrimSpace(target.Host) == "" {
		return
	}
	for _, candidate := range cookieImportCandidates(target) {
		cookies := src.Cookies(candidate)
		if len(cookies) == 0 {
			continue
		}
		dst.SetCookies(candidate, cookies)
	}
}

func cookieImportCandidates(target *url.URL) []*url.URL {
	candidates := []*url.URL{target}
	if target == nil || target.Host == "" {
		return candidates
	}
	oppositeScheme := "http"
	if strings.EqualFold(target.Scheme, "http") {
		oppositeScheme = "https"
	}
	if !strings.EqualFold(target.Scheme, oppositeScheme) {
		alt := *target
		alt.Scheme = oppositeScheme
		candidates = append(candidates, &alt)
	}
	root := &url.URL{Scheme: target.Scheme, Host: target.Host, Path: "/"}
	if root.String() != target.String() {
		candidates = append(candidates, root)
	}
	if !strings.EqualFold(root.Scheme, oppositeScheme) {
		altRoot := *root
		altRoot.Scheme = oppositeScheme
		candidates = append(candidates, &altRoot)
	}
	return candidates
}

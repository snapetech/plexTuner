package tuner

import (
	"context"
	"net/http"
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
	if client.Jar == nil {
		jarPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE"))
		jar, err := newPersistentCookieJar(jarPath)
		if err != nil {
			return client, nil, err
		}
		clone := *client
		clone.Jar = jar
		client = &clone
	}

	if !envBool("IPTV_TUNERR_CF_AUTO_BOOT", false) {
		return client, nil, nil
	}

	jar, ok := client.Jar.(*persistentCookieJar)
	if !ok {
		return client, nil, nil
	}
	boot := newCFBootstrapper(jar, uaCycleCandidates(detectedLavfUA))
	workingUA := strings.TrimSpace(boot.EnsureAccess(ctx, rawURL, client))
	if workingUA == "" {
		return client, nil, nil
	}
	return client, []string{workingUA}, nil
}

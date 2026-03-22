package guideinput

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/refio"
)

const defaultTimeout = 45 * time.Second
const guideInputUserAgent = "IptvTunerr/1.0"

func configuredGuideInputTimeout() time.Duration {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_XMLTV_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			timeout = d
		}
	}
	return timeout
}

func configuredProviderGuideInputTimeout() time.Duration {
	timeout := configuredGuideInputTimeout()
	if raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_EPG_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			timeout = d
		}
	}
	return timeout
}

func guideInputTimeoutForURL(rawURL string) time.Duration {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return configuredGuideInputTimeout()
	}
	for _, ref := range providerXMLTVRefsFromEnv() {
		if rawURL == strings.TrimSpace(ref) {
			return configuredProviderGuideInputTimeout()
		}
	}
	return configuredGuideInputTimeout()
}

func ProviderXMLTVURL(baseURL, user, pass string) string {
	return ProviderXMLTVURLWithSuffix(baseURL, user, pass, "")
}

func ProviderXMLTVURLWithSuffix(baseURL, user, pass, suffix string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	user = strings.TrimSpace(user)
	pass = strings.TrimSpace(pass)
	if baseURL == "" || user == "" || pass == "" {
		return ""
	}
	rawURL := baseURL + "/xmltv.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return rawURL
	}
	return rawURL + "&" + strings.TrimPrefix(suffix, "&")
}

func LoadGuideData(ref string) ([]byte, error) {
	return LoadGuideDataWithAllowed(ref, nil)
}

func LoadGuideDataWithAllowed(ref string, extraAllowedRemoteRefs []string) ([]byte, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, nil
	}
	if remote, ok, err := lookupAllowedRemoteGuideRef(ref, extraAllowedRemoteRefs); err != nil {
		return nil, err
	} else if ok {
		r, err := openRemoteGuideRef(context.Background(), remote)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	}
	local, err := refio.PrepareLocalFileRef(ref)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(local.Path())
}

func LoadAliasOverrides(ref string) (epglink.AliasOverrides, error) {
	return LoadAliasOverridesWithAllowed(ref, nil)
}

func LoadAliasOverridesWithAllowed(ref string, extraAllowedRemoteRefs []string) (epglink.AliasOverrides, error) {
	if strings.TrimSpace(ref) == "" {
		return epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}, nil
	}
	data, err := LoadGuideDataWithAllowed(ref, extraAllowedRemoteRefs)
	if err != nil {
		return epglink.AliasOverrides{}, err
	}
	return epglink.LoadAliasOverrides(bytes.NewReader(data))
}

func LoadXMLTVChannels(ref string) ([]epglink.XMLTVChannel, error) {
	return LoadXMLTVChannelsWithAllowed(ref, nil)
}

func LoadXMLTVChannelsWithAllowed(ref string, extraAllowedRemoteRefs []string) ([]epglink.XMLTVChannel, error) {
	data, err := LoadGuideDataWithAllowed(ref, extraAllowedRemoteRefs)
	if err != nil {
		return nil, err
	}
	return epglink.ParseXMLTVChannels(bytes.NewReader(data))
}

func LoadOptionalMatchReport(live []catalog.LiveChannel, xmltvRef, aliasesRef string) (*epglink.Report, error) {
	xmltvRef = strings.TrimSpace(xmltvRef)
	if xmltvRef == "" {
		return nil, nil
	}
	xmltvChans, err := LoadXMLTVChannels(xmltvRef)
	if err != nil {
		return nil, err
	}
	aliases, err := LoadAliasOverrides(aliasesRef)
	if err != nil {
		return nil, err
	}
	rep := epglink.MatchLiveChannels(live, xmltvChans, aliases)
	return &rep, nil
}

func lookupAllowedRemoteGuideRef(ref string, extraAllowedRemoteRefs []string) (refio.RemoteHTTPRef, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || !strings.HasPrefix(strings.ToLower(ref), "http") {
		return refio.RemoteHTTPRef{}, false, nil
	}
	remote, err := refio.PrepareRemoteHTTPRef(context.Background(), ref)
	if err != nil {
		return refio.RemoteHTTPRef{}, false, err
	}
	allowlist := configuredGuideInputRemoteAllowlist(extraAllowedRemoteRefs)
	if allowed, ok := allowlist[remote.URL()]; ok {
		return allowed, true, nil
	}
	return refio.RemoteHTTPRef{}, false, fmt.Errorf("remote ref not in guide allowlist")
}

func openRemoteGuideRef(ctx context.Context, ref refio.RemoteHTTPRef) (io.ReadCloser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := guideInputTimeoutForURL(ref.URL())
	ctx, cancel := context.WithTimeout(ctx, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.URL(), nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("User-Agent", guideInputUserAgent)
	resp, err := httpclient.WithTimeout(timeout).Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return &guideInputReadCloser{ReadCloser: resp.Body, cancel: cancel}, nil
}

type guideInputReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *guideInputReadCloser) Close() error {
	err := r.ReadCloser.Close()
	if r.cancel != nil {
		r.cancel()
	}
	return err
}

func configuredGuideInputRemoteAllowlist(extraAllowedRemoteRefs []string) map[string]refio.RemoteHTTPRef {
	allowlist := make(map[string]refio.RemoteHTTPRef)
	for _, raw := range configuredGuideInputRemoteRefsFromEnv(extraAllowedRemoteRefs) {
		allowed, err := refio.PrepareRemoteHTTPRef(context.Background(), raw)
		if err != nil {
			continue
		}
		allowlist[allowed.URL()] = allowed
	}
	return allowlist
}

func configuredGuideInputRemoteRefsFromEnv(extraAllowedRemoteRefs []string) []string {
	refs := []string{
		os.Getenv("IPTV_TUNERR_XMLTV_URL"),
		os.Getenv("IPTV_TUNERR_XMLTV_ALIASES"),
		os.Getenv("IPTV_TUNERR_HDHR_GUIDE_URL"),
	}
	refs = append(refs, providerXMLTVRefsFromEnv()...)
	refs = append(refs, strings.Split(os.Getenv("IPTV_TUNERR_GUIDE_INPUT_ALLOWED_URLS"), ",")...)
	refs = append(refs, extraAllowedRemoteRefs...)
	out := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out
}

func providerXMLTVRefsFromEnv() []string {
	defaultUser := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_USER"))
	defaultPass := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_PASS"))
	suffix := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX"))
	refs := []string{}
	if ref := ProviderXMLTVURLWithSuffix(os.Getenv("IPTV_TUNERR_PROVIDER_URL"), defaultUser, defaultPass, suffix); ref != "" {
		refs = append(refs, ref)
	}
	for _, base := range strings.Split(os.Getenv("IPTV_TUNERR_PROVIDER_URLS"), ",") {
		if ref := ProviderXMLTVURLWithSuffix(base, defaultUser, defaultPass, suffix); ref != "" {
			refs = append(refs, ref)
		}
	}
	for _, kv := range os.Environ() {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(key, "IPTV_TUNERR_PROVIDER_URL_") {
			index := strings.TrimPrefix(key, "IPTV_TUNERR_PROVIDER_URL_")
			user := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_USER_" + index))
			pass := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_PASS_" + index))
			if user == "" {
				user = defaultUser
			}
			if pass == "" {
				pass = defaultPass
			}
			if ref := ProviderXMLTVURLWithSuffix(value, user, pass, suffix); ref != "" {
				refs = append(refs, ref)
			}
		}
	}
	return refs
}

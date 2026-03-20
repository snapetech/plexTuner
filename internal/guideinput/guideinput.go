package guideinput

import (
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

func ProviderXMLTVURL(baseURL, user, pass string) string {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	user = strings.TrimSpace(user)
	pass = strings.TrimSpace(pass)
	if baseURL == "" || user == "" || pass == "" {
		return ""
	}
	return baseURL + "/xmltv.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
}

func LoadGuideData(ref string) ([]byte, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, nil
	}
	if remote, ok, err := prepareRemoteGuideRef(ref); err != nil {
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
	if strings.TrimSpace(ref) == "" {
		return epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}, nil
	}
	r, err := openGuideRef(ref)
	if err != nil {
		return epglink.AliasOverrides{}, err
	}
	defer r.Close()
	return epglink.LoadAliasOverrides(r)
}

func LoadXMLTVChannels(ref string) ([]epglink.XMLTVChannel, error) {
	r, err := openGuideRef(ref)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return epglink.ParseXMLTVChannels(r)
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

func openGuideRef(ref string) (io.ReadCloser, error) {
	if remote, ok, err := prepareRemoteGuideRef(ref); err != nil {
		return nil, err
	} else if ok {
		return openRemoteGuideRef(context.Background(), remote)
	}
	local, err := refio.PrepareLocalFileRef(ref)
	if err != nil {
		return nil, err
	}
	return os.Open(local.Path())
}

func prepareRemoteGuideRef(ref string) (refio.RemoteHTTPRef, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || !strings.HasPrefix(strings.ToLower(ref), "http") {
		return refio.RemoteHTTPRef{}, false, nil
	}
	remote, err := refio.PrepareRemoteHTTPRef(context.Background(), ref)
	if err != nil {
		return refio.RemoteHTTPRef{}, false, err
	}
	return remote, true, nil
}

func openRemoteGuideRef(ctx context.Context, ref refio.RemoteHTTPRef) (io.ReadCloser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.URL(), nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("User-Agent", guideInputUserAgent)
	resp, err := httpclient.WithTimeout(defaultTimeout).Do(req)
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

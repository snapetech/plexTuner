package guideinput

import (
	"context"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/refio"
)

const defaultTimeout = 45 * time.Second

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
		return refio.ReadRemoteHTTP(context.Background(), remote, defaultTimeout)
	}
	local, err := refio.PrepareLocalFileRef(ref)
	if err != nil {
		return nil, err
	}
	return refio.ReadLocalFile(local)
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
		return refio.OpenRemoteHTTP(context.Background(), remote, defaultTimeout)
	}
	local, err := refio.PrepareLocalFileRef(ref)
	if err != nil {
		return nil, err
	}
	return refio.OpenLocalFile(local)
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

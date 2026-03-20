package guideinput

import (
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
	return refio.ReadAll(ref, defaultTimeout)
}

func LoadAliasOverrides(ref string) (epglink.AliasOverrides, error) {
	if strings.TrimSpace(ref) == "" {
		return epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}, nil
	}
	r, err := refio.Open(ref, defaultTimeout)
	if err != nil {
		return epglink.AliasOverrides{}, err
	}
	defer r.Close()
	return epglink.LoadAliasOverrides(r)
}

func LoadXMLTVChannels(ref string) ([]epglink.XMLTVChannel, error) {
	r, err := refio.Open(ref, defaultTimeout)
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

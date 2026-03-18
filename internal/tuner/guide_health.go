package tuner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/epgdoctor"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
)

func (x *XMLTV) GuideHealth(now time.Time, aliasesRef string) (guidehealth.Report, error) {
	x.mu.RLock()
	data := append([]byte(nil), x.cachedXML...)
	x.mu.RUnlock()
	matchRep, err := x.buildMatchReport(aliasesRef)
	if err != nil {
		return guidehealth.Report{}, err
	}
	return guidehealth.Build(x.Channels, data, matchRep, now)
}

func (x *XMLTV) EPGDoctor(now time.Time, aliasesRef string) (epgdoctor.Report, error) {
	x.mu.RLock()
	data := append([]byte(nil), x.cachedXML...)
	x.mu.RUnlock()
	matchRep, err := x.buildMatchReport(aliasesRef)
	if err != nil {
		return epgdoctor.Report{}, err
	}
	gh, err := guidehealth.Build(x.Channels, data, matchRep, now)
	if err != nil {
		return epgdoctor.Report{}, err
	}
	return epgdoctor.Build(gh, matchRep, now), nil
}

func (x *XMLTV) buildMatchReport(aliasesRef string) (*epglink.Report, error) {
	x.mu.RLock()
	if x.cachedMatchReport != nil && x.cachedMatchAliases == strings.TrimSpace(aliasesRef) && x.cachedMatchExp.Equal(x.cacheExp) {
		rep := *x.cachedMatchReport
		x.mu.RUnlock()
		return &rep, nil
	}
	currentCacheExp := x.cacheExp
	x.mu.RUnlock()
	if len(x.Channels) == 0 {
		rep := epglink.Report{TotalChannels: 0, Methods: map[string]int{}}
		return &rep, nil
	}
	aliases, err := loadGuideHealthAliases(aliasesRef)
	if err != nil {
		return nil, err
	}
	type source struct {
		ref string
		ch  []epglink.XMLTVChannel
	}
	var sources []source
	if x.ProviderEPGEnabled {
		if ref := providerXMLTVURL(x.ProviderBaseURL, x.ProviderUser, x.ProviderPass); ref != "" {
			if chans, err := loadGuideHealthXMLTVChannels(ref); err == nil && len(chans) > 0 {
				sources = append(sources, source{ref: ref, ch: chans})
			}
		}
	}
	if ref := strings.TrimSpace(x.SourceURL); ref != "" {
		if chans, err := loadGuideHealthXMLTVChannels(ref); err == nil && len(chans) > 0 {
			sources = append(sources, source{ref: ref, ch: chans})
		}
	}
	if len(sources) == 0 {
		rep := epglink.Report{TotalChannels: len(x.Channels), Unmatched: len(x.Channels), Methods: map[string]int{}}
		for _, ch := range x.Channels {
			rep.Rows = append(rep.Rows, epglink.ChannelMatch{
				ChannelID:   ch.ChannelID,
				GuideNumber: ch.GuideNumber,
				GuideName:   ch.GuideName,
				TVGID:       ch.TVGID,
				EPGLinked:   ch.EPGLinked,
				Reason:      "no xmltv source available",
			})
		}
		return &rep, nil
	}
	protected := make(map[string]bool, len(x.Channels))
	final := epglink.Report{TotalChannels: len(x.Channels), Methods: map[string]int{}, Rows: make([]epglink.ChannelMatch, 0, len(x.Channels))}
	byChannelID := map[string]epglink.ChannelMatch{}
	for _, src := range sources {
		candidates := make([]catalog.LiveChannel, 0, len(x.Channels))
		for _, ch := range x.Channels {
			if protected[ch.ChannelID] {
				continue
			}
			candidates = append(candidates, ch)
		}
		if len(candidates) == 0 {
			break
		}
		rep := epglink.MatchLiveChannels(candidates, src.ch, aliases)
		for _, row := range rep.Rows {
			if row.Matched {
				protected[row.ChannelID] = true
			}
			if existing, ok := byChannelID[row.ChannelID]; !ok || (!existing.Matched && row.Matched) {
				byChannelID[row.ChannelID] = row
			}
		}
	}
	for _, ch := range x.Channels {
		row, ok := byChannelID[ch.ChannelID]
		if !ok {
			row = epglink.ChannelMatch{
				ChannelID:   ch.ChannelID,
				GuideNumber: ch.GuideNumber,
				GuideName:   ch.GuideName,
				TVGID:       ch.TVGID,
				EPGLinked:   ch.EPGLinked,
				Reason:      "no deterministic match",
			}
		}
		if row.Matched {
			final.Matched++
			final.Methods[string(row.Method)]++
		}
		final.Rows = append(final.Rows, row)
	}
	final.Unmatched = final.TotalChannels - final.Matched
	x.mu.Lock()
	repCopy := final
	x.cachedMatchReport = &repCopy
	x.cachedMatchAliases = strings.TrimSpace(aliasesRef)
	x.cachedMatchExp = currentCacheExp
	x.mu.Unlock()
	return &final, nil
}

func loadGuideHealthAliases(ref string) (epglink.AliasOverrides, error) {
	if strings.TrimSpace(ref) == "" {
		return epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}, nil
	}
	r, err := openGuideHealthRef(ref)
	if err != nil {
		return epglink.AliasOverrides{}, err
	}
	defer r.Close()
	return epglink.LoadAliasOverrides(r)
}

func loadGuideHealthXMLTVChannels(ref string) ([]epglink.XMLTVChannel, error) {
	r, err := openGuideHealthRef(ref)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return epglink.ParseXMLTVChannels(r)
}

func openGuideHealthRef(ref string) (io.ReadCloser, error) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ref, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "IptvTunerr/1.0")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			return nil, fmt.Errorf("http %d", resp.StatusCode)
		}
		return resp.Body, nil
	}
	return os.Open(ref)
}

func providerXMLTVURL(baseURL, user, pass string) string {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	user = strings.TrimSpace(user)
	pass = strings.TrimSpace(pass)
	if baseURL == "" || user == "" || pass == "" {
		return ""
	}
	return baseURL + "/xmltv.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
}

func (s *Server) serveGuideHealth() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aliasesRef := strings.TrimSpace(r.URL.Query().Get("aliases"))
		if aliasesRef == "" {
			aliasesRef = strings.TrimSpace(os.Getenv("IPTV_TUNERR_XMLTV_ALIASES"))
		}
		rep, err := s.xmltv.GuideHealth(time.Now(), aliasesRef)
		if err != nil {
			http.Error(w, "guide health unavailable: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rep)
	})
}

func (s *Server) serveEPGDoctor() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aliasesRef := strings.TrimSpace(r.URL.Query().Get("aliases"))
		if aliasesRef == "" {
			aliasesRef = strings.TrimSpace(os.Getenv("IPTV_TUNERR_XMLTV_ALIASES"))
		}
		rep, err := s.xmltv.EPGDoctor(time.Now(), aliasesRef)
		if err != nil {
			http.Error(w, "epg doctor unavailable: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rep)
	})
}

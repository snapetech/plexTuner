package gracenote

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	epgBaseDefault = "https://epg.provider.plex.tv"
	harvestDelay   = 250 * time.Millisecond
	harvestTimeout = 30 * time.Second
	userAgent      = "PlexTuner/1.0 (+plex-gracenote-harvest)"
)

// HarvestFromPlex queries the plex.tv Gracenote EPG API for all world regions
// and merges the results into db.  regionFilter (nil = all) narrows which regions
// are harvested.  langFilter (nil = all) keeps only channels with matching
// language codes (case-insensitive).  Returns the number of channels added and
// the total in the DB after merging.
func HarvestFromPlex(token string, db *DB, regionFilter []string, langFilter []string) (added int, total int, err error) {
	if strings.TrimSpace(token) == "" {
		return 0, 0, fmt.Errorf("token required")
	}

	// Build a quick lookup set for region filter.
	wantRegion := map[string]bool{}
	for _, r := range regionFilter {
		wantRegion[strings.ToLower(strings.TrimSpace(r))] = true
	}

	// Build a quick lookup set for language filter.
	wantLang := map[string]bool{}
	for _, l := range langFilter {
		wantLang[strings.ToLower(strings.TrimSpace(l))] = true
	}

	client := &http.Client{Timeout: harvestTimeout}

	// Track seen lineup IDs to avoid refetching the same lineup from multiple postal codes.
	seenLineup := map[string]bool{}

	for region, probes := range worldRegions {
		if len(wantRegion) > 0 && !wantRegion[strings.ToLower(region)] {
			continue
		}
		log.Printf("gracenote harvest: region %q (%d probes)", region, len(probes))

		for _, probe := range probes {
			country, postal := probe[0], probe[1]
			lineups, err := fetchLineups(client, token, country, postal)
			if err != nil {
				log.Printf("gracenote harvest: lineups %s/%s: %v", country, postal, err)
				time.Sleep(harvestDelay)
				continue
			}

			for _, luID := range lineups {
				if seenLineup[luID] {
					continue
				}
				seenLineup[luID] = true

				channels, err := fetchLineupChannels(client, token, luID)
				if err != nil {
					log.Printf("gracenote harvest: channels lineup=%s: %v", luID, err)
					time.Sleep(harvestDelay)
					continue
				}
				incoming := make([]Channel, 0, len(channels))
				for _, ch := range channels {
					if len(wantLang) > 0 {
						if !wantLang[strings.ToLower(ch.Language)] {
							continue
						}
					}
					incoming = append(incoming, ch)
				}
				n := db.Merge(incoming)
				added += n
				time.Sleep(harvestDelay)
			}
		}
	}
	return added, db.Len(), nil
}

// --- plex.tv EPG API helpers -------------------------------------------------

type lineupRaw struct {
	ID string `json:"id"`
}

type channelRaw struct {
	GridKey  string `json:"gridKey"`
	CallSign string `json:"callSign"`
	Title    string `json:"title"`
	Language string `json:"language"`
	IsHD     bool   `json:"isHd"`
}

func fetchLineups(client *http.Client, token, country, postal string) ([]string, error) {
	u := fmt.Sprintf("%s/lineups?X-Plex-Token=%s&country=%s&postalCode=%s",
		epgBaseDefault, url.QueryEscape(token), url.QueryEscape(country), url.QueryEscape(postal))
	body, err := doGET(client, u)
	if err != nil {
		return nil, err
	}
	var resp struct {
		MediaContainer struct {
			Lineup []lineupRaw `json:"Lineup"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse lineups response: %w", err)
	}
	ids := make([]string, 0, len(resp.MediaContainer.Lineup))
	for _, l := range resp.MediaContainer.Lineup {
		if strings.TrimSpace(l.ID) != "" {
			ids = append(ids, l.ID)
		}
	}
	return ids, nil
}

func fetchLineupChannels(client *http.Client, token, lineupID string) ([]Channel, error) {
	u := fmt.Sprintf("%s/lineups/%s/channels?X-Plex-Token=%s",
		epgBaseDefault, url.PathEscape(lineupID), url.QueryEscape(token))
	body, err := doGET(client, u)
	if err != nil {
		return nil, err
	}
	var resp struct {
		MediaContainer struct {
			Channel []channelRaw `json:"Channel"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse channels response: %w", err)
	}
	out := make([]Channel, 0, len(resp.MediaContainer.Channel))
	for _, rc := range resp.MediaContainer.Channel {
		gk := strings.TrimSpace(rc.GridKey)
		if gk == "" {
			continue
		}
		out = append(out, Channel{
			GridKey:  gk,
			CallSign: rc.CallSign,
			Title:    rc.Title,
			Language: rc.Language,
			IsHD:     rc.IsHD,
		})
	}
	return out, nil
}

func doGET(client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}

// --- world region postal codes -----------------------------------------------
// Derived from plex-wizard-epg-harvest.py WORLD_REGIONS; only includes entries
// that actually return ≥1 lineup from epg.provider.plex.tv.
// Each entry is [country_code, postal_code].

var worldRegions = map[string][][2]string{
	"North America — US": {
		{"US", "10001"},
		{"US", "90210"},
		{"US", "60601"},
		{"US", "77001"},
		{"US", "85001"},
		{"US", "19103"},
		{"US", "78201"},
		{"US", "92101"},
		{"US", "75201"},
		{"US", "95101"},
		{"US", "98101"},
		{"US", "02101"},
		{"US", "80202"},
		{"US", "37201"},
		{"US", "28201"},
		{"US", "97201"},
		{"US", "89101"},
		{"US", "70112"},
		{"US", "55401"},
		{"US", "44101"},
		{"US", "78520"},
	},
	"North America — Canada": {
		{"CA", "S4P3Y2"},
		{"CA", "T5A0A1"},
		{"CA", "H1A0A1"},
		{"CA", "V5K0A1"},
		{"CA", "K1A0A1"},
		{"CA", "M5V0A1"},
		{"CA", "R3B0A1"},
		{"CA", "B3H0A1"},
		{"CA", "E1A0A1"},
		{"CA", "G1A0A1"},
		{"CA", "X1A0A1"},
	},
	"Mexico / Central America": {
		{"MX", "06600"},
		{"MX", "44100"},
		{"MX", "64000"},
		{"MX", "20000"},
	},
	"South America": {
		{"CO", "110111"},
		{"PE", "15001"},
		{"EC", "170101"},
		{"UY", "11000"},
		{"CR", "10101"},
		{"PR", "00901"},
	},
	"Western Europe": {
		{"DE", "10115"},
		{"FR", "75001"},
		{"IT", "00100"},
		{"ES", "28001"},
		{"PT", "1000-001"},
		{"AT", "1010"},
		{"CH", "8001"},
		{"BE", "1000"},
		{"NL", "1011 AA"},
		{"IE", "D01"},
	},
	"Nordic Europe": {
		{"SE", "11120"},
		{"NO", "0150"},
		{"DK", "1000"},
		{"FI", "00100"},
	},
	"Eastern Europe": {
		{"PL", "00-001"},
	},
	"Oceania": {
		{"AU", "2000"},
		{"AU", "3000"},
		{"AU", "4000"},
		{"AU", "6000"},
		{"NZ", "1010"},
	},
	"India": {
		{"IN", "110001"},
		{"IN", "400001"},
		{"IN", "700001"},
		{"IN", "600001"},
	},
}

// Package plex provides optional programmatic registration of our tuner and XMLTV
// with Plex Media Server via API.
package plex

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

const (
	identifierDVR   = "tv.plex.grabbers.hdhomerun"
	identifierXMLTV = "tv.plex.providers.epg.xmltv"
)

type PlexAPIConfig struct {
	BaseURL      string
	PlexHost     string
	PlexToken    string
	FriendlyName string
	DeviceID     string
}

type DeviceInfo struct {
	Key  string
	UUID string
	URI  string
}

type DVRInfo struct {
	Key         int
	UUID        string
	LineupTitle string
	DeviceKey   string
}

func RegisterTunerViaAPI(cfg PlexAPIConfig) (*DeviceInfo, error) {
	baseURL := cfg.BaseURL
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "http://" + baseURL
	}
	deviceURI := baseURL
	client := &http.Client{Timeout: 30 * time.Second}
	var body []byte
	var lastErr error
	// Try each path in order. The /media/grabbers/devices/discover endpoint returns 200
	// but only lists already-registered devices (doesn't include the just-registered one).
	// The hdhomerun-specific path registers AND returns the newly added device.
	// We therefore try all paths and use the first response that contains our device.
	devicePaths := []string{
		"/media/grabbers/tv.plex.grabbers.hdhomerun/devices", // always returns the registered device
		"/media/grabbers/devices/discover",                   // newer path; 200 but may not include new device
		"/media/grabbers/devices",
	}

	tryParse := func(b []byte) *DeviceInfo {
		var mc MediaContainer
		if err := xml.Unmarshal(b, &mc); err != nil {
			return nil
		}
		for _, dev := range mc.Device {
			if dev.URI == deviceURI {
				fmt.Printf("[PLEX-REG] Found device: key=%s uuid=%s uri=%s\n", dev.Key, dev.UUID, dev.URI)
				return &DeviceInfo{Key: dev.Key, UUID: dev.UUID, URI: dev.URI}
			}
		}
		// Fall back to matching by deviceId (Plex may normalise the stored URI).
		for _, dev := range mc.Device {
			if strings.EqualFold(strings.TrimSpace(dev.Name), strings.TrimSpace(cfg.DeviceID)) ||
				strings.EqualFold(strings.TrimSpace(dev.DeviceID), strings.TrimSpace(cfg.DeviceID)) {
				fmt.Printf("[PLEX-REG] Found device by name/deviceId fallback: key=%s uuid=%s uri=%s name=%s deviceId=%s\n", dev.Key, dev.UUID, dev.URI, dev.Name, dev.DeviceID)
				return &DeviceInfo{Key: dev.Key, UUID: dev.UUID, URI: dev.URI}
			}
		}
		return nil
	}

	for _, p := range devicePaths {
		deviceURL := fmt.Sprintf("http://%s%s?uri=%s", cfg.PlexHost, p, url.QueryEscape(deviceURI))
		req, err := http.NewRequest("POST", deviceURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Plex-Token", cfg.PlexToken)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("register device request failed via %s: %w", p, err)
			continue
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("[PLEX-REG] Register device response (%s): status=%d body_len=%d\n", p, resp.StatusCode, len(body))

		if resp.StatusCode == 404 {
			lastErr = fmt.Errorf("register device endpoint %s returned 404", p)
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("register device via %s returned %d: %s", p, resp.StatusCode, string(body))
		}
		if info := tryParse(body); info != nil {
			return info, nil
		}
		// 200 but device not in response yet. Plex may need a moment to persist
		// the registration. Poll /media/grabbers/devices (full list) up to 3 times.
		for attempt := 0; attempt < 3; attempt++ {
			time.Sleep(2 * time.Second)
			listURL := fmt.Sprintf("http://%s/media/grabbers/devices?X-Plex-Token=%s", cfg.PlexHost, cfg.PlexToken)
			listReq, _ := http.NewRequest("GET", listURL, nil)
			if listResp, err := client.Do(listReq); err == nil {
				listBody, _ := io.ReadAll(listResp.Body)
				listResp.Body.Close()
				if info := tryParse(listBody); info != nil {
					fmt.Printf("[PLEX-REG] Found device via poll of /media/grabbers/devices (attempt %d)\n", attempt+1)
					return info, nil
				}
			}
		}
		lastErr = fmt.Errorf("register device via %s: device not found in response after retries", p)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("device not found in response")
}

type MediaContainer struct {
	Device         []Device         `xml:"Device"`
	Dvr            []Dvr            `xml:"Dvr"`
	Channel        []Channel        `xml:"Channel"`
	ChannelMapping []ChannelMapping `xml:"ChannelMapping"`
}

type Device struct {
	Key      string `xml:"key,attr"`
	UUID     string `xml:"uuid,attr"`
	URI      string `xml:"uri,attr"`
	Name     string `xml:"name,attr"`
	DeviceID string `xml:"deviceId,attr"`
	IP       string `xml:"ipAddress,attr"`
	Port     string `xml:"port,attr"`
}

type Dvr struct {
	Key           string   `xml:"key,attr"`
	UUID          string   `xml:"uuid,attr"`
	Title         string   `xml:"title,attr"`
	LineupTitle   string   `xml:"lineupTitle,attr"`
	EPGIdentifier string   `xml:"epgIdentifier,attr"`
	DeviceKey     string   `xml:"deviceKey,attr"`
	Lineups       []Lineup `xml:"Lineup"`
}

type Lineup struct {
	ID string `xml:"id,attr"`
}

func CreateDVRViaAPI(cfg PlexAPIConfig, deviceInfo *DeviceInfo) (dvrKey int, dvrUUID string, lineupIDs []string, err error) {
	xmltvURL := cfg.BaseURL + "/guide.xml"
	xmltvEncoded := url.QueryEscape(xmltvURL)
	lineup := fmt.Sprintf("lineup://tv.plex.providers.epg.xmltv/%s#%s", xmltvEncoded, url.QueryEscape(cfg.FriendlyName))

	dvrURL := fmt.Sprintf("http://%s/livetv/dvrs?language=eng&device=%s&lineup=%s",
		cfg.PlexHost, url.QueryEscape(deviceInfo.UUID), url.QueryEscape(lineup))

	fmt.Printf("[PLEX-REG] Creating DVR: url=%s\n", dvrURL)

	req, err := http.NewRequest("POST", dvrURL, nil)
	if err != nil {
		return 0, "", nil, err
	}
	req.Header.Set("X-Plex-Token", cfg.PlexToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, fmt.Errorf("create DVR request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if len(bodyStr) > 500 {
		bodyStr = bodyStr[:500]
	}
	fmt.Printf("[PLEX-REG] Create DVR response: status=%d body_len=%d body=%s\n", resp.StatusCode, len(body), bodyStr)

	if resp.StatusCode != 200 {
		return 0, "", nil, fmt.Errorf("create DVR returned %d: %s", resp.StatusCode, string(body))
	}

	var mc MediaContainer
	if err := xml.Unmarshal(body, &mc); err != nil {
		return 0, "", nil, fmt.Errorf("parse DVR response: %w", err)
	}

	if len(mc.Dvr) == 0 {
		return 0, "", nil, fmt.Errorf("no DVR in response")
	}

	dvr := mc.Dvr[0]
	key, _ := strconv.Atoi(dvr.Key)

	lineupIDs = make([]string, 0)
	for _, lu := range dvr.Lineups {
		lineupIDs = append(lineupIDs, lu.ID)
	}

	fmt.Printf("[PLEX-REG] Created DVR: key=%d uuid=%s lineupIDs=%v\n", key, dvr.UUID, lineupIDs)
	return key, dvr.UUID, lineupIDs, nil
}

func ReloadGuideAPI(plexHost, token string, dvrKey int) error {
	reloadURL := fmt.Sprintf("http://%s/livetv/dvrs/%d/reloadGuide?X-Plex-Token=%s",
		plexHost, dvrKey, token)

	fmt.Printf("[PLEX-REG] Reloading guide: url=%s\n", reloadURL)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(reloadURL, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("[PLEX-REG] Reload guide response: status=%d\n", resp.StatusCode)

	return nil
}

type ChannelMapping struct {
	ChannelKey       string `xml:"channelKey,attr"`
	DeviceIdentifier string `xml:"deviceIdentifier,attr"`
	LineupIdentifier string `xml:"lineupIdentifier,attr"`
}

func GetChannelMap(plexHost, token, deviceUUID string, lineupIDs []string) ([]ChannelMapping, error) {
	if len(lineupIDs) == 0 {
		return nil, fmt.Errorf("no lineup IDs provided")
	}

	chMapURL := fmt.Sprintf("http://%s/livetv/epg/channelmap?device=%s&lineup=%s&X-Plex-Token=%s",
		plexHost, url.QueryEscape(deviceUUID), url.QueryEscape(lineupIDs[0]), token)

	fmt.Printf("[PLEX-REG] Getting channel map: url=%s\n", chMapURL)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(chMapURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[PLEX-REG] Channel map response: status=%d body_len=%d\n", resp.StatusCode, len(body))

	var mc MediaContainer
	if err := xml.Unmarshal(body, &mc); err != nil {
		return nil, fmt.Errorf("parse channelmap response: %w", err)
	}

	channels := make([]ChannelMapping, 0, len(mc.ChannelMapping))
	for _, ch := range mc.ChannelMapping {
		channels = append(channels, ChannelMapping{
			ChannelKey:       ch.ChannelKey,
			DeviceIdentifier: ch.DeviceIdentifier,
			LineupIdentifier: ch.LineupIdentifier,
		})
	}

	fmt.Printf("[PLEX-REG] Found %d channel mappings\n", len(channels))
	return channels, nil
}

func ActivateChannelsAPI(cfg PlexAPIConfig, deviceKey string, channels []ChannelMapping) (int, error) {
	if len(channels) == 0 {
		return 0, fmt.Errorf("no channels to activate")
	}

	enabled := make([]string, 0, len(channels))
	parts := make([]string, 0, len(channels)*2)

	for _, ch := range channels {
		enabled = append(enabled, ch.DeviceIdentifier)
		parts = append(parts, fmt.Sprintf("channelMappingByKey[%s]=%s", ch.DeviceIdentifier, url.QueryEscape(ch.ChannelKey)))
		parts = append(parts, fmt.Sprintf("channelMapping[%s]=%s", ch.DeviceIdentifier, url.QueryEscape(ch.LineupIdentifier)))
	}

	query := "channelsEnabled=" + strings.Join(enabled, ",") + "&" + strings.Join(parts, "&")

	activateURL := fmt.Sprintf("http://%s/media/grabbers/devices/%s/channelmap?%s&X-Plex-Token=%s",
		cfg.PlexHost, deviceKey, query, cfg.PlexToken)

	fmt.Printf("[PLEX-REG] Activating channels: url_len=%d channels=%d\n", len(activateURL), len(channels))

	req, err := http.NewRequest("PUT", activateURL, nil)
	if err != nil {
		return 0, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("activate channels request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if len(bodyStr) > 200 {
		bodyStr = bodyStr[:200]
	}
	fmt.Printf("[PLEX-REG] Activate channels response: status=%d body=%s\n", resp.StatusCode, bodyStr)

	return len(channels), nil
}

func ListDVRsAPI(plexHost, token string) ([]DVRInfo, error) {
	u := fmt.Sprintf("http://%s/livetv/dvrs", plexHost)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Token", token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("list dvrs returned %d: %s", resp.StatusCode, string(body))
	}
	var mc MediaContainer
	if err := xml.NewDecoder(resp.Body).Decode(&mc); err != nil {
		return nil, err
	}
	out := make([]DVRInfo, 0, len(mc.Dvr))
	for _, d := range mc.Dvr {
		k, _ := strconv.Atoi(strings.TrimSpace(d.Key))
		title := strings.TrimSpace(d.LineupTitle)
		if title == "" {
			title = strings.TrimSpace(d.Title)
		}
		out = append(out, DVRInfo{
			Key:         k,
			UUID:        strings.TrimSpace(d.UUID),
			LineupTitle: title,
			DeviceKey:   strings.TrimSpace(d.DeviceKey),
		})
	}
	return out, nil
}

func ListDevicesAPI(plexHost, token string) ([]Device, error) {
	paths := []string{
		"/media/grabbers/devices",
		"/media/grabbers/tv.plex.grabbers.hdhomerun/devices",
	}
	client := &http.Client{Timeout: 30 * time.Second}
	var lastErr error
	for _, p := range paths {
		u := fmt.Sprintf("http://%s%s", plexHost, p)
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Plex-Token", token)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s returned 404", p)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			lastErr = fmt.Errorf("%s returned %d: %s", p, resp.StatusCode, string(body))
			continue
		}
		var mc MediaContainer
		err = xml.NewDecoder(resp.Body).Decode(&mc)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		return mc.Device, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no device endpoint succeeded")
	}
	return nil, lastErr
}

func DeleteDVRAPI(plexHost, token string, dvrKey int) error {
	u := fmt.Sprintf("http://%s/livetv/dvrs/%d", plexHost, dvrKey)
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Plex-Token", token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("delete dvr %d returned %d: %s", dvrKey, resp.StatusCode, string(body))
	}
	return nil
}

func DeleteDeviceAPI(plexHost, token, deviceKey string) error {
	paths := []string{
		fmt.Sprintf("/media/grabbers/devices/%s", url.PathEscape(deviceKey)),
		fmt.Sprintf("/media/grabbers/tv.plex.grabbers.hdhomerun/devices/%s", url.PathEscape(deviceKey)),
	}
	client := &http.Client{Timeout: 30 * time.Second}
	var lastErr error
	for _, p := range paths {
		u := fmt.Sprintf("http://%s%s", plexHost, p)
		req, err := http.NewRequest(http.MethodDelete, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-Plex-Token", token)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s returned 404", p)
			continue
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			lastErr = fmt.Errorf("%s returned %d: %s", p, resp.StatusCode, string(body))
			continue
		}
		resp.Body.Close()
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("delete device %s failed", deviceKey)
	}
	return lastErr
}

type Channel struct {
	Key         string `xml:"key,attr"`
	GuideNumber string `xml:"guideNumber,attr"`
	GuideName   string `xml:"guideName,attr"`
}

type ChannelInfo struct {
	GuideNumber string
	GuideName   string
}

func FullRegisterPlex(baseURL, plexHost, plexToken, friendlyName, deviceID string, channels []ChannelInfo) error {
	cfg := PlexAPIConfig{
		BaseURL:      baseURL,
		PlexHost:     plexHost,
		PlexToken:    plexToken,
		FriendlyName: friendlyName,
		DeviceID:     deviceID,
	}

	tokenPreview := plexToken
	if len(tokenPreview) > 8 {
		tokenPreview = tokenPreview[:8] + "..."
	}

	fmt.Printf("[PLEX-REG] === Starting Plex API registration ===\n")
	fmt.Printf("[PLEX-REG] BaseURL=%s Host=%s Token=%s\n", baseURL, plexHost, tokenPreview)

	fmt.Printf("[PLEX-REG] Step 1: Register HDHR device...\n")
	deviceInfo, err := RegisterTunerViaAPI(cfg)
	if err != nil {
		return fmt.Errorf("register device: %w", err)
	}
	fmt.Printf("[PLEX-REG] Device registered: key=%s uuid=%s\n", deviceInfo.Key, deviceInfo.UUID)

	fmt.Printf("[PLEX-REG] Step 2: Create DVR...\n")
	dvrKey, dvrUUID, lineupIDs, err := CreateDVRViaAPI(cfg, deviceInfo)
	if err != nil {
		return fmt.Errorf("create DVR: %w", err)
	}
	fmt.Printf("[PLEX-REG] DVR created: key=%d uuid=%s\n", dvrKey, dvrUUID)

	fmt.Printf("[PLEX-REG] Step 3: Reload guide...\n")
	if err := ReloadGuideAPI(plexHost, plexToken, dvrKey); err != nil {
		fmt.Printf("[PLEX-REG] Warning: reload guide failed: %v\n", err)
	}

	fmt.Printf("[PLEX-REG] Step 4: Wait for guide to populate...\n")
	time.Sleep(15 * time.Second)

	fmt.Printf("[PLEX-REG] Step 5: Get channel mappings...\n")
	chMappings, err := GetChannelMap(plexHost, plexToken, deviceInfo.UUID, lineupIDs)
	if err != nil {
		return fmt.Errorf("get channel map: %w", err)
	}
	fmt.Printf("[PLEX-REG] Found %d channel mappings\n", len(chMappings))

	if len(chMappings) > 0 {
		fmt.Printf("[PLEX-REG] Step 6: Activate channels...\n")
		activated, err := ActivateChannelsAPI(cfg, deviceInfo.Key, chMappings)
		if err != nil {
			return fmt.Errorf("activate channels: %w", err)
		}
		fmt.Printf("[PLEX-REG] Activated %d channels\n", activated)
	} else {
		fmt.Printf("[PLEX-REG] No channel mappings found - guide may not have been loaded yet\n")
	}

	fmt.Printf("[PLEX-REG] === Plex DVR setup complete! ===\n")
	return nil
}

func RegisterTuner(plexDataDir, baseURL string) error {
	parsed, parseErr := url.Parse(baseURL)
	if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("baseURL must be a valid http or https URL: %q", baseURL)
	}
	dbPath := filepath.Join(plexDataDir, "Plug-in Support", "Databases", "com.plexapp.plugins.library.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open Plex DB: %w", err)
	}
	defer db.Close()

	xmltvURL := baseURL + "/guide.xml"
	if err := updateURI(db, identifierDVR, baseURL); err != nil {
		return fmt.Errorf("update DVR URI: %w", err)
	}
	if err := updateURI(db, identifierXMLTV, xmltvURL); err != nil {
		return fmt.Errorf("update XMLTV URI: %w", err)
	}
	return nil
}

func updateURI(db *sql.DB, identifier, rawURI string) error {
	res, err := db.Exec(`UPDATE media_provider_resources SET uri = ? WHERE identifier = ?`, rawURI, identifier)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_, err = db.Exec(`INSERT INTO media_provider_resources (identifier, uri) VALUES (?, ?)`, identifier, rawURI)
		if err != nil {
			return fmt.Errorf("no existing row and insert failed: %w", err)
		}
		return nil
	}
	return nil
}

// FetchTunerLineup fetches /lineup.json from a PlexTuner base URL and returns
// the channels as a slice of LiveChannel. Only GuideNumber, GuideName, and TVGID
// DiscoverJSON represents the minimal subset of a tuner's /discover.json we care about.
type DiscoverJSON struct {
	DeviceID     string `json:"DeviceID"`
	FriendlyName string `json:"FriendlyName"`
	BaseURL      string `json:"BaseURL"`
	LineupURL    string `json:"LineupURL"`
	TunerCount   int    `json:"TunerCount"`
}

// FetchDiscoverJSON fetches /discover.json from a plex-tuner instance.
func FetchDiscoverJSON(baseURL string) (*DiscoverJSON, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	u := baseURL + "/discover.json"
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var d DiscoverJSON
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// are populated (the fields available from lineup.json). This is used by
// plex-epg-oracle to annotate channel names alongside channelmap rows.
func FetchTunerLineup(baseURL string) ([]catalog.LiveChannel, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	u := baseURL + "/lineup.json"
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("fetch lineup.json: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lineup.json returned %d", resp.StatusCode)
	}

	// lineup.json format: [{"GuideNumber":"1","GuideName":"ESPN HD","URL":"..."}]
	var rows []struct {
		GuideNumber string `json:"GuideNumber"`
		GuideName   string `json:"GuideName"`
		URL         string `json:"URL"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("parse lineup.json: %w", err)
	}
	out := make([]catalog.LiveChannel, 0, len(rows))
	for _, r := range rows {
		ch := catalog.LiveChannel{
			GuideNumber: r.GuideNumber,
			GuideName:   r.GuideName,
			StreamURL:   r.URL,
		}
		out = append(out, ch)
	}
	return out, nil
}

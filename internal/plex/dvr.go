// Package plex provides optional programmatic registration of our tuner and XMLTV
// with Plex Media Server via API (like Threadfin does).
package plex

import (
	"database/sql"
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

func RegisterTunerViaAPI(cfg PlexAPIConfig) (*DeviceInfo, error) {
	baseURL := cfg.BaseURL
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "http://" + baseURL
	}
	deviceURI := baseURL
	deviceURL := fmt.Sprintf("http://%s/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=%s",
		cfg.PlexHost, url.QueryEscape(deviceURI))

	req, err := http.NewRequest("POST", deviceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Token", cfg.PlexToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("register device request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[PLEX-REG] Register device response: status=%d body_len=%d\n", resp.StatusCode, len(body))

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("register device returned %d: %s", resp.StatusCode, string(body))
	}

	var mc MediaContainer
	if err := xml.Unmarshal(body, &mc); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	for _, dev := range mc.Device {
		if dev.URI == deviceURI {
			fmt.Printf("[PLEX-REG] Found device: key=%s uuid=%s uri=%s\n", dev.Key, dev.UUID, dev.URI)
			return &DeviceInfo{Key: dev.Key, UUID: dev.UUID, URI: dev.URI}, nil
		}
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
	Key  string `xml:"key,attr"`
	UUID string `xml:"uuid,attr"`
	URI  string `xml:"uri,attr"`
	Name string `xml:"name,attr"`
	IP   string `xml:"ipAddress,attr"`
	Port string `xml:"port,attr"`
}

type Dvr struct {
	Key           string   `xml:"key,attr"`
	UUID          string   `xml:"uuid,attr"`
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

	fmt.Printf("[PLEX-REG] === Starting Threadfin-style registration ===\n")
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

// Package plex provides optional programmatic registration of our tuner and XMLTV
// with Plex Media Server via API.
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

type DVRInfo struct {
	Key         int
	UUID        string
	LineupTitle string
	LineupURL   string   // Dvr.lineup attr value, e.g. "lineup://tv.plex.providers.epg.xmltv/http://host/guide.xml#name"
	DeviceKey   string   // numeric key of the first Device child (e.g. "179"); used for device lookups
	DeviceUUIDs []string // UUIDs of Device children, e.g. ["device://tv.plex.grabbers.hdhomerun/newsus"]
}

func RegisterTunerViaAPI(cfg PlexAPIConfig) (*DeviceInfo, error) {
	baseURL := cfg.BaseURL
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "http://" + baseURL
	}
	deviceURI := baseURL

	// Plex's /media/grabbers/devices/discover endpoint accepts a uri= param intending
	// to trigger a probe of the new device and return it in the response. In practice,
	// Endpoint ordering rationale:
	//   1. /media/grabbers/tv.plex.grabbers.hdhomerun/devices — probes the given uri= and
	//      returns only that device if found. Works correctly on 1.43.x.
	//   2. /media/grabbers/devices/discover — on 1.43.x this endpoint ignores the uri=
	//      param and returns the full list of already-registered devices, so a new device
	//      never appears in the response. Kept as fallback for older/future builds where
	//      it may behave differently.
	//   3. /media/grabbers/devices — generic fallback.
	//
	// After trying all paths, if no device is found in any response we fall back to
	// synthesising the device UUID from DeviceID (last resort; see below).
	client := &http.Client{Timeout: 30 * time.Second}
	var body []byte
	devicePaths := []string{
		"/media/grabbers/tv.plex.grabbers.hdhomerun/devices", // probes the uri= correctly
		"/media/grabbers/devices/discover",                   // may ignore uri=; kept for compat
		"/media/grabbers/devices",
	}
	for i, p := range devicePaths {
		deviceURL := fmt.Sprintf("http://%s%s?uri=%s", cfg.PlexHost, p, url.QueryEscape(deviceURI))
		req, err := http.NewRequest("POST", deviceURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Plex-Token", cfg.PlexToken)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[PLEX-REG] Register device request failed via %s: %v\n", p, err)
			continue
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("[PLEX-REG] Register device response (%s): status=%d body_len=%d\n", p, resp.StatusCode, len(body))

		if resp.StatusCode == 404 && i < len(devicePaths)-1 {
			fmt.Printf("[PLEX-REG] endpoint %s returned 404; trying next\n", p)
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("register device via %s returned %d: %s", p, resp.StatusCode, string(body))
		}
		// Parse this response and check for the device before trying the next path.
		// The /discover endpoint returns all devices (ignoring uri=), so we parse it but
		// still fall through if our device isn't in the list.
		var mc MediaContainer
		if xmlErr := xml.Unmarshal(body, &mc); xmlErr == nil {
			for _, dev := range mc.Device {
				if dev.URI == deviceURI {
					fmt.Printf("[PLEX-REG] Found device via %s: key=%s uuid=%s uri=%s\n", p, dev.Key, dev.UUID, dev.URI)
					return &DeviceInfo{Key: dev.Key, UUID: dev.UUID, URI: dev.URI}, nil
				}
			}
			for _, dev := range mc.Device {
				if strings.EqualFold(strings.TrimSpace(dev.DeviceID), strings.TrimSpace(cfg.DeviceID)) {
					fmt.Printf("[PLEX-REG] Found device by deviceId via %s: key=%s uuid=%s\n", p, dev.Key, dev.UUID)
					return &DeviceInfo{Key: dev.Key, UUID: dev.UUID, URI: dev.URI}, nil
				}
			}
		}
		// Device not in this response; try the next path.
		fmt.Printf("[PLEX-REG] device not found in response from %s; trying next\n", p)
	}
	// All paths exhausted without finding the device. Last resort: synthesise the device
	// UUID from DeviceID using Plex's well-known HDHR URI scheme. Note that a synthesised
	// UUID only works for DVR creation if Plex has already registered the device in its
	// internal database (e.g. via the grabbers endpoint above). If the endpoint probe
	// failed entirely, DVR creation will return "The device does not exist".
	if cfg.DeviceID != "" {
		synthUUID := "device://tv.plex.grabbers.hdhomerun/" + cfg.DeviceID
		fmt.Printf("[PLEX-REG] Device not found via any endpoint; using synthesised UUID: %s\n", synthUUID)
		return &DeviceInfo{Key: "", UUID: synthUUID, URI: deviceURI}, nil
	}

	return nil, fmt.Errorf("device not found in any Plex grabbers response and no DeviceID configured to synthesise UUID")
}

type MediaContainer struct {
	Message        string           `xml:"message,attr"` // set on Plex API error responses, e.g. "The device is in use with an existing DVR"
	Status         string           `xml:"status,attr"`  // "-1" on Plex API errors
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
	Lineup        string   `xml:"lineup,attr"` // decoded lineup URL, e.g. "lineup://tv.plex.providers.epg.xmltv/http://host/guide.xml#name"
	EPGIdentifier string   `xml:"epgIdentifier,attr"`
	Devices       []Device `xml:"Device"`
}

type Lineup struct {
	ID string `xml:"id,attr"`
}

// CreateDVRViaAPI creates a Plex DVR for deviceInfo, or reuses the existing one if the
// guide URL already matches. If a DVR exists for the device but points to a different
// guide URL (stale registration), it is deleted and recreated. This makes the function
// idempotent across restarts and safe across BaseURL changes.
func CreateDVRViaAPI(cfg PlexAPIConfig, deviceInfo *DeviceInfo) (dvrKey int, dvrUUID string, lineupIDs []string, err error) {
	desiredGuideURL := cfg.BaseURL + "/guide.xml"
	xmltvEncoded := url.QueryEscape(desiredGuideURL)
	lineup := fmt.Sprintf("lineup://tv.plex.providers.epg.xmltv/%s#%s", xmltvEncoded, url.QueryEscape(cfg.FriendlyName))

	dvrURL := fmt.Sprintf("http://%s/livetv/dvrs?language=eng&device=%s&lineup=%s",
		cfg.PlexHost, url.QueryEscape(deviceInfo.UUID), url.QueryEscape(lineup))

	client := &http.Client{Timeout: 30 * time.Second}

	for attempt := 0; attempt < 2; attempt++ {
		fmt.Printf("[PLEX-REG] Creating DVR (attempt %d): url=%s\n", attempt+1, dvrURL)

		req, err := http.NewRequest("POST", dvrURL, nil)
		if err != nil {
			return 0, "", nil, err
		}
		req.Header.Set("X-Plex-Token", cfg.PlexToken)

		resp, err := client.Do(req)
		if err != nil {
			return 0, "", nil, fmt.Errorf("create DVR request failed: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

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

		if len(mc.Dvr) > 0 {
			dvr := mc.Dvr[0]
			key, _ := strconv.Atoi(dvr.Key)
			// Dvr.Lineup is the decoded lineup URL; wrap in a slice for GetChannelMap compatibility.
			ids := []string{dvr.Lineup}
			fmt.Printf("[PLEX-REG] DVR created: key=%d uuid=%s lineup=%s\n", key, dvr.UUID, dvr.Lineup)
			return key, dvr.UUID, ids, nil
		}

		// Plex refused to create — only attempt recovery on first pass.
		if attempt > 0 || !strings.Contains(mc.Message, "device is in use") {
			return 0, "", nil, fmt.Errorf("no DVR in response")
		}

		// "device is in use with an existing DVR": look up all DVRs and find the one
		// belonging to our device by matching Device.UUID. Then compare its lineup URL
		// against the guide URL we want to register. If they match, the current
		// registration is already correct — return the existing DVR and proceed to
		// guide reload + channel activation. If they differ (e.g. BaseURL changed),
		// delete the stale DVR and retry creation on the next loop iteration.
		existing, lErr := ListDVRsAPI(cfg.PlexHost, cfg.PlexToken)
		if lErr != nil {
			return 0, "", nil, fmt.Errorf("DVR exists (device in use) but listing failed: %w", lErr)
		}

		matched := false
		for _, d := range existing {
			// Find the DVR that owns our device by UUID.
			ownsDevice := false
			for _, uuid := range d.DeviceUUIDs {
				if uuid == deviceInfo.UUID {
					ownsDevice = true
					break
				}
			}
			if !ownsDevice {
				continue
			}
			matched = true
			// Compare: does the registered lineup URL contain our desired guide URL?
			if strings.Contains(d.LineupURL, desiredGuideURL) {
				fmt.Printf("[PLEX-REG] DVR already registered with matching guide URL (key=%d) — reusing\n", d.Key)
				return d.Key, d.UUID, []string{d.LineupURL}, nil
			}
			// Guide URL mismatch — delete stale DVR and retry creation.
			fmt.Printf("[PLEX-REG] DVR guide URL mismatch (key=%d, registered=%q, desired=%q) — deleting stale DVR\n",
				d.Key, d.LineupURL, desiredGuideURL)
			if dErr := DeleteDVRAPI(cfg.PlexHost, cfg.PlexToken, d.Key); dErr != nil {
				return 0, "", nil, fmt.Errorf("delete stale DVR %d: %w", d.Key, dErr)
			}
			break // proceed to retry on next iteration
		}
		if !matched {
			return 0, "", nil, fmt.Errorf("device in use but no DVR found for device UUID %s", deviceInfo.UUID)
		}
	}
	return 0, "", nil, fmt.Errorf("no DVR in response after retry")
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
		uuids := make([]string, 0, len(d.Devices))
		devKey := ""
		for _, dev := range d.Devices {
			uuids = append(uuids, strings.TrimSpace(dev.UUID))
			if devKey == "" {
				devKey = strings.TrimSpace(dev.Key)
			}
		}
		out = append(out, DVRInfo{
			Key:         k,
			UUID:        strings.TrimSpace(d.UUID),
			LineupTitle: title,
			LineupURL:   strings.TrimSpace(d.Lineup),
			DeviceKey:   devKey,
			DeviceUUIDs: uuids,
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

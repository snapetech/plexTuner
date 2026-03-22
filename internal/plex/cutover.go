package plex

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type CutoverMapRow struct {
	Category     string `json:"category"`
	OldURI       string `json:"old_uri"`
	NewURI       string `json:"new_uri"`
	URIChanged   string `json:"uri_changed,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
	FriendlyName string `json:"friendly_name,omitempty"`
}

type DVRCutoverReport struct {
	PlexURL string             `json:"plex_url"`
	DryRun  bool               `json:"dry_run"`
	Rows    []DVRCutoverResult `json:"rows"`
}

type DVRCutoverResult struct {
	Category            string   `json:"category"`
	OldURI              string   `json:"old_uri,omitempty"`
	NewURI              string   `json:"new_uri,omitempty"`
	DeviceID            string   `json:"device_id,omitempty"`
	FriendlyName        string   `json:"friendly_name,omitempty"`
	MatchedDeviceKey    string   `json:"matched_device_key,omitempty"`
	MatchedDeviceUUID   string   `json:"matched_device_uuid,omitempty"`
	MatchedDeviceStatus string   `json:"matched_device_status,omitempty"`
	MatchedDVRKeys      []int    `json:"matched_dvr_keys,omitempty"`
	DeletedDVRKeys      []int    `json:"deleted_dvr_keys,omitempty"`
	DeletedDeviceKey    string   `json:"deleted_device_key,omitempty"`
	NewDeviceKey        string   `json:"new_device_key,omitempty"`
	NewDeviceUUID       string   `json:"new_device_uuid,omitempty"`
	NewDVRKey           int      `json:"new_dvr_key,omitempty"`
	NewDVRUUID          string   `json:"new_dvr_uuid,omitempty"`
	LineupIDs           []string `json:"lineup_ids,omitempty"`
	ChannelMapRows      int      `json:"channelmap_rows,omitempty"`
	Activated           int      `json:"activated,omitempty"`
	Error               string   `json:"error,omitempty"`
}

func LoadCutoverMap(path string) ([]CutoverMapRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rows := []CutoverMapRow{}
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			return nil, fmt.Errorf("%s:%d: expected at least 3 tab-separated columns", path, lineNo)
		}
		row := CutoverMapRow{
			Category:   strings.TrimSpace(parts[0]),
			OldURI:     strings.TrimSpace(parts[1]),
			NewURI:     strings.TrimSpace(parts[2]),
			URIChanged: fieldAt(parts, 3),
			DeviceID:   fieldAt(parts, 4),
		}
		if row.DeviceID == "" {
			row.DeviceID = row.Category
		}
		row.FriendlyName = fieldAt(parts, 5)
		if row.FriendlyName == "" {
			row.FriendlyName = row.Category
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func CutoverDVRs(plexBaseURL, plexToken string, rows []CutoverMapRow, reloadGuide, activate, doApply bool) (*DVRCutoverReport, error) {
	plexHost, err := hostPortFromBaseURL(plexBaseURL)
	if err != nil {
		return nil, err
	}
	devs, err := ListDevicesAPI(plexHost, plexToken)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	dvrs, err := ListDVRsAPI(plexHost, plexToken)
	if err != nil {
		return nil, fmt.Errorf("list dvrs: %w", err)
	}

	report := &DVRCutoverReport{
		PlexURL: strings.TrimSpace(plexBaseURL),
		DryRun:  !doApply,
		Rows:    make([]DVRCutoverResult, 0, len(rows)),
	}
	for _, row := range rows {
		result := DVRCutoverResult{
			Category:     row.Category,
			OldURI:       row.OldURI,
			NewURI:       row.NewURI,
			DeviceID:     row.DeviceID,
			FriendlyName: row.FriendlyName,
		}
		matchedDevice := findCutoverDevice(devs, row)
		if matchedDevice != nil {
			result.MatchedDeviceKey = matchedDevice.Key
			result.MatchedDeviceUUID = matchedDevice.UUID
			result.MatchedDeviceStatus = firstNonEmptyString(matchedDevice.Status, matchedDevice.State)
			result.MatchedDVRKeys = findCutoverDVRKeys(dvrs, *matchedDevice)
		}
		if !doApply {
			report.Rows = append(report.Rows, result)
			continue
		}
		if strings.TrimSpace(row.NewURI) == "" {
			result.Error = "new_uri is empty"
			report.Rows = append(report.Rows, result)
			continue
		}
		if matchedDevice != nil {
			for _, dvrKey := range result.MatchedDVRKeys {
				if err := DeleteDVRAPI(plexHost, plexToken, dvrKey); err != nil {
					result.Error = fmt.Sprintf("delete dvr %d: %v", dvrKey, err)
					break
				}
				result.DeletedDVRKeys = append(result.DeletedDVRKeys, dvrKey)
			}
			if result.Error == "" {
				if err := DeleteDeviceAPI(plexHost, plexToken, matchedDevice.Key); err != nil {
					result.Error = fmt.Sprintf("delete device %s: %v", matchedDevice.Key, err)
				} else {
					result.DeletedDeviceKey = matchedDevice.Key
				}
			}
		}
		if result.Error != "" {
			report.Rows = append(report.Rows, result)
			continue
		}
		cfg := PlexAPIConfig{
			BaseURL:      row.NewURI,
			PlexHost:     plexHost,
			PlexToken:    plexToken,
			FriendlyName: row.FriendlyName,
			DeviceID:     row.DeviceID,
		}
		dev, err := RegisterTunerViaAPI(cfg)
		if err != nil {
			result.Error = "register device: " + err.Error()
			report.Rows = append(report.Rows, result)
			continue
		}
		result.NewDeviceKey = dev.Key
		result.NewDeviceUUID = dev.UUID
		dvrKey, dvrUUID, lineupIDs, err := CreateDVRViaAPI(cfg, dev)
		if err != nil {
			result.Error = "create dvr: " + err.Error()
			report.Rows = append(report.Rows, result)
			continue
		}
		result.NewDVRKey = dvrKey
		result.NewDVRUUID = dvrUUID
		result.LineupIDs = append(result.LineupIDs, lineupIDs...)
		if reloadGuide {
			if err := ReloadGuideAPI(plexHost, plexToken, dvrKey); err != nil {
				result.Error = "reload guide: " + err.Error()
				report.Rows = append(report.Rows, result)
				continue
			}
		}
		mappings, err := GetChannelMap(plexHost, plexToken, dev.UUID, lineupIDs)
		if err != nil {
			result.Error = "get channelmap: " + err.Error()
			report.Rows = append(report.Rows, result)
			continue
		}
		result.ChannelMapRows = len(mappings)
		if activate {
			n, err := ActivateChannelsAPI(cfg, dev.Key, mappings)
			if err != nil {
				result.Error = "activate channels: " + err.Error()
			} else {
				result.Activated = n
			}
		}
		report.Rows = append(report.Rows, result)
	}
	return report, nil
}

func findCutoverDevice(devs []Device, row CutoverMapRow) *Device {
	for i := range devs {
		dev := &devs[i]
		if row.OldURI != "" && strings.EqualFold(strings.TrimSpace(dev.URI), strings.TrimSpace(row.OldURI)) {
			return dev
		}
	}
	for i := range devs {
		dev := &devs[i]
		if row.DeviceID != "" && strings.EqualFold(strings.TrimSpace(dev.DeviceID), strings.TrimSpace(row.DeviceID)) {
			return dev
		}
	}
	for i := range devs {
		dev := &devs[i]
		if row.Category != "" && strings.HasSuffix(strings.ToLower(strings.TrimSpace(dev.UUID)), "/"+strings.ToLower(strings.TrimSpace(row.Category))) {
			return dev
		}
	}
	return nil
}

func findCutoverDVRKeys(dvrs []DVRInfo, dev Device) []int {
	keys := []int{}
	for _, d := range dvrs {
		if d.DeviceKey == dev.Key {
			keys = append(keys, d.Key)
			continue
		}
		for _, uuid := range d.DeviceUUIDs {
			if uuid == dev.UUID {
				keys = append(keys, d.Key)
				break
			}
		}
	}
	return keys
}

func fieldAt(parts []string, idx int) string {
	if idx >= len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[idx])
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

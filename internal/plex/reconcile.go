package plex

import (
	"fmt"
	"log"
	"sort"
	"strings"
)

type reconcilePlan struct {
	KeepDeviceKey    string
	KeepDVRKey       int
	DeleteDeviceKeys []string
	DeleteDVRKeys    []int
}

func ReconcileTunerrRegistrations(cfg PlexAPIConfig) error {
	devices, err := ListDevicesAPI(cfg.PlexHost, cfg.PlexToken)
	if err != nil {
		return fmt.Errorf("list devices for reconcile: %w", err)
	}
	dvrs, err := ListDVRsAPI(cfg.PlexHost, cfg.PlexToken)
	if err != nil {
		return fmt.Errorf("list dvrs for reconcile: %w", err)
	}
	plan := buildTunerrReconcilePlan(cfg, devices, dvrs)
	if len(plan.DeleteDVRKeys) == 0 && len(plan.DeleteDeviceKeys) == 0 {
		return nil
	}
	for _, dvrKey := range plan.DeleteDVRKeys {
		log.Printf("[PLEX-REG] reconcile: deleting stale dvr=%d", dvrKey)
		if err := DeleteDVRAPI(cfg.PlexHost, cfg.PlexToken, dvrKey); err != nil {
			return fmt.Errorf("delete stale dvr %d: %w", dvrKey, err)
		}
	}
	for _, deviceKey := range plan.DeleteDeviceKeys {
		log.Printf("[PLEX-REG] reconcile: deleting stale device=%s", deviceKey)
		if err := DeleteDeviceAPI(cfg.PlexHost, cfg.PlexToken, deviceKey); err != nil {
			return fmt.Errorf("delete stale device %s: %w", deviceKey, err)
		}
	}
	return nil
}

func buildTunerrReconcilePlan(cfg PlexAPIConfig, devices []Device, dvrs []DVRInfo) reconcilePlan {
	desiredBase := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	desiredGuide := guideURLForBase(cfg.BaseURL)
	type matchedDevice struct {
		Device
		score int
	}
	matched := make([]matchedDevice, 0, len(devices))
	deviceKeys := map[string]struct{}{}
	deviceUUIDs := map[string]struct{}{}
	for _, dev := range devices {
		score := tunerrDeviceMatchScore(dev, cfg, desiredBase)
		if score <= 0 {
			continue
		}
		matched = append(matched, matchedDevice{Device: dev, score: score})
		deviceKeys[strings.TrimSpace(dev.Key)] = struct{}{}
		deviceUUIDs[strings.TrimSpace(dev.UUID)] = struct{}{}
	}
	if len(matched) == 0 {
		return reconcilePlan{}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score != matched[j].score {
			return matched[i].score > matched[j].score
		}
		return strings.TrimSpace(matched[i].Key) < strings.TrimSpace(matched[j].Key)
	})
	keepDeviceKey := ""
	if matched[0].score >= 4 {
		keepDeviceKey = strings.TrimSpace(matched[0].Key)
	}

	associatedDVRs := make([]DVRInfo, 0, len(dvrs))
	for _, dvr := range dvrs {
		if dvrBelongsToMatchedDevices(dvr, deviceKeys, deviceUUIDs) {
			associatedDVRs = append(associatedDVRs, dvr)
		}
	}
	keepDVRKey := 0
	for _, dvr := range associatedDVRs {
		if dvrGuideMatches(dvr, desiredGuide) {
			if keepDVRKey == 0 || dvr.Key < keepDVRKey {
				keepDVRKey = dvr.Key
			}
		}
	}

	plan := reconcilePlan{
		KeepDeviceKey: keepDeviceKey,
		KeepDVRKey:    keepDVRKey,
	}
	for _, dvr := range associatedDVRs {
		if keepDVRKey != 0 && dvr.Key == keepDVRKey {
			continue
		}
		plan.DeleteDVRKeys = append(plan.DeleteDVRKeys, dvr.Key)
	}
	for _, dev := range matched {
		key := strings.TrimSpace(dev.Key)
		if keepDeviceKey != "" && key == keepDeviceKey {
			continue
		}
		plan.DeleteDeviceKeys = append(plan.DeleteDeviceKeys, key)
	}
	sort.Ints(plan.DeleteDVRKeys)
	sort.Strings(plan.DeleteDeviceKeys)
	return plan
}

func tunerrDeviceMatchScore(dev Device, cfg PlexAPIConfig, desiredBase string) int {
	deviceID := strings.TrimSpace(cfg.DeviceID)
	devID := strings.TrimSpace(dev.DeviceID)
	devUUID := strings.TrimSpace(dev.UUID)
	devURI := strings.TrimRight(strings.TrimSpace(dev.URI), "/")
	matchDeviceID := deviceID != "" && (strings.EqualFold(devID, deviceID) || strings.HasSuffix(strings.ToLower(devUUID), "/"+strings.ToLower(deviceID)))
	matchURI := desiredBase != "" && strings.EqualFold(devURI, desiredBase)
	switch {
	case matchDeviceID && matchURI:
		return 4
	case matchDeviceID:
		return 3
	case matchURI:
		return 2
	default:
		return 0
	}
}

func dvrBelongsToMatchedDevices(dvr DVRInfo, deviceKeys, deviceUUIDs map[string]struct{}) bool {
	if _, ok := deviceKeys[strings.TrimSpace(dvr.DeviceKey)]; ok {
		return true
	}
	for _, uuid := range dvr.DeviceUUIDs {
		if _, ok := deviceUUIDs[strings.TrimSpace(uuid)]; ok {
			return true
		}
	}
	return false
}

func dvrGuideMatches(dvr DVRInfo, desiredGuide string) bool {
	lineup := strings.TrimSpace(dvr.LineupURL)
	if lineup == "" || strings.TrimSpace(desiredGuide) == "" {
		return false
	}
	encoded := strings.NewReplacer(":", "%3A", "/", "%2F").Replace(strings.TrimSpace(desiredGuide))
	return strings.Contains(lineup, desiredGuide) || strings.Contains(lineup, encoded)
}

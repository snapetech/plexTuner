package plex

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DVRSyncInstance describes one desired DVR injection: the tuner's base URL and the
// stable device identifier. ReconcileDVRs matches desired instances against what Plex
// already has registered by DeviceID, so it must be stable across runs.
type DVRSyncInstance struct {
	// Name is used only for log output.
	Name string
	// BaseURL is the full HTTP URL PlexTuner listens on
	// (e.g. http://plextuner-sports.plex.svc:5004).
	// Plex fetches /discover.json, /lineup.json, /guide.xml from here.
	BaseURL string
	// DeviceID is the stable unique identifier matching PLEX_TUNER_DEVICE_ID /
	// discover.json DeviceID. Used to correlate with Plex device rows.
	DeviceID string
	// FriendlyName is the label Plex shows in the DVR list
	// (matches PLEX_TUNER_FRIENDLY_NAME).
	FriendlyName string
}

// DVRSyncConfig holds Plex connection and reconcile behaviour parameters.
type DVRSyncConfig struct {
	// PlexHost is host:port without scheme (e.g. "plex.plex.svc:32400").
	PlexHost string
	// Token is the Plex auth token.
	Token string
	// Instances is the desired set of injected DVR tuners.
	Instances []DVRSyncInstance
	// DeleteUnknown removes DVR+device rows from Plex whose deviceId is not in
	// Instances. Only rows with make!="Silicondust" are considered so that real HDHR
	// wizard devices are never touched.
	DeleteUnknown bool
	// DryRun prints planned actions without making any API calls.
	DryRun bool
	// GuideWaitDuration is how long to wait after reloadGuide before fetching the
	// channel map. Defaults to 15s when zero.
	GuideWaitDuration time.Duration
	// HTTPClient is optional; a default 30 s client is used when nil.
	HTTPClient *http.Client
}

// DVRSyncResult summarises the outcome of one instance's reconcile.
type DVRSyncResult struct {
	Instance DVRSyncInstance
	// Action is one of: "created", "updated_uri", "refreshed", "skipped",
	// "deleted", "error".
	Action   string
	DVRKey   int
	Channels int
	Err      error
}

// devicesSnapshot groups the two API snapshots used throughout reconcile.
type devicesSnapshot struct {
	byID       map[string]deviceEntry // lower(DeviceID) → entry
	dvrByDevKy map[string]DVRInfo     // device key → DVR
	allDevices []deviceEntry
}

type deviceEntry struct {
	Device
	// Make is the hardware manufacturer field in Plex's device XML ("Silicondust"
	// for real HDHomeRun devices). PlexTuner injection rows have Make="" or "Unknown".
	Make string
}

// ReconcileDVRs brings Plex's injected DVR list in sync with cfg.Instances.
// It is safe to call repeatedly (idempotent for unchanged instances).
//
// For each desired instance:
//   - No device with matching DeviceID → register device + create DVR + activate channels
//   - Device exists, URI drifted → patch URI, reload guide, re-activate
//   - Device + DVR healthy → reload guide + re-activate (keeps channel map fresh)
//
// When DeleteUnknown is true, any injected DVR whose DeviceID is not in Instances
// and whose make is not "Silicondust" is deleted from Plex.
func ReconcileDVRs(ctx context.Context, cfg DVRSyncConfig) []DVRSyncResult {
	if cfg.GuideWaitDuration <= 0 {
		cfg.GuideWaitDuration = 15 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}

	snap, err := snapshotPlexState(cfg.PlexHost, cfg.Token)
	if err != nil {
		results := make([]DVRSyncResult, len(cfg.Instances))
		for i, inst := range cfg.Instances {
			results[i] = DVRSyncResult{Instance: inst, Action: "error",
				Err: fmt.Errorf("snapshot plex state: %w", err)}
		}
		return results
	}

	desiredIDs := make(map[string]bool, len(cfg.Instances))
	for _, inst := range cfg.Instances {
		desiredIDs[strings.ToLower(strings.TrimSpace(inst.DeviceID))] = true
	}

	var results []DVRSyncResult

	for _, inst := range cfg.Instances {
		if ctx.Err() != nil {
			results = append(results, DVRSyncResult{Instance: inst, Action: "error", Err: ctx.Err()})
			continue
		}
		res := reconcileOne(ctx, cfg, inst, snap)
		results = append(results, res)

		// After a creation, refresh the snapshot so subsequent iterations see new rows.
		if res.Action == "created" && !cfg.DryRun {
			if fresh, err := snapshotPlexState(cfg.PlexHost, cfg.Token); err == nil {
				snap = fresh
			}
		}
	}

	if cfg.DeleteUnknown {
		for _, dvr := range snap.dvrByDevKy {
			dev, ok := deviceForDVR(dvr, snap.allDevices)
			if !ok {
				continue
			}
			if desiredIDs[strings.ToLower(strings.TrimSpace(dev.DeviceID))] {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(dev.Make), "silicondust") {
				continue
			}
			label := dvr.LineupTitle
			if label == "" {
				label = fmt.Sprintf("dvr%d", dvr.Key)
			}
			if cfg.DryRun {
				log.Printf("[dvr-sync] DRY-RUN: would delete DVR %d (%s) device %s",
					dvr.Key, label, dev.Key)
			} else {
				log.Printf("[dvr-sync] deleting unknown injected DVR %d (%s) device %s",
					dvr.Key, label, dev.Key)
				if err := DeleteDVRAPI(cfg.PlexHost, cfg.Token, dvr.Key); err != nil {
					log.Printf("[dvr-sync] delete DVR %d: %v", dvr.Key, err)
				}
				if err := DeleteDeviceAPI(cfg.PlexHost, cfg.Token, dev.Key); err != nil {
					log.Printf("[dvr-sync] delete device %s: %v", dev.Key, err)
				}
			}
			results = append(results, DVRSyncResult{
				Instance: DVRSyncInstance{Name: label, DeviceID: dev.DeviceID},
				Action:   "deleted",
				DVRKey:   dvr.Key,
			})
		}
	}

	return results
}

func reconcileOne(
	ctx context.Context,
	cfg DVRSyncConfig,
	inst DVRSyncInstance,
	snap *devicesSnapshot,
) DVRSyncResult {
	devID := strings.ToLower(strings.TrimSpace(inst.DeviceID))
	existing, exists := snap.byID[devID]

	friendly := inst.FriendlyName
	if friendly == "" {
		friendly = inst.Name
	}
	apiCfg := PlexAPIConfig{
		BaseURL:      inst.BaseURL,
		PlexHost:     cfg.PlexHost,
		PlexToken:    cfg.Token,
		FriendlyName: friendly,
		DeviceID:     inst.DeviceID,
	}

	if !exists {
		return createFull(ctx, cfg, apiCfg, inst)
	}

	wantURI := strings.TrimRight(inst.BaseURL, "/")
	haveURI := strings.TrimRight(existing.URI, "/")
	uriChanged := haveURI != wantURI

	if uriChanged {
		if cfg.DryRun {
			log.Printf("[dvr-sync] DRY-RUN: %s device %s URI %q → %q",
				inst.Name, existing.Key, haveURI, wantURI)
		} else {
			log.Printf("[dvr-sync] %s updating device %s URI %q → %q",
				inst.Name, existing.Key, haveURI, wantURI)
			if err := patchDeviceURI(cfg.PlexHost, cfg.Token, existing.Key, wantURI, cfg.HTTPClient); err != nil {
				log.Printf("[dvr-sync] %s URI patch: %v (continuing)", inst.Name, err)
			}
		}
	}

	dvr, hasDVR := snap.dvrByDevKy[existing.Key]
	if !hasDVR {
		if cfg.DryRun {
			log.Printf("[dvr-sync] DRY-RUN: %s device exists (key=%s) but no DVR — would create",
				inst.Name, existing.Key)
			return DVRSyncResult{Instance: inst, Action: "created"}
		}
		log.Printf("[dvr-sync] %s device exists (key=%s) but no DVR — creating",
			inst.Name, existing.Key)
		devInfo := &DeviceInfo{Key: existing.Key, UUID: existing.UUID, URI: existing.URI}
		dvrKey, _, lineupIDs, err := CreateDVRViaAPI(apiCfg, devInfo)
		if err != nil {
			return DVRSyncResult{Instance: inst, Action: "error",
				Err: fmt.Errorf("create DVR: %w", err)}
		}
		ch, err := reloadAndActivate(ctx, cfg, apiCfg, devInfo, dvrKey, lineupIDs)
		return DVRSyncResult{Instance: inst, Action: "created", DVRKey: dvrKey, Channels: ch, Err: err}
	}

	action := "refreshed"
	if uriChanged {
		action = "updated_uri"
	}
	if cfg.DryRun {
		log.Printf("[dvr-sync] DRY-RUN: %s DVR %d — would reload guide + re-activate", inst.Name, dvr.Key)
		return DVRSyncResult{Instance: inst, Action: action, DVRKey: dvr.Key}
	}
	log.Printf("[dvr-sync] %s DVR %d — reloading guide + re-activating", inst.Name, dvr.Key)
	devInfo := &DeviceInfo{Key: existing.Key, UUID: existing.UUID, URI: existing.URI}
	lineupIDs := lineupIDsForDVR(cfg.PlexHost, cfg.Token, dvr.Key)
	ch, err := reloadAndActivate(ctx, cfg, apiCfg, devInfo, dvr.Key, lineupIDs)
	return DVRSyncResult{Instance: inst, Action: action, DVRKey: dvr.Key, Channels: ch, Err: err}
}

func createFull(
	ctx context.Context,
	cfg DVRSyncConfig,
	apiCfg PlexAPIConfig,
	inst DVRSyncInstance,
) DVRSyncResult {
	if cfg.DryRun {
		log.Printf("[dvr-sync] DRY-RUN: %s — would register device + create DVR at %s",
			inst.Name, inst.BaseURL)
		return DVRSyncResult{Instance: inst, Action: "created"}
	}
	log.Printf("[dvr-sync] %s — registering device at %s", inst.Name, inst.BaseURL)
	devInfo, err := RegisterTunerViaAPI(apiCfg)
	if err != nil {
		return DVRSyncResult{Instance: inst, Action: "error",
			Err: fmt.Errorf("register device: %w", err)}
	}
	log.Printf("[dvr-sync] %s — creating DVR (device key=%s)", inst.Name, devInfo.Key)
	dvrKey, _, lineupIDs, err := CreateDVRViaAPI(apiCfg, devInfo)
	if err != nil {
		return DVRSyncResult{Instance: inst, Action: "error",
			Err: fmt.Errorf("create DVR: %w", err)}
	}
	ch, err := reloadAndActivate(ctx, cfg, apiCfg, devInfo, dvrKey, lineupIDs)
	return DVRSyncResult{Instance: inst, Action: "created", DVRKey: dvrKey, Channels: ch, Err: err}
}

// reloadAndActivate triggers a guide reload, waits, fetches the channel map, and
// activates all channels. lineupIDs may be empty for pre-existing DVRs; in that case
// they are fetched from the DVR row itself.
func reloadAndActivate(
	ctx context.Context,
	cfg DVRSyncConfig,
	apiCfg PlexAPIConfig,
	devInfo *DeviceInfo,
	dvrKey int,
	lineupIDs []string,
) (int, error) {
	if err := ReloadGuideAPI(cfg.PlexHost, cfg.Token, dvrKey); err != nil {
		log.Printf("[dvr-sync] reload guide DVR %d: %v (continuing)", dvrKey, err)
	}

	if len(lineupIDs) == 0 {
		lineupIDs = lineupIDsForDVR(cfg.PlexHost, cfg.Token, dvrKey)
	}

	log.Printf("[dvr-sync] waiting %s for guide to populate (DVR %d)…",
		cfg.GuideWaitDuration, dvrKey)
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(cfg.GuideWaitDuration):
	}

	if len(lineupIDs) == 0 {
		return 0, fmt.Errorf("no lineup IDs for DVR %d after guide reload", dvrKey)
	}

	chMappings, err := GetChannelMap(cfg.PlexHost, cfg.Token, devInfo.UUID, lineupIDs)
	if err != nil {
		return 0, fmt.Errorf("get channel map DVR %d: %w", dvrKey, err)
	}
	if len(chMappings) == 0 {
		log.Printf("[dvr-sync] DVR %d: no channel mappings yet (guide may still be loading)", dvrKey)
		return 0, nil
	}

	n, err := ActivateChannelsAPI(apiCfg, devInfo.Key, chMappings)
	if err != nil {
		return n, fmt.Errorf("activate channels DVR %d: %w", dvrKey, err)
	}
	log.Printf("[dvr-sync] DVR %d: activated %d channels", dvrKey, n)
	return n, nil
}

// snapshotPlexState fetches devices and DVRs once and builds lookup maps.
func snapshotPlexState(plexHost, token string) (*devicesSnapshot, error) {
	devices, err := listDevicesWithMake(plexHost, token)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	dvrs, err := ListDVRsAPI(plexHost, token)
	if err != nil {
		return nil, fmt.Errorf("list dvrs: %w", err)
	}

	byID := make(map[string]deviceEntry, len(devices))
	for _, d := range devices {
		if id := strings.ToLower(strings.TrimSpace(d.DeviceID)); id != "" {
			byID[id] = d
		}
	}
	dvrByDevKy := make(map[string]DVRInfo, len(dvrs))
	for _, dvr := range dvrs {
		if dvr.DeviceKey != "" {
			dvrByDevKy[dvr.DeviceKey] = dvr
		}
	}
	return &devicesSnapshot{
		byID:       byID,
		dvrByDevKy: dvrByDevKy,
		allDevices: devices,
	}, nil
}

// deviceForDVR returns the deviceEntry whose key matches dvr.DeviceKey.
func deviceForDVR(dvr DVRInfo, all []deviceEntry) (deviceEntry, bool) {
	for _, d := range all {
		if strings.TrimSpace(d.Key) == strings.TrimSpace(dvr.DeviceKey) {
			return d, true
		}
	}
	return deviceEntry{}, false
}

// listDevicesWithMake wraps ListDevicesAPI but also captures the Make attribute
// which is not present on the base Device struct.
func listDevicesWithMake(plexHost, token string) ([]deviceEntry, error) {
	paths := []string{
		"/media/grabbers/devices",
		"/media/grabbers/tv.plex.grabbers.hdhomerun/devices",
	}
	client := &http.Client{Timeout: 30 * time.Second}

	type xmlDevice struct {
		Key      string `xml:"key,attr"`
		UUID     string `xml:"uuid,attr"`
		URI      string `xml:"uri,attr"`
		Name     string `xml:"name,attr"`
		DeviceID string `xml:"deviceId,attr"`
		Make     string `xml:"make,attr"`
	}
	type container struct {
		Devices []xmlDevice `xml:"Device"`
	}

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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("%s 404", p)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s returned %d", p, resp.StatusCode)
			continue
		}
		var mc container
		if err := xml.Unmarshal(body, &mc); err != nil {
			lastErr = err
			continue
		}
		out := make([]deviceEntry, len(mc.Devices))
		for i, d := range mc.Devices {
			out[i] = deviceEntry{
				Device: Device{
					Key:      d.Key,
					UUID:     d.UUID,
					URI:      d.URI,
					Name:     d.Name,
					DeviceID: d.DeviceID,
				},
				Make: d.Make,
			}
		}
		return out, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no device endpoint succeeded")
	}
	return nil, lastErr
}

// lineupIDsForDVR queries /livetv/dvrs/<key> and extracts the lineup attribute.
func lineupIDsForDVR(plexHost, token string, dvrKey int) []string {
	u := fmt.Sprintf("http://%s/livetv/dvrs/%d?X-Plex-Token=%s", plexHost, dvrKey, token)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(u)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	type dvrXML struct {
		Lineup string `xml:"lineup,attr"`
	}
	type container struct {
		Dvr []dvrXML `xml:"Dvr"`
	}
	var mc container
	if err := xml.Unmarshal(body, &mc); err != nil || len(mc.Dvr) == 0 {
		return nil
	}
	lineup := strings.TrimSpace(mc.Dvr[0].Lineup)
	if lineup == "" {
		return nil
	}
	return []string{lineup}
}

// patchDeviceURI attempts to update a device's URI via the Plex grabbers endpoint.
func patchDeviceURI(plexHost, token, deviceKey, newURI string, client *http.Client) error {
	paths := []string{
		fmt.Sprintf("/media/grabbers/devices/%s?uri=%s",
			url.PathEscape(deviceKey), url.QueryEscape(newURI)),
		fmt.Sprintf("/media/grabbers/tv.plex.grabbers.hdhomerun/devices/%s?uri=%s",
			url.PathEscape(deviceKey), url.QueryEscape(newURI)),
	}
	var lastErr error
	for _, p := range paths {
		u := fmt.Sprintf("http://%s%s", plexHost, p)
		req, err := http.NewRequest(http.MethodPut, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("X-Plex-Token", token)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
			return nil
		}
		lastErr = fmt.Errorf("%s returned %d", p, resp.StatusCode)
	}
	return lastErr
}

// InstancesFromSupervisorConfig parses a supervisor JSON config and returns the
// DVRSyncInstances for the injected (non-HDHR-wizard) children. This lets
// plex-dvr-sync drive itself from the same config the supervisor uses.
//
// Children with PLEX_TUNER_HDHR_NETWORK_MODE=true are skipped; those are wizard
// channels that Plex manages via its own device discovery.
func InstancesFromSupervisorConfig(raw []byte) ([]DVRSyncInstance, error) {
	type supInst struct {
		Name string            `json:"name"`
		Env  map[string]string `json:"env"`
	}
	type supCfg struct {
		Instances []supInst `json:"instances"`
	}
	var sc supCfg
	if err := json.Unmarshal(raw, &sc); err != nil {
		return nil, fmt.Errorf("parse supervisor config: %w", err)
	}
	var out []DVRSyncInstance
	for _, inst := range sc.Instances {
		if strings.EqualFold(inst.Env["PLEX_TUNER_HDHR_NETWORK_MODE"], "true") {
			continue
		}
		baseURL := strings.TrimSpace(inst.Env["PLEX_TUNER_BASE_URL"])
		deviceID := strings.TrimSpace(inst.Env["PLEX_TUNER_DEVICE_ID"])
		friendly := strings.TrimSpace(inst.Env["PLEX_TUNER_FRIENDLY_NAME"])
		if baseURL == "" || deviceID == "" {
			continue
		}
		out = append(out, DVRSyncInstance{
			Name:         inst.Name,
			BaseURL:      baseURL,
			DeviceID:     deviceID,
			FriendlyName: friendly,
		})
	}
	return out, nil
}

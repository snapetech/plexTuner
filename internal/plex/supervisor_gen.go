package plex

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SupervisorGenConfig holds inputs for supervisor config generation.
type SupervisorGenConfig struct {
	K3sPlexDir          string
	OutJSON             string
	OutYAML             string
	OutTSV              string
	Country             string
	PostalCode          string // used locally only, not logged
	Timezone            string // used locally only, not logged
	RegionProfile       string // "auto" or "na_en"
	HDHRm3uURL          string
	HDHRxmltv           string
	CatM3UURL           string
	CatXMLTVURL         string
	CategoryCounts      map[string]int // optional: confirmed linked counts for overflow sharding
	CategoryCap         int            // default 479
	HDHRLineupMax       int            // -1 = from preset
	HDHRLiveEPGOnly     *bool
	HDHREPGPrune        *bool
	HDHRStreamTranscode string // "", "on", "off", "auto", "auto_cached"
}

var defaultCategories = []string{
	"sports", "canada", "us", "canadamovies", "usmovies",
	"uk", "europe", "eusouth", "eueast", "latam", "intl",
}

var guideOffsetMap = map[string]int{
	"sports": 1000, "canada": 2000, "us": 3000, "canadamovies": 4000,
	"usmovies": 5000, "uk": 6000, "europe": 7000, "eusouth": 8000,
	"eueast": 9000, "latam": 10000, "intl": 11000,
}

type regionPreset struct {
	M3UURL          string
	XMLTVURL        string
	PreferLangs     string
	PreferLatin     bool
	TitleFallback   string
	LineupMax       int
	LiveEPGOnly     bool
	EPGPrune        bool
	StreamTranscode string
	LineupShape     string
	LineupRegion    string
}

var regionPresets = map[string]regionPreset{
	"na_en": {
		M3UURL:          "http://iptv-m3u-server.plex.svc/live.m3u",
		XMLTVURL:        "http://iptv-m3u-server.plex.svc/xmltv.xml",
		PreferLangs:     "en,eng",
		PreferLatin:     true,
		TitleFallback:   "channel",
		LineupMax:       479,
		LiveEPGOnly:     true,
		EPGPrune:        true,
		StreamTranscode: "on",
		LineupShape:     "na_en",
		LineupRegion:    "ca_west",
	},
}

type categoryShard struct {
	Base          string
	Name          string
	Skip          int
	Take          int
	ShardIndex    int
	ExpectedCount int
}

// GenerateSupervisorConfig builds supervisor JSON, k8s YAML, and cutover TSV.
func GenerateSupervisorConfig(cfg SupervisorGenConfig) error {
	if cfg.CategoryCap <= 0 {
		cfg.CategoryCap = 479
	}

	// Load HDHR deploy YAML to extract image + base URL.
	hdhrPath := cfg.K3sPlexDir + "/plextuner-hdhr-test-deployment.yaml"
	hdhrDeploy, err := loadFirstYAML(hdhrPath)
	if err != nil {
		return fmt.Errorf("load hdhr deploy: %w", err)
	}
	image := deepStr(hdhrDeploy, "spec", "template", "spec", "containers", 0, "image")

	// Choose preset.
	presetName, preset := choosePreset(cfg)

	hdhrM3U := cfg.HDHRm3uURL
	if hdhrM3U == "" {
		hdhrM3U = preset.M3UURL
	}
	hdhrXMLTV := cfg.HDHRxmltv
	if hdhrXMLTV == "" {
		hdhrXMLTV = preset.XMLTVURL
	}
	hdhrMax := cfg.HDHRLineupMax
	if hdhrMax < 0 {
		hdhrMax = preset.LineupMax
	}
	hdhrLiveEPG := preset.LiveEPGOnly
	if cfg.HDHRLiveEPGOnly != nil {
		hdhrLiveEPG = *cfg.HDHRLiveEPGOnly
	}
	hdhrEPGPrune := preset.EPGPrune
	if cfg.HDHREPGPrune != nil {
		hdhrEPGPrune = *cfg.HDHREPGPrune
	}
	hdhrTranscode := cfg.HDHRStreamTranscode
	if hdhrTranscode == "" {
		hdhrTranscode = preset.StreamTranscode
	}

	shards := expandShards(defaultCategories, cfg.CategoryCounts, cfg.CategoryCap)

	supJSON := buildSupervisorJSON(hdhrDeploy, shards, supervisorBuildParams{
		CatM3U:          cfg.CatM3UURL,
		CatXMLTV:        cfg.CatXMLTVURL,
		HDHM3U:          hdhrM3U,
		HDHRxmlTV:       hdhrXMLTV,
		HDHRMax:         hdhrMax,
		HDHRLiveEPG:     hdhrLiveEPG,
		HDHREPGPrune:    hdhrEPGPrune,
		HDHRTranscode:   hdhrTranscode,
		HDHRPreferLangs: preset.PreferLangs,
		HDHRPreferLatin: preset.PreferLatin,
		HDHRTitleFB:     preset.TitleFallback,
		HDHRShape:       preset.LineupShape,
		HDHRRegion:      preset.LineupRegion,
	})

	manifests := buildSinglepodManifest(supJSON, hdhrDeploy, image)
	tsv := buildCutoverTSV(supJSON)

	// Write outputs.
	supData, _ := json.MarshalIndent(supJSON, "", "  ")
	if err := os.WriteFile(cfg.OutJSON, append(supData, '\n'), 0644); err != nil {
		return fmt.Errorf("write supervisor JSON: %w", err)
	}

	var yamlParts []string
	for _, m := range manifests {
		b, _ := yaml.Marshal(m)
		yamlParts = append(yamlParts, string(b))
	}
	yamlOut := strings.Join(yamlParts, "---\n")
	if err := os.WriteFile(cfg.OutYAML, []byte(yamlOut), 0644); err != nil {
		return fmt.Errorf("write manifest YAML: %w", err)
	}

	if err := os.WriteFile(cfg.OutTSV, []byte(tsv), 0644); err != nil {
		return fmt.Errorf("write cutover TSV: %w", err)
	}

	fmt.Printf("HDHR preset: %s (timezone/country/postal used locally; not echoed)\n", presetName)
	overflow := 0
	for _, s := range shards {
		if s.Name != s.Base {
			overflow++
		}
	}
	fmt.Printf("Category shards: %d instances from %d bases (overflow shards=%d)\n",
		len(shards), len(defaultCategories), overflow)
	fmt.Printf("Wrote %s\n", cfg.OutJSON)
	fmt.Printf("Wrote %s\n", cfg.OutYAML)
	fmt.Printf("Wrote %s\n", cfg.OutTSV)
	return nil
}

func choosePreset(cfg SupervisorGenConfig) (string, regionPreset) {
	if cfg.RegionProfile != "auto" && cfg.RegionProfile != "" {
		if p, ok := regionPresets[cfg.RegionProfile]; ok {
			return cfg.RegionProfile, p
		}
	}
	tz := strings.ToLower(cfg.Timezone)
	c := strings.ToUpper(cfg.Country)
	pc := regexp.MustCompile(`\s+`).ReplaceAllString(strings.ToUpper(cfg.PostalCode), "")
	if strings.HasPrefix(tz, "america/") {
		return "na_en", regionPresets["na_en"]
	}
	if c == "CA" || c == "CAN" || c == "US" || c == "USA" {
		return "na_en", regionPresets["na_en"]
	}
	if regexp.MustCompile(`^[A-Z]\d[A-Z]\d[A-Z]\d$`).MatchString(pc) {
		return "na_en", regionPresets["na_en"]
	}
	return "na_en", regionPresets["na_en"]
}

func expandShards(bases []string, counts map[string]int, cap int) []categoryShard {
	var out []categoryShard
	for _, base := range bases {
		total := counts[strings.ToLower(base)]
		if cap <= 0 || total <= 0 || total <= cap {
			out = append(out, categoryShard{Base: base, Name: base, ExpectedCount: total})
			continue
		}
		num := (total + cap - 1) / cap
		for i := 0; i < num; i++ {
			suffix := ""
			if i > 0 {
				suffix = strconv.Itoa(i + 1)
			}
			expected := cap
			if rem := total - i*cap; rem < cap {
				expected = rem
			}
			out = append(out, categoryShard{
				Base: base, Name: base + suffix,
				Skip: i * cap, Take: cap, ShardIndex: i, ExpectedCount: expected,
			})
		}
	}
	return out
}

type supervisorBuildParams struct {
	CatM3U, CatXMLTV, HDHM3U, HDHRxmlTV string
	HDHRMax                             int
	HDHRLiveEPG, HDHREPGPrune           bool
	HDHRTranscode                       string
	HDHRPreferLangs                     string
	HDHRPreferLatin                     bool
	HDHRTitleFB                         string
	HDHRShape, HDHRRegion               string
}

func buildSupervisorJSON(hdhrDeploy map[string]any, shards []categoryShard, p supervisorBuildParams) map[string]any {
	hdhrBase := "http://plextuner-hdhr.plex.home"
	for _, a := range deepStrings(hdhrDeploy, "spec", "template", "spec", "containers", 0, "args") {
		if strings.HasPrefix(a, "-base-url=") {
			hdhrBase = strings.SplitN(a, "=", 2)[1]
		}
	}

	boolStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}

	instances := []any{
		map[string]any{
			"name": "hdhr-main",
			"args": []string{"run", "-mode=easy", "-addr=:5004", "-catalog=/data/hdhr-main/catalog.json", "-base-url=" + hdhrBase},
			"env": map[string]any{
				"PLEX_TUNER_HDHR_NETWORK_MODE":              "true",
				"PLEX_TUNER_SSDP_DISABLED":                  "false",
				"PLEX_TUNER_HDHR_SCAN_POSSIBLE":             "true",
				"PLEX_TUNER_FRIENDLY_NAME":                  "hdhr",
				"PLEX_TUNER_HDHR_FRIENDLY_NAME":             "hdhr",
				"PLEX_TUNER_HDHR_MANUFACTURER":              "Silicondust",
				"PLEX_TUNER_HDHR_MODEL_NUMBER":              "HDHR5-2US",
				"PLEX_TUNER_HDHR_FIRMWARE_NAME":             "hdhomerun5_atsc",
				"PLEX_TUNER_HDHR_FIRMWARE_VERSION":          "20240101",
				"PLEX_TUNER_HDHR_DEVICE_AUTH":               "plextuner",
				"PLEX_TUNER_M3U_URL":                        p.HDHM3U,
				"PLEX_TUNER_XMLTV_URL":                      p.HDHRxmlTV,
				"PLEX_TUNER_LIVE_EPG_ONLY":                  boolStr(p.HDHRLiveEPG),
				"PLEX_TUNER_EPG_PRUNE_UNLINKED":             boolStr(p.HDHREPGPrune),
				"PLEX_TUNER_LINEUP_MAX_CHANNELS":            strconv.Itoa(p.HDHRMax),
				"PLEX_TUNER_LINEUP_DROP_MUSIC":              "true",
				"PLEX_TUNER_LINEUP_SHAPE":                   p.HDHRShape,
				"PLEX_TUNER_LINEUP_REGION_PROFILE":          p.HDHRRegion,
				"PLEX_TUNER_STREAM_TRANSCODE":               p.HDHRTranscode,
				"PLEX_TUNER_STREAM_BUFFER_BYTES":            "-1",
				"PLEX_TUNER_XMLTV_PREFER_LANGS":             p.HDHRPreferLangs,
				"PLEX_TUNER_XMLTV_PREFER_LATIN":             boolStr(p.HDHRPreferLatin),
				"PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK": p.HDHRTitleFB,
				"PLEX_TUNER_GUIDE_NUMBER_OFFSET":            "0",
			},
		},
	}

	basePort := 5101
	for idx, shard := range shards {
		cat := shard.Name
		base := shard.Base
		port := strconv.Itoa(basePort + idx)
		baseOff := guideOffsetMap[base]
		if baseOff == 0 {
			baseOff = (11 + idx) * 1000
		}
		guideOff := baseOff + shard.ShardIndex*100000

		env := map[string]any{
			"PLEX_TUNER_M3U_URL":                        p.CatM3U,
			"PLEX_TUNER_XMLTV_URL":                      p.CatXMLTV,
			"PLEX_TUNER_LINEUP_CATEGORY":                cat,
			"PLEX_TUNER_LIVE_EPG_ONLY":                  "true",
			"PLEX_TUNER_EPG_PRUNE_UNLINKED":             "true",
			"PLEX_TUNER_STREAM_TRANSCODE":               "off",
			"PLEX_TUNER_STREAM_BUFFER_BYTES":            "-1",
			"PLEX_TUNER_LINEUP_MAX_CHANNELS":            "479",
			"TZ":                                        "America/Chicago",
			"PLEX_TUNER_DEVICE_ID":                      cat,
			"PLEX_TUNER_FRIENDLY_NAME":                  cat,
			"PLEX_TUNER_BASE_URL":                       "http://plextuner-" + cat + ".plex.svc:5004",
			"PLEX_TUNER_SSDP_DISABLED":                  "true",
			"PLEX_TUNER_HDHR_SCAN_POSSIBLE":             "false",
			"PLEX_TUNER_XMLTV_PREFER_LANGS":             "en,eng",
			"PLEX_TUNER_XMLTV_PREFER_LATIN":             "true",
			"PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK": "channel",
			"PLEX_TUNER_GUIDE_NUMBER_OFFSET":            strconv.Itoa(guideOff),
		}
		if shard.Skip > 0 {
			env["PLEX_TUNER_LINEUP_SKIP"] = strconv.Itoa(shard.Skip)
		}
		if shard.Take > 0 {
			env["PLEX_TUNER_LINEUP_TAKE"] = strconv.Itoa(shard.Take)
		}
		if shard.ShardIndex > 0 {
			env["PLEX_TUNER_LINEUP_CATEGORY"] = base
		}

		instances = append(instances, map[string]any{
			"name": cat,
			"args": []string{"run", "-mode=easy", "-addr=:" + port, "-catalog=/data/" + cat + "/catalog.json"},
			"env":  env,
		})
	}

	return map[string]any{
		"restart":      true,
		"restartDelay": "2s",
		"failFast":     false,
		"instances":    instances,
	}
}

func buildSinglepodManifest(supJSON map[string]any, hdhrDeploy map[string]any, image string) []map[string]any {
	hdhrTmpl := deepMap(hdhrDeploy, "spec", "template", "spec")
	hdhrContainer := deepMapArr(hdhrTmpl, "containers", 0)

	configmap := map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "plextuner-supervisor-config", "namespace": "plex"},
		"data":       map[string]any{"supervisor.json": mustJSON(supJSON)},
	}

	ports := []map[string]any{
		{"name": "hdhr-http", "containerPort": 5004, "protocol": "TCP"},
	}
	for _, instAny := range toSlice(supJSON["instances"]) {
		inst, _ := instAny.(map[string]any)
		if inst == nil || inst["name"] == "hdhr-main" {
			continue
		}
		args := toStringSlice(toSlice(inst["args"]))
		port := parseAddrPort(args)
		ports = append(ports, map[string]any{"name": "p" + strconv.Itoa(port), "containerPort": port, "protocol": "TCP"})
	}
	ports = append(ports,
		map[string]any{"name": "hdhr-disc", "containerPort": 65001, "protocol": "UDP"},
		map[string]any{"name": "hdhr-ctrl", "containerPort": 65001, "protocol": "TCP"},
	)

	dep := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "plextuner-supervisor", "namespace": "plex", "labels": map[string]any{"app": "plextuner-supervisor"}},
		"spec": map[string]any{
			"replicas": 1,
			"strategy": map[string]any{"type": "Recreate"},
			"selector": map[string]any{"matchLabels": map[string]any{"app": "plextuner-supervisor"}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{"app": "plextuner-supervisor"}},
				"spec": map[string]any{
					"nodeSelector": deepMap(hdhrTmpl, "nodeSelector"),
					"hostNetwork":  true,
					"dnsPolicy":    "ClusterFirstWithHostNet",
					"dnsConfig":    deepMap(hdhrTmpl, "dnsConfig"),
					"containers": []map[string]any{{
						"name":            "plextuner",
						"image":           image,
						"imagePullPolicy": stringVal(hdhrContainer, "imagePullPolicy"),
						"args":            []string{"supervise", "-config", "/config/supervisor.json"},
						"envFrom":         hdhrContainer["envFrom"],
						"env": []map[string]any{
							{"name": "PLEX_TUNER_PMS_TOKEN", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "plex-token", "key": "token"}}},
							{"name": "PLEX_TUNER_PMS_URL", "value": "http://plex.plex.svc:32400"},
							{"name": "PLEX_TUNER_PLEX_SESSION_REAPER", "value": "true"},
							{"name": "PLEX_TUNER_PLEX_SESSION_REAPER_POLL_S", "value": "2"},
							{"name": "PLEX_TUNER_PLEX_SESSION_REAPER_IDLE_S", "value": "15"},
							{"name": "PLEX_TUNER_PLEX_SESSION_REAPER_RENEW_LEASE_S", "value": "20"},
							{"name": "PLEX_TUNER_PLEX_SESSION_REAPER_HARD_LEASE_S", "value": "1800"},
							{"name": "PLEX_TUNER_PLEX_SESSION_REAPER_SSE", "value": "true"},
						},
						"ports": ports,
						"volumeMounts": []map[string]any{
							{"name": "supervisor-config", "mountPath": "/config"},
							{"name": "data", "mountPath": "/data"},
						},
						"readinessProbe": map[string]any{
							"httpGet":             map[string]any{"path": "/discover.json", "port": 5004},
							"initialDelaySeconds": 30, "periodSeconds": 10, "failureThreshold": 12,
						},
						"livenessProbe": map[string]any{
							"httpGet":             map[string]any{"path": "/discover.json", "port": 5004},
							"initialDelaySeconds": 60, "periodSeconds": 30, "failureThreshold": 5,
						},
						"resources": hdhrContainer["resources"],
					}},
					"volumes": []map[string]any{
						{"name": "supervisor-config", "configMap": map[string]any{"name": "plextuner-supervisor-config"}},
						{"name": "data", "emptyDir": map[string]any{}},
					},
				},
			},
		},
	}

	services := []map[string]any{
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": "plextuner-hdhr-test", "namespace": "plex"},
			"spec": map[string]any{
				"selector": map[string]any{"app": "plextuner-supervisor"},
				"ports":    []map[string]any{{"name": "http", "port": 5004, "targetPort": 5004, "protocol": "TCP"}},
			},
		},
	}
	for _, instAny := range toSlice(supJSON["instances"]) {
		inst, _ := instAny.(map[string]any)
		if inst == nil || inst["name"] == "hdhr-main" {
			continue
		}
		cat := fmt.Sprintf("%v", inst["name"])
		args := toStringSlice(toSlice(inst["args"]))
		target := parseAddrPort(args)
		services = append(services, map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": "plextuner-" + cat, "namespace": "plex"},
			"spec": map[string]any{
				"selector": map[string]any{"app": "plextuner-supervisor"},
				"ports":    []map[string]any{{"name": "http", "port": 5004, "targetPort": target, "protocol": "TCP"}},
			},
		})
	}

	var out []map[string]any
	out = append(out, configmap, dep)
	out = append(out, services...)
	return out
}

func buildCutoverTSV(supJSON map[string]any) string {
	var lines []string
	lines = append(lines, "# category\told_uri\tnew_uri\turi_changed\tdevice_id\tfriendly_name")
	for _, instAny := range toSlice(supJSON["instances"]) {
		inst, _ := instAny.(map[string]any)
		if inst == nil || inst["name"] == "hdhr-main" {
			continue
		}
		cat := fmt.Sprintf("%v", inst["name"])
		env, _ := inst["env"].(map[string]any)
		oldURI := "http://plextuner-" + cat + ".plex.svc:5004"
		newURI := fmt.Sprintf("%v", env["PLEX_TUNER_BASE_URL"])
		changed := "no"
		if oldURI != newURI {
			changed = "yes"
		}
		lines = append(lines, strings.Join([]string{
			cat, oldURI, newURI, changed,
			fmt.Sprintf("%v", env["PLEX_TUNER_DEVICE_ID"]),
			fmt.Sprintf("%v", env["PLEX_TUNER_FRIENDLY_NAME"]),
		}, "\t"))
	}
	return strings.Join(lines, "\n") + "\n"
}

// --- YAML / map helpers ---

func loadFirstYAML(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func deepStr(m any, keys ...any) string {
	cur := m
	for _, k := range keys {
		if cur == nil {
			return ""
		}
		switch kv := k.(type) {
		case string:
			mm, _ := cur.(map[string]any)
			cur = mm[kv]
		case int:
			sl, _ := cur.([]any)
			if kv >= len(sl) {
				return ""
			}
			cur = sl[kv]
		}
	}
	s, _ := cur.(string)
	return s
}

func deepStrings(m any, keys ...any) []string {
	cur := m
	for _, k := range keys {
		if cur == nil {
			return nil
		}
		switch kv := k.(type) {
		case string:
			mm, _ := cur.(map[string]any)
			cur = mm[kv]
		case int:
			sl, _ := cur.([]any)
			if kv >= len(sl) {
				return nil
			}
			cur = sl[kv]
		}
	}
	sl, _ := cur.([]any)
	var out []string
	for _, v := range sl {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func deepMap(m any, keys ...any) map[string]any {
	cur := m
	for _, k := range keys {
		if cur == nil {
			return nil
		}
		switch kv := k.(type) {
		case string:
			mm, _ := cur.(map[string]any)
			cur = mm[kv]
		}
	}
	out, _ := cur.(map[string]any)
	return out
}

func deepMapArr(m map[string]any, key string, idx int) map[string]any {
	sl, _ := m[key].([]any)
	if idx >= len(sl) {
		return map[string]any{}
	}
	out, _ := sl[idx].(map[string]any)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func toStringSlice(sl []any) []string {
	out := make([]string, 0, len(sl))
	for _, v := range sl {
		out = append(out, fmt.Sprintf("%v", v))
	}
	return out
}

func parseAddrPort(args []string) int {
	for _, a := range args {
		if strings.HasPrefix(a, "-addr=:") {
			n, err := strconv.Atoi(a[7:])
			if err == nil {
				return n
			}
		}
	}
	return 5004
}

func mustJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

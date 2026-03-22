package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/plex"
)

func plexOpsCommands() []commandSpec {
	dbInspectCmd := flag.NewFlagSet("plex-db-inspect", flag.ExitOnError)
	dbInspectDir := dbInspectCmd.String("plex-data-dir", "", "Plex data dir (contains Plug-in Support/Databases)")
	dbInspectOut := dbInspectCmd.String("out", "", "Optional JSON output path")

	apiInspectCmd := flag.NewFlagSet("plex-api-inspect", flag.ExitOnError)
	apiInspectURL := apiInspectCmd.String("plex-url", "", "Plex base URL")
	apiInspectToken := apiInspectCmd.String("token", "", "Plex token")
	apiInspectTuner := apiInspectCmd.String("tuner-base-url", "", "Optional tuner base URL for discover/lineup/guide probes")
	apiInspectProbes := apiInspectCmd.Bool("include-probes", true, "Probe known Live TV endpoints in addition to snapshotting device/DVR state")
	apiInspectOut := apiInspectCmd.String("out", "", "Optional JSON output path")

	deviceAuditCmd := flag.NewFlagSet("plex-device-audit", flag.ExitOnError)
	deviceAuditURL := deviceAuditCmd.String("plex-url", "", "Plex base URL")
	deviceAuditToken := deviceAuditCmd.String("token", "", "Plex token")
	deviceAuditOut := deviceAuditCmd.String("out", "", "Optional JSON output path")

	dvrRepairCmd := flag.NewFlagSet("plex-dvr-repair", flag.ExitOnError)
	dvrRepairURL := dvrRepairCmd.String("plex-url", "", "Plex base URL")
	dvrRepairToken := dvrRepairCmd.String("token", "", "Plex token")
	dvrRepairKey := dvrRepairCmd.Int("dvr-key", 0, "Plex DVR key to repair")
	dvrRepairReload := dvrRepairCmd.Bool("reload-guide", true, "Call reloadGuide before rebuilding channel activation")
	dvrRepairOut := dvrRepairCmd.String("out", "", "Optional JSON output path")

	dvrCutoverCmd := flag.NewFlagSet("plex-dvr-cutover", flag.ExitOnError)
	dvrCutoverURL := dvrCutoverCmd.String("plex-url", "", "Plex base URL")
	dvrCutoverToken := dvrCutoverCmd.String("token", "", "Plex token")
	dvrCutoverMap := dvrCutoverCmd.String("map", "", "TSV cutover map (category, old_uri, new_uri, uri_changed, device_id, friendly_name)")
	dvrCutoverReload := dvrCutoverCmd.Bool("reload-guide", true, "Call reloadGuide after recreating each DVR")
	dvrCutoverActivate := dvrCutoverCmd.Bool("activate", false, "Activate channel mappings after recreating each DVR")
	dvrCutoverDo := dvrCutoverCmd.Bool("do", false, "Actually delete/recreate matching DVR/device rows; default is dry-run")
	dvrCutoverOut := dvrCutoverCmd.String("out", "", "Optional JSON output path")

	apiRequestCmd := flag.NewFlagSet("plex-api-request", flag.ExitOnError)
	apiRequestBase := apiRequestCmd.String("base-url", "", "Target base URL")
	apiRequestToken := apiRequestCmd.String("token", "", "Optional token; added as X-Plex-Token query param")
	apiRequestMethod := apiRequestCmd.String("method", "GET", "HTTP method")
	apiRequestPath := apiRequestCmd.String("path", "", "Request path")
	apiRequestQuery := apiRequestCmd.String("query", "", "Optional query string without leading ?")
	apiRequestHeaders := apiRequestCmd.String("headers", "", "Optional comma-separated header pairs: Key: Value,Key2: Value2")
	apiRequestBody := apiRequestCmd.String("body", "", "Optional request body")
	apiRequestOut := apiRequestCmd.String("out", "", "Optional JSON output path")

	logInspectCmd := flag.NewFlagSet("plex-log-inspect", flag.ExitOnError)
	logInspectDir := logInspectCmd.String("plex-data-dir", "", "Plex data dir (contains Logs/)")
	logInspectOut := logInspectCmd.String("out", "", "Optional JSON output path")

	shareForceCmd := flag.NewFlagSet("plex-share-force-test", flag.ExitOnError)
	shareForceToken := shareForceCmd.String("token", "", "Plex owner token")
	shareForceMachineID := shareForceCmd.String("machine-id", "", "Processed machine identifier")
	shareForcePlexURL := shareForceCmd.String("plex-url", "", "Optional Plex base URL used to derive the machine identifier")
	shareForceClientID := shareForceCmd.String("client-id", "iptvtunerr-plex-share-force-test", "Client identifier for plex.tv share creation")
	shareForceUserID := shareForceCmd.Int("user-id", 0, "Target invited/shared user id")
	shareForceLibraryIDs := shareForceCmd.String("library-ids", "", "Comma-separated plex.tv section ids to restore during recreate")
	shareForceRequested := shareForceCmd.Int("requested-allow-tuners", 1, "Requested allowTuners value to send during recreate")
	shareForceAllowSync := shareForceCmd.Int("allow-sync", 1, "Requested allowSync value to send during recreate")
	shareForceDo := shareForceCmd.Bool("do", false, "Actually delete and recreate the share row; default is inspect-only")
	shareForceOut := shareForceCmd.String("out", "", "Optional JSON output path")

	return []commandSpec{
		{Name: "plex-db-inspect", Section: "Lab/ops", Summary: "Inspect Plex SQLite Live TV/provider state from disk", FlagSet: dbInspectCmd, Run: func(_ *config.Config, args []string) {
			_ = dbInspectCmd.Parse(args)
			handlePlexDBInspect(*dbInspectDir, *dbInspectOut)
		}},
		{Name: "plex-api-inspect", Section: "Lab/ops", Summary: "Snapshot Plex Live TV API/device/provider state", FlagSet: apiInspectCmd, Run: func(_ *config.Config, args []string) {
			_ = apiInspectCmd.Parse(args)
			handlePlexAPIInspect(*apiInspectURL, *apiInspectToken, *apiInspectTuner, *apiInspectProbes, *apiInspectOut)
		}},
		{Name: "plex-device-audit", Section: "Lab/ops", Summary: "Resolve and probe each registered Plex Live TV device URI", FlagSet: deviceAuditCmd, Run: func(_ *config.Config, args []string) {
			_ = deviceAuditCmd.Parse(args)
			handlePlexDeviceAudit(*deviceAuditURL, *deviceAuditToken, *deviceAuditOut)
		}},
		{Name: "plex-dvr-repair", Section: "Lab/ops", Summary: "Rebuild one DVR's enabled channel set from Plex's current channelmap", FlagSet: dvrRepairCmd, Run: func(_ *config.Config, args []string) {
			_ = dvrRepairCmd.Parse(args)
			handlePlexDVRRepair(*dvrRepairURL, *dvrRepairToken, *dvrRepairKey, *dvrRepairReload, *dvrRepairOut)
		}},
		{Name: "plex-dvr-cutover", Section: "Lab/ops", Summary: "Delete/recreate stale DVR/device rows from a URI cutover map", FlagSet: dvrCutoverCmd, Run: func(_ *config.Config, args []string) {
			_ = dvrCutoverCmd.Parse(args)
			handlePlexDVRCutover(*dvrCutoverURL, *dvrCutoverToken, *dvrCutoverMap, *dvrCutoverReload, *dvrCutoverActivate, *dvrCutoverDo, *dvrCutoverOut)
		}},
		{Name: "plex-api-request", Section: "Lab/ops", Summary: "Replay an arbitrary Plex/Plex.tv HTTP request", FlagSet: apiRequestCmd, Run: func(_ *config.Config, args []string) {
			_ = apiRequestCmd.Parse(args)
			handlePlexAPIRequest(*apiRequestBase, *apiRequestToken, *apiRequestMethod, *apiRequestPath, *apiRequestQuery, *apiRequestHeaders, *apiRequestBody, *apiRequestOut)
		}},
		{Name: "plex-log-inspect", Section: "Lab/ops", Summary: "Mine Plex Media Server logs for Live TV and grabber endpoints", FlagSet: logInspectCmd, Run: func(_ *config.Config, args []string) {
			_ = logInspectCmd.Parse(args)
			handlePlexLogInspect(*logInspectDir, *logInspectOut)
		}},
		{Name: "plex-share-force-test", Section: "Lab/ops", Summary: "Delete/recreate a Plex share row and report the observed allowTuners clamp", FlagSet: shareForceCmd, Run: func(_ *config.Config, args []string) {
			_ = shareForceCmd.Parse(args)
			handlePlexShareForceTest(*shareForceToken, *shareForceMachineID, *shareForcePlexURL, *shareForceClientID, *shareForceUserID, *shareForceLibraryIDs, *shareForceRequested, *shareForceAllowSync, *shareForceDo, *shareForceOut)
		}},
	}
}

func handlePlexDBInspect(plexDataDir, outPath string) {
	report, err := plex.InspectPlexDB(strings.TrimSpace(plexDataDir))
	if err != nil {
		log.Printf("Plex DB inspect failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(report, outPath)
}

func handlePlexAPIInspect(plexURL, plexToken, tunerBaseURL string, includeProbes bool, outPath string) {
	baseURL, token := resolvePlexAccess(plexURL, plexToken)
	if baseURL == "" || token == "" {
		log.Print("Need Plex API access: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
		os.Exit(1)
	}
	report, err := plex.SnapshotPlexAPI(baseURL, token, tunerBaseURL, includeProbes)
	if err != nil {
		log.Printf("Plex API inspect failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(report, outPath)
}

func handlePlexDeviceAudit(plexURL, plexToken, outPath string) {
	baseURL, token := resolvePlexAccess(plexURL, plexToken)
	if baseURL == "" || token == "" {
		log.Print("Need Plex API access: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
		os.Exit(1)
	}
	report, err := plex.AuditPlexDevices(baseURL, token)
	if err != nil {
		log.Printf("Plex device audit failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(report, outPath)
}

func handlePlexDVRRepair(plexURL, plexToken string, dvrKey int, reloadGuide bool, outPath string) {
	baseURL, token := resolvePlexAccess(plexURL, plexToken)
	if baseURL == "" {
		log.Print("Need Plex API access: set -plex-url or IPTV_TUNERR_PMS_URL (or PLEX_HOST)")
		os.Exit(1)
	}
	if dvrKey <= 0 {
		log.Print("Set -dvr-key")
		os.Exit(1)
	}
	host, err := hostPortFromBaseURL(baseURL)
	if err != nil {
		log.Printf("Bad -plex-url: %v", err)
		os.Exit(1)
	}
	if token == "" && host != "127.0.0.1:32400" && host != "localhost:32400" && host != "[::1]:32400" {
		log.Print("Need Plex token for non-localhost access: set -token or IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN")
		os.Exit(1)
	}
	report, err := plex.RepairDVRChannelActivation(baseURL, token, dvrKey, reloadGuide)
	if err != nil {
		log.Printf("Plex DVR repair failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(report, outPath)
}

func handlePlexDVRCutover(plexURL, plexToken, mapPath string, reloadGuide, activate, doApply bool, outPath string) {
	baseURL, token := resolvePlexAccess(plexURL, plexToken)
	if baseURL == "" || token == "" {
		log.Print("Need Plex API access: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
		os.Exit(1)
	}
	mapPath = strings.TrimSpace(mapPath)
	if mapPath == "" {
		log.Print("Set -map")
		os.Exit(1)
	}
	rows, err := plex.LoadCutoverMap(mapPath)
	if err != nil {
		log.Printf("Load cutover map failed: %v", err)
		os.Exit(1)
	}
	report, err := plex.CutoverDVRs(baseURL, token, rows, reloadGuide, activate, doApply)
	if err != nil {
		log.Printf("Plex DVR cutover failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(report, outPath)
}

func handlePlexAPIRequest(baseURL, token, method, path, queryRaw, headersRaw, body, outPath string) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		log.Print("Set -base-url")
		os.Exit(1)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		log.Print("Set -path")
		os.Exit(1)
	}
	queryVals := url.Values{}
	if q := strings.TrimSpace(queryRaw); q != "" {
		parsed, err := url.ParseQuery(q)
		if err != nil {
			log.Printf("Bad -query: %v", err)
			os.Exit(1)
		}
		queryVals = parsed
	}
	headerMap := map[string]string{}
	for _, part := range parseCSV(headersRaw) {
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			log.Printf("Bad header %q (want Key: Value)", part)
			os.Exit(1)
		}
		headerMap[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	rec, err := plex.DoHTTPRequest(plex.HTTPRequestSpec{
		BaseURL: baseURL,
		Method:  method,
		Path:    path,
		Query:   queryVals,
		Headers: headerMap,
		Body:    []byte(body),
		Token:   strings.TrimSpace(token),
	})
	if err != nil {
		log.Printf("Plex API request failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(rec, outPath)
}

func handlePlexLogInspect(plexDataDir, outPath string) {
	report, err := plex.InspectPlexLogs(strings.TrimSpace(plexDataDir))
	if err != nil {
		log.Printf("Plex log inspect failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(report, outPath)
}

func handlePlexShareForceTest(token, machineID, plexURL, clientID string, userID int, libraryIDsCSV string, requestedAllowTuners, allowSync int, doApply bool, outPath string) {
	token = strings.TrimSpace(firstNonEmpty(token, os.Getenv("IPTV_TUNERR_PMS_TOKEN"), os.Getenv("PLEX_TOKEN")))
	if token == "" {
		log.Print("Need owner Plex token: set -token or IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN")
		os.Exit(1)
	}
	if strings.TrimSpace(machineID) == "" {
		baseURL := strings.TrimSpace(plexURL)
		if baseURL == "" {
			baseURL = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_URL"))
		}
		if baseURL == "" {
			if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
				baseURL = "http://" + host + ":32400"
			}
		}
		if baseURL == "" {
			log.Print("Need -machine-id or a reachable -plex-url to derive it")
			os.Exit(1)
		}
		identity, err := plex.GetServerIdentity(baseURL, token)
		if err != nil {
			log.Printf("Derive machine id failed: %v", err)
			os.Exit(1)
		}
		machineID = identity["machine_identifier"]
	}
	if strings.TrimSpace(machineID) == "" {
		log.Print("Could not derive machine identifier; set -machine-id explicitly")
		os.Exit(1)
	}
	if userID == 0 {
		log.Print("Set -user-id")
		os.Exit(1)
	}
	libraryIDs := parseIntCSV(libraryIDsCSV)
	shared, err := plex.ListSharedServers(token, machineID)
	if err != nil {
		log.Printf("List shared servers failed: %v", err)
		os.Exit(1)
	}
	report := map[string]any{
		"machine_id":             machineID,
		"user_id":                userID,
		"requested_allow_tuners": requestedAllowTuners,
		"requested_allow_sync":   allowSync,
		"shared_server_before":   nil,
		"shared_server_after":    nil,
		"applied":                doApply,
		"note":                   "Plex currently clamps non-Home users back to allowTuners=0 during share creation.",
	}
	var current *plex.SharedServer
	for _, item := range shared {
		if item.UserID == userID {
			copied := item
			current = &copied
			break
		}
	}
	report["shared_server_before"] = current
	if !doApply {
		writeJSONOrStdout(report, outPath)
		return
	}
	if current == nil {
		log.Print("No existing shared-server row found for that user id")
		os.Exit(1)
	}
	if len(libraryIDs) == 0 {
		log.Print("Set -library-ids to the plex.tv section ids you want to restore during recreate")
		os.Exit(1)
	}
	if err := plex.DeleteSharedServer(token, machineID, current.ID); err != nil {
		log.Printf("Delete shared server failed: %v", err)
		os.Exit(1)
	}
	req := plex.SharedServerRequest{
		MachineIdentifier: machineID,
		LibrarySectionIDs: libraryIDs,
		InvitedID:         userID,
	}
	req.Settings.AllowTuners = requestedAllowTuners
	req.Settings.AllowSync = allowSync
	created, err := plex.CreateSharedServer(token, strings.TrimSpace(clientID), req)
	if err != nil {
		log.Printf("Recreate shared server failed: %v", err)
		os.Exit(1)
	}
	report["create_result"] = created
	after, err := plex.ListSharedServers(token, machineID)
	if err != nil {
		log.Printf("List shared servers after recreate failed: %v", err)
		os.Exit(1)
	}
	for _, item := range after {
		if item.UserID == userID {
			report["shared_server_after"] = item
			break
		}
	}
	writeJSONOrStdout(report, outPath)
}

func writeJSONOrStdout(v any, outPath string) {
	data, _ := json.MarshalIndent(v, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write JSON %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote %s", p)
		return
	}
	os.Stdout.Write(data)
	os.Stdout.WriteString("\n")
}

func parseIntCSV(s string) []int {
	parts := parseCSV(s)
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			log.Printf("Bad integer %q in CSV", part)
			os.Exit(1)
		}
		out = append(out, n)
	}
	return out
}

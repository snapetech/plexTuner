package emby

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

// Config holds the parameters for registering with an Emby or Jellyfin server.
type Config struct {
	// Host is the media server base URL, e.g. "http://192.168.1.10:8096".
	Host string
	// Token is the API key or access token created in the server dashboard.
	Token string
	// TunerURL is this iptvTunerr's base URL, e.g. "http://192.168.1.10:5004".
	// The server will probe TunerURL/discover.json during registration.
	TunerURL string
	// XMLTVURL is the guide URL served by this tuner. Defaults to TunerURL+"/guide.xml".
	XMLTVURL string
	// FriendlyName is the display name shown in the media server UI.
	FriendlyName string
	// TunerCount is the number of concurrent streams to advertise (default 2).
	TunerCount int
	// ServerType is "emby" or "jellyfin" — used only in log prefixes.
	// The APIs are identical; no behaviour differs between the two.
	ServerType string
}

func (c Config) logTag() string {
	if c.ServerType != "" {
		return "[" + c.ServerType + "-reg]"
	}
	return "[emby-reg]"
}

func (c Config) effectiveXMLTVURL() string {
	if c.XMLTVURL != "" {
		return c.XMLTVURL
	}
	return strings.TrimRight(strings.TrimSpace(c.TunerURL), "/") + "/guide.xml"
}

// authHeader returns the MediaBrowser authorization header value accepted by
// both Emby and Jellyfin.
func authHeader(token string) string {
	return fmt.Sprintf(`MediaBrowser Client="iptvTunerr", Device="iptvTunerr", DeviceId="iptvTunerr", Version="1.0.0", Token="%s"`, token)
}

func newHTTPClient() *http.Client {
	return httpclient.WithTimeout(30 * time.Second)
}

func joinHostURL(host, path string) string {
	return strings.TrimRight(strings.TrimSpace(host), "/") + path
}

// apiRequest performs a JSON API request with the MediaBrowser auth header.
// body may be nil for requests without a payload (GET, DELETE).
func apiRequest(client *http.Client, method, url, token string, body interface{}) (int, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", authHeader(token))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, nil
}

// RegisterTunerHost registers this tuner as an HDHomeRun tuner host on the server.
// Returns the server-assigned tuner host ID.
func RegisterTunerHost(cfg Config) (string, error) {
	tunerCount := cfg.TunerCount
	if tunerCount <= 0 {
		tunerCount = 2
	}
	body := TunerHostInfo{
		Type:                "hdhomerun",
		Url:                 cfg.TunerURL,
		FriendlyName:        cfg.FriendlyName,
		TunerCount:          tunerCount,
		ImportFavoritesOnly: false,
		AllowHWTranscoding:  false,
		AllowStreamSharing:  true,
		EnableStreamLooping: false,
		IgnoreDts:           false,
	}
	u := joinHostURL(cfg.Host, "/LiveTv/TunerHosts")
	client := newHTTPClient()
	status, data, err := apiRequest(client, http.MethodPost, u, cfg.Token, body)
	if err != nil {
		return "", fmt.Errorf("register tuner host: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("register tuner host returned %d: %s", status, trunc(string(data), 300))
	}
	var resp TunerHostInfo
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse tuner host response: %w", err)
	}
	if resp.Id == "" {
		return "", fmt.Errorf("tuner host response missing Id field: %s", trunc(string(data), 300))
	}
	return resp.Id, nil
}

func ListTunerHosts(cfg Config) ([]TunerHostInfo, error) {
	u := joinHostURL(cfg.Host, "/LiveTv/TunerHosts")
	client := newHTTPClient()
	status, data, err := apiRequest(client, http.MethodGet, u, cfg.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("list tuner hosts: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list tuner hosts returned %d: %s", status, trunc(string(data), 300))
	}
	var items []TunerHostInfo
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse tuner hosts: %w", err)
	}
	return items, nil
}

// DeleteTunerHost removes a tuner host by ID. Tolerates 404 (already gone).
func DeleteTunerHost(cfg Config, id string) error {
	if id == "" {
		return nil
	}
	u := fmt.Sprintf("%s?id=%s", joinHostURL(cfg.Host, "/LiveTv/TunerHosts"), id)
	client := newHTTPClient()
	status, data, err := apiRequest(client, http.MethodDelete, u, cfg.Token, nil)
	if err != nil {
		return fmt.Errorf("delete tuner host %s: %w", id, err)
	}
	if status != http.StatusOK && status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("delete tuner host %s returned %d: %s", id, status, trunc(string(data), 200))
	}
	return nil
}

// RegisterListingProvider registers the XMLTV guide as a listing provider.
// Returns the server-assigned listing provider ID.
func RegisterListingProvider(cfg Config) (string, error) {
	body := ListingsProviderInfo{
		Type:            "xmltv",
		Path:            cfg.effectiveXMLTVURL(),
		EnableAllTuners: true,
	}
	// validateListings=false avoids a synchronous XMLTV fetch during registration
	// which would block if the guide is not yet populated.
	u := joinHostURL(cfg.Host, "/LiveTv/ListingProviders") + "?validateListings=false&validateLogin=false"
	client := newHTTPClient()
	status, data, err := apiRequest(client, http.MethodPost, u, cfg.Token, body)
	if err != nil {
		return "", fmt.Errorf("register listing provider: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("register listing provider returned %d: %s", status, trunc(string(data), 300))
	}
	var resp ListingsProviderInfo
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse listing provider response: %w", err)
	}
	if resp.Id == "" {
		return "", fmt.Errorf("listing provider response missing Id field: %s", trunc(string(data), 300))
	}
	return resp.Id, nil
}

func ListListingProviders(cfg Config) ([]ListingsProviderInfo, error) {
	u := joinHostURL(cfg.Host, "/LiveTv/ListingProviders")
	client := newHTTPClient()
	status, data, err := apiRequest(client, http.MethodGet, u, cfg.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("list listing providers: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list listing providers returned %d: %s", status, trunc(string(data), 300))
	}
	var items []ListingsProviderInfo
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse listing providers: %w", err)
	}
	return items, nil
}

// DeleteListingProvider removes a listing provider by ID. Tolerates 404 (already gone).
func DeleteListingProvider(cfg Config, id string) error {
	if id == "" {
		return nil
	}
	u := fmt.Sprintf("%s?id=%s", joinHostURL(cfg.Host, "/LiveTv/ListingProviders"), id)
	client := newHTTPClient()
	status, data, err := apiRequest(client, http.MethodDelete, u, cfg.Token, nil)
	if err != nil {
		return fmt.Errorf("delete listing provider %s: %w", id, err)
	}
	if status != http.StatusOK && status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("delete listing provider %s returned %d: %s", id, status, trunc(string(data), 200))
	}
	return nil
}

// TriggerGuideRefresh finds the RefreshGuide scheduled task and starts it.
// This is best-effort: Jellyfin auto-queues a refresh when the tuner is saved,
// so a failure here is logged as a warning but does not fail registration.
func TriggerGuideRefresh(cfg Config) error {
	tasks, err := listScheduledTasks(cfg)
	if err != nil {
		return err
	}
	var taskID string
	for _, t := range tasks {
		if t.Key == "RefreshGuide" {
			taskID = t.Id
			break
		}
	}
	if taskID == "" {
		return fmt.Errorf("RefreshGuide task not found in %d scheduled tasks", len(tasks))
	}
	runURL := joinHostURL(cfg.Host, "/ScheduledTasks/Running/"+taskID)
	client := newHTTPClient()
	status, _, err := apiRequest(client, http.MethodPost, runURL, cfg.Token, nil)
	if err != nil {
		return fmt.Errorf("trigger guide refresh task %s: %w", taskID, err)
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("trigger guide refresh returned %d", status)
	}
	return nil
}

func listScheduledTasks(cfg Config) ([]ScheduledTask, error) {
	client := newHTTPClient()
	u := joinHostURL(cfg.Host, "/ScheduledTasks")
	status, data, err := apiRequest(client, http.MethodGet, u, cfg.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("list scheduled tasks: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list scheduled tasks returned %d", status)
	}
	var tasks []ScheduledTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("parse scheduled tasks: %w", err)
	}
	return tasks, nil
}

// GetChannelCount returns the total number of live TV channels the server has indexed.
// Returns 0 on any error — the watchdog treats 0 as "not healthy".
func GetChannelCount(cfg Config) int {
	u := joinHostURL(cfg.Host, "/LiveTv/Channels") + "?StartIndex=0&Limit=1"
	client := newHTTPClient()
	status, data, err := apiRequest(client, http.MethodGet, u, cfg.Token, nil)
	if err != nil || status != http.StatusOK {
		return 0
	}
	var list LiveTvChannelList
	if err := json.Unmarshal(data, &list); err != nil {
		return 0
	}
	return list.TotalRecordCount
}

// FullRegister registers this tuner with the Emby or Jellyfin server. It is
// idempotent: if stateFile exists with previously registered IDs, those entries
// are deleted first to avoid duplicates. New IDs are then saved back to stateFile.
//
// If stateFile is empty, state is not persisted and registration runs unconditionally
// on every call (fine for single-run or ephemeral containers).
func FullRegister(cfg Config, stateFile string) error {
	tag := cfg.logTag()
	serverLabel := cfg.ServerType
	if serverLabel == "" {
		serverLabel = "emby/jellyfin"
	}
	log.Printf("%s === Starting %s registration ===", tag, serverLabel)
	log.Printf("%s Host=%s TunerURL=%s XMLTV=%s", tag, cfg.Host, cfg.TunerURL, cfg.effectiveXMLTVURL())

	// Clean up any previous registration so we don't accumulate stale entries.
	if stateFile != "" {
		if prev, err := loadState(stateFile); err == nil {
			log.Printf("%s cleaning up previous registration (tunerHost=%s listingProvider=%s)",
				tag, prev.TunerHostID, prev.ListingProviderID)
			if err := DeleteTunerHost(cfg, prev.TunerHostID); err != nil {
				log.Printf("%s warning: delete old tuner host: %v", tag, err)
			}
			if err := DeleteListingProvider(cfg, prev.ListingProviderID); err != nil {
				log.Printf("%s warning: delete old listing provider: %v", tag, err)
			}
		}
	}

	// Step 1: Register tuner host.
	log.Printf("%s Step 1: Registering tuner host...", tag)
	tunerHostID, err := RegisterTunerHost(cfg)
	if err != nil {
		return fmt.Errorf("register tuner host: %w", err)
	}
	log.Printf("%s Tuner host registered: id=%s", tag, tunerHostID)

	// Step 2: Register XMLTV listing provider.
	log.Printf("%s Step 2: Registering XMLTV listing provider...", tag)
	listingProviderID, err := RegisterListingProvider(cfg)
	if err != nil {
		// Non-fatal: the tuner still streams without EPG data. Log and continue.
		log.Printf("%s warning: register listing provider failed: %v (tuner functional; guide unavailable)", tag, err)
		listingProviderID = ""
	} else {
		log.Printf("%s Listing provider registered: id=%s", tag, listingProviderID)
	}

	// Step 3: Kick the guide refresh task. Jellyfin queues one automatically when
	// the tuner host is saved; this is belt-and-suspenders for Emby.
	log.Printf("%s Step 3: Triggering guide refresh...", tag)
	if err := TriggerGuideRefresh(cfg); err != nil {
		log.Printf("%s warning: trigger guide refresh: %v (will refresh on next scheduled run)", tag, err)
	} else {
		log.Printf("%s Guide refresh triggered", tag)
	}

	// Persist state for idempotent future runs.
	if stateFile != "" {
		state := &RegistrationState{
			TunerHostID:       tunerHostID,
			ListingProviderID: listingProviderID,
			TunerURL:          cfg.TunerURL,
			XMLTVURL:          cfg.effectiveXMLTVURL(),
			RegisteredAt:      time.Now(),
		}
		if err := saveState(stateFile, state); err != nil {
			log.Printf("%s warning: save state file %s: %v", tag, stateFile, err)
		}
	}

	log.Printf("%s === Registration complete ===", tag)
	return nil
}

// trunc truncates s to at most n bytes, appending "..." if truncated.
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

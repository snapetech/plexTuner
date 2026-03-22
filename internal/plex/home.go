package plex

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var plexTVBaseURLForTest = "https://plex.tv"
var plexTVHTTPClient = func() *http.Client { return plexHTTPClient() }

type SharedServerRequest struct {
	MachineIdentifier string `json:"machineIdentifier"`
	LibrarySectionIDs []int  `json:"librarySectionIds"`
	InvitedID         int    `json:"invitedId"`
	Settings          struct {
		AllowTuners int `json:"allowTuners"`
		AllowSync   int `json:"allowSync,omitempty"`
	} `json:"settings"`
}

type SharedServer struct {
	ID           int    `json:"id"`
	UserID       int    `json:"user_id"`
	Username     string `json:"username,omitempty"`
	Email        string `json:"email,omitempty"`
	Home         bool   `json:"home"`
	AllowTuners  int    `json:"allow_tuners"`
	AllowSync    int    `json:"allow_sync"`
	AllLibraries bool   `json:"all_libraries"`
}

type SharedServerResult struct {
	Status      int                 `json:"status"`
	Requested   SharedServerRequest `json:"requested"`
	Observed    SharedServer        `json:"observed"`
	BodyPreview string              `json:"body_preview,omitempty"`
}

func CreateSharedServer(plexToken, clientID string, reqBody SharedServerRequest) (*SharedServerResult, error) {
	if strings.TrimSpace(plexToken) == "" {
		return nil, fmt.Errorf("plex token required")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("client identifier required")
	}
	if strings.TrimSpace(reqBody.MachineIdentifier) == "" {
		return nil, fmt.Errorf("machine identifier required")
	}
	if reqBody.InvitedID == 0 {
		return nil, fmt.Errorf("invited user id required")
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(plexTVBaseURLForTest, "/")
	u := fmt.Sprintf("%s/api/v2/shared_servers?X-Plex-Client-Identifier=%s&X-Plex-Token=%s",
		base, url.QueryEscape(clientID), url.QueryEscape(plexToken))
	httpReq, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := plexTVHTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("plex.tv shared_servers create returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return &SharedServerResult{
		Status:      resp.StatusCode,
		Requested:   reqBody,
		Observed:    parseCreatedSharedServer(body),
		BodyPreview: strings.TrimSpace(string(body)),
	}, nil
}

func ListSharedServers(plexToken, machineID string) ([]SharedServer, error) {
	if strings.TrimSpace(plexToken) == "" || strings.TrimSpace(machineID) == "" {
		return nil, fmt.Errorf("plex token and machine id required")
	}
	base := strings.TrimRight(plexTVBaseURLForTest, "/")
	u := fmt.Sprintf("%s/api/servers/%s/shared_servers?X-Plex-Token=%s",
		base, url.PathEscape(machineID), url.QueryEscape(plexToken))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := plexTVHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list shared servers returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseSharedServers(body), nil
}

func DeleteSharedServer(plexToken, machineID string, sharedServerID int) error {
	if strings.TrimSpace(plexToken) == "" || strings.TrimSpace(machineID) == "" || sharedServerID == 0 {
		return fmt.Errorf("plex token, machine id, and shared server id required")
	}
	base := strings.TrimRight(plexTVBaseURLForTest, "/")
	u := fmt.Sprintf("%s/api/servers/%s/shared_servers/%d?X-Plex-Token=%s",
		base, url.PathEscape(machineID), sharedServerID, url.QueryEscape(plexToken))
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	resp, err := plexTVHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete shared server %d returned %d: %s", sharedServerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func parseCreatedSharedServer(body []byte) SharedServer {
	var doc struct {
		ID           string `xml:"id,attr"`
		AllLibraries string `xml:"allLibraries,attr"`
		Invited      struct {
			ID       string `xml:"id,attr"`
			Home     string `xml:"home,attr"`
			Username string `xml:"username,attr"`
			Email    string `xml:"email,attr"`
		} `xml:"invited"`
		SharingSettings struct {
			AllowTuners string `xml:"allowTuners,attr"`
			AllowSync   string `xml:"allowSync,attr"`
		} `xml:"sharingSettings"`
	}
	_ = xml.Unmarshal(body, &doc)
	return SharedServer{
		ID:           atoiLoose(doc.ID),
		UserID:       atoiLoose(doc.Invited.ID),
		Username:     strings.TrimSpace(doc.Invited.Username),
		Email:        strings.TrimSpace(doc.Invited.Email),
		Home:         doc.Invited.Home == "1",
		AllowTuners:  atoiLoose(doc.SharingSettings.AllowTuners),
		AllowSync:    atoiLoose(doc.SharingSettings.AllowSync),
		AllLibraries: doc.AllLibraries == "1",
	}
}

func parseSharedServers(body []byte) []SharedServer {
	var doc struct {
		Items []struct {
			ID           string `xml:"id,attr"`
			UserID       string `xml:"userID,attr"`
			Username     string `xml:"username,attr"`
			Email        string `xml:"email,attr"`
			AllowTuners  string `xml:"allowTuners,attr"`
			AllowSync    string `xml:"allowSync,attr"`
			AllLibraries string `xml:"allLibraries,attr"`
		} `xml:"SharedServer"`
	}
	_ = xml.Unmarshal(body, &doc)
	out := make([]SharedServer, 0, len(doc.Items))
	for _, item := range doc.Items {
		out = append(out, SharedServer{
			ID:           atoiLoose(item.ID),
			UserID:       atoiLoose(item.UserID),
			Username:     strings.TrimSpace(item.Username),
			Email:        strings.TrimSpace(item.Email),
			AllowTuners:  atoiLoose(item.AllowTuners),
			AllowSync:    atoiLoose(item.AllowSync),
			AllLibraries: item.AllLibraries == "1",
		})
	}
	return out
}

func atoiLoose(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

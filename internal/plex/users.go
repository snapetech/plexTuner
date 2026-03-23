package plex

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type UserInfo struct {
	ID         int    `json:"id"`
	UUID       string `json:"uuid,omitempty"`
	Username   string `json:"username,omitempty"`
	Title      string `json:"title,omitempty"`
	Email      string `json:"email,omitempty"`
	Home       bool   `json:"home"`
	Managed    bool   `json:"managed"`
	Restricted bool   `json:"restricted"`
}

func ListUsers(plexToken string) ([]UserInfo, error) {
	plexToken = strings.TrimSpace(plexToken)
	if plexToken == "" {
		return nil, fmt.Errorf("plex token required")
	}
	base := strings.TrimRight(strings.TrimSpace(plexTVBaseURLForTest), "/")
	u := fmt.Sprintf("%s/api/users?X-Plex-Token=%s", base, url.QueryEscape(plexToken))
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
		return nil, fmt.Errorf("list users returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var doc struct {
		Users []struct {
			ID         string `xml:"id,attr"`
			UUID       string `xml:"uuid,attr"`
			Username   string `xml:"username,attr"`
			Title      string `xml:"title,attr"`
			Email      string `xml:"email,attr"`
			Home       string `xml:"home,attr"`
			Restricted string `xml:"restricted,attr"`
		} `xml:"User"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse users: %w", err)
	}
	out := make([]UserInfo, 0, len(doc.Users))
	for _, item := range doc.Users {
		restricted := item.Restricted == "1" || strings.EqualFold(strings.TrimSpace(item.Restricted), "true")
		out = append(out, UserInfo{
			ID:         atoiLoose(item.ID),
			UUID:       strings.TrimSpace(item.UUID),
			Username:   strings.TrimSpace(item.Username),
			Title:      strings.TrimSpace(item.Title),
			Email:      strings.TrimSpace(item.Email),
			Home:       item.Home == "1" || strings.EqualFold(strings.TrimSpace(item.Home), "true"),
			Managed:    restricted,
			Restricted: restricted,
		})
	}
	return out, nil
}

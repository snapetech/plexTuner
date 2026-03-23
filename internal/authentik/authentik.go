package authentik

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

type Config struct {
	Host  string
	Token string
}

type User struct {
	ID         string         `json:"id,omitempty"`
	Username   string         `json:"username,omitempty"`
	Name       string         `json:"name,omitempty"`
	Email      string         `json:"email,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Groups     []string       `json:"groups,omitempty"`
}

type Group struct {
	ID    string   `json:"id,omitempty"`
	Name  string   `json:"name,omitempty"`
	Users []string `json:"users,omitempty"`
}

func ListUsers(cfg Config) ([]User, error) {
	status, data, err := apiRequest(cfg, http.MethodGet, apiURL(cfg, "/core/users/?page_size=200"), nil)
	if err != nil {
		return nil, fmt.Errorf("list authentik users: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list authentik users returned %d: %s", status, trunc(string(data), 300))
	}
	users, err := decodeUserList(data)
	if err != nil {
		return nil, fmt.Errorf("parse authentik users: %w", err)
	}
	return users, nil
}

func GetUser(cfg Config, userID string) (*User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user id required")
	}
	status, data, err := apiRequest(cfg, http.MethodGet, apiURL(cfg, "/core/users/"+url.PathEscape(userID)+"/"), nil)
	if err != nil {
		return nil, fmt.Errorf("get authentik user: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("get authentik user returned %d: %s", status, trunc(string(data), 300))
	}
	user, err := decodeUser(data)
	if err != nil {
		return nil, fmt.Errorf("parse authentik user: %w", err)
	}
	return user, nil
}

func ListGroups(cfg Config) ([]Group, error) {
	status, data, err := apiRequest(cfg, http.MethodGet, apiURL(cfg, "/core/groups/?page_size=200"), nil)
	if err != nil {
		return nil, fmt.Errorf("list authentik groups: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list authentik groups returned %d: %s", status, trunc(string(data), 300))
	}
	groups, err := decodeGroupList(data)
	if err != nil {
		return nil, fmt.Errorf("parse authentik groups: %w", err)
	}
	return groups, nil
}

func CreateUser(cfg Config, user User) (string, error) {
	if strings.TrimSpace(user.Username) == "" {
		return "", fmt.Errorf("username required")
	}
	body := map[string]any{
		"username":  user.Username,
		"name":      strings.TrimSpace(firstNonEmpty(user.Name, user.Username)),
		"is_active": true,
	}
	if email := strings.TrimSpace(user.Email); email != "" {
		body["email"] = email
	}
	if len(user.Attributes) > 0 {
		body["attributes"] = user.Attributes
	}
	status, data, err := apiRequest(cfg, http.MethodPost, apiURL(cfg, "/core/users/"), body)
	if err != nil {
		return "", fmt.Errorf("create authentik user %q: %w", user.Username, err)
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return "", fmt.Errorf("create authentik user %q returned %d: %s", user.Username, status, trunc(string(data), 300))
	}
	if created, err := decodeUser(data); err == nil && strings.TrimSpace(created.ID) != "" {
		return strings.TrimSpace(created.ID), nil
	}
	users, err := ListUsers(cfg)
	if err != nil {
		return "", err
	}
	if found := FindUserByUsername(users, user.Username); found != nil {
		return strings.TrimSpace(found.ID), nil
	}
	return "", fmt.Errorf("created authentik user %q but could not resolve id", user.Username)
}

func UpdateUser(cfg Config, userID string, patch map[string]any) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user id required")
	}
	if len(patch) == 0 {
		return nil
	}
	status, data, err := apiRequest(cfg, http.MethodPatch, apiURL(cfg, "/core/users/"+url.PathEscape(userID)+"/"), patch)
	if err != nil {
		return fmt.Errorf("update authentik user %q: %w", userID, err)
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("update authentik user %q returned %d: %s", userID, status, trunc(string(data), 300))
	}
	return nil
}

func CreateGroup(cfg Config, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("group name required")
	}
	status, data, err := apiRequest(cfg, http.MethodPost, apiURL(cfg, "/core/groups/"), map[string]any{"name": name, "is_superuser": false})
	if err != nil {
		return "", fmt.Errorf("create authentik group %q: %w", name, err)
	}
	if status != http.StatusCreated && status != http.StatusOK && status != http.StatusConflict {
		return "", fmt.Errorf("create authentik group %q returned %d: %s", name, status, trunc(string(data), 300))
	}
	if created, err := decodeGroup(data); err == nil && strings.TrimSpace(created.ID) != "" {
		return strings.TrimSpace(created.ID), nil
	}
	groups, err := ListGroups(cfg)
	if err != nil {
		return "", err
	}
	if found := FindGroup(groups, name); found != nil {
		return strings.TrimSpace(found.ID), nil
	}
	return "", fmt.Errorf("created authentik group %q but could not resolve id", name)
}

func AddUserToGroup(cfg Config, groupID, userID string) error {
	groupID = strings.TrimSpace(groupID)
	userID = strings.TrimSpace(userID)
	if groupID == "" || userID == "" {
		return fmt.Errorf("group id and user id required")
	}
	status, data, err := apiRequest(cfg, http.MethodPost, apiURL(cfg, "/core/groups/"+url.PathEscape(groupID)+"/add_user/"), map[string]any{"pk": userID})
	if err != nil {
		return fmt.Errorf("add authentik user %q to group %q: %w", userID, groupID, err)
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("add authentik user %q to group %q returned %d: %s", userID, groupID, status, trunc(string(data), 300))
	}
	return nil
}

func SetPassword(cfg Config, userID, password string) error {
	userID = strings.TrimSpace(userID)
	password = strings.TrimSpace(password)
	if userID == "" {
		return fmt.Errorf("user id required")
	}
	if password == "" {
		return fmt.Errorf("password required")
	}
	status, data, err := apiRequest(cfg, http.MethodPost, apiURL(cfg, "/core/users/"+url.PathEscape(userID)+"/set_password/"), map[string]any{
		"password":        password,
		"password_repeat": password,
	})
	if err != nil {
		return fmt.Errorf("set authentik password for %q: %w", userID, err)
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("set authentik password for %q returned %d: %s", userID, status, trunc(string(data), 300))
	}
	return nil
}

func SendRecoveryEmail(cfg Config, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user id required")
	}
	status, data, err := apiRequest(cfg, http.MethodPost, apiURL(cfg, "/core/users/"+url.PathEscape(userID)+"/recovery_email/"), map[string]any{})
	if err != nil {
		return fmt.Errorf("send authentik recovery email for %q: %w", userID, err)
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("send authentik recovery email for %q returned %d: %s", userID, status, trunc(string(data), 300))
	}
	return nil
}

func FindUserByUsername(users []User, username string) *User {
	username = strings.TrimSpace(username)
	for _, user := range users {
		if strings.EqualFold(strings.TrimSpace(user.Username), username) {
			copy := user
			return &copy
		}
	}
	return nil
}

func FindGroup(groups []Group, name string) *Group {
	name = strings.TrimSpace(name)
	for _, group := range groups {
		if strings.EqualFold(strings.TrimSpace(group.Name), name) {
			copy := group
			return &copy
		}
	}
	return nil
}

func apiURL(cfg Config, path string) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.Host), "/")
	if !strings.Contains(base, "/api/v3") {
		base += "/api/v3"
	}
	return base + path
}

func apiRequest(cfg Config, method, target string, body any) (int, []byte, error) {
	client := httpclient.WithTimeout(30 * time.Second)
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(cfg.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, data, nil
}

func decodeUserList(data []byte) ([]User, error) {
	raw, err := decodeListResults(data)
	if err != nil {
		return nil, err
	}
	users := make([]User, 0, len(raw))
	for _, entry := range raw {
		user, err := decodeUser(entry)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, nil
}

func decodeGroupList(data []byte) ([]Group, error) {
	raw, err := decodeListResults(data)
	if err != nil {
		return nil, err
	}
	groups := make([]Group, 0, len(raw))
	for _, entry := range raw {
		group, err := decodeGroup(entry)
		if err != nil {
			return nil, err
		}
		groups = append(groups, *group)
	}
	return groups, nil
}

func decodeListResults(data []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var list []json.RawMessage
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	var page struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(trimmed, &page); err != nil {
		return nil, err
	}
	return page.Results, nil
}

func decodeUser(data []byte) (*User, error) {
	var raw struct {
		ID         any            `json:"id"`
		PK         any            `json:"pk"`
		Username   string         `json:"username"`
		Name       string         `json:"name"`
		Email      string         `json:"email"`
		Attributes map[string]any `json:"attributes"`
		Groups     []any          `json:"groups"`
		AKGroups   []any          `json:"ak_groups"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &raw); err != nil {
		return nil, err
	}
	groups := toStringSlice(raw.Groups)
	groups = append(groups, toStringSlice(raw.AKGroups)...)
	return &User{
		ID:         stringifyID(firstNonNil(raw.PK, raw.ID)),
		Username:   strings.TrimSpace(raw.Username),
		Name:       strings.TrimSpace(raw.Name),
		Email:      strings.TrimSpace(raw.Email),
		Attributes: raw.Attributes,
		Groups:     compactStrings(groups),
	}, nil
}

func decodeGroup(data []byte) (*Group, error) {
	var raw struct {
		ID    any    `json:"id"`
		PK    any    `json:"pk"`
		Name  string `json:"name"`
		Users []any  `json:"users"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &raw); err != nil {
		return nil, err
	}
	return &Group{
		ID:    stringifyID(firstNonNil(raw.PK, raw.ID)),
		Name:  strings.TrimSpace(raw.Name),
		Users: compactStrings(toStringSlice(raw.Users)),
	}, nil
}

func toStringSlice(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if str := stringifyID(value); str != "" {
			out = append(out, str)
		}
	}
	return out
}

func stringifyID(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil && stringifyID(value) != "" {
			return value
		}
	}
	return nil
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

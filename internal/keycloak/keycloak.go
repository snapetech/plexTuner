package keycloak

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

type Config struct {
	Host     string
	Realm    string
	Token    string
	Username string
	Password string
}

type User struct {
	ID              string              `json:"id,omitempty"`
	Username        string              `json:"username,omitempty"`
	Email           string              `json:"email,omitempty"`
	FirstName       string              `json:"firstName,omitempty"`
	LastName        string              `json:"lastName,omitempty"`
	Enabled         bool                `json:"enabled,omitempty"`
	Groups          []string            `json:"groups,omitempty"`
	Attributes      map[string][]string `json:"attributes,omitempty"`
	RequiredActions []string            `json:"requiredActions,omitempty"`
}

type Group struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

type Credential struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary,omitempty"`
}

type ExecuteActionsEmailOptions struct {
	ClientID    string
	RedirectURI string
	LifespanSec int
}

func ResolveConfig(cfg Config) (Config, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Realm = strings.TrimSpace(cfg.Realm)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.Password = strings.TrimSpace(cfg.Password)
	if cfg.Host == "" {
		return Config{}, fmt.Errorf("keycloak host required")
	}
	if cfg.Realm == "" {
		return Config{}, fmt.Errorf("keycloak realm required")
	}
	if cfg.Username != "" || cfg.Password != "" {
		if cfg.Username == "" || cfg.Password == "" {
			return Config{}, fmt.Errorf("keycloak username and password required together")
		}
		token, err := MintAdminToken(cfg.Host, cfg.Realm, cfg.Username, cfg.Password)
		if err != nil {
			return Config{}, err
		}
		cfg.Token = token
	}
	if cfg.Token == "" {
		return Config{}, fmt.Errorf("keycloak token required")
	}
	return cfg, nil
}

func MintAdminToken(host, realm, username, password string) (string, error) {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	realm = strings.TrimSpace(realm)
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if host == "" {
		return "", fmt.Errorf("keycloak host required")
	}
	if realm == "" {
		return "", fmt.Errorf("keycloak realm required")
	}
	if username == "" || password == "" {
		return "", fmt.Errorf("keycloak username and password required")
	}
	form := url.Values{}
	form.Set("client_id", "admin-cli")
	form.Set("grant_type", "password")
	form.Set("username", username)
	form.Set("password", password)
	req, err := http.NewRequest(http.MethodPost, host+"/realms/"+url.PathEscape(realm)+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := httpclient.WithTimeout(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("mint keycloak admin token: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mint keycloak admin token returned %d: %s", resp.StatusCode, trunc(string(data), 300))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("parse keycloak admin token response: %w", err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", fmt.Errorf("keycloak admin token response missing access_token")
	}
	return strings.TrimSpace(parsed.AccessToken), nil
}

func ListUsers(cfg Config) ([]User, error) {
	status, data, err := apiRequest(cfg, http.MethodGet, adminURL(cfg, "/users"), nil)
	if err != nil {
		return nil, fmt.Errorf("list keycloak users: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list keycloak users returned %d: %s", status, trunc(string(data), 300))
	}
	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("parse keycloak users: %w", err)
	}
	return users, nil
}

func GetUser(cfg Config, userID string) (*User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user id required")
	}
	status, data, err := apiRequest(cfg, http.MethodGet, adminURL(cfg, "/users/"+url.PathEscape(userID)), nil)
	if err != nil {
		return nil, fmt.Errorf("get keycloak user: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("get keycloak user returned %d: %s", status, trunc(string(data), 300))
	}
	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("parse keycloak user: %w", err)
	}
	return &user, nil
}

func GetUserGroups(cfg Config, userID string) ([]Group, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user id required")
	}
	status, data, err := apiRequest(cfg, http.MethodGet, adminURL(cfg, "/users/"+url.PathEscape(userID)+"/groups"), nil)
	if err != nil {
		return nil, fmt.Errorf("get keycloak user groups: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("get keycloak user groups returned %d: %s", status, trunc(string(data), 300))
	}
	var groups []Group
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("parse keycloak user groups: %w", err)
	}
	return groups, nil
}

func ListGroups(cfg Config) ([]Group, error) {
	status, data, err := apiRequest(cfg, http.MethodGet, adminURL(cfg, "/groups"), nil)
	if err != nil {
		return nil, fmt.Errorf("list keycloak groups: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list keycloak groups returned %d: %s", status, trunc(string(data), 300))
	}
	var groups []Group
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("parse keycloak groups: %w", err)
	}
	return groups, nil
}

func CreateUser(cfg Config, user User) (string, error) {
	if strings.TrimSpace(user.Username) == "" {
		return "", fmt.Errorf("username required")
	}
	user.Enabled = true
	status, data, err := apiRequest(cfg, http.MethodPost, adminURL(cfg, "/users"), user)
	if err != nil {
		return "", fmt.Errorf("create keycloak user %q: %w", user.Username, err)
	}
	if status != http.StatusCreated && status != http.StatusNoContent {
		return "", fmt.Errorf("create keycloak user %q returned %d: %s", user.Username, status, trunc(string(data), 300))
	}
	users, err := ListUsers(cfg)
	if err != nil {
		return "", err
	}
	if found := FindUserByUsername(users, user.Username); found != nil {
		return strings.TrimSpace(found.ID), nil
	}
	return "", fmt.Errorf("created keycloak user %q but could not resolve id", user.Username)
}

func UpdateUser(cfg Config, userID string, user User) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user id required")
	}
	status, data, err := apiRequest(cfg, http.MethodPut, adminURL(cfg, "/users/"+url.PathEscape(userID)), user)
	if err != nil {
		return fmt.Errorf("update keycloak user %q: %w", userID, err)
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("update keycloak user %q returned %d: %s", userID, status, trunc(string(data), 300))
	}
	return nil
}

func CreateGroup(cfg Config, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("group name required")
	}
	status, data, err := apiRequest(cfg, http.MethodPost, adminURL(cfg, "/groups"), map[string]any{"name": name})
	if err != nil {
		return "", fmt.Errorf("create keycloak group %q: %w", name, err)
	}
	if status != http.StatusCreated && status != http.StatusNoContent && status != http.StatusConflict {
		return "", fmt.Errorf("create keycloak group %q returned %d: %s", name, status, trunc(string(data), 300))
	}
	groups, err := ListGroups(cfg)
	if err != nil {
		return "", err
	}
	if found := FindGroup(groups, name); found != nil {
		return strings.TrimSpace(found.ID), nil
	}
	return "", fmt.Errorf("created keycloak group %q but could not resolve id", name)
}

func AddUserToGroup(cfg Config, userID, groupID string) error {
	userID = strings.TrimSpace(userID)
	groupID = strings.TrimSpace(groupID)
	if userID == "" || groupID == "" {
		return fmt.Errorf("user id and group id required")
	}
	status, data, err := apiRequest(cfg, http.MethodPut, adminURL(cfg, "/users/"+url.PathEscape(userID)+"/groups/"+url.PathEscape(groupID)), nil)
	if err != nil {
		return fmt.Errorf("add keycloak user %q to group %q: %w", userID, groupID, err)
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("add keycloak user %q to group %q returned %d: %s", userID, groupID, status, trunc(string(data), 300))
	}
	return nil
}

func ResetPassword(cfg Config, userID string, credential Credential) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user id required")
	}
	if strings.TrimSpace(credential.Type) == "" {
		credential.Type = "password"
	}
	if strings.TrimSpace(credential.Value) == "" {
		return fmt.Errorf("credential value required")
	}
	status, data, err := apiRequest(cfg, http.MethodPut, adminURL(cfg, "/users/"+url.PathEscape(userID)+"/reset-password"), credential)
	if err != nil {
		return fmt.Errorf("reset keycloak password for %q: %w", userID, err)
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("reset keycloak password for %q returned %d: %s", userID, status, trunc(string(data), 300))
	}
	return nil
}

func ExecuteActionsEmail(cfg Config, userID string, actions []string, opts ExecuteActionsEmailOptions) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user id required")
	}
	trimmed := make([]string, 0, len(actions))
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action != "" {
			trimmed = append(trimmed, action)
		}
	}
	if len(trimmed) == 0 {
		return fmt.Errorf("at least one action required")
	}
	values := url.Values{}
	if clientID := strings.TrimSpace(opts.ClientID); clientID != "" {
		values.Set("client_id", clientID)
	}
	if redirectURI := strings.TrimSpace(opts.RedirectURI); redirectURI != "" {
		values.Set("redirect_uri", redirectURI)
	}
	if opts.LifespanSec > 0 {
		values.Set("lifespan", fmt.Sprintf("%d", opts.LifespanSec))
	}
	target := adminURL(cfg, "/users/"+url.PathEscape(userID)+"/execute-actions-email")
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	status, data, err := apiRequest(cfg, http.MethodPut, target, trimmed)
	if err != nil {
		return fmt.Errorf("execute keycloak actions email for %q: %w", userID, err)
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("execute keycloak actions email for %q returned %d: %s", userID, status, trunc(string(data), 300))
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

func FindGroup(groups []Group, nameOrPath string) *Group {
	nameOrPath = strings.TrimSpace(nameOrPath)
	for _, group := range groups {
		if strings.EqualFold(strings.TrimSpace(group.Name), nameOrPath) || strings.EqualFold(strings.TrimSpace(group.Path), nameOrPath) {
			copy := group
			return &copy
		}
		if strings.EqualFold("/"+strings.TrimSpace(group.Name), nameOrPath) {
			copy := group
			return &copy
		}
	}
	return nil
}

func adminURL(cfg Config, path string) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.Host), "/")
	realm := url.PathEscape(strings.TrimSpace(cfg.Realm))
	return base + "/admin/realms/" + realm + path
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
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
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

func trunc(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

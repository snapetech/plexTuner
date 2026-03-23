package emby

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

func ListUsers(cfg Config) ([]UserInfo, error) {
	client := newHTTPClient()
	u := joinHostURL(cfg.Host, "/Users")
	status, data, err := apiRequest(client, http.MethodGet, u, cfg.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list users returned %d: %s", status, trunc(string(data), 300))
	}
	var users []UserInfo
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("parse users: %w", err)
	}
	return users, nil
}

func CreateUser(cfg Config, name string) (*UserInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("user name required")
	}
	client := newHTTPClient()
	q := url.Values{}
	q.Set("Name", name)
	u := joinHostURL(cfg.Host, "/Users/New") + "?" + q.Encode()
	status, data, err := apiRequest(client, http.MethodPost, u, cfg.Token, map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("create user %q: %w", name, err)
	}
	switch status {
	case http.StatusOK, http.StatusCreated:
		var user UserInfo
		if err := json.Unmarshal(data, &user); err != nil {
			return nil, fmt.Errorf("parse created user %q: %w", name, err)
		}
		return &user, nil
	case http.StatusNoContent:
		users, err := ListUsers(cfg)
		if err != nil {
			return nil, fmt.Errorf("lookup created user %q: %w", name, err)
		}
		if user := FindUserByName(users, name); user != nil {
			return user, nil
		}
		return nil, fmt.Errorf("created user %q but follow-up lookup did not find it", name)
	default:
		return nil, fmt.Errorf("create user %q returned %d: %s", name, status, trunc(string(data), 300))
	}
}

func GetUser(cfg Config, id string) (*UserInfo, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("user id required")
	}
	client := newHTTPClient()
	u := joinHostURL(cfg.Host, "/Users/"+url.PathEscape(id))
	status, data, err := apiRequest(client, http.MethodGet, u, cfg.Token, nil)
	if err != nil {
		return nil, fmt.Errorf("get user %q: %w", id, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("get user %q returned %d: %s", id, status, trunc(string(data), 300))
	}
	var user UserInfo
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("parse user %q: %w", id, err)
	}
	return &user, nil
}

func UpdateUserPolicy(cfg Config, id string, policy map[string]any) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("user id required")
	}
	if len(policy) == 0 {
		return fmt.Errorf("user policy required")
	}
	client := newHTTPClient()
	u := joinHostURL(cfg.Host, "/Users/"+url.PathEscape(id)+"/Policy")
	status, data, err := apiRequest(client, http.MethodPost, u, cfg.Token, policy)
	if err != nil {
		return fmt.Errorf("update user policy %q: %w", id, err)
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("update user policy %q returned %d: %s", id, status, trunc(string(data), 300))
	}
	return nil
}

func FindUserByName(users []UserInfo, name string) *UserInfo {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for _, user := range users {
		if strings.EqualFold(strings.TrimSpace(user.Name), name) {
			copy := user
			return &copy
		}
	}
	return nil
}

func MergeDesiredUserPolicy(current map[string]any, desired DesiredUserPolicy) (map[string]any, []string, error) {
	if !desired.HasAnyValue() {
		return clonePolicyMap(current), nil, nil
	}
	if len(current) == 0 {
		return nil, nil, fmt.Errorf("current user policy unavailable")
	}
	next := clonePolicyMap(current)
	drift := make([]string, 0, 5)
	if applyDesiredPolicyValue(next, "EnableLiveTvAccess", desired.EnableLiveTvAccess) {
		drift = append(drift, "live_tv_access")
	}
	if applyDesiredPolicyValue(next, "EnableRemoteAccess", desired.EnableRemoteAccess) {
		drift = append(drift, "remote_access")
	}
	if applyDesiredPolicyValue(next, "EnableContentDownloading", desired.EnableContentDownloading) {
		drift = append(drift, "content_downloading")
	}
	if applyDesiredPolicyValue(next, "EnableSyncTranscoding", desired.EnableSyncTranscoding) {
		drift = append(drift, "sync_transcoding")
	}
	if applyDesiredPolicyValue(next, "EnableAllFolders", desired.EnableAllFolders) {
		drift = append(drift, "all_folders")
	}
	slices.Sort(drift)
	return next, drift, nil
}

func (p DesiredUserPolicy) HasAnyValue() bool {
	return p.EnableLiveTvAccess != nil ||
		p.EnableRemoteAccess != nil ||
		p.EnableContentDownloading != nil ||
		p.EnableSyncTranscoding != nil ||
		p.EnableAllFolders != nil
}

func clonePolicyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func applyDesiredPolicyValue(policy map[string]any, key string, desired *bool) bool {
	if desired == nil {
		return false
	}
	if current, ok := policyBool(policy, key); ok && current == *desired {
		return false
	}
	policy[key] = *desired
	return true
}

func policyBool(policy map[string]any, key string) (bool, bool) {
	value, ok := policy[strings.TrimSpace(key)]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case nil:
		return false, false
	default:
		return false, false
	}
}

func UserActivationPending(user UserInfo) (bool, string) {
	if strings.TrimSpace(user.ID) == "" {
		return false, ""
	}
	if user.IsDisabled {
		return false, ""
	}
	if user.EnableAutoLogin || user.HasConfiguredPassword || user.HasConfiguredEasyPwd {
		return false, ""
	}
	return true, "no configured password or auto-login detected"
}

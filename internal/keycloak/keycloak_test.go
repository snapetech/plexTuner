package keycloak

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testConfig(host string) Config {
	return Config{Host: host, Realm: "master", Token: "token"}
}

func TestMintAdminTokenAndResolveConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/realms/master/protocol/openid-connect/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if r.Form.Get("client_id") != "admin-cli" || r.Form.Get("grant_type") != "password" {
				t.Fatalf("form=%v", r.Form)
			}
			if r.Form.Get("username") != "admin" || r.Form.Get("password") != "secret" {
				t.Fatalf("form=%v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh-token"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	token, err := MintAdminToken(srv.URL, "master", "admin", "secret")
	if err != nil {
		t.Fatalf("MintAdminToken: %v", err)
	}
	if token != "fresh-token" {
		t.Fatalf("token=%q", token)
	}
	cfg, err := ResolveConfig(Config{Host: srv.URL, Realm: "master", Username: "admin", Password: "secret"})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if cfg.Token != "fresh-token" {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestListUsersAndFindUserByUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/realms/master/users" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]User{{ID: "1", Username: "alice"}})
	}))
	defer srv.Close()

	users, err := ListUsers(testConfig(srv.URL))
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 || FindUserByUsername(users, "ALICE") == nil {
		t.Fatalf("users=%+v", users)
	}
}

func TestCreateGroupAndAddUserToGroup(t *testing.T) {
	var addedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/master/groups":
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/groups":
			_ = json.NewEncoder(w).Encode([]Group{{ID: "g1", Name: "tunerr:live-tv", Path: "/tunerr:live-tv"}})
		case r.Method == http.MethodPut && r.URL.Path == "/admin/realms/master/users/u1/groups/g1":
			addedPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	groupID, err := CreateGroup(testConfig(srv.URL), "tunerr:live-tv")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if groupID != "g1" {
		t.Fatalf("groupID=%q", groupID)
	}
	if err := AddUserToGroup(testConfig(srv.URL), "u1", "g1"); err != nil {
		t.Fatalf("AddUserToGroup: %v", err)
	}
	if addedPath == "" {
		t.Fatal("group membership not applied")
	}
}

func TestCreateUser(t *testing.T) {
	var gotBody User
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/master/users":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users":
			_ = json.NewEncoder(w).Encode([]User{{ID: "u1", Username: "alice"}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	id, err := CreateUser(testConfig(srv.URL), User{Username: "alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if id != "u1" || gotBody.Username != "alice" || !gotBody.Enabled {
		t.Fatalf("id=%q body=%+v", id, gotBody)
	}
}

func TestGetAndUpdateUser(t *testing.T) {
	var gotBody User
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u1":
			_ = json.NewEncoder(w).Encode(User{ID: "u1", Username: "alice", Email: "old@example.com"})
		case r.Method == http.MethodPut && r.URL.Path == "/admin/realms/master/users/u1":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	user, err := GetUser(testConfig(srv.URL), "u1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.Email != "old@example.com" {
		t.Fatalf("user=%+v", user)
	}
	if err := UpdateUser(testConfig(srv.URL), "u1", User{Username: "alice", Email: "new@example.com"}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if gotBody.Email != "new@example.com" {
		t.Fatalf("gotBody=%+v", gotBody)
	}
}

func TestFindGroup(t *testing.T) {
	group := FindGroup([]Group{{ID: "g1", Name: "tunerr:sync", Path: "/tunerr:sync"}}, "/tunerr:sync")
	if group == nil || !strings.EqualFold(group.ID, "g1") {
		t.Fatalf("group=%+v", group)
	}
}

func TestResetPasswordAndExecuteActionsEmail(t *testing.T) {
	var passwordBody Credential
	var emailActions []string
	var emailQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/admin/realms/master/users/u1/reset-password":
			if err := json.NewDecoder(r.Body).Decode(&passwordBody); err != nil {
				t.Fatalf("decode password body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/admin/realms/master/users/u1/execute-actions-email":
			if err := json.NewDecoder(r.Body).Decode(&emailActions); err != nil {
				t.Fatalf("decode actions body: %v", err)
			}
			emailQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	if err := ResetPassword(testConfig(srv.URL), "u1", Credential{Type: "password", Value: "Temp123!", Temporary: true}); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if passwordBody.Value != "Temp123!" || !passwordBody.Temporary {
		t.Fatalf("passwordBody=%+v", passwordBody)
	}
	if err := ExecuteActionsEmail(testConfig(srv.URL), "u1", []string{"UPDATE_PASSWORD", "VERIFY_EMAIL"}, ExecuteActionsEmailOptions{
		ClientID:    "iptvtunerr",
		RedirectURI: "https://example.com/callback",
		LifespanSec: 1800,
	}); err != nil {
		t.Fatalf("ExecuteActionsEmail: %v", err)
	}
	if len(emailActions) != 2 || !strings.Contains(emailQuery, "client_id=iptvtunerr") || !strings.Contains(emailQuery, "lifespan=1800") {
		t.Fatalf("emailActions=%v emailQuery=%q", emailActions, emailQuery)
	}
}

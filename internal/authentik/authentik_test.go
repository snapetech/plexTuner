package authentik

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testConfig(host string) Config {
	return Config{Host: host, Token: "token"}
}

func TestListUsersAndFindUserByUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/core/users/" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"pk": 1, "username": "alice", "groups": []any{7}}},
		})
	}))
	defer srv.Close()

	users, err := ListUsers(testConfig(srv.URL))
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 || FindUserByUsername(users, "ALICE") == nil || users[0].Groups[0] != "7" {
		t.Fatalf("users=%+v", users)
	}
}

func TestCreateGroupAndAddUserToGroup(t *testing.T) {
	var addedPath string
	var addBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/core/groups/":
			_ = json.NewEncoder(w).Encode(map[string]any{"pk": "g1", "name": "tunerr:live-tv"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/core/groups/g1/add_user/":
			addedPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&addBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
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
	if err := AddUserToGroup(testConfig(srv.URL), "g1", "u1"); err != nil {
		t.Fatalf("AddUserToGroup: %v", err)
	}
	if addedPath == "" || addBody["pk"] != "u1" {
		t.Fatalf("addedPath=%q addBody=%v", addedPath, addBody)
	}
}

func TestCreateUserAndGetUser(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/core/users/":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"pk": 2, "username": "alice"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/core/users/2/":
			_ = json.NewEncoder(w).Encode(map[string]any{"pk": 2, "username": "alice", "ak_groups": []any{"7"}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	id, err := CreateUser(testConfig(srv.URL), User{
		Username:   "alice",
		Name:       "Alice Example",
		Email:      "alice@example.com",
		Attributes: map[string]any{"tunerr_subject_hint": "plex:u-1"},
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if id != "2" || gotBody["username"] != "alice" {
		t.Fatalf("id=%q body=%+v", id, gotBody)
	}
	attrs, _ := gotBody["attributes"].(map[string]any)
	if attrs["tunerr_subject_hint"] != "plex:u-1" {
		t.Fatalf("attrs=%v", attrs)
	}
	user, err := GetUser(testConfig(srv.URL), "2")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.ID != "2" || len(user.Groups) != 1 || user.Groups[0] != "7" {
		t.Fatalf("user=%+v", user)
	}
}

func TestUpdateUser(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v3/core/users/2/":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	if err := UpdateUser(testConfig(srv.URL), "2", map[string]any{"email": "new@example.com"}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if gotBody["email"] != "new@example.com" {
		t.Fatalf("gotBody=%v", gotBody)
	}
}

func TestSetPasswordAndSendRecoveryEmail(t *testing.T) {
	var passwordBody map[string]any
	var recoveryPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/core/users/u1/set_password/":
			if err := json.NewDecoder(r.Body).Decode(&passwordBody); err != nil {
				t.Fatalf("decode password body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/core/users/u1/recovery_email/":
			recoveryPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	if err := SetPassword(testConfig(srv.URL), "u1", "Temp123!"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if passwordBody["password"] != "Temp123!" || passwordBody["password_repeat"] != "Temp123!" {
		t.Fatalf("passwordBody=%v", passwordBody)
	}
	if err := SendRecoveryEmail(testConfig(srv.URL), "u1"); err != nil {
		t.Fatalf("SendRecoveryEmail: %v", err)
	}
	if !strings.Contains(recoveryPath, "/recovery_email/") {
		t.Fatalf("recoveryPath=%q", recoveryPath)
	}
}

func TestFindGroup(t *testing.T) {
	group := FindGroup([]Group{{ID: "g1", Name: "tunerr:sync"}}, "tunerr:sync")
	if group == nil || !strings.EqualFold(group.ID, "g1") {
		t.Fatalf("group=%+v", group)
	}
}

package emby

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Users" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]UserInfo{
			{ID: "u-1", Name: "alice", Policy: map[string]any{"EnableLiveTvAccess": true}},
			{ID: "u-2", Name: "bob"},
		})
	}))
	defer srv.Close()

	users, err := ListUsers(newTestConfig(srv.URL, "emby"))
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 || users[0].Name != "alice" {
		t.Fatalf("users=%+v", users)
	}
	if users[0].Policy["EnableLiveTvAccess"] != true {
		t.Fatalf("policy=%+v", users[0].Policy)
	}
}

func TestCreateUser(t *testing.T) {
	var gotName string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/Users/New" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotName = r.URL.Query().Get("Name")
		_ = json.NewEncoder(w).Encode(UserInfo{ID: "u-1", Name: gotName})
	}))
	defer srv.Close()

	user, err := CreateUser(newTestConfig(srv.URL, "jellyfin"), "alice")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if gotName != "alice" {
		t.Fatalf("gotName=%q", gotName)
	}
	if user == nil || user.Name != "alice" {
		t.Fatalf("user=%+v", user)
	}
}

func TestCreateUserNoContentFallsBackToList(t *testing.T) {
	var createCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/Users/New":
			createCalls++
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/Users":
			_ = json.NewEncoder(w).Encode([]UserInfo{{ID: "u-2", Name: "bob"}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	user, err := CreateUser(newTestConfig(srv.URL, "emby"), "bob")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if createCalls != 1 || user == nil || user.ID != "u-2" {
		t.Fatalf("createCalls=%d user=%+v", createCalls, user)
	}
}

func TestFindUserByName(t *testing.T) {
	user := FindUserByName([]UserInfo{{ID: "u-1", Name: "Alice"}}, "alice")
	if user == nil || user.ID != "u-1" {
		t.Fatalf("user=%+v", user)
	}
	if FindUserByName([]UserInfo{{Name: "Alice"}}, "bob") != nil {
		t.Fatal("expected nil")
	}
}

func TestGetUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Users/u-1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(UserInfo{ID: "u-1", Name: "alice", Policy: map[string]any{"EnableRemoteAccess": true}})
	}))
	defer srv.Close()

	user, err := GetUser(newTestConfig(srv.URL, "emby"), "u-1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user == nil || user.ID != "u-1" || user.Policy["EnableRemoteAccess"] != true {
		t.Fatalf("user=%+v", user)
	}
}

func TestUpdateUserPolicy(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/Users/u-1/Policy" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode policy: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	err := UpdateUserPolicy(newTestConfig(srv.URL, "jellyfin"), "u-1", map[string]any{
		"EnableLiveTvAccess": true,
		"EnableRemoteAccess": true,
	})
	if err != nil {
		t.Fatalf("UpdateUserPolicy: %v", err)
	}
	if got["EnableLiveTvAccess"] != true || got["EnableRemoteAccess"] != true {
		t.Fatalf("got=%+v", got)
	}
}

func TestMergeDesiredUserPolicy(t *testing.T) {
	yes := true
	no := false
	merged, drift, err := MergeDesiredUserPolicy(map[string]any{
		"EnableLiveTvAccess": true,
		"EnableRemoteAccess": false,
	}, DesiredUserPolicy{
		EnableLiveTvAccess:       &yes,
		EnableRemoteAccess:       &yes,
		EnableContentDownloading: &no,
	})
	if err != nil {
		t.Fatalf("MergeDesiredUserPolicy: %v", err)
	}
	if len(drift) != 2 {
		t.Fatalf("drift=%v", drift)
	}
	if merged["EnableLiveTvAccess"] != true || merged["EnableRemoteAccess"] != true || merged["EnableContentDownloading"] != false {
		t.Fatalf("merged=%+v", merged)
	}
}

func TestMergeDesiredUserPolicyRequiresCurrentPolicy(t *testing.T) {
	yes := true
	_, _, err := MergeDesiredUserPolicy(nil, DesiredUserPolicy{EnableLiveTvAccess: &yes})
	if err == nil || !strings.Contains(err.Error(), "current user policy unavailable") {
		t.Fatalf("err=%v", err)
	}
}

func TestUserActivationPending(t *testing.T) {
	pending, reason := UserActivationPending(UserInfo{ID: "u-1"})
	if !pending || !strings.Contains(reason, "no configured password") {
		t.Fatalf("pending=%t reason=%q", pending, reason)
	}
	pending, reason = UserActivationPending(UserInfo{ID: "u-1", HasConfiguredPassword: true})
	if pending || reason != "" {
		t.Fatalf("pending=%t reason=%q", pending, reason)
	}
	pending, reason = UserActivationPending(UserInfo{ID: "u-1", EnableAutoLogin: true})
	if pending || reason != "" {
		t.Fatalf("pending=%t reason=%q", pending, reason)
	}
	pending, reason = UserActivationPending(UserInfo{ID: "u-1", IsDisabled: true})
	if pending || reason != "" {
		t.Fatalf("pending=%t reason=%q", pending, reason)
	}
}

func TestCreateUserRequiresName(t *testing.T) {
	_, err := CreateUser(newTestConfig("http://emby:8096", "emby"), " ")
	if err == nil || !strings.Contains(err.Error(), "user name required") {
		t.Fatalf("err=%v", err)
	}
}

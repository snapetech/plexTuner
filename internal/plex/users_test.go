package plex

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListUsers(t *testing.T) {
	oldClient := plexTVHTTPClient
	oldBase := plexTVBaseURLForTest
	defer func() {
		plexTVHTTPClient = oldClient
		plexTVBaseURLForTest = oldBase
	}()

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<MediaContainer size="2">
  <User id="10" uuid="u-10" username="alice" title="Alice" email="alice@example.com" home="1" restricted="0" />
  <User id="11" uuid="u-11" username="" title="Kids Room" email="" home="0" restricted="1" />
</MediaContainer>`))
	}))
	defer srv.Close()

	plexTVHTTPClient = func() *http.Client { return srv.Client() }
	plexTVBaseURLForTest = srv.URL

	users, err := ListUsers("token")
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if !strings.Contains(gotPath, "/api/users") {
		t.Fatalf("path=%q", gotPath)
	}
	if len(users) != 2 {
		t.Fatalf("users=%+v", users)
	}
	if users[0].ID != 10 || users[0].Username != "alice" || !users[0].Home {
		t.Fatalf("user0=%+v", users[0])
	}
	if users[1].ID != 11 || users[1].Title != "Kids Room" || !users[1].Managed || !users[1].Restricted {
		t.Fatalf("user1=%+v", users[1])
	}
}

func TestListUsersRequiresToken(t *testing.T) {
	_, err := ListUsers("")
	if err == nil || !strings.Contains(err.Error(), "plex token required") {
		t.Fatalf("err=%v", err)
	}
}

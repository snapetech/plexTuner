package plex

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateSharedServer(t *testing.T) {
	oldClient := plexTVHTTPClient
	oldBase := plexTVBaseURLForTest
	defer func() {
		plexTVHTTPClient = oldClient
		plexTVBaseURLForTest = oldBase
	}()

	var gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<sharedServer id="1"><invited id="99" home="0" username="demo" email="demo@example.com"/><sharingSettings allowTuners="0" allowSync="1"/></sharedServer>`))
	}))
	defer srv.Close()

	plexTVHTTPClient = func() *http.Client { return srv.Client() }
	plexTVBaseURLForTest = srv.URL

	req := SharedServerRequest{
		MachineIdentifier: "machine-1",
		LibrarySectionIDs: []int{1, 2},
		InvitedID:         99,
	}
	req.Settings.AllowTuners = 1
	req.Settings.AllowSync = 1
	resp, err := CreateSharedServer("token", "client-1", req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 {
		t.Fatalf("unexpected status: %d", resp.Status)
	}
	if !strings.Contains(gotPath, "/api/v2/shared_servers") {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	var decoded SharedServerRequest
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Settings.AllowTuners != 1 || decoded.InvitedID != 99 {
		t.Fatalf("unexpected request body: %+v", decoded)
	}
	if resp.Observed.AllowTuners != 0 {
		t.Fatalf("expected parsed observed allowTuners=0, got %+v", resp.Observed)
	}
}

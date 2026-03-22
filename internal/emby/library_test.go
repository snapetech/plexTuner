package emby

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnsureLibraryReusesExisting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Library/VirtualFolders/Query" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(VirtualFolderQueryResult{
			Items: []VirtualFolderInfo{{
				Name:           "Catchup Sports",
				CollectionType: "movies",
				ItemID:         "lib-1",
				Locations:      []string{"/data/catchup/sports"},
			}},
		})
	}))
	defer srv.Close()

	lib, created, err := EnsureLibrary(newTestConfig(srv.URL, "emby"), LibraryCreateSpec{
		Name:           "Catchup Sports",
		CollectionType: "movies",
		Path:           "/data/catchup/sports",
	})
	if err != nil {
		t.Fatalf("EnsureLibrary: %v", err)
	}
	if created {
		t.Fatal("expected existing library to be reused")
	}
	if lib == nil || lib.ID != "lib-1" {
		t.Fatalf("unexpected library: %+v", lib)
	}
}

func TestEnsureLibraryCreatesMissing(t *testing.T) {
	var (
		listCalls   int
		createBody  AddVirtualFolder
		createCalls int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			listCalls++
			if listCalls == 1 {
				_ = json.NewEncoder(w).Encode(VirtualFolderQueryResult{})
				return
			}
			_ = json.NewEncoder(w).Encode(VirtualFolderQueryResult{
				Items: []VirtualFolderInfo{{
					Name:           "Catchup Movies",
					CollectionType: "movies",
					ID:             "lib-2",
					Locations:      []string{"/data/catchup/movies"},
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/Library/VirtualFolders":
			createCalls++
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	lib, created, err := EnsureLibrary(newTestConfig(srv.URL, "jellyfin"), LibraryCreateSpec{
		Name:           "Catchup Movies",
		CollectionType: "movies",
		Path:           "/data/catchup/movies",
		Refresh:        true,
	})
	if err != nil {
		t.Fatalf("EnsureLibrary: %v", err)
	}
	if !created {
		t.Fatal("expected library creation")
	}
	if createCalls != 1 {
		t.Fatalf("createCalls=%d want 1", createCalls)
	}
	if createBody.Name != "" || createBody.CollectionType != "" || createBody.RefreshLibrary || len(createBody.Paths) != 0 {
		t.Fatalf("unexpected create body: %+v", createBody)
	}
	if lib == nil || lib.ID != "lib-2" {
		t.Fatalf("unexpected created library: %+v", lib)
	}
}

func TestCreateLibraryJellyfinUsesQueryParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/Library/VirtualFolders" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("name"); got != "Catchup Test Sports" {
			t.Fatalf("name=%q", got)
		}
		if got := r.URL.Query().Get("collectionType"); got != "movies" {
			t.Fatalf("collectionType=%q", got)
		}
		if got := r.URL.Query().Get("paths"); got != "/config/catchup/sports" {
			t.Fatalf("paths=%q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body) != 0 {
			t.Fatalf("expected empty JSON body, got %+v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	err := CreateLibrary(newTestConfig(srv.URL, "jellyfin"), LibraryCreateSpec{
		Name:           "Catchup Test Sports",
		CollectionType: "movies",
		Path:           "/config/catchup/sports",
		Refresh:        true,
	})
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
}

func TestEnsureLibraryFallsBackToLegacyVirtualFoldersList(t *testing.T) {
	var createCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders":
			_ = json.NewEncoder(w).Encode([]VirtualFolderInfo{{
				Name:           "Catchup Test General",
				CollectionType: "movies",
				ID:             "legacy-1",
				Locations:      []string{"/config/catchup/general"},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/Library/VirtualFolders":
			createCalls++
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	lib, created, err := EnsureLibrary(newTestConfig(srv.URL, "jellyfin"), LibraryCreateSpec{
		Name:           "Catchup Test General",
		CollectionType: "movies",
		Path:           "/config/catchup/general",
	})
	if err != nil {
		t.Fatalf("EnsureLibrary: %v", err)
	}
	if created {
		t.Fatal("expected fallback-listed library to be reused")
	}
	if createCalls != 0 {
		t.Fatalf("unexpected createCalls=%d", createCalls)
	}
	if lib == nil || lib.ID != "legacy-1" {
		t.Fatalf("unexpected library: %+v", lib)
	}
}

func TestRefreshLibraryScan(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/Library/Refresh" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Authorization"), "testtoken") {
			t.Fatalf("missing auth header")
		}
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := RefreshLibraryScan(newTestConfig(srv.URL, "emby")); err != nil {
		t.Fatalf("RefreshLibraryScan: %v", err)
	}
	if !called {
		t.Fatal("expected refresh call")
	}
}

func TestListLibraries_trimsTrailingSlashHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Library/VirtualFolders/Query" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(VirtualFolderQueryResult{})
	}))
	defer srv.Close()

	_, err := ListLibraries(newTestConfig(srv.URL+"/", "emby"))
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
}

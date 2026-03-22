package entitlements

import (
	"path/filepath"
	"testing"
)

func TestLoadSaveAndAuthenticate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xtream-users.json")
	saved, err := SaveFile(path, Ruleset{
		Users: []User{
			{
				Username:           "viewer",
				Password:           "secret",
				AllowLive:          true,
				AllowedChannelIDs:  []string{"ch1", "ch1"},
				AllowedCategoryIDs: []string{"news", "News"},
			},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Version != Version || len(saved.Users) != 1 {
		t.Fatalf("saved=%+v", saved)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := Authenticate(loaded, "viewer", "secret"); !ok {
		t.Fatalf("authenticate failed: %+v", loaded)
	}
	if len(loaded.Users[0].AllowedChannelIDs) != 1 || len(loaded.Users[0].AllowedCategoryIDs) != 1 {
		t.Fatalf("dedupe failed: %+v", loaded.Users[0])
	}
}

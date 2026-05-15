package emby

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RegistrationState persists the IDs assigned by the media server on registration.
// Written atomically so a crash between register and save doesn't corrupt the file.
type RegistrationState struct {
	TunerHostID       string    `json:"tuner_host_id"`
	ListingProviderID string    `json:"listing_provider_id"`
	TunerURL          string    `json:"tuner_url"`
	XMLTVURL          string    `json:"xmltv_url"`
	RegisteredAt      time.Time `json:"registered_at"`
}

func loadState(file string) (*RegistrationState, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var s RegistrationState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveState(file string, s *RegistrationState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(file); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to overwrite symlinked emby registration state %q", file)
	}
	return writeRegistrationStateFile(dir, file, data)
}

func writeRegistrationStateFile(dir, file string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".emby-state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, file); err != nil {
		return err
	}
	cleanup = false
	return os.Chmod(file, 0o600)
}

package emby

import (
	"encoding/json"
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
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

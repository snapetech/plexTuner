package tuner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CatchupCapsuleLaneFile struct {
	GeneratedAt string           `json:"generated_at"`
	SourceReady bool             `json:"source_ready"`
	Lane        string           `json:"lane"`
	Capsules    []CatchupCapsule `json:"capsules"`
}

func DefaultCatchupCapsuleLanes() []string {
	return []string{"sports", "movies", "general"}
}

func SaveCatchupCapsuleLanes(outDir string, preview CatchupCapsulePreview) (map[string]string, error) {
	if strings.TrimSpace(outDir) == "" {
		return nil, fmt.Errorf("output directory required")
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", outDir, err)
	}
	_ = os.Chmod(outDir, 0o700)
	byLane := map[string][]CatchupCapsule{}
	for _, capsule := range preview.Capsules {
		lane := strings.TrimSpace(capsule.Lane)
		if lane == "" {
			lane = "general"
		}
		byLane[lane] = append(byLane[lane], capsule)
	}
	written := map[string]string{}
	for _, lane := range DefaultCatchupCapsuleLanes() {
		capsules := byLane[lane]
		if len(capsules) == 0 {
			continue
		}
		body := CatchupCapsuleLaneFile{
			GeneratedAt: preview.GeneratedAt,
			SourceReady: preview.SourceReady,
			Lane:        lane,
			Capsules:    capsules,
		}
		data, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal lane %s: %w", lane, err)
		}
		path := filepath.Join(outDir, lane+".json")
		if err := writePrivateCatchupArtifact(path, data); err != nil {
			return nil, fmt.Errorf("write lane %s: %w", lane, err)
		}
		written[lane] = path
	}
	manifestData, err := json.MarshalIndent(map[string]any{
		"generated_at":  preview.GeneratedAt,
		"source_ready":  preview.SourceReady,
		"capsule_count": len(preview.Capsules),
		"lane_order":    DefaultCatchupCapsuleLanes(),
		"written_lanes": written,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	if err := writePrivateCatchupArtifact(manifestPath, manifestData); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}
	return written, nil
}

func writePrivateCatchupArtifact(path string, body []byte) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to overwrite symlinked catchup artifact %q", path)
		}
		if info.IsDir() {
			return fmt.Errorf("refusing to overwrite directory catchup artifact %q", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".catchup-artifact-*.tmp")
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
	if _, err := tmp.Write(body); err != nil {
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
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return os.Chmod(path, 0o600)
}

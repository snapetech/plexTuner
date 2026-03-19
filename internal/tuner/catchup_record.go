package tuner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

type CatchupRecordedItem struct {
	CapsuleID  string `json:"capsule_id"`
	Lane       string `json:"lane"`
	Title      string `json:"title"`
	ChannelID  string `json:"channel_id"`
	OutputPath string `json:"output_path"`
	SourceURL  string `json:"source_url"`
	Bytes      int64  `json:"bytes"`
}

type CatchupRecordManifest struct {
	GeneratedAt string                `json:"generated_at"`
	RootDir     string                `json:"root_dir"`
	Recorded    []CatchupRecordedItem `json:"recorded"`
}

func ResolveCatchupRecordSourceURL(capsule CatchupCapsule, streamBaseURL string) (string, error) {
	sourceURL := strings.TrimSpace(capsule.ReplayURL)
	if sourceURL != "" {
		return sourceURL, nil
	}
	streamBaseURL = strings.TrimRight(strings.TrimSpace(streamBaseURL), "/")
	if streamBaseURL == "" {
		return "", fmt.Errorf("stream base url required")
	}
	return streamBaseURL + "/stream/" + capsule.ChannelID, nil
}

func RecordCatchupCapsule(ctx context.Context, capsule CatchupCapsule, streamBaseURL, outDir string, client *http.Client) (CatchupRecordedItem, error) {
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		return CatchupRecordedItem{}, fmt.Errorf("output directory required")
	}
	if client == nil {
		client = httpclient.ForStreaming()
	}
	sourceURL, err := ResolveCatchupRecordSourceURL(capsule, streamBaseURL)
	if err != nil {
		return CatchupRecordedItem{}, err
	}
	laneDir := filepath.Join(outDir, firstNonEmptyString(capsule.Lane, "general"))
	if err := os.MkdirAll(laneDir, 0o755); err != nil {
		return CatchupRecordedItem{}, err
	}
	path := filepath.Join(laneDir, sanitizeCatchupName(capsule.CapsuleID)+".ts")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return CatchupRecordedItem{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return CatchupRecordedItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CatchupRecordedItem{}, fmt.Errorf("record %s status=%d", capsule.CapsuleID, resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return CatchupRecordedItem{}, err
	}
	n, copyErr := io.Copy(f, resp.Body)
	_ = f.Close()
	if copyErr != nil && ctx.Err() == nil {
		return CatchupRecordedItem{}, copyErr
	}
	return CatchupRecordedItem{
		CapsuleID:  capsule.CapsuleID,
		Lane:       capsule.Lane,
		Title:      capsule.Title,
		ChannelID:  capsule.ChannelID,
		OutputPath: path,
		SourceURL:  sourceURL,
		Bytes:      n,
	}, nil
}

func RecordCatchupCapsules(ctx context.Context, preview CatchupCapsulePreview, streamBaseURL, outDir string, maxDuration time.Duration, client *http.Client) (CatchupRecordManifest, error) {
	streamBaseURL = strings.TrimRight(strings.TrimSpace(streamBaseURL), "/")
	outDir = strings.TrimSpace(outDir)
	if streamBaseURL == "" {
		return CatchupRecordManifest{}, fmt.Errorf("stream base url required")
	}
	if outDir == "" {
		return CatchupRecordManifest{}, fmt.Errorf("output directory required")
	}
	if client == nil {
		client = httpclient.ForStreaming()
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return CatchupRecordManifest{}, err
	}
	manifest := CatchupRecordManifest{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RootDir:     outDir,
	}
	for _, capsule := range preview.Capsules {
		if strings.ToLower(strings.TrimSpace(capsule.State)) != "in_progress" {
			continue
		}
		reqCtx := ctx
		if maxDuration > 0 {
			var cancel context.CancelFunc
			reqCtx, cancel = context.WithTimeout(ctx, maxDuration)
			defer cancel()
		}
		item, err := RecordCatchupCapsule(reqCtx, capsule, streamBaseURL, outDir, client)
		if err != nil {
			return manifest, err
		}
		manifest.Recorded = append(manifest.Recorded, item)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return manifest, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "record-manifest.json"), data, 0o600); err != nil {
		return manifest, err
	}
	return manifest, nil
}

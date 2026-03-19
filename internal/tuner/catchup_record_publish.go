package tuner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CatchupRecordedPublishManifest struct {
	GeneratedAt string                         `json:"generated_at"`
	RootDir     string                         `json:"root_dir"`
	Items       []CatchupRecordedPublishedItem `json:"items"`
}

type CatchupRecordedPublishedItem struct {
	CapsuleID string `json:"capsule_id"`
	Lane      string `json:"lane"`
	Title     string `json:"title"`
	Directory string `json:"directory"`
	MediaPath string `json:"media_path"`
	NFOPath   string `json:"nfo_path"`
	SourceTS  string `json:"source_ts"`
}

func PublishRecordedCatchupItem(rootDir string, capsule CatchupCapsule, recorded CatchupRecordedItem) (CatchupRecordedPublishedItem, error) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return CatchupRecordedPublishedItem{}, fmt.Errorf("publish root required")
	}
	if strings.TrimSpace(recorded.OutputPath) == "" {
		return CatchupRecordedPublishedItem{}, fmt.Errorf("recorded output path required")
	}
	start, _ := time.Parse(time.RFC3339, capsule.Start)
	lane := firstNonEmptyString(capsule.Lane, "general")
	dirName := catchupPublishDirName(capsule, start)
	itemDir := filepath.Join(rootDir, lane, dirName)
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		return CatchupRecordedPublishedItem{}, err
	}
	baseName := dirName
	mediaPath := filepath.Join(itemDir, baseName+".ts")
	if err := linkOrCopyFile(recorded.OutputPath, mediaPath); err != nil {
		return CatchupRecordedPublishedItem{}, fmt.Errorf("publish media: %w", err)
	}
	nfoPath := filepath.Join(itemDir, baseName+".nfo")
	if err := os.WriteFile(nfoPath, BuildCatchupMovieNFO(capsule), 0o600); err != nil {
		return CatchupRecordedPublishedItem{}, fmt.Errorf("write nfo: %w", err)
	}
	return CatchupRecordedPublishedItem{
		CapsuleID: capsule.CapsuleID,
		Lane:      lane,
		Title:     capsule.Title,
		Directory: itemDir,
		MediaPath: mediaPath,
		NFOPath:   nfoPath,
		SourceTS:  recorded.OutputPath,
	}, nil
}

func SaveRecordedCatchupPublishManifest(rootDir string, items []CatchupRecordedPublishedItem) error {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return fmt.Errorf("publish root required")
	}
	body := CatchupRecordedPublishManifest{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RootDir:     rootDir,
		Items:       items,
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(rootDir, "recorded-publish-manifest.json"), data, 0o600)
}

func BuildRecordedCatchupPublishManifest(rootDir, libraryPrefix string, items []CatchupRecordedPublishedItem) CatchupPublishManifest {
	rootDir = strings.TrimSpace(rootDir)
	now := time.Now().UTC().Format(time.RFC3339)
	manifest := CatchupPublishManifest{
		GeneratedAt: now,
		SourceReady: true,
		ReplayMode:  "recorded",
		RootDir:     rootDir,
		Libraries:   []CatchupPublishedLibrary{},
		Items:       []CatchupPublishedItem{},
	}
	lanes := map[string]int{}
	for _, item := range items {
		lane := firstNonEmptyString(strings.TrimSpace(item.Lane), "general")
		if _, ok := lanes[lane]; !ok {
			manifest.Libraries = append(manifest.Libraries, CatchupPublishedLibrary{
				Lane:           lane,
				Name:           CatchupLibraryName(libraryPrefix, lane),
				CollectionType: "movies",
				Path:           filepath.Join(rootDir, lane),
			})
		}
		lanes[lane]++
		manifest.Items = append(manifest.Items, CatchupPublishedItem{
			CapsuleID:  item.CapsuleID,
			Lane:       lane,
			Title:      item.Title,
			State:      "completed",
			Directory:  item.Directory,
			StreamPath: item.MediaPath,
			NFOPath:    item.NFOPath,
		})
	}
	sort.SliceStable(manifest.Libraries, func(i, j int) bool {
		return manifest.Libraries[i].Lane < manifest.Libraries[j].Lane
	})
	for i := range manifest.Libraries {
		manifest.Libraries[i].ItemCount = lanes[manifest.Libraries[i].Lane]
	}
	sort.SliceStable(manifest.Items, func(i, j int) bool {
		if manifest.Items[i].Lane == manifest.Items[j].Lane {
			return manifest.Items[i].Title < manifest.Items[j].Title
		}
		return manifest.Items[i].Lane < manifest.Items[j].Lane
	})
	return manifest
}

func linkOrCopyFile(src, dst string) error {
	if src == dst {
		return nil
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

package tuner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

type CatchupPublishedItem struct {
	CapsuleID   string `json:"capsule_id"`
	Lane        string `json:"lane"`
	Title       string `json:"title"`
	ChannelName string `json:"channel_name"`
	State       string `json:"state"`
	ReplayMode  string `json:"replay_mode"`
	Start       string `json:"start"`
	Stop        string `json:"stop"`
	Directory   string `json:"directory"`
	StreamPath  string `json:"stream_path"`
	NFOPath     string `json:"nfo_path"`
	StreamURL   string `json:"stream_url"`
}

type CatchupPublishedLibrary struct {
	Lane           string `json:"lane"`
	Name           string `json:"name"`
	CollectionType string `json:"collection_type"`
	Path           string `json:"path"`
	ItemCount      int    `json:"item_count"`
}

type CatchupPublishManifest struct {
	GeneratedAt   string                    `json:"generated_at"`
	SourceReady   bool                      `json:"source_ready"`
	ReplayMode    string                    `json:"replay_mode"`
	RootDir       string                    `json:"root_dir"`
	StreamBaseURL string                    `json:"stream_base_url"`
	Libraries     []CatchupPublishedLibrary `json:"libraries"`
	Items         []CatchupPublishedItem    `json:"items"`
}

func CatchupLibraryName(libraryPrefix, lane string) string {
	libraryPrefix = strings.TrimSpace(libraryPrefix)
	if libraryPrefix == "" {
		libraryPrefix = "Catchup"
	}
	return libraryPrefix + " " + catchupLibraryTitle(strings.TrimSpace(lane))
}

func SaveCatchupCapsuleLibraryLayout(outDir, streamBaseURL, libraryPrefix string, preview CatchupCapsulePreview) (CatchupPublishManifest, error) {
	outDir = strings.TrimSpace(outDir)
	streamBaseURL = strings.TrimRight(strings.TrimSpace(streamBaseURL), "/")
	libraryPrefix = strings.TrimSpace(libraryPrefix)
	if outDir == "" {
		return CatchupPublishManifest{}, fmt.Errorf("output directory required")
	}
	if streamBaseURL == "" {
		return CatchupPublishManifest{}, fmt.Errorf("stream base url required")
	}
	if libraryPrefix == "" {
		libraryPrefix = "Catchup"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return CatchupPublishManifest{}, fmt.Errorf("mkdir %s: %w", outDir, err)
	}
	manifest := CatchupPublishManifest{
		GeneratedAt:   preview.GeneratedAt,
		SourceReady:   preview.SourceReady,
		ReplayMode:    firstNonEmptyString(preview.ReplayMode, "launcher"),
		RootDir:       outDir,
		StreamBaseURL: streamBaseURL,
	}
	laneCounts := map[string]int{}
	for _, lane := range DefaultCatchupCapsuleLanes() {
		laneDir := filepath.Join(outDir, lane)
		if err := os.MkdirAll(laneDir, 0o755); err != nil {
			return CatchupPublishManifest{}, fmt.Errorf("mkdir lane %s: %w", laneDir, err)
		}
		manifest.Libraries = append(manifest.Libraries, CatchupPublishedLibrary{
			Lane:           lane,
			Name:           CatchupLibraryName(libraryPrefix, lane),
			CollectionType: "movies",
			Path:           laneDir,
		})
	}
	for _, capsule := range preview.Capsules {
		lane := strings.TrimSpace(capsule.Lane)
		if lane == "" {
			lane = "general"
		}
		start, err := time.Parse(time.RFC3339, capsule.Start)
		if err != nil {
			return CatchupPublishManifest{}, fmt.Errorf("parse capsule start %s: %w", capsule.CapsuleID, err)
		}
		dirName := catchupPublishDirName(capsule, start)
		itemDir := filepath.Join(outDir, lane, dirName)
		if err := os.MkdirAll(itemDir, 0o755); err != nil {
			return CatchupPublishManifest{}, fmt.Errorf("mkdir item %s: %w", itemDir, err)
		}
		baseName := dirName
		streamPath := filepath.Join(itemDir, baseName+".strm")
		streamURL := streamBaseURL + "/stream/" + capsule.ChannelID
		if strings.TrimSpace(capsule.ReplayURL) != "" {
			streamURL = strings.TrimSpace(capsule.ReplayURL)
		}
		if err := os.WriteFile(streamPath, []byte(streamURL+"\n"), 0o600); err != nil {
			return CatchupPublishManifest{}, fmt.Errorf("write strm %s: %w", streamPath, err)
		}
		nfoPath := filepath.Join(itemDir, baseName+".nfo")
		if err := os.WriteFile(nfoPath, buildCatchupMovieNFO(capsule), 0o600); err != nil {
			return CatchupPublishManifest{}, fmt.Errorf("write nfo %s: %w", nfoPath, err)
		}
		manifest.Items = append(manifest.Items, CatchupPublishedItem{
			CapsuleID:   capsule.CapsuleID,
			Lane:        lane,
			Title:       capsule.Title,
			ChannelName: capsule.ChannelName,
			State:       capsule.State,
			ReplayMode:  firstNonEmptyString(capsule.ReplayMode, "launcher"),
			Start:       capsule.Start,
			Stop:        capsule.Stop,
			Directory:   itemDir,
			StreamPath:  streamPath,
			NFOPath:     nfoPath,
			StreamURL:   streamURL,
		})
		laneCounts[lane]++
	}
	sort.SliceStable(manifest.Items, func(i, j int) bool {
		if manifest.Items[i].Lane == manifest.Items[j].Lane {
			return manifest.Items[i].Title < manifest.Items[j].Title
		}
		return manifest.Items[i].Lane < manifest.Items[j].Lane
	})
	for i := range manifest.Libraries {
		manifest.Libraries[i].ItemCount = laneCounts[manifest.Libraries[i].Lane]
	}
	manifestPath := filepath.Join(outDir, "publish-manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return CatchupPublishManifest{}, fmt.Errorf("marshal publish manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		return CatchupPublishManifest{}, fmt.Errorf("write publish manifest: %w", err)
	}
	return manifest, nil
}

func catchupPublishDirName(c CatchupCapsule, start time.Time) string {
	name := firstNonEmptyString(c.Title, c.ChannelName, c.CapsuleID)
	if c.SubTitle != "" {
		name += " " + c.SubTitle
	}
	name += " " + start.UTC().Format("2006-01-02 15-04 UTC")
	return sanitizeCatchupName(name)
}

func catchupLibraryTitle(lane string) string {
	if lane == "" {
		return "General"
	}
	return strings.ToUpper(lane[:1]) + lane[1:]
}

func sanitizeCatchupName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "capsule"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "capsule"
	}
	return out
}

func buildCatchupMovieNFO(c CatchupCapsule) []byte {
	type tagValue struct {
		XMLName xml.Name
		Value   string `xml:",chardata"`
	}
	type movieNFO struct {
		XMLName   xml.Name   `xml:"movie"`
		Title     string     `xml:"title"`
		SortTitle string     `xml:"sorttitle,omitempty"`
		Outline   string     `xml:"outline,omitempty"`
		Plot      string     `xml:"plot,omitempty"`
		Premiered string     `xml:"premiered,omitempty"`
		Aired     string     `xml:"aired,omitempty"`
		Year      int        `xml:"year,omitempty"`
		Runtime   int        `xml:"runtime,omitempty"`
		UniqueID  string     `xml:"uniqueid,omitempty"`
		Studio    string     `xml:"studio,omitempty"`
		Genres    []tagValue `xml:"genre,omitempty"`
		Tags      []tagValue `xml:"tag,omitempty"`
	}
	start, _ := time.Parse(time.RFC3339, c.Start)
	title := strings.TrimSpace(c.Title)
	if title == "" {
		title = strings.TrimSpace(c.ChannelName)
	}
	plot := strings.TrimSpace(c.Desc)
	if plot == "" {
		plot = strings.TrimSpace(c.ChannelName)
	}
	genres := make([]tagValue, 0, len(c.Categories))
	for _, cat := range c.Categories {
		cat = strings.TrimSpace(cat)
		if cat == "" {
			continue
		}
		genres = append(genres, tagValue{XMLName: xml.Name{Local: "genre"}, Value: cat})
	}
	tags := []tagValue{
		{XMLName: xml.Name{Local: "tag"}, Value: "iptvTunerr catchup"},
		{XMLName: xml.Name{Local: "tag"}, Value: c.Lane},
	}
	if c.ChannelName != "" {
		tags = append(tags, tagValue{XMLName: xml.Name{Local: "tag"}, Value: c.ChannelName})
	}
	doc := movieNFO{
		Title:     title,
		SortTitle: title,
		Outline:   firstNonEmptyString(c.SubTitle, c.ChannelName),
		Plot:      plot,
		Premiered: start.Format("2006-01-02"),
		Aired:     start.Format("2006-01-02"),
		Year:      start.Year(),
		Runtime:   c.DurationMins,
		UniqueID:  c.CapsuleID,
		Studio:    c.ChannelName,
		Genres:    genres,
		Tags:      tags,
	}
	data, _ := xml.MarshalIndent(doc, "", "  ")
	return append([]byte(xml.Header), append(data, '\n')...)
}

func BuildCatchupMovieNFO(c CatchupCapsule) []byte {
	return buildCatchupMovieNFO(c)
}

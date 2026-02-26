//go:build linux
// +build linux

package vodfs

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// MovieDirName returns the Plex movie folder name: "MovieName (Year)".
func MovieDirName(title string, year int) string {
	title = safeFSName(title)
	if year > 0 {
		return fmt.Sprintf("%s (%d)", title, year)
	}
	return title
}

// MovieFileName returns the Plex movie file name: "MovieName (Year).mp4".
func MovieFileName(title string, year int) string {
	return MovieDirName(title, year) + ".mp4"
}

// MovieFileNameForStream returns a Plex movie file name using a source-informed extension when possible.
func MovieFileNameForStream(title string, year int, streamURL string) string {
	return MovieDirName(title, year) + VODFileExt(streamURL)
}

// ShowDirName returns the Plex TV show folder name: "Show Name (Year)".
func ShowDirName(title string, year int) string {
	title = safeFSName(title)
	if year > 0 {
		return fmt.Sprintf("%s (%d)", title, year)
	}
	return title
}

// SeasonDirName returns the Plex season folder name: "Season 01".
func SeasonDirName(seasonNum int) string {
	return fmt.Sprintf("Season %02d", seasonNum)
}

// EpisodeFileName returns the Plex episode file name: "Show Name (Year) - s01e01 - Episode Title.mp4".
func EpisodeFileName(showTitle string, showYear int, seasonNum, episodeNum int, episodeTitle string) string {
	show := ShowDirName(showTitle, showYear)
	ep := fmt.Sprintf("s%02de%02d", seasonNum, episodeNum)
	episodeTitle = safeFSName(episodeTitle)
	if episodeTitle != "" {
		return fmt.Sprintf("%s - %s - %s.mp4", show, ep, episodeTitle)
	}
	return fmt.Sprintf("%s - %s.mp4", show, ep)
}

// EpisodeFileNameForStream returns a Plex episode file name using a source-informed extension when possible.
func EpisodeFileNameForStream(showTitle string, showYear int, seasonNum, episodeNum int, episodeTitle, streamURL string) string {
	show := ShowDirName(showTitle, showYear)
	ep := fmt.Sprintf("s%02de%02d", seasonNum, episodeNum)
	episodeTitle = safeFSName(episodeTitle)
	ext := VODFileExt(streamURL)
	if episodeTitle != "" {
		return fmt.Sprintf("%s - %s - %s%s", show, ep, episodeTitle, ext)
	}
	return fmt.Sprintf("%s - %s%s", show, ep, ext)
}

// VODFileExt returns the best-effort media extension to expose in VODFS based on source URL.
// We preserve common direct-file extensions (e.g. .mkv) so Plex doesn't see mismatched bytes vs filename.
// HLS/unknown sources default to .mp4 because the materializer remux path writes MP4.
func VODFileExt(streamURL string) string {
	if streamURL == "" {
		return ".mp4"
	}
	u, err := url.Parse(streamURL)
	if err != nil {
		return ".mp4"
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	switch ext {
	case ".mp4", ".m4v", ".mkv", ".webm", ".mov", ".avi", ".ts":
		return ext
	case ".m3u8":
		return ".mp4"
	default:
		return ".mp4"
	}
}

// SafeBase returns a filesystem-safe base name (no path separators or nulls).
func SafeBase(name string) string {
	return safeFSName(filepath.Base(name))
}

func safeFSName(name string) string {
	if name == "" {
		return ""
	}
	// FUSE directory entries cannot contain path separators or NUL bytes.
	name = strings.ReplaceAll(name, "\x00", "")
	name = strings.ReplaceAll(name, "/", " - ")
	name = strings.TrimSpace(name)
	if name == "" {
		return "_"
	}
	return name
}

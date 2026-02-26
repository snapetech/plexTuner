//go:build linux
// +build linux

package vodfs

import (
	"fmt"
	"path/filepath"
)

// MovieDirName returns the Plex movie folder name: "MovieName (Year)".
func MovieDirName(title string, year int) string {
	if year > 0 {
		return fmt.Sprintf("%s (%d)", title, year)
	}
	return title
}

// MovieFileName returns the Plex movie file name: "MovieName (Year).mp4".
func MovieFileName(title string, year int) string {
	return MovieDirName(title, year) + ".mp4"
}

// ShowDirName returns the Plex TV show folder name: "Show Name (Year)".
func ShowDirName(title string, year int) string {
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
	if episodeTitle != "" {
		return fmt.Sprintf("%s - %s - %s.mp4", show, ep, episodeTitle)
	}
	return fmt.Sprintf("%s - %s.mp4", show, ep)
}

// SafeBase returns a filesystem-safe base name (no path separators or nulls).
func SafeBase(name string) string {
	return filepath.Base(name)
}

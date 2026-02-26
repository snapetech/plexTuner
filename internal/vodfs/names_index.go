//go:build linux
// +build linux

package vodfs

import (
	"fmt"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func buildUniqueMovieDirNames(movies []catalog.Movie) map[string]string {
	baseCounts := make(map[string]int, len(movies))
	for _, m := range movies {
		baseCounts[MovieDirName(m.Title, m.Year)]++
	}
	out := make(map[string]string, len(movies))
	for _, m := range movies {
		base := MovieDirName(m.Title, m.Year)
		if baseCounts[base] <= 1 {
			out[m.ID] = base
			continue
		}
		out[m.ID] = fmt.Sprintf("%s [%s]", base, m.ID)
	}
	return out
}

func buildUniqueSeriesDirNames(series []catalog.Series) map[string]string {
	baseCounts := make(map[string]int, len(series))
	for _, s := range series {
		baseCounts[ShowDirName(s.Title, s.Year)]++
	}
	out := make(map[string]string, len(series))
	for _, s := range series {
		base := ShowDirName(s.Title, s.Year)
		if baseCounts[base] <= 1 {
			out[s.ID] = base
			continue
		}
		out[s.ID] = fmt.Sprintf("%s [%s]", base, s.ID)
	}
	return out
}

package vodfs

import (
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

const entryAttrTimeout = 60 * time.Second

// Root is the FUSE root node holding movies and series and lookup maps.
type Root struct {
	Movies  []*catalog.Movie
	Series  []*catalog.Series
	Live    []*catalog.LiveChannel
	// Prebuilt for O(1) lookup by directory name
	MovieByDirName  map[string]*catalog.Movie
	SeriesByDirName map[string]*catalog.Series
}

// NewRoot builds a Root from catalog slices and populates MovieByDirName and SeriesByDirName for O(1) lookup.
func NewRoot(movies []catalog.Movie, series []catalog.Series, live []catalog.LiveChannel) *Root {
	r := &Root{
		Movies:          make([]*catalog.Movie, len(movies)),
		Series:          make([]*catalog.Series, len(series)),
		Live:            make([]*catalog.LiveChannel, len(live)),
		MovieByDirName:  make(map[string]*catalog.Movie),
		SeriesByDirName: make(map[string]*catalog.Series),
	}
	for i := range movies {
		m := &movies[i]
		r.Movies[i] = m
		r.MovieByDirName[MovieDirName(m)] = m
	}
	for i := range series {
		s := &series[i]
		r.Series[i] = s
		r.SeriesByDirName[ShowDirName(s)] = s
	}
	for i := range live {
		r.Live[i] = &live[i]
	}
	return r
}

// MovieDirName returns a stable directory name for a movie (for FUSE lookup).
func MovieDirName(m *catalog.Movie) string {
	if m == nil {
		return ""
	}
	return m.ID
}

// ShowDirName returns a stable directory name for a series (for FUSE lookup).
func ShowDirName(s *catalog.Series) string {
	if s == nil {
		return ""
	}
	return s.ID
}

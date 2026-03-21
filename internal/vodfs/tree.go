package vodfs

import "github.com/snapetech/iptvtunerr/internal/catalog"

// Tree captures the synthetic VOD directory layout independently of any
// platform-specific filesystem backend.
type Tree struct {
	Movies          []catalog.Movie
	Series          []catalog.Series
	movieDirNames   map[string]string
	seriesDirNames  map[string]string
	movieByDirName  map[string]int
	seriesByDirName map[string]int
}

func NewTree(movies []catalog.Movie, series []catalog.Series) *Tree {
	t := &Tree{
		Movies: movies,
		Series: series,
	}
	t.movieDirNames = buildUniqueMovieDirNames(t.Movies)
	t.seriesDirNames = buildUniqueSeriesDirNames(t.Series)
	t.movieByDirName = make(map[string]int, len(t.Movies))
	for i := range t.Movies {
		name := t.MovieDirName(&t.Movies[i])
		if name != "" {
			t.movieByDirName[name] = i
		}
	}
	t.seriesByDirName = make(map[string]int, len(t.Series))
	for i := range t.Series {
		name := t.ShowDirName(&t.Series[i])
		if name != "" {
			t.seriesByDirName[name] = i
		}
	}
	return t
}

func (t *Tree) MovieDirName(m *catalog.Movie) string {
	if t == nil || m == nil {
		return ""
	}
	if n, ok := t.movieDirNames[m.ID]; ok && n != "" {
		return n
	}
	return MovieDirName(m.Title, m.Year)
}

func (t *Tree) ShowDirName(s *catalog.Series) string {
	if t == nil || s == nil {
		return ""
	}
	if n, ok := t.seriesDirNames[s.ID]; ok && n != "" {
		return n
	}
	return ShowDirName(s.Title, s.Year)
}

func (t *Tree) LookupMovieDir(name string) (*catalog.Movie, bool) {
	if t == nil {
		return nil, false
	}
	idx, ok := t.movieByDirName[name]
	if !ok || idx < 0 || idx >= len(t.Movies) {
		return nil, false
	}
	return &t.Movies[idx], true
}

func (t *Tree) LookupShowDir(name string) (*catalog.Series, bool) {
	if t == nil {
		return nil, false
	}
	idx, ok := t.seriesByDirName[name]
	if !ok || idx < 0 || idx >= len(t.Series) {
		return nil, false
	}
	return &t.Series[idx], true
}

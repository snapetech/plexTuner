package vodfs

import (
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestNewTreeLookupMovieDir(t *testing.T) {
	movies := []catalog.Movie{
		{ID: "m1", Title: "Movie", Year: 2024},
		{ID: "m2", Title: "Movie", Year: 2024},
	}
	tree := NewTree(movies, nil)
	if tree == nil {
		t.Fatal("expected tree")
	}
	if _, ok := tree.LookupMovieDir("Live: Movie (2024) [m1]"); !ok {
		t.Fatalf("expected uniquified movie directory lookup to work")
	}
	if _, ok := tree.LookupMovieDir("Live: Missing (2024)"); ok {
		t.Fatalf("unexpected lookup hit for missing movie dir")
	}
}

func TestNewTreeLookupShowDir(t *testing.T) {
	series := []catalog.Series{
		{ID: "s1", Title: "Show", Year: 2020},
	}
	tree := NewTree(nil, series)
	if _, ok := tree.LookupShowDir("Live: Show (2020)"); !ok {
		t.Fatalf("expected show lookup to work")
	}
}

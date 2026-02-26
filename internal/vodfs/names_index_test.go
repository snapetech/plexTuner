package vodfs

import (
	"testing"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestBuildUniqueMovieDirNames_DedupesCollisions(t *testing.T) {
	movies := []catalog.Movie{
		{ID: "a1", Title: "Same", Year: 2024},
		{ID: "a2", Title: "Same", Year: 2024},
		{ID: "b1", Title: "Different", Year: 2024},
	}
	got := buildUniqueMovieDirNames(movies)
	if got["b1"] != "Different (2024)" {
		t.Fatalf("non-colliding movie name changed: %q", got["b1"])
	}
	if got["a1"] == got["a2"] {
		t.Fatalf("colliding movie names not uniquified: %q", got["a1"])
	}
	if got["a1"] != "Same (2024) [a1]" && got["a2"] != "Same (2024) [a2]" {
		t.Fatalf("unexpected collision naming: %+v", got)
	}
}

func TestBuildUniqueSeriesDirNames_DedupesCollisions(t *testing.T) {
	series := []catalog.Series{
		{ID: "s1", Title: "Show", Year: 2020},
		{ID: "s2", Title: "Show", Year: 2020},
		{ID: "s3", Title: "Show", Year: 2021},
	}
	got := buildUniqueSeriesDirNames(series)
	if got["s3"] != "Show (2021)" {
		t.Fatalf("non-colliding series name changed: %q", got["s3"])
	}
	if got["s1"] == got["s2"] {
		t.Fatalf("colliding series names not uniquified: %q", got["s1"])
	}
}

package catalog

import "testing"

func TestSplitVODIntoLanes(t *testing.T) {
	movies := []Movie{
		{ID: "m1", Title: "UK-BBC - Concert Night", Category: "music", Region: "uk"},
		{ID: "m2", Title: "TOP - The Best You Can (2025)", Category: "movies", Region: "us"},
		{ID: "m3", Title: "AR-OSN - Arabic News Special", Category: "news", Region: "mena"},
	}
	series := []Series{
		{ID: "s1", Title: "4K-ESPN - NHL Tonight", Category: "sports", Region: "us"},
		{ID: "s2", Title: "4K-NF - Cobra Kai (US)", Category: "tv", Region: "us"},
		{ID: "s3", Title: "4K-NF - Dark (DE)", Category: "tv", Region: "europe"},
	}

	lanes := SplitVODIntoLanes(movies, series)
	counts := map[string]struct{ m, s int }{}
	for _, lane := range lanes {
		counts[lane.Name] = struct{ m, s int }{len(lane.Movies), len(lane.Series)}
	}

	if got := counts["music"]; got.m != 1 {
		t.Fatalf("music lane movies=%d want 1", got.m)
	}
	if got := counts["news"]; got.m != 1 {
		t.Fatalf("news lane movies=%d want 1", got.m)
	}
	if got := counts["movies"]; got.m != 1 {
		t.Fatalf("movies lane movies=%d want 1", got.m)
	}
	if got := counts["sports"]; got.s != 1 {
		t.Fatalf("sports lane series=%d want 1", got.s)
	}
	if got := counts["bcastUS"]; got.s != 1 {
		t.Fatalf("bcastUS lane series=%d want 1", got.s)
	}
	if got := counts["euroUK"]; got.s != 1 {
		t.Fatalf("euroUK lane series=%d want 1", got.s)
	}
}

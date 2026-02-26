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
		{ID: "s2", Title: "4K-NF - Cobra Kai (US)", Category: "tv", Region: "us", Language: "en", SourceTag: "4K-NF", ProviderCategoryName: "ENGLISH SERIES"},
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
	if got := counts["euroUKTV"]; got.s != 1 {
		t.Fatalf("euroUKTV lane series=%d want 1", got.s)
	}
}

func TestSplitVODIntoLanes_BcastUSStricterForDubbedUSCATitles(t *testing.T) {
	series := []Series{
		{ID: "s1", Title: "IR - North of North (2025) (CA)", Category: "tv", Region: "ca", Language: "en", SourceTag: "IR", ProviderCategoryName: "PERSIAN انگلیسی"},
		{ID: "s2", Title: "EN - Reacher (2022) (US)", Category: "tv", Region: "us", Language: "en", SourceTag: "EN", ProviderCategoryName: "ENGLISH SERIES"},
	}
	lanes := SplitVODIntoLanes(nil, series)
	counts := map[string]int{}
	for _, lane := range lanes {
		counts[lane.Name] = len(lane.Series)
	}
	if counts["bcastUS"] != 1 {
		t.Fatalf("bcastUS series=%d want 1", counts["bcastUS"])
	}
	if counts["tv"] != 1 {
		t.Fatalf("tv series=%d want 1 (dubbed CA title should fall back to tv)", counts["tv"])
	}
}

package catalog

import "testing"

func TestApplyVODTaxonomyEnrichesAndSortsDeterministically(t *testing.T) {
	movies := []Movie{
		{ID: "2", Title: "4K-NF - The Movie (US) (2024)", Year: 2024},
		{ID: "1", Title: "AR-OSN - Arabic News Special", Year: 2024},
		{ID: "3", Title: "UK-BBC - Concert Night", Year: 2023, ProviderCategoryName: "Music Concerts UK"},
	}
	series := []Series{
		{ID: "s2", Title: "4K-ESPN - NHL Tonight"},
		{ID: "s1", Title: "4K-NF - Kids Mystery Show", ProviderCategoryName: "Kids TV Shows"},
	}

	gotMovies, gotSeries := ApplyVODTaxonomy(movies, series)
	byMovieID := map[string]Movie{}
	for _, m := range gotMovies {
		byMovieID[m.ID] = m
	}
	bySeriesID := map[string]Series{}
	for _, s := range gotSeries {
		bySeriesID[s.ID] = s
	}

	if byMovieID["2"].Category != "movies" || byMovieID["2"].Region == "" {
		t.Fatalf("movie enrichment missing: %+v", byMovieID["2"])
	}
	if byMovieID["3"].Category != "music" || byMovieID["3"].Region != "uk" {
		t.Fatalf("expected provider-category-driven UK concert to classify as music/uk, got %+v", byMovieID["3"])
	}
	if byMovieID["1"].Category != "movies" || byMovieID["1"].Language != "en" {
		t.Fatalf("expected generic Arabic title text to stay in movies without provider category, got %+v", byMovieID["1"])
	}
	if byMovieID["1"].Region != "mena" {
		t.Fatalf("expected mena region, got %+v", byMovieID["1"])
	}

	if bySeriesID["s1"].Category != "kids" {
		t.Fatalf("expected kids series classification, got %+v", bySeriesID["s1"])
	}
	if bySeriesID["s2"].Category != "sports" {
		t.Fatalf("expected sports series classification, got %+v", bySeriesID["s2"])
	}
}

func TestApplyVODTaxonomyPrefersProviderCategorySignals(t *testing.T) {
	movies, series := ApplyVODTaxonomy(
		[]Movie{
			{ID: "m1", Title: "EN - News of the World (2020)", ProviderCategoryName: "Drama Movies"},
			{ID: "m2", Title: "EN - The Sound of Music (1965)", ProviderCategoryName: "Classic Movies"},
		},
		[]Series{
			{ID: "s1", Title: "EN - The Newsroom (2012) (US)", ProviderCategoryName: "US TV Shows"},
			{ID: "s2", Title: "EN - Hockey Night Recap", ProviderCategoryName: "Sports Replays"},
		},
	)
	if movies[0].Category != "movies" {
		t.Fatalf("provider category should keep News of the World in movies: %+v", movies[0])
	}
	if movies[1].Category != "movies" {
		t.Fatalf("provider category should keep Sound of Music in movies: %+v", movies[1])
	}
	// Sorted deterministically: sports category before tv.
	if series[0].Category != "sports" || series[1].Category != "tv" {
		t.Fatalf("expected provider categories to classify sports+tv, got %+v %+v", series[0], series[1])
	}
}

func TestApplyVODTaxonomyDetectsArabicScriptLanguage(t *testing.T) {
	movies, _ := ApplyVODTaxonomy([]Movie{{ID: "m1", Title: "مسلسل عربي"}}, nil)
	if movies[0].Language != "ar" {
		t.Fatalf("expected ar language, got %q", movies[0].Language)
	}
}

func TestApplyVODTaxonomyAvoidsCommonFalsePositives(t *testing.T) {
	movies, series := ApplyVODTaxonomy(
		[]Movie{
			{ID: "m1", Title: "EN - News of the World (2020)"},
			{ID: "m2", Title: "EN - The Sound of Music (1965)"},
			{ID: "m3", Title: "4K-EN - Star Wars: Episode I - The Phantom Menace (1999)"},
			{ID: "m4", Title: "4K-AR - Nickel Boys (2024)"},
		},
		[]Series{{ID: "s1", Title: "EN - The Newsroom (2012) (US)"}},
	)

	got := map[string]Movie{}
	for _, m := range movies {
		got[m.ID] = m
	}
	if got["m1"].Category == "news" {
		t.Fatalf("News of the World should not be classified as news: %+v", got["m1"])
	}
	if got["m2"].Category == "music" {
		t.Fatalf("The Sound of Music should not be classified as music lane by title substring: %+v", got["m2"])
	}
	if got["m3"].Region == "mena" {
		t.Fatalf("Phantom Menace should not be classified as mena by substring: %+v", got["m3"])
	}
	if got["m4"].Category == "kids" {
		t.Fatalf("Nickel Boys should not be classified as kids by substring: %+v", got["m4"])
	}
	if len(series) != 1 || series[0].Category == "news" {
		t.Fatalf("The Newsroom should not be forced into news lane by generic title word: %+v", series)
	}
}

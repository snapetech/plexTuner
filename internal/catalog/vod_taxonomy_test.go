package catalog

import "testing"

func TestApplyVODTaxonomyEnrichesAndSortsDeterministically(t *testing.T) {
	movies := []Movie{
		{ID: "2", Title: "4K-NF - The Movie (US) (2024)", Year: 2024},
		{ID: "1", Title: "AR-OSN - Arabic News Special", Year: 2024},
		{ID: "3", Title: "UK-BBC - Concert Night", Year: 2023},
	}
	series := []Series{
		{ID: "s2", Title: "4K-ESPN - NHL Tonight"},
		{ID: "s1", Title: "4K-NF - Kids Mystery Show"},
	}

	gotMovies, gotSeries := ApplyVODTaxonomy(movies, series)

	if gotMovies[0].Category != "movies" || gotMovies[0].Region == "" {
		t.Fatalf("movie enrichment missing: %+v", gotMovies[0])
	}
	if gotMovies[1].Category != "music" || gotMovies[1].Region != "uk" {
		t.Fatalf("expected UK concert to classify as music/uk, got %+v", gotMovies[1])
	}
	if gotMovies[2].Category != "news" || gotMovies[2].Language != "en" {
		t.Fatalf("expected Arabic-news-tagged title to classify as news, got %+v", gotMovies[2])
	}
	if gotMovies[2].Region != "mena" {
		t.Fatalf("expected mena region, got %+v", gotMovies[2])
	}

	if gotSeries[0].Category != "kids" {
		t.Fatalf("expected kids series first after sorting, got %+v", gotSeries[0])
	}
	if gotSeries[1].Category != "sports" {
		t.Fatalf("expected sports series classification, got %+v", gotSeries[1])
	}
}

func TestApplyVODTaxonomyDetectsArabicScriptLanguage(t *testing.T) {
	movies, _ := ApplyVODTaxonomy([]Movie{{ID: "m1", Title: "مسلسل عربي"}}, nil)
	if movies[0].Language != "ar" {
		t.Fatalf("expected ar language, got %q", movies[0].Language)
	}
}

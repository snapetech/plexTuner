package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type VODLaneCatalog struct {
	Name   string
	Movies []Movie
	Series []Series
}

// DefaultVODLanes returns the built-in catch-up/category split lane order.
func DefaultVODLanes() []string {
	return []string{
		"bcastUS",
		"sports",
		"news",
		"kids",
		"music",
		"euroUKMovies",
		"euroUKTV",
		"menaMovies",
		"menaTV",
		// legacy aggregate names kept in order list for backward compatibility if
		// custom/older splitters emit them.
		"euroUK",
		"mena",
		"movies",
		"tv",
		"intl",
	}
}

// SplitVODIntoLanes classifies movies/series into category lanes. It assumes the
// input has already had ApplyVODTaxonomy run, but will still function if fields
// are empty (falls back to title heuristics).
func SplitVODIntoLanes(movies []Movie, series []Series) []VODLaneCatalog {
	laneMap := map[string]*VODLaneCatalog{}
	ensure := func(name string) *VODLaneCatalog {
		if c, ok := laneMap[name]; ok {
			return c
		}
		c := &VODLaneCatalog{Name: name}
		laneMap[name] = c
		return c
	}

	for _, m := range movies {
		lane := laneForMovie(m)
		ensure(lane).Movies = append(ensure(lane).Movies, m)
	}
	for _, s := range series {
		lane := laneForSeries(s)
		ensure(lane).Series = append(ensure(lane).Series, s)
	}

	out := make([]VODLaneCatalog, 0, len(laneMap))
	for _, name := range DefaultVODLanes() {
		if c, ok := laneMap[name]; ok {
			out = append(out, *c)
			delete(laneMap, name)
		}
	}
	// Append any future/unknown lanes deterministically.
	extra := make([]string, 0, len(laneMap))
	for k := range laneMap {
		extra = append(extra, k)
	}
	sort.Strings(extra)
	for _, k := range extra {
		out = append(out, *laneMap[k])
	}
	return out
}

func laneForMovie(m Movie) string {
	category, region, lang, source := movieSeriesFields(m.Category, m.Region, m.Language, m.SourceTag, m.ProviderCategoryName, m.Title, "movie")
	_ = lang
	_ = source
	switch category {
	case "sports":
		return "sports"
	case "news":
		return "news"
	case "kids":
		return "kids"
	case "music":
		return "music"
	}
	switch region {
	case "uk", "europe":
		return "euroUKMovies"
	case "mena":
		return "menaMovies"
	case "us", "ca":
		return "movies"
	case "intl":
		return "movies"
	default:
		return "movies"
	}
}

func laneForSeries(s Series) string {
	category, region, lang, source := movieSeriesFields(s.Category, s.Region, s.Language, s.SourceTag, s.ProviderCategoryName, s.Title, "show")
	_ = lang
	_ = source
	switch category {
	case "sports":
		return "sports"
	case "news":
		return "news"
	case "kids":
		return "kids"
	case "music":
		return "music"
	}
	switch region {
	case "uk", "europe":
		return "euroUKTV"
	case "mena":
		return "menaTV"
	case "us", "ca":
		if isLikelyBcastUSSeries(region, languageOrDefault(lang), source, s.ProviderCategoryName, s.Title) {
			return "bcastUS"
		}
		return "tv"
	case "intl":
		return "tv"
	default:
		return "tv"
	}
}

func isLikelyBcastUSSeries(region, language, sourceTag, providerCategoryName, title string) bool {
	if region != "us" && region != "ca" {
		return false
	}
	if language != "en" {
		return false
	}
	upCat := strings.ToUpper(strings.TrimSpace(providerCategoryName))
	upTag := strings.ToUpper(strings.TrimSpace(sourceTag))
	upTitle := strings.ToUpper(strings.TrimSpace(title))

	// Keep dubbed/subbed/regional repack categories out of the broadcast lane.
	if containsAny(upCat, "PERSIAN", "ARAB", "HINDI", "TURK", "DUB", "SUB", "FRENCH", "GERMAN", "ITALIAN", "SPANISH") {
		return false
	}
	if upTag != "" && !containsAny(upTag, "EN", "US", "CA", "4K-NF", "4K-A+", "4K-D+", "AMZN", "HBO", "HULU", "NF", "A+", "D+") {
		return false
	}
	if containsAny(upCat, "CANADIAN", "CANADA", "US SERIES", "USA", "AMERICAN", "ENGLISH SERIES", "ENGLISH TV") {
		return true
	}
	// Generic provider categories like "ENGLISH SERIES" often vary; allow common TV buckets.
	if containsAny(upCat, "SERIES", "TV SHOW", "DRAMA", "COMEDY", "SITCOM", "REALITY", "SOAP", "CRIME", "THRILLER") &&
		containsAny(upTitle, "(US)", "(CA)") {
		return true
	}
	// Final weak fallback: explicit US/CA marker + English-ish source tag.
	return containsAny(upTitle, "(US)", "(CA)") && containsAny(upTag, "EN", "US", "CA", "4K-EN")
}

func languageOrDefault(s string) string {
	if strings.TrimSpace(s) == "" {
		return "en"
	}
	return s
}

func movieSeriesFields(category, region, language, sourceTag, providerCategoryName, title, kind string) (string, string, string, string) {
	if category == "" || region == "" || language == "" {
		c, r, l, s := classifyVOD(title, kind, providerCategoryName)
		if category == "" {
			category = c
		}
		if region == "" {
			region = r
		}
		if language == "" {
			language = l
		}
		if sourceTag == "" {
			sourceTag = s
		}
	}
	if region == "" {
		region = "intl"
	}
	if category == "" {
		category = kindDefaultCategory(kind)
	}
	return category, region, language, sourceTag
}

// SaveVODLanes writes per-lane catalog JSON files under outDir and returns the
// written file paths keyed by lane name. Each lane catalog keeps only VOD data
// (movies/series) and drops live_channels.
func SaveVODLanes(outDir string, lanes []VODLaneCatalog) (map[string]string, error) {
	if strings.TrimSpace(outDir) == "" {
		return nil, fmt.Errorf("output directory required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", outDir, err)
	}
	written := map[string]string{}
	for _, lane := range lanes {
		if len(lane.Movies) == 0 && len(lane.Series) == 0 {
			continue
		}
		p := filepath.Join(outDir, lane.Name+".json")
		c := New()
		c.ReplaceWithLive(lane.Movies, lane.Series, nil)
		if err := c.Save(p); err != nil {
			return nil, fmt.Errorf("save lane %s: %w", lane.Name, err)
		}
		written[lane.Name] = p
	}
	return written, nil
}

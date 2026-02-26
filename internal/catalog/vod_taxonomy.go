package catalog

import (
	"regexp"
	"sort"
	"strings"
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// ApplyVODTaxonomy enriches movie/series entries with coarse category metadata and
// returns deterministically sorted slices. This is intentionally heuristic and
// lightweight; it provides stable grouping for catch-up/category library splits.
func ApplyVODTaxonomy(movies []Movie, series []Series) ([]Movie, []Series) {
	outMovies := make([]Movie, len(movies))
	copy(outMovies, movies)
	for i := range outMovies {
		cat, region, lang, source := classifyVODTitle(outMovies[i].Title, "movie")
		outMovies[i].Category = cat
		outMovies[i].Region = region
		outMovies[i].Language = lang
		outMovies[i].SourceTag = source
	}
	sort.SliceStable(outMovies, func(i, j int) bool {
		return movieSortLess(outMovies[i], outMovies[j])
	})

	outSeries := make([]Series, len(series))
	copy(outSeries, series)
	for i := range outSeries {
		cat, region, lang, source := classifyVODTitle(outSeries[i].Title, "show")
		outSeries[i].Category = cat
		outSeries[i].Region = region
		outSeries[i].Language = lang
		outSeries[i].SourceTag = source
	}
	sort.SliceStable(outSeries, func(i, j int) bool {
		return seriesSortLess(outSeries[i], outSeries[j])
	})
	return outMovies, outSeries
}

func movieSortLess(a, b Movie) bool {
	ak, bk := sortTuple(a.Category, a.Region, a.Language, a.SourceTag, a.Title), sortTuple(b.Category, b.Region, b.Language, b.SourceTag, b.Title)
	if ak != bk {
		return ak < bk
	}
	if a.Year != b.Year {
		return a.Year < b.Year
	}
	return a.ID < b.ID
}

func seriesSortLess(a, b Series) bool {
	ak, bk := sortTuple(a.Category, a.Region, a.Language, a.SourceTag, a.Title), sortTuple(b.Category, b.Region, b.Language, b.SourceTag, b.Title)
	if ak != bk {
		return ak < bk
	}
	if a.Year != b.Year {
		return a.Year < b.Year
	}
	return a.ID < b.ID
}

func sortTuple(parts ...string) string {
	for i := range parts {
		parts[i] = normalizeSortKey(parts[i])
	}
	return strings.Join(parts, "\x1f")
}

func normalizeSortKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlphaNum.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func classifyVODTitle(title, kind string) (category, region, language, sourceTag string) {
	category = kindDefaultCategory(kind)
	region = "intl"
	language = inferLanguage(title)
	sourceTag, displayTitle := splitSourcePrefix(title)
	hay := strings.ToUpper(strings.TrimSpace(displayTitle))
	sourceHay := strings.ToUpper(strings.TrimSpace(sourceTag))
	all := strings.TrimSpace(sourceHay + " " + hay)

	switch {
	case containsAny(all, " TSN ", " ESPN", " SPORTS", " DAZN", " SKY SPORTS", " BT SPORT", " NHL ", " NFL ", " NBA ", " MLB ", " UFC ", " WWE ", " BEIN SPORT"):
		category = "sports"
	case containsAny(all, " NEWS", " CNN", " BBC NEWS", " FOX NEWS", " MSNBC", " CNBC", " BLOOMBERG", " ALJAZEERA", " AL JAZEERA"):
		category = "news"
	case containsAny(all, " MUSIC", " MTV", " MUCH", " VEVO", " CONCERT"):
		category = "music"
	case containsAny(all, " DISNEY", " NICK", " NICKELODEON", " CARTOON", " PBS KIDS", " KIDS"):
		category = "kids"
	}

	switch {
	case containsAny(all, " UK", " GB", " BBC", " ITV", " CHANNEL 4", " SKY ", " BRIT", "(GB)"):
		region = "uk"
	case containsAny(all, " CANADA", "(CA)", " CTV", " CBC", " GLOBAL", " CITYTV", " ROGERS"):
		region = "ca"
	case containsAny(all, " US", "(US)", " NBC", " CBS", " ABC", " FOX"):
		region = "us"
	case containsAny(all, " ARAB", " MENA", " OSN", " SHAHID", " BEIN", " AL JAZEERA"):
		region = "mena"
	case containsAny(all, " EU", " EURO", " FRANCE", " GERMAN", " ITAL", " SPAIN"):
		region = "europe"
	}

	if category == "movie" && (region == "uk" || region == "europe") {
		// keep movie category but region drives future library split
	}
	return category, region, language, sourceTag
}

func kindDefaultCategory(kind string) string {
	switch kind {
	case "show":
		return "tv"
	default:
		return "movies"
	}
}

func splitSourcePrefix(title string) (prefix, rest string) {
	t := strings.TrimSpace(title)
	if t == "" {
		return "", ""
	}
	parts := strings.SplitN(t, " - ", 2)
	if len(parts) != 2 {
		return "", t
	}
	p := strings.TrimSpace(parts[0])
	if p == "" || len(p) > 24 {
		return "", t
	}
	// Heuristic: tags are short, usually uppercase-ish, may contain digits/+/_.
	if !looksLikeSourceTag(p) {
		return "", t
	}
	return p, strings.TrimSpace(parts[1])
}

func looksLikeSourceTag(s string) bool {
	upperish := 0
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			upperish++
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '+' || r == '&':
		default:
			return false
		}
	}
	return upperish >= 2
}

func inferLanguage(title string) string {
	if containsArabicScript(title) {
		return "ar"
	}
	if containsCyrillicScript(title) {
		return "ru"
	}
	return "en"
}

func containsArabicScript(s string) bool {
	for _, r := range s {
		if r >= 0x0600 && r <= 0x06FF {
			return true
		}
	}
	return false
}

func containsCyrillicScript(s string) bool {
	for _, r := range s {
		if r >= 0x0400 && r <= 0x04FF {
			return true
		}
	}
	return false
}

func containsAny(s string, needles ...string) bool {
	padded := " " + strings.ToUpper(s) + " "
	for _, n := range needles {
		if strings.Contains(padded, strings.ToUpper(n)) {
			return true
		}
	}
	return false
}

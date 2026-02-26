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
		cat, region, lang, source := classifyVOD(outMovies[i].Title, "movie", outMovies[i].ProviderCategoryName)
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
		cat, region, lang, source := classifyVOD(outSeries[i].Title, "show", outSeries[i].ProviderCategoryName)
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

func classifyVOD(title, kind, providerCategoryName string) (category, region, language, sourceTag string) {
	category = kindDefaultCategory(kind)
	region = "intl"
	language = inferLanguage(title)
	sourceTag, displayTitle := splitSourcePrefix(title)
	hay := strings.ToUpper(strings.TrimSpace(displayTitle))
	sourceHay := strings.ToUpper(strings.TrimSpace(sourceTag))
	providerHay := strings.ToUpper(strings.TrimSpace(providerCategoryName))
	all := strings.TrimSpace(sourceHay + " " + hay)

	if r := inferRegionFromSourceTag(sourceHay); r != "" {
		region = r
	}
	if c := inferCategoryFromSourceTag(sourceHay); c != "" {
		category = c
	}
	if c, r := inferCategoryRegionFromProviderCategory(providerHay, kind); c != "" || r != "" {
		if c != "" {
			category = c
		}
		if r != "" {
			region = r
		}
	}

	switch {
	case containsAny(all, " TSN ", " ESPN ", " DAZN ", " SKY SPORTS", " BT SPORT", " NHL ", " NFL ", " NBA ", " MLB ", " UFC ", " WWE ", " BEIN SPORT", " FORMULA 1 ", " F1 "):
		category = "sports"
	case containsAny(all, " CNN ", " BBC NEWS", " FOX NEWS", " MSNBC ", " CNBC ", " BLOOMBERG", " ALJAZEERA", " AL JAZEERA", " FRANCE 24 ", " SKY NEWS"):
		category = "news"
	case containsAny(all, " MTV ", " MUCHMUSIC", " VEVO ", " APPLE MUSIC LIVE", " LIVE AT WEMBLEY", " UNPLUGGED"):
		category = "music"
	case containsAny(all, " NICKELODEON", " CARTOON NETWORK", " PBS KIDS", " DISNEY JUNIOR", " DISNEY CHANNEL", " DISNEY XD"):
		category = "kids"
	}

	switch {
	case containsAny(all, " UK", " GB", " BBC", " ITV", " CHANNEL 4", " SKY ", " BRIT", "(GB)"):
		region = "uk"
	case containsAny(all, " CANADA", "(CA)", " CTV", " CBC", " GLOBAL", " CITYTV", " ROGERS"):
		region = "ca"
	case containsAny(all, " US", "(US)", " NBC", " CBS", " ABC", " FOX"):
		region = "us"
	case containsAny(all, " OSN ", " SHAHID", " BEIN ", " AL JAZEERA", "(AE)", "(SA)", "(EG)", "(QA)"):
		region = "mena"
	case containsAny(all, "(DE)", "(FR)", "(ES)", "(IT)", "(NL)", "(SE)", "(NO)", "(DK)", "(FI)"):
		region = "europe"
	}

	if category == "movie" && (region == "uk" || region == "europe") {
		// keep movie category but region drives future library split
	}
	return category, region, language, sourceTag
}

func inferCategoryRegionFromProviderCategory(cat, kind string) (category, region string) {
	if cat == "" {
		return "", ""
	}
	category = ""
	region = ""
	switch {
	case containsAny(cat, "SPORT", "NBA", "NHL", "NFL", "MLB", "UFC", "WWE", "MOTORSPORT", "FORMULA", "SOCCER", "FOOTBALL"):
		category = "sports"
	case containsAny(cat, "NEWS", "CURRENT AFFAIRS"):
		category = "news"
	case containsAny(cat, "KIDS", "CHILD", "CARTOON", "ANIMATION", "DISNEY", "NICK"):
		category = "kids"
	case containsAny(cat, "MUSIC", "CONCERT", "KARAOKE"):
		category = "music"
	case containsAny(cat, "MOVIE", "FILM", "CINEMA"):
		category = "movies"
	case kind == "show" && containsAny(cat, "SERIES", "TV SHOW", "SHOW"):
		category = "tv"
	}
	switch {
	case containsAny(cat, "UK", "BRIT", "BRITISH"):
		region = "uk"
	case containsAny(cat, "CANADA", "CANADIAN"):
		region = "ca"
	case containsAny(cat, "USA", "UNITED STATES", "US "):
		region = "us"
	case containsAny(cat, "ARAB", "MENA", "MIDDLE EAST", "GULF"):
		region = "mena"
	case containsAny(cat, "EURO", "FRANCE", "GERMAN", "ITAL", "SPAIN", "NORDIC"):
		region = "europe"
	}
	return category, region
}

func inferRegionFromSourceTag(tag string) string {
	switch {
	case tag == "":
		return ""
	case strings.HasPrefix(tag, "UK"), strings.HasPrefix(tag, "GB"):
		return "uk"
	case strings.HasPrefix(tag, "US"), strings.HasPrefix(tag, "EN-US"):
		return "us"
	case strings.HasPrefix(tag, "CA"), strings.HasPrefix(tag, "CAN"):
		return "ca"
	case strings.HasPrefix(tag, "AR"), strings.HasPrefix(tag, "IR"), strings.HasPrefix(tag, "MENA"), strings.HasPrefix(tag, "BEIN"), strings.HasPrefix(tag, "OSN"):
		return "mena"
	case strings.HasPrefix(tag, "DE"), strings.HasPrefix(tag, "FR"), strings.HasPrefix(tag, "ES"), strings.HasPrefix(tag, "IT"), strings.HasPrefix(tag, "NL"), strings.HasPrefix(tag, "SE"), strings.HasPrefix(tag, "NO"), strings.HasPrefix(tag, "DK"), strings.HasPrefix(tag, "FI"):
		return "europe"
	}
	return ""
}

func inferCategoryFromSourceTag(tag string) string {
	switch {
	case tag == "":
		return ""
	case strings.Contains(tag, "KIDS"):
		return "kids"
	case strings.HasPrefix(tag, "MTV"), strings.Contains(tag, "MUSIC"):
		return "music"
	case strings.Contains(tag, "SPORT"), strings.Contains(tag, "WWE"), strings.Contains(tag, "UFC"), strings.Contains(tag, "F1"):
		return "sports"
	case strings.Contains(tag, "NEWS"):
		return "news"
	}
	return ""
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

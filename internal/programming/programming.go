package programming

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

const RecipeVersion = 1

type Recipe struct {
	Version            int      `json:"version"`
	SelectedCategories []string `json:"selected_categories,omitempty"`
	ExcludedCategories []string `json:"excluded_categories,omitempty"`
	IncludedChannelIDs []string `json:"included_channel_ids,omitempty"`
	ExcludedChannelIDs []string `json:"excluded_channel_ids,omitempty"`
	OrderMode          string   `json:"order_mode,omitempty"` // source | custom | recommended
	CustomOrder        []string `json:"custom_order,omitempty"`
	UpdatedAt          string   `json:"updated_at,omitempty"`
}

type CategorySummary struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	SourceTag      string   `json:"source_tag,omitempty"`
	Count          int      `json:"count"`
	EPGLinkedCount int      `json:"epg_linked_count"`
	SampleChannels []string `json:"sample_channels,omitempty"`
}

type CategoryMember struct {
	CategoryID  string `json:"category_id"`
	Bucket      string `json:"bucket,omitempty"`
	ChannelID   string `json:"channel_id"`
	GuideNumber string `json:"guide_number"`
	GuideName   string `json:"guide_name"`
	TVGID       string `json:"tvg_id,omitempty"`
	SourceTag   string `json:"source_tag,omitempty"`
	GroupTitle  string `json:"group_title,omitempty"`
}

type TaxonomyBucket string

const (
	BucketLocalBroadcast       TaxonomyBucket = "local_broadcast"
	BucketGeneralEntertainment TaxonomyBucket = "general_entertainment"
	BucketNewsInfo             TaxonomyBucket = "news_info"
	BucketSports               TaxonomyBucket = "sports"
	BucketLifestyleHome        TaxonomyBucket = "lifestyle_home"
	BucketDocumentaryHistory   TaxonomyBucket = "documentary_history"
	BucketChildrenFamily       TaxonomyBucket = "children_family"
	BucketRealitySpecialized   TaxonomyBucket = "reality_specialized"
	BucketPremiumNetworks      TaxonomyBucket = "premium_networks"
	BucketRegionalSports       TaxonomyBucket = "regional_sports"
	BucketReligious            TaxonomyBucket = "religious"
	BucketInternational        TaxonomyBucket = "international"
)

var bucketOrder = []TaxonomyBucket{
	BucketLocalBroadcast,
	BucketGeneralEntertainment,
	BucketNewsInfo,
	BucketSports,
	BucketLifestyleHome,
	BucketDocumentaryHistory,
	BucketChildrenFamily,
	BucketRealitySpecialized,
	BucketPremiumNetworks,
	BucketRegionalSports,
	BucketReligious,
	BucketInternational,
}

func LoadRecipeFile(path string) (Recipe, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return NormalizeRecipe(Recipe{}), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NormalizeRecipe(Recipe{}), nil
		}
		return Recipe{}, err
	}
	var recipe Recipe
	if err := json.Unmarshal(data, &recipe); err != nil {
		return Recipe{}, err
	}
	return NormalizeRecipe(recipe), nil
}

func SaveRecipeFile(path string, recipe Recipe) (Recipe, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Recipe{}, fmt.Errorf("programming recipe file not configured")
	}
	recipe = NormalizeRecipe(recipe)
	data, err := json.MarshalIndent(recipe, "", "  ")
	if err != nil {
		return Recipe{}, err
	}
	dir := filepath.Dir(filepath.Clean(path))
	tmp, err := os.CreateTemp(dir, ".programming-recipe-*.json.tmp")
	if err != nil {
		return Recipe{}, err
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		if writeErr != nil {
			return Recipe{}, writeErr
		}
		return Recipe{}, closeErr
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return Recipe{}, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return Recipe{}, err
	}
	return recipe, nil
}

func NormalizeRecipe(recipe Recipe) Recipe {
	recipe.Version = RecipeVersion
	recipe.SelectedCategories = dedupeSorted(recipe.SelectedCategories)
	recipe.ExcludedCategories = dedupeSorted(recipe.ExcludedCategories)
	recipe.IncludedChannelIDs = dedupeSorted(recipe.IncludedChannelIDs)
	recipe.ExcludedChannelIDs = dedupeSorted(recipe.ExcludedChannelIDs)
	recipe.CustomOrder = dedupeKeepOrder(recipe.CustomOrder)
	switch strings.ToLower(strings.TrimSpace(recipe.OrderMode)) {
	case "", "source", "custom", "recommended":
		recipe.OrderMode = strings.ToLower(strings.TrimSpace(recipe.OrderMode))
		if recipe.OrderMode == "" {
			recipe.OrderMode = "source"
		}
	default:
		recipe.OrderMode = "source"
	}
	recipe.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return recipe
}

func BuildCategoryInventory(channels []catalog.LiveChannel) []CategorySummary {
	type row struct {
		id      string
		label   string
		source  string
		count   int
		epg     int
		samples []string
	}
	byID := map[string]*row{}
	for _, ch := range channels {
		id, label, source := CategoryIdentity(ch)
		cur, ok := byID[id]
		if !ok {
			cur = &row{id: id, label: label, source: source}
			byID[id] = cur
		}
		cur.count++
		if ch.EPGLinked || strings.TrimSpace(ch.TVGID) != "" {
			cur.epg++
		}
		if len(cur.samples) < 3 && strings.TrimSpace(ch.GuideName) != "" {
			cur.samples = append(cur.samples, strings.TrimSpace(ch.GuideName))
		}
	}
	out := make([]CategorySummary, 0, len(byID))
	for _, cur := range byID {
		out = append(out, CategorySummary{
			ID:             cur.id,
			Label:          cur.label,
			SourceTag:      cur.source,
			Count:          cur.count,
			EPGLinkedCount: cur.epg,
			SampleChannels: append([]string(nil), cur.samples...),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			if out[i].SourceTag == out[j].SourceTag {
				return out[i].Label < out[j].Label
			}
			return out[i].SourceTag < out[j].SourceTag
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func CategoryMembers(channels []catalog.LiveChannel, categoryID string) []CategoryMember {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil
	}
	out := make([]CategoryMember, 0)
	for _, ch := range channels {
		id, _, _ := CategoryIdentity(ch)
		if id != categoryID {
			continue
		}
		out = append(out, CategoryMember{
			CategoryID:  id,
			Bucket:      string(ClassifyChannel(ch)),
			ChannelID:   strings.TrimSpace(ch.ChannelID),
			GuideNumber: strings.TrimSpace(ch.GuideNumber),
			GuideName:   strings.TrimSpace(ch.GuideName),
			TVGID:       strings.TrimSpace(ch.TVGID),
			SourceTag:   strings.TrimSpace(ch.SourceTag),
			GroupTitle:  strings.TrimSpace(ch.GroupTitle),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GuideNumber == out[j].GuideNumber {
			return out[i].GuideName < out[j].GuideName
		}
		return out[i].GuideNumber < out[j].GuideNumber
	})
	return out
}

func ApplyRecipe(channels []catalog.LiveChannel, recipe Recipe) []catalog.LiveChannel {
	if len(channels) == 0 {
		return nil
	}
	recipe = NormalizeRecipe(recipe)
	selected := make(map[string]struct{}, len(recipe.SelectedCategories))
	for _, id := range recipe.SelectedCategories {
		selected[strings.TrimSpace(id)] = struct{}{}
	}
	excludedCategories := make(map[string]struct{}, len(recipe.ExcludedCategories))
	for _, id := range recipe.ExcludedCategories {
		excludedCategories[strings.TrimSpace(id)] = struct{}{}
	}
	included := make(map[string]struct{}, len(recipe.IncludedChannelIDs))
	for _, id := range recipe.IncludedChannelIDs {
		included[strings.TrimSpace(id)] = struct{}{}
	}
	excluded := make(map[string]struct{}, len(recipe.ExcludedChannelIDs))
	for _, id := range recipe.ExcludedChannelIDs {
		excluded[strings.TrimSpace(id)] = struct{}{}
	}

	filtered := make([]catalog.LiveChannel, 0, len(channels))
	for _, ch := range channels {
		channelID := strings.TrimSpace(ch.ChannelID)
		categoryID, _, _ := CategoryIdentity(ch)
		if _, dropCategory := excludedCategories[categoryID]; dropCategory {
			if _, force := included[channelID]; !force {
				continue
			}
		}
		if _, drop := excluded[channelID]; drop {
			continue
		}
		keep := len(selected) == 0
		if !keep {
			_, keep = selected[categoryID]
		}
		if _, force := included[channelID]; force {
			keep = true
		}
		if keep {
			filtered = append(filtered, ch)
		}
	}
	if len(filtered) == 0 {
		return filtered
	}
	switch recipe.OrderMode {
	case "custom":
		filtered = applyCustomOrder(filtered, recipe.CustomOrder)
	case "recommended":
		filtered = applyRecommendedOrder(filtered, recipe.CustomOrder)
	}
	return filtered
}

func CategoryIdentity(ch catalog.LiveChannel) (id, label, sourceTag string) {
	label = strings.TrimSpace(ch.GroupTitle)
	sourceTag = strings.TrimSpace(ch.SourceTag)
	switch {
	case label == "" && sourceTag == "":
		label = "Uncategorized"
	case label == "":
		label = sourceTag
	}
	base := slug(strings.ToLower(label))
	if base == "" {
		base = "uncategorized"
	}
	if sourceTag != "" && !strings.EqualFold(sourceTag, label) {
		src := slug(strings.ToLower(sourceTag))
		if src == "" {
			src = "source"
		}
		base = src + "--" + base
	}
	return base, label, sourceTag
}

func applyCustomOrder(channels []catalog.LiveChannel, order []string) []catalog.LiveChannel {
	rank := map[string]int{}
	for i, id := range order {
		id = strings.TrimSpace(id)
		if id != "" {
			rank[id] = i
		}
	}
	out := append([]catalog.LiveChannel(nil), channels...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, iok := rank[strings.TrimSpace(out[i].ChannelID)]
		rj, jok := rank[strings.TrimSpace(out[j].ChannelID)]
		switch {
		case iok && jok:
			return ri < rj
		case iok:
			return true
		case jok:
			return false
		default:
			return false
		}
	})
	return out
}

func applyRecommendedOrder(channels []catalog.LiveChannel, order []string) []catalog.LiveChannel {
	rank := map[string]int{}
	for i, id := range dedupeKeepOrder(order) {
		rank[strings.TrimSpace(id)] = i
	}
	bucketRank := map[TaxonomyBucket]int{}
	for i, bucket := range bucketOrder {
		bucketRank[bucket] = i
	}
	out := append([]catalog.LiveChannel(nil), channels...)
	sort.SliceStable(out, func(i, j int) bool {
		leftBucket := ClassifyChannel(out[i])
		rightBucket := ClassifyChannel(out[j])
		if bucketRank[leftBucket] != bucketRank[rightBucket] {
			return bucketRank[leftBucket] < bucketRank[rightBucket]
		}
		ri, iok := rank[strings.TrimSpace(out[i].ChannelID)]
		rj, jok := rank[strings.TrimSpace(out[j].ChannelID)]
		switch {
		case iok && jok && ri != rj:
			return ri < rj
		case iok != jok:
			return iok
		}
		if strings.TrimSpace(out[i].GuideNumber) != strings.TrimSpace(out[j].GuideNumber) {
			return strings.TrimSpace(out[i].GuideNumber) < strings.TrimSpace(out[j].GuideNumber)
		}
		return strings.TrimSpace(out[i].GuideName) < strings.TrimSpace(out[j].GuideName)
	})
	return out
}

func ClassifyChannel(ch catalog.LiveChannel) TaxonomyBucket {
	s := normalizedSearchText(ch)
	group := strings.ToLower(strings.TrimSpace(ch.GroupTitle))
	switch {
	case containsAny(group+" "+s, []string{"abc", "cbs", "nbc", "fox", "pbs", "cw", "ctv", "cbc", "global", "citytv", "omni", "local", "broadcast"}):
		return BucketLocalBroadcast
	case containsAny(group+" "+s, []string{"regional sports", "rsn", "sportsnet one", "msg", "yes network", "bally", "marquee"}):
		return BucketRegionalSports
	case containsAny(group+" "+s, []string{"news", "cnn", "msnbc", "fox news", "cnbc", "bloomberg", "weather"}):
		return BucketNewsInfo
	case containsAny(group+" "+s, []string{"sport", "espn", "tsn", "nfl", "nba", "nhl", "mlb", "golf", "tennis", "soccer", "fight", "boxing", "ufc"}):
		return BucketSports
	case containsAny(group+" "+s, []string{"hbo", "showtime", "starz", "cinemax", "movie channel", "premium"}):
		return BucketPremiumNetworks
	case containsAny(group+" "+s, []string{"kids", "family", "disney", "nick", "nickelodeon", "cartoon", "pbs kids", "junior", "jr", "boomerang"}):
		return BucketChildrenFamily
	case containsAny(group+" "+s, []string{"history", "documentary", "national geographic", "nat geo", "science", "smithsonian"}):
		return BucketDocumentaryHistory
	case containsAny(group+" "+s, []string{"food", "hgtv", "lifestyle", "home", "travel", "cooking", "magnolia"}):
		return BucketLifestyleHome
	case containsAny(group+" "+s, []string{"reality", "court", "crime", "specialized", "shopping", "qvc", "hsn", "game show"}):
		return BucketRealitySpecialized
	case containsAny(group+" "+s, []string{"faith", "religion", "christian", "catholic", "tbn", "daystar", "church"}):
		return BucketReligious
	case looksInternational(ch):
		return BucketInternational
	default:
		return BucketGeneralEntertainment
	}
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}

func looksInternational(ch catalog.LiveChannel) bool {
	group := strings.ToLower(strings.TrimSpace(ch.GroupTitle))
	s := normalizedSearchText(ch)
	if containsAny(group+" "+s, []string{"intl", "international", "latino", "spanish", "french", "arabic", "hindi", "turkish", "filipino"}) {
		return true
	}
	tvg := strings.ToLower(strings.TrimSpace(ch.TVGID))
	switch {
	case strings.HasSuffix(tvg, ".us"), strings.HasSuffix(tvg, ".ca"), strings.HasSuffix(tvg, ".uk"), strings.HasSuffix(tvg, ".gb"):
		return false
	case tvg != "":
		return true
	default:
		return false
	}
}

func normalizedSearchText(ch catalog.LiveChannel) string {
	parts := []string{
		strings.TrimSpace(ch.GuideName),
		strings.TrimSpace(ch.TVGID),
		strings.TrimSpace(ch.GroupTitle),
		strings.TrimSpace(ch.SourceTag),
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func UpdateRecipeCategories(recipe Recipe, action string, categoryIDs []string) Recipe {
	recipe = NormalizeRecipe(recipe)
	ids := dedupeSorted(categoryIDs)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "include":
		recipe.SelectedCategories = dedupeSorted(append(recipe.SelectedCategories, ids...))
		recipe.ExcludedCategories = subtractValues(recipe.ExcludedCategories, ids)
	case "exclude":
		recipe.ExcludedCategories = dedupeSorted(append(recipe.ExcludedCategories, ids...))
		recipe.SelectedCategories = subtractValues(recipe.SelectedCategories, ids)
	case "remove", "clear":
		recipe.SelectedCategories = subtractValues(recipe.SelectedCategories, ids)
		recipe.ExcludedCategories = subtractValues(recipe.ExcludedCategories, ids)
	}
	return NormalizeRecipe(recipe)
}

func UpdateRecipeChannels(recipe Recipe, action string, channelIDs []string) Recipe {
	recipe = NormalizeRecipe(recipe)
	ids := dedupeSorted(channelIDs)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "include":
		recipe.IncludedChannelIDs = dedupeSorted(append(recipe.IncludedChannelIDs, ids...))
		recipe.ExcludedChannelIDs = subtractValues(recipe.ExcludedChannelIDs, ids)
	case "exclude":
		recipe.ExcludedChannelIDs = dedupeSorted(append(recipe.ExcludedChannelIDs, ids...))
		recipe.IncludedChannelIDs = subtractValues(recipe.IncludedChannelIDs, ids)
	case "remove", "clear":
		recipe.IncludedChannelIDs = subtractValues(recipe.IncludedChannelIDs, ids)
		recipe.ExcludedChannelIDs = subtractValues(recipe.ExcludedChannelIDs, ids)
	}
	return NormalizeRecipe(recipe)
}

func subtractValues(existing, remove []string) []string {
	if len(existing) == 0 || len(remove) == 0 {
		return dedupeSorted(existing)
	}
	rm := map[string]struct{}{}
	for _, v := range remove {
		v = strings.TrimSpace(v)
		if v != "" {
			rm[v] = struct{}{}
		}
	}
	out := make([]string, 0, len(existing))
	for _, v := range existing {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, drop := rm[v]; drop {
			continue
		}
		out = append(out, v)
	}
	return dedupeSorted(out)
}

func dedupeSorted(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func dedupeKeepOrder(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func slug(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '/' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

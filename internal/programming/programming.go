package programming

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

const RecipeVersion = 1

var backupNameNoise = regexp.MustCompile(`[^a-z0-9]+`)

type Recipe struct {
	Version              int      `json:"version"`
	SelectedCategories   []string `json:"selected_categories,omitempty"`
	ExcludedCategories   []string `json:"excluded_categories,omitempty"`
	IncludedChannelIDs   []string `json:"included_channel_ids,omitempty"`
	ExcludedChannelIDs   []string `json:"excluded_channel_ids,omitempty"`
	PreferredBackupIDs   []string `json:"preferred_backup_ids,omitempty"`
	OrderMode            string   `json:"order_mode,omitempty"` // source | custom | recommended
	CustomOrder          []string `json:"custom_order,omitempty"`
	CollapseExactBackups bool     `json:"collapse_exact_backups,omitempty"`
	UpdatedAt            string   `json:"updated_at,omitempty"`
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
	CategoryID  string         `json:"category_id"`
	Bucket      string         `json:"bucket,omitempty"`
	ChannelID   string         `json:"channel_id"`
	GuideNumber string         `json:"guide_number"`
	GuideName   string         `json:"guide_name"`
	TVGID       string         `json:"tvg_id,omitempty"`
	SourceTag   string         `json:"source_tag,omitempty"`
	GroupTitle  string         `json:"group_title,omitempty"`
	Descriptor  FeedDescriptor `json:"descriptor,omitempty"`
}

type FeedDescriptor struct {
	Label       string   `json:"label,omitempty"`
	Region      string   `json:"region,omitempty"`
	Category    string   `json:"category,omitempty"`
	Source      string   `json:"source,omitempty"`
	QualityTags []string `json:"quality_tags,omitempty"`
	Variant     string   `json:"variant,omitempty"`
}

type TaxonomyBucket string

type BackupMatchStrategy string

const (
	BackupMatchTVGID BackupMatchStrategy = "tvg_id_exact"
	BackupMatchDNAID BackupMatchStrategy = "dna_id_exact"
)

type BackupGroupMember struct {
	ChannelID   string         `json:"channel_id"`
	DNAID       string         `json:"dna_id,omitempty"`
	GuideNumber string         `json:"guide_number"`
	GuideName   string         `json:"guide_name"`
	TVGID       string         `json:"tvg_id,omitempty"`
	SourceTag   string         `json:"source_tag,omitempty"`
	GroupTitle  string         `json:"group_title,omitempty"`
	StreamCount int            `json:"stream_count"`
	PrimaryURL  string         `json:"primary_url,omitempty"`
	Descriptor  FeedDescriptor `json:"descriptor,omitempty"`
}

type BackupGroup struct {
	Key           string              `json:"key"`
	MatchStrategy BackupMatchStrategy `json:"match_strategy"`
	DisplayName   string              `json:"display_name"`
	PrimaryID     string              `json:"primary_channel_id"`
	PrimarySource string              `json:"primary_source_tag,omitempty"`
	BackupCount   int                 `json:"backup_count"`
	MemberCount   int                 `json:"member_count"`
	Members       []BackupGroupMember `json:"members"`
}

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
	recipe.PreferredBackupIDs = dedupeKeepOrder(recipe.PreferredBackupIDs)
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
			Descriptor:  DescribeChannel(ch),
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
	return applyRecipe(channels, recipe, true)
}

func ApplyRecipePreview(channels []catalog.LiveChannel, recipe Recipe) []catalog.LiveChannel {
	return applyRecipe(channels, recipe, false)
}

func applyRecipe(channels []catalog.LiveChannel, recipe Recipe, collapseBackups bool) []catalog.LiveChannel {
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
	if collapseBackups && recipe.CollapseExactBackups {
		filtered = CollapseExactBackupGroupsWithPreferences(filtered, recipe.PreferredBackupIDs)
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

func UpdateRecipeOrder(recipe Recipe, action string, channelIDs []string, beforeID, afterID string) Recipe {
	recipe = NormalizeRecipe(recipe)
	order := dedupeKeepOrder(recipe.CustomOrder)
	ids := dedupeKeepOrder(channelIDs)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "append":
		order = append(orderWithout(order, ids), ids...)
	case "prepend":
		order = append(ids, orderWithout(order, ids)...)
	case "before":
		order = insertRelative(order, ids, strings.TrimSpace(beforeID), true)
	case "after":
		order = insertRelative(order, ids, strings.TrimSpace(afterID), false)
	case "remove", "clear":
		order = orderWithout(order, ids)
	default:
		return recipe
	}
	recipe.CustomOrder = dedupeKeepOrder(order)
	if len(recipe.CustomOrder) > 0 {
		recipe.OrderMode = "custom"
	} else if recipe.OrderMode == "custom" {
		recipe.OrderMode = "source"
	}
	return NormalizeRecipe(recipe)
}

func orderWithout(order, remove []string) []string {
	if len(order) == 0 || len(remove) == 0 {
		return dedupeKeepOrder(order)
	}
	rm := map[string]struct{}{}
	for _, id := range remove {
		id = strings.TrimSpace(id)
		if id != "" {
			rm[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(order))
	for _, id := range order {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, drop := rm[id]; drop {
			continue
		}
		out = append(out, id)
	}
	return dedupeKeepOrder(out)
}

func insertRelative(order, ids []string, anchor string, before bool) []string {
	base := orderWithout(order, ids)
	if len(ids) == 0 {
		return base
	}
	anchor = strings.TrimSpace(anchor)
	if anchor == "" {
		return append(base, ids...)
	}
	at := -1
	for i, id := range base {
		if strings.TrimSpace(id) == anchor {
			at = i
			break
		}
	}
	if at < 0 {
		return append(base, ids...)
	}
	insertAt := at
	if !before {
		insertAt = at + 1
	}
	out := make([]string, 0, len(base)+len(ids))
	out = append(out, base[:insertAt]...)
	out = append(out, ids...)
	out = append(out, base[insertAt:]...)
	return dedupeKeepOrder(out)
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

func UpdateRecipeBackupPreferences(recipe Recipe, action string, channelIDs []string) Recipe {
	ids := dedupeKeepOrder(channelIDs)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "prefer":
		recipe.PreferredBackupIDs = append(ids, recipe.PreferredBackupIDs...)
	case "remove", "unprefer":
		deny := toSet(ids)
		filtered := make([]string, 0, len(recipe.PreferredBackupIDs))
		for _, id := range recipe.PreferredBackupIDs {
			if _, drop := deny[strings.TrimSpace(id)]; drop {
				continue
			}
			filtered = append(filtered, id)
		}
		recipe.PreferredBackupIDs = filtered
	case "clear":
		recipe.PreferredBackupIDs = nil
	}
	return NormalizeRecipe(recipe)
}

func BuildBackupGroups(channels []catalog.LiveChannel) []BackupGroup {
	return BuildBackupGroupsWithPreferences(channels, nil)
}

func BuildBackupGroupsWithPreferences(channels []catalog.LiveChannel, preferred []string) []BackupGroup {
	type accum struct {
		strategy BackupMatchStrategy
		key      string
		members  []catalog.LiveChannel
	}
	groups := map[string]*accum{}
	order := make([]string, 0)
	for _, ch := range channels {
		key, strategy, ok := backupIdentity(ch)
		if !ok {
			continue
		}
		cur, exists := groups[key]
		if !exists {
			cur = &accum{strategy: strategy, key: key}
			groups[key] = cur
			order = append(order, key)
		}
		cur.members = append(cur.members, ch)
	}
	out := make([]BackupGroup, 0, len(groups))
	for _, key := range order {
		cur := groups[key]
		if cur == nil || len(cur.members) < 2 {
			continue
		}
		orderedMembers := preferBackupMembers(cur.members, preferred)
		members := make([]BackupGroupMember, 0, len(cur.members))
		for _, ch := range orderedMembers {
			members = append(members, BackupGroupMember{
				ChannelID:   strings.TrimSpace(ch.ChannelID),
				DNAID:       strings.TrimSpace(ch.DNAID),
				GuideNumber: strings.TrimSpace(ch.GuideNumber),
				GuideName:   strings.TrimSpace(ch.GuideName),
				TVGID:       strings.TrimSpace(ch.TVGID),
				SourceTag:   strings.TrimSpace(ch.SourceTag),
				GroupTitle:  strings.TrimSpace(ch.GroupTitle),
				StreamCount: visibleStreamCount(ch),
				PrimaryURL:  strings.TrimSpace(ch.StreamURL),
				Descriptor:  DescribeChannel(ch),
			})
		}
		primary := orderedMembers[0]
		display := strings.TrimSpace(primary.GuideName)
		if display == "" {
			display = strings.TrimSpace(primary.ChannelID)
		}
		out = append(out, BackupGroup{
			Key:           cur.key,
			MatchStrategy: cur.strategy,
			DisplayName:   display,
			PrimaryID:     strings.TrimSpace(primary.ChannelID),
			PrimarySource: strings.TrimSpace(primary.SourceTag),
			BackupCount:   len(cur.members) - 1,
			MemberCount:   len(cur.members),
			Members:       members,
		})
	}
	return out
}

func CollapseExactBackupGroups(channels []catalog.LiveChannel) []catalog.LiveChannel {
	return CollapseExactBackupGroupsWithPreferences(channels, nil)
}

func CollapseExactBackupGroupsWithPreferences(channels []catalog.LiveChannel, preferred []string) []catalog.LiveChannel {
	if len(channels) < 2 {
		return append([]catalog.LiveChannel(nil), channels...)
	}
	type accum struct {
		order   int
		members []catalog.LiveChannel
	}
	out := make([]catalog.LiveChannel, 0, len(channels))
	indexByKey := map[string]int{}
	grouped := map[string]*accum{}
	for _, ch := range channels {
		key, _, ok := backupIdentity(ch)
		if !ok {
			out = append(out, cloneChannel(ch))
			continue
		}
		cur, exists := grouped[key]
		if !exists {
			cur = &accum{order: len(out)}
			grouped[key] = cur
			indexByKey[key] = len(out)
			out = append(out, catalog.LiveChannel{})
		}
		cur.members = append(cur.members, ch)
	}
	for key, idx := range indexByKey {
		cur := grouped[key]
		if cur == nil || len(cur.members) == 0 {
			continue
		}
		orderedMembers := preferBackupMembers(cur.members, preferred)
		merged := cloneChannel(orderedMembers[0])
		for _, backup := range orderedMembers[1:] {
			merged = mergeBackupChannel(merged, backup)
		}
		out[idx] = merged
	}
	return out
}

func preferBackupMembers(channels []catalog.LiveChannel, preferred []string) []catalog.LiveChannel {
	out := append([]catalog.LiveChannel(nil), channels...)
	if len(out) < 2 || len(preferred) == 0 {
		return out
	}
	rank := map[string]int{}
	for i, id := range preferred {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := rank[id]; !exists {
			rank[id] = i
		}
	}
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

func mergeBackupChannel(primary, backup catalog.LiveChannel) catalog.LiveChannel {
	out := cloneChannel(primary)
	for _, url := range append([]string{backup.StreamURL}, backup.StreamURLs...) {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		if strings.TrimSpace(out.StreamURL) == "" {
			out.StreamURL = url
		}
		if !containsString(out.StreamURLs, url) {
			out.StreamURLs = append(out.StreamURLs, url)
		}
	}
	for _, auth := range backup.StreamAuths {
		if !containsStreamAuth(out.StreamAuths, auth) {
			out.StreamAuths = append(out.StreamAuths, auth)
		}
	}
	if strings.TrimSpace(out.TVGID) == "" {
		out.TVGID = strings.TrimSpace(backup.TVGID)
	}
	if strings.TrimSpace(out.DNAID) == "" {
		out.DNAID = strings.TrimSpace(backup.DNAID)
	}
	if !out.EPGLinked && backup.EPGLinked {
		out.EPGLinked = true
	}
	return out
}

func cloneChannel(ch catalog.LiveChannel) catalog.LiveChannel {
	out := ch
	if len(ch.StreamURLs) > 0 {
		out.StreamURLs = append([]string(nil), ch.StreamURLs...)
	}
	if len(ch.StreamAuths) > 0 {
		out.StreamAuths = append([]catalog.StreamAuth(nil), ch.StreamAuths...)
	}
	return out
}

func backupIdentity(ch catalog.LiveChannel) (string, BackupMatchStrategy, bool) {
	nameKey := normalizedBackupName(ch)
	if tvg := strings.ToLower(strings.TrimSpace(ch.TVGID)); tvg != "" {
		if nameKey != "" {
			return "tvg:" + tvg + "|name:" + nameKey, BackupMatchTVGID, true
		}
		return "tvg:" + tvg, BackupMatchTVGID, true
	}
	if dna := strings.ToLower(strings.TrimSpace(ch.DNAID)); dna != "" {
		if nameKey != "" {
			return "dna:" + dna + "|name:" + nameKey, BackupMatchDNAID, true
		}
		return "dna:" + dna, BackupMatchDNAID, true
	}
	return "", "", false
}

func normalizedBackupName(ch catalog.LiveChannel) string {
	raw := strings.ToLower(strings.TrimSpace(ch.GuideName))
	if raw == "" {
		raw = strings.ToLower(strings.TrimSpace(ch.TVGID))
	}
	if raw == "" {
		return ""
	}
	fields := strings.Fields(backupNameNoise.ReplaceAllString(raw, " "))
	if len(fields) == 0 {
		return ""
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		switch field {
		case "", "us", "ca", "hd", "uhd", "sd", "fhd", "4k", "raw", "fps", "60fps", "30fps":
			continue
		}
		out = append(out, field)
	}
	return strings.Join(out, " ")
}

func DescribeChannel(ch catalog.LiveChannel) FeedDescriptor {
	descriptor := FeedDescriptor{
		Region:      inferChannelRegion(ch),
		Category:    bucketDisplayLabel(ClassifyChannel(ch)),
		Source:      strings.ToUpper(strings.TrimSpace(ch.SourceTag)),
		QualityTags: inferQualityTags(ch),
		Variant:     inferVariantTag(ch),
	}
	parts := make([]string, 0, 3)
	if descriptor.Region != "" {
		parts = append(parts, descriptor.Region)
	}
	if descriptor.Category != "" {
		parts = append(parts, descriptor.Category)
	}
	if len(descriptor.QualityTags) > 0 {
		parts = append(parts, strings.Join(descriptor.QualityTags, " / "))
	}
	if len(parts) == 0 && descriptor.Source != "" {
		parts = append(parts, descriptor.Source)
	}
	descriptor.Label = strings.Join(parts, " | ")
	return descriptor
}

func DescribeChannels(channels []catalog.LiveChannel) map[string]FeedDescriptor {
	if len(channels) == 0 {
		return nil
	}
	out := make(map[string]FeedDescriptor, len(channels))
	for _, ch := range channels {
		channelID := strings.TrimSpace(ch.ChannelID)
		if channelID == "" {
			continue
		}
		out[channelID] = DescribeChannel(ch)
	}
	return out
}

func bucketDisplayLabel(bucket TaxonomyBucket) string {
	switch bucket {
	case BucketLocalBroadcast:
		return "LOCAL BROADCAST"
	case BucketGeneralEntertainment:
		return "ENTERTAINMENT"
	case BucketNewsInfo:
		return "NEWS & INFO"
	case BucketSports:
		return "SPORTS"
	case BucketLifestyleHome:
		return "LIFESTYLE & HOME"
	case BucketDocumentaryHistory:
		return "DOCUMENTARY & HISTORY"
	case BucketChildrenFamily:
		return "CHILDREN & FAMILY"
	case BucketRealitySpecialized:
		return "REALITY & SPECIALIZED"
	case BucketPremiumNetworks:
		return "PREMIUM NETWORKS"
	case BucketRegionalSports:
		return "REGIONAL SPORTS"
	case BucketReligious:
		return "RELIGIOUS"
	case BucketInternational:
		return "INTERNATIONAL"
	default:
		return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(string(bucket)), "_", " "))
	}
}

func inferChannelRegion(ch catalog.LiveChannel) string {
	tvg := strings.ToLower(strings.TrimSpace(ch.TVGID))
	switch {
	case strings.HasSuffix(tvg, ".us"):
		return "US"
	case strings.HasSuffix(tvg, ".ca"):
		return "CA"
	case strings.HasSuffix(tvg, ".uk"), strings.HasSuffix(tvg, ".gb"):
		return "UK"
	}
	hay := " " + normalizedBackupName(catalog.LiveChannel{
		GuideName:  strings.Join([]string{ch.GuideName, ch.GroupTitle, ch.SourceTag}, " "),
		TVGID:      ch.TVGID,
		SourceTag:  ch.SourceTag,
		GroupTitle: ch.GroupTitle,
	}) + " "
	switch {
	case strings.Contains(hay, " usa "), strings.Contains(hay, " united states "), strings.Contains(hay, " us "):
		return "US"
	case strings.Contains(hay, " canada "), strings.Contains(hay, " ca "):
		return "CA"
	case strings.Contains(hay, " uk "), strings.Contains(hay, " britain "), strings.Contains(hay, " england "):
		return "UK"
	}
	return ""
}

func inferQualityTags(ch catalog.LiveChannel) []string {
	raw := strings.ToLower(strings.Join([]string{
		strings.TrimSpace(ch.GuideName),
		strings.TrimSpace(ch.GroupTitle),
		strings.TrimSpace(ch.SourceTag),
		strings.TrimSpace(ch.TVGID),
	}, " "))
	raw = backupNameNoise.ReplaceAllString(raw, " ")
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil
	}
	joined := " " + strings.Join(fields, " ") + " "
	tags := make([]string, 0, 4)
	switch {
	case strings.Contains(joined, " 4k "), strings.Contains(joined, " uhd "), strings.Contains(joined, " 2160 "):
		tags = append(tags, "4K")
	case strings.Contains(joined, " fhd "), strings.Contains(joined, " 1080 "):
		tags = append(tags, "FHD")
	case strings.Contains(joined, " hd "), strings.Contains(joined, " 720 "):
		tags = append(tags, "HD")
	case strings.Contains(joined, " sd "):
		tags = append(tags, "SD")
	}
	if strings.Contains(joined, " raw ") {
		tags = append(tags, "RAW")
	}
	switch {
	case strings.Contains(joined, " 60fps "), strings.Contains(joined, " 60 fps "):
		tags = append(tags, "60 FPS")
	case strings.Contains(joined, " 50fps "), strings.Contains(joined, " 50 fps "):
		tags = append(tags, "50 FPS")
	case strings.Contains(joined, " 30fps "), strings.Contains(joined, " 30 fps "):
		tags = append(tags, "30 FPS")
	}
	return tags
}

func inferVariantTag(ch catalog.LiveChannel) string {
	joined := " " + normalizedBackupName(catalog.LiveChannel{GuideName: strings.Join([]string{ch.GuideName, ch.GroupTitle}, " ")}) + " "
	switch {
	case strings.Contains(joined, " east "):
		return "EAST"
	case strings.Contains(joined, " west "):
		return "WEST"
	case strings.Contains(joined, " plus "):
		return "PLUS"
	case strings.Contains(joined, " pacific "):
		return "PACIFIC"
	case strings.Contains(joined, " mountain "):
		return "MOUNTAIN"
	default:
		return ""
	}
}

func visibleStreamCount(ch catalog.LiveChannel) int {
	seen := map[string]struct{}{}
	for _, url := range append([]string{ch.StreamURL}, ch.StreamURLs...) {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		seen[url] = struct{}{}
	}
	return len(seen)
}

func containsString(in []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, v := range in {
		if strings.TrimSpace(v) == want {
			return true
		}
	}
	return false
}

func containsStreamAuth(in []catalog.StreamAuth, want catalog.StreamAuth) bool {
	for _, v := range in {
		if strings.TrimSpace(v.Prefix) == strings.TrimSpace(want.Prefix) &&
			strings.TrimSpace(v.User) == strings.TrimSpace(want.User) &&
			strings.TrimSpace(v.Pass) == strings.TrimSpace(want.Pass) {
			return true
		}
	}
	return false
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

func toSet(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out[v] = struct{}{}
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

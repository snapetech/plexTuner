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
	ChannelID   string `json:"channel_id"`
	GuideNumber string `json:"guide_number"`
	GuideName   string `json:"guide_name"`
	TVGID       string `json:"tvg_id,omitempty"`
	SourceTag   string `json:"source_tag,omitempty"`
	GroupTitle  string `json:"group_title,omitempty"`
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
		if _, drop := excluded[channelID]; drop {
			continue
		}
		keep := len(selected) == 0
		if !keep {
			categoryID, _, _ := CategoryIdentity(ch)
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
	if recipe.OrderMode == "custom" && len(recipe.CustomOrder) > 0 {
		filtered = applyCustomOrder(filtered, recipe.CustomOrder)
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

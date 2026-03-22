package programming

import (
	"path/filepath"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestBuildCategoryInventory(t *testing.T) {
	got := BuildCategoryInventory([]catalog.LiveChannel{
		{ChannelID: "1", GuideName: "DirecTV News", GroupTitle: "DirecTV", SourceTag: "iptv", TVGID: "news.one"},
		{ChannelID: "2", GuideName: "DirecTV Sports", GroupTitle: "DirecTV", SourceTag: "iptv"},
		{ChannelID: "3", GuideName: "Sling Local", GroupTitle: "Sling", SourceTag: "iptv", TVGID: "local.one"},
	})
	if len(got) != 2 {
		t.Fatalf("categories=%#v", got)
	}
	if got[0].ID != "iptv--directv" || got[0].Count != 2 || got[0].EPGLinkedCount != 1 {
		t.Fatalf("directv category=%#v", got[0])
	}
}

func TestApplyRecipe_CategoryIncludeExcludeAndCustomOrder(t *testing.T) {
	channels := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "One", GroupTitle: "DirecTV", SourceTag: "iptv"},
		{ChannelID: "2", GuideNumber: "102", GuideName: "Two", GroupTitle: "DirecTV", SourceTag: "iptv"},
		{ChannelID: "3", GuideNumber: "103", GuideName: "Three", GroupTitle: "Sling", SourceTag: "iptv"},
	}
	recipe := Recipe{
		SelectedCategories: []string{"iptv--directv"},
		IncludedChannelIDs: []string{"3"},
		ExcludedChannelIDs: []string{"2"},
		OrderMode:          "custom",
		CustomOrder:        []string{"3", "1"},
	}
	got := ApplyRecipe(channels, recipe)
	if len(got) != 2 {
		t.Fatalf("filtered=%#v", got)
	}
	if got[0].ChannelID != "3" || got[1].ChannelID != "1" {
		t.Fatalf("ordered=%#v", got)
	}
}

func TestApplyRecipe_ExcludedCategoriesAndRecommendedOrder(t *testing.T) {
	channels := []catalog.LiveChannel{
		{ChannelID: "sports", GuideNumber: "300", GuideName: "ESPN", GroupTitle: "Sports", SourceTag: "iptv"},
		{ChannelID: "news", GuideNumber: "200", GuideName: "CNN", GroupTitle: "News", SourceTag: "iptv"},
		{ChannelID: "local", GuideNumber: "100", GuideName: "NBC 4", GroupTitle: "Local", SourceTag: "iptv"},
		{ChannelID: "intl", GuideNumber: "900", GuideName: "TV5 Monde", GroupTitle: "French", SourceTag: "iptv", TVGID: "tv5.fr"},
	}
	recipe := Recipe{
		ExcludedCategories: []string{"iptv--french"},
		OrderMode:          "recommended",
		CustomOrder:        []string{"news"},
	}
	got := ApplyRecipe(channels, recipe)
	if len(got) != 3 {
		t.Fatalf("filtered=%#v", got)
	}
	if got[0].ChannelID != "local" || got[1].ChannelID != "news" || got[2].ChannelID != "sports" {
		t.Fatalf("recommended order=%#v", got)
	}
}

func TestUpdateRecipeMutations(t *testing.T) {
	recipe := UpdateRecipeCategories(Recipe{}, "include", []string{"cat-a", "cat-b"})
	recipe = UpdateRecipeCategories(recipe, "exclude", []string{"cat-c"})
	recipe = UpdateRecipeChannels(recipe, "include", []string{"ch1"})
	recipe = UpdateRecipeChannels(recipe, "exclude", []string{"ch2"})
	recipe = UpdateRecipeBackupPreferences(recipe, "prefer", []string{"ch2", "ch1"})
	if len(recipe.SelectedCategories) != 2 || len(recipe.ExcludedCategories) != 1 {
		t.Fatalf("category recipe=%#v", recipe)
	}
	if len(recipe.IncludedChannelIDs) != 1 || len(recipe.ExcludedChannelIDs) != 1 || len(recipe.PreferredBackupIDs) != 2 {
		t.Fatalf("channel recipe=%#v", recipe)
	}
	recipe = UpdateRecipeCategories(recipe, "remove", []string{"cat-b", "cat-c"})
	recipe = UpdateRecipeChannels(recipe, "clear", []string{"ch1", "ch2"})
	recipe = UpdateRecipeBackupPreferences(recipe, "remove", []string{"ch2"})
	if len(recipe.SelectedCategories) != 1 || len(recipe.ExcludedCategories) != 0 || len(recipe.IncludedChannelIDs) != 0 || len(recipe.ExcludedChannelIDs) != 0 || len(recipe.PreferredBackupIDs) != 1 {
		t.Fatalf("mutated recipe=%#v", recipe)
	}
}

func TestUpdateRecipeOrder(t *testing.T) {
	recipe := Recipe{OrderMode: "source", CustomOrder: []string{"2", "3"}}
	recipe = UpdateRecipeOrder(recipe, "prepend", []string{"1"}, "", "")
	recipe = UpdateRecipeOrder(recipe, "after", []string{"4"}, "", "2")
	recipe = UpdateRecipeOrder(recipe, "remove", []string{"3"}, "", "")
	if recipe.OrderMode != "custom" {
		t.Fatalf("order mode=%q", recipe.OrderMode)
	}
	want := []string{"1", "2", "4"}
	if len(recipe.CustomOrder) != len(want) {
		t.Fatalf("custom order=%v", recipe.CustomOrder)
	}
	for i := range want {
		if recipe.CustomOrder[i] != want[i] {
			t.Fatalf("custom order=%v want=%v", recipe.CustomOrder, want)
		}
	}
}

func TestLoadSaveRecipeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programming.json")
	saved, err := SaveRecipeFile(path, Recipe{
		SelectedCategories:   []string{"iptv--directv"},
		OrderMode:            "custom",
		CustomOrder:          []string{"2", "1"},
		CollapseExactBackups: true,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Version != RecipeVersion {
		t.Fatalf("version=%d", saved.Version)
	}
	loaded, err := LoadRecipeFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.OrderMode != "custom" || len(loaded.CustomOrder) != 2 {
		t.Fatalf("loaded=%#v", loaded)
	}
	if !loaded.CollapseExactBackups {
		t.Fatalf("loaded=%#v", loaded)
	}
}

func TestBuildBackupGroupsAndCollapse(t *testing.T) {
	channels := []catalog.LiveChannel{
		{ChannelID: "sling-syfy", DNAID: "dna-syfy", TVGID: "syfy.us", GuideNumber: "401", GuideName: "SyFy", SourceTag: "sling", StreamURL: "http://a/1", StreamURLs: []string{"http://a/1"}},
		{ChannelID: "directv-syfy", DNAID: "dna-syfy", TVGID: "syfy.us", GuideNumber: "5401", GuideName: "SyFy", SourceTag: "directv", StreamURL: "http://b/1", StreamURLs: []string{"http://b/1"}},
		{ChannelID: "cnn", DNAID: "dna-cnn", TVGID: "cnn.us", GuideNumber: "200", GuideName: "CNN", SourceTag: "iptv", StreamURL: "http://c/1"},
	}
	groups := BuildBackupGroups(channels)
	if len(groups) != 1 {
		t.Fatalf("groups=%#v", groups)
	}
	if groups[0].PrimaryID != "sling-syfy" || groups[0].BackupCount != 1 || groups[0].MatchStrategy != BackupMatchTVGID {
		t.Fatalf("group=%#v", groups[0])
	}
	collapsed := CollapseExactBackupGroups(channels)
	if len(collapsed) != 2 {
		t.Fatalf("collapsed=%#v", collapsed)
	}
	if collapsed[0].ChannelID != "sling-syfy" || len(collapsed[0].StreamURLs) != 2 {
		t.Fatalf("collapsed primary=%#v", collapsed[0])
	}
}

func TestBuildBackupGroupsAndCollapse_WithPreferences(t *testing.T) {
	channels := []catalog.LiveChannel{
		{ChannelID: "sling-syfy", DNAID: "dna-syfy", TVGID: "syfy.us", GuideNumber: "401", GuideName: "SyFy", SourceTag: "sling", StreamURL: "http://a/1", StreamURLs: []string{"http://a/1"}},
		{ChannelID: "directv-syfy", DNAID: "dna-syfy", TVGID: "syfy.us", GuideNumber: "5401", GuideName: "SyFy", SourceTag: "directv", StreamURL: "http://b/1", StreamURLs: []string{"http://b/1"}},
	}
	groups := BuildBackupGroupsWithPreferences(channels, []string{"directv-syfy"})
	if len(groups) != 1 || groups[0].PrimaryID != "directv-syfy" {
		t.Fatalf("groups=%#v", groups)
	}
	collapsed := CollapseExactBackupGroupsWithPreferences(channels, []string{"directv-syfy"})
	if len(collapsed) != 1 {
		t.Fatalf("collapsed=%#v", collapsed)
	}
	if collapsed[0].ChannelID != "directv-syfy" || collapsed[0].StreamURL != "http://b/1" || len(collapsed[0].StreamURLs) != 2 {
		t.Fatalf("collapsed primary=%#v", collapsed[0])
	}
}

func TestBuildBackupGroupsDoesNotCollapseVariantNames(t *testing.T) {
	channels := []catalog.LiveChannel{
		{ChannelID: "amc", DNAID: "dna-amc", TVGID: "amc.us", GuideNumber: "401", GuideName: "AMC HD", SourceTag: "iptv", StreamURL: "http://a/1"},
		{ChannelID: "amc-plus", DNAID: "dna-amc", TVGID: "amc.us", GuideNumber: "402", GuideName: "AMC Plus", SourceTag: "iptv", StreamURL: "http://b/1"},
		{ChannelID: "animal-east", DNAID: "dna-animal", TVGID: "animalplanet.us", GuideNumber: "501", GuideName: "Animal Planet HD", SourceTag: "iptv", StreamURL: "http://c/1"},
		{ChannelID: "animal-west", DNAID: "dna-animal", TVGID: "animalplanet.us", GuideNumber: "502", GuideName: "Animal Planet West HD", SourceTag: "iptv", StreamURL: "http://d/1"},
	}
	if groups := BuildBackupGroups(channels); len(groups) != 0 {
		t.Fatalf("groups=%#v want 0", groups)
	}
	if collapsed := CollapseExactBackupGroups(channels); len(collapsed) != 4 {
		t.Fatalf("collapsed=%#v want 4 rows", collapsed)
	}
}

func TestDescribeChannel(t *testing.T) {
	ch := catalog.LiveChannel{
		ChannelID:  "aspire",
		GuideName:  "US: ASPIRE HD RAW 60fps",
		GroupTitle: "Entertainment",
		SourceTag:  "strong8k",
		TVGID:      "AspireTV.us",
	}
	got := DescribeChannel(ch)
	if got.Region != "US" || got.Category != "ENTERTAINMENT" {
		t.Fatalf("descriptor=%+v", got)
	}
	if len(got.QualityTags) != 3 || got.QualityTags[0] != "HD" || got.QualityTags[1] != "RAW" || got.QualityTags[2] != "60 FPS" {
		t.Fatalf("quality=%+v", got)
	}
	if got.Label != "US | ENTERTAINMENT | HD / RAW / 60 FPS" {
		t.Fatalf("label=%q", got.Label)
	}
}

func TestApplyRecipePreviewDoesNotCollapseBackups(t *testing.T) {
	channels := []catalog.LiveChannel{
		{ChannelID: "a", DNAID: "dna", TVGID: "tvg", GuideNumber: "1", GuideName: "One", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://a/1"},
		{ChannelID: "b", DNAID: "dna", TVGID: "tvg", GuideNumber: "2", GuideName: "One", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://b/1"},
	}
	recipe := Recipe{CollapseExactBackups: true}
	if got := ApplyRecipe(channels, recipe); len(got) != 1 {
		t.Fatalf("collapsed len=%d want 1", len(got))
	}
	if got := ApplyRecipePreview(channels, recipe); len(got) != 2 {
		t.Fatalf("preview len=%d want 2", len(got))
	}
}

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

func TestLoadSaveRecipeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programming.json")
	saved, err := SaveRecipeFile(path, Recipe{
		SelectedCategories: []string{"iptv--directv"},
		OrderMode:          "custom",
		CustomOrder:        []string{"2", "1"},
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
}

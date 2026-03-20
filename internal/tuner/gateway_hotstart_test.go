package tuner

import (
	"os"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestMatchesHotStartGroupTitle(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		channel *catalog.LiveChannel
		want    bool
	}{
		{
			name:    "empty env",
			env:     "",
			channel: &catalog.LiveChannel{GroupTitle: "Sports"},
			want:    false,
		},
		{
			name:    "empty group title",
			env:     "Sports",
			channel: &catalog.LiveChannel{GroupTitle: ""},
			want:    false,
		},
		{
			name:    "nil channel",
			env:     "Sports",
			channel: nil,
			want:    false,
		},
		{
			name:    "substring case insensitive",
			env:     "sports",
			channel: &catalog.LiveChannel{GroupTitle: "US | Sports"},
			want:    true,
		},
		{
			name:    "first of multiple needles",
			env:     "news, sports",
			channel: &catalog.LiveChannel{GroupTitle: "Local News HD"},
			want:    true,
		},
		{
			name:    "no match",
			env:     "Sports",
			channel: &catalog.LiveChannel{GroupTitle: "Movies"},
			want:    false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { _ = os.Unsetenv("IPTV_TUNERR_HOT_START_GROUP_TITLES") })
			if tc.env != "" {
				t.Setenv("IPTV_TUNERR_HOT_START_GROUP_TITLES", tc.env)
			} else {
				_ = os.Unsetenv("IPTV_TUNERR_HOT_START_GROUP_TITLES")
			}
			if got := matchesHotStartGroupTitle(tc.channel); got != tc.want {
				t.Fatalf("matchesHotStartGroupTitle() = %v want %v", got, tc.want)
			}
		})
	}
}

func TestGateway_hotStartConfigFromGroupTitles(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HOT_START_ENABLED", "true")
	t.Setenv("IPTV_TUNERR_HOT_START_GROUP_TITLES", "Sports")
	_ = os.Unsetenv("IPTV_TUNERR_HOT_START_CHANNELS")

	g := &Gateway{}
	cfg := g.hotStartConfig(&catalog.LiveChannel{ChannelID: "x", GroupTitle: "US | Sports"}, "web")
	if !cfg.Enabled {
		t.Fatal("expected hot-start enabled")
	}
	if cfg.Reason != "group_title" {
		t.Fatalf("reason=%q want group_title", cfg.Reason)
	}
}

func TestGateway_hotStartConfigFavoriteBeatsGroupTitle(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HOT_START_ENABLED", "true")
	t.Setenv("IPTV_TUNERR_HOT_START_CHANNELS", "1001")
	t.Setenv("IPTV_TUNERR_HOT_START_GROUP_TITLES", "Sports")

	g := &Gateway{}
	cfg := g.hotStartConfig(&catalog.LiveChannel{ChannelID: "1001", GroupTitle: "Sports"}, "web")
	if !cfg.Enabled {
		t.Fatal("expected hot-start enabled")
	}
	if cfg.Reason != "favorite" {
		t.Fatalf("reason=%q want favorite (explicit list wins)", cfg.Reason)
	}
}

package iptvorg

import (
	"testing"
)

func TestNormName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CNN", "cnn"},
		{"CNN HD", "cnn hd"},
		{"US: CNN", "us cnn"},
		{"BBC News", "bbc news"},
		{"TV5 Monde", "tv5 monde"},
	}
	for _, c := range cases {
		if got := normName(c.in); got != c.want {
			t.Errorf("normName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStripForMatch(t *testing.T) {
	cases := []struct{ in, want string }{
		{"US: CNN HD", "cnn"},
		{"GO: ABC NEWS LIVE", "abc news live"},
		{"SLING: ESPN", "espn"},
		{"DE: ARD HD", "ard"},
		{"FOX NEWS", "fox news"},
	}
	for _, c := range cases {
		if got := stripForMatch(c.in); got != c.want {
			t.Errorf("stripForMatch(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestShortCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"cnn.us", "cnn"},
		{"bbc-news.uk", "bbc-news"},
		{"tbn.us", "tbn"},
		{"x", ""}, // too short
	}
	for _, c := range cases {
		if got := shortCode(c.in); got != c.want {
			t.Errorf("shortCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEnrichTVGID(t *testing.T) {
	db := &DB{
		Channels: []Channel{
			{ID: "cnn.us", Name: "CNN", AltNames: []string{"Cable News Network"}, Country: "US"},
			{ID: "bbc-news.uk", Name: "BBC News", AltNames: nil, Country: "GB"},
		},
	}
	db.buildIndices()

	cases := []struct {
		tvgID, name, wantID, wantMethod string
	}{
		{"", "CNN", "cnn.us", "iptvorg_name_exact"},
		{"", "US: CNN HD", "cnn.us", "iptvorg_name_stripped"},
		{"cnn.us", "CNN", "cnn.us", "iptvorg_name_exact"},
		{"", "BBC News", "bbc-news.uk", "iptvorg_name_exact"},
		{"", "GO: CNN", "cnn.us", "iptvorg_name_stripped"},
		{"", "Unknown Channel XYZ", "", ""},
	}
	for _, c := range cases {
		gotID, gotMethod := db.EnrichTVGID(c.tvgID, c.name)
		if gotID != c.wantID || gotMethod != c.wantMethod {
			t.Errorf("EnrichTVGID(%q, %q) = (%q, %q), want (%q, %q)",
				c.tvgID, c.name, gotID, gotMethod, c.wantID, c.wantMethod)
		}
	}
}

package gracenote

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestNormaliseCallSign(t *testing.T) {
	cases := []struct{ in, want string }{
		{"TSN1HD", "tsn1"},
		{"CBKTDT", "cbkt"},
		{"TSN1.ca", "tsn1"},
		{"CBKT.ca", "cbkt"},
		{"HBOHD", "hbo"}, // strip trailing HD → hbo (HBO is a valid channel base)
		{"CBC", "cbc"},
		{"CNN", "cnn"},
		{"", ""},
	}
	for _, c := range cases {
		got := normaliseCallSign(c.in)
		if got != c.want {
			t.Errorf("normaliseCallSign(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLookupByCallSign(t *testing.T) {
	db := &DB{
		Channels: []Channel{
			{GridKey: "aaa", CallSign: "TSN1HD", Title: "TSN 1"},
			{GridKey: "bbb", CallSign: "CBKTDT", Title: "CBC Saskatchewan"},
			{GridKey: "ccc", CallSign: "CNN", Title: "CNN"},
		},
	}
	db.buildIndices()

	cases := []struct{ query, wantKey string }{
		{"TSN1HD", "aaa"},
		{"TSN1.ca", "aaa"}, // .ca stripped → tsn1
		{"cbktdt", "bbb"},
		{"CBKT.ca", "bbb"},
		{"CNN", "ccc"},
		{"MSNBC", ""},
	}
	for _, c := range cases {
		ch := db.LookupByCallSign(c.query)
		got := ""
		if ch != nil {
			got = ch.GridKey
		}
		if got != c.wantKey {
			t.Errorf("LookupByCallSign(%q): got gridKey %q, want %q", c.query, got, c.wantKey)
		}
	}
}

func TestEnrichTVGID(t *testing.T) {
	db := &DB{
		Channels: []Channel{
			{GridKey: "abc123", CallSign: "TSN1HD", Title: "TSN 1"},
			{GridKey: "def456", CallSign: "CBKTDT", Title: "CBC Saskatchewan"},
		},
	}
	db.buildIndices()

	cases := []struct {
		tvgID, name string
		wantKey     string
		wantMethod  string
	}{
		{"abc123", "", "abc123", "gracenote_gridkey_exact"},
		{"TSN1HD", "TSN 1", "abc123", "gracenote_callsign"},
		{"TSN1.ca", "", "abc123", "gracenote_callsign"},
		{"", "CBC Saskatchewan", "def456", "gracenote_title"},
		{"UNKNOWN", "No Match", "", ""},
	}
	for _, c := range cases {
		gotKey, gotMethod := db.EnrichTVGID(c.tvgID, c.name)
		if gotKey != c.wantKey || gotMethod != c.wantMethod {
			t.Errorf("EnrichTVGID(%q, %q): got (%q, %q), want (%q, %q)",
				c.tvgID, c.name, gotKey, gotMethod, c.wantKey, c.wantMethod)
		}
	}
}

func TestMergeDedup(t *testing.T) {
	db := &DB{}
	db.buildIndices()
	n := db.Merge([]Channel{
		{GridKey: "aaa", CallSign: "TSN1HD", Title: "TSN 1"},
		{GridKey: "bbb", CallSign: "CBKTDT", Title: "CBC"},
	})
	if n != 2 {
		t.Fatalf("Merge initial: want 2 added, got %d", n)
	}
	// Merge same again — should add 0.
	n2 := db.Merge([]Channel{
		{GridKey: "aaa", CallSign: "TSN1HD", Title: "TSN 1"},
		{GridKey: "ccc", CallSign: "CNN", Title: "CNN"},
	})
	if n2 != 1 {
		t.Fatalf("Merge dedup: want 1 added, got %d", n2)
	}
	if db.Len() != 3 {
		t.Fatalf("Len after merges: want 3, got %d", db.Len())
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gn.json")

	db := &DB{
		Channels: []Channel{
			{GridKey: "abc123", CallSign: "TSN1HD", Title: "TSN 1", Language: "en"},
		},
	}
	db.buildIndices()
	if err := db.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	db2, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if db2.Len() != 1 {
		t.Fatalf("Load: want 1 channel, got %d", db2.Len())
	}
	if db2.LookupByCallSign("TSN1HD") == nil {
		t.Error("LookupByCallSign after Load: want match, got nil")
	}
}

func TestLoadMissingFile(t *testing.T) {
	db, err := Load("/tmp/does_not_exist_gracenote_test.json")
	if err != nil {
		t.Fatalf("Load missing: want nil error, got %v", err)
	}
	if db.Len() != 0 {
		t.Errorf("Load missing: want 0 channels, got %d", db.Len())
	}
}

func TestJSONRoundtrip(t *testing.T) {
	ch := Channel{GridKey: "abc", CallSign: "CBC", Title: "CBC", Language: "en", IsHD: true}
	data, _ := json.Marshal(ch)
	var got Channel
	json.Unmarshal(data, &got)
	if got != ch {
		t.Errorf("JSON roundtrip: got %+v, want %+v", got, ch)
	}
}

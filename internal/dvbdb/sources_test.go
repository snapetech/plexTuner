package dvbdb

import (
	"strings"
	"testing"
)

// ── Enigma2 lamedb ────────────────────────────────────────────────────────────

const lamedbV4Sample = `eDVB services /4/
transponders
00820000:2af8:013e
	s 10719000:27500000:1:3:130:2:0:1:2:2:2
/
end
services
1133:00820000:2af8:013e:1:2000
BBC FIRST
p:CANAL+,c:0000a2,c:011fff
1134:00820000:2af8:013e:1:2001
BBC Earth HD
p:SKY,c:0000a3
0001:00010002:0003:0004:1:0
No Provider
end
`

func TestParseLamedb_Basic(t *testing.T) {
	db := New()
	added, total, err := ParseLamedbReader(db, strings.NewReader(lamedbV4Sample), "test")
	if err != nil {
		t.Fatalf("ParseLamedbReader error: %v", err)
	}
	if added < 3 {
		t.Errorf("want ≥3 added, got %d (total=%d)", added, total)
	}

	// BBC FIRST: SID=0x1133, TSID=0x2af8, ONID=0x013e
	e := db.LookupTriplet(0x013e, 0x2af8, 0x1133)
	if e == nil {
		t.Fatal("BBC FIRST not found by triplet")
	}
	if e.Name != "BBC FIRST" {
		t.Errorf("want name BBC FIRST, got %q", e.Name)
	}
	if e.NetworkName != "CANAL+" {
		t.Errorf("want provider CANAL+, got %q", e.NetworkName)
	}

	// BBC Earth HD
	e2 := db.LookupTriplet(0x013e, 0x2af8, 0x1134)
	if e2 == nil {
		t.Fatal("BBC Earth HD not found")
	}
	if e2.NetworkName != "SKY" {
		t.Errorf("want provider SKY, got %q", e2.NetworkName)
	}

	// No Provider
	e3 := db.LookupTriplet(0x0004, 0x0003, 0x0001)
	if e3 == nil {
		t.Fatal("No Provider entry not found")
	}
	if e3.Name != "No Provider" {
		t.Errorf("want No Provider, got %q", e3.Name)
	}
}

func TestParseLamedb_MissingServicesSection(t *testing.T) {
	db := New()
	_, _, err := ParseLamedbReader(db, strings.NewReader("eDVB services /4/\ntransponders\nend\n"), "test")
	if err == nil {
		t.Error("expected error for missing services section")
	}
}

func TestParseLamedb_NotLamedb(t *testing.T) {
	db := New()
	_, _, err := ParseLamedbReader(db, strings.NewReader("# This is not a lamedb\nsome: data\n"), "test")
	if err == nil {
		t.Error("expected error for non-lamedb file")
	}
}

func TestParseLamedb_EmptyFile(t *testing.T) {
	db := New()
	_, _, err := ParseLamedbReader(db, strings.NewReader(""), "test")
	if err == nil {
		t.Error("expected error for empty file")
	}
}

// ── VDR channels.conf ─────────────────────────────────────────────────────────

// Realistic VDR channels.conf lines.
// Format: Name;Provider:Freq:Params:Source:SymRate:VPID:APID:TPID:CAID:SID:NID:TID:RID
const vdrSample = `# VDR channel list
:Astra 28.2E
BBC ONE;BBC:10714:hC56M2O0S0:S28.2E:22000:2360:2361=eng,2362=eng:2366:0:6301:2:2045:0
BBC TWO;BBC:10714:hC56M2O0S0:S28.2E:22000:2362:2363=eng:2366:0:6302:2:2045:0
ITV1;ITV:10714:hC56M2O0S0:S28.2E:22000:2410:2411=eng:2466:0:6320:2:2045:0
:Hot Bird 13E
RAI 1;RAI:11034:hC34M2O0S0:S13.0E:27500:160:80=ita,1281=ita:0:0:1:318:12641:0
`

func TestParseVDRChannels_Basic(t *testing.T) {
	db := New()
	added, _, err := ParseVDRChannelsReader(db, strings.NewReader(vdrSample), "test")
	if err != nil {
		t.Fatalf("ParseVDRChannelsReader error: %v", err)
	}
	if added < 4 {
		t.Errorf("want ≥4 added, got %d", added)
	}

	// BBC ONE: SID=6301, NID(ONID)=2, TID(TSID)=2045
	e := db.LookupTriplet(2, 2045, 6301)
	if e == nil {
		t.Fatal("BBC ONE not found by triplet")
	}
	if e.Name != "BBC ONE" {
		t.Errorf("want BBC ONE, got %q", e.Name)
	}
	if e.NetworkName != "BBC" {
		t.Errorf("want provider BBC, got %q", e.NetworkName)
	}

	// RAI 1: SID=1, NID=318, TID=12641
	e2 := db.LookupTriplet(318, 12641, 1)
	if e2 == nil {
		t.Fatal("RAI 1 not found")
	}
	if e2.Name != "RAI 1" {
		t.Errorf("want RAI 1, got %q", e2.Name)
	}
}

func TestParseVDRChannels_SkipsCommentAndGroupLines(t *testing.T) {
	db := New()
	input := "# comment\n:Group Header\nBBC ONE;BBC:10714:h:S28.2E:22000:100:101:0:0:6301:2:2045:0\n"
	added, _, err := ParseVDRChannelsReader(db, strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if added != 1 {
		t.Errorf("want 1 added, got %d", added)
	}
}

func TestParseVDRChannels_TooFewFields(t *testing.T) {
	db := New()
	// Only 10 fields — should be skipped, not crash
	input := "BBC:10714:h:S28.2E:22000:100:101:0:0:6301\n"
	before := db.Len()
	_, _, err := ParseVDRChannelsReader(db, strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db.Len() != before {
		t.Errorf("should not have added any entry for short line")
	}
}

// ── TvHeadend channel JSON ────────────────────────────────────────────────────

const tvhEntriesJSON = `{"entries":[
  {"name":"CNN International","dvb_service_id":1234,"dvb_network_id":318,"dvb_transport_stream_id":2045,"dvb_provider":"Turner","country":"US","xmltv_import_checks":"cnn.us"},
  {"name":"BBC World News","dvb_service_id":5678,"dvb_network_id":2,"dvb_transport_stream_id":2045,"dvb_provider":"BBC"},
  {"name":"NoIDs"}
],"total":3}`

const tvhBareArrayJSON = `[
  {"name":"Sky News","dvb_service_id":999,"dvb_network_id":9018,"dvb_transport_stream_id":4101}
]`

func TestParseTvheadendJSON_Wrapper(t *testing.T) {
	db := New()
	added, _, err := ParseTvheadendJSON(db, []byte(tvhEntriesJSON), "test")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// "NoIDs" has no triplet so should be skipped
	if added != 2 {
		t.Errorf("want 2 added, got %d", added)
	}
	e := db.LookupTriplet(318, 2045, 1234)
	if e == nil {
		t.Fatal("CNN not found")
	}
	if e.Name != "CNN International" {
		t.Errorf("want CNN International, got %q", e.Name)
	}
	if e.TVGID != "cnn.us" {
		t.Errorf("want tvg-id cnn.us, got %q", e.TVGID)
	}
}

func TestParseTvheadendJSON_BareArray(t *testing.T) {
	db := New()
	added, _, err := ParseTvheadendJSON(db, []byte(tvhBareArrayJSON), "test")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if added != 1 {
		t.Errorf("want 1 added, got %d", added)
	}
	e := db.LookupTriplet(9018, 4101, 999)
	if e == nil {
		t.Fatal("Sky News not found")
	}
}

func TestParseTvheadendJSON_Invalid(t *testing.T) {
	db := New()
	_, _, err := ParseTvheadendJSON(db, []byte("not json at all {{{"), "test")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ── splitPaths (main.go helper, tested via dvbdb indirectly) ─────────────────

func TestSplitPathsHelper(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"/a/b", []string{"/a/b"}},
		{"/a/b,/c/d", []string{"/a/b", "/c/d"}},
		{" /a , /b , ", []string{"/a", "/b"}},
	}
	for _, tc := range cases {
		got := splitPathsTest(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitPaths(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitPaths(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

// splitPathsTest is a local copy so we can test the logic without importing main.
func splitPathsTest(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ── EnrichTVGID integration ───────────────────────────────────────────────────

func TestEnrichTVGID_FromLamedb(t *testing.T) {
	db := New()
	_, _, err := ParseLamedbReader(db, strings.NewReader(lamedbV4Sample), "test")
	if err != nil {
		t.Fatal(err)
	}

	// Should find via triplet (ONID=0x013e=318, TSID=0x2af8=11000, SID=0x1133=4403)
	tvgID, method := db.EnrichTVGID(0x013e, 0x2af8, 0x1133, "BBC FIRST")
	if tvgID == "" {
		t.Error("expected non-empty tvgID from lamedb triplet")
	}
	if method == "" {
		t.Error("expected non-empty method")
	}
}

func TestEnrichTVGID_FromVDR(t *testing.T) {
	db := New()
	_, _, err := ParseVDRChannelsReader(db, strings.NewReader(vdrSample), "test")
	if err != nil {
		t.Fatal(err)
	}

	// BBC ONE: ONID=2, TSID=2045, SID=6301
	tvgID, _ := db.EnrichTVGID(2, 2045, 6301, "BBC ONE")
	if tvgID == "" {
		t.Error("expected tvgID from VDR triplet")
	}
}

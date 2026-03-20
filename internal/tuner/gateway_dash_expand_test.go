package tuner

import (
	"strings"
	"testing"
)

func TestParseXsdDurationSeconds(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"PT12S", 12, true},
		{"PT1H2M3S", 3600 + 120 + 3, true},
		{"P1D", 86400, true},
		{"PT30.5S", 30.5, true},
		{"", 0, false},
		{"nope", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseXsdDurationSeconds(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseXsdDurationSeconds(%q) = (%v,%v) want (%v,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestExpandDASHSegmentTemplatesToSegmentList_uniformTemplate(t *testing.T) {
	mpd := `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" mediaPresentationDuration="PT12S" type="static">
<Period duration="PT12S">
<AdaptationSet>
<Representation id="v1" bandwidth="500000">
<SegmentTemplate timescale="1" duration="6" startNumber="1" media="https://cdn.example/a-$Number$.m4s" initialization="https://cdn.example/init.mp4"/>
</Representation>
</AdaptationSet>
</Period>
</MPD>`
	out := string(expandDASHSegmentTemplatesToSegmentList([]byte(mpd)))
	if !strings.Contains(out, "<SegmentList") {
		t.Fatalf("expected SegmentList: %q", out)
	}
	if !strings.Contains(out, `media="https://cdn.example/a-1.m4s"`) || !strings.Contains(out, `media="https://cdn.example/a-2.m4s"`) {
		t.Fatalf("expected two explicit segment URLs: %q", out)
	}
	if strings.Contains(out, "<SegmentTemplate") {
		t.Fatalf("template should be replaced: %q", out)
	}
}

func TestExpandDASHSegmentTemplatesToSegmentList_skipsTimePlaceholder(t *testing.T) {
	mpd := `<MPD mediaPresentationDuration="PT12S"><Period duration="PT12S"><Representation id="x"><SegmentTemplate duration="6" timescale="1" media="s-$Number$-$Time$.mp4"/></Representation></Period></MPD>`
	out := string(expandDASHSegmentTemplatesToSegmentList([]byte(mpd)))
	if strings.Contains(out, "<SegmentList") {
		t.Fatalf("should not expand when $Time$ is used: %q", out)
	}
}

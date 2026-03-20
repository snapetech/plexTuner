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
		t.Fatalf("should not expand when $Time$ is used without SegmentTimeline: %q", out)
	}
}

func TestExpandDASHSegmentTemplatesToSegmentList_pairedClosingTag(t *testing.T) {
	mpd := `<MPD mediaPresentationDuration="PT12S"><Period duration="PT12S"><Representation id="v1" bandwidth="1">
<SegmentTemplate timescale="1" duration="6" startNumber="1" media="https://cdn.example/a-$Number$.m4s" initialization="https://cdn.example/init.mp4">
</SegmentTemplate></Representation></Period></MPD>`
	out := string(expandDASHSegmentTemplatesToSegmentList([]byte(mpd)))
	if !strings.Contains(out, "<SegmentList") || !strings.Contains(out, "a-1.m4s") {
		t.Fatalf("expected paired-tag template expanded: %q", out)
	}
}

func TestExpandDASHSegmentTemplatesToSegmentList_segmentTimelineWithTime(t *testing.T) {
	mpd := `<MPD><Period duration="PT10S"><Representation id="x" bandwidth="1"><SegmentTemplate timescale="1" startNumber="1" media="https://cdn.example/seg-$Number$-t$Time$.m4s"><SegmentTimeline><S t="0" d="5"/><S d="5"/></SegmentTimeline></SegmentTemplate></Representation></Period></MPD>`
	out := string(expandDASHSegmentTemplatesToSegmentList([]byte(mpd)))
	if !strings.Contains(out, "<SegmentList") || !strings.Contains(out, "<SegmentTimeline") {
		t.Fatalf("expected SegmentList with timeline: %q", out)
	}
	if !strings.Contains(out, "t0.m4s") || !strings.Contains(out, "t5.m4s") {
		t.Fatalf("expected $Time$ substitution: %q", out)
	}
	if strings.Contains(out, "$Time$") || strings.Contains(out, "$Number$") {
		t.Fatalf("placeholders should be replaced: %q", out)
	}
}

func TestExpandDASHSegmentTemplatesToSegmentList_paddedNumber(t *testing.T) {
	mpd := `<MPD mediaPresentationDuration="PT10S"><Period duration="PT10S"><Representation id="x"><SegmentTemplate timescale="1" duration="5" startNumber="1" media="https://cdn.example/a-$Number%05d$.m4s"/></Representation></Period></MPD>`
	out := string(expandDASHSegmentTemplatesToSegmentList([]byte(mpd)))
	if !strings.Contains(out, `media="https://cdn.example/a-00001.m4s"`) || !strings.Contains(out, `media="https://cdn.example/a-00002.m4s"`) {
		t.Fatalf("expected zero-padded segment indices: %q", out)
	}
}

func TestDashSubstituteNumberTemplate(t *testing.T) {
	if got := dashSubstituteNumberTemplate(`x-$Number%05d$-y`, 7); got != "x-00007-y" {
		t.Fatalf("got %q", got)
	}
	if got := dashSubstituteNumberTemplate(`$Number$`, 42); got != "42" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandDASHSegmentTemplatesToSegmentList_segmentTimelineSPairedTags(t *testing.T) {
	mpd := `<MPD><Period duration="PT10S"><Representation id="x" bandwidth="1"><SegmentTemplate timescale="1" startNumber="1" media="https://cdn.example/s-$Number$.mp4"><SegmentTimeline>
<S t="0" d="5"></S>
<S d="5"></S>
</SegmentTimeline></SegmentTemplate></Representation></Period></MPD>`
	out := string(expandDASHSegmentTemplatesToSegmentList([]byte(mpd)))
	if !strings.Contains(out, "s-1.mp4") || !strings.Contains(out, "s-2.mp4") {
		t.Fatalf("expected two SegmentURLs from paired <S></S> timeline: %q", out)
	}
	if strings.Contains(out, "$Number$") {
		t.Fatalf("unexpected placeholder: %q", out)
	}
}

func TestExpandDASHSegmentTemplatesToSegmentList_segmentTimelineSWithInnerContent(t *testing.T) {
	mpd := `<MPD><Period duration="PT10S"><Representation id="x" bandwidth="1"><SegmentTemplate timescale="1" startNumber="1" media="https://cdn.example/s-$Number$.mp4"><SegmentTimeline>
<S t="0" d="5"><!-- seg --></S>
<S d="5">whitespace and text ignored</S>
</SegmentTimeline></SegmentTemplate></Representation></Period></MPD>`
	out := string(expandDASHSegmentTemplatesToSegmentList([]byte(mpd)))
	if !strings.Contains(out, "s-1.mp4") || !strings.Contains(out, "s-2.mp4") {
		t.Fatalf("expected expansion with non-empty <S> bodies: %q", out)
	}
}

func TestExpandDASHSegmentTemplatesToSegmentList_segmentTimelineNestedS(t *testing.T) {
	// Invalid DASH (S should not nest); outer d applies as one segment, inner S is not a separate row.
	mpd := `<MPD><Period duration="PT20S"><Representation id="x" bandwidth="1"><SegmentTemplate timescale="1" startNumber="1" media="https://cdn.example/s-$Number$.mp4"><SegmentTimeline>
<S t="0" d="10"><S d="3"/></S>
<S d="5"/>
</SegmentTimeline></SegmentTemplate></Representation></Period></MPD>`
	out := string(expandDASHSegmentTemplatesToSegmentList([]byte(mpd)))
	if !strings.Contains(out, "s-1.mp4") || !strings.Contains(out, "s-2.mp4") {
		t.Fatalf("expected two top-level S rows after balanced parse: %q", out)
	}
	if strings.Contains(out, "s-3.mp4") {
		t.Fatalf("nested <S> must not add an extra segment row: %q", out)
	}
}

func TestDashConsumeSTag_quotedGreaterThanInAttribute(t *testing.T) {
	s := `<S d="5" note="a>b"/>`
	attrs, after, ok := dashConsumeSTag(s, 0)
	if !ok || after != len(s) {
		t.Fatalf("ok=%v after=%d want %d attrs=%q", ok, after, len(s), attrs)
	}
	if !strings.Contains(attrs, `note="a>b"`) && !strings.Contains(attrs, "a>b") {
		t.Fatalf("attrs %q", attrs)
	}
}

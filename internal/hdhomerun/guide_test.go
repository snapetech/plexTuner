package hdhomerun

import (
	"context"
	"strings"
	"testing"
)

func TestAnalyzeGuideXMLStats_sample(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?>
<tv>
  <channel id="1"><display-name>A</display-name></channel>
  <channel id="2"><display-name>B</display-name></channel>
  <programme start="20200101000000" channel="1"><title>X</title></programme>
</tv>`)
	st, err := AnalyzeGuideXMLStats(raw)
	if err != nil {
		t.Fatal(err)
	}
	if st.ChannelTags != 2 || st.Programmes != 1 {
		t.Fatalf("got %+v", st)
	}
}

func TestGuideURLFromBase(t *testing.T) {
	if g := GuideURLFromBase("http://192.168.1.10"); g != "http://192.168.1.10/guide.xml" {
		t.Fatalf("%q", g)
	}
	if g := GuideURLFromBase("http://x/guide.xml"); g != "http://x/guide.xml" {
		t.Fatalf("%q", g)
	}
	if g := GuideURLFromBase("   "); g != "" {
		t.Fatalf("%q", g)
	}
}

func TestFetchGuideXMLRejectsEmptyBaseURL(t *testing.T) {
	_, err := FetchGuideXML(context.Background(), nil, "   ")
	if err == nil || !strings.Contains(err.Error(), "base url required") {
		t.Fatalf("err=%v", err)
	}
}

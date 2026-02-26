package tuner

import (
	"strings"
	"testing"
)

func TestParsePlexLiveSessionRowsFiltersAndParses(t *testing.T) {
	xmlDoc := `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="2">
  <Video title="Live A" key="/livetv/sessions/live-1">
    <Player address="192.168.1.10" product="Plex Web" platform="Chrome" device="Linux" machineIdentifier="m1" state="playing"/>
    <Session id="sess-1"/>
    <TranscodeSession key="/transcode/sessions/tx-1" timeStamp="123.4" maxOffsetAvailable="17.5" minOffsetAvailable="1.0"/>
  </Video>
  <Video title="Not Live" key="/library/metadata/123">
    <Player address="192.168.1.11" machineIdentifier="m2" state="playing"/>
  </Video>
</MediaContainer>`

	rows, err := parsePlexLiveSessionRows(strings.NewReader(xmlDoc), "", "")
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len=%d want 1", len(rows))
	}
	r := rows[0]
	if r.LiveKey != "/livetv/sessions/live-1" || r.TranscodeID != "tx-1" || r.MachineID != "m1" {
		t.Fatalf("unexpected row: %+v", r)
	}
	if r.MaxOffsetAvail != 17.5 || r.MinOffsetAvail != 1.0 {
		t.Fatalf("offset parse failed: %+v", r)
	}

	rows, err = parsePlexLiveSessionRows(strings.NewReader(xmlDoc), "m1", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("machine filter failed rows=%d err=%v", len(rows), err)
	}
	rows, err = parsePlexLiveSessionRows(strings.NewReader(xmlDoc), "nope", "")
	if err != nil {
		t.Fatalf("machine miss parse err: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("machine miss rows=%d want 0", len(rows))
	}
}

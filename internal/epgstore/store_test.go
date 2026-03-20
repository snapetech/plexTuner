package epgstore

import (
	"path/filepath"
	"testing"
	"time"
)

func TestOpen_migrateAndVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "epg", "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if s.SchemaVersion() != 2 {
		t.Fatalf("schema version: %d want 2", s.SchemaVersion())
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = s2.Close()
}

func TestOpen_emptyPath(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestSyncMergedGuideXML_retainPrune(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	// One programme ended long ago, one in the future.
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="c1"><display-name>One</display-name></channel>
  <programme start="20200101000000 +0000" stop="20200101010000 +0000" channel="c1">
    <title>Old</title>
  </programme>
  <programme start="20350101000000 +0000" stop="20350101010000 +0000" channel="c1">
    <title>Future</title>
  </programme>
</tv>`
	pruned, err := s.SyncMergedGuideXML([]byte(xml), 24)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Fatalf("pruned=%d want 1", pruned)
	}
	p, _, err := s.RowCounts()
	if err != nil {
		t.Fatal(err)
	}
	if p != 1 {
		t.Fatalf("remaining programmes=%d want 1", p)
	}
}

func TestSyncMergedGuideXML_roundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "g.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="c1"><display-name>One</display-name></channel>
  <programme start="20260319100000 +0000" stop="20260319110000 +0000" channel="c1">
    <title>News</title>
    <category>News</category>
  </programme>
</tv>`
	if _, err := s.SyncMergedGuideXML([]byte(xml), 0); err != nil {
		t.Fatal(err)
	}
	p, c, err := s.RowCounts()
	if err != nil {
		t.Fatal(err)
	}
	if p != 1 || c != 1 {
		t.Fatalf("counts prog=%d ch=%d", p, c)
	}
	g, err := s.GlobalMaxStopUnix()
	if err != nil {
		t.Fatal(err)
	}
	wantStop := int64(time.Date(2026, 3, 19, 11, 0, 0, 0, time.UTC).Unix())
	if g != wantStop {
		t.Fatalf("global max stop: %d want %d", g, wantStop)
	}
	m, err := s.MaxStopUnixPerChannel()
	if err != nil {
		t.Fatal(err)
	}
	if m["c1"] != wantStop {
		t.Fatalf("per-ch max: %+v", m)
	}
	ls, err := s.MetaLastSyncUTC()
	if err != nil || ls == "" {
		t.Fatalf("last sync: %q err=%v", ls, err)
	}
}

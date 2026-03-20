package epgstore

import (
	"path/filepath"
	"testing"
)

func TestEnforceMaxDBBytes_noopWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	n, err := s.EnforceMaxDBBytes(0)
	if err != nil || n != 0 {
		t.Fatalf("got n=%d err=%v", n, err)
	}
}

func TestEnforceMaxDBBytes_trimsWhenHuge(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "q2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="c1"><display-name>One</display-name></channel>
  <programme start="20200101000000 +0000" stop="20200101010000 +0000" channel="c1"><title>Old</title></programme>
  <programme start="20350101000000 +0000" stop="20350101010000 +0000" channel="c1"><title>Future</title></programme>
</tv>`
	if _, err := s.SyncMergedGuideXML([]byte(xml), 0); err != nil {
		t.Fatal(err)
	}
	sz1, _, _ := s.DBFileStat()
	if sz1 <= 0 {
		t.Fatalf("unexpected size %d", sz1)
	}
	// Impossibly small cap — should delete programmes until sqlite shrinks (best-effort).
	n, err := s.EnforceMaxDBBytes(1024)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected some deletes, got %d", n)
	}
	p, _, err := s.RowCounts()
	if err != nil {
		t.Fatal(err)
	}
	if p > 1 {
		t.Fatalf("expected at most 1 programme left, got %d", p)
	}
}

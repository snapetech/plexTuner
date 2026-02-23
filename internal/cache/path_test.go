package cache

import (
	"path/filepath"
	"testing"
)

func TestPath_stable(t *testing.T) {
	p1 := Path("/cache", "movie_abc123")
	p2 := Path("/cache", "movie_abc123")
	if p1 != p2 {
		t.Errorf("Path should be stable: %q vs %q", p1, p2)
	}
}

func TestPath_sanitized(t *testing.T) {
	p := Path("/cache", "id/with/slash")
	if filepath.Base(p) != "id_with_slash.mp4" {
		t.Errorf("slashes should be sanitized: %s", p)
	}
}

func TestPartialPath(t *testing.T) {
	pp := PartialPath("/cache", "x")
	if pp == Path("/cache", "x") {
		t.Error("PartialPath should differ from Path")
	}
	if filepath.Ext(pp) != ".partial" {
		t.Errorf("ext: %s", filepath.Ext(pp))
	}
}

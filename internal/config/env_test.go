package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile_missing(t *testing.T) {
	err := LoadEnvFile(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatalf("missing file should return nil: %v", err)
	}
}

func TestLoadEnvFile_setsEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=bar\n# comment\nBAZ=quux\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FOO") != "bar" {
		t.Errorf("FOO = %q", os.Getenv("FOO"))
	}
	if os.Getenv("BAZ") != "quux" {
		t.Errorf("BAZ = %q", os.Getenv("BAZ"))
	}
}

func TestLoadEnvFile_unquote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(`X="hello world"`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("X") != "hello world" {
		t.Errorf("X = %q", os.Getenv("X"))
	}
}

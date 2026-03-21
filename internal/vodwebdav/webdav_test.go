package vodwebdav

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/vodfs"
)

type testMat struct{ path string }

func (t testMat) Materialize(context.Context, string, string) (string, error) { return t.path, nil }

func TestDAVFSStatAndReadMovie(t *testing.T) {
	tmp := t.TempDir()
	local := filepath.Join(tmp, "movie.mp4")
	if err := os.WriteFile(local, []byte("movie-bytes"), 0o600); err != nil {
		t.Fatalf("write local movie: %v", err)
	}
	fs := &davFS{
		tree: NewTestTree(),
		mat:  testMat{path: local},
	}
	info, err := fs.Stat(context.Background(), "/Movies")
	if err != nil {
		t.Fatalf("stat movies: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected /Movies to be a directory")
	}
	file, err := fs.OpenFile(context.Background(), "/Movies/Live: Movie (2024)/Live: Movie (2024).mp4", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open movie: %v", err)
	}
	defer file.Close()
	buf := make([]byte, 32)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read movie: %v", err)
	}
	if got := string(buf[:n]); got != "movie-bytes" {
		t.Fatalf("unexpected movie bytes %q", got)
	}
}

func TestMountHint(t *testing.T) {
	if got := MountHint("darwin", "127.0.0.1:8123"); got == "" {
		t.Fatal("expected darwin mount hint")
	}
	if got := MountHint("windows", "127.0.0.1:8123"); got == "" {
		t.Fatal("expected windows mount hint")
	}
}

func TestMountCommand(t *testing.T) {
	if got := MountCommand("darwin", "127.0.0.1:8123", "/Volumes/Test"); got == "" {
		t.Fatal("expected darwin mount command")
	}
	if got := MountCommand("windows", "127.0.0.1:8123", "Z:"); got == "" {
		t.Fatal("expected windows mount command")
	}
}

func NewTestTree() *vodfs.Tree {
	return vodfs.NewTree(
		[]catalog.Movie{{ID: "m1", Title: "Movie", Year: 2024, StreamURL: "http://example.com/movie.mp4"}},
		[]catalog.Series{{ID: "s1", Title: "Show", Year: 2020}},
	)
}

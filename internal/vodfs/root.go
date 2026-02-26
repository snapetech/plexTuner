//go:build linux
// +build linux

package vodfs

import (
	"context"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/materializer"
)

// Root holds catalog snapshot and materializer; implements root of VODFS.
type Root struct {
	fs.Inode
	Movies          []catalog.Movie
	Series          []catalog.Series
	Mat             materializer.Interface
	movieDirNames   map[string]string // movieID -> unique dir name
	seriesDirNames  map[string]string // seriesID -> unique dir name
	movieByDirName  map[string]int    // unique movie dir name -> index in Movies
	seriesByDirName map[string]int    // unique show dir name -> index in Series
}

var _ fs.NodeLookuper = (*Root)(nil)

func (r *Root) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	switch name {
	case "Movies":
		moviesNode := &MoviesDirNode{Root: r}
		ch := r.NewInode(ctx, moviesNode, fs.StableAttr{
			Mode: fuse.S_IFDIR,
			Ino:  r.ino("dir:Movies"),
		})
		out.Mode = fuse.S_IFDIR | 0755
		out.SetEntryTimeout(1 * time.Second)
		out.SetAttrTimeout(1 * time.Second)
		return ch, 0
	case "TV":
		tvNode := &TVDirNode{Root: r}
		ch := r.NewInode(ctx, tvNode, fs.StableAttr{
			Mode: fuse.S_IFDIR,
			Ino:  r.ino("dir:TV"),
		})
		out.Mode = fuse.S_IFDIR | 0755
		out.SetEntryTimeout(1 * time.Second)
		out.SetAttrTimeout(1 * time.Second)
		return ch, 0
	default:
		return nil, syscall.ENOENT
	}
}

func (r *Root) ino(key string) uint64 {
	return inoFromString("plexvod:" + key)
}

func (r *Root) movieDirName(m *catalog.Movie) string {
	if m == nil {
		return ""
	}
	if n, ok := r.movieDirNames[m.ID]; ok && n != "" {
		return n
	}
	return MovieDirName(m.Title, m.Year)
}

func (r *Root) showDirName(s *catalog.Series) string {
	if s == nil {
		return ""
	}
	if n, ok := r.seriesDirNames[s.ID]; ok && n != "" {
		return n
	}
	return ShowDirName(s.Title, s.Year)
}

func (r *Root) buildNameIndexes() {
	r.movieDirNames = buildUniqueMovieDirNames(r.Movies)
	r.seriesDirNames = buildUniqueSeriesDirNames(r.Series)
	r.movieByDirName = make(map[string]int, len(r.Movies))
	for i := range r.Movies {
		m := &r.Movies[i]
		name := r.movieDirName(m)
		if name != "" {
			r.movieByDirName[name] = i
		}
	}
	r.seriesByDirName = make(map[string]int, len(r.Series))
	for i := range r.Series {
		s := &r.Series[i]
		name := r.showDirName(s)
		if name != "" {
			r.seriesByDirName[name] = i
		}
	}
}

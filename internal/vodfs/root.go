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
	Movies  []catalog.Movie
	Series  []catalog.Series
	Mat materializer.Interface
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

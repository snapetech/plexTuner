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
)

// MoviesDirNode lists movie folders.
type MoviesDirNode struct {
	fs.Inode
	Root *Root
}

var _ fs.NodeReaddirer = (*MoviesDirNode)(nil)
var _ fs.NodeLookuper = (*MoviesDirNode)(nil)

func (n *MoviesDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	return &movieDirStream{root: n.Root}, 0
}

func (n *MoviesDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if idx, ok := n.Root.movieByDirName[name]; ok && idx >= 0 && idx < len(n.Root.Movies) {
		m := &n.Root.Movies[idx]
		child := &MovieDirNode{Root: n.Root, Movie: m}
		ch := n.NewInode(ctx, child, fs.StableAttr{
			Mode: fuse.S_IFDIR,
			Ino:  n.Root.ino("movie:" + m.ID),
		})
		out.Mode = fuse.S_IFDIR | 0755
		out.SetEntryTimeout(1 * time.Second)
		out.SetAttrTimeout(1 * time.Second)
		return ch, 0
	}
	return nil, syscall.ENOENT
}

// MovieDirNode is a single movie folder containing one file: "Title (Year).mp4".
type MovieDirNode struct {
	fs.Inode
	Root  *Root
	Movie *catalog.Movie
}

var _ fs.NodeReaddirer = (*MovieDirNode)(nil)
var _ fs.NodeLookuper = (*MovieDirNode)(nil)

func (n *MovieDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fileName := MovieFileNameForStream(n.Movie.Title, n.Movie.Year, n.Movie.StreamURL)
	entries := []fuse.DirEntry{{
		Name: fileName,
		Ino:  n.Root.ino("file:movie:" + n.Movie.ID),
		Mode: fuse.S_IFREG | 0444,
	}}
	return fs.NewListDirStream(entries), 0
}

func (n *MovieDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fileName := MovieFileNameForStream(n.Movie.Title, n.Movie.Year, n.Movie.StreamURL)
	if name != fileName {
		return nil, syscall.ENOENT
	}
	vf := &VirtualFileNode{
		Root:      n.Root,
		AssetID:   n.Movie.ID,
		StreamURL: n.Movie.StreamURL,
		Size:      0,
	}
	ch := n.NewInode(ctx, vf, fs.StableAttr{
		Mode: fuse.S_IFREG,
		Ino:  n.Root.ino("file:movie:" + n.Movie.ID),
	})
	out.Mode = fuse.S_IFREG | 0444
	out.Size = vf.placeholderSize()
	out.SetEntryTimeout(1 * time.Second)
	out.SetAttrTimeout(1 * time.Second)
	return ch, 0
}

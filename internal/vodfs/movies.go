package vodfs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

// MoviesDirNode is the "Movies" directory.
type MoviesDirNode struct {
	fs.Inode
	root *Root
}

var _ fs.NodeReaddirer = (*MoviesDirNode)(nil)
var _ fs.NodeLookuper = (*MoviesDirNode)(nil)

func (n *MoviesDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := make([]fuse.DirEntry, 0, len(n.root.Movies))
	for _, m := range n.root.Movies {
		if m != nil {
			entries = append(entries, fuse.DirEntry{Name: MovieDirName(m), Mode: fuse.S_IFDIR})
		}
	}
	return fs.NewListDirStream(entries), 0
}

func (n *MoviesDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	m := n.root.MovieByDirName[name]
	if m == nil {
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(ctx, &MovieDirNode{root: n.root, movie: m}, fs.StableAttr{Mode: fuse.S_IFDIR})
	out.SetAttrTimeout(entryAttrTimeout)
	out.SetEntryTimeout(entryAttrTimeout)
	return ch, 0
}

// MovieDirNode is a single movie directory (contains one virtual file, e.g. the stream).
type MovieDirNode struct {
	fs.Inode
	root  *Root
	movie *catalog.Movie
}

var _ fs.NodeReaddirer = (*MovieDirNode)(nil)
var _ fs.NodeLookuper = (*MovieDirNode)(nil)

func (n *MovieDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	// One entry: the playable file (e.g. stream.m3u8 or stream.mp4)
	entries := []fuse.DirEntry{{Name: "stream", Mode: fuse.S_IFREG}}
	return fs.NewListDirStream(entries), 0
}

func (n *MovieDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name != "stream" {
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(ctx, &VirtualFileNode{streamURL: n.movie.StreamURL}, fs.StableAttr{Mode: fuse.S_IFREG})
	out.SetAttrTimeout(entryAttrTimeout)
	out.SetEntryTimeout(entryAttrTimeout)
	return ch, 0
}

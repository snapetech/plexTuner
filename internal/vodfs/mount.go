package vodfs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Mount mounts the VOD catalog at dir. root must have MovieByDirName and SeriesByDirName populated.
func Mount(dir string, root *Root) (*fuse.Server, error) {
	rootNode := &RootNode{root: root}
	to := entryAttrTimeout
	opts := &fs.Options{
		EntryTimeout: &to,
		AttrTimeout:  &to,
	}
	server, err := fs.Mount(dir, rootNode, opts)
	if err != nil {
		return nil, err
	}
	return server, nil
}

// RootNode implements fs.InodeEmbedder for the FUSE root.
type RootNode struct {
	fs.Inode
	root *Root
}

var _ fs.NodeGetattrer = (*RootNode)(nil)
var _ fs.NodeReaddirer = (*RootNode)(nil)
var _ fs.NodeLookuper = (*RootNode)(nil)

func (r *RootNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = fuse.S_IFDIR | 0755
	out.Size = 0
	return 0
}

func (r *RootNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := []fuse.DirEntry{
		{Name: "Movies", Mode: fuse.S_IFDIR},
		{Name: "TV", Mode: fuse.S_IFDIR},
	}
	return fs.NewListDirStream(entries), 0
}

func (r *RootNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	switch name {
	case "Movies":
		ch := r.NewInode(ctx, &MoviesDirNode{root: r.root}, fs.StableAttr{Mode: fuse.S_IFDIR})
		out.SetAttrTimeout(entryAttrTimeout)
		out.SetEntryTimeout(entryAttrTimeout)
		return ch, 0
	case "TV":
		ch := r.NewInode(ctx, &TVDirNode{root: r.root}, fs.StableAttr{Mode: fuse.S_IFDIR})
		out.SetAttrTimeout(entryAttrTimeout)
		out.SetEntryTimeout(entryAttrTimeout)
		return ch, 0
	default:
		return nil, syscall.ENOENT
	}
}

//go:build linux
// +build linux

package vodfs

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/plextuner/plex-tuner/internal/catalog"
)

// TVDirNode lists show folders.
type TVDirNode struct {
	fs.Inode
	Root *Root
}

var _ fs.NodeReaddirer = (*TVDirNode)(nil)
var _ fs.NodeLookuper = (*TVDirNode)(nil)

func (n *TVDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := make([]fuse.DirEntry, 0, len(n.Root.Series))
	for _, s := range n.Root.Series {
		dirName := ShowDirName(s.Title, s.Year)
		entries = append(entries, fuse.DirEntry{
			Name: dirName,
			Ino:  n.Root.ino("series:" + s.ID),
			Mode: fuse.S_IFDIR | 0755,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *TVDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	for i := range n.Root.Series {
		s := &n.Root.Series[i]
		if ShowDirName(s.Title, s.Year) == name {
			child := &ShowDirNode{Root: n.Root, Series: s}
			ch := n.NewInode(ctx, child, fs.StableAttr{
				Mode: fuse.S_IFDIR,
				Ino:  n.Root.ino("series:" + s.ID),
			})
			out.Mode = fuse.S_IFDIR | 0755
			out.SetEntryTimeout(1 * time.Second)
			out.SetAttrTimeout(1 * time.Second)
			return ch, 0
		}
	}
	return nil, syscall.ENOENT
}

// ShowDirNode is a show folder containing Season 01, Season 02, ...
type ShowDirNode struct {
	fs.Inode
	Root   *Root
	Series *catalog.Series
}

var _ fs.NodeReaddirer = (*ShowDirNode)(nil)
var _ fs.NodeLookuper = (*ShowDirNode)(nil)

func (n *ShowDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := make([]fuse.DirEntry, 0, len(n.Series.Seasons))
	for _, se := range n.Series.Seasons {
		name := SeasonDirName(se.Number)
		entries = append(entries, fuse.DirEntry{
			Name: name,
			Ino:  n.Root.ino(fmt.Sprintf("season:%s:%d", n.Series.ID, se.Number)),
			Mode: fuse.S_IFDIR | 0755,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *ShowDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	for i := range n.Series.Seasons {
		se := &n.Series.Seasons[i]
		if SeasonDirName(se.Number) == name {
			child := &SeasonDirNode{Root: n.Root, Series: n.Series, Season: se}
			ch := n.NewInode(ctx, child, fs.StableAttr{
				Mode: fuse.S_IFDIR,
				Ino:  n.Root.ino(fmt.Sprintf("season:%s:%d", n.Series.ID, se.Number)),
			})
			out.Mode = fuse.S_IFDIR | 0755
			out.SetEntryTimeout(1 * time.Second)
			out.SetAttrTimeout(1 * time.Second)
			return ch, 0
		}
	}
	return nil, syscall.ENOENT
}

// SeasonDirNode is a season folder containing episode files.
type SeasonDirNode struct {
	fs.Inode
	Root   *Root
	Series *catalog.Series
	Season *catalog.Season
}

var _ fs.NodeReaddirer = (*SeasonDirNode)(nil)
var _ fs.NodeLookuper = (*SeasonDirNode)(nil)

func (n *SeasonDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := make([]fuse.DirEntry, 0, len(n.Season.Episodes))
	for _, ep := range n.Season.Episodes {
		fileName := EpisodeFileName(n.Series.Title, n.Series.Year, ep.SeasonNum, ep.EpisodeNum, ep.Title)
		entries = append(entries, fuse.DirEntry{
			Name: fileName,
			Ino:  n.Root.ino("file:ep:" + ep.ID),
			Mode: fuse.S_IFREG | 0444,
		})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *SeasonDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	for i := range n.Season.Episodes {
		ep := &n.Season.Episodes[i]
		fileName := EpisodeFileName(n.Series.Title, n.Series.Year, ep.SeasonNum, ep.EpisodeNum, ep.Title)
		if fileName == name {
			vf := &VirtualFileNode{
				Root:      n.Root,
				AssetID:   ep.ID,
				StreamURL: ep.StreamURL,
				Size:      0,
			}
			ch := n.NewInode(ctx, vf, fs.StableAttr{
				Mode: fuse.S_IFREG,
				Ino:  n.Root.ino("file:ep:" + ep.ID),
			})
			out.Mode = fuse.S_IFREG | 0444
			out.Size = 0
			out.SetEntryTimeout(1 * time.Second)
			out.SetAttrTimeout(1 * time.Second)
			return ch, 0
		}
	}
	return nil, syscall.ENOENT
}

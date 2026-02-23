package vodfs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

// TVDirNode is the "TV" directory.
type TVDirNode struct {
	fs.Inode
	root *Root
}

var _ fs.NodeReaddirer = (*TVDirNode)(nil)
var _ fs.NodeLookuper = (*TVDirNode)(nil)

func (n *TVDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := make([]fuse.DirEntry, 0, len(n.root.Series))
	for _, s := range n.root.Series {
		if s != nil {
			entries = append(entries, fuse.DirEntry{Name: ShowDirName(s), Mode: fuse.S_IFDIR})
		}
	}
	return fs.NewListDirStream(entries), 0
}

func (n *TVDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	s := n.root.SeriesByDirName[name]
	if s == nil {
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(ctx, &ShowDirNode{root: n.root, series: s}, fs.StableAttr{Mode: fuse.S_IFDIR})
	out.SetAttrTimeout(entryAttrTimeout)
	out.SetEntryTimeout(entryAttrTimeout)
	return ch, 0
}

// ShowDirNode is a single show directory (Seasons).
type ShowDirNode struct {
	fs.Inode
	root   *Root
	series *catalog.Series
}

var _ fs.NodeReaddirer = (*ShowDirNode)(nil)
var _ fs.NodeLookuper = (*ShowDirNode)(nil)

func (n *ShowDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := make([]fuse.DirEntry, 0, len(n.series.Seasons))
	for _, se := range n.series.Seasons {
		entries = append(entries, fuse.DirEntry{Name: "Season " + strconvItoa(se.Number), Mode: fuse.S_IFDIR})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *ShowDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	// Parse "Season N" or just "Season N"
	var num int
	if len(name) > 7 && name[:7] == "Season " {
		num = atoi(name[7:])
	}
	if num < 1 {
		return nil, syscall.ENOENT
	}
	var season *catalog.Season
	for i := range n.series.Seasons {
		if n.series.Seasons[i].Number == num {
			season = &n.series.Seasons[i]
			break
		}
	}
	if season == nil {
		return nil, syscall.ENOENT
	}
	ch := n.NewInode(ctx, &SeasonDirNode{series: n.series, season: season}, fs.StableAttr{Mode: fuse.S_IFDIR})
	out.SetAttrTimeout(entryAttrTimeout)
	out.SetEntryTimeout(entryAttrTimeout)
	return ch, 0
}

// SeasonDirNode is a season directory (Episodes).
type SeasonDirNode struct {
	fs.Inode
	series *catalog.Series
	season *catalog.Season
}

var _ fs.NodeReaddirer = (*SeasonDirNode)(nil)
var _ fs.NodeLookuper = (*SeasonDirNode)(nil)

func (n *SeasonDirNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := make([]fuse.DirEntry, 0, len(n.season.Episodes))
	for _, ep := range n.season.Episodes {
		entries = append(entries, fuse.DirEntry{Name: ep.Title + ".stream", Mode: fuse.S_IFREG})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *SeasonDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	base := name
	if len(name) > 7 && name[len(name)-7:] == ".stream" {
		base = name[:len(name)-7]
	}
	for i := range n.season.Episodes {
		if n.season.Episodes[i].Title == base {
			ch := n.NewInode(ctx, &VirtualFileNode{streamURL: n.season.Episodes[i].StreamURL}, fs.StableAttr{Mode: fuse.S_IFREG})
			out.SetAttrTimeout(entryAttrTimeout)
			out.SetEntryTimeout(entryAttrTimeout)
			return ch, 0
		}
	}
	return nil, syscall.ENOENT
}

func strconvItoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

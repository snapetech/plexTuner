//go:build linux
// +build linux

package vodfs

import (
	"context"
	"os"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// VirtualFileNode represents a single VOD file (movie or episode).
// StreamURL is passed to the materializer to download on demand.
type VirtualFileNode struct {
	fs.Inode
	Root      *Root
	AssetID   string
	StreamURL string
	Size      uint64
}

var _ fs.NodeGetattrer = (*VirtualFileNode)(nil)
var _ fs.NodeReader = (*VirtualFileNode)(nil)

func (n *VirtualFileNode) getPath(ctx context.Context) (path string, size int64) {
	if n.Root.Mat == nil {
		return "", 0
	}
	path, err := n.Root.Mat.Materialize(ctx, n.AssetID, n.StreamURL)
	if err != nil || path == "" {
		return "", 0
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", 0
	}
	return path, fi.Size()
}

// Getattr returns metadata without materializing. Size may be 0 until the file is opened/read.
// Do not call Materialize here or Plex scans will trigger mass downloads.
func (n *VirtualFileNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = n.Size
	out.Mode = fuse.S_IFREG | 0444
	out.SetTimes(nil, &time.Time{}, nil)
	return 0
}

func (n *VirtualFileNode) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	path, size := n.getPath(ctx)
	if path == "" {
		return fuse.ReadResultData(dest[:0]), 0
	}
	end := off + int64(len(dest))
	if end > size {
		end = size
	}
	if off >= size {
		return fuse.ReadResultData(dest[:0]), 0
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, syscall.EIO
	}
	defer f.Close()
	nread, err := f.ReadAt(dest[:end-off], off)
	if err != nil && nread == 0 {
		return nil, syscall.EIO
	}
	return fuse.ReadResultData(dest[:nread]), 0
}

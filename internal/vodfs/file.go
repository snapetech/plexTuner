package vodfs

import (
	"context"
	"io"
	"os"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/materializer"
)

// VirtualFileNode represents a VOD stream file. Implements NodeOpener; keeps FD open for reads.
type VirtualFileNode struct {
	fs.Inode
	streamURL string
}

var _ fs.NodeOpener = (*VirtualFileNode)(nil)
var _ fs.NodeGetattrer = (*VirtualFileNode)(nil)

func (n *VirtualFileNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	client := httpclient.Default()
	size, err := materializer.ContentLength(client, n.streamURL)
	if err != nil || size < 0 {
		size = 0
	}
	out.Mode = fuse.S_IFREG | 0644
	out.Size = uint64(size)
	return 0
}

func (n *VirtualFileNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	dest, err := os.CreateTemp("", "plextuner-*")
	if err != nil {
		return nil, 0, syscall.EIO
	}
	path := dest.Name()
	dest.Close()
	client := httpclient.Default()
	if err := materializer.Download(client, n.streamURL, path); err != nil {
		os.Remove(path)
		return nil, 0, syscall.EIO
	}
	f, err := os.Open(path)
	if err != nil {
		os.Remove(path)
		return nil, 0, syscall.EIO
	}
	info, _ := f.Stat()
	size := uint64(0)
	if info != nil {
		size = uint64(info.Size())
	}
	return &virtualFileHandle{path: path, f: f, size: size}, fuse.FOPEN_KEEP_CACHE, 0
}

// virtualFileHandle holds an open file descriptor for reading; implements FileReader and FileReleaser.
type virtualFileHandle struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size uint64
}

var _ fs.FileReleaser = (*virtualFileHandle)(nil)
var _ fs.FileReader = (*virtualFileHandle)(nil)

func (h *virtualFileHandle) Release(ctx context.Context) syscall.Errno {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.f != nil {
		h.f.Close()
		h.f = nil
	}
	os.Remove(h.path)
	return 0
}

func (h *virtualFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.f != nil {
		n, err := h.f.ReadAt(dest, off)
		if err != nil && err != io.EOF {
			return nil, syscall.EIO
		}
		return fuse.ReadResultData(dest[:n]), 0
	}
	// Fallback: open, read, close
	f, err := os.Open(h.path)
	if err != nil {
		return nil, syscall.EIO
	}
	defer f.Close()
	n, err := f.ReadAt(dest, off)
	if err != nil && err != io.EOF {
		return nil, syscall.EIO
	}
	return fuse.ReadResultData(dest[:n]), 0
}

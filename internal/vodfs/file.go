//go:build linux
// +build linux

package vodfs

import (
	"context"
	"log"
	"os"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	icache "github.com/plextuner/plex-tuner/internal/cache"
	"github.com/plextuner/plex-tuner/internal/materializer"
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
var _ fs.NodeOpener = (*VirtualFileNode)(nil)
var _ fs.NodeReader = (*VirtualFileNode)(nil)

func (n *VirtualFileNode) placeholderSize() uint64 {
	if n.Size > 0 {
		return n.Size
	}
	if n.StreamURL != "" {
		return 1
	}
	return 0
}

func (n *VirtualFileNode) getPath(ctx context.Context) (path string, size int64) {
	if n.Root.Mat == nil {
		log.Printf("vodfs: materialize skipped asset=%s reason=no-materializer", n.AssetID)
		return "", 0
	}
	path, err := n.Root.Mat.Materialize(ctx, n.AssetID, n.StreamURL)
	if err != nil || path == "" {
		if err != nil {
			log.Printf("vodfs: materialize failed asset=%s err=%v", n.AssetID, err)
		} else {
			log.Printf("vodfs: materialize failed asset=%s err=empty-path", n.AssetID)
		}
		return "", 0
	}
	fi, err := os.Stat(path)
	if err != nil {
		log.Printf("vodfs: materialize stat failed asset=%s path=%q err=%v", n.AssetID, path, err)
		return "", 0
	}
	log.Printf("vodfs: materialize ok asset=%s path=%q size=%d", n.AssetID, path, fi.Size())
	return path, fi.Size()
}

// Getattr returns metadata without materializing. Size may be 0 until the file is opened/read.
// Do not call Materialize here or Plex scans will trigger mass downloads.
func (n *VirtualFileNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	// Plex scanners commonly ignore zero-byte media files. We avoid materializing on
	// getattr (to prevent mass downloads during scans), so expose a small non-zero
	// placeholder size when the real size is unknown.
	out.Size = n.placeholderSize()
	out.Mode = fuse.S_IFREG | 0444
	out.SetTimes(nil, &time.Time{}, nil)
	return 0
}

func (n *VirtualFileNode) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	log.Printf("vodfs: read asset=%s off=%d len=%d", n.AssetID, off, len(dest))
	// For cache-backed VODFS, prefer a progressive read path first so Plex can get
	// probe bytes from a growing .partial file instead of waiting for full download.
	if res, ok := n.tryProgressiveRead(ctx, dest, off); ok {
		return res, 0
	}
	path, size := n.getPath(ctx)
	if path == "" {
		log.Printf("vodfs: read eof asset=%s reason=no-path", n.AssetID)
		return fuse.ReadResultData(dest[:0]), 0
	}
	return n.readLocal(path, size, dest, off)
}

func (n *VirtualFileNode) readLocal(path string, size int64, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := off + int64(len(dest))
	if end > size {
		end = size
	}
	if off >= size {
		log.Printf("vodfs: read eof asset=%s reason=off>=size off=%d size=%d", n.AssetID, off, size)
		return fuse.ReadResultData(dest[:0]), 0
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, syscall.EIO
	}
	defer f.Close()
	nread, err := f.ReadAt(dest[:end-off], off)
	if err != nil && nread == 0 {
		log.Printf("vodfs: read eio asset=%s path=%q off=%d err=%v", n.AssetID, path, off, err)
		return nil, syscall.EIO
	}
	log.Printf("vodfs: read ok asset=%s path=%q off=%d n=%d", n.AssetID, path, off, nread)
	return fuse.ReadResultData(dest[:nread]), 0
}

func (n *VirtualFileNode) tryProgressiveRead(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, bool) {
	cm, ok := n.Root.Mat.(*materializer.Cache)
	if !ok || cm == nil || cm.CacheDir == "" || n.AssetID == "" || n.StreamURL == "" {
		return nil, false
	}

	// Kick off (or join) background materialization without blocking this read on full completion.
	go func(assetID, streamURL string) {
		bgctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if _, err := cm.Materialize(bgctx, assetID, streamURL); err != nil {
			log.Printf("vodfs: progressive materialize background failed asset=%s err=%v", assetID, err)
		}
	}(n.AssetID, n.StreamURL)

	finalPath := icache.Path(cm.CacheDir, n.AssetID)
	partialPath := icache.PartialPath(cm.CacheDir, n.AssetID)
	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return nil, false
		default:
		}
		for _, p := range []string{finalPath, partialPath} {
			if fi, err := os.Stat(p); err == nil && fi.Size() > off {
				log.Printf("vodfs: progressive read asset=%s using=%q size=%d", n.AssetID, p, fi.Size())
				res, errno := n.readLocal(p, fi.Size(), dest, off)
				if errno == 0 {
					return res, true
				}
			}
		}
		if time.Now().After(deadline) {
			return nil, false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (n *VirtualFileNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	log.Printf("vodfs: open asset=%s flags=0x%x", n.AssetID, flags)
	// These files are virtual/materialized on demand; force direct I/O so reads hit
	// the node Read path instead of stale kernel page cache entries from prior EOFs.
	return nil, fuse.FOPEN_DIRECT_IO, 0
}

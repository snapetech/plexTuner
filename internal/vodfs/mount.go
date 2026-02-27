//go:build linux
// +build linux

package vodfs

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/materializer"
)

// Mount mounts VODFS at mountPoint using the given catalog snapshot and materializer.
// It blocks until the process receives SIGINT or the server exits.
func Mount(mountPoint string, movies []catalog.Movie, series []catalog.Series, mat materializer.Interface) error {
	return MountWithAllowOther(mountPoint, movies, series, mat, false)
}

// MountWithAllowOther mounts VODFS and optionally enables FUSE allow_other so
// non-mounting users/processes (for example Plex in another runtime context) can access it.
func MountWithAllowOther(mountPoint string, movies []catalog.Movie, series []catalog.Series, mat materializer.Interface, allowOther bool) error {
	root := &Root{
		Movies: movies,
		Series: series,
		Mat:    mat,
	}
	if root.Mat == nil {
		root.Mat = &materializer.Stub{}
	}
	root.buildNameIndexes()

	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug:      false,
			AllowOther: allowOther,
		},
	}
	server, err := fs.Mount(mountPoint, root, opts)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		log.Println("Unmounting VODFS...")
		_ = server.Unmount()
	}()

	server.Wait()
	stop()
	return nil
}

// MountBackground mounts VODFS in the background and returns an unmount function.
// Unlike MountWithAllowOther it does not block; call the returned func to unmount
// (e.g. before remounting with a refreshed catalog). ctx cancellation also unmounts.
func MountBackground(ctx context.Context, mountPoint string, movies []catalog.Movie, series []catalog.Series, mat materializer.Interface, allowOther bool) (unmount func(), err error) {
	root := &Root{
		Movies: movies,
		Series: series,
		Mat:    mat,
	}
	if root.Mat == nil {
		root.Mat = &materializer.Stub{}
	}
	root.buildNameIndexes()

	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug:      false,
			AllowOther: allowOther,
		},
	}
	server, err := fs.Mount(mountPoint, root, opts)
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		_ = server.Unmount()
	}()

	return func() { _ = server.Unmount() }, nil
}

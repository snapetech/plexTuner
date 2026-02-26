//go:build !linux
// +build !linux

package vodfs

import (
	"fmt"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/materializer"
)

// Mount is unavailable on non-Linux builds because VODFS currently depends on go-fuse.
func Mount(mountPoint string, movies []catalog.Movie, series []catalog.Series, mat materializer.Interface) error {
	return fmt.Errorf("vodfs mount is only supported on linux builds")
}

// MountWithAllowOther is unavailable on non-Linux builds because VODFS currently depends on go-fuse.
func MountWithAllowOther(mountPoint string, movies []catalog.Movie, series []catalog.Series, mat materializer.Interface, allowOther bool) error {
	return fmt.Errorf("vodfs mount is only supported on linux builds")
}

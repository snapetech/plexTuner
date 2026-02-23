package vodfs

import (
	"hash/fnv"
)

// Stable inode numbers from path-like keys so same logical file gets same inode.
func inoFromString(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

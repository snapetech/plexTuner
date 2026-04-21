//go:build !windows

package tuner

import (
	"os"
	"syscall"
)

func lockProviderSharedLeaseFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockProviderSharedLeaseFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

//go:build windows

package tuner

import "os"

func lockProviderSharedLeaseFile(_ *os.File) error {
	return nil
}

func unlockProviderSharedLeaseFile(_ *os.File) error {
	return nil
}

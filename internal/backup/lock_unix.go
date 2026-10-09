//go:build !windows

package backup

import (
	"os"

	"golang.org/x/sys/unix"
)

// Kernel locks disappear after a crash; the lock file itself need not be deleted.
func lockStore(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func syncDirectory(name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

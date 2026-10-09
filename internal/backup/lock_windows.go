package backup

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockStore(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}

// Windows does not expose directory fsync through os.File.Sync.
func syncDirectory(string) error { return nil }

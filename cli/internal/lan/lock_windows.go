//go:build windows

package lan

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func lockBusy(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }

func lockDirectory(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}

// Windows does not expose directory fsync through os.File.Sync. Files are
// flushed before atomic publication; power-cut durability needs platform QA.
func syncDirectory(root *os.Root) error { return nil }

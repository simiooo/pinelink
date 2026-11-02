//go:build unix

package lan

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func lockBusy(err error) bool { return errors.Is(err, unix.EWOULDBLOCK) }

func lockDirectory(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }

func syncDirectory(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

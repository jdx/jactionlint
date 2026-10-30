//go:build unix

package cache

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// LockDir takes an exclusive advisory lock on the store, so that two runs do not rewrite the index at once.
func LockDir(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		unix.Flock(int(f.Fd()), unix.LOCK_UN)
		f.Close()
	}, nil
}

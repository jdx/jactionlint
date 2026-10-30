//go:build !unix

package cache

// LockDir takes an exclusive lock on the directory; there is nothing to lock on this OS.
func LockDir(dir string) (func(), error) { return func() {}, nil }

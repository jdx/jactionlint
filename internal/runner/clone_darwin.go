package runner

import "golang.org/x/sys/unix"

// cloneFile makes dst a copy-on-write clone of src (clonefile on APFS).
func cloneFile(src, dst string) error {
	return unix.Clonefile(src, dst, 0)
}

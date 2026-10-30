//go:build linux && !amd64 && !arm64

package sandbox

import "fmt"

func applySeccomp(net Net) error {
	return fmt.Errorf("the sandbox supports Linux on amd64 and arm64 only")
}

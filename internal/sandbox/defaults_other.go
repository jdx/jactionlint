//go:build !linux && !darwin

package sandbox

var (
	systemExec  []string
	systemRead  []string
	deviceFiles []string
	sharedTmp   []string
)

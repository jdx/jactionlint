//go:build linux

package sandbox

const (
	auditArch = 0xC000003E // AUDIT_ARCH_X86_64
	x32Bit    = 0x40000000
)

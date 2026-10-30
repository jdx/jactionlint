//go:build linux && (amd64 || arm64)

package sandbox

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// A classic BPF program that the kernel runs on every system call of the step. It only denies: a call that no
// rule names is allowed, so a new kernel feature is not broken, and Landlock carries the filesystem policy.

type sockFilter struct {
	code uint16
	jt   uint8
	jf   uint8
	k    uint32
}

const (
	bpfLD   = 0x00
	bpfW    = 0x00
	bpfABS  = 0x20
	bpfALU  = 0x04
	bpfAND  = 0x50
	bpfJMP  = 0x05
	bpfJEQ  = 0x10
	bpfJSET = 0x40
	bpfK    = 0x00
	bpfRET  = 0x06

	seccompRetAllow = 0x7fff0000
	seccompRetKill  = 0x80000000 // SECCOMP_RET_KILL_PROCESS
	seccompRetErrno = 0x00050000

	offNr   = 0  // struct seccomp_data: nr
	offArch = 4  // arch
	offArg0 = 16 // args[0], low 32 bits on little endian
	offArg1 = 24 // args[1]

	sockDgram = 2
	sockRaw   = 3
)

// asm assembles a BPF program with labels, so that jump distances are computed and checked.
type asm struct {
	ins    []sockFilter
	labels map[string]int
	fix    []fixup
}

type fixup struct {
	at     int
	jt, jf string
}

func newAsm() *asm { return &asm{labels: map[string]int{}} }

func (a *asm) label(name string) { a.labels[name] = len(a.ins) }

func (a *asm) ld(off uint32) { a.ins = append(a.ins, sockFilter{code: bpfLD | bpfW | bpfABS, k: off}) }

func (a *asm) and(mask uint32) {
	a.ins = append(a.ins, sockFilter{code: bpfALU | bpfAND | bpfK, k: mask})
}

func (a *asm) ret(v uint32) { a.ins = append(a.ins, sockFilter{code: bpfRET | bpfK, k: v}) }

// jeq jumps to jt when the accumulator equals k and to jf otherwise; an empty label falls through.
func (a *asm) jeq(k uint32, jt, jf string) {
	a.fix = append(a.fix, fixup{len(a.ins), jt, jf})
	a.ins = append(a.ins, sockFilter{code: bpfJMP | bpfJEQ | bpfK, k: k})
}

func (a *asm) jset(k uint32, jt, jf string) {
	a.fix = append(a.fix, fixup{len(a.ins), jt, jf})
	a.ins = append(a.ins, sockFilter{code: bpfJMP | bpfJSET | bpfK, k: k})
}

func (a *asm) link() ([]sockFilter, error) {
	dist := func(at int, label string) (uint8, error) {
		if label == "" {
			return 0, nil
		}
		to, ok := a.labels[label]
		if !ok {
			return 0, fmt.Errorf("seccomp: unknown label %q", label)
		}
		d := to - (at + 1)
		if d < 0 || d > 255 {
			return 0, fmt.Errorf("seccomp: jump to %q out of range (%d)", label, d)
		}
		return uint8(d), nil
	}
	for _, f := range a.fix {
		var err error
		if a.ins[f.at].jt, err = dist(f.at, f.jt); err != nil {
			return nil, err
		}
		if a.ins[f.at].jf, err = dist(f.at, f.jf); err != nil {
			return nil, err
		}
	}
	return a.ins, nil
}

// deniedSyscalls are refused with EPERM for every step. They are the ways out of the sandbox that a step could
// otherwise take, and the ways to read or write another process.
var deniedSyscalls = []uint32{
	unix.SYS_PTRACE, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PIDFD_GETFD, unix.SYS_KCMP,
	unix.SYS_BPF, unix.SYS_PERF_EVENT_OPEN, unix.SYS_USERFAULTFD,
	unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER,
	unix.SYS_KEYCTL, unix.SYS_ADD_KEY, unix.SYS_REQUEST_KEY,
	unix.SYS_KEXEC_LOAD, unix.SYS_KEXEC_FILE_LOAD, unix.SYS_REBOOT,
	unix.SYS_INIT_MODULE, unix.SYS_FINIT_MODULE, unix.SYS_DELETE_MODULE,
	unix.SYS_MOUNT, unix.SYS_UMOUNT2, unix.SYS_PIVOT_ROOT, unix.SYS_MOVE_MOUNT, unix.SYS_OPEN_TREE,
	unix.SYS_FSOPEN, unix.SYS_FSCONFIG, unix.SYS_FSMOUNT, unix.SYS_FSPICK, unix.SYS_MOUNT_SETATTR,
	unix.SYS_UNSHARE, unix.SYS_SETNS,
	unix.SYS_SWAPON, unix.SYS_SWAPOFF, unix.SYS_ACCT, unix.SYS_SYSLOG, unix.SYS_SETHOSTNAME, unix.SYS_SETDOMAINNAME,
}

// buildSeccomp assembles the filter for a network mode.
func buildSeccomp(net Net) ([]sockFilter, error) {
	a := newAsm()
	const errnoPerm = seccompRetErrno | uint32(unix.EPERM)
	const errnoAcces = seccompRetErrno | uint32(unix.EACCES)

	// A call made through another ABI would carry other numbers: kill the process.
	a.ld(offArch)
	a.jeq(auditArch, "", "kill")
	a.ld(offNr)
	if x32Bit != 0 {
		a.jset(x32Bit, "eperm", "")
	}
	for _, nr := range deniedSyscalls {
		a.jeq(nr, "eperm", "")
	}
	a.jeq(unix.SYS_SOCKET, "socket", "allow")

	a.label("socket")
	a.ld(offArg0)
	switch net {
	case NetNone:
		a.jeq(unix.AF_INET, "eacces", "")
		a.jeq(unix.AF_INET6, "eacces", "")
		a.jeq(unix.AF_PACKET, "eacces", "allow")
	case NetProxy:
		a.jeq(unix.AF_PACKET, "eacces", "")
		a.jeq(unix.AF_INET, "inet", "")
		a.jeq(unix.AF_INET6, "inet", "allow")
		a.label("inet")
		// only stream sockets: the type is in the low nibble of args[1], above it are flags
		a.ld(offArg1)
		a.and(0xf)
		a.jeq(sockDgram, "eacces", "")
		a.jeq(sockRaw, "eacces", "allow")
	default:
		a.jeq(unix.AF_PACKET, "eacces", "allow")
	}

	a.label("allow")
	a.ret(seccompRetAllow)
	a.label("eperm")
	a.ret(errnoPerm)
	a.label("eacces")
	a.ret(errnoAcces)
	a.label("kill")
	a.ret(seccompRetKill)
	return a.link()
}

type sockFprog struct {
	len    uint16
	_      [6]byte
	filter *sockFilter
}

func applySeccomp(net Net) error {
	prog, err := buildSeccomp(net)
	if err != nil {
		return err
	}
	fp := sockFprog{len: uint16(len(prog)), filter: &prog[0]}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}
	if _, _, errno := unix.Syscall(unix.SYS_PRCTL, unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&fp))); errno != 0 {
		return fmt.Errorf("seccomp filter: %w", errno)
	}
	return nil
}

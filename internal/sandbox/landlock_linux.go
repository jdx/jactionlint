package sandbox

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Filesystem access rights of Landlock, by the ABI that introduced them.
const (
	fsExecute    = unix.LANDLOCK_ACCESS_FS_EXECUTE
	fsWriteFile  = unix.LANDLOCK_ACCESS_FS_WRITE_FILE
	fsReadFile   = unix.LANDLOCK_ACCESS_FS_READ_FILE
	fsReadDir    = unix.LANDLOCK_ACCESS_FS_READ_DIR
	fsRemoveDir  = unix.LANDLOCK_ACCESS_FS_REMOVE_DIR
	fsRemoveFile = unix.LANDLOCK_ACCESS_FS_REMOVE_FILE
	fsMakeChar   = unix.LANDLOCK_ACCESS_FS_MAKE_CHAR
	fsMakeDir    = unix.LANDLOCK_ACCESS_FS_MAKE_DIR
	fsMakeReg    = unix.LANDLOCK_ACCESS_FS_MAKE_REG
	fsMakeSock   = unix.LANDLOCK_ACCESS_FS_MAKE_SOCK
	fsMakeFifo   = unix.LANDLOCK_ACCESS_FS_MAKE_FIFO
	fsMakeBlock  = unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK
	fsMakeSym    = unix.LANDLOCK_ACCESS_FS_MAKE_SYM
	fsRefer      = unix.LANDLOCK_ACCESS_FS_REFER     // ABI 2
	fsTruncate   = unix.LANDLOCK_ACCESS_FS_TRUNCATE  // ABI 3
	fsIoctlDev   = unix.LANDLOCK_ACCESS_FS_IOCTL_DEV // ABI 5
	fsResolveUnx = 1 << 16                           // ABI 9: connect to a pathname unix socket

	netConnectTCP = unix.LANDLOCK_ACCESS_NET_CONNECT_TCP // ABI 4

	scopeAbstractUnix = unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET // ABI 6
	scopeSignal       = unix.LANDLOCK_SCOPE_SIGNAL               // ABI 6
)

const fsRightsReadOnly = fsReadFile | fsReadDir
const fsRightsFileOnly = fsExecute | fsWriteFile | fsReadFile | fsTruncate | fsIoctlDev

// landlockABI returns the highest Landlock ABI the kernel supports, or an error saying why there is none.
func landlockABI() (int, error) {
	v, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		switch errno {
		case unix.ENOSYS:
			return 0, fmt.Errorf("the kernel has no Landlock (needs Linux 5.13 or newer)")
		case unix.EOPNOTSUPP:
			return 0, fmt.Errorf("the kernel has Landlock built in but disabled (boot with lsm=landlock,...)")
		}
		return 0, fmt.Errorf("landlock is unavailable: %w", errno)
	}
	return int(v), nil
}

type rulesetAttr struct {
	handledFS  uint64
	handledNet uint64
	scoped     uint64
}

func fsHandled(abi int) uint64 {
	h := uint64(fsExecute | fsWriteFile | fsReadFile | fsReadDir | fsRemoveDir | fsRemoveFile | fsMakeChar | fsMakeDir |
		fsMakeReg | fsMakeSock | fsMakeFifo | fsMakeBlock | fsMakeSym)
	if abi >= 2 {
		h |= fsRefer
	}
	if abi >= 3 {
		h |= fsTruncate
	}
	if abi >= 5 {
		h |= fsIoctlDev
	}
	if abi >= 9 {
		h |= fsResolveUnx
	}
	return h
}

// applyLandlock restricts the calling thread (and so the process it execs) to the paths of the policy.
func applyLandlock(p *Policy, abi int) error {
	if abi < 1 {
		return fmt.Errorf("no Landlock")
	}
	attr := rulesetAttr{handledFS: fsHandled(abi)}
	size := uintptr(8)
	if p.Net == NetProxy {
		if abi < 4 {
			return fmt.Errorf("--net=proxy needs Landlock ABI 4 (Linux 6.7), this kernel has ABI %d", abi)
		}
		attr.handledNet = netConnectTCP
	}
	if abi >= 4 {
		size = 16
	}
	if abi >= 6 {
		attr.scoped = scopeAbstractUnix | scopeSignal
		size = 24
	}
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), size, 0)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	rs := int(fd)
	defer unix.Close(rs)

	handled := attr.handledFS
	add := func(path string, rights uint64) error {
		pfd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			if err == unix.ENOENT || err == unix.ENOTDIR || err == unix.EACCES || err == unix.ELOOP {
				return nil // nothing to allow
			}
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer unix.Close(pfd)
		var st unix.Stat_t
		if err := unix.Fstat(pfd, &st); err != nil {
			return err
		}
		rights &= handled
		if st.Mode&unix.S_IFMT != unix.S_IFDIR {
			rights &= fsRightsFileOnly
		}
		if rights == 0 {
			return nil
		}
		pb := unix.LandlockPathBeneathAttr{Allowed_access: rights, Parent_fd: int32(pfd)}
		if _, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(rs), unix.LANDLOCK_RULE_PATH_BENEATH, uintptr(unsafe.Pointer(&pb)), 0, 0, 0); errno != 0 {
			return fmt.Errorf("landlock_add_rule %s: %w", path, errno)
		}
		return nil
	}

	for _, s := range cleanPaths(p.Exec) {
		if err := add(s, fsRightsReadOnly|fsExecute); err != nil {
			return err
		}
	}
	for _, s := range cleanPaths(p.Read) {
		if err := add(s, fsRightsReadOnly); err != nil {
			return err
		}
	}
	for _, s := range cleanPaths(p.Write) {
		// everything the ABI handles, which includes executing, creating sockets and connecting to them
		if err := add(s, handled); err != nil {
			return err
		}
	}
	if p.Net == NetProxy {
		for _, port := range p.ConnectPorts {
			na := struct {
				allowed uint64
				port    uint64
			}{netConnectTCP, uint64(port)}
			if _, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(rs), landlockRuleNetPort, uintptr(unsafe.Pointer(&na)), 0, 0, 0); errno != 0 {
				return fmt.Errorf("landlock_add_rule port %d: %w", port, errno)
			}
		}
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(rs), 0, 0); errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}
	return nil
}

const landlockRuleNetPort = 2 // LANDLOCK_RULE_NET_PORT

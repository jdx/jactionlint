package sandbox

var systemExec = []string{
	"/System", "/usr", "/bin", "/sbin", "/Library", "/opt", "/private/etc", "/etc",
	"/private/var/db/timezone", "/private/var/db/dyld", "/Applications/Xcode.app", "/Library/Developer",
}

var systemRead = []string{"/private/var/db/mds", "/private/var/db/DetachedSignatures"}

var deviceFiles = []string{"/dev/null", "/dev/zero", "/dev/tty", "/dev/dtracehelper"}

// sharedTmp is shared with the host.
var sharedTmp = []string{"/private/tmp", "/private/var/tmp", "/private/var/folders"}

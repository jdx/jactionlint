package sandbox

// systemExec are read-and-run directories of a Linux system.
var systemExec = []string{
	"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/opt", "/nix", "/snap", "/etc",
	"/run/systemd/resolve", // where /etc/resolv.conf often points
}

// systemRead are single files and small directories that tools read. /proc is not here as a whole: reading
// /proc/<pid>/environ of the user's other processes would defeat the home directory rule.
var systemRead = []string{
	"/proc/cpuinfo", "/proc/meminfo", "/proc/stat", "/proc/uptime", "/proc/loadavg", "/proc/version",
	"/proc/filesystems", "/proc/sys",
	"/sys/devices/system/cpu", "/sys/fs/cgroup",
	"/var/lib/ca-certificates", "/var/lib/dpkg/status",
}

// deviceFiles are the character devices every program uses.
var deviceFiles = []string{"/dev/null", "/dev/zero", "/dev/full", "/dev/random", "/dev/urandom", "/dev/tty", "/dev/ptmx", "/dev/pts", "/dev/shm"}

// sharedTmp is shared with the host: Landlock cannot give a step a private /tmp (the namespace backend will).
var sharedTmp = []string{"/tmp", "/var/tmp"}

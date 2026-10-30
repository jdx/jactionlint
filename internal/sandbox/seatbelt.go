package sandbox

import (
	"fmt"
	"sort"
	"strings"
)

// seatbeltProfile renders a policy as a Seatbelt (SBPL) profile for sandbox-exec. It has no build tag so that its text can
// be tested everywhere; only the macOS backend runs it.
func seatbeltProfile(p *Policy) string {
	q := func(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }

	w("(version 1)")
	w("(deny default)")
	w(";; processes: a step starts programs and children, and signals its own group")
	w("(allow process-fork)")
	w("(allow signal (target self))")
	w("(allow process-info* (target self))")
	w("(allow sysctl-read)")
	w("(allow mach-lookup (global-name \"com.apple.system.opendirectoryd.libinfo\") (global-name \"com.apple.system.notification_center\") (global-name \"com.apple.trustd.agent\") (global-name \"com.apple.SecurityServer\") (global-name \"com.apple.logd\") (global-name \"com.apple.system.logger\"))")
	w("(allow ipc-posix-shm-read* ipc-posix-shm-write-data ipc-posix-shm-write-create)")
	w("(allow file-read-metadata)") // stat of any path, like Landlock; contents stay closed
	w(";; devices")
	w("(allow file-write-data (literal \"/dev/null\") (literal \"/dev/zero\") (literal \"/dev/dtracehelper\"))")
	w("(allow file-read* (literal \"/dev/null\") (literal \"/dev/zero\") (literal \"/dev/random\") (literal \"/dev/urandom\") (literal \"/dev/tty\"))")
	w("(allow pseudo-tty)")

	section := func(op string, paths []string) {
		paths = cleanPaths(paths)
		sort.Strings(paths)
		for _, s := range paths {
			w("(allow %s (subpath %s))", op, q(s))
		}
	}
	// macOS keeps /tmp, /var and /etc under /private; cleanPaths resolves the links, and the originals stay valid too.
	w(";; read and run")
	section("file-read* process-exec", p.Exec)
	w(";; read")
	section("file-read*", p.Read)
	w(";; read, write and run")
	section("file-read* file-write* process-exec", p.Write)

	switch p.Net {
	case NetNone:
		w(";; network: none (unix sockets stay)")
		w("(allow network* (local unix-socket))")
		w("(allow network* (remote unix-socket))")
	case NetProxy:
		w(";; network: only the ports of the proxy and the cache service on this machine")
		for _, port := range p.ConnectPorts {
			w("(allow network-outbound (remote ip %s))", q(fmt.Sprintf("localhost:%d", port)))
		}
		w("(allow network* (local unix-socket))")
		w("(allow network* (remote unix-socket))")
	default:
		w(";; network: open")
		w("(allow network*)")
	}
	return b.String()
}

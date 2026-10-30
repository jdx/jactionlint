package sandbox

import (
	"fmt"
	"sort"
	"strings"
)

// seatbeltProfile renders a policy as a Seatbelt (SBPL) profile for sandbox-exec. It has no build tag so that its text can
// be tested everywhere; only the macOS backend runs it.
//
// The profile starts from "allow default" and closes what a step must not touch; the last rule that matches wins. A
// profile that starts from "deny default" has to list every Mach service, sysctl and system file that a dynamically
// linked program needs, and a missing one makes bash abort at start. The price of this shape is that a place that the
// profile does not name is readable (like /etc on Linux), so the places that hold personal data are named: the users'
// home directories, external volumes and the keychains. Writing is allowed nowhere except the allow list.
func seatbeltProfile(p *Policy) string {
	q := func(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	subpaths := func(paths []string) string {
		paths = cleanPaths(paths)
		sort.Strings(paths)
		var parts []string
		for _, s := range paths {
			parts = append(parts, "(subpath "+q(s)+")")
		}
		return strings.Join(parts, " ")
	}
	// macOS keeps /tmp, /var and /etc under /private. cleanPaths resolves links, and /private/... is what the kernel
	// matches; the names a step uses are links to them.
	allowed := append(append(append([]string{}, p.Exec...), p.Read...), p.Write...)

	w("(version 1)")
	w(";; everything is allowed first; the rules below close what a step must not touch (the last matching rule wins)")
	w("(allow default)")

	w(";; files: nothing is written except the allow list")
	w("(deny file-write*)")
	if len(p.Write) > 0 {
		w("(allow file-write* %s)", subpaths(p.Write))
	}
	w("(allow file-write* (literal \"/dev/null\") (literal \"/dev/dtracehelper\") (literal \"/dev/tty\") (regex #\"^/dev/ttys[0-9]+$\"))")

	// mkdir -p tries to create every directory of a path and expects "exists" for the ones that are there; a denied
	// parent would answer "not permitted" and the tool would stop. Creating the ancestors themselves is allowed (they
	// exist, so nothing is created), their other children stay closed.
	seen := map[string]bool{}
	for _, wp := range cleanPaths(p.Write) {
		for d := wp; d != "/" && d != "." && !seen[d]; d = parentDir(d) {
			seen[d] = true
			w("(allow file-write-create (literal %s))", q(d))
		}
	}

	w(";; files: the places with personal data are closed, then what the step may read is opened again")
	// Only the contents are closed (file-read-data, which also covers listing a directory). A stat still works, as with
	// Landlock: node's realpath and mkdir -p stat every parent directory and stop on "not permitted".
	w("(deny file-read-data (subpath \"/Users\") (subpath \"/Volumes\") (subpath \"/private/var/root\") (subpath \"/Library/Keychains\"))")
	if len(allowed) > 0 {
		w("(allow file-read-data %s)", subpaths(allowed))
	}

	w(";; unix sockets: no agent or daemon socket outside the allow list (ssh-agent, docker.sock)")
	w("(deny network-outbound (remote unix-socket (subpath \"/private/tmp\") (subpath \"/private/var\") (subpath \"/Users\") (subpath \"/Volumes\")))")
	if len(p.Write) > 0 {
		w("(allow network-outbound (remote unix-socket %s))", subpaths(p.Write))
	}

	switch p.Net {
	case NetNone:
		w(";; network: none (unix sockets stay)")
		w("(deny network-outbound (remote ip))")
		w("(deny network-inbound (local ip))")
	case NetProxy:
		w(";; network: only the ports of the proxy and the cache service on this machine")
		w("(deny network-outbound (remote ip))")
		for _, port := range p.ConnectPorts {
			w("(allow network-outbound (remote ip %s))", q(fmt.Sprintf("localhost:%d", port)))
		}
	default:
		w(";; network: open")
	}
	return b.String()
}

func parentDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}

package sandbox

import (
	"sort"
	"strings"
)

// DefaultPassEnv are the variables of the caller that a step still sees: the minimum for tools to find each other
// and to print readable text. Everything else, secrets and tokens above all, is dropped.
var DefaultPassEnv = []string{"PATH", "LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES", "TERM", "TZ", "NO_COLOR", "COLORTERM"}

// dangerousEnv are variables that change what code a process loads. A step can set them for itself in its own
// env, but the runner refuses them from files that later steps read (GITHUB_ENV).
var dangerousEnv = map[string]bool{
	"NODE_OPTIONS": true, "LD_PRELOAD": true, "LD_LIBRARY_PATH": true, "LD_AUDIT": true,
	"BASH_ENV": true, "ENV": true, "PYTHONSTARTUP": true, "PYTHONPATH": true, "RUBYOPT": true, "PERL5OPT": true,
	"GIT_SSH_COMMAND": true, "GIT_EXTERNAL_DIFF": true, "GIT_ASKPASS": true, "SSH_ASKPASS": true,
}

// IsDangerousEnv reports whether a variable must not be set through a file command such as GITHUB_ENV.
func IsDangerousEnv(name string) bool {
	u := strings.ToUpper(name)
	return dangerousEnv[u] || strings.HasPrefix(u, "DYLD_") || strings.HasPrefix(u, "LD_")
}

// ScrubEnv builds the environment of a step: the pass-through variables of the parent that are listed in pass, then
// the extra values, which win. parent and the result are in KEY=VALUE form; the result is sorted.
func ScrubEnv(parent []string, pass []string, extra map[string]string) []string {
	want := map[string]bool{}
	for _, n := range pass {
		want[n] = true
	}
	m := map[string]string{}
	for _, kv := range parent {
		k, v, ok := strings.Cut(kv, "=")
		if ok && want[k] {
			m[k] = v
		}
	}
	for k, v := range extra {
		m[k] = v
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

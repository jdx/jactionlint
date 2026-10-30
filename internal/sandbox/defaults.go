package sandbox

import (
	"os"
	"path/filepath"
	"strings"
)

// Options are what DefaultPolicy builds a policy from.
type Options struct {
	// Workspace is the copy of the repository that the step works in; Home, Temp and ToolCache are writable too.
	Workspace string
	Home      string
	Temp      string
	ToolCache string
	// ExtraWrite and ExtraRead are directories the caller allows in addition (--allow-write, --allow-read).
	ExtraWrite []string
	ExtraRead  []string
	// PathDirs are the directories of the PATH of the step; they are readable and executable.
	PathDirs []string
	// RealHome is the home directory of the user; it is used to find tool installs that are safe to read.
	RealHome string
	Net      Net
	// ConnectPorts are the loopback ports a step may reach with NetProxy (the proxy and the cache service).
	ConnectPorts []uint16
}

// DefaultPolicy returns the policy of a step: the system directories and the tools on the PATH can be read and
// run; the workspace, the fake home, the temp directories and the tool cache can be written; the real home and
// everything else cannot be touched.
func DefaultPolicy(o Options) *Policy {
	p := &Policy{Version: PolicyVersion, Net: o.Net, ConnectPorts: o.ConnectPorts}
	p.Exec = append(p.Exec, systemExec...)
	p.Read = append(p.Read, systemRead...)
	p.Exec = append(p.Exec, o.PathDirs...)
	if o.RealHome != "" {
		for _, rel := range toolRoots {
			p.Exec = append(p.Exec, filepath.Join(o.RealHome, rel))
		}
	}
	p.Exec = append(p.Exec, o.ExtraRead...)
	p.Write = append(p.Write, deviceFiles...)
	p.Write = append(p.Write, o.Workspace, o.Home, o.Temp, o.ToolCache)
	p.Write = append(p.Write, sharedTmp...)
	p.Write = append(p.Write, o.ExtraWrite...)
	p.Exec = nonEmpty(p.Exec)
	p.Read = nonEmpty(p.Read)
	p.Write = nonEmpty(p.Write)
	return p
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// toolRoots are directories under the real home where version managers keep the tools they install. They hold
// distributions, not credentials, and the PATH of a step usually points into them.
var toolRoots = []string{
	".local/share/mise/installs", ".local/share/mise/shims", ".local/share/aquaproj-aqua", ".asdf/installs", ".asdf/shims",
	".rustup/toolchains", ".nvm/versions", ".pyenv/versions", ".sdkman/candidates", ".volta/tools",
}

// PathDirs splits a PATH variable into its absolute directories.
func PathDirs(path string) []string {
	var out []string
	for _, d := range strings.Split(path, string(os.PathListSeparator)) {
		if d != "" && filepath.IsAbs(d) {
			out = append(out, d)
		}
	}
	return out
}

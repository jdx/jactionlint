// Package sandbox runs a command with its access to the machine restricted by the operating system: Landlock and
// seccomp on Linux, Seatbelt on macOS. Everything else is refused. A policy is plain data; the restrictions are
// applied by a helper process (see MaybeHelper) just before the command starts, so the command cannot undo them.
package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Net is the network access of the command.
type Net string

const (
	// NetOpen puts no restriction on the network.
	NetOpen Net = "open"
	// NetNone denies creating internet sockets (AF_INET, AF_INET6). Unix sockets still work.
	NetNone Net = "none"
	// NetProxy lets TCP connect only to ConnectPorts on loopback-or-anywhere (Landlock rules are by port) and denies
	// UDP and raw sockets. It needs Landlock ABI 4 (Linux 6.7) or Seatbelt.
	NetProxy Net = "proxy"
)

// PolicyVersion is the version of the serialized policy. The helper refuses any other.
const PolicyVersion = 1

// Policy says what a command may touch. Paths must be absolute. A path that does not exist is ignored.
type Policy struct {
	Version int `json:"version"`
	// Exec is read and execute access to the paths (directories apply to everything beneath).
	Exec []string `json:"exec,omitempty"`
	// Read is read-only access.
	Read []string `json:"read,omitempty"`
	// Write is read, write, create, delete and execute access.
	Write []string `json:"write,omitempty"`
	Net   Net      `json:"net"`
	// ConnectPorts are the TCP ports that the command may connect to when Net is NetProxy.
	ConnectPorts []uint16 `json:"connect_ports,omitempty"`

	// Argv, Env and Dir describe the command. Env is the whole environment: nothing is inherited.
	Argv []string `json:"argv"`
	Env  []string `json:"env,omitempty"`
	Dir  string   `json:"dir,omitempty"`
}

// Validate checks a policy before it is applied. The helper calls it on what it reads, so it must not trust it.
func (p *Policy) Validate() error {
	if p.Version != PolicyVersion {
		return fmt.Errorf("policy version %d is not supported (want %d)", p.Version, PolicyVersion)
	}
	switch p.Net {
	case NetOpen, NetNone, NetProxy:
	default:
		return fmt.Errorf("unknown network mode %q", p.Net)
	}
	if len(p.Argv) == 0 || p.Argv[0] == "" {
		return errors.New("policy has no command")
	}
	for _, list := range [][]string{p.Exec, p.Read, p.Write} {
		for _, s := range list {
			if !filepath.IsAbs(s) {
				return fmt.Errorf("policy path %q is not absolute", s)
			}
			if strings.ContainsRune(s, 0) {
				return errors.New("policy path contains a NUL byte")
			}
		}
	}
	if p.Dir != "" && !filepath.IsAbs(p.Dir) {
		return fmt.Errorf("policy directory %q is not absolute", p.Dir)
	}
	if p.Net != NetProxy && len(p.ConnectPorts) > 0 {
		return errors.New("connect ports only apply to the proxy network mode")
	}
	return nil
}

// ParsePolicy decodes and validates a serialized policy.
func ParsePolicy(b []byte) (*Policy, error) {
	var p Policy
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("invalid policy: %w", err)
	}
	if dec.More() {
		return nil, errors.New("invalid policy: trailing data")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Marshal serializes the policy.
func (p *Policy) Marshal() ([]byte, error) {
	if p.Version == 0 {
		p.Version = PolicyVersion
	}
	return json.Marshal(p)
}

// Clean returns the paths made absolute and cleaned, with symlinks resolved when the path exists, because the
// kernel rules attach to the real location.
func cleanPaths(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		if s == "" {
			continue
		}
		s = filepath.Clean(s)
		if r, err := filepath.EvalSymlinks(s); err == nil {
			s = r
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

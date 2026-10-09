package jactionlint

import (
	"net/netip"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// This file has the helpers which the download and credential rules (unverified-download,
// insecure-ssh-keyscan, insecure-url-scheme, checkout-static-credentials) share.

// shellScope tracks the shell that a `run:` step uses, so that a rule analyzes only the scripts that
// are written for bash or sh. The default shell of Windows runners is pwsh, and the analyzer does not
// understand it, so such scripts are skipped.
type shellScope struct {
	workflowShell string
	jobShell      string
	windows       bool
}

func (s *shellScope) enterWorkflow(w *Workflow) {
	s.workflowShell = ""
	if w.Defaults != nil && w.Defaults.Run != nil && w.Defaults.Run.Shell != nil {
		s.workflowShell = w.Defaults.Run.Shell.Value
	}
}

func (s *shellScope) enterJob(j *Job) {
	s.jobShell, s.windows = "", false
	if j.Defaults != nil && j.Defaults.Run != nil && j.Defaults.Run.Shell != nil {
		s.jobShell = j.Defaults.Run.Shell.Value
	}
	if j.RunsOn != nil {
		for _, l := range j.RunsOn.Labels {
			v := strings.ToLower(l.Value)
			if v == "windows" || strings.HasPrefix(v, "windows-") {
				s.windows = true
			}
		}
	}
}

func (s *shellScope) leaveJob() {
	s.jobShell, s.windows = "", false
}

// analyze analyzes the script of the step. It returns nil when the script is not written for bash or
// sh, or does not parse. The origin maps positions of the script to the file.
func (s *shellScope) analyze(run *ExecRun) (*runscript.Script, runscript.Origin) {
	if run == nil || run.Run == nil || run.Run.Pos == nil {
		return nil, runscript.Origin{}
	}
	shell := ""
	switch {
	case run.Shell != nil:
		shell = run.Shell.Value
	case s.jobShell != "":
		shell = s.jobShell
	case s.workflowShell != "":
		shell = s.workflowShell
	case s.windows:
		return nil, runscript.Origin{}
	}
	if strings.Contains(shell, "${{") {
		return nil, runscript.Origin{}
	}
	sc, err := runscript.Analyze(run.Run.Value, shell)
	if err != nil {
		return nil, runscript.Origin{}
	}
	return sc, run.Run.scriptOrigin()
}

// urlParts splits a URL which starts with one of the schemes. ok is false when v is not such a URL.
// host is lower case, without the user information and the port, and empty when it is not known
// statically (it contains an expression or a variable).
func urlParts(v string) (scheme, host string, ok bool) {
	i := strings.Index(v, "://")
	if i <= 0 {
		return "", "", false
	}
	scheme = strings.ToLower(v[:i])
	rest := v[i+3:]
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	if j := strings.LastIndexByte(rest, '@'); j >= 0 {
		rest = rest[j+1:]
	}
	switch {
	case strings.HasPrefix(rest, "["): // IPv6
		if j := strings.IndexByte(rest, ']'); j >= 0 {
			rest = rest[1:j]
		}
	default:
		if j := strings.LastIndexByte(rest, ':'); j >= 0 {
			rest = rest[:j]
		}
	}
	if strings.ContainsAny(rest, "${}`\\\"' ") {
		return scheme, "", true
	}
	return scheme, strings.ToLower(strings.TrimSuffix(rest, ".")), true
}

// isLocalHost reports whether the host cannot be reached over the internet, so that a plain text
// protocol to it does not cross an untrusted network: loopback and private addresses, `localhost`,
// the names of containers and services (a host without a dot) and the reserved local domains.
func isLocalHost(host string) bool {
	if host == "" {
		return true
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsUnspecified()
	}
	if !strings.Contains(host, ".") {
		return true
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".svc", ".lan", ".test", ".home.arpa", ".invalid"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// matchesAllowedURL reports whether the URL is accepted by an entry of the "allow" option of a rule: an
// entry with "://" is a prefix of the URL, any other entry is a host name.
func matchesAllowedURL(allow []string, url string) bool {
	_, host, _ := urlParts(url)
	for _, a := range allow {
		a = strings.TrimSpace(a)
		switch {
		case a == "":
		case strings.Contains(a, "://"):
			if strings.HasPrefix(strings.ToLower(url), strings.ToLower(a)) {
				return true
			}
		case host != "" && strings.EqualFold(a, host):
			return true
		}
	}
	return false
}

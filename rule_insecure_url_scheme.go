package jactionlint

import (
	"sort"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleInsecureURLScheme is a rule checker which reports URLs with a scheme that sends data in the clear
// or without authenticating the server (`http://`, `ftp://` and `git://`) where a workflow fetches
// something from a remote host: the URLs that `curl`, `wget`, `git` and the package managers are given
// in a `run:` script, and inputs of actions in `with:` whose whole value is such a URL.
//
// URLs of hosts that cannot be reached over the internet are not reported (loopback and private
// addresses, `localhost`, names without a dot such as the services of a job, and the reserved local
// domains), nor are URLs whose host is an expression or a variable. A proxy given to `curl`/`wget` and
// the XML namespaces of text are out of scope: only the location a command downloads from is checked.
//
// There is no fix: whether the host serves the same content over HTTPS is not known.
type RuleInsecureURLScheme struct {
	RuleBase
	shell shellScope
}

// NewRuleInsecureURLScheme creates a new RuleInsecureURLScheme instance.
func NewRuleInsecureURLScheme() *RuleInsecureURLScheme {
	return &RuleInsecureURLScheme{
		RuleBase: RuleBase{
			name: "insecure-url-scheme",
			desc: "Checks that downloads use https and not an unencrypted scheme like http, ftp or git",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleInsecureURLScheme) VisitWorkflowPre(n *Workflow) error {
	rule.shell.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleInsecureURLScheme) VisitJobPre(n *Job) error {
	rule.shell.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleInsecureURLScheme) VisitJobPost(n *Job) error {
	rule.shell.leaveJob()
	return nil
}

var insecureSchemes = map[string]string{"http": "https", "ftp": "https", "git": "https"}

// insecureURL reports whether the value is a URL with an insecure scheme for a host on the internet. It
// returns the URL with the secure scheme.
func insecureURL(v string) (string, bool) {
	v = strings.TrimSpace(v)
	scheme, host, ok := urlParts(v)
	if !ok {
		return "", false
	}
	secure, bad := insecureSchemes[scheme]
	if !bad || host == "" || isLocalHost(host) {
		return "", false
	}
	// git+http://host is git+https://host
	i := strings.Index(v, "://")
	return v[:i-len(scheme)] + secure + v[i:], true
}

// VisitStep is callback when visiting Step node.
func (rule *RuleInsecureURLScheme) VisitStep(n *Step) error {
	switch e := n.Exec.(type) {
	case *ExecAction:
		names := make([]string, 0, len(e.Inputs))
		for name := range e.Inputs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			in := e.Inputs[name]
			if in == nil || in.Value == nil || strings.ContainsAny(strings.TrimSpace(in.Value.Value), " \t\n") {
				continue
			}
			if secure, ok := insecureURL(in.Value.Value); ok {
				pos := in.Value.Pos
				if pos == nil {
					pos = in.Name.Pos
				}
				rule.ReportIDf("insecure-url-scheme", pos, "the input %q is %q, which is fetched without encryption or authentication of the server, so anyone on the connection can read or replace what is transferred. use %q", name, strings.TrimSpace(in.Value.Value), secure)
			}
		}
	case *ExecRun:
		sc, origin := rule.shell.analyze(e)
		if sc == nil {
			return nil
		}
		for _, c := range sc.Commands {
			if !fetchesFromURL(c) {
				continue
			}
			for _, w := range urlWords(c) {
				if secure, ok := insecureURL(w.Value); ok {
					rule.ReportIDf("insecure-url-scheme", scriptPos(sc, origin, w.Offset), "%q is fetched by %q without encryption or authentication of the server, so anyone on the connection can read or replace what is transferred. use %q", w.Value, c.Name, secure)
				}
			}
		}
	}
	return nil
}

// fetchesFromURL reports whether the command takes locations to download from or to install from.
func fetchesFromURL(c *runscript.Command) bool {
	switch c.Tool {
	case "curl", "wget", "pip", "pipx", "uv", "uvx", "npm", "pnpm", "yarn", "bun", "cargo", "gem", "go", "poetry":
		return true
	}
	switch c.Name {
	case "aria2c", "axel", "http", "https", "svn":
		return true
	case "git":
		// `git config url.https://github.com/.insteadOf git://github.com/` rewrites the insecure URL: not a fetch
		for _, w := range c.Positional {
			switch w.Value {
			case "clone", "fetch", "pull", "push", "remote", "submodule", "ls-remote":
				return true
			}
		}
	}
	return false
}

// urlWords returns the words of the command that are a location: the arguments, and the values of options
// such as --index-url=URL. The proxy options of curl and wget name a proxy and not a download.
func urlWords(c *runscript.Command) []*runscript.Word {
	var out []*runscript.Word
	out = append(out, c.Positional...)
	for _, f := range c.Flags {
		if f.Value == nil {
			continue
		}
		switch f.Name {
		case "-x", "--proxy", "--preproxy", "--noproxy", "--proxy-header", "--header", "-H", "-e", "--referer", "-d", "--data", "--data-raw",
			"--data-binary", "--data-urlencode", "-F", "--form", "-w", "--write-out", "--post-data", "--body-data", "-u", "--user":
			continue
		}
		out = append(out, f.Value)
	}
	return out
}

func init() {
	registerRules(RuleInfo{
		ID: "insecure-url-scheme", Group: RuleGroupSecurity, Summary: "A download uses http, ftp or git instead of https.",
		DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-insecure-url-scheme",
	})
	registerRuleFactory("insecure-url-scheme", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("insecure-url-scheme") {
			return nil
		}
		return []Rule{NewRuleInsecureURLScheme()}
	})
}

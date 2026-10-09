package jactionlint

import (
	"path"
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleUnverifiedDownload is a rule checker which reports code that a `run:` script downloads and runs
// without checking it:
//
//   - a download piped into a shell or an interpreter (`curl ... | sh`, `wget -O- ... | sudo bash`,
//     `bash <(curl ...)`, `sh -c "$(curl ...)"`, `curl ... | python3 -`);
//   - a downloaded file that the same script makes executable or runs (or installs with dpkg, rpm or apt)
//     without a checksum or signature check in between;
//   - a download made with TLS certificate verification turned off.
//
// A pipe into a program that is not an interpreter (`curl ... | tar -x`, `| jq`) is not reported. Neither is
// a URL that names a full git commit (`raw.githubusercontent.com/o/r/<sha>/install.sh`), because the URL
// itself fixes the content, a loopback or private host, or a URL accepted by the "allow" option.
type RuleUnverifiedDownload struct {
	RuleBase
	shell shellScope
}

// NewRuleUnverifiedDownload creates a new RuleUnverifiedDownload instance.
func NewRuleUnverifiedDownload() *RuleUnverifiedDownload {
	return &RuleUnverifiedDownload{
		RuleBase: RuleBase{
			name: "unverified-download",
			desc: "Checks that scripts do not run what they download without verifying it",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnverifiedDownload) VisitWorkflowPre(n *Workflow) error {
	rule.shell.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUnverifiedDownload) VisitJobPre(n *Job) error {
	rule.shell.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleUnverifiedDownload) VisitJobPost(n *Job) error {
	rule.shell.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleUnverifiedDownload) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok {
		return nil
	}
	sc, origin := rule.shell.analyze(run)
	if sc == nil {
		return nil
	}
	allow := rule.Config().ruleOptionStrings("unverified-download", "allow")
	a := &downloadAnalysis{rule: rule, sc: sc, origin: origin, allow: allow}
	a.run()
	return nil
}

const downloadRemedy = "if it installs a tool, install the tool with mise instead (jdx/mise-action pinned by SHA, or \"mise use\" with a committed mise.lock, which records the version and checksum of each tool). otherwise download the file, check its checksum or signature (sha256sum -c, gpg --verify, cosign verify-blob or gh attestation verify) before running it, or use the package of the vendor"

type download struct {
	cmd      *runscript.Command
	url      *runscript.Word
	verified bool
	reported bool
}

type downloadAnalysis struct {
	rule   *RuleUnverifiedDownload
	sc     *runscript.Script
	origin runscript.Origin
	allow  []string

	seen map[int]bool // offsets of the downloaders already reported
}

func (a *downloadAnalysis) report(offset int, format string, args ...any) {
	if a.seen == nil {
		a.seen = map[int]bool{}
	}
	if a.seen[offset] {
		return
	}
	a.seen[offset] = true
	a.rule.ReportIDf("unverified-download", scriptPos(a.sc, a.origin, offset), format, args...)
}

func (a *downloadAnalysis) run() {
	a.pipes()
	a.insecureTLS()
	a.files()
}

// trusted reports whether the URL is one the rule does not judge.
func (a *downloadAnalysis) trusted(u *runscript.Word) bool {
	if u == nil {
		return false
	}
	if matchesAllowedURL(a.allow, u.Value) {
		return true
	}
	_, host, ok := urlParts(u.Value)
	if !ok {
		return false
	}
	if host != "" && isLocalHost(host) {
		return true
	}
	return isCommitPinnedURL(u.Value, host)
}

var reCommitPath = regexp.MustCompile(`/[0-9a-f]{40}(/|$|\?|#)`)

// isCommitPinnedURL reports whether the URL names a full commit on a code hosting site, so that its
// content cannot change.
func isCommitPinnedURL(u, host string) bool {
	switch {
	case host == "github.com", host == "raw.githubusercontent.com", host == "gist.githubusercontent.com",
		host == "gitlab.com", host == "codeberg.org", host == "bitbucket.org":
		return reCommitPath.MatchString(u)
	}
	return false
}

func urlText(u *runscript.Word) string {
	if u == nil {
		return "an unknown URL"
	}
	return u.Value
}

func (a *downloadAnalysis) pipes() {
	for _, sp := range a.sc.ShellPipes() {
		if a.trusted(sp.URL) {
			continue
		}
		what := shellPipeText(sp)
		a.report(sp.Downloader.Offset, "the script downloaded from %q is run by %s without being verified, so whoever controls that server or the connection to it controls this job. %s", urlText(sp.URL), what, downloadRemedy)
	}
	// downloads piped into an interpreter other than a shell
	for _, p := range a.sc.Pipelines {
		for i := 1; i < len(p.Stages); i++ {
			for _, c := range p.Stages[i].Commands {
				if c.Pipeline != p || c.Stage != i || !readsScriptFromStdin(c) {
					continue
				}
				for _, st := range p.Stages[:i] {
					for _, d := range st.Commands {
						if d.Tool != "curl" && d.Tool != "wget" {
							continue
						}
						u := downloadURLOf(d)
						if a.trusted(u) {
							continue
						}
						a.report(d.Offset, "the script downloaded from %q is run by %q without being verified, so whoever controls that server or the connection to it controls this job. %s", urlText(u), c.Name, downloadRemedy)
					}
				}
			}
		}
	}
}

func shellPipeText(sp *runscript.ShellPipe) string {
	name := sp.Shell.Name
	if sp.Form == "subst" {
		return "\"" + name + "\" through a command substitution"
	}
	return "\"" + name + "\""
}

// isScriptInterpreter reports whether the command is an interpreter other than a shell.
func isScriptInterpreter(c *runscript.Command) bool {
	if c.Tool != "" || c.Name == "" {
		return false
	}
	return rePythonName.MatchString(c.Name) || c.Name == "perl" || c.Name == "ruby" || c.Name == "node" || c.Name == "php"
}

// interpreterScript reports a downloaded file that the interpreter runs as its script. With -r the first
// operand may be the module to load (node -r mod script.js), so every operand is looked at.
func (t *fileTracker) interpreterScript(c *runscript.Command, match func(*runscript.Word) *download) {
	ops := c.Positional
	if len(ops) > 1 && !c.HasFlag("-r") {
		ops = ops[:1]
	}
	for _, w := range ops {
		if d := match(w); d != nil {
			t.report(d, w.Value, "is run by "+c.Name)
			return
		}
	}
}

// interpreterRunsAScript reports whether the options of the interpreter leave the operand as the script to
// run: with -c, -m, -e and the like the program is given in the options.
func interpreterRunsAScript(c *runscript.Command) bool {
	switch {
	case rePythonName.MatchString(c.Name):
		return !c.HasFlag("-c", "-m")
	case c.Name == "perl":
		return !c.HasFlag("-e", "-E")
	case c.Name == "ruby":
		return !c.HasFlag("-e")
	case c.Name == "node":
		return !c.HasFlag("-e", "--eval", "-p", "--print")
	case c.Name == "php":
		return !c.HasFlag("-r")
	}
	return false
}

var rePythonName = regexp.MustCompile(`^(python|py)[0-9.]*$`)

// readsScriptFromStdin reports whether the command is an interpreter (other than the shells, which
// ShellPipes covers) that runs the script it reads from its standard input.
func readsScriptFromStdin(c *runscript.Command) bool {
	if c.Tool != "" || c.Name == "" {
		return false
	}
	switch {
	case rePythonName.MatchString(c.Name):
		if c.HasFlag("-c", "-m") {
			return false
		}
	case c.Name == "perl", c.Name == "ruby", c.Name == "node", c.Name == "php":
		if len(c.Flags) > 0 {
			return false
		}
	default:
		return false
	}
	if len(c.Positional) == 0 {
		return true
	}
	return len(c.Positional) == 1 && c.Positional[0].Value == "-"
}

// downloadURLOf returns the URL a curl or wget command fetches.
func downloadURLOf(c *runscript.Command) *runscript.Word {
	if vs := c.FlagValues("--url"); len(vs) > 0 {
		return vs[0]
	}
	for _, w := range c.Positional {
		return w
	}
	return nil
}

func (a *downloadAnalysis) insecureTLS() {
	for _, c := range a.sc.Commands {
		var f *runscript.Flag
		switch c.Tool {
		case "curl":
			f = c.Flag("-k", "--insecure", "--proxy-insecure", "--doh-insecure")
		case "wget":
			f = c.Flag("--no-check-certificate")
		}
		if f == nil {
			continue
		}
		name := f.Name
		if len(name) > 2 && name[0] == '-' && name[1] != '-' {
			name = "-k" // a cluster such as -fsSLk: only -k disables the verification
		}
		a.report(f.Word.Offset, "%q disables the verification of the TLS certificate of the server, so the download can be replaced by anyone on the connection and nothing proves who sent it. remove %q and make the runner trust the certificate (for a private CA, \"--cacert\" of curl or \"--ca-certificate\" of wget)", name, name)
	}
}

// Downloaded files

type fileTracker struct {
	a       *downloadAnalysis
	tracked map[string]*download
}

func (a *downloadAnalysis) files() {
	t := &fileTracker{a: a, tracked: map[string]*download{}}
	var all []*download
	for _, c := range a.sc.Commands {
		switch {
		case c.Tool == "curl" || c.Tool == "wget":
			u := downloadURLOf(c)
			if a.trusted(u) {
				continue
			}
			d := &download{cmd: c, url: u}
			all = append(all, d)
			for _, dest := range a.destinations(c) {
				t.tracked[dest] = d
			}
		case isVerification(c):
			for _, d := range all {
				d.verified = true
			}
		default:
			t.event(c)
		}
	}
}

// destinations returns the files that the curl or wget command writes, normalized.
func (a *downloadAnalysis) destinations(c *runscript.Command) []string {
	var out []string
	add := func(p string) {
		p = normPath(p)
		if p != "" && p != "-" && p != "/dev/stdout" && p != "/dev/null" && !strings.HasSuffix(p, "/") {
			out = append(out, p)
		}
	}
	switch c.Tool {
	case "curl":
		for _, w := range c.FlagValues("-o", "--output") {
			add(w.Value)
		}
		if c.HasFlag("-O", "--remote-name", "--remote-name-all") {
			for _, w := range c.Positional {
				add(urlBase(w.Value))
			}
		}
	case "wget":
		outs := c.FlagValues("-O", "--output-document")
		for _, w := range outs {
			add(w.Value)
		}
		if len(outs) == 0 {
			dir := ""
			if ds := c.FlagValues("-P", "--directory-prefix"); len(ds) > 0 {
				dir = strings.TrimSuffix(ds[0].Value, "/") + "/"
			}
			for _, w := range c.Positional {
				if b := urlBase(w.Value); b != "" {
					add(dir + b)
				}
			}
		}
	}
	for _, r := range a.sc.Redirects {
		if !r.Write || r.Target == nil || r.Cmd == nil || (r.Fd != "" && r.Fd != "1") {
			continue
		}
		if r.Cmd == c || (r.Tee && r.Cmd.Pipeline != nil && r.Cmd.Pipeline == c.Pipeline) {
			add(r.Target.Value)
		}
	}
	return out
}

func urlBase(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
		if j := strings.IndexByte(u, '/'); j >= 0 {
			u = u[j:]
		} else {
			return ""
		}
	}
	b := path.Base(u)
	if b == "/" || b == "." {
		return ""
	}
	return b
}

func normPath(p string) string {
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	return p
}

var reExecMode = regexp.MustCompile(`[+=][rwXst]*x`)

// hasExecMode reports whether a chmod mode (or install -m mode) sets an execute bit.
func hasExecMode(m string) bool {
	if m == "" {
		return false
	}
	if m[0] >= '0' && m[0] <= '7' {
		for _, c := range m {
			if c < '0' || c > '7' {
				return false
			}
		}
		last := m[len(m)-1]
		mid := byte('0')
		if len(m) >= 2 {
			mid = m[len(m)-2]
		}
		first := byte('0')
		if len(m) >= 3 {
			first = m[len(m)-3]
		}
		return (last-'0')&1 == 1 || (mid-'0')&1 == 1 || (first-'0')&1 == 1
	}
	return reExecMode.MatchString(m)
}

var shellNames = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ash": true, "ksh": true}

// event handles a command that may use a downloaded file.
func (t *fileTracker) event(c *runscript.Command) {
	if len(t.tracked) == 0 || c.NameWord == nil {
		return
	}
	match := func(w *runscript.Word) *download {
		if w == nil {
			return nil
		}
		return t.tracked[normPath(w.Value)]
	}
	switch {
	case c.Name == "mv" || c.Name == "cp":
		if len(c.Positional) < 2 {
			return
		}
		dst := c.Positional[len(c.Positional)-1].Value
		for _, w := range c.Positional[:len(c.Positional)-1] {
			if d := match(w); d != nil {
				t.tracked[normPath(dst)] = d
				if strings.HasSuffix(dst, "/") {
					t.tracked[normPath(dst+path.Base(w.Value))] = d
				}
			}
		}
	case c.Name == "chmod":
		if len(c.Positional) < 2 || !hasExecMode(c.Positional[0].Value) {
			return
		}
		for _, w := range c.Positional[1:] {
			if d := match(w); d != nil {
				t.report(d, w.Value, "is made executable")
			}
		}
	case c.Name == "install":
		// install is not a tool the analyzer knows, so its option values are positional words
		mode, args, dst := installOperands(c)
		srcs := args
		if dst == "" {
			if len(args) < 2 {
				return
			}
			dst, srcs = args[len(args)-1].Value, args[:len(args)-1]
		}
		for _, w := range srcs {
			if d := match(w); d != nil {
				t.tracked[normPath(dst)] = d
				t.tracked[normPath(strings.TrimSuffix(dst, "/")+"/"+path.Base(w.Value))] = d
				if hasExecMode(mode) {
					t.report(d, w.Value, "is installed as an executable")
				}
			}
		}
	case strings.Contains(c.NameWord.Value, "/"):
		if d := match(c.NameWord); d != nil {
			t.report(d, c.NameWord.Value, "is run")
		} else if isScriptInterpreter(c) && interpreterRunsAScript(c) {
			t.interpreterScript(c, match) // /usr/bin/python3 install.py
		}
	case isScriptInterpreter(c) && interpreterRunsAScript(c):
		t.interpreterScript(c, match)
	case shellNames[c.Name] && !c.HasFlag("-c"):
		if len(c.Positional) > 0 {
			if d := match(c.Positional[0]); d != nil {
				t.report(d, c.Positional[0].Value, "is run by "+c.Name)
			}
		}
	case c.Tool == "source" || c.Tool == ".":
		if len(c.Positional) > 0 {
			if d := match(c.Positional[0]); d != nil {
				t.report(d, c.Positional[0].Value, "is sourced")
			}
		}
	case c.Name == "dpkg" && c.HasFlag("-i", "--install"), c.Name == "rpm" && c.HasFlag("-i", "-U", "--install", "--upgrade"),
		c.Tool == "apt" && c.Verb() == "install":
		for _, w := range c.Positional {
			if d := match(w); d != nil {
				t.report(d, w.Value, "is installed with "+c.Name)
			}
		}
	}
}

func (t *fileTracker) report(d *download, file, what string) {
	if d.verified || d.reported {
		return
	}
	d.reported = true
	t.a.report(d.cmd.Offset, "the file %q downloaded from %q %s without a checksum or signature check in this script, so whatever the server (or anyone on the connection) sends is trusted. %s", file, urlText(d.url), what, downloadRemedy)
}

// isVerification reports whether the command checks a checksum or a signature.
func isVerification(c *runscript.Command) bool {
	switch c.Name {
	case "sha256sum", "sha512sum", "sha1sum", "sha224sum", "sha384sum", "b2sum", "b3sum", "shasum", "sha256", "sha512":
		// Without -c these only print the hash
		return c.HasFlag("-c") || c.HasFlag("--check")
	case "minisign", "slsa-verifier", "gpgv", "gpgv2", "signify", "rekor-cli", "notation":
		return true
	case "gpg", "gpg2":
		return c.HasFlag("--verify")
	case "cosign":
		return strings.HasPrefix(c.Verb(), "verify")
	case "openssl":
		// Hashing alone prints the digest; a signature is checked with -verify
		return c.Verb() == "dgst" && c.HasFlag("-verify")
	case "gh":
		return (c.Sub(0) == "attestation" || c.Sub(0) == "release") && c.Sub(1) == "verify"
	case "ssh-keygen":
		return c.HasFlag("-Y")
	}
	return false
}

func init() {
	registerRules(RuleInfo{
		ID: "unverified-download", Group: RuleGroupSecurity, Summary: "A script runs what it downloads without verifying it.",
		DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-unverified-download",
		Options: []RuleOption{{Name: "allow", Kind: RuleOptionStrings, Summary: "Hosts, or URL prefixes (entries with \"://\"), whose downloads are accepted without verification."}},
	})
	registerRuleFactory("unverified-download", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("unverified-download") {
			return nil
		}
		return []Rule{NewRuleUnverifiedDownload()}
	})
}

// installOperands returns the mode given to install(1), its operands (sources and destination) and the
// directory given with -t, which is then the destination of all the operands.
func installOperands(c *runscript.Command) (mode string, args []*runscript.Word, target string) {
	for i := 0; i < len(c.Args); i++ {
		v := c.Args[i].Value
		// value takes the value of an option: the rest of the cluster, or the next word
		value := func(rest string) string {
			if rest != "" {
				return rest
			}
			if i+1 < len(c.Args) {
				i++
				return c.Args[i].Value
			}
			return ""
		}
		switch {
		case v == "--mode" || v == "--target-directory":
			if v == "--mode" {
				mode = value("")
			} else {
				target = value("")
			}
		case strings.HasPrefix(v, "--mode="):
			mode = strings.TrimPrefix(v, "--mode=")
		case strings.HasPrefix(v, "--target-directory="):
			target = strings.TrimPrefix(v, "--target-directory=")
		case strings.HasPrefix(v, "--owner") || strings.HasPrefix(v, "--group"):
			if !strings.Contains(v, "=") {
				i++
			}
		case strings.HasPrefix(v, "--"):
		case strings.HasPrefix(v, "-") && v != "-":
			// A cluster of short options such as -Dm755 or -m 755 -t dir
		cluster:
			for j := 1; j < len(v); j++ {
				switch v[j] {
				case 'm':
					mode = value(v[j+1:])
					break cluster
				case 't':
					target = value(v[j+1:])
					break cluster
				case 'o', 'g', 'S':
					value(v[j+1:])
					break cluster
				}
			}
		default:
			args = append(args, c.Args[i])
		}
	}
	return mode, args, target
}

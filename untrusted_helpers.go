package jactionlint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// This file has what untrusted-checkout and untrusted-artifact share: which triggers run with
// privileges while their input is controlled by someone else, how to tell that the workflow that
// produced the input is trusted, which conditions are guards and what "runs the code" means.

// untrustedHeadContexts are the contexts that name the code of a pull request or of the run that
// triggered a workflow_run workflow.
var untrustedHeadContexts = [][]string{
	{"github", "head_ref"},
	{"github", "event", "pull_request", "head"},
	{"github", "event", "pull_request", "merge_commit_sha"},
	{"github", "event", "workflow_run", "head_sha"},
	{"github", "event", "workflow_run", "head_branch"},
	{"github", "event", "workflow_run", "head_commit"},
	{"github", "event", "workflow_run", "head_repository"},
	{"github", "event", "workflow_run", "pull_requests"},
}

// guardContexts are the contexts that a condition reads to restrict a job or a step to code that
// someone trusted: the repository the pull request comes from, the labels a maintainer set, the
// event that started the upstream run and who started it. A job or step with such a condition is
// not reported, because the rules cannot judge whether the condition is sufficient.
var guardContexts = [][]string{
	{"github", "event", "pull_request", "head", "repo"},
	{"github", "event", "workflow_run", "head_repository"},
	{"github", "event", "workflow_run", "event"},
	{"github", "event", "label"},
	{"github", "event", "pull_request", "labels"},
	{"github", "event", "pull_request", "user"},
	{"github", "event", "pull_request", "author_association"},
	{"github", "event", "workflow_run", "actor"},
	{"github", "actor"},
	{"github", "triggering_actor"},
	// an earlier job or step decided whether the code is trusted
	{"needs"},
	{"steps"},
}

func refsMatchAny(refs []exprRef, prefixes [][]string) (exprRef, bool) {
	for _, r := range refs {
		for _, p := range prefixes {
			if refCovers(r.chain, p) {
				return r, true
			}
		}
	}
	return exprRef{}, false
}

// conditionIsGuard reports whether the condition reads one of the guard contexts. An
// unparsable condition is treated as a guard.
func conditionIsGuard(s *String) bool {
	if s == nil {
		return false
	}
	exprs, ok := conditionExprs(s)
	if !ok {
		return true
	}
	var refs []exprRef
	for _, e := range exprs {
		collectExprRefs(e, nil, &refs)
	}
	for _, r := range refs {
		for _, g := range guardContexts {
			if !refCovers(r.chain, g) {
				continue
			}
			if len(r.chain) == 4 && refCovers(r.chain, []string{"github", "event", "workflow_run", "event"}) && !workflowRunEventIsGuard(r) {
				continue
			}
			return true
		}
	}
	return false
}

// workflowRunEventIsGuard reports whether a comparison of the event of the triggering run restricts it
// to events that people with write access cause. `== 'pull_request'` does the opposite.
func workflowRunEventIsGuard(r exprRef) bool {
	c, ok := r.parent.(*CompareOpNode)
	if !ok || !c.Kind.IsEqualityOp() {
		return true
	}
	other := c.Right
	if c.Right == r.node {
		other = c.Left
	}
	lit, ok := other.(*StringNode)
	if !ok {
		return true
	}
	trusted := trustedUpstreamEvents[strings.ToLower(lit.Value)]
	if c.Kind == CompareOpNodeKindEq {
		return trusted
	}
	return !trusted
}

// untrustedValue reports whether the string refers to the head of a pull request or of the triggering
// run, and describes the reference.
func untrustedValue(value string) (string, bool) {
	if strings.Contains(value, "refs/pull/") {
		return value, true
	}
	refs, _ := stringExprRefs(value)
	if r, ok := refsMatchAny(refs, untrustedHeadContexts); ok {
		return r.String(), true
	}
	return "", false
}

// privilegedEvents tells which of the events the workflow handles run with privileges for input that
// someone else controls. A workflow_run event is not one when the workflows it waits for can only be
// started by people with write access.
func privilegedEvents(w *Workflow, project *Project, sib *siblingWorkflows) []string {
	var ret []string
	for _, event := range privilegedTriggers {
		switch event {
		case "pull_request_target":
			if hasEvent(w, event) {
				ret = append(ret, event)
			}
		case "workflow_run":
			for _, we := range webhookEvents(w, event) {
				if !upstreamWorkflowsTrusted(project, sib, we) {
					ret = append(ret, event)
					break
				}
			}
		}
		// issue_comment is not handled: it carries no head to check out, a comment would have to name it
	}
	return ret
}

// trustedUpstreamEvents are the events that only people with write access to the repository can cause.
var trustedUpstreamEvents = map[string]bool{
	"push": true, "schedule": true, "workflow_dispatch": true, "release": true, "merge_group": true, "repository_dispatch": true,
}

// upstreamWorkflowsTrusted reports whether every workflow that the workflow_run event waits for is
// in the project and is started only by events that people with write access cause. It is false when
// it cannot tell: no project, a pattern or an expression as a name, or a workflow that is missing.
func upstreamWorkflowsTrusted(project *Project, sib *siblingWorkflows, we *WebhookEvent) bool {
	if project == nil || len(we.Workflows) == 0 {
		return false
	}
	byName, ok := sib.parsed(project)
	if !ok {
		return false
	}
	for _, n := range we.Workflows {
		if n == nil || n.ContainsExpression() || strings.ContainsAny(n.Value, "*?[]!+") {
			return false
		}
		w, ok := byName[strings.ToLower(n.Value)]
		if !ok || len(w.On) == 0 {
			return false
		}
		for _, e := range w.On {
			if !trustedUpstreamEvents[e.EventName()] {
				return false
			}
		}
	}
	return true
}

// nonExecutingCommands are commands that read, move, print or fetch files without running them.
var nonExecutingCommands = map[string]bool{
	"echo": true, "printf": true, "cat": true, "ls": true, "grep": true, "egrep": true, "fgrep": true, "sed": true,
	"awk": true, "find": true, "test": true, "[": true, "[[": true, "cp": true, "mv": true, "rm": true, "mkdir": true,
	"touch": true, "chmod": true, "chown": true, "tar": true, "unzip": true, "zip": true, "gzip": true, "gunzip": true,
	"jq": true, "yq": true, "git": true, "gh": true, "curl": true, "wget": true, "true": true, "false": true,
	"sleep": true, "cd": true, "pwd": true, "export": true, "set": true, "unset": true, "head": true, "tail": true,
	"wc": true, "sort": true, "uniq": true, "diff": true, "date": true, "tr": true, "cut": true, "basename": true,
	"dirname": true, "realpath": true, "readlink": true, "sha256sum": true, "sha1sum": true, "md5sum": true,
	"shasum": true, "ln": true, "stat": true, "du": true, "df": true, "which": true, "type": true, "read": true,
	"local": true, "declare": true, "readonly": true, "shift": true, "exit": true, "return": true, "printenv": true,
	"tee": true, "mktemp": true, "rsync": true, "xz": true, "bzip2": true, "7z": true, "7za": true, "file": true,
	"column": true, "nl": true, "paste": true, "comm": true, "join": true, "split": true, "tac": true, "rev": true,
	"id": true, "whoami": true, "uname": true, "hostname": true, "env": true, "trap": true, "wait": true,
	// shell keywords and builtins that never start a program
	"break": true, "continue": true, "compgen": true, "complete": true, "alias": true, "unalias": true, "let": true,
	"getopts": true, "hash": true, "umask": true, "ulimit": true, "typeset": true, "jobs": true, "bg": true, "fg": true,
	"disown": true, "dirs": true, "popd": true, "pushd": true, "history": true, "mapfile": true, "readarray": true,
	"caller": true, "enable": true, "shopt": true, "bind": true, ":": true, "{": true, "}": true, "!": true,
	// programs that print, convert or count data
	"xxd": true, "od": true, "hexdump": true, "base64": true, "strings": true, "seq": true, "expr": true, "nproc": true,
	"tput": true, "yes": true, "sync": true, "lsb_release": true, "fold": true, "fmt": true, "iconv": true, "less": true,
	"more": true, "ps": true, "free": true, "lscpu": true, "cmp": true, "ssh-keyscan": true, "ping": true,
	"nslookup": true, "dig": true, "systemctl": true, "service": true, "update-ca-certificates": true, "ldconfig": true,
}

// packageInstallers are the system package managers. They run no code of the workspace unless a
// package file of it is installed.
var packageInstallers = map[string]bool{"apt-get": true, "apt": true, "apt-cache": true, "aptitude": true, "dpkg": true, "dnf": true, "yum": true, "apk": true, "zypper": true, "pacman": true}

// runsFromOtherDir reports whether the command is an interpreter that runs a script from outside the
// directory, so the directory is only data in its arguments: `python3 base/check.py pr-head`.
func runsFromOtherDir(c *runscript.Command, dir string) bool {
	if dir == "" || len(c.Wrappers) > 0 {
		return false
	}
	switch c.Name {
	case "python", "python3", "node", "ruby", "perl", "bash", "sh", "deno":
	default:
		return false
	}
	if c.HasFlag("-m", "-c", "-e", "-p", "-r", "-i", "--eval", "--import", "--require") {
		return false
	}
	if len(c.Positional) == 0 || c.Positional[0].Dynamic() {
		return false
	}
	script := normalizeDir(c.Positional[0].Value)
	return script != "" && !strings.HasPrefix(script, "-") && !wordIsUnder(c.Positional[0], dir)
}

// commandRunsCode reports whether the command may run code that is in the workspace: a script, a
// program, an interpreter, a build tool.
func commandRunsCode(c *runscript.Command) bool {
	if c.NameWord == nil {
		return false
	}
	if c.Name == "" || c.NameWord.Dynamic() {
		return true // the command is not known
	}
	if strings.Contains(c.NameWord.Value, "/") {
		return true // ./script.sh, build/tool
	}
	if packageInstallers[c.Name] {
		for _, w := range c.Words {
			if strings.Contains(w.Value, "/") || strings.HasSuffix(w.Value, ".deb") || strings.HasSuffix(w.Value, ".rpm") {
				return true // a package file of the workspace
			}
		}
		return false
	}
	if c.Name == "enable" {
		// `enable -f ./lib.so name` loads a shared object, whose code then runs in the shell
		for _, a := range c.Args {
			if !a.Dynamic() && strings.HasPrefix(a.Value, "-") && !strings.HasPrefix(a.Value, "--") && strings.Contains(a.Value, "f") {
				return true
			}
		}
	}
	return !nonExecutingCommands[c.Name]
}

// buildActions are actions that run what is in the workspace.
var buildActions = map[string]bool{
	"github/codeql-action/autobuild": true,
	"github/codeql-action/analyze":   true,
	"pre-commit/action":              true,
	"golangci/golangci-lint-action":  true,
}

// normalizeDir turns the path of a checkout or a download into a relative directory, "" for the
// workspace root.
func normalizeDir(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/") // Windows runners
	p = strings.TrimPrefix(p, "${{ github.workspace }}/")
	p = strings.TrimPrefix(p, "$GITHUB_WORKSPACE/")
	for strings.HasPrefix(p, "./") {
		p = strings.TrimPrefix(p, "./")
	}
	p = strings.TrimRight(p, "/")
	if p == "." {
		return ""
	}
	return p
}

// wordIsUnder reports whether the word names the directory or something below it.
func wordIsUnder(w *runscript.Word, dir string) bool {
	if dir == "" || w == nil {
		return false
	}
	v := normalizeDir(w.Value)
	return v == dir || strings.HasPrefix(v, dir+"/")
}

// commandTouches reports whether the command or the directory it runs in refers to the directory.
func commandTouches(c *runscript.Command, dir string) bool {
	for _, w := range c.Words {
		if wordIsUnder(w, dir) {
			return true
		}
	}
	return false
}

// stepWorkdir returns the working directory of a run step, normalized.
func stepWorkdir(s *Step) string {
	if e, ok := s.Exec.(*ExecRun); ok && e.WorkingDirectory != nil {
		return normalizeDir(e.WorkingDirectory.Value)
	}
	return ""
}

// stepRunsCode reports whether the step may run code from the directory ("" is the whole workspace)
// and describes how. A run step whose script cannot be analyzed counts as running code. The
// commands of the script that start before offset (a position in the script) are ignored.
func stepRunsCode(s *Step, dir string, after int) (string, bool) {
	switch e := s.Exec.(type) {
	case *ExecAction:
		if e.Uses == nil || e.Uses.ContainsExpression() {
			return "", false
		}
		u := ParseUses(e.Uses.Value)
		switch {
		case u.Kind == UsesLocal:
			// An action below the directory of the checkout is code of the checkout. With a checkout in a
			// subdirectory, an action elsewhere in the workspace is not.
			if v := normalizeDir(e.Uses.Value); dir != "" && v != dir && !strings.HasPrefix(v, dir+"/") {
				return "", false
			}
			return "the local action " + e.Uses.Value, true
		case u.Kind == UsesAction && buildActions[strings.ToLower(u.CanonicalName())]:
			if dir != "" {
				return "", false
			}
			return u.CanonicalName(), true
		}
	case *ExecRun:
		if e.Run == nil {
			return "", false
		}
		script, _ := analyzeRun(e)
		wd := stepWorkdir(s)
		if script == nil {
			if after > 0 {
				return "", false
			}
			// The script is not bash, so its commands are not known: look for the directory in the text
			if dir != "" && !(wd == dir || strings.HasPrefix(wd, dir+"/")) && !strings.Contains(strings.ReplaceAll(e.Run.Value, "\\", "/"), dir) {
				return "", false
			}
			return "a script that is not bash", true
		}
		inDir := dir != "" && (wd == dir || strings.HasPrefix(wd, dir+"/"))
		for _, c := range script.Commands {
			if dir != "" && (c.Name == "cd" || c.Name == "pushd") && len(c.Positional) > 0 && wordIsUnder(c.Positional[0], dir) {
				inDir = true // the commands after it run in the directory
				continue
			}
			if c.Offset < after || !commandRunsCode(c) {
				continue
			}
			if dir != "" && !inDir && (!commandTouches(c, dir) || runsFromOtherDir(c, dir)) {
				continue
			}
			name := c.Name
			if name == "" {
				name = "a command"
			}
			return "\"" + name + "\"", true
		}
	}
	return "", false
}

var validationRegex = regexp.MustCompile(`=~|\bgrep\b[^\n|;&]*\s-[a-zA-Z]*[ExPwo]|\bgrep\b[^\n|;&]*\s-e\b|sha(1|256|512)sum\b[^\n]*(-c|--check)|\bshasum\b[^\n]*(-c|--check)|gh\s+attestation\s+verify|cosign\s+verify|gpg\s+--verify|minisign\s+-V|\bjq\b[^\n]*\s-e\b|\s-(eq|ne|gt|ge|lt|le)\s|\bcase\b[^\n]*\bin\b|tr\s+-[a-z]*d|isdigit|isnumeric`)

// scriptValidates reports whether the run script checks or sanitizes data: a regular expression
// match, a checksum or signature verification, a numeric comparison or deleting characters.
func scriptValidates(s *Step) bool {
	e, ok := s.Exec.(*ExecRun)
	return ok && e.Run != nil && validationRegex.MatchString(e.Run.Value)
}

// untrustedEnvNames returns the names of the environment variables of the workflow, the job and the step whose
// value refers to something untrusted. The variables of the workflow are inherited by every step.
func untrustedEnvNames(wf *Workflow, j *Job, s *Step) map[string]bool {
	names := map[string]bool{}
	var wfEnv *Env
	if wf != nil {
		wfEnv = wf.Env
	}
	for _, env := range []*Env{wfEnv, j.Env, s.Env} {
		if env == nil {
			continue
		}
		for _, v := range env.Vars {
			if v.Value != nil {
				if _, bad := untrustedValue(v.Value.Value); bad {
					names[v.Name.Value] = true
				}
			}
		}
	}
	return names
}

// siblingWorkflows reads the workflows of a project for the rules that look at the workflows a
// workflow_run event waits for. A project with hundreds of workflows would otherwise be read and parsed
// again for each of them. One value serves the files linted in one run (LocalActionsCache.siblings), so
// the files are read when the first rule asks and not again.
type siblingWorkflows struct {
	parseOnce sync.Once
	byName    map[string]*Workflow
	parseOK   bool

	namesOnce sync.Once
	nameSet   workflowNames
}

// parsed returns the workflows of the project by their lower-cased names, and false when they could not
// be determined: a file that cannot be read or parsed, or a name with an expression. A nil value reads
// the files on every call.
func (s *siblingWorkflows) parsed(project *Project) (map[string]*Workflow, bool) {
	if s == nil {
		return readParsedWorkflows(project)
	}
	s.parseOnce.Do(func() { s.byName, s.parseOK = readParsedWorkflows(project) })
	return s.byName, s.parseOK
}

// names returns the names of the workflows of the project (see readWorkflowNames).
func (s *siblingWorkflows) names(project *Project) workflowNames {
	if s == nil {
		return readWorkflowNames(project.WorkflowsDir(), project.RootDir())
	}
	s.namesOnce.Do(func() { s.nameSet = readWorkflowNames(project.WorkflowsDir(), project.RootDir()) })
	return s.nameSet
}

func readParsedWorkflows(project *Project) (map[string]*Workflow, bool) {
	dir := project.WorkflowsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	byName := map[string]*Workflow{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if e.IsDir() || (ext != ".yml" && ext != ".yaml") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, false
		}
		w, _ := Parse(b)
		if w == nil {
			return nil, false
		}
		name := ""
		if w.Name != nil && w.Name.Value != "" {
			name = w.Name.Value
		} else if rel, err := filepath.Rel(project.RootDir(), p); err == nil {
			name = filepath.ToSlash(rel)
		}
		if strings.Contains(name, "${{") {
			return nil, false
		}
		byName[strings.ToLower(name)] = w
	}
	return byName, true
}

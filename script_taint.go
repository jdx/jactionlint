package jactionlint

import (
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// This file tells which commands of a script print text that an outsider wrote. The data flow of github-env (see
// rule_github_env.go) starts from the expressions of the workflow and the variables of the environment; here it
// also starts from the output of
//
//   - `gh pr view` and `gh issue view` with `--json` fields that hold free text (title, body, comments, the head ref,
//     the files and the author of a pull request), and `gh api repos/.../pulls` and `.../issues` with a `--jq` filter that
//     selects such a property, or without a filter (the JSON holds them all);
//   - `git log`, `git show`, `git diff`, `git branch --show-current` and the like, once the pull request is checked
//     out by an earlier step of the job or by the script: the commit messages, the names of the files, the author
//     and the name of the branch are then what the pull request says. Without such a checkout the history is the one
//     of the repository, which the rules leave alone.
//
// The lists are the models of CodeQL (actions/ql/lib/ext/config/untrusted_gh_command.yml and
// untrusted_git_command.yml), which also have the kind of each source. A source that is one line (a title, a branch)
// cannot add a record to $GITHUB_OUTPUT; the others can.
type taintSource struct {
	// desc names the command: `gh pr view --json title`.
	desc string
	// kinds says what the text is, see kindText.
	kinds int
}

// What a source prints. A title or a subject is one line of free text, a body, comments, the files of a pull request or a
// commit message can have many lines, and the name of a branch or of an account is one line that git and GitHub restrict
// (it cannot hold a line break). CodeQL calls the last kind "branch" and "username" and does not count it as a source for
// the injection into $GITHUB_ENV.
const (
	kindText  = 1
	kindMulti = 2
	kindName  = 4
)

// multiline is whether the text can hold a line break.
func (t taintSource) multiline() bool { return t.kinds&kindMulti != 0 }

// nameOnly is whether the text is only the name of a branch or of an account.
func (t taintSource) nameOnly() bool { return t.kinds == kindName }

// ghTaintedFields are the fields of `--json` of `gh pr view` and `gh issue view` that hold text of outsiders, and what
// each is.
var ghTaintedFields = map[string]map[string]int{
	"pr": {
		"headRefName": kindName, "title": kindText, "body": kindMulti, "comments": kindMulti, "files": kindMulti,
		"author": kindName, "reviews": kindMulti, "latestReviews": kindMulti, "commits": kindMulti,
	},
	"issue": {"title": kindText, "body": kindMulti, "comments": kindMulti, "author": kindName},
}

// ghJQTokens are the properties that a `--jq` filter of `gh api` or `gh pr view` selects when it reads text of an
// outsider.
var ghJQTokens = []struct {
	re   *regexp.Regexp
	kind int
}{
	{regexp.MustCompile(`\.body\b|\bbody\b|messageBody|\.message\b|\.filename\b|\bpath\b`), kindMulti},
	{regexp.MustCompile(`\.title\b|\btitle\b|messageHeadline`), kindText},
	{regexp.MustCompile(`headRefName|\.head\.ref\b|\.head\b|\.user\b|\.login\b|\bauthor\b`), kindName},
}

var (
	reGhAPIPath    = regexp.MustCompile(`(^|/)repos/[^\s]+/(pulls|issues)(/|$)`)
	reGhTextPath   = regexp.MustCompile(`/(comments|reviews|files|commits)\b`)
	rePullRefText  = regexp.MustCompile(`refs/pull/|\bpull/[^\s/]+/(head|merge)\b`)
	gitSourceVerbs = map[string]bool{"log": true, "show": true, "diff": true, "diff-tree": true, "branch": true, "rev-parse": true, "symbolic-ref": true}
	// the placeholders of a pretty format that print text of an outsider, by kind
	reGitSubject = regexp.MustCompile(`%(s|ae|aE|ce|cE)\b|oneline`)
	reGitBody    = regexp.MustCompile(`%(b|B|N)\b`)
	reGitName    = regexp.MustCompile(`%(an|aN|cn|cN)\b`)
)

// scriptTaint returns the source of text that the command prints when it is one, see the top of the file.
func (rule *RuleGitHubEnv) scriptTaint(s *runscript.Script, c *runscript.Command) (taintSource, bool) {
	switch c.Name {
	case "gh":
		return ghTaint(c)
	case "git":
		if verb, ok := gitSourceVerb(c); ok && rule.prHeadCheckedOut(s, c) {
			return gitTaint(c, verb)
		}
	}
	return taintSource{}, false
}

// jqFilter returns the static text of the `--jq` or `-q` option of the command, and whether there is one.
func jqFilter(c *runscript.Command) (string, bool) {
	vals := c.FlagValues("--jq", "-q")
	if len(vals) == 0 {
		return "", false
	}
	if w := vals[0]; w != nil && !w.Dynamic() {
		return w.Value, true
	}
	return "", true // a filter that is not known: it may select anything
}

func ghTaint(c *runscript.Command) (taintSource, bool) {
	switch c.Verb() {
	case "pr", "issue":
		if c.Sub(1) != "view" {
			return taintSource{}, false
		}
		fields := ghTaintedFields[c.Verb()]
		kinds := 0
		for _, f := range c.FlagValues("--json") {
			if f.Dynamic() {
				return taintSource{desc: ghDesc(c), kinds: kindMulti}, true // the fields are not known
			}
			for _, name := range strings.Split(f.Value, ",") {
				kinds |= fields[strings.TrimSpace(name)]
			}
		}
		if kinds == 0 {
			return taintSource{}, false
		}
		if jq, hasJQ := jqFilter(c); hasJQ {
			// the filter decides what is printed: a filter that reads none of the free text properties (`.number`)
			// clears the fields, one that reads a title is one line
			if sel, ok := jqSelects(jq); ok {
				if sel == 0 {
					return taintSource{}, false
				}
				kinds = sel
			}
		}
		return taintSource{desc: ghDesc(c), kinds: kinds}, true
	case "api":
		path := ""
		if len(c.Positional) > 1 {
			path = c.Positional[1].Value // `repos/$REPO/pulls/$N` is the path whatever the variables hold
		}
		if path == "" || !reGhAPIPath.MatchString(path) {
			return taintSource{}, false
		}
		jq, hasJQ := jqFilter(c)
		if !hasJQ {
			return taintSource{desc: ghDesc(c), kinds: kindText}, true // the JSON holds the title, the body ... on one line
		}
		sel, ok := jqSelects(jq)
		if !ok {
			return taintSource{}, false
		}
		if sel == 0 && !reGhTextPath.MatchString(path) {
			return taintSource{}, false
		}
		if sel == 0 {
			sel = kindMulti
		}
		return taintSource{desc: ghDesc(c), kinds: sel}, true
	}
	return taintSource{}, false
}

// jqSelects reports which kinds of text a jq filter reads, 0 when it reads none. ok is false when the filter is empty.
func jqSelects(filter string) (kinds int, ok bool) {
	if strings.TrimSpace(filter) == "" {
		return 0, false
	}
	for _, t := range ghJQTokens {
		if t.re.MatchString(filter) {
			kinds |= t.kind
		}
	}
	if kinds != 0 && strings.Contains(filter, "[]") {
		kinds |= kindMulti // a filter that iterates prints one line for each element
	}
	return kinds, true
}

// ghDesc is the command line as the message shows it: `gh pr view --json title`, `gh api repos/$R/pulls/$N --jq .body`.
func ghDesc(c *runscript.Command) string {
	if c.Verb() == "api" && len(c.Positional) > 1 {
		d := "gh api " + c.Positional[1].Value
		if jq, ok := jqFilter(c); ok && jq != "" {
			d += " --jq " + jq
		}
		return d
	}
	parts := []string{"gh"}
	for _, w := range c.Positional {
		if w.Dynamic() {
			break
		}
		parts = append(parts, w.Value)
		if len(parts) == 3 {
			break
		}
	}
	for _, f := range c.Flags {
		if f.Name == "--json" && f.Value != nil && !f.Value.Dynamic() {
			parts = append(parts, "--json", f.Value.Value)
		}
	}
	return strings.Join(parts, " ")
}

// gitSourceVerb returns the subcommand of git that the command runs when it is one of the sources.
func gitSourceVerb(c *runscript.Command) (string, bool) {
	for _, w := range c.Positional {
		if !w.Dynamic() && gitSourceVerbs[w.Value] {
			return w.Value, true
		}
	}
	return "", false
}

func gitTaint(c *runscript.Command, verb string) (taintSource, bool) {
	switch verb {
	case "log", "show":
		var all []string
		for _, w := range c.Words {
			all = append(all, w.Value)
		}
		line := strings.Join(all, " ")
		kinds := 0
		if reGitSubject.MatchString(line) {
			kinds |= kindText
		}
		if reGitBody.MatchString(line) {
			kinds |= kindMulti
		}
		if reGitName.MatchString(line) {
			kinds |= kindName
		}
		if kinds != 0 {
			return taintSource{desc: "git " + verb + " --format", kinds: kinds}, true
		}
	case "diff", "diff-tree":
		return taintSource{desc: "git " + verb, kinds: kindMulti}, true
	case "branch":
		if c.HasFlag("--show-current") {
			return taintSource{desc: "git branch --show-current", kinds: kindName}, true
		}
	case "rev-parse":
		if c.HasFlag("--abbrev-ref") {
			return taintSource{desc: "git rev-parse --abbrev-ref", kinds: kindName}, true
		}
	case "symbolic-ref":
		if c.HasFlag("--short") {
			return taintSource{desc: "git symbolic-ref --short", kinds: kindName}, true
		}
	}
	return taintSource{}, false
}

// prHeadCheckedOut reports whether the code of a pull request is in the workspace when the command runs: an earlier
// step of the job checks out a head that outsiders control, or the script did it before the command (`gh pr checkout`,
// a fetch of refs/pull/N/head).
func (rule *RuleGitHubEnv) prHeadCheckedOut(s *runscript.Script, c *runscript.Command) bool {
	if s != nil {
		for _, other := range s.Commands {
			if other.Offset >= c.Offset {
				break
			}
			if other.Tool == "gh" && other.Verb() == "pr" && other.Sub(1) == "checkout" {
				return true
			}
			if other.Name == "git" {
				for _, w := range other.Words {
					if rePullRefText.MatchString(w.Value) {
						return true
					}
				}
			}
		}
	}
	if rule.job == nil {
		return false
	}
	for _, st := range rule.job.Steps {
		if st == rule.step {
			break
		}
		switch e := st.Exec.(type) {
		case *ExecAction:
			_, u := usesOfStep(st)
			if u == nil || !u.isRepoAction("actions/checkout") {
				continue
			}
			if in, ok := e.Inputs["ref"]; ok && in != nil && in.Value != nil {
				if _, bad := untrustedString(in.Value); bad {
					return true
				}
			}
		case *ExecRun:
			if e.Run != nil && (strings.Contains(e.Run.Value, "gh pr checkout") || rePullRefText.MatchString(e.Run.Value)) {
				return true
			}
		}
	}
	return false
}

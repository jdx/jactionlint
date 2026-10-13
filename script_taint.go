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
	// multiline is whether the text can hold a line break.
	multiline bool
}

// ghTaintedFields are the fields of `--json` of `gh pr view` and `gh issue view` that hold text of outsiders. The value
// tells whether the field can hold several lines.
var ghTaintedFields = map[string]map[string]bool{
	"pr": {
		"headRefName": false, "title": false, "body": true, "comments": true, "files": true, "author": false,
		"reviews": true, "latestReviews": true, "commits": true,
	},
	"issue": {"title": false, "body": true, "comments": true, "author": false},
}

// ghJQTokens are the properties that a `--jq` filter of `gh api` or `gh pr view` selects when it reads text of an
// outsider, and whether they can hold several lines.
var ghJQTokens = []struct {
	re        *regexp.Regexp
	multiline bool
}{
	{regexp.MustCompile(`\.body\b|\bbody\b|messageBody|\.message\b`), true},
	{regexp.MustCompile(`\.title\b|\btitle\b|messageHeadline|headRefName|\.head\.ref\b|\.head\b|\.user\b|\.login\b|\bauthor\b|\.filename\b|\bpath\b|\.name\b`), false},
}

var (
	reGhAPIPath    = regexp.MustCompile(`(^|/)repos/[^\s]+/(pulls|issues)(/|$)`)
	reGhTextPath   = regexp.MustCompile(`/(comments|reviews|files|commits)\b`)
	reGitFormat    = regexp.MustCompile(`%(s|b|B|an|ae|aN|aE|cn|ce|cN|cE|N)\b|%[sbB]|oneline`)
	rePullRefText  = regexp.MustCompile(`refs/pull/|\bpull/[^\s/]+/(head|merge)\b`)
	gitSourceVerbs = map[string]bool{"log": true, "show": true, "diff": true, "diff-tree": true, "branch": true, "rev-parse": true, "symbolic-ref": true}
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
		var listed []string
		for _, f := range c.FlagValues("--json") {
			if f.Dynamic() {
				return taintSource{desc: ghDesc(c), multiline: true}, true // the fields are not known
			}
			listed = append(listed, strings.Split(f.Value, ",")...)
		}
		tainted, multiline := false, false
		for _, name := range listed {
			if ml, ok := fields[strings.TrimSpace(name)]; ok {
				tainted, multiline = true, multiline || ml
			}
		}
		if !tainted {
			return taintSource{}, false
		}
		if jq, hasJQ := jqFilter(c); hasJQ {
			// the filter decides what is printed: a filter that reads none of the free text properties (`.number`)
			// clears the fields, one that reads a title is one line
			if sel, ok := jqSelects(jq); ok {
				if sel == 0 {
					return taintSource{}, false
				}
				multiline = sel&2 != 0
			}
		}
		return taintSource{desc: ghDesc(c), multiline: multiline}, true
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
			return taintSource{desc: ghDesc(c), multiline: false}, true // the JSON holds the title, the body ...
		}
		sel, ok := jqSelects(jq)
		if !ok {
			return taintSource{}, false
		}
		if sel == 0 && !reGhTextPath.MatchString(path) {
			return taintSource{}, false
		}
		return taintSource{desc: ghDesc(c), multiline: sel&2 != 0}, true
	}
	return taintSource{}, false
}

// jqSelects reports which kinds of free text a jq filter reads: 1 is a property that is one line, 2 one that can hold
// several lines. ok is false when the filter is not a text the rule can read.
func jqSelects(filter string) (kinds int, ok bool) {
	if strings.TrimSpace(filter) == "" {
		return 0, false
	}
	for _, t := range ghJQTokens {
		if t.re.MatchString(filter) {
			if t.multiline {
				kinds |= 2
			} else {
				kinds |= 1
			}
		}
	}
	if kinds != 0 && strings.Contains(filter, "[]") {
		kinds |= 2 // a filter that iterates prints one line for each element
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
	text := func(w *runscript.Word) string { return w.Value }
	var all []string
	for _, w := range c.Words {
		all = append(all, text(w))
	}
	line := strings.Join(all, " ")
	switch verb {
	case "log", "show":
		if reGitFormat.MatchString(line) {
			return taintSource{desc: "git " + verb + " --format", multiline: strings.Contains(line, "%b") || strings.Contains(line, "%B") || strings.Contains(line, "%N")}, true
		}
	case "diff", "diff-tree":
		return taintSource{desc: "git " + verb, multiline: true}, true
	case "branch":
		if c.HasFlag("--show-current") {
			return taintSource{desc: "git branch --show-current"}, true
		}
	case "rev-parse":
		if c.HasFlag("--abbrev-ref") {
			return taintSource{desc: "git rev-parse --abbrev-ref"}, true
		}
	case "symbolic-ref":
		if c.HasFlag("--short") {
			return taintSource{desc: "git symbolic-ref --short"}, true
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

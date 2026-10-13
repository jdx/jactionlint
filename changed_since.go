package jactionlint

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// This file reports only the findings in what changed since a revision (--changed-since): a pull request check that
// shows the findings of the lines of the diff and not the ones the repository already had. It is a filter of what is
// shown. The checks run as always over every file and over the whole call graph of the repository, so a finding that
// depends on a caller, a callee or another file is the one a full run reports, and the baseline is applied first. It
// is not a way to check less.
//
// What counts as changed is what git says changed between the merge base of the revision and HEAD and the working
// tree: committed, staged and unstaged changes of tracked files, and every line of a file that is not tracked and not
// ignored. A deleted block marks the lines on both sides of the place it was, because removing a line (a
// "permissions:", a guard) can be the cause of a finding next to it. A moved file (a rename) counts as new, so its
// findings are shown.

// ChangedScope says how much of a changed file is shown.
type ChangedScope string

const (
	// ChangedScopeLines shows the findings on the changed lines (the default).
	ChangedScopeLines ChangedScope = "lines"
	// ChangedScopeFiles shows every finding of a file that changed.
	ChangedScopeFiles ChangedScope = "files"
)

// ParseChangedScope reads the value of --changed-scope.
func ParseChangedScope(s string) (ChangedScope, error) {
	switch ChangedScope(s) {
	case "", ChangedScopeLines:
		return ChangedScopeLines, nil
	case ChangedScopeFiles:
		return ChangedScopeFiles, nil
	}
	return "", fmt.Errorf("invalid value %q for --changed-scope. available values are \"lines\" and \"files\"", s)
}

// ChangedInfo is what --changed-since did in a run. The SARIF log has it in properties.changedSince, so that a
// consumer can tell a log with the findings of a diff from a full one.
type ChangedInfo struct {
	// Since is the revision that was asked for.
	Since string `json:"since"`
	// Scope is "lines" or "files".
	Scope string `json:"scope"`
	// Hidden counts the findings that were not shown because they are outside what changed. Baselined findings are
	// not counted here.
	Hidden int `json:"hidden"`
}

// changedFilter is the state of --changed-since in a Linter.
type changedFilter struct {
	rev   string
	scope ChangedScope

	mu     sync.Mutex
	states map[string]*changedState // by project root
	hidden int
	err    error
}

// changedState is what git said about one repository.
type changedState struct {
	err   error
	files map[string]*changedLines // by path relative to the root with "/"
}

// changedLines are the lines of a file that changed: all of them, or the listed ranges (1-based, both ends included).
type changedLines struct {
	all    bool
	ranges [][2]int
}

func (c *changedLines) touches(from, to int) bool {
	if c.all {
		return true
	}
	if to < from {
		to = from
	}
	for _, r := range c.ranges {
		if from <= r[1] && r[0] <= to {
			return true
		}
	}
	return false
}

func newChangedFilter(rev string, scope ChangedScope) *changedFilter {
	return &changedFilter{rev: rev, scope: scope, states: map[string]*changedState{}}
}

// info returns what the filter did, or nil when --changed-since is off.
func (l *Linter) changedInfo() *ChangedInfo {
	c := l.changed
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return &ChangedInfo{Since: c.rev, Scope: string(c.scope), Hidden: c.hidden}
}

// changedError returns the reason --changed-since could not be applied: git is missing, the revision is unknown, or the
// history has no merge base. The run fails instead of showing everything, since a check that was asked to compare with a
// revision and could not should not look like it did.
func (l *Linter) changedError() error {
	if l.changed == nil {
		return nil
	}
	l.changed.mu.Lock()
	defer l.changed.mu.Unlock()
	return l.changed.err
}

// filterChanged moves the findings that are outside what changed out of the results. A result that is not about a
// file of a repository (stdin) and the results of the baseline files keep everything.
func (l *Linter) filterChanged(results []fileResult) []fileResult {
	c := l.changed
	if c == nil {
		return results
	}
	for i := range results {
		r := &results[i]
		if r.baselineFile || r.file == "" || r.file == l.stdin || len(r.errs) == 0 {
			continue
		}
		abs := l.absFilePath(r.file)
		if _, err := os.Stat(abs); err != nil {
			continue // content that is not a file on disk: nothing says what changed in it
		}
		p, err := l.projects.At(abs)
		if err != nil || p == nil {
			continue // not in a repository: git has no answer, and showing a finding is the safe side
		}
		root := absPath(p.RootDir())
		rel, err := filepath.Rel(root, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		st := c.stateFor(root)
		if st.err != nil {
			c.mu.Lock()
			if c.err == nil {
				c.err = st.err
			}
			c.mu.Unlock()
			continue
		}
		lines := st.files[filepath.ToSlash(rel)]
		kept := r.errs[:0:0]
		hidden := 0
		for _, e := range r.errs {
			if lines != nil && (c.scope == ChangedScopeFiles || e.Line <= 0 || lines.touches(e.Line, e.EndLine)) {
				kept = append(kept, e)
				continue
			}
			hidden++
		}
		r.errs = kept
		if hidden > 0 {
			c.mu.Lock()
			c.hidden += hidden
			c.mu.Unlock()
		}
	}
	return results
}

// reportChangedNote tells on the log output how many findings --changed-since left out. Like the note of the baseline
// only the text format prints it; a program reads the other formats.
func (l *Linter) reportChangedNote() {
	if _, ok := l.printer.(textPrinter); !ok {
		return
	}
	info := l.changedInfo()
	if info == nil || info.Hidden == 0 {
		return
	}
	what := "outside the changed lines"
	if info.Scope == string(ChangedScopeFiles) {
		what = "in files that did not change"
	}
	fmt.Fprintf(l.logOut, "note: %d finding(s) %s since %s are not shown (--changed-since)\n", info.Hidden, what, info.Since)
}

func (c *changedFilter) stateFor(root string) *changedState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if st, ok := c.states[root]; ok {
		return st
	}
	st := &changedState{}
	st.files, st.err = readChangedFiles(root, c.rev)
	c.states[root] = st
	return st
}

// gitOutput runs git in the directory and returns its standard output.
func gitOutput(root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := gitReadCommand(ctx, root, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if _, missing := err.(*exec.Error); missing {
			return nil, fmt.Errorf("--changed-since needs git, which was not found")
		}
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// readChangedFiles asks git which lines changed since the merge base of the revision and HEAD in the repository at root.
func readChangedFiles(root, rev string) (map[string]*changedLines, error) {
	if strings.HasPrefix(rev, "-") {
		return nil, fmt.Errorf("invalid revision %q for --changed-since", rev)
	}
	if _, err := gitOutput(root, "rev-parse", "--verify", "--quiet", rev+"^{commit}"); err != nil {
		return nil, fmt.Errorf("--changed-since: git does not know the revision %q in %s. fetch it (a shallow clone needs more history, for example fetch-depth: 0 of actions/checkout) or name another one", rev, root)
	}
	base, err := gitOutput(root, "merge-base", rev, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("--changed-since: git found no merge base of %q and HEAD in %s. fetch more history (fetch-depth: 0 of actions/checkout)", rev, root)
	}
	baseRev := strings.TrimSpace(string(base))
	// --relative: the paths are those of root, whatever the top of the work tree of git is
	diff, err := gitOutput(root, "diff", "--relative", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames", "--src-prefix=a/", "--dst-prefix=b/", "-U0", baseRev, "--")
	if err != nil {
		return nil, fmt.Errorf("--changed-since: %w", err)
	}
	files := parseChangedDiff(string(diff))
	untracked, err := gitOutput(root, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("--changed-since: %w", err)
	}
	for _, f := range strings.Split(string(untracked), "\x00") {
		if f != "" {
			files[filepath.ToSlash(f)] = &changedLines{all: true}
		}
	}
	return files, nil
}

var reHunk = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// parseChangedDiff reads the output of git diff -U0: the new side of each hunk. A file that is new has one hunk of
// all of its lines; a deleted file is not there. A hunk that only deletes marks the lines on both sides of the place.
func parseChangedDiff(diff string) map[string]*changedLines {
	files := map[string]*changedLines{}
	var cur *changedLines
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			cur = nil
		case strings.HasPrefix(line, "+++ "):
			name := strings.TrimSuffix(strings.TrimPrefix(line, "+++ "), "\t") // git ends a name with spaces by a tab
			if name == "/dev/null" {
				cur = nil
				continue
			}
			name = strings.TrimPrefix(name, "b/")
			if unq, err := strconv.Unquote(name); err == nil && strings.HasPrefix(name, `"`) {
				name = strings.TrimPrefix(unq, "b/")
			}
			cur = files[name]
			if cur == nil {
				cur = &changedLines{}
				files[name] = cur
			}
		case cur != nil && strings.HasPrefix(line, "@@"):
			m := reHunk.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			start, _ := strconv.Atoi(m[1])
			count := 1
			if m[2] != "" {
				count, _ = strconv.Atoi(m[2])
			}
			if count == 0 {
				// Only a deletion: the new side is empty and start is the line before the place
				cur.ranges = append(cur.ranges, [2]int{max(start, 1), start + 1})
				continue
			}
			cur.ranges = append(cur.ranges, [2]int{start, start + count - 1})
		}
	}
	for _, c := range files {
		sort.Slice(c.ranges, func(i, j int) bool { return c.ranges[i][0] < c.ranges[j][0] })
	}
	return files
}

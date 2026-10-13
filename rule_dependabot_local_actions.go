package jactionlint

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// This file is the second check of dependabot-missing-actions-update: an update with the "github-actions" ecosystem
// reads the workflows of ".github/workflows" and the action.yml of the directory it is given, so with "directory: /" the
// local composite actions of ".github/actions/*" are never updated (zizmor#1806, dependabot-core#6345). A directory
// of "directories" can be a glob, which is how one entry reaches all of them: "/.github/actions/**/*".

// localActionsWithExternalUses returns the directories ("/" and then the path from the root of the repository, with a
// slash in front) of the local composite actions whose steps use an action or a reusable workflow that Dependabot can
// update, in order. A file that does not parse is skipped.
func localActionsWithExternalUses(p *Project) []string {
	var dirs []string
	seen := map[string]bool{}
	for _, f := range p.ActionFiles() {
		rel, err := filepath.Rel(p.RootDir(), f)
		if err != nil {
			continue
		}
		dir := normalizeDependabotDirectory(path.Dir(filepath.ToSlash(rel)))
		if seen[dir] {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		w, _ := ParseAction(b)
		if w == nil || !workflowHasExternalActions(w) {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}

// isUpdatableUses reports whether a `uses:` value is an action or a reusable workflow which Dependabot can update: not a
// local path, not a docker:// image and not a value that has an expression. It is the one predicate of both checks of
// dependabot-missing-actions-update, for the workflows and for the local actions.
func isUpdatableUses(uses *String) bool {
	if uses == nil {
		return false
	}
	u := ParseUses(uses.Value)
	return !u.Dynamic && (u.Kind == UsesAction || u.Kind == UsesReusableWorkflow)
}

// workflowHasExternalActions reports whether a job of the workflow calls a reusable workflow, or a step of it (a step of the
// synthetic job of a composite action too) uses an action, that Dependabot can update (isUpdatableUses). It is the one walk
// of the jobs and the steps for both checks.
func workflowHasExternalActions(w *Workflow) bool {
	for _, j := range w.Jobs {
		if j == nil {
			continue
		}
		if j.WorkflowCall != nil && isUpdatableUses(j.WorkflowCall.Uses) {
			return true
		}
		for _, s := range flattenSteps(j.Steps) {
			if a, ok := s.Exec.(*ExecAction); ok && isUpdatableUses(a.Uses) {
				return true
			}
		}
	}
	return false
}

// normalizeDependabotDirectory writes a directory of a Dependabot update the way directories of the repository are
// written here: a slash in front, none at the end ("/" for the root).
func normalizeDependabotDirectory(d string) string {
	d = strings.TrimSpace(d)
	d = path.Clean("/" + d)
	return d
}

// dependabotGlob turns a glob of "directories" into a regular expression: "*" matches the characters of one name,
// "?" one of them, and "**" any number of directories ("a/**/b" matches "a/b" and "a/x/y/b"; "a/**/*" matches every
// directory below a).
func dependabotGlob(pattern string) (*regexp.Regexp, error) {
	segs := strings.Split(normalizeDependabotDirectory(pattern), "/")[1:]
	var b strings.Builder
	b.WriteString("^")
	for i, seg := range segs {
		last := i == len(segs)-1
		switch {
		case seg == "**":
			if last {
				b.WriteString("(?:/.*)?")
			} else {
				b.WriteString("(?:/[^/]+)*")
			}
			continue
		}
		b.WriteString("/")
		if seg == "*" {
			b.WriteString("[^/]+") // a name, not nothing
			continue
		}
		for _, r := range seg {
			switch r {
			case '*':
				b.WriteString("[^/]*")
			case '?':
				b.WriteString("[^/]")
			default:
				b.WriteString(regexp.QuoteMeta(string(r)))
			}
		}
	}
	if len(segs) == 1 && segs[0] == "" {
		b.WriteString("/")
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// dependabotDirectoryCovers reports whether the directory entry of an update (a literal or a glob) names the directory.
func dependabotDirectoryCovers(entry, dir string) bool {
	if normalizeDependabotDirectory(entry) == dir {
		return true
	}
	if !strings.ContainsAny(entry, "*?") {
		return false
	}
	re, err := dependabotGlob(entry)
	return err == nil && re.MatchString(dir)
}

// checkLocalActions reports the local composite actions that the github-actions updates of the file do not cover.
func (r *RuleDependabotMissingActionsUpdate) checkLocalActions(updates []*DependabotUpdate) {
	dirs := localActionsWithExternalUses(r.project)
	var missing []string
	for _, dir := range dirs {
		covered := false
		for _, u := range updates {
			if u.Directory != nil && dependabotDirectoryCovers(u.Directory.Value, dir) {
				covered = true
			}
			for _, d := range u.Directories {
				if d != nil && dependabotDirectoryCovers(d.Value, dir) {
					covered = true
				}
			}
		}
		if !covered {
			missing = append(missing, dir)
		}
	}
	if len(missing) == 0 {
		return
	}
	first := updates[0]
	pos := first.Pos
	switch {
	case first.Directory != nil && first.Directory.Pos != nil:
		pos = first.Directory.Pos
	case len(first.Directories) > 0 && first.Directories[0] != nil && first.Directories[0].Pos != nil:
		pos = first.Directories[0].Pos
	}
	glob := ""
	for _, d := range missing {
		if strings.HasPrefix(d, "/.github/actions/") {
			glob = "/.github/actions/**/*"
		}
	}
	var shown []string
	for i, d := range missing {
		if i == 3 {
			shown = append(shown, fmt.Sprintf("and %d more", len(missing)-3))
			break
		}
		shown = append(shown, fmt.Sprintf("%q", d))
	}
	// What the updates name now stays, and the directories they miss come after it
	var suggest []string
	seenDir := map[string]bool{}
	add := func(entry string) {
		key := normalizeDependabotDirectory(entry)
		if strings.ContainsAny(entry, "*?") {
			key = strings.TrimSpace(entry)
		}
		if !seenDir[key] {
			seenDir[key] = true
			suggest = append(suggest, fmt.Sprintf("%q", entry))
		}
	}
	rootNamed := slices.Contains(missing, "/")
	for _, u := range updates {
		if u.Directory != nil && normalizeDependabotDirectory(u.Directory.Value) == "/" {
			rootNamed = true
		}
		for _, d := range u.Directories {
			if d != nil && normalizeDependabotDirectory(d.Value) == "/" {
				rootNamed = true
			}
		}
	}
	if rootNamed {
		add("/") // the workflows are read from the root
	}
	for _, u := range updates {
		if u.Directory != nil {
			add(u.Directory.Value)
		}
		for _, d := range u.Directories {
			if d != nil {
				add(d.Value)
			}
		}
	}
	if glob != "" {
		add(glob)
	}
	for _, d := range missing {
		if !strings.HasPrefix(d, "/.github/actions/") {
			add(d)
		}
	}
	msg := fmt.Sprintf("the \"github-actions\" update does not cover the local composite actions in %s, which use actions, so Dependabot never updates them: it reads the workflows and the \"action.yml\" of the directories it is given, and the directory \"/\" is the workflows and the root only. add them: \"directories: [%s]\"", strings.Join(shown, ", "), strings.Join(suggest, ", "))
	r.ReportID("dependabot-missing-actions-update", pos, msg)
	e := r.errs[len(r.errs)-1]
	if fix, why := r.localActionsFix(updates, suggest); fix != nil {
		e.Fix = fix
	} else if why != nil {
		e.NoFix = why
	}
}

// localActionsFix replaces "directory: /" of the only github-actions update by the list of directories, when the key
// is written as a plain block mapping entry the edit can read.
func (r *RuleDependabotMissingActionsUpdate) localActionsFix(updates []*DependabotUpdate, suggest []string) (*Fix, *NoFix) {
	unsupported := &NoFix{Code: NoFixUnsupportedShape, Reason: "the update is not a single \"directory: /\" written in a way the fix can edit, add the directories by hand"}
	if r.src == nil || len(updates) != 1 {
		return nil, unsupported
	}
	u := updates[0]
	if u.Directory == nil || u.Directory.Pos == nil || normalizeDependabotDirectory(u.Directory.Value) != "/" || len(u.Directories) > 0 {
		return nil, unsupported
	}
	indent, _, _, after, ok := r.src.keyOfValueAt(u.Directory.Pos, "directory")
	if !ok {
		return nil, unsupported
	}
	quoted := len(after) > 0 && (after[0] == '"' || after[0] == '\'')
	n, ok := scalarToken(after, u.Directory.Value, quoted)
	if !ok {
		return nil, unsupported
	}
	content, lineStart, _, found := r.src.line(u.Directory.Pos.Line)
	if !found {
		return nil, unsupported
	}
	valueStart := lineStart + u.Directory.Pos.Col - 1
	keyStart := lineStart + indent
	if keyStart > valueStart || valueStart > lineStart+len(content) {
		return nil, unsupported
	}
	return &Fix{
		Description: "Update the local composite actions too",
		// It makes Dependabot propose updates it did not, so it is not applied without being asked
		Unsafe: true,
		Edits:  []TextEdit{{Start: keyStart, End: valueStart + n, NewText: "directories: [" + strings.Join(suggest, ", ") + "]"}},
	}, nil
}

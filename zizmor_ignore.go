package jactionlint

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

// zizmorIgnoreRe matches the zizmor ignore comment `# zizmor: ignore[rule-a,rule-b]` the way zizmor
// does: the spacing is fixed, the rule list is in brackets and only white space, then free text (the
// reason), may follow the closing bracket. It is not anchored so that it is found after another
// comment: `uses: x # v2.9.2 # zizmor: ignore[unpinned-uses]`.
var zizmorIgnoreRe = regexp.MustCompile(`# zizmor: ignore\[([^\]]*)\](?:\s(.*))?$`)

// zizmorAlias is a diagnostic of jactionlint which a zizmor audit name stands for.
type zizmorAlias struct {
	// ID is the ID of the diagnostic.
	ID string
	// MessageContains narrows the alias to the diagnostics of the ID whose message contains it. It is
	// empty when every diagnostic of the ID is meant.
	MessageContains string
}

// zizmorAliases lists the zizmor audits whose findings jactionlint reports under another ID. An
// audit whose name is the ID of a rule of jactionlint needs no entry: the name is used as is. See
// docs/v2-migration.md, which documents this table.
var zizmorAliases = map[string][]zizmorAlias{
	// jactionlint reports a missing permissions block, which is one case of excessive-permissions
	"excessive-permissions": {{ID: "missing-permissions"}},
	// Docker images are checked by unpinned-uses
	"unpinned-images": {{ID: "unpinned-uses", MessageContains: "docker image"}},
}

// zizmorTargets returns the diagnostics which a zizmor audit name stands for. It is empty for an audit
// which jactionlint has no counterpart of, which makes the name in a comment inert.
func zizmorTargets(name string) []zizmorAlias {
	var ret []zizmorAlias
	if _, ok := LookupRule(name); ok {
		ret = append(ret, zizmorAlias{ID: name})
	}
	return append(ret, zizmorAliases[name]...)
}

// zizmorName is one audit name of a zizmor ignore comment.
type zizmorName struct {
	name string
	// col is the 1-based column of the name in the line, counted in characters.
	col int
}

// zizmorComment is one `# zizmor: ignore[...]` comment of a source.
type zizmorComment struct {
	line int
	// start is the byte offset of the "# zizmor: ignore[" text in the line.
	start int
	// hash is the byte offset of the '#' which opens the comment. It is before start when the comment starts
	// with other text ("# note # zizmor: ignore[...]").
	hash int
	// inline is true when the comment follows content on its line.
	inline bool
	names  []zizmorName
	// reason is the free text after the closing bracket.
	reason string
}

// scanZizmorComments finds the zizmor ignore comments of the source. lines are the lines of the source
// without line terminators. Only real comments count: a '#' in a quoted scalar or a block scalar does
// not.
func scanZizmorComments(src []byte, lines []string) []zizmorComment {
	if !bytes.Contains(src, []byte("zizmor")) {
		return nil
	}
	var ret []zizmorComment
	for _, c := range NewCommentIndex(src).All() {
		if c.Line < 1 || c.Line > len(lines) {
			continue
		}
		line := lines[c.Line-1]
		hash := byteOffsetOfColumn(line, c.Column)
		m := zizmorIgnoreRe.FindStringSubmatchIndex(line[hash:])
		if m == nil {
			continue
		}
		zc := zizmorComment{line: c.Line, start: hash + m[0], hash: hash, inline: c.Inline}
		if m[4] >= 0 {
			zc.reason = strings.TrimSpace(line[hash+m[4] : hash+m[5]])
		}
		list := line[hash+m[2] : hash+m[3]]
		off := hash + m[2]
		seen := map[string]bool{}
		for _, raw := range strings.Split(list, ",") {
			name := strings.TrimSpace(raw)
			at := off + len(raw) - len(strings.TrimLeft(raw, " \t"))
			off += len(raw) + 1
			// A name with white space in it is an invalid ignore for zizmor
			if name == "" || strings.ContainsAny(name, " \t") || seen[name] {
				continue
			}
			seen[name] = true
			zc.names = append(zc.names, zizmorName{name, utf8.RuneCountInString(line[:at]) + 1})
		}
		ret = append(ret, zc)
	}
	return ret
}

// byteOffsetOfColumn converts a 1-based column counted in characters to a byte offset.
func byteOffsetOfColumn(line string, col int) int {
	n := 0
	for i := range line {
		n++
		if n == col {
			return i
		}
	}
	return len(line)
}

var (
	zizmorHeaderKeyRe   = regexp.MustCompile(`:\s*(?:[&!]\S*\s*)*$`)
	zizmorHeaderBlockRe = regexp.MustCompile(`(?:^|[\s:])[|>][-+0-9]*$`)
)

// zizmorHeaderRange returns the lines [start, end] (1-based) opened by a line whose content before the
// comment ends with a key without a value ("on:", "permissions:") or a block scalar header ("run: |").
// zizmor accepts the comment anywhere in the span of its finding, and a finding about a value, such as
// the script of a step or the events of a workflow, spans the lines of that value, so a comment on
// the line which opens it applies to the lines it holds.
func zizmorHeaderRange(lines []string, c zizmorComment) (int, int, bool) {
	i := c.line - 1
	head := strings.TrimRight(lines[i][:c.hash], " \t")
	isKey := zizmorHeaderKeyRe.MatchString(head)
	if !c.inline || (!isKey && !zizmorHeaderBlockRe.MatchString(head)) {
		return 0, 0, false
	}
	// The key may sit behind "- " of a sequence item. Its children are indented deeper than the key.
	col := indentOf(lines[i])
	for strings.HasPrefix(lines[i][col:], "- ") {
		col += 2
		col += indentOf(lines[i][col:])
	}
	end := c.line
	for j := i + 1; j < len(lines); j++ {
		if cm, b := isCommentOrBlank(lines[j]); b || (cm && (isKey || indentOf(lines[j]) <= col)) {
			// A line which starts with "#" in a block scalar is part of the script, not a comment
			continue
		}
		ind := indentOf(lines[j])
		if ind < col || (ind == col && !(isKey && isSequenceItem(lines[j]))) {
			break
		}
		end = j + 1
	}
	return c.line, end, true
}

// parseZizmorIgnores collects the zizmor ignore comments of the source as ignores. A comment applies
// to the findings whose region contains the comment, like in zizmor, and, when it sits on a line which
// opens a block, to the findings inside the block. A rule list can name audits jactionlint does not
// have: they are skipped.
func parseZizmorIgnores(src []byte) []inlineIgnore {
	if !bytes.Contains(src, []byte("zizmor")) {
		return nil
	}
	lines := splitSourceLines(src)
	var ret []inlineIgnore
	for _, c := range scanZizmorComments(src, lines) {
		ig := inlineIgnore{commentLine: c.line}
		for _, n := range c.names {
			targets := zizmorTargets(n.name)
			if len(targets) == 0 {
				continue
			}
			ig.entries = append(ig.entries, &inlineIgnoreEntry{line: c.line, col: n.col, zizmor: n.name, targets: targets})
		}
		if len(ig.entries) == 0 {
			continue
		}
		if s, e, ok := zizmorHeaderRange(lines, c); ok {
			ig.start, ig.end = s, e
		}
		ret = append(ret, ig)
	}
	return ret
}

// splitSourceLines splits the source into lines without the line terminators ("\n" or "\r\n").
func splitSourceLines(src []byte) []string {
	lines := strings.Split(string(src), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// matchesZizmor returns whether the error is one of the diagnostics which the audit name of a zizmor
// ignore stands for.
func (e *inlineIgnoreEntry) matchesZizmor(err *Error) bool {
	for _, t := range e.targets {
		if t.ID == err.ID && (t.MessageContains == "" || strings.Contains(err.Message, t.MessageContains)) {
			return true
		}
	}
	return false
}

// zizmorEntryActive returns whether a zizmor ignore could suppress anything with the configuration:
// one of the diagnostics it stands for must be enabled.
func zizmorEntryActive(e *inlineIgnoreEntry, cfg *Config, online bool) bool {
	for _, t := range e.targets {
		// An online rule cannot report anything while the online checks are off
		if cfg.RuleRuns(t.ID, online) {
			return true
		}
	}
	return false
}

// MigrateZizmorIgnores rewrites the trailing `# zizmor: ignore[...]` comments of a workflow into
// `# jactionlint ignore=...` comments on the line above, with the same indentation, and returns the
// new content and the audit names which were migrated. The reason after the brackets moves to a plain
// comment line above. An audit without a counterpart in jactionlint, or whose findings are only a part
// of a jactionlint diagnostic (unpinned-images), stays in the zizmor comment, which jactionlint
// keeps honoring. Comments which are not trailing are left alone. The result is unchanged when
// nothing can be migrated, so running it twice changes nothing. When the rewritten file would not
// have the same YAML data as the original, the source is returned unchanged.
func MigrateZizmorIgnores(src []byte) ([]byte, []string) {
	if !bytes.Contains(src, []byte("zizmor")) {
		return src, nil
	}
	// Keep the line terminators to leave CRLF files as they are
	raw := strings.SplitAfter(string(src), "\n")
	lines := splitSourceLines(src)

	var migrated []string
	byLine := map[int][]string{} // line => lines to insert above
	replaced := map[int]string{} // line => new line content without terminator
	for _, c := range scanZizmorComments(src, lines) {
		if !c.inline {
			continue
		}
		var ids, kept []string
		var moved []string
		for _, n := range c.names {
			found := migrationIDs(n.name)
			if len(found) == 0 {
				kept = append(kept, n.name)
				continue
			}
			moved = append(moved, n.name)
			for _, id := range found {
				if !containsString(ids, id) {
					ids = append(ids, id)
				}
			}
		}
		if len(moved) == 0 {
			continue
		}
		migrated = append(migrated, moved...)

		line := lines[c.line-1]
		head := strings.TrimRight(line[:c.start], " \t")
		var tail string
		if len(kept) > 0 {
			tail = " # zizmor: ignore[" + strings.Join(kept, ",") + "]"
			if c.reason != "" {
				tail += " " + c.reason
			}
		}
		replaced[c.line] = head + tail

		indent := line[:indentOf(line)]
		var ins []string
		if c.reason != "" && len(kept) == 0 {
			ins = append(ins, indent+"# "+c.reason)
		}
		ins = append(ins, indent+"# jactionlint ignore="+strings.Join(ids, ","))
		byLine[c.line] = ins
	}
	if len(migrated) == 0 {
		return src, nil
	}

	var out strings.Builder
	for i, r := range raw {
		n := i + 1
		eol := ""
		body := r
		if strings.HasSuffix(body, "\n") {
			eol = "\n"
			body = body[:len(body)-1]
		}
		if strings.HasSuffix(body, "\r") {
			eol = "\r" + eol
			body = body[:len(body)-1]
		}
		for _, l := range byLine[n] {
			out.WriteString(l)
			out.WriteString(eol)
			if eol == "" { // last line without a terminator
				out.WriteString("\n")
			}
		}
		if s, ok := replaced[n]; ok {
			body = s
		}
		out.WriteString(body)
		out.WriteString(eol)
	}

	res := []byte(out.String())
	if !sameYAMLData(src, res) {
		return src, nil
	}
	return res, migrated
}

// migrationIDs returns the IDs of the jactionlint rules which replace the zizmor audit in an ignore
// comment: the rule of the same name, and the rules it is reported under as well (excessive-permissions
// covers missing-permissions). It is empty when the audit cannot be migrated. An alias that covers only some
// diagnostics of its rule (unpinned-images) cannot be written as a rule ID, so such an audit stays in the zizmor comment.
func migrationIDs(name string) []string {
	var ret []string
	for _, a := range zizmorAliases[name] {
		if a.MessageContains != "" {
			return nil // covers only part of the findings of a rule: the zizmor comment stays
		}
	}
	if _, ok := LookupRule(name); ok {
		ret = append(ret, name)
	}
	for _, a := range zizmorAliases[name] {
		if _, ok := LookupRule(a.ID); ok && !containsString(ret, a.ID) {
			ret = append(ret, a.ID)
		}
	}
	return ret
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// sameYAMLData returns whether both sources hold the same YAML data. Comments do not count.
func sameYAMLData(a, b []byte) bool {
	var x, y any
	if err := yaml.Unmarshal(a, &x); err != nil {
		return false
	}
	if err := yaml.Unmarshal(b, &y); err != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// MigrateZizmorIgnoreFiles runs MigrateZizmorIgnores on the files and writes the files which changed.
// It returns the migrated audit names by file path, only for the files which changed.
func MigrateZizmorIgnoreFiles(paths []string) (map[string][]string, error) {
	ret := map[string][]string{}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("could not read %q: %w", p, err)
		}
		out, names := MigrateZizmorIgnores(src)
		if len(names) == 0 {
			continue
		}
		if err := writeFileKeepingMode(p, out); err != nil {
			return nil, err
		}
		ret[p] = names
	}
	return ret, nil
}

// MigrateIgnores runs MigrateZizmorIgnoreFiles on the given files, or on the workflow files of the
// project when none is given, and reports what changed.
func (l *Linter) MigrateIgnores(paths []string) error {
	if len(paths) == 0 {
		p, err := l.projects.At(l.cwd)
		if err != nil {
			return err
		}
		if p == nil {
			return fmt.Errorf("no project was found in any parent directories of %q. check workflows directory is put correctly in your Git repository", l.cwd)
		}
		if paths, err = collectWorkflowFiles(p.WorkflowsDir()); err != nil {
			return err
		}
	}
	res, err := MigrateZizmorIgnoreFiles(paths)
	if err != nil {
		return err
	}
	if len(res) == 0 {
		fmt.Fprintln(l.out, "No zizmor ignore comment could be migrated. Nothing was changed")
		return nil
	}
	for _, p := range paths {
		if names, ok := res[p]; ok {
			fmt.Fprintf(l.out, "%s: migrated zizmor ignore comments: %s\n", p, strings.Join(names, ", "))
		}
	}
	return nil
}

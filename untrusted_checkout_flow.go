package jactionlint

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// This file follows a revision of a pull request through shell variables and script files for untrusted-checkout.
//
// The invariant. Taint is a fact about one position of one script, in execution order:
//
//   - A variable at a position is untrusted, clean or unknown. The value at a position is decided by walking the
//     assignments before it backwards: an unconditional assignment (not in a branch, loop, function, pipeline or
//     subshell, not `+=`) REPLACES what came before, so a clean value ends the taint, also of a variable the script
//     inherited; a conditional one may or may not have happened, so it can only ADD taint and the walk goes on. What the
//     script started with is the fallback. Unknown (command output, `read`, `eval`, arrays, step outputs) is not
//     reported: this rule only reports what it can show.
//   - What a script starts with is its environment, a map from name to "untrusted (where from)" or "clean". For a
//     step it is the env of the workflow, the job, then what earlier steps wrote to $GITHUB_ENV (an unconditional
//     write replaces, a conditional one can only add), then the env of the step. For a script file that a command
//     starts it is built from the parent at the call: only the exported variables (the ones inherited, which stay
//     exported when assigned, and the ones `export`ed), evaluated at the call, then the assignments in front of the
//     call applied last, plus the child's own positional parameters. The parent's positional parameters and
//     unexported variables never reach the child.
//   - The working directory follows literal `cd` commands; a script file is looked up from it. A `cd` that is
//     conditional, dynamic or absolute makes the directory unknown and the script is not followed.
//   - A value written to $GITHUB_ENV (echo, printf, here document) is judged as a whole: untrusted when its text names the
//     head, `refs/pull/` or an untrusted variable, clean when it is fully resolved and none of those, unknown otherwise
//     (a command substitution, a format that is not understood). Only a clean unconditional value replaces what an
//     earlier step wrote; unknown keeps it.
//   - Attacker-controlled: the head of a pull request or of the triggering run (untrustedValue) and `refs/pull/...`.
//   - Reaches the working tree: a command that puts a revision in it (gitCheckoutVerbs, `gh pr checkout`) and names
//     an untrusted word, or a `git fetch` of an untrusted word followed by a command that changes the working tree.
//     A script file that does that counts at the command that starts it.
//   - Runs code from it: unchanged (stepRunsCode) for the commands after the position where the revision reached the
//     working tree (not after the fetch). When the script file runs code after its checkout, the step is reported at
//     once.
//
// Known limits, documented in docs/checks.md: a write to $GITHUB_ENV through a here document, `declare -x`, `read`,
// `eval`, arrays, command output, `pushd` and a computed script path are not followed, and scripts nested more than
// maxScriptDepth deep are not.

// maxScriptDepth is how many levels of script files that start script files are followed.
const maxScriptDepth = 3

// revTaint tells which shell variables of a script hold a revision of a pull request.
type revTaint struct {
	s *runscript.Script
	// pred says whether an expression names a revision that someone else controls; refs whether a `refs/pull/` word
	// does. The head of a pull request is the default (untrustedValue); a taint of the number of the pull request that a
	// comment is on only has the contexts of that number and no refs.
	pred func(string) (string, bool)
	refs bool
	// num follows the number of the pull request that a comment is on, for gh pr checkout; nil when there is none.
	num *revTaint
	// base is the environment the script starts with: for each name where an untrusted value comes from, "" for a
	// variable that is exported but clean.
	base map[string]string
	memo map[varKey]string
}

type varKey struct {
	name string
	off  int
}

func newRevTaint(s *runscript.Script, base map[string]string) *revTaint {
	return &revTaint{s: s, base: base, memo: map[varKey]string{}, pred: untrustedValue, refs: true}
}

// newTaint is newRevTaint with what the rule knows about the head of a pull request: the outputs of the actions that
// look it up.
func (rule *RuleUntrustedCheckout) newTaint(s *runscript.Script, base map[string]string) *revTaint {
	t := newRevTaint(s, base)
	t.pred = rule.untrusted
	return t
}

// newNumberTaint follows the number of the pull request that a comment is on, which `gh pr checkout` takes as a
// revision.
func newNumberTaint(s *runscript.Script, base map[string]string) *revTaint {
	t := newRevTaint(s, base)
	t.pred, t.refs = issueNumberValue, false
	return t
}

// names returns the variables that hold a revision of a pull request at the offset.
func (t *revTaint) names(off int) map[string]bool {
	out := map[string]bool{}
	cands := map[string]bool{}
	for n := range t.base {
		cands[n] = true
	}
	for _, a := range t.s.Assignments {
		if a.Offset < off {
			cands[a.Name] = true
		}
	}
	for _, fv := range t.s.ForVars {
		if fv.Offset < off {
			cands[fv.Name] = true
		}
	}
	for n := range cands {
		if t.varAt(n, off) != "" {
			out[n] = true
		}
	}
	return out
}

// varAt returns what the variable holds at the offset when it is untrusted ("" when it is not or when that is not
// known). See the invariant at the top of the file. Every step of the walk goes to an assignment that starts before
// the offset it is evaluated at, so the walk ends without a depth limit; the results are remembered because a chain
// like `A=$A$A` would otherwise be walked exponentially.
func (t *revTaint) varAt(name string, off int) string {
	key := varKey{name, off}
	if d, ok := t.memo[key]; ok {
		return d
	}
	d := t.walkVar(name, off)
	t.memo[key] = d
	return d
}

func (t *revTaint) walkVar(name string, off int) string {
	type event struct {
		off int
		a   *runscript.Assignment
		fv  *runscript.ForVar
	}
	var evs []event
	for _, a := range t.s.Assignments {
		if a.Name == name && a.Offset < off && (a.Cmd == nil || a.Cmd.Decl) {
			evs = append(evs, event{off: a.Offset, a: a})
		}
	}
	for _, fv := range t.s.ForVars {
		if fv.Name == name && fv.Offset < off {
			evs = append(evs, event{off: fv.Offset, fv: fv})
		}
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].off < evs[j].off })
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		switch {
		case e.fv != nil:
			for _, w := range e.fv.Items {
				if d := t.wordAt(w, e.off); d != "" {
					return d
				}
			}
		case e.a.Array:
			// the elements are not followed
		default:
			if e.a.Value != nil {
				if d := t.wordAt(e.a.Value, e.a.Offset); d != "" {
					return d
				}
			}
			if !e.a.Cond && !e.a.Append && (e.a.Cmd == nil || e.a.Value != nil) {
				return "" // replaced by a value that is not untrusted
			}
		}
	}
	return t.base[name]
}

// wordAt returns what makes the word untrusted at the offset, "" if nothing does.
func (t *revTaint) wordAt(w *runscript.Word, off int) string {
	if w == nil {
		return ""
	}
	for _, e := range w.Exprs {
		if what, bad := t.pred("${{ " + e + " }}"); bad {
			return what
		}
	}
	if t.refs && namesPullRef(w.Value) {
		return w.Value
	}
	for _, v := range w.Vars {
		if d := t.varAt(v, off); d != "" {
			return d
		}
	}
	for _, c := range w.Subs {
		for _, sw := range c.Words {
			if d := t.wordAt(sw, off); d != "" {
				return d
			}
		}
	}
	return ""
}

// cwdAt returns the directory (relative to the project, "" for its root) that commands of the script at the offset
// run in, starting from dir. It is false when a `cd` before the offset leaves it unknown.
func (t *revTaint) cwdAt(dir string, off int) (string, bool) {
	for _, c := range t.s.Commands {
		if c.Offset >= off {
			break
		}
		switch c.Name {
		case "pushd", "popd":
			return "", false
		case "cd":
			if c.Cond || len(c.Positional) != 1 || c.Positional[0].Dynamic() {
				return "", false
			}
			target := c.Positional[0].Value
			if target == "" || target == "-" || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "~") || strings.HasPrefix(target, "-") {
				return "", false
			}
			dir = path.Clean(path.Join(dir, target))
			if dir == "." {
				dir = ""
			}
			if dir == ".." || strings.HasPrefix(dir, "../") {
				return "", false
			}
		}
	}
	return dir, true
}

// readScript reads the script file that a command of the step starts, nil when it is not one the rule may follow:
// it is missing, too large, not a regular file, outside the project or does not parse.
func (rule *RuleUntrustedCheckout) readScript(dir, rel string) *runscript.Script {
	if rule.project == nil || rule.project.root == "" || rel == "" {
		return nil
	}
	path := filepath.Join(rule.project.root, filepath.FromSlash(dir), filepath.FromSlash(rel))
	if sc, ok := rule.scripts[path]; ok {
		return sc
	}
	sc := rule.loadScript(path)
	if rule.scripts == nil {
		rule.scripts = map[string]*runscript.Script{}
	}
	rule.scripts[path] = sc // a missing or unusable file is remembered as well
	return sc
}

func (rule *RuleUntrustedCheckout) loadScript(path string) *runscript.Script {
	real, err := filepath.EvalSymlinks(path)
	root, rerr := filepath.EvalSymlinks(rule.project.root)
	if err != nil || rerr != nil || !isPathInDir(root, real) {
		return nil
	}
	if fi, err := os.Stat(real); err != nil || !fi.Mode().IsRegular() || fi.Size() > maxScriptSize {
		return nil
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return nil
	}
	script, err := runscript.Analyze(string(b), "")
	if err != nil {
		return nil
	}
	return script
}

// childBase returns the environment of the script that the command starts, see the invariant at the top of the file.
// inherited is the environment of the process the parent runs in.
func childBase(t *revTaint, c *runscript.Command, script *runscript.Word, inherited map[string]string) map[string]string {
	exported := map[string]bool{}
	for n := range inherited {
		if !isPositional(n) {
			exported[n] = true
		}
	}
	for _, a := range t.s.Assignments {
		if a.Cmd != nil && a.Cmd.Decl && a.Cmd.Name == "export" && a.Offset < c.Offset {
			exported[a.Name] = true
		}
	}
	base := map[string]string{}
	for n := range exported {
		base[n] = t.varAt(n, c.Offset)
	}
	for _, a := range c.Assigns { // NAME=value ./script.sh: applied last
		base[a.Name] = t.wordAt(a.Value, c.Offset)
	}
	n := 0
	for _, w := range c.Words {
		if w.Offset <= script.Offset {
			continue
		}
		n++
		if d := t.wordAt(w, c.Offset); d != "" {
			base[strconv.Itoa(n)], base["@"], base["*"] = d, d, d
		}
	}
	return base
}

// isPositional reports whether the name is a positional parameter or one of the lists of them.
func isPositional(name string) bool {
	if name == "@" || name == "*" {
		return true
	}
	_, err := strconv.Atoi(name)
	return err == nil
}

// scriptWord returns the word of the command that names the script file it starts.
func scriptWord(c *runscript.Command) *runscript.Word {
	switch c.Name {
	case "bash", "sh":
		if len(c.Positional) > 0 {
			return c.Positional[0]
		}
		return nil
	}
	return c.NameWord
}

// firstWorkTree returns the index of the first command that changes the working tree, -1 if there is none.
func firstWorkTree(cmds []*runscript.Command) int {
	for i, c := range cmds {
		if c.Name == "git" && workTreeVerbs[c.Verb()] {
			return i
		}
	}
	return -1
}

// scriptChecksOut reports whether running the script puts untrusted code in the working tree: it describes the
// revision, and says what the script runs after that ("" when it runs nothing, so a later step has to). dir is the
// directory the script runs in.
func (rule *RuleUntrustedCheckout) scriptChecksOut(sc *runscript.Script, base, numBase map[string]string, dir string, depth int) (what, how string, ok bool) {
	t := rule.newTaint(sc, base)
	if numBase != nil {
		t.num = newNumberTaint(sc, numBase)
	}
	for i, c := range sc.Commands {
		var env map[string]bool
		if c.Name == "git" || c.Name == "gh" {
			env = t.names(c.Offset)
		}
		at := i // the command after which the code is in the working tree
		what, ok = rule.commandChecksOutUntrusted(c, env, t.num)
		if !ok && c.Name == "git" && c.Verb() == "fetch" {
			if k := firstWorkTree(sc.Commands[i+1:]); k >= 0 {
				if what, ok = rule.commandNamesUntrusted(c, env); ok {
					at = i + 1 + k
				}
			}
		}
		if ok {
			what = t.describe(what, c.Offset)
		}
		if !ok && depth > 0 {
			what, how, ok = rule.followScript(t, c, base, dir, depth-1)
		}
		if !ok {
			continue
		}
		if how == "" {
			for _, d := range sc.Commands[at+1:] {
				if commandRunsCode(d) {
					how = quoteName(d)
					break
				}
			}
		}
		return what, how, true
	}
	return "", "", false
}

// describe turns the "$NAME" that commandNamesUntrusted gives for a variable into where its value comes from.
func (t *revTaint) describe(what string, off int) string {
	if n, ok := strings.CutPrefix(what, "$"); ok {
		if d := t.varAt(n, off); d != "" {
			return d
		}
	}
	return what
}

func quoteName(c *runscript.Command) string {
	name := c.Name
	if name == "" {
		name = "a command"
	}
	return "\"" + name + "\""
}

// followScript follows a command that starts a script file of the project into the file. dir is the directory the
// script that holds the command starts in.
func (rule *RuleUntrustedCheckout) followScript(t *revTaint, c *runscript.Command, inherited map[string]string, dir string, depth int) (what, how string, ok bool) {
	rel, sw := scriptPath(c), scriptWord(c)
	if rel == "" || sw == nil {
		return "", "", false
	}
	cwd, known := t.cwdAt(dir, c.Offset)
	if !known {
		return "", "", false
	}
	child := rule.readScript(cwd, rel)
	if child == nil {
		return "", "", false
	}
	var numBase map[string]string
	if t.num != nil {
		numBase = childBase(t.num, c, sw, t.num.base)
	}
	return rule.scriptChecksOut(child, childBase(t, c, sw, inherited), numBase, cwd, depth)
}

// envNameOf returns the variable that an entry of an environment file ("NAME=value") sets, "" if it is not one.
func envNameOf(entry string) string {
	name, _, found := strings.Cut(entry, "=")
	if !found || name == "" {
		return ""
	}
	for i, r := range name {
		if r != '_' && !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(i > 0 && r >= '0' && r <= '9') {
			return ""
		}
	}
	return name
}

// carryEnv records what the script writes to $GITHUB_ENV: the later steps of the job see it in their environment. An
// unconditional write replaces what an earlier step wrote, a conditional one can only add. `echo` writes one entry,
// that of its first word, whose value is all of its words; `printf` writes the entries of its format with the
// arguments filled in. Writes through a here document are not followed.
func (rule *RuleUntrustedCheckout) carryEnv(sc *runscript.Script, t *revTaint, stepCond bool, into *map[string]string) {
	// A value is untrusted, clean or unknown. Only a clean value replaces what an earlier step wrote: a value that
	// cannot be resolved (a command substitution, a format that is not understood) keeps it.
	set := func(name, d string, cond, unknown bool) {
		if d == "" && (cond || unknown || stepCond) { // a step with an `if:` may not run
			return
		}
		if *into == nil {
			*into = map[string]string{}
		}
		(*into)[name] = d
	}
	for _, w := range sc.WritesTo("GITHUB_ENV") {
		if w.Heredoc != nil {
			cond := len(w.Producers) == 0 || w.Producers[0].Cond
			for name, v := range heredocEntries(w.Heredoc, t) {
				set(name, v.what, cond, v.unknown)
			}
		}
		for _, p := range w.Producers {
			words := p.Positional
			if len(words) == 0 {
				continue
			}
			switch p.Name {
			case "echo":
				name := envNameOf(words[0].Value)
				if name == "" || strings.Contains(words[0].Value[:len(name)], "$") {
					continue
				}
				d, unknown := "", false
				for _, word := range words {
					if d = t.wordAt(word, p.Offset); d != "" {
						break
					}
					unknown = unknown || word.Subst
				}
				set(name, d, p.Cond, unknown)
			case "printf":
				format := words[0]
				if len(format.Vars) > 0 || format.Subst || strings.ContainsAny(format.Raw, "`") {
					continue // unknown what it writes: the earlier value stays
				}
				if !simpleFormat(format.Value) {
					continue // unknown what it writes: the earlier value stays
				}
				args := words[1:]
				segs := strings.Split(format.Value, `\n`)
				specs := 0
				for _, seg := range segs {
					specs += strings.Count(strings.ReplaceAll(seg, "%%", ""), "%")
				}
				passes := 1
				if specs > 0 && len(args) > specs {
					passes = (len(args) + specs - 1) / specs // the format is reused while arguments are left
				}
				next := 0
				for range passes {
					for _, seg := range segs {
						n := strings.Count(strings.ReplaceAll(seg, "%%", ""), "%")
						used := args[min(next, len(args)):min(next+n, len(args))]
						next += n
						entry := seg
						if (strings.HasPrefix(seg, "%s") || strings.HasPrefix(seg, "%b")) && len(used) > 0 {
							entry = used[0].Value + seg[2:] // the line starts with the first argument
						}
						name := envNameOf(entry)
						if name == "" {
							continue
						}
						d, unknown := literalTaint(seg, t.pred), false
						for _, a := range used {
							if d != "" {
								break
							}
							d = t.wordAt(a, p.Offset)
							unknown = unknown || a.Subst
						}
						set(name, d, p.Cond, unknown)
					}
				}
			}
		}
	}
}

// simpleFormat reports whether the printf format only has directives that take one argument and print it as is.
func simpleFormat(f string) bool {
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			continue
		}
		i++
		if i >= len(f) || !strings.ContainsRune("sbqd%", rune(f[i])) {
			return false
		}
	}
	return true
}

// exprTaint returns what makes a text with `${{ }}` expressions untrusted, "" if nothing does.
func exprTaint(text string, pred func(string) (string, bool)) string {
	for {
		i := strings.Index(text, "${{")
		if i < 0 {
			return ""
		}
		j := strings.Index(text[i:], "}}")
		if j < 0 {
			return ""
		}
		if what, bad := pred(text[i : i+j+2]); bad {
			return what
		}
		text = text[i+j+2:]
	}
}

// literalTaint returns what makes the text untrusted without looking at shell variables: an expression for the head of
// a pull request, or `refs/pull/`.
func literalTaint(text string, pred func(string) (string, bool)) string {
	if what := exprTaint(text, pred); what != "" {
		return what
	}
	if namesPullRef(text) {
		return text
	}
	return ""
}

// pullRefRe matches the short form of the ref of a pull request, `pull/<number>/head` and `pull/<number>/merge`, that
// git accepts for `refs/pull/...` (it looks a name up in refs/ too). The number can be a literal, a shell variable or
// an expression. The ref must start the word or follow `:`, `+` or `=`, so `origin/pull/1/head` (a name below a
// remote) and a URL are not matched.
var pullRefRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9_./-])pull/(?:\$\{\{[^}]*\}\}|[^/\s])+/(?:head|merge)\b`)

// namesPullRef reports whether the text names a ref of a pull request: `refs/pull/...` or `pull/<number>/head`.
func namesPullRef(text string) bool {
	return strings.Contains(text, "refs/pull/") || pullRefRe.MatchString(text)
}

// envValue is the value of an entry written to an environment file: what makes it untrusted, or unknown when
// it cannot be resolved.
type envValue struct {
	what    string
	unknown bool
}

// textValue judges the text of a here document. In an unquoted one the shell expands `$NAME` and `${NAME}`, which are
// resolved at the offset; a command substitution or arithmetic makes the value unknown.
func (t *revTaint) textValue(text string, quoted bool, off int) envValue {
	if what := literalTaint(text, t.pred); what != "" {
		return envValue{what: what}
	}
	if quoted {
		return envValue{}
	}
	v := envValue{}
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' {
			i++
			continue
		}
		if text[i] != '$' || i+1 >= len(text) {
			continue
		}
		rest := text[i+1:]
		if strings.HasPrefix(rest, "{{") {
			continue // an expression, judged above
		}
		if rest[0] == '(' || rest[0] == '`' {
			v.unknown = true
			continue
		}
		braced := strings.HasPrefix(rest, "{")
		rest = strings.TrimPrefix(rest, "{")
		n := 0
		for n < len(rest) && (rest[n] == '_' || rest[n] >= 'a' && rest[n] <= 'z' || rest[n] >= 'A' && rest[n] <= 'Z' || n > 0 && rest[n] >= '0' && rest[n] <= '9') {
			n++
		}
		if n == 0 || braced && (n >= len(rest) || rest[n] != '}') {
			// a parameter that is not a plain name (`$1`, `${#X}`) or has an operator (`${X:+main}`, `${X:-y}`, `${X%%.*}`)
			// does not give the value of the variable: unknown, which neither clears a saved taint nor invents one
			if n > 0 || braced || strings.ContainsRune("0123456789@*#?!-$", rune(rest[0])) {
				v.unknown = true
			}
			continue
		}
		if d := t.varAt(rest[:n], off); d != "" {
			return envValue{what: d}
		}
	}
	if strings.Contains(text, "`") {
		v.unknown = true
	}
	return v
}

// heredocEntries returns the entries a here document writes to an environment file: `NAME=value` lines and
// `NAME<<DELIM` blocks that end at a line that is DELIM.
func heredocEntries(h *runscript.Heredoc, t *revTaint) map[string]envValue {
	out := map[string]envValue{}
	lines := strings.Split(h.Body, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if name, delim, ok := strings.Cut(line, "<<"); ok && name != "" && envNameOf(name+"=") == name {
			text := ""
			for i++; i < len(lines) && lines[i] != delim; i++ {
				text += lines[i] + "\n"
			}
			out[name] = t.textValue(text, h.Quoted, h.Offset)
			continue
		}
		if name := envNameOf(line); name != "" {
			out[name] = t.textValue(line, h.Quoted, h.Offset)
		}
	}
	return out
}

// envDescs describes the variables of an env block: where an untrusted value comes from, "" for a clean one.
func envDescs(env *Env, into map[string]string, pred func(*String) (string, bool)) {
	if env == nil {
		return
	}
	for _, v := range env.Vars {
		if v.Name == nil {
			continue
		}
		into[v.Name.Value] = ""
		if v.Value != nil {
			if _, bad := pred(v.Value); bad {
				into[v.Name.Value] = "$" + v.Name.Value
			}
		}
	}
}

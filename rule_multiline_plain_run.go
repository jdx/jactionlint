package jactionlint

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// RuleMultilinePlainRun detects a `run:` which is written as a plain or quoted scalar over several lines of the file
// without a block indicator, and whose lines look like separate commands:
//
//   - run: echo "start"
//     npm ci
//     npm test
//
// YAML folds the line breaks of such a scalar into spaces, so the shell runs one command, `echo "start" npm ci npm
// test`, and the steps after the first never happen. The block indicator `|` keeps the lines.
//
// Wrapping a long single command over several lines is legitimate and common (`docker run --rm \` is not even needed:
// the fold joins the lines), so the rule reports only when a line starts with the name of a command which is not an
// argument of the command before it: the previous line does not end with an operator (`&&`, `|`, `\`, ...) and does not
// leave a quote or a bracket open, the command before it is not one that takes a command as an argument (`docker`,
// `ssh`, `sudo`, `xargs`, ...), and the line starts with a command of the list below, or with an assignment after a
// command that takes no assignments (`echo a` then `x=1`). A blank line of the scalar is a real line break in the value,
// so lines that are separated by one are not looked at.
//
// The fix writes the scalar as a literal block (`run: |`) with one line of the scalar on each line. It is offered
// only when that is exact: the scalar is the plain value of a `run:` key written on the line of the key, there is no
// blank line, no comment and no escape in it, and nothing follows it on its last line.
type RuleMultilinePlainRun struct {
	RuleBase
	runContext
	src *sourceIndex
}

// NewRuleMultilinePlainRun creates a new RuleMultilinePlainRun instance. The source of the file lets the rule
// attach fixes to its findings. It can be nil.
func NewRuleMultilinePlainRun(src []byte) *RuleMultilinePlainRun {
	r := &RuleMultilinePlainRun{
		RuleBase: NewRuleBase("multiline-plain-run", "Checks for \"run:\" written over several lines without \"|\", which YAML folds into one line"),
	}
	if src != nil {
		r.src = newSourceIndex(src)
	}
	return r
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleMultilinePlainRun) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleMultilinePlainRun) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleMultilinePlainRun) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleMultilinePlainRun) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil {
		return nil
	}
	if sh := rule.shellName(run); sh == "python" || sh == "" {
		return nil // not a shell of commands, or not known
	}
	f := foldedScalarOf(run.Run)
	if f == nil {
		return nil
	}
	idx, ok := commandBoundary(f.texts)
	if !ok {
		return nil
	}
	idx = f.textLine[idx]
	line := f.startLine + idx
	col := 1
	if rule.src != nil && line <= len(rule.src.lineStarts) {
		text := string(rule.src.src[rule.src.lineStarts[line-1]:rule.src.lineEnd(line)])
		col = utf8.RuneCountInString(text[:len(text)-len(strings.TrimLeft(text, " \t"))]) + 1
	}
	shown := 0
	for _, l := range f.texts {
		if l != "" {
			shown++
		}
	}
	e := rule.errorIDAt("multiline-plain-run", &Pos{Line: line, Col: col}, foldedMessage(f, shown)).endAt(&Pos{Line: line, Col: col + utf8.RuneCountInString(f.lines[idx])})
	rule.fix(e, run.Run, f)
	return nil
}

// foldedMessage describes what the shell gets. The text is given only when it is known exactly: YAML folds the lines of
// the scalar into spaces and keeps a blank line as a line break, `”` of a single quoted scalar is one quote, and the
// escapes of a double quoted scalar are not decoded here.
func foldedMessage(f *foldedScalar, lines int) string {
	head := "this \"run:\" is not a block scalar, so YAML folds its " + strconv.Itoa(lines) + " lines into one line"
	blank, escape := false, false
	for _, l := range f.texts {
		blank = blank || l == ""
		escape = escape || (f.style == '"' && strings.Contains(l, `\`))
	}
	if blank {
		// YAML joins the lines between two blank lines only
		return "this \"run:\" is not a block scalar, so YAML joins the lines of each group between blank lines into one line (a blank line stays a line break), and the shell runs each group as a single command. write `run: |` to keep the lines"
	}
	if escape {
		return head + " and the shell runs them as a single command (the text is not shown because the double quoted scalar has escapes). write `run: |` to keep the lines"
	}
	text := strings.Join(f.texts, " ")
	if f.style == '\'' {
		text = strings.ReplaceAll(text, "''", "'")
	}
	if utf8.RuneCountInString(text) > 80 {
		r := []rune(text)
		text = string(r[:77]) + "..."
	}
	return head + " and the shell runs the single command `" + text + "`. write `run: |` to keep the lines"
}

// keyPrefix matches the text in front of the value of a `run:` key written on one line: the indentation, an optional
// `- ` of a sequence item and the key.
var keyPrefix = regexp.MustCompile(`^( *)(- +)?run:( +)$`)

// fix attaches the fix which writes the scalar as a literal block, or the reason why there is none.
func (rule *RuleMultilinePlainRun) fix(e *Error, s *String, f *foldedScalar) {
	no := func(why string) { e.NoFix = &NoFix{Code: NoFixUnsupportedShape, Reason: why} }
	x := rule.src
	if x == nil || !x.valid {
		return
	}
	if f.endLine > len(x.lineStarts) {
		no("the position of the script in the file is unknown, add `|` after `run:` and put each command on its own line by hand")
		return
	}
	first := string(x.src[x.lineStarts[f.startLine-1]:x.lineEnd(f.startLine)])
	if f.startCol > len(first) {
		return
	}
	m := keyPrefix.FindStringSubmatch(first[:f.startCol])
	if m == nil {
		no("the script does not start on the line of the `run:` key, add `|` after `run:` and put each command on its own line by hand")
		return
	}
	last := string(x.src[x.lineStarts[f.endLine-1]:x.lineEnd(f.endLine)])
	end := f.endCol
	if f.style != 0 {
		if end >= len(last) || last[end] != f.style {
			no("the end of the quoted script is not where the fix expects it, add `|` by hand")
			return
		}
		end++
	}
	if strings.TrimSpace(last[end:]) != "" {
		no("a comment follows the script, so the fix would move it into the script. add `|` after `run:` and put each command on its own line by hand")
		return
	}
	for _, l := range f.texts {
		if l == "" {
			no("the script has a blank line, which YAML keeps as a line break, so the fix is not exact. add `|` by hand")
			return
		}
	}
	if !fixableLines(f.texts) {
		no("the fix is offered only when every line is a complete command that starts with a well-known command (no assignment, operator, open quote or bracket, heredoc or backslash), and this script is not like that. add `|` after `run:` and put each command on its own line by hand")
		return
	}
	body := make([]string, 0, len(f.texts))
	for _, l := range f.texts {
		switch {
		case l == "":
			no("the script has a blank line, which YAML keeps as a line break, so the fix is not exact. add `|` by hand")
			return
		case f.style == '"' && strings.Contains(l, `\`):
			no("the quoted script has an escape, so the fix is not exact. add `|` after `run:` and put each command on its own line by hand")
			return
		case strings.Contains(l, " #") || strings.HasPrefix(l, "#"):
			no("the script has a `#`, which is a comment in the shell but could be one in YAML too, so the fix is not exact. add `|` by hand")
			return
		}
		if f.style == '\'' {
			l = strings.ReplaceAll(l, "''", "'")
		}
		body = append(body, l)
	}
	nl := x.newline()
	indent := strings.Repeat(" ", len(m[1])+len(m[2])+2)
	var sb strings.Builder
	sb.WriteString("|")
	for _, l := range body {
		sb.WriteString(nl)
		sb.WriteString(indent)
		sb.WriteString(l)
	}
	start := x.lineStarts[f.startLine-1] + f.startCol
	stop := x.lineStarts[f.endLine-1] + end
	e.Fix = &Fix{Description: "Write the script as a block scalar", Edits: []TextEdit{{Start: start, End: stop, NewText: sb.String()}}}
}

// foldedScalar is the source text of a plain or quoted scalar that spans several lines of the file. YAML folds its
// line breaks into spaces, so the lines are one line of text.
type foldedScalar struct {
	// style is 0 for a plain scalar, '"' or '\'' for a quoted one.
	style byte
	// lines are the text of each line of the scalar in the file, without the indentation and the trailing spaces. The
	// first line starts at the first character of the value and the last one ends at its last character: quotes are
	// not part of them. A blank line of the scalar is an empty string.
	lines []string
	// texts are the lines without the first and the last line of a quoted scalar when they only hold the quote: YAML folds
	// that line break into a space, so it is not a blank line. A blank line of the scalar is still an empty string.
	// textLine is, for each text, the index of its line in lines.
	texts    []string
	textLine []int
	// startLine and endLine are the lines of the file of the first and the last line of the scalar.
	startLine, endLine int
	// startCol is the byte offset in the first line where the scalar starts (at the quote of a quoted one), and endCol the
	// byte offset in the last line right after its last character (before the closing quote of a quoted one).
	startCol, endCol int
}

// foldedScalarOf returns the lines of the string when it is a plain or quoted scalar of the file written over several
// lines. It returns nil for a block scalar, a scalar on one line and a string that was not read from a file.
func foldedScalarOf(s *String) *foldedScalar {
	if s == nil || s.src == nil || s.Pos == nil || s.Literal {
		return nil
	}
	lines := s.src.lines
	if s.Pos.Line < 1 || s.Pos.Line > len(lines) {
		return nil
	}
	first := trimCR(lines[s.Pos.Line-1])
	start, ok := byteColOf(first, s.Pos.Col)
	if !ok || start >= len(first) {
		return nil
	}
	f := &foldedScalar{startLine: s.Pos.Line, startCol: start}
	switch first[start] {
	case '|', '>':
		return nil
	case '"', '\'':
		f.style = first[start]
	}
	endLine, endCol, ok := s.valueByteAt(len(s.Value))
	if !ok || endLine <= s.Pos.Line || endLine > len(lines) {
		return nil
	}
	f.endLine = endLine
	for l := s.Pos.Line; l <= endLine; l++ {
		text := trimCR(lines[l-1])
		from, to := 0, len(text)
		if l == s.Pos.Line {
			from = start
			if f.style != 0 {
				from++
			}
		}
		if l == endLine {
			if endCol > len(text) {
				return nil
			}
			to = endCol
		}
		if from > to {
			return nil
		}
		f.lines = append(f.lines, strings.TrimSpace(text[from:to]))
	}
	f.endCol = endCol
	for i, l := range f.lines {
		if l == "" && f.style != 0 && (i == 0 || i == len(f.lines)-1) {
			continue // only the quote
		}
		f.texts = append(f.texts, l)
		f.textLine = append(f.textLine, i)
	}
	return f
}

// commandWords are the commands which, at the start of a line of a folded script, make the line a command of its own.
var commandWords = set("echo cd export mkdir rm cp mv cat ls chmod chown touch ln tar unzip zip curl wget npm npx yarn pnpm bun pip pip3 pipx uv poetry make cmake cargo rustup go git gh apt apt-get dpkg brew choco dotnet mvn gradle ./gradlew docker kubectl helm terraform aws gcloud az source set unset exit sleep printf pwd python python3 node bash sh pytest tox nox flake8 black ruff mypy rsync scp mkdocs sphinx-build")

// launcherWords are the commands which take another command as an argument, so a command of the next line may be it.
var launcherWords = set("docker kubectl helm ssh sudo doas env time nice nohup xargs exec timeout watch parallel find bash sh python python3 node xvfb-run command builtin xcrun")

// assignmentFree are the commands after which `x=1` on the next line is a separate statement and not an argument.
var assignmentFree = set("echo cd mkdir rm cp mv cat ls chmod chown touch ln tar unzip zip curl wget npm npx yarn pnpm bun pip pip3 pipx cargo go git gh apt apt-get brew sleep printf")

var assignmentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

func set(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

// continuationEnd are the endings of a line which continue the command on the next one.
var continuationEnd = []string{"&&", "||", "|", ";", "&", `\`, "(", "{", "!", "=", ",", "--", "|&", ">", ">>", "<"}

// continuationWords are the shell keywords which, as the last word of a line, continue the command on the next one.
var continuationWords = set("then do else elif in")

// wrapperWords are the commands which run the command that follows them with a changed environment, and do not change
// which command it is.
var wrapperWords = set("env command exec time nohup nice")

// commandBoundary returns the index of the first line of the lines which starts a command of its own after a
// line of another command, or false when the lines may be one wrapped command.
func commandBoundary(lines []string) (int, bool) {
	group := 0 // the first line of the group of lines that are not separated by a blank line
	for i := 0; i <= len(lines); i++ {
		if i < len(lines) && lines[i] != "" {
			continue
		}
		if idx, ok := boundaryIn(lines[group:i]); ok {
			return group + idx, true
		}
		group = i + 1
	}
	return 0, false
}

func boundaryIn(lines []string) (int, bool) {
	if len(lines) < 2 {
		return 0, false
	}
	cmd := effectiveWord(lines[0]) // the command the lines so far belong to
	text := lines[0]               // the text of the lines so far
	for i := 1; i < len(lines); i++ {
		if isNewCommand(cmd, text, firstWord(lines[i])) {
			return i, true
		}
		text += " " + lines[i]
	}
	return 0, false
}

// fixWords is the closed list of commands that a line must start with for the fix to rewrite the lines: well-known commands
// which are not assignments, keywords, wrappers, launchers or shells, so each line is a plain command of its own.
var fixWords = set("echo cd mkdir rm cp mv cat ls chmod chown touch ln tar unzip zip curl wget npm npx yarn pnpm bun pip pip3 pipx uv poetry make cmake cargo rustup go git gh apt apt-get dpkg brew choco dotnet mvn gradle ./gradlew pytest tox nox flake8 black ruff mypy rsync scp sleep printf pwd mkdocs sphinx-build")

// fixableLines reports whether the lines are in the narrow shape the fix rewrites: every line starts with a command of
// fixWords, ends complete (no backslash, operator or keyword at its end, no quote, bracket, backtick or `$(` left open),
// has no heredoc, and each one is a command of its own. When in doubt it is false.
func fixableLines(lines []string) bool {
	if !everyLineStartsACommand(lines) {
		return false
	}
	for _, l := range lines {
		if !fixWords[firstWord(l)] || assignmentRe.MatchString(firstWord(l)) {
			return false
		}
		if strings.Contains(l, "<<") || opensMore(l) {
			return false
		}
		for _, end := range continuationEnd {
			if strings.HasSuffix(l, end) {
				return false
			}
		}
		if f := strings.Fields(l); len(f) > 0 && continuationWords[f[len(f)-1]] {
			return false
		}
	}
	return true
}

// everyLineStartsACommand reports whether each line after the first one is a command of its own, so that writing the
// lines one below the other keeps what each line does. A script where some lines continue the command before them
// (`make test` then `CC=gcc`) and others do not cannot be rewritten without a guess.
func everyLineStartsACommand(lines []string) bool {
	if len(lines) < 2 {
		return false
	}
	cmd := effectiveWord(lines[0])
	text := lines[0]
	for _, l := range lines[1:] {
		if !isNewCommand(cmd, text, firstWord(l)) {
			return false
		}
		cmd, text = effectiveWord(l), l
	}
	return true
}

func firstWord(line string) string {
	f := strings.Fields(line)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// effectiveWord returns the command which a line runs: the first word that is not an assignment (`DEBIAN_FRONTEND=x`),
// a wrapper (`env`, `time`, `nohup`, ...) or an option of a wrapper. It is "" when the line has nothing else: the
// assignments then belong to the command of the next line.
func effectiveWord(line string) string {
	f := strings.Fields(line)
	for _, w := range f {
		if assignmentRe.MatchString(w) || wrapperWords[w] || (strings.HasPrefix(w, "-") && len(w) > 1) {
			continue
		}
		return w
	}
	return ""
}

// isNewCommand reports whether the line which starts with the word is a command of its own after the text, which is
// the command cmd and the lines folded into it.
func isNewCommand(cmd, text, w string) bool {
	if !commandWords[cmd] {
		return false // what the text is is not known: a script of the repository, a variable, an assignment, ...
	}
	if launcherWords[cmd] {
		return false
	}
	for _, end := range continuationEnd {
		if strings.HasSuffix(text, end) {
			return false
		}
	}
	if f := strings.Fields(text); len(f) > 0 && continuationWords[f[len(f)-1]] {
		return false
	}
	if opensMore(text) {
		return false
	}
	if commandWords[w] {
		return true
	}
	return assignmentRe.MatchString(w) && assignmentFree[cmd]
}

// opensMore reports whether the text leaves a quote, a bracket or an expression open, so that the next line is part of it.
func opensMore(text string) bool {
	if strings.Count(text, "${{") > strings.Count(text, "}}") {
		return true
	}
	var single, double, backtick bool
	depth := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '\\' && !single:
			i++
		case c == '\'' && !double:
			single = !single
		case c == '"' && !single:
			double = !double
		case c == '`' && !single:
			backtick = !backtick
		case !single && !double && (c == '(' || c == '{' || c == '['):
			depth++
		case !single && !double && (c == ')' || c == '}' || c == ']'):
			depth--
		}
	}
	return single || double || backtick || depth > 0
}

func init() {
	registerRules(
		RuleInfo{ID: "multiline-plain-run", Group: RuleGroupCorrectness, Summary: "A run: is written over several lines without |, so YAML folds the lines into one command.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, Fixable: true, DocsAnchor: "check-multiline-plain-run"},
	)
	registerRuleFactory("multiline-plain-run", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleMultilinePlainRun(env.src)}
	})
}

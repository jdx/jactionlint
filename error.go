package jactionlint

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"text/template"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
)

var (
	bold   = color.New(color.Bold)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	gray   = color.New(color.FgHiBlack)
)

// Error represents an error detected by jactionlint rules
type Error struct {
	// Message is an error message.
	Message string
	// Filepath is a file path where the error occurred.
	Filepath string
	// Line is a line number where the error occurred. This value is 1-based.
	Line int
	// Column is a column number where the error occurred. This value is 1-based and counts Unicode
	// code points.
	Column int
	// EndLine is the line number of the end of the problematic region. This value is 1-based. It is
	// the same as Line unless the region spans several lines.
	EndLine int
	// EndColumn is the column number just after the end of the problematic region on EndLine. It is
	// 1-based and counts Unicode code points, so a region of one character at column 5 has EndColumn
	// 6. When a rule did not report where the region ends, the linter fills it with the end of the
	// token which starts at Line and Column.
	EndColumn int
	// Kind is a string to represent kind of the error. Usually rule name which found the error. It is
	// the legacy way to group errors. Use ID to tell which diagnostic was reported.
	Kind string
	// ID is the stable identifier of the diagnostic such as "unpinned-uses". See Rules for all IDs of
	// jactionlint. Errors reported by custom rules through RuleBase.Error have the Kind as their ID.
	ID string
	// Severity is how serious the error is. It is determined by the configuration: the level of the
	// rule or the default level of the rule.
	Severity Severity
	// DocURL is a URL of the documentation of the diagnostic. It is empty for custom rules.
	DocURL string
	// Fix is an automatic correction for the error. It is nil when the error cannot be fixed
	// mechanically.
	Fix *Fix
	// Baselined is true when the baseline file accepts the finding (see Config.Baseline). The linter
	// does not return such errors from its methods or print them, except in the SARIF log (as
	// suppressed results) and the summary format; it is set only on the way there.
	Baselined bool
}

// Fix is an automatic correction for an Error. It is a list of edits to a single file. The edits
// must not overlap each other.
type Fix struct {
	// Description describes what applying the fix does, e.g. "Add timeout-minutes".
	Description string `json:"description"`
	// Unsafe marks a fix which may change the behavior of the workflow. Unsafe fixes are applied
	// only when requested explicitly and are not put in SARIF output.
	Unsafe bool `json:"unsafe,omitempty"`
	// Edits are the replacements to apply.
	Edits []TextEdit `json:"edits"`
}

// TextEdit replaces the bytes in [Start, End) of a file with NewText. Offsets are byte offsets
// from the beginning of the file. Start == End inserts the text, and an empty NewText deletes the
// range.
type TextEdit struct {
	// Start is the byte offset where the replaced range starts.
	Start int `json:"start"`
	// End is the byte offset just after the replaced range.
	End int `json:"end"`
	// NewText is the text to put in place of the range.
	NewText string `json:"new_text"`
}

// validFor reports whether all edits are inside the source and none of them overlap.
func (f *Fix) validFor(src []byte) bool {
	if f == nil || len(f.Edits) == 0 {
		return false
	}
	edits := slices.Clone(f.Edits)
	slices.SortFunc(edits, func(a, b TextEdit) int {
		if a.Start != b.Start {
			return a.Start - b.Start
		}
		return a.End - b.End
	})
	prevEnd := 0
	for i, e := range edits {
		if e.Start < 0 || e.End < e.Start || e.End > len(src) {
			return false
		}
		if i > 0 && e.Start < prevEnd {
			return false
		}
		prevEnd = e.End
	}
	return true
}

// Error returns summary of the error as string.
func (e *Error) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s [%s]", e.Filepath, e.Line, e.Column, e.Message, e.Kind)
}

func (e *Error) String() string {
	return e.Error()
}

func errorAt(pos *Pos, kind string, id string, msg string) *Error {
	return &Error{
		Message: msg,
		Line:    pos.Line,
		Column:  pos.Col,
		Kind:    kind,
		ID:      id,
	}
}

func errorfAt(pos *Pos, kind string, id string, format string, args ...interface{}) *Error {
	return errorAt(pos, kind, id, fmt.Sprintf(format, args...))
}

// GetTemplateFields fields for formatting this error with Go template.
func (e *Error) GetTemplateFields(source []byte) *ErrorTemplateFields {
	snippet := ""
	end := e.Column
	if len(source) > 0 && e.Line > 0 {
		if l, ok := e.getLine(source); ok {
			snippet = l
			if len(l) >= e.Column-1 {
				if i := e.getIndicator(l); i != "" {
					snippet += "\n" + i
					end = len(i) // Byte length can be used here because this line only contains ASCII
				}
			}
		}
	}

	endLine := e.EndLine
	if endLine == 0 {
		endLine = e.Line
	}

	return &ErrorTemplateFields{
		Message:   e.Message,
		Filepath:  e.Filepath,
		Line:      e.Line,
		Column:    e.Column,
		Kind:      e.Kind,
		Snippet:   snippet,
		EndColumn: end,
		ID:        e.ID,
		Severity:  e.Severity,
		DocURL:    e.DocURL,
		EndLine:   endLine,
		Fix:       e.Fix,
	}
}

// PrettyPrint prints the error with user-friendly way. It prints file name, source position, error
// message with colorful output and source snippet with indicator. When nil is set to source, no
// source snippet is not printed. To disable colorful output, set true to fatih/color.NoColor.
func (e *Error) PrettyPrint(w io.Writer, source []byte) {
	e.prettyPrint(w, source, false)
}

// prettyPrint is PrettyPrint which can show the rule ID instead of the kind. The output format of an
// error of error level is the one of the former versions. Errors of the other levels are prefixed
// with their level so that they can be told apart from errors.
func (e *Error) prettyPrint(w io.Writer, source []byte, showID bool) {
	yellow.Fprint(w, e.Filepath)
	gray.Fprint(w, ":")
	fmt.Fprint(w, e.Line)
	gray.Fprint(w, ":")
	fmt.Fprint(w, e.Column)
	gray.Fprint(w, ": ")
	prefix := ""
	switch e.Severity {
	case SeverityWarning:
		prefix = "warning: "
	case SeverityInfo:
		prefix = "info: "
	}
	bold.Fprint(w, prefix+e.Message)
	label := e.Kind
	if showID && e.ID != "" {
		label = e.ID
	}
	gray.Fprintf(w, " [%s]\n", label)

	if len(source) == 0 || e.Line <= 0 {
		return
	}
	line, ok := e.getLine(source)
	if !ok || len(line) < e.Column-1 {
		return
	}

	lnum := fmt.Sprintf("%d | ", e.Line)
	indent := strings.Repeat(" ", len(lnum)-2)
	gray.Fprintf(w, "%s|\n", indent)
	gray.Fprint(w, lnum)
	fmt.Fprintln(w, line)
	gray.Fprintf(w, "%s| ", indent)
	green.Fprintln(w, e.getIndicator(line))
}

func (e *Error) getLine(source []byte) (string, bool) {
	s := bufio.NewScanner(bytes.NewReader(source))
	l := 0
	for s.Scan() {
		l++
		if l == e.Line {
			return s.Text(), true
		}
	}
	return "", false
}

func (e *Error) getIndicator(line string) string {
	if e.Column <= 0 {
		return ""
	}

	start := e.Column - 1 // Column is 1-based

	// Count width of non-space characters after '^' for underline
	uw := 0
	r := strings.NewReader(line[start:])
	for {
		c, s, err := r.ReadRune()
		if err != nil || s == 0 || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		uw += runewidth.RuneWidth(c)
	}
	if uw > 0 {
		uw-- // Decrement for place for '^'
	}

	// Count width of spaces before '^'
	sw := runewidth.StringWidth(line[:start])
	return fmt.Sprintf("%s^%s", strings.Repeat(" ", sw), strings.Repeat("~", uw))
}

func compareErrors(lhs, rhs *Error) int {
	if c := strings.Compare(lhs.Filepath, rhs.Filepath); c != 0 {
		return c
	}
	if lhs.Line != rhs.Line {
		return lhs.Line - rhs.Line
	}
	if lhs.Column != rhs.Column {
		return lhs.Column - rhs.Column
	}
	return strings.Compare(lhs.Message, rhs.Message)
}

func equalsErrors(lhs, rhs *Error) bool {
	return lhs.Filepath == rhs.Filepath &&
		lhs.Line == rhs.Line &&
		lhs.Column == rhs.Column &&
		lhs.Message == rhs.Message
}

// ErrorTemplateFields holds all fields to format one error message.
type ErrorTemplateFields struct {
	// Message is error message body.
	Message string `json:"message"`
	// Filepath is a canonical relative file path. This is empty when input was read from stdin.
	// When encoding into JSON, this field may be omitted when the file path is empty.
	Filepath string `json:"filepath,omitempty"`
	// Line is a line number of error position.
	Line int `json:"line"`
	// Column is a column number of error position.
	Column int `json:"column"`
	// Kind is a rule name the error belongs to.
	Kind string `json:"kind"`
	// Snippet is a code snippet and indicator to indicate where the error occurred.
	// When encoding into JSON, this field may be omitted when the snippet is empty.
	Snippet string `json:"snippet,omitempty"`
	// EndColumn is a column number where the error indicator (^~~~~~~) ends. When no indicator
	// can be shown, EndColumn is equal to Column. Note that it is the column of the last character
	// of the indicator and counts the display width of the characters, which differs from
	// Error.EndColumn that is the exclusive end of the region counted in Unicode code points.
	EndColumn int `json:"end_column"`
	// ID is the stable ID of the rule which found the error such as "unpinned-uses".
	ID string `json:"id"`
	// Severity is "error", "warn" or "info".
	Severity Severity `json:"severity"`
	// DocURL is the URL of the documentation of the rule. When encoding into JSON, this field may be
	// omitted when the error is not from a built-in rule.
	DocURL string `json:"doc_url,omitempty"`
	// EndLine is the line number where the region of the error ends. It is the same as Line unless the
	// region spans several lines.
	EndLine int `json:"end_line"`
	// Fix is the automatic fix for the error. When encoding into JSON, this field is omitted when the
	// error cannot be fixed automatically.
	Fix *Fix `json:"fix,omitempty"`
}

func unescapeBackslash(s string) string {
	// https://golang.org/ref/spec#Rune_literals
	r := strings.NewReplacer(
		`\a`, "\a",
		`\b`, "\b",
		`\f`, "\f",
		`\n`, "\n",
		`\r`, "\r",
		`\t`, "\t",
		`\v`, "\v",
		`\\`, "\\",
	)
	return r.Replace(s)
}

func toPascalCase(s string) string {
	ss := strings.FieldsFunc(s, func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9')
	})
	for i, s := range ss {
		var c rune
		for _, c = range s {
			break
		}
		if 'a' <= c && c <= 'z' {
			ss[i] = strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return strings.Join(ss, "")
}

type ruleTemplateFields struct {
	Name        string
	Description string
}

// ruleInfoTemplateFields is the fields of a rule which a template for formatting errors can use through
// the allRules function.
type ruleInfoTemplateFields struct {
	// ID is the stable ID of the rule such as "unpinned-uses".
	ID string
	// Name is the ID in Pascal case such as "UnpinnedUses".
	Name string
	// Description is a one-line description of the rule.
	Description string
	// Group is "correctness", "security", "policy" or "style".
	Group string
	// DefaultLevel is the severity of the rule when it is enabled without an explicit level.
	DefaultLevel Severity
	// Profile is the first profile which enables the rule. It is empty when no profile enables it.
	Profile string
	// URL is the URL of the documentation of the rule.
	URL string
}

func compareRuleTemplateByName(lhs, rhs *ruleTemplateFields) int {
	return strings.Compare(lhs.Name, rhs.Name)
}

// ErrorFormatter is a formatter to format a slice of ErrorTemplateFields. It is used for
// formatting error messages with -format option.
type ErrorFormatter struct {
	temp    *template.Template
	rules   map[string]*ruleTemplateFields
	rulesMu sync.Mutex
}

// NewErrorFormatter creates new ErrorFormatter instance. Given format must contain at least one
// {{ }} placeholder. Escaped characters like \n in the format string are unescaped.
func NewErrorFormatter(format string) (*ErrorFormatter, error) {
	if !strings.Contains(format, "{{") {
		return nil, fmt.Errorf("template to format error messages must contain at least one {{ }} placeholder: %s", format)
	}

	r := map[string]*ruleTemplateFields{
		"syntax-check": {"syntax-check", "Checks for GitHub Actions workflow syntax"},
	}

	funcs := template.FuncMap(map[string]interface{}{
		"json": func(data interface{}) (string, error) {
			var b strings.Builder
			enc := json.NewEncoder(&b)
			if err := enc.Encode(data); err != nil {
				return "", fmt.Errorf("could not encode template value into JSON: %w", err)
			}
			return b.String(), nil
		},
		"replace": func(s string, oldnew ...string) string {
			return strings.NewReplacer(oldnew...).Replace(s)
		},
		"toPascalCase": toPascalCase,
		"getVersion":   getCommandVersion,
		"allRules": func() []*ruleInfoTemplateFields {
			rules := Rules()
			ret := make([]*ruleInfoTemplateFields, 0, len(rules))
			for _, r := range rules {
				ret = append(ret, &ruleInfoTemplateFields{
					ID:           r.ID,
					Name:         toPascalCase(r.ID),
					Description:  r.Summary,
					Group:        string(r.Group),
					DefaultLevel: r.DefaultLevel,
					Profile:      string(r.Profile),
					URL:          r.DocURL(),
				})
			}
			return ret
		},
		"allKinds": func() []*ruleTemplateFields {
			ret := make([]*ruleTemplateFields, 0, len(r))
			for _, e := range r {
				ret = append(ret, e)
			}
			slices.SortFunc(ret, compareRuleTemplateByName)
			return ret
		},
	})
	t, err := template.New("error formatter").Funcs(funcs).Parse(unescapeBackslash(format))
	if err != nil {
		return nil, fmt.Errorf("template %q to format error messages could not be parsed: %w", format, err)
	}

	return &ErrorFormatter{t, r, sync.Mutex{}}, nil
}

// Print formats the slice of template fields and prints it with given writer.
func (f *ErrorFormatter) Print(out io.Writer, t []*ErrorTemplateFields) error {
	if err := f.temp.Execute(out, t); err != nil {
		return fmt.Errorf("could not format error messages: %w", err)
	}
	return nil
}

// PrintErrors prints the errors after formatting them with template.
func (f *ErrorFormatter) PrintErrors(out io.Writer, errs []*Error, src []byte) error {
	t := make([]*ErrorTemplateFields, 0, len(errs))
	for _, err := range errs {
		t = append(t, err.GetTemplateFields(src))
	}
	return f.Print(out, t)
}

// RegisterRule registers the rule. Registered rules are used to get description and index of error
// kinds when you use `kindDescription` or `kindIndex` functions in an error format template. This
// method can be called multiple times safely in parallel.
func (f *ErrorFormatter) RegisterRule(r Rule) {
	// Synchronize access to f.rules (#370)
	f.rulesMu.Lock()
	defer f.rulesMu.Unlock()

	n := r.Name()
	if _, ok := f.rules[n]; !ok {
		f.rules[n] = &ruleTemplateFields{n, r.Description()}
	}
}

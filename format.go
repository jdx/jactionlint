package jactionlint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Output formats which -format and LinterOptions.Format accept besides Go templates.
const (
	// FormatText is the default format. Each error is printed with its source snippet and an indicator.
	FormatText = "text"
	// FormatOneline prints one error per line without the source snippet.
	FormatOneline = "oneline"
	// FormatJSON prints a JSON array of the errors. It is the same as the "{{json .}}" template.
	FormatJSON = "json"
	// FormatJSONL prints one JSON object per error per line (JSON Lines).
	FormatJSONL = "jsonl"
	// FormatSARIF prints a SARIF 2.1.0 log.
	FormatSARIF = "sarif"
	// FormatGCC prints "file:line:col: severity: message [id]" like GCC does.
	FormatGCC = "gcc"
	// FormatGitHub prints GitHub Actions workflow commands which annotate the files in the pull request.
	FormatGitHub = "github"
)

// nativeFormats are the names of the formats which are not Go templates.
var nativeFormats = []string{FormatText, FormatOneline, FormatJSON, FormatJSONL, FormatSARIF, FormatGCC, FormatGitHub}

// fileResult is the result of linting one file.
type fileResult struct {
	// file is the path of the file as it was given to the linter. It is empty for content which did
	// not come from a file.
	file string
	// path is the path shown in the errors: relative to the working directory when possible.
	path string
	src  []byte
	errs []*Error
}

// printer prints the results of linting.
type printer interface {
	// print prints the results. The notes are messages about the run itself, e.g. deprecation warnings
	// of the configuration. Formats which have a place for them print them there. The other formats
	// ignore them because the linter has already written them to the log.
	print(w io.Writer, results []fileResult, notes []string) error
}

// structured reports whether the printer makes a document which other tools parse. For such a format the
// log must stay free of the messages which the document carries, since tools like hk parse stdout and
// stderr together.
func structured(p printer) bool {
	_, ok := p.(sarifPrinter)
	return ok
}

// newPrinter creates the printer for the format. A format with "{{" is a Go template.
func newPrinter(format string, oneline, showIDs bool, tmpl *ErrorFormatter) (printer, error) {
	switch format {
	case "", FormatText:
		return textPrinter{oneline: oneline, showIDs: showIDs}, nil
	case FormatOneline:
		return textPrinter{oneline: true, showIDs: showIDs}, nil
	case FormatJSON:
		return jsonPrinter{}, nil
	case FormatJSONL:
		return jsonlPrinter{}, nil
	case FormatSARIF:
		return sarifPrinter{}, nil
	case FormatGCC:
		return gccPrinter{}, nil
	case FormatGitHub:
		return githubPrinter{}, nil
	}
	if tmpl != nil {
		return templatePrinter{tmpl}, nil
	}
	return nil, fmt.Errorf("invalid format %q. available formats are %s. a Go template which has at least one {{ }} placeholder can be also used", format, quotes(nativeFormats))
}

// isTemplateFormat reports whether the format is a Go template instead of one of the native formats.
func isTemplateFormat(format string) bool {
	return format != "" && !slices.Contains(nativeFormats, format) && strings.Contains(format, "{{")
}

// --- text -----------------------------------------------------------------------------------

type textPrinter struct {
	oneline, showIDs bool
}

func (p textPrinter) print(w io.Writer, results []fileResult, _ []string) error {
	for _, r := range results {
		src := r.src
		if p.oneline {
			src = nil
		}
		for _, e := range r.errs {
			e.prettyPrint(w, src, p.showIDs)
		}
	}
	return nil
}

// --- template -------------------------------------------------------------------------------

type templatePrinter struct {
	f *ErrorFormatter
}

func (p templatePrinter) print(w io.Writer, results []fileResult, _ []string) error {
	var fields []*ErrorTemplateFields
	for _, r := range results {
		for _, e := range r.errs {
			fields = append(fields, e.GetTemplateFields(r.src))
		}
	}
	if fields == nil {
		fields = []*ErrorTemplateFields{}
	}
	return p.f.Print(w, fields)
}

// --- json -----------------------------------------------------------------------------------

func allTemplateFields(results []fileResult) []*ErrorTemplateFields {
	fields := []*ErrorTemplateFields{}
	for _, r := range results {
		for _, e := range r.errs {
			fields = append(fields, e.GetTemplateFields(r.src))
		}
	}
	return fields
}

func encodeJSON(w io.Writer, v any) error {
	// Same encoding as the "json" function of the template for formatting errors
	return json.NewEncoder(w).Encode(v)
}

type jsonPrinter struct{}

func (jsonPrinter) print(w io.Writer, results []fileResult, _ []string) error {
	return encodeJSON(w, allTemplateFields(results))
}

type jsonlPrinter struct{}

func (jsonlPrinter) print(w io.Writer, results []fileResult, _ []string) error {
	for _, f := range allTemplateFields(results) {
		if err := encodeJSON(w, f); err != nil {
			return err
		}
	}
	return nil
}

// --- gcc and github -------------------------------------------------------------------------

type gccPrinter struct{}

func (gccPrinter) print(w io.Writer, results []fileResult, _ []string) error {
	for _, r := range results {
		for _, e := range r.errs {
			fmt.Fprintf(w, "%s:%d:%d: %s: %s [%s]\n", e.Filepath, e.Line, e.Column, e.Severity.gccName(), e.Message, e.ID)
		}
	}
	return nil
}

func (s Severity) gccName() string {
	switch s {
	case SeverityInfo:
		return "note"
	case SeverityWarning:
		return "warning"
	}
	return "error"
}

type githubPrinter struct{}

var (
	githubDataEscaper     = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	githubPropertyEscaper = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
)

func (githubPrinter) print(w io.Writer, results []fileResult, _ []string) error {
	for _, r := range results {
		for _, e := range r.errs {
			cmd := "error"
			switch e.Severity {
			case SeverityWarning:
				cmd = "warning"
			case SeverityInfo:
				cmd = "notice"
			}
			props := []string{"file=" + githubPropertyEscaper.Replace(filepath.ToSlash(e.Filepath))}
			if e.Line > 0 {
				props = append(props, fmt.Sprintf("line=%d", e.Line))
				if e.EndLine > 0 {
					props = append(props, fmt.Sprintf("endLine=%d", e.EndLine))
				}
				if e.Column > 0 {
					props = append(props, fmt.Sprintf("col=%d", e.Column))
					// GitHub takes an inclusive end column but Error.EndColumn is exclusive
					if e.EndColumn > e.Column && e.EndLine == e.Line {
						props = append(props, fmt.Sprintf("endColumn=%d", e.EndColumn-1))
					}
				}
			}
			props = append(props, "title="+githubPropertyEscaper.Replace(e.ID))
			fmt.Fprintf(w, "::%s %s::%s\n", cmd, strings.Join(props, ","), githubDataEscaper.Replace(e.Message))
		}
	}
	return nil
}

// --- sarif ----------------------------------------------------------------------------------

type sarifPrinter struct{}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifReplacement struct {
	DeletedRegion   sarifRegion `json:"deletedRegion"`
	InsertedContent struct {
		Text string `json:"text"`
	} `json:"insertedContent"`
}

type sarifArtifactChange struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Replacements     []sarifReplacement    `json:"replacements"`
}

type sarifFix struct {
	Description     sarifMessage          `json:"description"`
	ArtifactChanges []sarifArtifactChange `json:"artifactChanges"`
}

type sarifResult struct {
	RuleID     string            `json:"ruleId"`
	RuleIndex  *int              `json:"ruleIndex,omitempty"`
	Level      string            `json:"level"`
	Message    sarifMessage      `json:"message"`
	Locations  []sarifLocation   `json:"locations"`
	Fixes      []sarifFix        `json:"fixes,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifRule struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	ShortDescription sarifMessage    `json:"shortDescription"`
	HelpURI          string          `json:"helpUri"`
	DefaultConfig    sarifRuleConfig `json:"defaultConfiguration"`
	Properties       map[string]any  `json:"properties"`
}

type sarifDriver struct {
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	SemanticVersion string      `json:"semanticVersion,omitempty"`
	InformationURI  string      `json:"informationUri"`
	Rules           []sarifRule `json:"rules"`
}

type sarifNotification struct {
	Level   string       `json:"level"`
	Message sarifMessage `json:"message"`
}

type sarifInvocation struct {
	ExecutionSuccessful            bool                `json:"executionSuccessful"`
	ToolConfigurationNotifications []sarifNotification `json:"toolConfigurationNotifications,omitempty"`
}

type sarifRun struct {
	Tool struct {
		Driver sarifDriver `json:"driver"`
	} `json:"tool"`
	Invocations []sarifInvocation `json:"invocations,omitempty"`
	ColumnKind  string            `json:"columnKind"`
	Results     []sarifResult     `json:"results"`
}

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

func (s Severity) sarifLevel() string {
	switch s {
	case SeverityInfo:
		return "note"
	case SeverityWarning:
		return "warning"
	}
	return "error"
}

// sarifURI returns the URI of a file path. A relative path stays relative so that consumers resolve it
// from the directory where jactionlint ran, which is how hk and GitHub find the file.
func sarifURI(path string) string {
	p := filepath.ToSlash(path)
	u := url.URL{Path: p}
	if filepath.IsAbs(path) {
		u.Scheme = "file"
		if !strings.HasPrefix(p, "/") {
			u.Path = "/" + p // Windows drive letter
		}
	}
	return u.String()
}

// offsetPosition returns the 1-based line and the 1-based column counted in Unicode code points of the
// byte offset in the source. A carriage return before a line feed is not counted so that the end of a
// line is the same position with LF and CRLF.
func offsetPosition(src []byte, off int) (line, col int) {
	line, col = 1, 1
	for i := 0; i < off; {
		r, w := utf8.DecodeRune(src[i:])
		switch {
		case r == '\n':
			line++
			col = 1
		case r == '\r' && i+1 < len(src) && src[i+1] == '\n':
			// Not counted
		default:
			col++
		}
		i += w
	}
	// An offset at the carriage return of CRLF is the end of the line
	return line, col
}

// editsConflict reports whether two edits cannot be applied together. Identical edits do not conflict
// since tools apply them once.
func editsConflict(a, b TextEdit) bool {
	if a == b {
		return false
	}
	if a.Start > b.Start {
		a, b = b, a
	}
	return b.Start < a.End || (b.Start == a.Start && (a.End == a.Start || b.End == b.Start))
}

// sarifFixes builds the "fixes" of a result. It returns nil when the error has no fix, when the fix is
// unsafe, or when its edits are invalid or conflict with the edits which are already in the log. Tools
// like "hk util sarif-diff" apply the fixes of all results together and give up when any of them is
// broken, so a fix which could not be applied is better left out: the finding is then reported as one
// which needs the fixer.
func sarifFixes(e *Error, src []byte, accepted *[]TextEdit) []sarifFix {
	f := e.Fix
	if f == nil || f.Unsafe || !f.validFor(src) {
		return nil
	}
	for _, edit := range f.Edits {
		for _, a := range *accepted {
			if editsConflict(a, edit) {
				return nil
			}
		}
	}

	uri := sarifURI(e.Filepath)
	edits := slices.Clone(f.Edits)
	slices.SortFunc(edits, func(a, b TextEdit) int {
		if a.Start != b.Start {
			return a.Start - b.Start
		}
		return a.End - b.End
	})
	change := sarifArtifactChange{ArtifactLocation: sarifArtifactLocation{URI: uri}}
	for _, edit := range edits {
		sl, sc := offsetPosition(src, edit.Start)
		el, ec := offsetPosition(src, edit.End)
		var r sarifReplacement
		r.DeletedRegion = sarifRegion{StartLine: sl, StartColumn: sc, EndLine: el, EndColumn: ec}
		r.InsertedContent.Text = edit.NewText
		change.Replacements = append(change.Replacements, r)
		*accepted = append(*accepted, edit)
	}
	desc := f.Description
	if desc == "" {
		desc = "Fix " + e.ID
	}
	return []sarifFix{{Description: sarifMessage{desc}, ArtifactChanges: []sarifArtifactChange{change}}}
}

func (sarifPrinter) print(w io.Writer, results []fileResult, notes []string) error {
	// Files are sorted so that the log does not depend on the order of the arguments
	results = slices.Clone(results)
	slices.SortStableFunc(results, func(a, b fileResult) int { return strings.Compare(a.path, b.path) })

	run := sarifRun{ColumnKind: "unicodeCodePoints", Results: []sarifResult{}}
	if len(notes) > 0 {
		inv := sarifInvocation{ExecutionSuccessful: true}
		for _, n := range notes {
			inv.ToolConfigurationNotifications = append(inv.ToolConfigurationNotifications, sarifNotification{"warning", sarifMessage{n}})
		}
		run.Invocations = []sarifInvocation{inv}
	}
	d := &run.Tool.Driver
	d.Name = "jactionlint"
	d.Version = getCommandVersion()
	if semverRe.MatchString(d.Version) {
		d.SemanticVersion = d.Version
	}
	d.InformationURI = "https://github.com/jdx/jactionlint"
	d.Rules = []sarifRule{}

	// Only the rules which appear in the results are described
	used := map[string]bool{}
	for _, r := range results {
		for _, e := range r.errs {
			used[e.ID] = true
		}
	}
	ids := make([]string, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	index := map[string]int{}
	for _, id := range ids {
		info, ok := ruleIndex[id]
		if !ok {
			continue // Custom rule
		}
		index[id] = len(d.Rules)
		d.Rules = append(d.Rules, sarifRule{
			ID:               info.ID,
			Name:             toPascalCase(info.ID),
			ShortDescription: sarifMessage{info.Summary},
			HelpURI:          info.DocURL(),
			DefaultConfig:    sarifRuleConfig{Level: info.DefaultLevel.sarifLevel()},
			Properties:       map[string]any{"tags": []string{string(info.Group)}},
		})
	}

	for _, r := range results {
		var accepted []TextEdit
		for _, e := range r.errs {
			res := sarifResult{
				RuleID:  e.ID,
				Level:   e.Severity.sarifLevel(),
				Message: sarifMessage{e.Message},
				Locations: []sarifLocation{{PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: sarifURI(e.Filepath)},
				}}},
			}
			if i, ok := index[e.ID]; ok {
				res.RuleIndex = &i
			}
			if e.Kind != "" && e.Kind != e.ID {
				res.Properties = map[string]string{"kind": e.Kind}
			}
			if e.Line > 0 {
				reg := &sarifRegion{StartLine: e.Line}
				if e.Column > 0 {
					reg.StartColumn = e.Column
					if e.EndLine >= e.Line && e.EndColumn > 0 {
						reg.EndLine, reg.EndColumn = e.EndLine, e.EndColumn
					}
				}
				res.Locations[0].PhysicalLocation.Region = reg
			}
			res.Fixes = sarifFixes(e, r.src, &accepted)
			run.Results = append(run.Results, res)
		}
	}

	log := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs:    []sarifRun{run},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(log); err != nil {
		return fmt.Errorf("could not encode SARIF log: %w", err)
	}
	_, err := w.Write(buf.Bytes())
	return err
}

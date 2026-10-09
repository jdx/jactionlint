package jactionlint

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// DefaultBaselineFile is the path of the baseline file relative to the root of the repository.
const DefaultBaselineFile = ".github/jactionlint-baseline.json"

// baselineVersion is the format version of the baseline file.
const baselineVersion = 1

// BaselineEntry is one accepted finding of a baseline file. It identifies the finding without its
// line number so that it survives edits which only move the line: the file, the rule, a fingerprint
// of the finding and an occurrence index for findings that look identical.
type BaselineEntry struct {
	// File is the path of the file relative to the root of the repository, with forward slashes.
	File string `json:"file"`
	// Rule is the ID of the rule which reported the finding.
	Rule string `json:"rule"`
	// Fingerprint is a hash of the rule, the enclosing job or top-level key, the trimmed source line
	// and the normalized message.
	Fingerprint string `json:"fingerprint"`
	// Context is a hash of the same without the message. A finding whose fingerprint changed only
	// because a new release rewords the message still matches the entry through it.
	Context string `json:"context"`
	// Occurrence tells identical findings (same fingerprint in the same file) apart, counting from 0
	// in the order of the file.
	Occurrence int `json:"occurrence"`
	// Message is the message of the finding when the baseline was written. It is for people reading the
	// file; matching does not use it.
	Message string `json:"message"`

	// line is the line of the entry in the baseline file, for reporting unused entries.
	line int
}

// Baseline is a set of accepted findings. See ReadBaselineFile.
type Baseline struct {
	// Version is the format version of the file.
	Version int `json:"version"`
	// Entries are the accepted findings in the order of the file.
	Entries []*BaselineEntry `json:"entries"`
}

// ReadBaselineFile reads a baseline file written by "jactionlint -baseline-write".
func ReadBaselineFile(path string) (*Baseline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read baseline file %q: %w", path, err)
	}
	bl, err := ParseBaseline(b)
	if err != nil {
		return nil, fmt.Errorf("could not parse baseline file %q: %w", path, err)
	}
	return bl, nil
}

// ParseBaseline parses the content of a baseline file.
func ParseBaseline(b []byte) (*Baseline, error) {
	bl := &Baseline{}
	dec := json.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("a baseline must be a JSON object")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := tok.(string)
		switch key {
		case "version":
			if err := dec.Decode(&bl.Version); err != nil {
				return nil, fmt.Errorf("invalid \"version\": %w", err)
			}
		case "entries":
			if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
				return nil, errors.New("\"entries\" must be an array")
			}
			for dec.More() {
				var raw json.RawMessage
				if err := dec.Decode(&raw); err != nil {
					return nil, err
				}
				start := int(dec.InputOffset()) - len(raw)
				var e BaselineEntry
				if err := json.Unmarshal(raw, &e); err != nil {
					return nil, fmt.Errorf("invalid entry %d: %w", len(bl.Entries)+1, err)
				}
				if e.File == "" || e.Rule == "" || e.Fingerprint == "" {
					return nil, fmt.Errorf("entry %d needs \"file\", \"rule\" and \"fingerprint\"", len(bl.Entries)+1)
				}
				if start < 0 || start > len(b) {
					start = 0
				}
				e.line = 1 + bytes.Count(b[:start], []byte("\n"))
				bl.Entries = append(bl.Entries, &e)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil, err
			}
		}
	}
	if bl.Version != baselineVersion {
		return nil, fmt.Errorf("unsupported baseline version %d. this jactionlint reads version %d. regenerate the file with -baseline-write", bl.Version, baselineVersion)
	}
	return bl, nil
}

// Marshal returns the content of the baseline file. The output is deterministic so writing the same
// findings twice gives the same bytes.
func (bl *Baseline) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	out := struct {
		Version int              `json:"version"`
		Entries []*BaselineEntry `json:"entries"`
	}{baselineVersion, bl.Entries}
	if out.Entries == nil {
		out.Entries = []*BaselineEntry{}
	}
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- fingerprints ---------------------------------------------------------------------------

// baselineInfo is the identity of a finding in a baseline. It is computed once the findings of a
// file are final.
type baselineInfo struct {
	file        string
	fingerprint string
	context     string
	occurrence  int
}

var (
	lineRefRe = regexp.MustCompile(`(?i)\b(line|lines|column|col)(:\s*|\s+)\d+`)
	keyLineRe = regexp.MustCompile(`^(?:"[^"]*"|'[^']*'|[A-Za-z0-9_.\-/ ]+?)\s*:(?:\s|$)`)
)

// normalizeBaselineMessage removes what changes without the finding changing: runs of spaces, line or
// column numbers the message mentions and the path of the repository.
func normalizeBaselineMessage(msg, root string) string {
	if root != "" {
		// Some messages contain absolute paths. A baseline is shared by checkouts at different places.
		for _, r := range []string{root, filepath.ToSlash(root)} {
			if len(r) > 1 {
				msg = strings.ReplaceAll(msg, r, "<root>")
			}
		}
	}
	msg = lineRefRe.ReplaceAllString(msg, "$1 N")
	return strings.Join(strings.Fields(msg), " ")
}

func hashParts(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func lineIndent(l []byte) int {
	n := 0
	for n < len(l) && l[n] == ' ' {
		n++
	}
	return n
}

// yamlKeyOfLine reports whether the line is a "key:" line of a block mapping and returns the key.
func yamlKeyOfLine(trimmed string) (string, bool) {
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
		return "", false
	}
	m := keyLineRe.FindString(trimmed)
	if m == "" {
		return "", false
	}
	return strings.Trim(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(m), ":")), `"'`), true
}

// baselineScope returns the identity of the job (or the other top-level section) which contains the
// line: the top-level key and the key just below it, e.g. "jobs/build". It makes the same text in
// two jobs two different findings, so inserting a job does not renumber the occurrences in the
// others. It is empty when the structure is not found (flow style, documents without keys).
func baselineScope(lines [][]byte, line int) string {
	if line < 1 || line > len(lines) {
		return ""
	}
	idx := line - 1
	top := -1
	for i := idx; i >= 0; i-- {
		l := lines[i]
		if s := strings.TrimSpace(string(l)); s == "---" || s == "..." {
			break
		}
		if lineIndent(l) != 0 {
			continue
		}
		if _, ok := yamlKeyOfLine(string(l)); ok {
			top = i
			break
		}
	}
	if top < 0 {
		return ""
	}
	topKey, _ := yamlKeyOfLine(string(lines[top]))
	childIndent := -1
	child := ""
	for i := top + 1; i <= idx; i++ {
		l := lines[i]
		t := strings.TrimSpace(string(l))
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		ind := lineIndent(l)
		if ind == 0 {
			break
		}
		if childIndent < 0 {
			childIndent = ind
		}
		if ind == childIndent {
			if k, ok := yamlKeyOfLine(t); ok {
				child = k
			}
		}
	}
	if child == "" {
		return topKey
	}
	return topKey + "/" + child
}

// baselineScopes returns baselineScope of every line in one pass over the lines, so that a file with many
// findings is not scanned once for each. The result at the index i is the scope of the line i+1.
func baselineScopes(lines [][]byte) []string {
	scopes := make([]string, len(lines))
	var topKey, child string
	var inTop, broken bool
	childIndent := -1
	for i, l := range lines {
		if s := strings.TrimSpace(string(l)); s == "---" || s == "..." {
			inTop = false
			continue // the scope of the separator is empty
		}
		if lineIndent(l) == 0 {
			if k, ok := yamlKeyOfLine(string(l)); ok {
				topKey, child, inTop, broken, childIndent = k, "", true, false, -1
				scopes[i] = topKey
				continue
			}
		}
		if !inTop {
			continue
		}
		// The line after a top-level key belongs to the key just below it, found at the first indentation
		if t := strings.TrimSpace(string(l)); t != "" && !strings.HasPrefix(t, "#") && !broken {
			if ind := lineIndent(l); ind == 0 {
				broken = true // a line at the margin that is not a key ends the search
			} else {
				if childIndent < 0 {
					childIndent = ind
				}
				if ind == childIndent {
					if k, ok := yamlKeyOfLine(t); ok {
						child = k
					}
				}
			}
		}
		scopes[i] = topKey
		if child != "" {
			scopes[i] = topKey + "/" + child
		}
	}
	return scopes
}

// computeBaselineInfo sets the baseline identity of each error. The errors must be sorted by
// position. fileKey is the path relative to the repository root with forward slashes.
func computeBaselineInfo(fileKey, root string, src []byte, errs []*Error) map[*Error]*baselineInfo {
	infos := make(map[*Error]*baselineInfo, len(errs))
	lines := sourceLines(src)
	scopes := baselineScopes(lines)
	counts := map[string]int{}
	for _, e := range errs {
		text := ""
		if e.Line >= 1 && e.Line <= len(lines) {
			text = strings.Join(strings.Fields(string(lines[e.Line-1])), " ")
		}
		scope := ""
		if e.Line >= 1 && e.Line <= len(scopes) {
			scope = scopes[e.Line-1]
		}
		ctx := hashParts(e.ID, scope, text)
		fp := hashParts(e.ID, scope, text, normalizeBaselineMessage(e.Message, root))
		key := e.ID + "\x00" + fp
		infos[e] = &baselineInfo{file: fileKey, fingerprint: fp, context: ctx, occurrence: counts[key]}
		counts[key]++
	}
	return infos
}

// baselineFileKey returns the key of a file in the baseline: its path relative to root with forward
// slashes. A file outside the root keeps its path as given.
func baselineFileKey(root, cwd, path string) string {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, abs)
	}
	if root != "" {
		if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

// --- applying -------------------------------------------------------------------------------

// baselineState is a baseline loaded for one repository while linting.
type baselineState struct {
	// path is the file the baseline was read from.
	path string
	// root is the root directory entries are relative to.
	root string
	bl   *Baseline
	// byFile groups the entries by file key, in file order.
	byFile map[string][]*BaselineEntry
	// cfg is the configuration of the repository, which decides the level of unused-baseline-entry.
	cfg *Config
	// raw is the content of the file.
	raw []byte

	mu     sync.Mutex
	linted map[string]bool
	seen   map[*BaselineEntry]bool
}

func newBaselineState(path, root string, bl *Baseline) *baselineState {
	s := &baselineState{path: path, root: root, bl: bl, byFile: map[string][]*BaselineEntry{}, linted: map[string]bool{}, seen: map[*BaselineEntry]bool{}}
	for _, e := range bl.Entries {
		s.byFile[e.File] = append(s.byFile[e.File], e)
	}
	return s
}

// match marks the errors of one file which the baseline accepts as Baselined and remembers the
// entries which matched. An error matches an entry of the same rule with the same fingerprint and
// occurrence. The errors left over then match leftover entries of the same rule and context, which
// keeps a baseline working when a release rewords a message. Every entry accepts one error.
func (s *baselineState) match(fileKey string, errs []*Error, infos map[*Error]*baselineInfo) {
	entries := s.byFile[fileKey]
	if len(entries) == 0 {
		return
	}
	// The file may be linted again in the same run (the passes of -fix): what the last lint saw counts
	s.mu.Lock()
	for _, be := range entries {
		delete(s.seen, be)
	}
	s.mu.Unlock()
	used := make(map[*BaselineEntry]bool, len(entries))
	type key struct {
		rule, fp string
		occ      int
	}
	exact := make(map[key]*BaselineEntry, len(entries))
	for _, e := range entries {
		exact[key{e.Rule, e.Fingerprint, e.Occurrence}] = e
	}
	var rest []*Error
	for _, e := range errs {
		info := infos[e]
		if info == nil || e.ID == unusedBaselineEntryID {
			continue
		}
		if be, ok := exact[key{e.ID, info.fingerprint, info.occurrence}]; ok && !used[be] {
			used[be] = true
			e.Baselined = true
			continue
		}
		rest = append(rest, e)
	}
	for _, e := range rest {
		if contextFallbackExcluded[e.ID] {
			// Its message is about the action, and the context is only the line that calls it: another problem
			// of the same action is a new finding, not a reworded one
			continue
		}
		for _, be := range entries {
			if !used[be] && be.Rule == e.ID && be.Context != "" && be.Context == infos[e].context {
				used[be] = true
				e.Baselined = true
				break
			}
		}
	}
	s.mu.Lock()
	for be := range used {
		s.seen[be] = true
	}
	s.mu.Unlock()
}

// contextFallbackExcluded are the rules whose message carries what the finding is about (which advisory, which
// problem of the called action or workflow) while the context is only the line of the call: a leftover entry
// must not accept another finding of them as a reworded one.
var contextFallbackExcluded = map[string]bool{
	"invalid-local-action":     true,
	"invalid-local-workflow":   true,
	"known-vulnerable-actions": true,
}

// unusedBaselineEntryID is the ID of the diagnostic for baseline entries which match nothing.
const unusedBaselineEntryID = "unused-baseline-entry"

func init() {
	registerRules(RuleInfo{
		ID: unusedBaselineEntryID, Group: RuleGroupPolicy,
		Summary:      "A baseline entry matches no finding any more, so the baseline can shrink.",
		DefaultLevel: SeverityInfo, Profile: ProfileCorrectness,
	})
}

// splitBaselined separates the errors which the baseline accepts from the others. Both keep the order.
func splitBaselined(errs []*Error) (visible, baselined []*Error) {
	for _, e := range errs {
		if e.Baselined {
			baselined = append(baselined, e)
		} else {
			visible = append(visible, e)
		}
	}
	return visible, baselined
}

// baselineEntryFor builds the baseline entry of a finding.
func baselineEntryFor(e *Error, info *baselineInfo) *BaselineEntry {
	return &BaselineEntry{
		File:        info.file,
		Rule:        e.ID,
		Fingerprint: info.fingerprint,
		Context:     info.context,
		Occurrence:  info.occurrence,
		Message:     e.Message,
	}
}

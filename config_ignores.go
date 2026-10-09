package jactionlint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v4"
)

// ignoreExpiryWarning is how long before the "expires" date of a configuration ignore it is
// reported as about to expire.
const ignoreExpiryWarning = 14 * 24 * time.Hour

// expiresLayout is the format of the "expires" date.
const expiresLayout = "2006-01-02"

// ConfigIgnore is one entry of the "ignores" list of the configuration file. Unlike an inline ignore
// comment, it does not live next to the code, so a tool that rewrites the line (Renovate or Dependabot
// bumping `uses: actions/checkout@<sha> # v4`) cannot drop it. It matches a finding by the rule and by
// attributes of the place in the workflow where the finding is: the file, the job, the step and the
// `uses:` value of the step. All attributes which are set must match.
//
//	ignores:
//	  - rule: unpinned-uses
//	    uses: actions/checkout        # any ref
//	    file: .github/workflows/*.yml
//	    reason: the org pins this through a ruleset
//	    expires: 2027-01-31
type ConfigIgnore struct {
	// Rules are the rule IDs whose findings are ignored. At least one is required.
	Rules []string
	// File is a glob matched against the path of the file ("**" crosses directories). The path is
	// matched relative to the project root and as the linter prints it. Empty matches every file.
	File string
	// Uses matches the `uses:` value of the step (or the called workflow of a job). It is a pattern
	// as of forbidden-uses ("actions/checkout", "actions/*", "owner/repo/sub@v1", names are
	// case-insensitive and any ref matches when the pattern has none), a glob with `*` for values
	// which are not repository references ("docker://alpine*", "./.github/actions/*"), or a regular
	// expression between slashes ("/^actions\/(checkout|cache)@/") matched against the whole value.
	// Empty matches every step.
	Uses string
	// Job is the ID of the job (its key in "jobs").
	Job string
	// Step is the "id" or the "name" of the step.
	Step string
	// Reason says why the finding is accepted. It is for the readers of the config file; it is shown in
	// the messages about the entry.
	Reason string
	// Expires is the last day (YYYY-MM-DD, UTC) the entry suppresses findings. After it the findings
	// come back and the entry is reported by the expired-ignore rule. Empty never expires.
	Expires string

	// Path is the config file the entry was read from. It is set by ReadConfigFile.
	Path string
	// Line and Column are the position of the entry in the config file. They are 1-based.
	Line, Column int

	expires  time.Time
	usesPat  UsesPattern
	usesRe   *regexp.Regexp
	usesGlob bool
}

// Snippet returns the entry as YAML text for the "ignores" list of a config file, one item with the
// attributes which are set. Tools which migrate other kinds of ignore comments (for instance
// zizmor's) use it to write entries.
func (ig ConfigIgnore) Snippet() string {
	var b strings.Builder
	prefix := "- "
	put := func(k, v string) {
		if v == "" {
			return
		}
		b.WriteString(prefix + k + ": " + yamlScalar(v) + "\n")
		prefix = "  "
	}
	if len(ig.Rules) == 1 {
		put("rule", ig.Rules[0])
	} else if len(ig.Rules) > 1 {
		quoted := make([]string, len(ig.Rules))
		for i, r := range ig.Rules {
			quoted[i] = yamlScalar(r)
		}
		b.WriteString(prefix + "rule: [" + strings.Join(quoted, ", ") + "]\n")
		prefix = "  "
	}
	put("file", ig.File)
	put("uses", ig.Uses)
	put("job", ig.Job)
	put("step", ig.Step)
	put("reason", ig.Reason)
	put("expires", ig.Expires)
	return b.String()
}

// yamlScalar quotes s when it would not be read back as the same plain string.
func yamlScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return strings.TrimSuffix(string(out), "\n")
}

var configIgnoreKeys = []string{"rule", "file", "uses", "job", "step", "reason", "expires"}

// UnmarshalYAML implements yaml.Unmarshaler.
func (ig *ConfigIgnore) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("yaml: an item of \"ignores\" must be a mapping at line %d, column %d", n.Line, n.Column)
	}
	out := ConfigIgnore{Line: n.Line, Column: n.Column}
	keys, vals := mappingPairs(n)
	for i, k := range keys {
		v := vals[i]
		str := func() (string, error) {
			if v.Kind != yaml.ScalarNode || v.Tag == "!!null" {
				return "", fmt.Errorf("%q in \"ignores\" must be a string at line %d, column %d", k.Value, v.Line, v.Column)
			}
			return v.Value, nil
		}
		var err error
		switch k.Value {
		case "rule":
			switch v.Kind {
			case yaml.ScalarNode:
				out.Rules = []string{v.Value}
			case yaml.SequenceNode:
				for _, c := range v.Content {
					if c.Kind != yaml.ScalarNode {
						return fmt.Errorf("\"rule\" in \"ignores\" must be a rule ID or a list of rule IDs at line %d, column %d", c.Line, c.Column)
					}
					out.Rules = append(out.Rules, c.Value)
				}
			default:
				return fmt.Errorf("\"rule\" in \"ignores\" must be a rule ID or a list of rule IDs at line %d, column %d", v.Line, v.Column)
			}
		case "file":
			out.File, err = str()
		case "uses":
			out.Uses, err = str()
		case "job":
			out.Job, err = str()
		case "step":
			out.Step, err = str()
		case "reason":
			out.Reason, err = str()
		case "expires":
			out.Expires, err = str()
		}
		if err != nil {
			return err
		}
	}
	if err := out.validate(); err != nil {
		return fmt.Errorf("%w at line %d, column %d", err, n.Line, n.Column)
	}
	*ig = out
	return nil
}

// validate checks the entry and prepares the matchers.
func (ig *ConfigIgnore) validate() error {
	if len(ig.Rules) == 0 {
		return errors.New("\"rule\" is required in an item of \"ignores\"")
	}
	for _, r := range ig.Rules {
		if _, ok := ruleIndex[r]; !ok {
			return fmt.Errorf("unknown rule ID %q in \"ignores\"%s", r, suggestRuleID(r))
		}
	}
	if ig.File == "" && ig.Uses == "" && ig.Job == "" && ig.Step == "" {
		return fmt.Errorf("an item of \"ignores\" for %s needs at least one of \"file\", \"uses\", \"job\" and \"step\". to turn the rule off everywhere, use \"rules\"", quotes(ig.Rules))
	}
	if ig.File != "" && !doublestar.ValidatePattern(ig.File) {
		return fmt.Errorf("invalid glob pattern %q in \"file\" of \"ignores\"", ig.File)
	}
	if ig.Expires != "" {
		t, err := time.Parse(expiresLayout, ig.Expires)
		if err != nil {
			return fmt.Errorf("invalid date %q in \"expires\" of \"ignores\". write it as YYYY-MM-DD", ig.Expires)
		}
		ig.expires = t
	}
	if u := ig.Uses; u != "" {
		if len(u) >= 2 && strings.HasPrefix(u, "/") && strings.HasSuffix(u, "/") {
			re, err := regexp.Compile(u[1 : len(u)-1])
			if err != nil {
				return fmt.Errorf("invalid regular expression %q in \"uses\" of \"ignores\": %w", u, err)
			}
			ig.usesRe = re
		} else {
			ig.usesPat = ParseUsesPattern(u)
			ig.usesGlob = strings.Contains(u, "*")
		}
	}
	return nil
}

// hasRule reports whether the entry covers the rule.
func (ig *ConfigIgnore) hasRule(id string) bool {
	return slices.Contains(ig.Rules, id)
}

// expired reports whether the last day of the entry is before the day of now.
func (ig *ConfigIgnore) expired(now time.Time) bool {
	if ig.expires.IsZero() {
		return false
	}
	return utcDay(now).After(ig.expires)
}

// utcDay truncates the time to the start of its UTC day.
func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// matchUses reports whether the pattern matches the value of a `uses:` key.
func (ig *ConfigIgnore) matchUses(value string) bool {
	switch {
	case ig.Uses == "":
		return true
	case value == "":
		return false
	case ig.usesRe != nil:
		return ig.usesRe.MatchString(value)
	}
	ref := ParseUses(value)
	if ig.usesPat.Match(ref) {
		return true
	}
	if ref.Kind == UsesAction || ref.Kind == UsesReusableWorkflow {
		return false // a repository reference is judged by the pattern alone: its refs are case-sensitive
	}
	// Docker images and local paths are not repository references: compare the whole value
	if ig.usesGlob {
		return wildcardMatch(strings.ToLower(ig.Uses), strings.ToLower(value))
	}
	return strings.EqualFold(ig.Uses, value) // docker://alpine:3.20 or ./.github/actions/foo, as written
}

// someRuleRuns reports whether one of the rules of the entry could have reported anything in this run:
// an entry for a rule that is off, or for an online rule while the online checks are off, is not unused.
func (ig *ConfigIgnore) someRuleRuns(cfg *Config, online bool) bool {
	for _, id := range ig.Rules {
		if cfg.RuleRuns(id, online) {
			return true
		}
	}
	return false
}

// matchFile reports whether the file glob matches the path. rel is the path relative to the project
// root, or empty when it is unknown.
func (ig *ConfigIgnore) matchFile(path, rel string) bool {
	if ig.File == "" {
		return true
	}
	if doublestar.MatchUnvalidated(ig.File, filepath.ToSlash(path)) {
		return true
	}
	return rel != "" && doublestar.MatchUnvalidated(ig.File, filepath.ToSlash(rel))
}

// ignoreScope is where in a workflow a line is: the job, the step and the `uses:` value there. Fields
// which cannot be told are empty, and an entry which asks for an attribute that is empty never matches.
// A finding is never suppressed because of a guess.
type ignoreScope struct {
	job      string
	stepID   string
	stepName string
	uses     string
	// hasStep is true when the line is inside exactly one step.
	hasStep bool
}

// scopeIndex finds the ignoreScope of a line of a workflow.
type scopeIndex struct {
	jobs []jobRange
	// ordered is true when the ranges of the jobs do not overlap and come in the order of the lines,
	// which lets a lookup bisect them. Flow style can break that.
	ordered bool
}

type jobRange struct {
	start, end int // 1-based inclusive lines
	id         string
	uses       string
	steps      []stepRange
	ordered    bool // the ranges of the steps do not overlap and come in the order of the lines
	dead       bool // the job never runs: its "if:" is the literal false
}

type stepRange struct {
	start, end int
	id, name   string
	uses       string
	dead       bool // the step never runs: its "if:" is the literal false
}

// lastStepEnd returns the 1-based last line of the step item which starts at the 0-based line i, for the
// step that no other step follows: the lines nested under the item, not the keys of the job that come
// after "steps" ("timeout-minutes", "env"). The first line is "- key: value", or a key below a "-" alone.
func lastStepEnd(lines []string, i int) int {
	indent := func(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }
	base := indent(lines[i])
	nested := base + 1 // lines of the item are indented deeper than its dash
	if !isSequenceItem(lines[i]) {
		nested = base // the keys of the item sit at the column of the first key
	}
	end := i + 1
	for j := i + 1; j < len(lines); j++ {
		if c, b := isCommentOrBlank(lines[j]); b || c {
			continue
		}
		if indent(lines[j]) < nested {
			break
		}
		end = j + 1
	}
	return end
}

// newScopeIndex computes the line ranges of the jobs and the steps of a workflow. A range is the
// block of the job key or the step item, which is what the inline ignore comments cover too. Source
// that is not a block mapping, such as a flow-style `{...}` on one line, gives overlapping ranges,
// which scopeAt treats as unknown.
func newScopeIndex(w *Workflow, src []byte) *scopeIndex {
	if w == nil {
		return nil
	}
	lines := strings.Split(string(src), "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	idx := &scopeIndex{}
	for _, job := range w.Jobs {
		if job == nil || job.Pos == nil || job.Pos.Line < 1 || job.Pos.Line > len(lines) {
			continue
		}
		jr := jobRange{start: job.Pos.Line, end: ignoreTargetEnd(lines, job.Pos.Line-1), dead: isStaticallyFalse(job.If)}
		if job.ID != nil {
			jr.id = job.ID.Value
		}
		if job.WorkflowCall != nil && job.WorkflowCall.Uses != nil {
			jr.uses = job.WorkflowCall.Uses.Value
		}
		for i, step := range job.Steps {
			if step == nil || step.Pos == nil || step.Pos.Line < 1 || step.Pos.Line > len(lines) {
				continue
			}
			sr := stepRange{start: step.Pos.Line, end: min(jr.end, lastStepEnd(lines, step.Pos.Line-1))}
			// A step ends where the next one starts. The item of the next step may begin on the line
			// before its first key ("-" alone), which is not part of this step.
			for _, next := range job.Steps[i+1:] {
				if next != nil && next.Pos != nil && next.Pos.Line > step.Pos.Line {
					sr.end = next.Pos.Line - 1
					if sr.end > sr.start && strings.TrimSpace(lines[sr.end-1]) == "-" {
						sr.end--
					}
					break
				}
			}
			if sr.end < sr.start {
				sr.end = sr.start
			}
			sr.dead = isStaticallyFalse(step.If)
			if step.ID != nil {
				sr.id = step.ID.Value
			}
			if step.Name != nil {
				sr.name = step.Name.Value
			}
			if a, ok := step.Exec.(*ExecAction); ok && a != nil && a.Uses != nil {
				sr.uses = a.Uses.Value
			}
			jr.steps = append(jr.steps, sr)
		}
		jr.ordered = rangesOrdered(len(jr.steps), func(i int) (int, int) { return jr.steps[i].start, jr.steps[i].end })
		idx.jobs = append(idx.jobs, jr)
	}
	// The jobs come from a map, in any order
	sort.Slice(idx.jobs, func(i, j int) bool { return idx.jobs[i].start < idx.jobs[j].start })
	idx.ordered = rangesOrdered(len(idx.jobs), func(i int) (int, int) { return idx.jobs[i].start, idx.jobs[i].end })
	return idx
}

// rangesOrdered reports whether the line ranges (inclusive) come in order and do not overlap.
func rangesOrdered(n int, at func(i int) (start, end int)) bool {
	prevEnd := 0
	for i := 0; i < n; i++ {
		s, e := at(i)
		if s <= prevEnd || e < s {
			return false
		}
		prevEnd = e
	}
	return true
}

// findRange returns the indexes of the ranges which contain the line, up to two: more than one means
// that the line is ambiguous. For ordered ranges it bisects, so that the lookups for the findings of
// a file with many steps do not scan all steps each.
func findRange(n int, ordered bool, at func(i int) (start, end int), line int) []int {
	if ordered {
		i := sort.Search(n, func(i int) bool { s, _ := at(i); return s > line }) - 1
		if i >= 0 {
			if _, e := at(i); line <= e {
				return []int{i}
			}
		}
		return nil
	}
	var found []int
	for i := 0; i < n && len(found) < 2; i++ {
		if s, e := at(i); s <= line && line <= e {
			found = append(found, i)
		}
	}
	return found
}

// scopeAt returns the scope of a line.
func (idx *scopeIndex) scopeAt(line int) ignoreScope {
	var sc ignoreScope
	if idx == nil {
		return sc
	}
	jobs := findRange(len(idx.jobs), idx.ordered, func(i int) (int, int) { return idx.jobs[i].start, idx.jobs[i].end }, line)
	if len(jobs) != 1 {
		return sc // outside every job, or ambiguous
	}
	j := &idx.jobs[jobs[0]]
	sc.job = j.id
	sc.uses = j.uses
	steps := findRange(len(j.steps), j.ordered, func(i int) (int, int) { return j.steps[i].start, j.steps[i].end }, line)
	switch len(steps) {
	case 0:
	case 1:
		st := &j.steps[steps[0]]
		sc.hasStep = true
		sc.stepID, sc.stepName, sc.uses = st.id, st.name, st.uses
	default:
		// Several steps share the line (flow style): which one is unknown
		sc.uses = ""
		sc.hasStep = false
	}
	return sc
}

// matches reports whether the entry covers a finding of the rule at the scope.
func (ig *ConfigIgnore) matches(id string, sc ignoreScope, path, rel string) bool {
	if !ig.hasRule(id) || !ig.matchFile(path, rel) {
		return false
	}
	if ig.Job != "" && ig.Job != sc.job {
		return false
	}
	if ig.Step != "" && !(sc.hasStep && (ig.Step == sc.stepID || ig.Step == sc.stepName)) {
		return false
	}
	return ig.matchUses(sc.uses)
}

// ignoreContext is what finishCheck needs to match the "ignores" of the config: the project of the
// file and where its jobs and steps are. It is nil for files which are not matched.
type ignoreContext struct {
	project *Project
	scopes  *scopeIndex
}

// ignoreRun records, for one run of the linter, which configuration ignores suppressed something and
// which files were linted with each config. It is what lets the linter tell, once the run is over,
// which entries were never needed.
type ignoreRun struct {
	mu   sync.Mutex
	cfgs map[*Config]*ignoreRunConfig
}

type ignoreRunConfig struct {
	project *Project
	files   map[string]bool // absolute paths linted with this config
	used    map[*ConfigIgnore]bool
}

func (r *ignoreRun) config(cfg *Config) *ignoreRunConfig {
	if r.cfgs == nil {
		r.cfgs = map[*Config]*ignoreRunConfig{}
	}
	rc := r.cfgs[cfg]
	if rc == nil {
		rc = &ignoreRunConfig{files: map[string]bool{}, used: map[*ConfigIgnore]bool{}}
		r.cfgs[cfg] = rc
	}
	return rc
}

// track notes that a file is linted with the config, so that its ignores are reported at the end.
func (l *Linter) trackIgnoreConfig(cfg *Config, project *Project, path string) {
	if cfg == nil || len(cfg.Ignores) == 0 {
		return
	}
	l.ignoreRun.mu.Lock()
	defer l.ignoreRun.mu.Unlock()
	rc := l.ignoreRun.config(cfg)
	if project != nil {
		rc.project = project
	}
	if path != "" && path != "<stdin>" {
		rc.files[l.absFilePath(path)] = true
	}
}

// absFilePath returns the absolute form of a path given to the linter, which is relative to the
// working directory of the linter.
func (l *Linter) absFilePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return absPath(filepath.Join(l.cwd, path))
}

// configIgnoreNow returns the current time for the expiry of configuration ignores.
func (l *Linter) configIgnoreNow() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

// matchConfigIgnores returns the errors which the "ignores" of the config suppress, and marks the
// entries that did it as used. It does not remove anything so that inline ignores see every error.
func (l *Linter) matchConfigIgnores(errs []*Error, cfg *Config, project *Project, path string, scopes *scopeIndex) map[*Error]bool {
	if cfg == nil || len(cfg.Ignores) == 0 {
		return nil
	}
	now := l.configIgnoreNow()
	rel := ""
	if project != nil {
		if r, err := filepath.Rel(project.RootDir(), l.absFilePath(path)); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	var hit map[*Error]bool
	for _, err := range errs {
		sc := scopes.scopeAt(err.Line)
		for i := range cfg.Ignores {
			ig := &cfg.Ignores[i]
			if ig.expired(now) || !ig.matches(err.ID, sc, path, rel) {
				continue
			}
			if hit == nil {
				hit = map[*Error]bool{}
			}
			hit[err] = true
			l.ignoreRun.mu.Lock()
			l.ignoreRun.config(cfg).used[ig] = true
			l.ignoreRun.mu.Unlock()
			l.debug("Error %q is ignored due to the \"ignores\" entry at %s:%d", err.Message, ig.Path, ig.Line)
		}
	}
	return hit
}

// dropIgnored removes the errors in the set.
func dropIgnored(errs []*Error, hit map[*Error]bool) []*Error {
	if len(hit) == 0 {
		return errs
	}
	kept := errs[:0]
	for _, e := range errs {
		if !hit[e] {
			kept = append(kept, e)
		}
	}
	return kept
}

// finishIgnoreRun reports the problems of the configuration ignores used in the run: entries which
// expired or are about to, and entries which matched nothing. The errors are located in the config
// files and are returned as extra results. The run state is reset.
func (l *Linter) finishIgnoreRun(results []fileResult) []fileResult {
	l.ignoreRun.mu.Lock()
	cfgs := l.ignoreRun.cfgs
	l.ignoreRun.cfgs = nil
	l.ignoreRun.mu.Unlock()
	if len(cfgs) == 0 {
		return results
	}

	now := l.configIgnoreNow()
	today := utcDay(now)
	byPath := map[string][]*Error{}
	var paths []string
	add := func(path string, e *Error) {
		if _, ok := byPath[path]; !ok {
			paths = append(paths, path)
		}
		byPath[path] = append(byPath[path], e)
	}

	for cfg, rc := range cfgs {
		covered := rc.coveredFiles(l)
		for i := range cfg.Ignores {
			ig := &cfg.Ignores[i]
			var e *Error
			upcoming := false
			switch {
			case ig.expired(now):
				e = ig.errorAt("expired-ignore", fmt.Sprintf("the ignore for %s expired on %s and no longer suppresses anything. fix the findings, or renew it with a new date in \"expires\"%s", describeIgnore(ig), ig.Expires, ignoreReason(ig)))
			case !ig.expires.IsZero() && ig.expires.Sub(today) <= ignoreExpiryWarning:
				days := int(ig.expires.Sub(today) / (24 * time.Hour))
				e = ig.errorAt("expired-ignore", fmt.Sprintf("the ignore for %s expires on %s (in %d days)%s", describeIgnore(ig), ig.Expires, days, ignoreReason(ig)))
				upcoming = true
			case !rc.used[ig] && ig.someRuleRuns(cfg, l.online.enabled || (!l.online.off && cfg.Online)) && rc.judgeUnused(ig, covered):
				e = ig.errorAt("unused-ignore", fmt.Sprintf("the ignore for %s did not suppress any finding. remove it from \"ignores\"%s", describeIgnore(ig), ignoreReason(ig)))
			default:
				continue
			}
			level := cfg.RuleLevel(e.ID)
			if level == SeverityOff {
				continue
			}
			e.Severity = level
			if upcoming {
				e.Severity = min(level, SeverityInfo)
			}
			add(ig.Path, e)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		errs := byPath[p]
		slices.SortFunc(errs, compareErrors)
		var src []byte
		display := p
		if p != "" {
			src, _ = os.ReadFile(p)
			if l.cwd != "" {
				if r, err := filepath.Rel(l.cwd, l.absFilePath(p)); err == nil {
					display = r
				}
			}
		}
		lines := sourceLines(src)
		for _, e := range errs {
			e.Filepath = display
			e.fillRegion(lines)
		}
		if l.minSeverity > SeverityInfo {
			kept := errs[:0]
			for _, e := range errs {
				if e.Severity >= l.minSeverity {
					kept = append(kept, e)
				}
			}
			errs = kept
		}
		if len(errs) > 0 {
			results = append(results, fileResult{file: p, path: display, src: src, errs: errs})
		}
	}
	return results
}

// errorAt makes an error located at the entry in the config file. id is "expired-ignore" or
// "unused-ignore".
func (ig *ConfigIgnore) errorAt(id, msg string) *Error {
	var e *Error
	if id == "unused-ignore" {
		e = &Error{ID: "unused-ignore", Message: msg}
	} else {
		e = &Error{ID: "expired-ignore", Message: msg}
	}
	e.Line, e.Column, e.Kind, e.Filepath = max(ig.Line, 1), max(ig.Column, 1), "ignore", ig.Path
	e.DocURL = ruleIndex[id].DocURL()
	return e
}

func describeIgnore(ig *ConfigIgnore) string {
	var attrs []string
	if ig.File != "" {
		attrs = append(attrs, "file "+fmt.Sprintf("%q", ig.File))
	}
	if ig.Job != "" {
		attrs = append(attrs, "job "+fmt.Sprintf("%q", ig.Job))
	}
	if ig.Step != "" {
		attrs = append(attrs, "step "+fmt.Sprintf("%q", ig.Step))
	}
	if ig.Uses != "" {
		attrs = append(attrs, "uses "+fmt.Sprintf("%q", ig.Uses))
	}
	return fmt.Sprintf("%s (%s)", quotes(ig.Rules), strings.Join(attrs, ", "))
}

func ignoreReason(ig *ConfigIgnore) string {
	if ig.Reason == "" {
		return ""
	}
	return fmt.Sprintf(" (reason: %s)", ig.Reason)
}

// coveredFiles returns the absolute paths which a complete run would lint with the config's project.
func (rc *ignoreRunConfig) coveredFiles(l *Linter) map[string]bool {
	if rc.project == nil {
		return nil
	}
	all := map[string]bool{}
	files, err := projectWorkflowFiles(rc.project.WorkflowsDir())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil // a repository with only actions has no workflows directory
	}
	for _, f := range files {
		all[filepath.Clean(absPath(f))] = true
	}
	for _, f := range rc.project.DependabotFiles() {
		all[filepath.Clean(absPath(f))] = true
	}
	for _, f := range l.callGraphOf(rc.project).actionPaths() {
		all[filepath.Clean(absPath(f))] = true
	}
	return all
}

// judgeUnused tells whether an entry that matched nothing can be called unused. When only some files
// were linted (a pre-commit hook passes the changed files), the entry may be needed by a file which
// was not looked at, so it is judged only if every file it could apply to was linted.
func (rc *ignoreRunConfig) judgeUnused(ig *ConfigIgnore, covered map[string]bool) bool {
	if covered == nil {
		return false
	}
	root := ""
	if rc.project != nil {
		root = rc.project.RootDir()
	}
	candidates := map[string]bool{}
	for f := range covered {
		candidates[f] = true
	}
	for f := range rc.files {
		candidates[f] = true
	}
	for f := range candidates {
		rel := ""
		if root != "" {
			if r, err := filepath.Rel(root, f); err == nil {
				rel = r
			}
		}
		if ig.matchFile(f, rel) && !rc.files[f] {
			return false
		}
	}
	return true
}

func init() {
	registerRules(
		RuleInfo{ID: "expired-ignore", Group: RuleGroupPolicy, Summary: "An entry of \"ignores\" in the config file has expired or is about to.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-unused-ignore"},
	)
}

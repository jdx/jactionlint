package jactionlint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"go.yaml.in/yaml/v4"
)

// This file answers one question for each check that the configuration declares as required: can a pull request (or
// a merge queue group, or a push) in the declared scope end with no job that reports the check? GitHub then waits for
// the check forever.
//
// It is a reachability question on a small graph. A scenario (an event, the activity of a pull request, a base branch
// and a changed file) passes the trigger filters of a workflow or not; a workflow runs its jobs; a job that calls a local
// reusable workflow runs the jobs of that workflow under the name "caller / callee". A check is reported when some path
// from the scenario ends at a job with the declared name. Each edge is decided as yes, no or unknown, and a scenario
// is reported only when every edge from it is a proven no.
//
// The checks GitHub creates follow these rules (GitHub's documentation of required status checks and of matrix jobs):
//
//   - the check of a job is named by its `name:`, or its ID when it has none. A matrix job adds the values of its
//     combination in parentheses, unless its name uses the matrix context. A job of a reusable workflow is
//     "caller / callee". The values are compared as a set, because GitHub does not document their order.
//   - a job skipped by its `if:`, or because a job it needs failed, still reports a check, which counts as passing.
//     So a condition on a job never makes a required check pending, and this file does not judge conditions. A matrix job
//     or a reusable workflow call that is skipped reports under its unexpanded name: only one that can never run is
//     reported, using the same static-false and status-function analysis as the other rules.
//   - a workflow that its trigger filters (`types`, `branches`, `branches-ignore`, `paths` and `paths-ignore`) do not start creates no
//     check at all: the required check stays pending.
//
// The analysis says "unknown" rather than guess: a name or a filter with an expression, a matrix that is not literal, a
// reusable workflow of another repository, a filter with a `!` entry (whose result depends on the order of the entries)
// and a branch filter when the declaration names no branches.

// RequiredCheck is an item of the "required-checks" list of the config file: a check that branch protection or a
// ruleset requires, and the events it is required for.
type RequiredCheck struct {
	// Name is the name of the check as GitHub shows it. The comparison is exact.
	Name string
	// Events are the events the check is required for. Empty means pull_request.
	Events []string
	// Branches are the literal names of the base branches the requirement protects. Empty means that they are not
	// known, and the branch filters of the workflows are not judged.
	Branches []string
	// Path is the config file the item was read from. It is set by ReadConfigFile.
	Path string
	// Line and Column are the position of the name in the config file. They are 1-based.
	Line, Column int
}

// requiredCheckEvents are the events a required check can be declared for: the events which run the workflows that
// report the checks of a pull request, a merge queue group or a pushed commit.
var requiredCheckEvents = []string{"pull_request", "pull_request_target", "merge_group", "push"}

// UnmarshalYAML implements yaml.Unmarshaler.
func (rc *RequiredCheck) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("an item of \"required-checks\" must be a mapping at line %d, column %d", n.Line, n.Column)
	}
	out := RequiredCheck{Line: n.Line, Column: n.Column}
	strs := func(k, v *yaml.Node) ([]string, error) {
		if v.Kind != yaml.SequenceNode || len(v.Content) == 0 {
			return nil, fmt.Errorf("%q in \"required-checks\" must be a non-empty list of strings at line %d, column %d", k.Value, v.Line, v.Column)
		}
		var ret []string
		for _, c := range v.Content {
			if c.Kind != yaml.ScalarNode || c.Tag == "!!null" || c.Value == "" {
				return nil, fmt.Errorf("%q in \"required-checks\" must be a non-empty list of strings at line %d, column %d", k.Value, c.Line, c.Column)
			}
			ret = append(ret, c.Value)
		}
		return ret, nil
	}
	keys, vals := mappingPairs(n)
	for i, k := range keys {
		v := vals[i]
		var err error
		switch k.Value {
		case "name":
			if v.Kind != yaml.ScalarNode || v.Tag == "!!null" || strings.TrimSpace(v.Value) == "" {
				return fmt.Errorf("\"name\" in \"required-checks\" must be a non-empty string at line %d, column %d", v.Line, v.Column)
			}
			out.Name, out.Line, out.Column = v.Value, v.Line, v.Column
		case "events":
			out.Events, err = strs(k, v)
			for _, e := range out.Events {
				if err == nil && !slices.Contains(requiredCheckEvents, e) {
					err = fmt.Errorf("unknown event %q in \"required-checks\" at line %d, column %d. available events are %s", e, v.Line, v.Column, sortedQuotes(requiredCheckEvents))
				}
			}
		case "branches":
			out.Branches, err = strs(k, v)
			for _, b := range out.Branches {
				if err == nil && (strings.ContainsAny(b, "*?[]+!") || ContainsExpression(b)) {
					err = fmt.Errorf("branch %q in \"required-checks\" must be a literal branch name, not a pattern, at line %d, column %d", b, v.Line, v.Column)
				}
			}
		}
		if err != nil {
			return err
		}
	}
	if out.Name == "" {
		return fmt.Errorf("\"name\" is required in an item of \"required-checks\" at line %d, column %d", n.Line, n.Column)
	}
	*rc = out
	return nil
}

// events returns the events the check is required for.
func (rc *RequiredCheck) events() []string {
	if len(rc.Events) == 0 {
		return []string{"pull_request"}
	}
	return rc.Events
}

// --- the workflows of a repository ---------------------------------------------------------

// checkWorkflows are the workflows of a repository by their path relative to the root, with slashes.
type checkWorkflows map[string]*Workflow

// readCheckWorkflows parses the workflow files of the repository at root.
func readCheckWorkflows(root string) checkWorkflows {
	ws := checkWorkflows{}
	files, _ := projectWorkflowFiles(filepath.Join(root, ".github", "workflows")) // no directory, no workflows
	for _, f := range files {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if w, _ := Parse(src); w != nil {
			ws[filepath.ToSlash(rel)] = w
		}
	}
	return ws
}

// with returns the workflows with the one at rel replaced by w: the file being linted, whose content may differ from
// the disk.
func (ws checkWorkflows) with(rel string, w *Workflow) checkWorkflows {
	ret := make(checkWorkflows, len(ws)+1)
	for k, v := range ws {
		ret[k] = v
	}
	ret[rel] = w
	return ret
}

func (ws checkWorkflows) sortedPaths() []string {
	ps := make([]string, 0, len(ws))
	for p := range ws {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}

type lazyCheckWorkflows struct {
	once sync.Once
	ws   checkWorkflows
}

// --- three-state answers ---------------------------------------------------------------

type reach int

const (
	reachNo reach = iota
	reachUnknown
	reachYes
)

// --- names of checks -----------------------------------------------------------------------

// checkJob is a job on the way from a workflow that an event starts to the job that reports a check.
type checkJob struct {
	file string
	w    *Workflow
	job  *Job
	// expands is whether the check exists only when the job runs: a matrix job adds its values to the name, a caller
	// of a reusable workflow adds the names of the called jobs. Skipped, it reports its bare name.
	expands bool
}

// nameCandidate is a set of check names that one path of jobs can report.
type nameCandidate struct {
	chain []checkJob
	// match tells whether the candidate reports the name.
	match func(name string) reach
	// why says why the match can be unknown.
	why string
}

func exactName(n string) func(string) reach {
	return func(s string) reach {
		if s == n {
			return reachYes
		}
		return reachNo
	}
}

func jobLabel(file string, job *Job) string {
	return fmt.Sprintf("job %q of %s", job.ID.Value, file)
}

// reMatrixRef is a name made of text and `${{ matrix.KEY }}` only.
var reMatrixRef = regexp.MustCompile(`\$\{\{\s*matrix\.([A-Za-z_][A-Za-z0-9_-]*)\s*\}\}`)

// literalMatrix returns the combinations of a matrix whose rows are lists of scalars, with the keys in lower case, or
// false when the matrix is not literal (an expression, `include` or `exclude`, an object value) or too large.
func literalMatrix(m *Matrix) ([]map[string]string, bool) {
	if m == nil || m.Expression != nil || m.Include != nil || m.Exclude != nil {
		return nil, false
	}
	combos := []map[string]string{{}}
	keys := make([]string, 0, len(m.Rows))
	for k := range m.Rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		row := m.Rows[k]
		if row == nil || row.Expression != nil || len(row.Values) == 0 {
			return nil, false
		}
		var next []map[string]string
		for _, c := range combos {
			for _, v := range row.Values {
				s, ok := v.(*RawYAMLString)
				if !ok || ContainsExpression(s.Value) || strings.Contains(s.Value, ", ") || strings.ContainsAny(s.Value, "()") {
					return nil, false
				}
				n := make(map[string]string, len(c)+1)
				for ck, cv := range c {
					n[ck] = cv
				}
				n[k] = s.Value
				next = append(next, n)
			}
		}
		if len(next) > 256 {
			return nil, false
		}
		combos = next
	}
	return combos, true
}

func valueSetKey(vs []string) string {
	vs = slices.Clone(vs)
	sort.Strings(vs)
	return strings.Join(vs, "\x00")
}

// templateRegexp matches the strings that a name with `${{ }}` can evaluate to: its literal text, anything in place of
// an expression, and an optional matrix suffix when the job has a matrix.
func templateRegexp(name string, matrix bool) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	rest := name
	for {
		i := strings.Index(rest, "${{")
		if i < 0 {
			b.WriteString(regexp.QuoteMeta(rest))
			break
		}
		b.WriteString(regexp.QuoteMeta(rest[:i]))
		b.WriteString("(?s:.*)")
		j := strings.Index(rest[i:], "}}")
		if j < 0 {
			break
		}
		rest = rest[i+j+2:]
	}
	if matrix {
		b.WriteString(`(?: \(.*\))?`)
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// ownNames returns the candidate of the names that the job itself reports, before a reusable workflow it calls adds
// the names of its jobs.
func ownNames(file string, w *Workflow, job *Job) nameCandidate {
	cj := checkJob{file: file, w: w, job: job}
	name := job.ID.Value
	if job.Name != nil {
		name = job.Name.Value
	}
	var m *Matrix
	hasMatrix := false
	if job.Strategy != nil {
		m = job.Strategy.Matrix
		hasMatrix = job.Strategy.Expression != nil || m != nil
	}
	if !hasMatrix {
		if ContainsExpression(name) {
			re := templateRegexp(name, false)
			return nameCandidate{chain: []checkJob{cj}, why: jobLabel(file, job) + " has a name computed at runtime", match: func(s string) reach {
				if re.MatchString(s) {
					return reachUnknown
				}
				return reachNo
			}}
		}
		return nameCandidate{chain: []checkJob{cj}, match: exactName(name)}
	}
	cj.expands = true
	combos, literal := literalMatrix(m)
	if job.Strategy.Expression != nil {
		literal = false
	}
	if ContainsExpression(name) {
		// A name that is text and matrix values only is computed for each combination; any other expression is not
		if literal && !ContainsExpression(reMatrixRef.ReplaceAllString(name, "")) {
			names := map[string]bool{}
			for _, c := range combos {
				ok := true
				n := reMatrixRef.ReplaceAllStringFunc(name, func(ref string) string {
					v, found := c[strings.ToLower(reMatrixRef.FindStringSubmatch(ref)[1])]
					ok = ok && found
					return v
				})
				if ok {
					names[n] = true
				}
			}
			if len(names) == len(combos) {
				return nameCandidate{chain: []checkJob{cj}, match: func(s string) reach {
					if names[s] {
						return reachYes
					}
					return reachNo
				}}
			}
		}
		re := templateRegexp(name, true)
		return nameCandidate{chain: []checkJob{cj}, why: jobLabel(file, job) + " has a name computed at runtime", match: func(s string) reach {
			if re.MatchString(s) {
				return reachUnknown
			}
			return reachNo
		}}
	}
	prefix := name + " ("
	if !literal {
		return nameCandidate{chain: []checkJob{cj}, why: jobLabel(file, job) + " has a matrix that is not a literal list of values", match: func(s string) reach {
			if strings.HasPrefix(s, prefix) && strings.HasSuffix(s, ")") {
				return reachUnknown
			}
			return reachNo
		}}
	}
	sets := map[string]bool{}
	for _, c := range combos {
		vs := make([]string, 0, len(c))
		for _, v := range c {
			vs = append(vs, v)
		}
		sets[valueSetKey(vs)] = true
	}
	return nameCandidate{chain: []checkJob{cj}, match: func(s string) reach {
		if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, ")") {
			return reachNo
		}
		if sets[valueSetKey(strings.Split(s[len(prefix):len(s)-1], ", "))] {
			return reachYes
		}
		return reachNo
	}}
}

// callNames combines the names of a caller with the names of the jobs of the workflow it calls: "caller / callee".
func callNames(caller nameCandidate, callee nameCandidate) nameCandidate {
	chain := append(slices.Clone(caller.chain), callee.chain...)
	chain[len(caller.chain)-1].expands = true
	why := callee.why
	if why == "" {
		why = caller.why
	}
	return nameCandidate{chain: chain, why: why, match: func(s string) reach {
		best := reachNo
		for i := 0; i+3 <= len(s); i++ {
			if s[i:i+3] != " / " {
				continue
			}
			l, r := caller.match(s[:i]), callee.match(s[i+3:])
			if l == reachNo || r == reachNo {
				continue
			}
			if l == reachYes && r == reachYes {
				return reachYes
			}
			best = reachUnknown
		}
		return best
	}}
}

// maxReusableNesting is how deep reusable workflows can be nested on GitHub.
const maxReusableNesting = 10

// jobCandidates returns the names that the job reports, following the local reusable workflow it calls.
func jobCandidates(ws checkWorkflows, file string, w *Workflow, job *Job, depth int) []nameCandidate {
	own := ownNames(file, w, job)
	if job.WorkflowCall == nil || job.WorkflowCall.Uses == nil {
		return []nameCandidate{own}
	}
	uses := job.WorkflowCall.Uses.Value
	if depth >= maxReusableNesting || ContainsExpression(uses) {
		return []nameCandidate{callNames(own, nameCandidate{why: jobLabel(file, job) + " calls a reusable workflow that is not known", match: func(string) reach { return reachUnknown }})}
	}
	target, ok := localTarget(uses)
	if !ok || target.kind != 'w' {
		return []nameCandidate{callNames(own, nameCandidate{why: fmt.Sprintf("%s calls the reusable workflow %q of another repository", jobLabel(file, job), uses), match: func(string) reach { return reachUnknown }})}
	}
	callee, ok := ws[target.path]
	if !ok {
		return []nameCandidate{callNames(own, nameCandidate{why: fmt.Sprintf("%s calls %q, which is not a workflow of the repository", jobLabel(file, job), uses), match: func(string) reach { return reachUnknown }})}
	}
	var ret []nameCandidate
	for _, id := range jobIDsInOrder(callee) {
		for _, c := range jobCandidates(ws, target.path, callee, callee.Jobs[id], depth+1) {
			ret = append(ret, callNames(own, c))
		}
	}
	return ret
}

// onlyCalled reports whether the workflow runs only when another workflow calls it: its jobs report no check under
// their own names.
func onlyCalled(w *Workflow) bool {
	if len(w.On) == 0 {
		return false
	}
	for _, e := range w.On {
		if _, ok := e.(*WorkflowCallEvent); !ok {
			return false
		}
	}
	return true
}

// checkSource is a path from a workflow that an event starts to a job which reports the check.
type checkSource struct {
	root  string
	w     *Workflow
	chain []checkJob
}

// checkResolution is what the workflows say about a check name.
type checkResolution struct {
	// sources report the name for sure when their workflow runs.
	sources []checkSource
	// maybe are the reasons why other jobs may report it.
	maybe []string
}

func resolveCheck(ws checkWorkflows, name string) *checkResolution {
	res := &checkResolution{}
	for _, p := range ws.sortedPaths() {
		w := ws[p]
		if onlyCalled(w) {
			continue // its jobs report checks under the names of their callers
		}
		for _, id := range jobIDsInOrder(w) {
			for _, c := range jobCandidates(ws, p, w, w.Jobs[id], 0) {
				switch c.match(name) {
				case reachYes:
					res.sources = append(res.sources, checkSource{root: p, w: w, chain: c.chain})
				case reachUnknown:
					res.maybe = append(res.maybe, c.why)
				}
			}
		}
	}
	return res
}

// suggestChecks returns the names that the workflows report and that look like the declared one: the same text in
// another case, or the name of a job of a reusable workflow without its caller.
func suggestChecks(ws checkWorkflows, name string) []string {
	seen := map[string]bool{}
	var ret []string
	add := func(n string) {
		if !seen[n] && n != name {
			seen[n] = true
			ret = append(ret, n)
		}
	}
	for _, p := range ws.sortedPaths() {
		w := ws[p]
		if onlyCalled(w) {
			continue
		}
		for _, id := range jobIDsInOrder(w) {
			j := w.Jobs[id]
			own := id
			if j.Name != nil {
				own = j.Name.Value
			}
			if ContainsExpression(own) {
				continue
			}
			if strings.EqualFold(own, name) || strings.EqualFold(id, name) {
				add(own)
			}
			if j.WorkflowCall == nil || j.WorkflowCall.Uses == nil {
				continue
			}
			if t, ok := localTarget(j.WorkflowCall.Uses.Value); ok && t.kind == 'w' {
				if callee, ok := ws[t.path]; ok {
					for _, cid := range jobIDsInOrder(callee) {
						cn := cid
						if callee.Jobs[cid].Name != nil {
							cn = callee.Jobs[cid].Name.Value
						}
						if strings.EqualFold(cn, name) || strings.EqualFold(cid, name) {
							add(own + " / " + cn)
						}
					}
				}
			}
		}
	}
	return ret
}

// --- jobs which never run ------------------------------------------------------------------

// neverRuns reports whether the job can never run: its `if:` is the literal false, or it needs a job which never runs
// and has no status check function that would run it anyway (the analysis of gate-job-skipped-on-failure). It
// returns the job whose condition is the cause.
func neverRuns(w *Workflow, job *Job, seen map[*Job]bool) *Job {
	if seen[job] {
		return nil
	}
	seen[job] = true
	if isStaticallyFalse(job.If) {
		return job
	}
	cond, ok := conditionExprs(job.If)
	if !ok || callsStatusFunction(cond, false) {
		return nil
	}
	for _, n := range job.Needs {
		if dep, ok := w.Jobs[strings.ToLower(n.Value)]; ok && dep != nil {
			if cause := neverRuns(w, dep, seen); cause != nil {
				return cause
			}
		}
	}
	return nil
}

// deadJob returns the first job of the source that expands the name and never runs, so the name is never reported.
func (s *checkSource) deadJob() (checkJob, *Job) {
	for _, cj := range s.chain {
		if !cj.expands {
			continue
		}
		if cause := neverRuns(cj.w, cj.job, map[*Job]bool{}); cause != nil {
			return cj, cause
		}
	}
	return checkJob{}, nil
}

// --- trigger filters -----------------------------------------------------------------------

// checkScenario is a change that a required check is required for.
type checkScenario struct {
	event    string
	activity string // "opened" or "synchronize" for a pull request, else empty
	branch   string // the base branch, empty when it is not declared
	path     string // the only changed file
}

// blame is why a workflow does not start for a scenario.
type blame struct {
	file string
	pos  *Pos
	kind string // "event", "types", "branches", "branches-ignore", "tags", "paths", "paths-ignore", "job"
	what string
	fix  string
}

// filterMatch matches a value against a pattern of a trigger filter. It is yes or no only when the pattern is
// understood and the answer does not depend on the case of the letters (GitHub does not document it).
func filterMatch(pat, value string, isPath bool) reach {
	if pat == "" || strings.HasPrefix(pat, "!") || ContainsExpression(pat) || strings.HasSuffix(pat, "/") ||
		strings.HasPrefix(pat, "/") || strings.HasPrefix(pat, "./") || (!isPath && strings.HasPrefix(pat, "refs/")) {
		return reachUnknown
	}
	if isPath && len(ValidatePathGlob(pat)) > 0 || !isPath && len(ValidateRefGlob(pat)) > 0 {
		return reachUnknown
	}
	src, _, _, ok := filterGlobRegexp(pat)
	if !ok {
		return reachUnknown
	}
	exact, err1 := regexp.Compile(src)
	folded, err2 := regexp.Compile("(?i)" + src)
	if err1 != nil || err2 != nil {
		return reachUnknown
	}
	switch a, b := exact.MatchString(value), folded.MatchString(value); {
	case a && b:
		return reachYes
	case !a && !b:
		return reachNo
	}
	return reachUnknown
}

// filterIncludes tells whether a list of patterns without negations includes the value, and returns the entry that
// matches it.
func filterIncludes(f *WebhookEventFilter, value string, isPath bool) (reach, *String) {
	ans := reachNo
	for _, v := range f.Values {
		if strings.HasPrefix(v.Value, "!") {
			return reachUnknown, nil // the result depends on the order of the entries
		}
	}
	for _, v := range f.Values {
		switch filterMatch(v.Value, value, isPath) {
		case reachYes:
			return reachYes, v
		case reachUnknown:
			ans = reachUnknown
		}
	}
	return ans, nil
}

func eventHook(w *Workflow, event string) *WebhookEvent {
	for _, e := range w.On {
		if h, ok := e.(*WebhookEvent); ok && h.Hook != nil && h.Hook.Value == event {
			return h
		}
	}
	return nil
}

// onPos is the position of the first event of the workflow, where "on:" is.
func onPos(w *Workflow) *Pos {
	var pos *Pos
	for _, e := range w.On {
		var p *Pos
		switch e := e.(type) {
		case *WebhookEvent:
			p = e.Pos
		case *ScheduledEvent:
			p = e.Pos
		case *WorkflowDispatchEvent:
			p = e.Pos
		case *RepositoryDispatchEvent:
			p = e.Pos
		case *WorkflowCallEvent:
			p = e.Pos
		case *ImageVersionEvent:
			p = e.Pos
		}
		if p != nil && (pos == nil || p.IsBefore(pos)) {
			pos = p
		}
	}
	if pos == nil {
		return &Pos{Line: 1, Col: 1}
	}
	return pos
}

func filterPos(f *WebhookEventFilter, h *WebhookEvent) *Pos {
	if f != nil && f.Name != nil && f.Name.Pos != nil {
		return f.Name.Pos
	}
	return h.Pos
}

// starts tells whether the trigger filters of the workflow start it for the scenario. It returns why not, or why it
// cannot tell.
func starts(file string, w *Workflow, sc checkScenario) (reach, *blame, string) {
	h := eventHook(w, sc.event)
	if h == nil {
		return reachNo, &blame{file: file, pos: onPos(w), kind: "event", what: fmt.Sprintf("the workflow does not run on %q", sc.event), fix: fmt.Sprintf("add %q to \"on:\", or remove it from the \"events\" of the declaration", sc.event)}, ""
	}
	if sc.activity != "" && len(h.Types) > 0 {
		found := false
		for _, t := range h.Types {
			if ContainsExpression(t.Value) {
				return reachUnknown, nil, fmt.Sprintf("the \"types\" of %q in %s have an expression", sc.event, file)
			}
			found = found || t.Value == sc.activity
		}
		if !found {
			return reachNo, &blame{file: file, pos: h.Types[0].Pos, kind: "types", what: fmt.Sprintf("the \"types\" of %q do not include %q", sc.event, sc.activity), fix: fmt.Sprintf("add %q to \"types\"", sc.activity)}, ""
		}
	}
	unknown := ""
	switch sc.event {
	case "pull_request", "pull_request_target", "merge_group":
		if !h.Branches.IsEmpty() {
			if sc.branch == "" {
				unknown = fmt.Sprintf("%s filters %q by branch and the declaration names no \"branches\"", file, sc.event)
			} else {
				switch t, _ := filterIncludes(h.Branches, sc.branch, false); t {
				case reachNo:
					return reachNo, &blame{file: file, pos: filterPos(h.Branches, h), kind: "branches", what: fmt.Sprintf("no entry of \"branches\" of %q matches the branch %q", sc.event, sc.branch), fix: fmt.Sprintf("add %q to \"branches\"", sc.branch)}, ""
				case reachUnknown:
					unknown = fmt.Sprintf("jactionlint cannot evaluate the \"branches\" of %q in %s", sc.event, file)
				}
			}
		}
		if !h.BranchesIgnore.IsEmpty() {
			if sc.branch == "" {
				unknown = fmt.Sprintf("%s filters %q by branch and the declaration names no \"branches\"", file, sc.event)
			} else {
				switch t, v := filterIncludes(h.BranchesIgnore, sc.branch, false); t {
				case reachYes:
					return reachNo, &blame{file: file, pos: v.Pos, kind: "branches-ignore", what: fmt.Sprintf("the entry %q of \"branches-ignore\" of %q matches the branch %q", v.Value, sc.event, sc.branch), fix: "remove the entry"}, ""
				case reachUnknown:
					unknown = fmt.Sprintf("jactionlint cannot evaluate the \"branches-ignore\" of %q in %s", sc.event, file)
				}
			}
		}
	case "push":
		if h.Branches.IsEmpty() && h.BranchesIgnore.IsEmpty() && (!h.Tags.IsEmpty() || !h.TagsIgnore.IsEmpty()) {
			f := h.Tags
			if f.IsEmpty() {
				f = h.TagsIgnore
			}
			return reachNo, &blame{file: file, pos: filterPos(f, h), kind: "tags", what: "\"push\" has only tag filters, so it runs for tags and never for a pushed branch", fix: "add \"branches\" to \"push\""}, ""
		}
		if !h.Branches.IsEmpty() || !h.BranchesIgnore.IsEmpty() {
			unknown = fmt.Sprintf("%s filters \"push\" by the pushed branch, which the declaration does not name", file)
		}
	}
	switch sc.event {
	case "pull_request", "pull_request_target", "push":
		if !h.Paths.IsEmpty() {
			switch t, _ := filterIncludes(h.Paths, sc.path, true); t {
			case reachNo:
				return reachNo, &blame{file: file, pos: filterPos(h.Paths, h), kind: "paths", what: fmt.Sprintf("no entry of \"paths\" of %q matches %q", sc.event, sc.path), fix: "remove the \"paths\" filter and skip the work inside the job instead (a skipped job reports success), or add a workflow with the same job name and the opposite filter"}, ""
			case reachUnknown:
				unknown = fmt.Sprintf("jactionlint cannot evaluate the \"paths\" of %q in %s for %q", sc.event, file, sc.path)
			}
		}
		if !h.PathsIgnore.IsEmpty() {
			switch t, v := filterIncludes(h.PathsIgnore, sc.path, true); t {
			case reachYes:
				return reachNo, &blame{file: file, pos: v.Pos, kind: "paths-ignore", what: fmt.Sprintf("the entry %q of \"paths-ignore\" of %q matches %q", v.Value, sc.event, sc.path), fix: "remove the \"paths-ignore\" filter and skip the work inside the job instead (a skipped job reports success), or add a workflow with the same job name and the opposite filter"}, ""
			case reachUnknown:
				unknown = fmt.Sprintf("jactionlint cannot evaluate the \"paths-ignore\" of %q in %s for %q", sc.event, file, sc.path)
			}
		}
	}
	if unknown != "" {
		return reachUnknown, nil, unknown
	}
	return reachYes, nil, ""
}

// witnessPath returns a path that the pattern matches, or "".
func witnessPath(pat string) string {
	if strings.HasPrefix(pat, "!") {
		return ""
	}
	var b strings.Builder
	rs := []rune(pat)
	for i := 0; i < len(rs); i++ {
		switch c := rs[i]; c {
		case '*':
			if i+1 < len(rs) && rs[i+1] == '*' {
				i++
				if i+1 < len(rs) && rs[i+1] == '/' {
					i++ // `**/` can match nothing
					continue
				}
			}
			b.WriteString("x")
		case '?', '+':
		case '[':
			end := slices.Index(rs[i:], ']')
			if end < 2 {
				return ""
			}
			b.WriteRune(rs[i+1])
			i += end
		case '\\':
			if i+1 < len(rs) {
				i++
				b.WriteRune(rs[i])
			}
		default:
			b.WriteRune(c)
		}
	}
	p := b.String()
	if p == "" || strings.HasSuffix(p, "/") || filterMatch(pat, p, true) != reachYes {
		return ""
	}
	return p
}

// genericChangedPaths are the files a scenario may change besides the ones that the filters name.
var genericChangedPaths = []string{"README.md", "docs/index.md", "src/main.go"}

// scenarios returns the changes to try for the declaration: each event, activity and branch, and each changed file
// that the path filters of the workflows that report the check make interesting.
func scenarios(rc *RequiredCheck, sources []checkSource) []checkScenario {
	var ret []checkScenario
	for _, ev := range rc.events() {
		paths := slices.Clone(genericChangedPaths)
		for _, s := range sources {
			if h := eventHook(s.w, ev); h != nil {
				for _, f := range []*WebhookEventFilter{h.Paths, h.PathsIgnore} {
					if f == nil {
						continue
					}
					for _, v := range f.Values {
						if p := witnessPath(v.Value); p != "" && !slices.Contains(paths, p) {
							paths = append(paths, p)
						}
					}
				}
			}
		}
		activities := []string{""}
		if ev == "pull_request" || ev == "pull_request_target" {
			activities = []string{"opened", "synchronize"}
		}
		branches := rc.Branches
		if len(branches) == 0 {
			branches = []string{""}
		}
		for _, a := range activities {
			for _, b := range branches {
				for _, p := range paths {
					ret = append(ret, checkScenario{event: ev, activity: a, branch: b, path: p})
				}
			}
		}
	}
	return ret
}

// describe returns the change of the scenario in words, with the parts which the blame is about.
func (sc checkScenario) describe(kind string) string {
	var s string
	switch sc.event {
	case "pull_request", "pull_request_target":
		s = "a pull request"
		if kind == "types" && sc.activity == "synchronize" {
			s = "a commit pushed to a pull request"
		} else if kind == "types" {
			s = "a pull request that is " + sc.activity
		}
		if sc.event == "pull_request_target" {
			s += " (pull_request_target)"
		}
	case "merge_group":
		s = "a merge queue group"
	default:
		s = "a push"
	}
	if sc.branch != "" && (kind == "branches" || kind == "branches-ignore") {
		s += fmt.Sprintf(" into %q", sc.branch)
	}
	if kind == "paths" || kind == "paths-ignore" {
		s += fmt.Sprintf(" which changes only %q", sc.path)
	}
	return s
}

// --- the verdict ---------------------------------------------------------------------------

// checkProblem is a proven way for a required check to stay pending, located in a workflow file.
type checkProblem struct {
	file string
	pos  *Pos
	msg  string
}

// checkVerdict is what the analysis concludes about one declaration.
type checkVerdict struct {
	// unresolved is true when no job can report the name.
	unresolved bool
	// suggestions are names that the workflows report and look like the declared one.
	suggestions []string
	// problems are the proven ways for the check to stay pending.
	problems []checkProblem
	// unknown says why the analysis cannot tell whether the check always reports, or is empty.
	unknown string
}

func analyzeRequiredCheck(ws checkWorkflows, rc *RequiredCheck, declared string) *checkVerdict {
	v := &checkVerdict{}
	res := resolveCheck(ws, rc.Name)
	if len(res.sources) == 0 {
		if len(res.maybe) > 0 {
			v.unknown = "no job reports it for sure, and " + res.maybe[0]
			return v
		}
		v.unresolved = true
		v.suggestions = suggestChecks(ws, rc.Name)
		return v
	}
	seen := map[string]bool{}
	add := func(b *blame, example string) {
		k := fmt.Sprintf("%s:%d:%d:%s", b.file, b.pos.Line, b.pos.Col, b.kind)
		if seen[k] {
			return
		}
		seen[k] = true
		msg := fmt.Sprintf("required check %q never reports for %s, because %s, so GitHub waits for it forever and the change cannot merge. %s (declared at %s)", rc.Name, example, b.what, b.fix, declared)
		if b.kind == "job" {
			msg = fmt.Sprintf("required check %q is never reported, because %s. %s (declared at %s)", rc.Name, b.what, b.fix, declared)
		}
		v.problems = append(v.problems, checkProblem{file: b.file, pos: b.pos, msg: msg})
	}
	// A source whose job never runs reports the name in no scenario
	var live []checkSource
	var dead []*blame
	for _, s := range res.sources {
		cj, cause := s.deadJob()
		if cause == nil {
			live = append(live, s)
			continue
		}
		pos := cause.Pos
		if cause.If != nil && cause.If.Pos != nil {
			pos = cause.If.Pos
		}
		what := fmt.Sprintf("job %q never runs (its \"if:\" is always false), and GitHub reports a skipped matrix job or reusable workflow call under its bare name", cj.job.ID.Value)
		if cause != cj.job {
			what = fmt.Sprintf("job %q needs job %q, which never runs (its \"if:\" is always false), so it is skipped and GitHub reports it under its bare name", cj.job.ID.Value, cause.ID.Value)
			pos = cj.job.Pos
		}
		dead = append(dead, &blame{file: cj.file, pos: pos, kind: "job", what: what, fix: "remove the condition, or require another check"})
	}
	if len(live) == 0 {
		if len(res.maybe) > 0 {
			v.unknown = "the jobs that report it never run, and " + res.maybe[0]
			return v
		}
		for _, b := range dead {
			add(b, "")
		}
		return v
	}
	for _, sc := range scenarios(rc, live) {
		var blames []*blame
		unknown := ""
		reported := false
		for _, s := range live {
			t, b, why := starts(s.root, s.w, sc)
			switch t {
			case reachYes:
				reported = true
			case reachUnknown:
				if unknown == "" {
					unknown = why
				}
			case reachNo:
				blames = append(blames, b)
			}
			if reported {
				break
			}
		}
		if reported {
			continue
		}
		if unknown == "" && len(res.maybe) > 0 {
			unknown = res.maybe[0]
		}
		if unknown != "" {
			if v.unknown == "" {
				v.unknown = unknown
			}
			continue
		}
		for _, b := range blames {
			add(b, sc.describe(b.kind))
		}
	}
	return v
}

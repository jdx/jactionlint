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
					err = fmt.Errorf("unknown event %q in \"required-checks\" at line %d, column %d. available events are %s", e, v.Line, v.Column, sortedQuotes(slices.Clone(requiredCheckEvents)))
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

// with returns the workflows with the ones in linted replaced: the files as they were linted, whose content may differ
// from the disk.
func (ws checkWorkflows) with(linted map[string]*Workflow) checkWorkflows {
	ret := make(checkWorkflows, len(ws)+len(linted))
	for k, v := range ws {
		ret[k] = v
	}
	for k, v := range linted {
		ret[k] = v
	}
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

// lazyCheckWorkflows are the workflows of a repository, read once per run and shared by the files linted in parallel.
// The syntax trees cache what they parse (String.exprCache) without a lock, so whoever reads them holds mu.
type lazyCheckWorkflows struct {
	once sync.Once
	mu   sync.Mutex
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
				// GitHub prints a value from what it means: an unquoted 1.10 is the number 1.1. A value whose text is not
				// what GitHub prints is not literal.
				s, ok := v.(*RawYAMLString)
				if !ok || !isStableMatrixString(s) || strings.Contains(s.Value, ", ") || strings.ContainsAny(s.Value, "()") {
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
	// The expression lexer finds where each expression ends, also when a string in it has "}}". A fresh String keeps
	// the parse out of the caches of the shared syntax trees.
	c := (&String{Value: name}).placeholders()
	pos := 0
	for _, p := range c.list {
		start := p.after - len("${{")
		b.WriteString(regexp.QuoteMeta(name[pos:start]))
		b.WriteString("(?s:.*)")
		pos = p.after + p.n
	}
	if c.failed {
		b.WriteString("(?s:.*)") // an expression that does not parse: anything from it on
	} else {
		b.WriteString(regexp.QuoteMeta(name[pos:]))
	}
	if matrix {
		b.WriteString(`(?: \(.*\))?`)
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// jobBaseName is the name of the check of a job before a matrix or a reusable workflow expands it: its name, or its ID.
func jobBaseName(job *Job) string {
	if job.Name != nil {
		return job.Name.Value
	}
	return job.ID.Value
}

// hasMatrix reports whether GitHub expands the job into one job per combination of a matrix.
func hasMatrix(job *Job) bool {
	return job.Strategy != nil && (job.Strategy.Expression != nil || job.Strategy.Matrix != nil)
}

// maySkip reports whether GitHub can skip the job: it has a condition, or it needs other jobs.
func maySkip(job *Job) bool {
	return (job.If != nil && !isStaticallyTrue(job.If)) || len(job.Needs) > 0
}

// isStaticallyTrue reports whether the condition is the literal true.
func isStaticallyTrue(s *String) bool {
	node, _, _, ok := parseWholeExpr(s)
	return ok && evalLiteralBool(node) == condTrue
}

// bareNames returns the candidate of the name that a matrix job or a reusable workflow call reports when GitHub skips
// it: the name before the expansion. A job that never runs reports it for sure, a job that may be skipped reports it
// only sometimes, and a job that always runs never reports it.
func bareNames(file string, w *Workflow, job *Job) nameCandidate {
	cj := checkJob{file: file, w: w, job: job}
	name := jobBaseName(job)
	var re *regexp.Regexp
	if ContainsExpression(name) {
		re = templateRegexp(name, false)
	}
	return nameCandidate{chain: []checkJob{cj}, why: jobLabel(file, job) + " reports its unexpanded name only when GitHub skips it", match: func(s string) reach {
		never := neverRuns(w, job, map[*Job]bool{}) != nil
		if !never && !maySkip(job) {
			return reachNo // the job always runs, so it is always expanded
		}
		if re != nil {
			if re.MatchString(s) {
				return reachUnknown // what GitHub shows for the name of a skipped job is not documented
			}
			return reachNo
		}
		switch {
		case s != name:
			return reachNo
		case never:
			return reachYes
		}
		return reachUnknown
	}}
}

// ownNames returns the candidate of the names that the job itself reports, before a reusable workflow it calls adds
// the names of its jobs.
func ownNames(file string, w *Workflow, job *Job) nameCandidate {
	cj := checkJob{file: file, w: w, job: job}
	name := jobBaseName(job)
	var m *Matrix
	if job.Strategy != nil {
		m = job.Strategy.Matrix
	}
	if !hasMatrix(job) {
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
			resolved := true // every reference names a key of the matrix
			for _, c := range combos {
				n := reMatrixRef.ReplaceAllStringFunc(name, func(ref string) string {
					v, found := c[strings.ToLower(reMatrixRef.FindStringSubmatch(ref)[1])]
					resolved = resolved && found
					return v
				})
				names[n] = true // combinations may share a name
			}
			if resolved {
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

// maxNameCandidates bounds the names that the reusable workflows of one job expand to. A workflow that calls others
// many times multiplies them; beyond the bound the names of the job are unknown.
const maxNameCandidates = 2000

// candidateWalk is the state of the expansion of the names of the jobs of one workflow.
type candidateWalk struct {
	ws checkWorkflows
	// active are the workflows on the path of calls being expanded: a workflow that calls itself is a cycle, which
	// GitHub rejects.
	active map[string]bool
	n      int // the candidates made so far
}

// jobCandidates returns the names that the job reports, following the local reusable workflows it calls.
func jobCandidates(ws checkWorkflows, file string, w *Workflow, job *Job) []nameCandidate {
	cw := &candidateWalk{ws: ws, active: map[string]bool{file: true}}
	cs := cw.job(file, w, job, 0)
	if cw.n > maxNameCandidates {
		return []nameCandidate{{chain: []checkJob{{file: file, w: w, job: job}}, why: jobLabel(file, job) + " calls reusable workflows that expand to too many jobs", match: func(string) reach { return reachUnknown }}}
	}
	return cs
}

func (cw *candidateWalk) job(file string, w *Workflow, job *Job, depth int) []nameCandidate {
	ws := cw.ws
	if cw.n > maxNameCandidates {
		return nil
	}
	cw.n++
	own := ownNames(file, w, job)
	calls := job.WorkflowCall != nil && job.WorkflowCall.Uses != nil
	var bare []nameCandidate
	if hasMatrix(job) || calls {
		bare = []nameCandidate{bareNames(file, w, job)}
	}
	if !calls {
		return append([]nameCandidate{own}, bare...)
	}
	uses := job.WorkflowCall.Uses.Value
	unknownCallee := func(why string) []nameCandidate {
		return append([]nameCandidate{callNames(own, nameCandidate{why: why, match: func(string) reach { return reachUnknown }})}, bare...)
	}
	if depth >= maxReusableNesting || ContainsExpression(uses) {
		return unknownCallee(jobLabel(file, job) + " calls a reusable workflow that is not known")
	}
	target, ok := localTarget(uses)
	if !ok || target.kind != 'w' {
		return unknownCallee(fmt.Sprintf("%s calls the reusable workflow %q of another repository", jobLabel(file, job), uses))
	}
	callee, ok := ws[target.path]
	if !ok {
		return unknownCallee(fmt.Sprintf("%s calls %q, which is not a workflow of the repository", jobLabel(file, job), uses))
	}
	if cw.active[target.path] {
		return unknownCallee(fmt.Sprintf("%s calls %q, which is already on the path of calls (a cycle)", jobLabel(file, job), uses))
	}
	cw.active[target.path] = true
	defer delete(cw.active, target.path)
	ret := bare
	for _, id := range jobIDsInOrder(callee) {
		for _, c := range cw.job(target.path, callee, callee.Jobs[id], depth+1) {
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
	// maybe are the jobs that may report it, with the reason it is not known.
	maybe []maybeSource
}

// maybeSource is a path from a workflow to a job that may report the check.
type maybeSource struct {
	root string
	w    *Workflow
	why  string
}

// firstMaybe returns why the first job that may report the check does, or "".
func (r *checkResolution) firstMaybe() string {
	if len(r.maybe) == 0 {
		return ""
	}
	return r.maybe[0].why
}

func resolveCheck(ws checkWorkflows, name string) *checkResolution {
	res := &checkResolution{}
	for _, p := range ws.sortedPaths() {
		w := ws[p]
		if onlyCalled(w) {
			continue // its jobs report checks under the names of their callers
		}
		for _, id := range jobIDsInOrder(w) {
			for _, c := range jobCandidates(ws, p, w, w.Jobs[id]) {
				switch c.match(name) {
				case reachYes:
					res.sources = append(res.sources, checkSource{root: p, w: w, chain: c.chain})
				case reachUnknown:
					res.maybe = append(res.maybe, maybeSource{root: p, w: w, why: c.why})
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
				if ex, ok := expandedExample(j); ok && own == name {
					add(ex) // a matrix job reports its bare name only when it is skipped
				}
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

// expandedExample returns the name of the check of the first combination of a literal matrix, with the values in the
// order of the keys in the file.
func expandedExample(job *Job) (string, bool) {
	if !hasMatrix(job) || job.Strategy.Expression != nil {
		return "", false
	}
	m := job.Strategy.Matrix
	if _, ok := literalMatrix(m); !ok {
		return "", false
	}
	rows := make([]*MatrixRow, 0, len(m.Rows))
	for _, r := range m.Rows {
		rows = append(rows, r)
	}
	slices.SortFunc(rows, func(a, b *MatrixRow) int {
		if a.Name.Pos.IsBefore(b.Name.Pos) {
			return -1
		}
		return 1
	})
	vs := make([]string, 0, len(rows))
	for _, r := range rows {
		vs = append(vs, r.Values[0].(*RawYAMLString).Value)
	}
	return jobBaseName(job) + " (" + strings.Join(vs, ", ") + ")", true
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
	// Any status check function, success() included, replaces the implicit success(): such a job may run after a job
	// it needs was skipped (`!success()`), so only a condition without one is known to skip it
	cond, ok := conditionExprs(job.If)
	if !ok || callsStatusFunction(cond, true) {
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
	path     string // the only changed file, empty to leave the path filters out
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
		if sc.path == "" {
			break // the scenario asks about the other filters only
		}
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

// genericChangedPaths are the files a scenario may change besides the ones that the filters name and the workflow files.
var genericChangedPaths = []string{"README.md", "docs/index.md", "src/main.go", "package.json"}

// scenarios returns the changes to try for the declaration: each event, activity and branch, and each changed file
// that the path filters of the workflows that report the check make interesting.
func scenarios(rc *RequiredCheck, sources []checkSource) []checkScenario {
	var ret []checkScenario
	for _, ev := range rc.events() {
		paths := slices.Clone(genericChangedPaths)
		for _, s := range sources {
			if !slices.Contains(paths, s.root) {
				paths = append(paths, s.root) // a change of the workflow itself
			}
		}
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
			v.unknown = "no job reports it for sure, and " + res.firstMaybe()
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
			v.unknown = "the jobs that report it never run, and " + res.firstMaybe()
			return v
		}
		for _, b := range dead {
			add(b, "")
		}
		return v
	}
	type group struct{ event, activity, branch string }
	var groups []group
	decided := map[group]bool{} // a change of the group is reported, or cannot be decided
	for _, sc := range scenarios(rc, live) {
		g := group{sc.event, sc.activity, sc.branch}
		if !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
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
		// A job whose name is not known may report the check, but only when its workflow may start for the change
		for _, m := range res.maybe {
			if unknown != "" {
				break
			}
			if t, _, _ := starts(m.root, m.w, sc); t != reachNo {
				unknown = m.why
			}
		}
		decided[g] = true
		if unknown != "" {
			if v.unknown == "" {
				v.unknown = unknown
			}
			continue
		}
		// When a workflow that reports the check runs on the event, its filters are the cause: adding the event to
		// another workflow with a job of the same name is not the fix
		subscribed := false
		for _, s := range live {
			subscribed = subscribed || eventHook(s.w, sc.event) != nil
		}
		for _, b := range blames {
			if subscribed && b.kind == "event" {
				continue
			}
			add(b, sc.describe(b.kind))
		}
	}
	// The changed files tried are examples. When none of them is skipped by every workflow, the path filters may still
	// leave a file uncovered: say so unless the filters prove that every file starts a workflow.
	for _, g := range groups {
		if decided[g] || v.unknown != "" {
			continue
		}
		if why := pathGap(live, checkScenario{event: g.event, activity: g.activity, branch: g.branch}); why != "" {
			v.unknown = why
		}
	}
	return v
}

// matchesEveryPath reports whether an entry of `paths` matches every file.
func matchesEveryPath(pat string) bool {
	return pat == "**" || pat == "**/*" || pat == "**/**"
}

// pathGap tells whether the path filters of the workflows that start for the scenario (all but the path filters) may
// leave a changed file that starts none of them, when no example change found one. It returns why it cannot tell, or
// "" when every file starts one of them.
func pathGap(live []checkSource, sc checkScenario) string {
	var includes []string
	var ignoreOnly []*WebhookEventFilter
	started := false
	undecided := "" // why a workflow's filters cannot be evaluated; it matters only if no workflow covers every file
	for _, s := range live {
		if t, _, _ := starts(s.root, s.w, sc); t != reachYes {
			continue
		}
		started = true
		h := eventHook(s.w, sc.event)
		if sc.event == "merge_group" || h.Paths.IsEmpty() && h.PathsIgnore.IsEmpty() {
			return "" // the workflow starts whatever the change
		}
		evaluable := true
		for _, f := range []*WebhookEventFilter{h.Paths, h.PathsIgnore} {
			if f == nil {
				continue
			}
			for _, v := range f.Values {
				if strings.HasPrefix(v.Value, "!") || ContainsExpression(v.Value) {
					evaluable = false // a negation is applied in order, and an expression is not known
				}
			}
		}
		if !evaluable {
			if undecided == "" {
				undecided = fmt.Sprintf("jactionlint cannot evaluate the path filters of %q in %s", sc.event, s.root)
			}
			continue
		}
		if h.Paths.IsEmpty() {
			ignoreOnly = append(ignoreOnly, h.PathsIgnore)
			continue
		}
		for _, v := range h.Paths.Values {
			if matchesEveryPath(v.Value) {
				return ""
			}
			includes = append(includes, v.Value)
		}
	}
	if !started {
		return ""
	}
	// A workflow with only `paths-ignore` starts for every file but the ignored ones. When each of its entries is an
	// entry of `paths` of another workflow (GitHub's workaround of two workflows with opposite filters), the two
	// together start for every file.
	for _, ig := range ignoreOnly {
		covered := true
		for _, v := range ig.Values {
			covered = covered && slices.Contains(includes, v.Value)
		}
		if covered {
			return ""
		}
	}
	if undecided != "" {
		return undecided
	}
	return "no example change jactionlint tried is skipped by the path filters of every workflow that reports it, but they may still skip a change of another file"
}

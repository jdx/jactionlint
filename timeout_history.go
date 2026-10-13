package jactionlint

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
)

// This file reports how long the jobs of the workflows of a repository really took (--suggest-timeouts), to help a
// person choose a timeout-minutes, or the default-minutes of the rule missing-timeout. It is the precedent of
// ghatm: the durations of the jobs of the recent successful runs, and a number computed from the longest one.
//
// It is a report and nothing else. No file is written and no rule or fix uses a number from it: a timeout is a
// decision of the person who reads the report (what a cancelled job costs, and whether the next run may be slower than the
// ones on record). The report says what it cannot know instead of hiding it: how many runs and which dates it
// read, how many samples a job has, that a matrix is one job with several legs, which jobs it could not match to the
// file, and that a longest run on record is not a bound on the next one.

// TimeoutReportVersion is the version of the document of --suggest-timeouts with --format json.
const TimeoutReportVersion = 1

const (
	// DefaultTimeoutHistoryRuns is how many of the latest successful runs of each workflow are read.
	DefaultTimeoutHistoryRuns = 20
	// DefaultTimeoutBuffer is the factor by which the longest duration on record is multiplied.
	DefaultTimeoutBuffer = 2.0
	// minTimeoutSamples is how many durations a job needs for the report to suggest a number: with fewer, the longest of them says
	// too little about the next run.
	minTimeoutSamples = 3
)

// TimeoutOptions configure SuggestTimeouts.
type TimeoutOptions struct {
	// Runs is how many of the latest successful runs of each workflow are read (1 to 100). 0 means DefaultTimeoutHistoryRuns.
	Runs int
	// Buffer is the factor of the suggestion: the longest duration on record times Buffer, rounded up to whole
	// minutes. It must be at least 1. 0 means DefaultTimeoutBuffer.
	Buffer float64
}

// TimeoutReport is the document of --suggest-timeouts.
type TimeoutReport struct {
	Version int `json:"version"`
	// Repository is "owner/repo".
	Repository string `json:"repository"`
	// Runs is how many successful runs of each workflow were asked for, and Buffer the factor of the suggestions.
	Runs   int     `json:"runs_per_workflow"`
	Buffer float64 `json:"buffer"`
	// MinSamples is how many durations a job needs to get a suggestion.
	MinSamples int `json:"min_samples"`
	// Complete is false when a lookup failed (a rate limit, a 403, a timeout): the numbers are from what could be read.
	Complete bool `json:"complete"`
	// SkippedLookups counts the lookups that failed.
	SkippedLookups int `json:"skipped_lookups"`
	// LargestSuggestedMinutes is the largest suggestion of the report. A single default-minutes of the rule missing-timeout has
	// to be at least this long not to cancel the slowest job on record. It is absent when there is no suggestion.
	LargestSuggestedMinutes int                `json:"largest_suggested_minutes,omitempty"`
	Workflows               []*TimeoutWorkflow `json:"workflows"`
}

// TimeoutWorkflow is a workflow file of the report.
type TimeoutWorkflow struct {
	// Path is relative to the root of the repository, with "/" as the separator.
	Path string `json:"path"`
	// Runs is how many successful runs were read, and Oldest and Newest the dates (UTC) on which the oldest and the newest
	// were created.
	Runs   int    `json:"runs"`
	Oldest string `json:"oldest,omitempty"`
	Newest string `json:"newest,omitempty"`
	// Error says why the workflow has no numbers: GitHub has no run of it, the runs could not be read.
	Error string `json:"error,omitempty"`
	// TruncatedRuns counts the runs whose jobs are more than the 100 the report reads (a large matrix).
	TruncatedRuns int `json:"truncated_runs,omitempty"`
	// Unmatched are the jobs of the runs that the report could not match to a job of the file.
	Unmatched []TimeoutUnmatched `json:"unmatched,omitempty"`
	Jobs      []*TimeoutJob      `json:"jobs"`
}

// TimeoutUnmatched is a job of the runs that matches no job of the file.
type TimeoutUnmatched struct {
	Name    string `json:"name"`
	Samples int    `json:"samples"`
}

// TimeoutJob is a job of a workflow file.
type TimeoutJob struct {
	ID string `json:"id"`
	// Line is the line of the job in the file.
	Line int `json:"line,omitempty"`
	// Kind is "job", "matrix" (one job that runs once for each combination of values: the legs share the timeout) or
	// "reusable-workflow" (a call of a workflow, which has the jobs and the timeouts).
	Kind string `json:"kind"`
	// DeclaredMinutes is the timeout-minutes the job sets; DeclaredDynamic is true when an expression decides it.
	DeclaredMinutes *float64 `json:"declared_minutes,omitempty"`
	DeclaredDynamic bool     `json:"declared_dynamic,omitempty"`
	// Legs is how many different names of the runs matched: the combinations of a matrix that ran.
	Legs int `json:"legs"`
	// Samples is how many successful executions of the job (every leg) were measured.
	Samples int `json:"samples"`
	// MinMinutes, MedianMinutes and MaxMinutes are the duration of a job from its start on a runner to its end, in minutes
	// to a tenth. They are absent without samples.
	MinMinutes    *float64 `json:"min_minutes,omitempty"`
	MedianMinutes *float64 `json:"median_minutes,omitempty"`
	MaxMinutes    *float64 `json:"max_minutes,omitempty"`
	// SuggestedMinutes is MaxMinutes times the buffer, rounded up to whole minutes. It is absent when the job has fewer than
	// MinSamples samples (Note says so).
	SuggestedMinutes int `json:"suggested_minutes,omitempty"`
	// Note says why a job has no suggestion.
	Note string `json:"note,omitempty"`
}

// timeoutJobSpec is a job of the file as the matching sees it.
type timeoutJobSpec struct {
	job     *TimeoutJob
	id      string
	name    string         // the literal name, or ""
	pattern *regexp.Regexp // the name with expressions, or nil
	// patternMatrix is pattern followed by the values of a matrix in parentheses, for a matrix
	patternMatrix *regexp.Regexp
	matrix        bool
	reusable      bool
	// opaque is true when the name of the job is only an expression, so no run can be matched to the job by name
	opaque bool
	// durations of the matched executions in seconds, and the names of the runs that matched
	durations []float64
	legs      map[string]bool
}

var reExpression = regexp.MustCompile(`\$\{\{.*?\}\}`)

func newTimeoutJobSpec(id string, j *Job) *timeoutJobSpec {
	sp := &timeoutJobSpec{id: id, legs: map[string]bool{}}
	sp.matrix = j.Strategy != nil && (j.Strategy.Matrix != nil || j.Strategy.Expression != nil)
	sp.reusable = j.WorkflowCall != nil
	if j.Name != nil && j.Name.Value != "" {
		if j.Name.ContainsExpression() {
			parts := reExpression.Split(j.Name.Value, -1)
			literal := false
			for i := range parts {
				literal = literal || strings.TrimSpace(parts[i]) != ""
				parts[i] = regexp.QuoteMeta(parts[i])
			}
			if !literal {
				// A name that is only an expression fits any name of a run, so it fits none by its name: ${{ matrix.os }}
				// would take the durations of every other job
				sp.opaque = true
			} else {
				expr := strings.Join(parts, ".+")
				sp.pattern = regexp.MustCompile("^" + expr + "$")
				sp.patternMatrix = regexp.MustCompile("^" + expr + `\s\(.+\)$`)
			}
		} else {
			sp.name = j.Name.Value
		}
	}
	return sp
}

// matches reports whether the name of a job of a run is a job of the file: its name (its ID when it has none), an
// expression in the name standing for any text, and for a matrix the values in parentheses that GitHub adds to the
// name; for a call of a workflow the name of the job of the callee after " / ".
func (sp *timeoutJobSpec) matches(name string) bool {
	if sp.opaque {
		return false
	}
	if sp.reusable {
		base := sp.id
		if sp.name != "" {
			base = sp.name
		}
		return name == sp.id || name == base || strings.HasPrefix(name, sp.id+" / ") || strings.HasPrefix(name, base+" / ")
	}
	if sp.pattern != nil {
		return sp.pattern.MatchString(name) || (sp.matrix && sp.patternMatrix.MatchString(name))
	}
	base := sp.name
	if base == "" {
		base = sp.id
	}
	return name == base || (sp.matrix && strings.HasPrefix(name, base+" (") && strings.HasSuffix(name, ")"))
}

// SuggestTimeouts reads the durations of the jobs of the recent successful runs of the workflows and returns the report.
// The files are workflow files, or the workflows of the repository when there are none. It needs the online checks
// (LinterOptions.Online): the report is made of what GitHub says. It writes nothing.
func (l *Linter) SuggestTimeouts(files []string, project *Project, opts TimeoutOptions) (*TimeoutReport, error) {
	if opts.Runs == 0 {
		opts.Runs = DefaultTimeoutHistoryRuns
	}
	if opts.Buffer == 0 {
		opts.Buffer = DefaultTimeoutBuffer
	}
	if opts.Runs < 1 || opts.Runs > maxWorkflowRuns {
		return nil, &usageError{fmt.Errorf("--timeout-history-runs must be between 1 and %d", maxWorkflowRuns)}
	}
	if opts.Buffer < 1 || math.IsNaN(opts.Buffer) || math.IsInf(opts.Buffer, 0) {
		return nil, &usageError{errors.New("--timeout-buffer must be a number of at least 1")}
	}
	if len(files) == 0 {
		var err error
		files, project, err = l.repositoryFiles(l.cwd)
		if err != nil {
			return nil, err
		}
	}
	if project == nil {
		p, err := l.projects.At(files[0])
		if err != nil {
			return nil, err
		}
		project = p
	}
	if project == nil {
		return nil, errors.New("--suggest-timeouts needs a repository to tell which one GitHub is asked about")
	}
	repo := (&RuleEnv{project: project}).repository()
	if repo == "" {
		return nil, errors.New("--suggest-timeouts could not tell the GitHub repository: set GITHUB_REPOSITORY, or give the clone a remote \"origin\" on github.com")
	}
	owner, name, _ := strings.Cut(repo, "/")
	sess, err := l.onlineSession(l.configFor(project))
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, &usageError{errors.New("--suggest-timeouts needs the online checks, which are turned off")}
	}
	if _, ok := sess.client.(GitHubRunsClient); !ok {
		return nil, errNoRunsLookup
	}

	rep := &TimeoutReport{Version: TimeoutReportVersion, Repository: repo, Runs: opts.Runs, Buffer: opts.Buffer, MinSamples: minTimeoutSamples, Workflows: []*TimeoutWorkflow{}}
	root := absPath(project.RootDir())
	for _, f := range files {
		if l.isDependabotFile(f) || l.isActionsLockFile(f) || l.isActionFile(f) {
			continue
		}
		abs := l.absFilePath(f)
		src, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		rel, rerr := filepath.Rel(root, abs)
		if rerr != nil || strings.HasPrefix(rel, "..") {
			rel = f
		}
		wf := l.timeoutWorkflow(sess, owner, name, filepath.ToSlash(rel), filepath.Base(abs), src, opts)
		rep.Workflows = append(rep.Workflows, wf)
	}
	sort.SliceStable(rep.Workflows, func(i, j int) bool { return rep.Workflows[i].Path < rep.Workflows[j].Path })
	rep.SkippedLookups = sess.skippedLookups()
	rep.Complete = rep.SkippedLookups == 0
	for _, wf := range rep.Workflows {
		for _, j := range wf.Jobs {
			rep.LargestSuggestedMinutes = max(rep.LargestSuggestedMinutes, j.SuggestedMinutes)
		}
	}
	return rep, nil
}

func (l *Linter) timeoutWorkflow(sess *onlineSession, owner, repo, path, file string, src []byte, opts TimeoutOptions) *TimeoutWorkflow {
	wf := &TimeoutWorkflow{Path: path, Jobs: []*TimeoutJob{}}
	w, _ := Parse(src)
	if w == nil {
		wf.Error = "the file is not valid YAML"
		return wf
	}
	var specs []*timeoutJobSpec
	for id, j := range w.Jobs {
		if j == nil {
			continue
		}
		sp := newTimeoutJobSpec(id, j)
		sp.job = &TimeoutJob{ID: id, Kind: "job", Legs: 0}
		if j.Pos != nil {
			sp.job.Line = j.Pos.Line
		}
		switch {
		case sp.reusable:
			sp.job.Kind = "reusable-workflow"
		case sp.matrix:
			sp.job.Kind = "matrix"
		}
		if t := j.TimeoutMinutes; t != nil {
			if t.Expression != nil {
				sp.job.DeclaredDynamic = true
			} else {
				v := t.Value
				sp.job.DeclaredMinutes = &v
			}
		}
		specs = append(specs, sp)
	}
	sort.Slice(specs, func(i, j int) bool {
		a, b := specs[i].job, specs[j].job
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.ID < b.ID
	})
	for _, sp := range specs {
		wf.Jobs = append(wf.Jobs, sp.job)
	}

	if !validWorkflowFile(file) {
		wf.Error = "the file name cannot be put in the request to GitHub"
		return wf
	}
	runs, err := sess.WorkflowRuns(owner, repo, file, opts.Runs)
	if err != nil {
		switch {
		case errors.Is(err, ErrGitHubNotFound):
			wf.Error = "GitHub has no runs for this file: it was never pushed, or the token cannot read the runs of the repository"
		default:
			wf.Error = "the runs could not be read: " + err.Error()
		}
		return wf
	}
	wf.Runs = len(runs)
	if len(runs) == 0 {
		wf.Error = "no successful run on record"
	}
	for i, r := range runs {
		d := r.CreatedAt.UTC().Format("2006-01-02")
		if i == 0 || d > wf.Newest {
			wf.Newest = d
		}
		if i == 0 || d < wf.Oldest {
			wf.Oldest = d
		}
	}

	// The jobs of each run, a few requests at a time
	jobs := make([]*GitHubRunJobs, len(runs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i, r := range runs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if j, err := sess.RunJobs(owner, repo, r.ID); err == nil {
				jobs[i] = j
			}
		}()
	}
	wg.Wait()

	unmatched := map[string]int{}
	for _, rj := range jobs {
		if rj == nil {
			continue
		}
		if rj.Truncated {
			wf.TruncatedRuns++
		}
		for _, j := range rj.Jobs {
			if j.Conclusion != "success" || j.StartedAt.IsZero() || j.CompletedAt.Before(j.StartedAt) {
				continue
			}
			var hit *timeoutJobSpec
			n := 0
			for _, sp := range specs {
				if sp.matches(j.Name) {
					hit, n = sp, n+1
				}
			}
			if n != 1 {
				unmatched[j.Name]++ // no job, or several that the name fits
				continue
			}
			hit.legs[j.Name] = true
			hit.durations = append(hit.durations, j.CompletedAt.Sub(j.StartedAt).Seconds())
		}
	}
	for _, sp := range specs {
		sp.finish(opts)
	}
	for name, n := range unmatched {
		wf.Unmatched = append(wf.Unmatched, TimeoutUnmatched{Name: name, Samples: n})
	}
	sort.Slice(wf.Unmatched, func(i, j int) bool {
		a, b := wf.Unmatched[i], wf.Unmatched[j]
		if a.Samples != b.Samples {
			return a.Samples > b.Samples
		}
		return a.Name < b.Name
	})
	return wf
}

func tenths(seconds float64) *float64 {
	v := math.Round(seconds/6) / 10 // minutes to a tenth
	return &v
}

// finish computes the numbers of a job from its durations.
func (sp *timeoutJobSpec) finish(opts TimeoutOptions) {
	j := sp.job
	j.Legs = len(sp.legs)
	j.Samples = len(sp.durations)
	switch {
	case sp.opaque:
		j.Note = "the name of the job is only an expression, so its runs cannot be told from the other jobs by name and are listed as not matched"
		j.Samples, j.Legs = 0, 0
		return
	case sp.reusable:
		j.Note = "a call of a reusable workflow: the timeouts belong to the jobs of the called workflow, which this report does not follow"
		j.Samples, j.Legs = 0, 0
		return
	case j.Samples == 0:
		j.Note = "no successful execution of this job on record"
		return
	}
	d := append([]float64(nil), sp.durations...)
	sort.Float64s(d)
	j.MinMinutes, j.MaxMinutes = tenths(d[0]), tenths(d[len(d)-1])
	mid := d[len(d)/2]
	if len(d)%2 == 0 {
		mid = (d[len(d)/2-1] + d[len(d)/2]) / 2
	}
	j.MedianMinutes = tenths(mid)
	if j.Samples < minTimeoutSamples {
		j.Note = fmt.Sprintf("only %d sample(s): too few for a suggestion", j.Samples)
		return
	}
	j.SuggestedMinutes = max(int(math.Ceil(d[len(d)-1]/60*opts.Buffer)), 1)
}

// JSON returns the report as indented JSON with a final newline.
func (r *TimeoutReport) JSON() ([]byte, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// WriteText writes the report for a person.
func (r *TimeoutReport) WriteText(w io.Writer) error {
	fmt.Fprintf(w, "Durations of the jobs of %s: the latest %d successful runs of each workflow, from the start of a job on a runner to its end.\n", r.Repository, r.Runs)
	fmt.Fprintf(w, "A suggestion is the longest duration on record times %s, rounded up to whole minutes, and needs at least %d samples.\n", trimFloat(r.Buffer), r.MinSamples)
	for _, wf := range r.Workflows {
		fmt.Fprintf(w, "\n%s", wf.Path)
		if wf.Runs > 0 {
			fmt.Fprintf(w, " (%s, %s to %s)", countNoun(wf.Runs, "run"), wf.Oldest, wf.Newest)
		}
		fmt.Fprintln(w)
		if wf.Error != "" {
			fmt.Fprintf(w, "  %s\n", wf.Error)
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, j := range wf.Jobs {
			declared := "timeout-minutes not set"
			switch {
			case j.DeclaredDynamic:
				declared = "timeout-minutes from an expression"
			case j.DeclaredMinutes != nil:
				declared = "timeout-minutes: " + trimFloat(*j.DeclaredMinutes)
			}
			kind := j.ID
			switch {
			case j.Kind == "matrix" && j.Legs > 0:
				kind += fmt.Sprintf(" (matrix, %s)", countNoun(j.Legs, "leg"))
			case j.Kind == "reusable-workflow":
				kind += " (calls a workflow)"
			}
			stats := ""
			switch {
			case j.MaxMinutes != nil:
				stats = fmt.Sprintf("samples %d\tmin %s\tmedian %s\tmax %s", j.Samples, trimFloat(*j.MinMinutes), trimFloat(*j.MedianMinutes), trimFloat(*j.MaxMinutes))
			default:
				stats = "samples 0\t\t\t"
			}
			sug := ""
			switch {
			case j.SuggestedMinutes > 0:
				sug = fmt.Sprintf("suggested %d", j.SuggestedMinutes)
			case j.Note != "":
				sug = j.Note
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", kind, declared, stats, sug)
		}
		tw.Flush()
		if wf.TruncatedRuns > 0 {
			fmt.Fprintf(w, "  %d run(s) have more than 100 jobs: the first 100 were read\n", wf.TruncatedRuns)
		}
		for i, u := range wf.Unmatched {
			if i == 5 {
				fmt.Fprintf(w, "  and %d more names of jobs that match no job of the file\n", len(wf.Unmatched)-5)
				break
			}
			fmt.Fprintf(w, "  not matched to a job of the file: %q (%d samples; a job that was renamed since, or a name the report cannot parse)\n", u.Name, u.Samples)
		}
	}
	fmt.Fprintln(w)
	if r.LargestSuggestedMinutes > 0 {
		fmt.Fprintf(w, "The largest suggestion is %d minutes: a single default-minutes of the rule missing-timeout has to be at least this long not to cancel the slowest job on record.\n", r.LargestSuggestedMinutes)
	}
	if !r.Complete {
		fmt.Fprintf(w, "Incomplete: %d lookup(s) failed (a rate limit, a token that cannot read the runs, or a timeout), so the numbers are from what could be read.\n", r.SkippedLookups)
	}
	fmt.Fprintln(w, "Read this before using a number: it is what happened in the runs on record, not a bound on the next one. A job can be slower on a busy runner, with a cold cache, or after a change that")
	fmt.Fprintln(w, "the runs on record predate. Runs that failed or were cancelled are left out, a matrix is one job whose legs share the timeout, and a job is matched to the file by its name, so a job that")
	fmt.Fprintln(w, "was renamed is not. jactionlint writes none of these numbers: set timeout-minutes by hand, or default-minutes of missing-timeout, after you have looked at them.")
	return nil
}

func trimFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}

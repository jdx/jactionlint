package jactionlint

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
)

// FixMode selects which automatic fixes are applied.
type FixMode int

const (
	// FixModeSafe applies the fixes which do not change the behavior of the workflow.
	FixModeSafe FixMode = iota + 1
	// FixModeUnsafe applies the safe fixes and the ones which may change the behavior of the workflow.
	FixModeUnsafe
)

// maxFixPasses limits how many times a file is fixed and linted again. A fix can make another one
// possible or conflict with another fix of the same pass, so fixing repeats until no fix is left. A
// file which still has fixes after this many passes is not written (see FixFailure).
const maxFixPasses = 10

// FixOptions are the options of fixing files.
type FixOptions struct {
	// Mode selects the fixes to apply. It is required.
	Mode FixMode
	// Rules restricts fixing to the fixes of the errors with these rule IDs. When it is empty, the
	// "fix.rules" of the config file is used, and when that is empty too, the fixes of all rules are applied.
	Rules []string
	// DryRun computes the fixes but does not write the files. FixResult.Diff has the changes.
	DryRun bool
	// MaxPasses overrides the number of passes after which fixing gives up. Zero means the default (10).
	MaxPasses int
}

// FixResult is the result of fixing files.
type FixResult struct {
	// Fixed is the paths of the files which were rewritten (or would be with DryRun), as they were given
	// to the linter.
	Fixed []string
	// Applied is the number of fixes which were applied.
	Applied int
	// ByRule is the number of the applied fixes for each rule ID.
	ByRule map[string]int
	// Errors are the errors which remain after fixing, including the ones without a fix. They were
	// printed with the output format of the linter.
	Errors []*Error
	// Failures are the files a fix was refused for. See FixFailure.
	Failures []FixFailure
	// Diff is the changes of all the fixed files as a unified diff. It is set only with DryRun.
	Diff string
}

// FixFailure reports a fix which was not applied because it would have damaged the file, or fixing
// which did not converge. The rest of the fixes of the file are still applied, except when fixing did
// not converge: then the file is left as it was before the fixes which kept repeating.
type FixFailure struct {
	// File is the path of the file as it was given to the linter.
	File string
	// Rules are the IDs of the rules whose fixes are to blame, sorted.
	Rules []string
	// Reason says what was wrong.
	Reason string
}

func (f FixFailure) String() string {
	return fmt.Sprintf("%s: %s (rules: %s)", f.File, f.Reason, strings.Join(f.Rules, ", "))
}

// fixPriority is the order in which the fixes of different rules claim the text when their edits
// overlap: the lower the number, the earlier. The fix of the rule which comes first is applied in the
// pass, and the other one is dropped for the pass and retried in the next, when the finding is
// reported again against the new text (or is gone). The order puts the fixes which remove a
// security problem from the code first, then the ones which change values, then the ones which add
// metadata, and the removal of unused ignore comments last. IDs which are not listed (custom rules)
// come after the listed ones, in alphabetical order. A fix of a safe kind always wins over an unsafe one.
var fixPriority = map[string]int{
	"template-injection":   10,
	"insecure-commands":    20,
	"artipacked":           30,
	"bot-conditions":       40,
	"unpinned-uses":        50,
	"self-repository":      60,
	"obfuscation":          70,
	"missing-permissions":  80,
	"missing-timeout":      90,
	"anonymous-definition": 100,
	"unused-ignore":        900,
}

func fixRank(id string) int {
	if r, ok := fixPriority[id]; ok {
		return r
	}
	return 500
}

// plannedFix is a fix chosen for a pass.
type plannedFix struct {
	// id is the ID of the rule which reported the error.
	id   string
	err  *Error
	fix  *Fix
	line int
}

func newPlannedFix(e *Error) plannedFix {
	id := e.ID
	if id == "" {
		id = e.Kind
	}
	return plannedFix{id: id, err: e, fix: e.Fix, line: e.Line}
}

func (p plannedFix) firstStart() int {
	s := -1
	for _, e := range p.fix.Edits {
		if s < 0 || e.Start < s {
			s = e.Start
		}
	}
	return s
}

// planFixes chooses the fixes to apply in one pass. A fix is chosen when it is allowed by the mode
// and by only (when it is not empty), is valid for the source, and none of its edits conflict with
// the fixes chosen before it. Fixes are considered in a fixed order (safe before unsafe, then by
// fixPriority, then by rule ID, then by position), so the result does not depend on the order of the
// errors. The fixes which lost to another are returned as dropped.
func planFixes(src []byte, errs []*Error, mode FixMode, only, banned map[string]bool) (chosen, dropped []plannedFix) {
	var cands []plannedFix
	for _, e := range errs {
		f := e.Fix
		if f == nil || (f.Unsafe && mode != FixModeUnsafe) || !f.validFor(src) {
			continue
		}
		p := newPlannedFix(e)
		if (len(only) > 0 && !only[p.id]) || banned[p.id] {
			continue
		}
		cands = append(cands, p)
	}
	slices.SortStableFunc(cands, func(a, b plannedFix) int {
		if a.fix.Unsafe != b.fix.Unsafe {
			if b.fix.Unsafe {
				return -1
			}
			return 1
		}
		if ra, rb := fixRank(a.id), fixRank(b.id); ra != rb {
			return ra - rb
		}
		if c := strings.Compare(a.id, b.id); c != 0 {
			return c
		}
		if sa, sb := a.firstStart(), b.firstStart(); sa != sb {
			return sa - sb
		}
		if a.line != b.line {
			return a.line - b.line
		}
		return strings.Compare(a.err.Message, b.err.Message)
	})

	var edits editSet
Fixes:
	for _, p := range cands {
		for _, edit := range p.fix.Edits {
			if edits.conflicts(edit) {
				dropped = append(dropped, p)
				continue Fixes
			}
		}
		// The edits of one fix may conflict with each other; those of the fix are added one by one
		// like before, so the later ones are not checked against the earlier ones of the same fix.
		for _, edit := range p.fix.Edits {
			edits.add(edit)
		}
		chosen = append(chosen, p)
	}
	return chosen, dropped
}

// editsOf returns the distinct edits of the fixes.
func editsOf(fixes []plannedFix) []TextEdit {
	var edits []TextEdit
	seen := map[TextEdit]bool{}
	for _, p := range fixes {
		for _, e := range p.fix.Edits {
			if !seen[e] {
				seen[e] = true
				edits = append(edits, e)
			}
		}
	}
	return edits
}

// applyEdits applies non-conflicting edits to the source.
func applyEdits(src []byte, edits []TextEdit) []byte {
	sorted := slices.Clone(edits)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return sorted[i].End < sorted[j].End
	})
	var out bytes.Buffer
	out.Grow(len(src))
	pos := 0
	for _, e := range sorted {
		out.Write(src[pos:e.Start])
		out.WriteString(e.NewText)
		pos = e.End
	}
	out.Write(src[pos:])
	return out.Bytes()
}

// applyFixes applies the fixes of the errors to the source. A fix is applied only when it is allowed by
// the mode, all its edits are valid and none of them conflict with the fixes chosen before it (see
// planFixes). It returns the new source and the number of applied fixes. The source is returned as is
// when nothing was applied. It does not check the result: FixFiles does.
func applyFixes(src []byte, errs []*Error, mode FixMode) ([]byte, int) {
	chosen, _ := planFixes(src, errs, mode, nil, nil)
	if len(chosen) == 0 {
		return src, 0
	}
	return applyEdits(src, editsOf(chosen)), len(chosen)
}

// applyVerified applies the chosen fixes and checks the result with verifyFix. When the result is
// bad it keeps the fixes one by one, which are checked on their own, and refuses the ones whose edits
// break the file. The refused fixes are returned with the reason.
func applyVerified(src []byte, chosen []plannedFix) (out []byte, accepted []plannedFix, refused map[string]string) {
	out = applyEdits(src, editsOf(chosen))
	if err := verifyFix(src, out, editsOf(chosen)); err == nil {
		return out, chosen, nil
	}
	refused = map[string]string{}
	out = src
	for _, p := range chosen {
		try := append(slices.Clone(accepted), p)
		edits := editsOf(try)
		next := applyEdits(src, edits)
		if err := verifyFix(src, next, edits); err != nil {
			if _, ok := refused[p.id]; !ok {
				refused[p.id] = fmt.Sprintf("the fix at line %d would damage the file: %s", p.line, err)
			}
			continue
		}
		accepted, out = try, next
	}
	return out, accepted, refused
}

// FixRepository fixes the files of the nearest project which LintRepository lints: the workflows and the Dependabot configuration.
// When the directory path is empty, the current working directory will be used instead.
func (l *Linter) FixRepository(dir string, mode FixMode) (*FixResult, error) {
	return l.FixRepositoryWithOptions(dir, FixOptions{Mode: mode})
}

// FixRepositoryWithOptions is like FixRepository with options.
func (l *Linter) FixRepositoryWithOptions(dir string, opts FixOptions) (*FixResult, error) {
	if dir == "" {
		dir = l.cwd
	}
	files, p, err := l.repositoryFiles(dir)
	if err != nil {
		return nil, err
	}
	return l.FixFilesWithOptions(files, p, opts)
}

// FixFiles applies the automatic fixes of the errors found in the files and writes the files which
// changed. See FixFilesWithOptions.
func (l *Linter) FixFiles(filepaths []string, project *Project, mode FixMode) (*FixResult, error) {
	return l.FixFilesWithOptions(filepaths, project, FixOptions{Mode: mode})
}

// FixFilesWithOptions applies the automatic fixes of the errors found in the files and writes the
// files which changed. The project parameter can be nil. In the case, a project is detected from the
// file path.
//
// Fixing happens in memory: each pass lints the text, chooses the non-conflicting fixes (see
// planFixes for how fixes of different rules are ordered), applies them, checks that the result is
// valid YAML which differs from the text before only where the edits are (see verifyFix), and lints
// the result again. It stops when no fix is left, so running it again changes nothing. The file is
// written once at the end, atomically, and not at all when the fixes are unchanged or DryRun is set.
//
// A fix whose edits break the file is refused and its rule is not fixed in that file again; the
// refusal is in the Failures of the result. If the fixes never settle (the same text comes back, or
// MaxPasses is exceeded) the file is left as it was before the fixes which kept repeating and the
// failure names the rules.
//
// Only the fixes allowed by the mode are applied: unsafe fixes need FixModeUnsafe. The errors which
// have no fix are never changed, so they remain. The errors which remain are printed with the
// output format of the linter and returned (to the log writer instead of the output with DryRun, as
// the output has the diff).
func (l *Linter) FixFilesWithOptions(filepaths []string, project *Project, opts FixOptions) (*FixResult, error) {
	if opts.Mode != FixModeSafe && opts.Mode != FixModeUnsafe {
		return nil, errors.New("invalid fix mode")
	}
	res := &FixResult{ByRule: map[string]int{}}
	if len(filepaths) == 0 {
		return res, nil
	}
	maxPasses := opts.MaxPasses
	if maxPasses <= 0 {
		maxPasses = maxFixPasses
	}

	results, err := l.lintFilesQuietly(filepaths, project)
	if err != nil {
		return nil, err
	}

	var diff strings.Builder
	for i, r := range results {
		proj := project
		if proj == nil {
			if proj, err = l.projects.At(r.file); err != nil {
				return nil, err
			}
		}
		only := map[string]bool{}
		restrict := opts.Rules
		if len(restrict) == 0 {
			if cfg := l.configFor(proj); cfg != nil {
				restrict = cfg.Fix.Rules
			}
		}
		for _, id := range restrict {
			only[id] = true
		}

		fr, err := l.fixOne(r, proj, opts.Mode, only, maxPasses)
		if err != nil {
			return nil, err
		}
		for _, f := range fr.failures {
			res.Failures = append(res.Failures, FixFailure{File: r.file, Rules: f.rules, Reason: f.reason})
		}
		results[i] = fr.result
		if bytes.Equal(fr.result.src, r.src) {
			continue
		}
		if !opts.DryRun {
			if err := writeFileUnchanged(r.file, r.src, fr.result.src); err != nil {
				return nil, err
			}
		}
		name := filepath.ToSlash(r.path)
		diff.WriteString(unifiedDiff("a/"+name, "b/"+name, r.src, fr.result.src))
		l.log("Applied", fr.n, "fix(es) to", r.path)
		res.Applied += fr.n
		for id, n := range fr.byRule {
			res.ByRule[id] += n
		}
		res.Fixed = append(res.Fixed, r.file)
	}
	if opts.DryRun {
		res.Diff = diff.String()
		if _, err := fmt.Fprint(l.out, res.Diff); err != nil {
			return nil, err
		}
	}

	results = l.withBaselineResults(l.finishIgnoreRun(results)) // after the fixes: a config file is no file to fix
	for _, r := range results {
		res.Errors = append(res.Errors, r.errs...)
	}
	out := l.out
	if opts.DryRun {
		out = l.logOut // stdout has the diff
	}
	if err := l.printer.print(out, results, l.runInfo()); err != nil {
		return nil, err
	}
	l.printFixSummary(res, opts.DryRun)
	l.reportBaselineNote(results)
	return res, nil
}

func (l *Linter) printFixSummary(res *FixResult, dryRun bool) {
	if res.Applied > 0 {
		verb := "Fixed"
		if dryRun {
			verb = "Would fix"
		}
		fmt.Fprintf(l.logOut, "%s %d problem(s) in %d file(s)\n", verb, res.Applied, len(res.Fixed))
		ids := make([]string, 0, len(res.ByRule))
		for id := range res.ByRule {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool {
			if res.ByRule[ids[i]] != res.ByRule[ids[j]] {
				return res.ByRule[ids[i]] > res.ByRule[ids[j]]
			}
			return ids[i] < ids[j]
		})
		for _, id := range ids {
			fmt.Fprintf(l.logOut, "  %s: %d\n", id, res.ByRule[id])
		}
	}
	for _, f := range res.Failures {
		fmt.Fprintf(l.logOut, "error: %s\n", f)
	}
}

type fixFailure struct {
	rules  []string
	reason string
}

// fixOutcome is the result of fixing one file.
type fixOutcome struct {
	// result is the lint result of the final text.
	result   fileResult
	n        int
	byRule   map[string]int
	failures []fixFailure
}

// fixOne fixes the text of one file in memory.
func (l *Linter) fixOne(r fileResult, project *Project, mode FixMode, only map[string]bool, maxPasses int) (*fixOutcome, error) {
	type pass struct {
		byRule map[string]int
		n      int
	}
	out := &fixOutcome{result: r, byRule: map[string]int{}}
	states := [][]byte{r.src} // states[k] is the text after k passes
	hashes := map[[32]byte]int{sha256.Sum256(r.src): 0}
	var passes []pass
	banned := map[string]bool{}
	// Fixing pays the baseline down: baselined findings are fixed too
	cur, errs := r.src, withBaselined(r)

	settled := false
	for n := 0; ; n++ {
		chosen, dropped := planFixes(cur, errs, mode, only, banned)
		for _, d := range dropped {
			l.debug("fix of %s at line %d is dropped in this pass because it conflicts with a fix of a rule of higher priority", d.id, d.line)
		}
		if len(chosen) == 0 {
			settled = true
			break
		}
		if n >= maxPasses {
			ids := ruleIDsOf(chosen)
			out.failures = append(out.failures, fixFailure{ids, fmt.Sprintf("fixing did not converge after %d passes, so the file was left as it was before the fixes (the last pass still had fixes)", maxPasses)})
			states, passes = states[:1], nil
			break
		}
		next, accepted, refused := applyVerified(cur, chosen)
		for id, why := range refused {
			banned[id] = true
			out.failures = append(out.failures, fixFailure{[]string{id}, why})
		}
		if len(accepted) == 0 {
			if len(refused) > 0 {
				// The fixes of the rules left out for overlapping a refused one are no longer blocked: plan
				// again. Every refusal bans a rule, so this ends, and it does not use up a pass.
				n--
				continue
			}
			settled = true
			break
		}
		p := pass{byRule: map[string]int{}, n: len(accepted)}
		for _, a := range accepted {
			p.byRule[a.id]++
		}
		passes = append(passes, p)
		h := sha256.Sum256(next)
		if first, ok := hashes[h]; ok {
			// The text came back: the fixes of passes first..n undo each other
			rules := map[string]bool{}
			for _, q := range passes[first:] {
				for id := range q.byRule {
					rules[id] = true
				}
			}
			ids := make([]string, 0, len(rules))
			for id := range rules {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			out.failures = append(out.failures, fixFailure{ids, fmt.Sprintf("the fixes undo each other: the text of pass %d came back after pass %d, so the file was left as it was before them", first, n+1)})
			states, passes = states[:first+1], passes[:first]
			break
		}
		hashes[h] = len(states)
		states = append(states, next)
		cur = next
		res, err := l.lintSource(r.file, cur, project)
		if err != nil {
			return nil, err
		}
		errs = withBaselined(res)
	}

	final := states[len(states)-1]
	if !settled || !bytes.Equal(final, cur) {
		// Rolled back to an earlier text
		res, err := l.lintSource(r.file, final, project)
		if err != nil {
			return nil, err
		}
		out.result = res
	} else if len(states) > 1 {
		out.result = newFileResult(r.file, r.path, cur, errs)
	}
	for _, p := range passes {
		out.n += p.n
		for id, c := range p.byRule {
			out.byRule[id] += c
		}
	}
	return out, nil
}

func ruleIDsOf(fixes []plannedFix) []string {
	seen := map[string]bool{}
	var ids []string
	for _, f := range fixes {
		if !seen[f.id] {
			seen[f.id] = true
			ids = append(ids, f.id)
		}
	}
	sort.Strings(ids)
	return ids
}

// lintSource lints the text of a file which is not necessarily the text on the disk.
func (l *Linter) lintSource(file string, src []byte, project *Project) (fileResult, error) {
	path := file
	if l.cwd != "" {
		if rel, err := filepath.Rel(l.cwd, file); err == nil {
			path = rel
		}
	}
	proc := newConcurrentProcess(runtime.NumCPU())
	dbg := l.debugWriter()
	localActions := NewLocalActionsCache(project, dbg)
	localReusableWorkflows := NewLocalReusableWorkflowCache(project, l.cwd, dbg)
	errs, err := l.check(path, src, project, proc, localActions, localReusableWorkflows)
	proc.wait()
	if err != nil {
		return fileResult{}, fmt.Errorf("fatal error while checking %s: %w", path, err)
	}
	return newFileResult(file, path, src, errs), nil
}

// configFor returns the configuration which applies to files of the project.
// The --profile option, when given, replaces the profile of the configuration.
func (l *Linter) configFor(project *Project) *Config {
	var cfg *Config
	switch {
	case l.defaultConfig != nil:
		cfg = l.defaultConfig
	case project != nil && project.Config() != nil:
		cfg = project.Config()
	default:
		cfg = l.globalConfig
	}
	if l.profile == "" {
		return cfg
	}
	if cfg == nil {
		return &Config{Profile: l.profile}
	}
	if cfg.Profile == l.profile {
		return cfg
	}
	if c, ok := l.profiled.Load(cfg); ok {
		return c.(*Config)
	}
	c := *cfg
	c.Profile = l.profile
	actual, _ := l.profiled.LoadOrStore(cfg, &c)
	return actual.(*Config)
}

// writeFileUnchanged writes the new text of the file unless the file changed since it was read.
func writeFileUnchanged(path string, old, new []byte) error {
	cur, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not write %q: %w", path, err)
	}
	if !bytes.Equal(cur, old) {
		return fmt.Errorf("could not write %q: the file changed while it was being fixed", path)
	}
	return writeFileKeepingMode(path, new)
}

// writeFileKeepingMode overwrites the file with the content atomically: it writes a temporary file in
// the same directory and renames it over the file, so a crash leaves the old or the new content and
// never half of it. It keeps the permission bits and writes the bytes as they are, so line breaks
// (CRLF) are kept. A symbolic link is followed, so the link stays a link.
func writeFileKeepingMode(path string, b []byte) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".jactionlint-fix-*")
	if err != nil {
		// Writing in place would truncate the file first, and a failure in the middle (a full disk) loses it
		return fmt.Errorf("could not write %q: no temporary file can be made in its directory, so the file was left as it was: %w", path, err)
	}
	name := tmp.Name()
	defer os.Remove(name) // No-op after the rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write %q: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not write %q: %w", path, err)
	}
	if err := os.Chmod(name, mode); err != nil {
		return fmt.Errorf("could not write %q: %w", path, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("could not write %q, which was left as it was: %w", path, err)
	}
	return nil
}

// withBaselined returns the errors of the result including the ones the baseline accepts, in the order
// of the file.
func withBaselined(r fileResult) []*Error {
	if len(r.baselined) == 0 {
		return r.errs
	}
	all := append(slices.Clone(r.errs), r.baselined...)
	slices.SortStableFunc(all, compareErrors)
	return all
}

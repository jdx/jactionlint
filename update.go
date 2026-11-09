package jactionlint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

// This file is `jactionlint update`: it reads the `uses:` values of the workflows and composite actions, finds the
// latest release of each external action or reusable workflow that the update policy allows, and reports, previews or
// writes the new references. It never changes anything unless UpdateApply is asked for.

// UpdateMode is what `jactionlint update` does with the updates it finds.
type UpdateMode int

const (
	// UpdateReport lists the updates and changes nothing.
	UpdateReport UpdateMode = iota
	// UpdateCheck lists them like UpdateReport. The command exits with status 1 when a reference violates the policy.
	UpdateCheck
	// UpdateDiff prints the changes as a unified diff and writes nothing.
	UpdateDiff
	// UpdateApply writes the changes.
	UpdateApply
)

// Rule IDs of the findings of `jactionlint update`. They are not rules of the linter: they name what the update found
// in the SARIF, JSON and text output.
const (
	// UpdateAvailableID is a reference that has a newer eligible release, or that the policy wants pinned.
	UpdateAvailableID = "update-available"
	// UpdateUnverifiedID is a reference whose version could not be verified or whose update could not be looked up.
	UpdateUnverifiedID = "update-unverified"
	// UpdatePolicyID is a reference that violates the update policy (its release is in the cooldown, is a prerelease,
	// or is not immutable) and for which no eligible replacement exists.
	UpdatePolicyID = "update-policy"
	// UpdateHeldID is a newer release that exists but that the policy does not allow yet. It is informational.
	UpdateHeldID = "update-held"
)

// UpdateOptions are the options of Linter.Update. A nil or empty field leaves the setting of the "update" mapping of the
// config file, which is what the command line options override.
type UpdateOptions struct {
	Mode UpdateMode
	// MinAge overrides "cooldown-days": the age in days a release must have. 0 turns the cooldown off.
	MinAge *int
	// Freeze overrides "freeze".
	Freeze *bool
	// PinOnly pins the referenced release to its commit without upgrading it. It implies Freeze.
	PinOnly bool
	// BranchToTag are regular expressions: a reference whose ref (a branch name) matches one is converted to the latest
	// eligible release.
	BranchToTag []string
	// IncludeTags and ExcludeTags override "include-tags" and "exclude-tags".
	IncludeTags, ExcludeTags []string
	// IncludeActions and ExcludeActions are regular expressions matched against "owner/repo[/subpath]". An action is
	// updated only when it matches one of the includes (if any) and none of the excludes.
	IncludeActions, ExcludeActions []string
	// ActionRules are "REGEX=ignore" or "REGEX=cooldown:DAYS". The first rule whose regular expression matches the value
	// of `uses:` followed by its comment ("actions/checkout@v4 # v4.1.0") applies.
	ActionRules []string

	// now is the clock, for tests.
	now func() time.Time
	// relock runs `gh actions-lock --relock` in a repository root, for tests. Nil runs gh.
	relock func(ctx context.Context, dir string) error
}

// UpdateResult is what Linter.Update did.
type UpdateResult struct {
	// Updates is the number of references that have an update (or a pin) to apply.
	Updates int
	// Violations is the number of references that violate the policy, or whose version could not be verified.
	Violations int
	// Files are the files that were changed (or would be changed).
	Files []string
	// Diff is the changes as a unified diff.
	Diff string
	// Errors are the findings, which were printed.
	Errors []*Error
}

type updateAction struct {
	re   *regexp.Regexp
	kind string // "ignore" or "cooldown"
	days int
}

// updatePolicy is the effective update settings for one file.
type updatePolicy struct {
	cooldownDays   int
	freeze         bool
	pinOnly        bool
	include        []*tagPattern
	exclude        []*tagPattern
	excludePre     bool
	requireImmut   bool
	branchToTag    []*regexp.Regexp
	includeActions []*regexp.Regexp
	excludeActions []*regexp.Regexp
	rules          []updateAction
}

func compileRegexps(what string, in []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, s := range in {
		re, err := regexp.Compile(s)
		if err != nil {
			return nil, &usageError{fmt.Errorf("invalid regular expression %q for %s: %w", s, what, err)}
		}
		out = append(out, re)
	}
	return out, nil
}

func compileTagPatterns(what string, in []string) ([]*tagPattern, error) {
	var out []*tagPattern
	for _, s := range in {
		p, err := compileTagPattern(s)
		if err != nil {
			return nil, &usageError{fmt.Errorf("invalid pattern %q for %s: %w", s, what, err)}
		}
		out = append(out, p)
	}
	return out, nil
}

// newUpdatePolicy merges the config file and the options, the options winning.
func newUpdatePolicy(cfg UpdateConfig, o *UpdateOptions) (*updatePolicy, error) {
	p := &updatePolicy{}
	if cfg.CooldownDays != nil {
		p.cooldownDays = *cfg.CooldownDays
	}
	if o.MinAge != nil {
		p.cooldownDays = *o.MinAge
	}
	if cfg.Freeze != nil {
		p.freeze = *cfg.Freeze
	}
	if o.Freeze != nil {
		p.freeze = *o.Freeze
	}
	if o.PinOnly {
		p.freeze, p.pinOnly = true, true
	}
	if cfg.ExcludePrereleases != nil {
		p.excludePre = *cfg.ExcludePrereleases
	}
	if cfg.RequireImmutableReleases != nil {
		p.requireImmut = *cfg.RequireImmutableReleases
	}
	for _, r := range o.ActionRules {
		if _, a, ok := cutLast(r, "="); ok && strings.HasPrefix(a, "cooldown:") {
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "cooldown:")); err == nil && n > maxCooldownDays {
				return nil, &usageError{fmt.Errorf("invalid --action-rule %q: the cooldown is at most %d days", r, maxCooldownDays)}
			}
		}
	}
	if p.cooldownDays > maxCooldownDays {
		return nil, &usageError{fmt.Errorf("the cooldown of %d days is more than the %d days that can be compared", p.cooldownDays, maxCooldownDays)}
	}
	inc, exc := []string(cfg.IncludeTags), []string(cfg.ExcludeTags)
	if o.IncludeTags != nil {
		inc = o.IncludeTags
	}
	if o.ExcludeTags != nil {
		exc = o.ExcludeTags
	}
	var err error
	if p.include, err = compileTagPatterns("--include-tag", inc); err != nil {
		return nil, err
	}
	if p.exclude, err = compileTagPatterns("--exclude-tag", exc); err != nil {
		return nil, err
	}
	if p.branchToTag, err = compileRegexps("--branch-to-tag", o.BranchToTag); err != nil {
		return nil, err
	}
	if p.includeActions, err = compileRegexps("--include-action", o.IncludeActions); err != nil {
		return nil, err
	}
	if p.excludeActions, err = compileRegexps("--exclude-action", o.ExcludeActions); err != nil {
		return nil, err
	}
	for _, r := range o.ActionRules {
		pat, act, ok := cutLast(r, "=")
		if !ok {
			return nil, &usageError{fmt.Errorf("invalid --action-rule %q: write REGEX=ignore or REGEX=cooldown:DAYS", r)}
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, &usageError{fmt.Errorf("invalid regular expression in --action-rule %q: %w", r, err)}
		}
		rule := updateAction{re: re}
		switch {
		case act == "ignore":
			rule.kind = "ignore"
		case strings.HasPrefix(act, "cooldown:"):
			n, err := strconv.Atoi(strings.TrimPrefix(act, "cooldown:"))
			if err != nil || n < 0 {
				return nil, &usageError{fmt.Errorf("invalid --action-rule %q: the cooldown is a number of days, 0 or more", r)}
			}
			rule.kind, rule.days = "cooldown", n
		default:
			return nil, &usageError{fmt.Errorf("invalid --action-rule %q: the action is ignore or cooldown:DAYS", r)}
		}
		p.rules = append(p.rules, rule)
	}
	return p, nil
}

func cutLast(s, sep string) (before, after string, ok bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func anyMatch(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func (p *updatePolicy) tagAllowed(tag string) bool {
	for _, e := range p.exclude {
		if e.match(tag) {
			return false
		}
	}
	if len(p.include) == 0 {
		return true
	}
	for _, i := range p.include {
		if i.match(tag) {
			return true
		}
	}
	return false
}

// updateSite is a `uses:` value in a file.
type updateSite struct {
	line, col int
	lineStart int
	v         usesValueOnLine
	ref       *UsesRef
	// comment is the text after the "#" that follows the value, and commentAt the byte offset of that text in the file.
	comment   string
	commentAt int
	hasHash   bool
	// unscanned is a reference that cannot be edited in place: a block scalar, a value over several lines or with escapes.
	unscanned bool
}

// updateEdit is the change for one site: the text that replaces the ref (and the comment).
type updateEdit struct {
	edits []TextEdit
}

type updateOutcome struct {
	id    string
	level Severity
	msg   string
	edit  *updateEdit
}

// maxCooldownDays is the longest cooldown: 100 years, far below the point where a duration in nanoseconds overflows.
const maxCooldownDays = 36500

// maxUpdateCandidates bounds how many releases of one action are looked at for one reference. Each costs requests.
const maxUpdateCandidates = 30

// Update checks the external actions and reusable workflows of the files (the repository when there are none), reports
// the updates and, with UpdateApply, writes them. The online checks must be on: the releases come from GitHub.
func (l *Linter) Update(paths []string, uo UpdateOptions) (*UpdateResult, error) {
	now := uo.now
	if now == nil {
		now = time.Now
	}
	var proj *Project
	files := paths
	if len(files) == 0 {
		var err error
		files, proj, err = l.updateRepositoryFiles(l.cwd)
		if err != nil {
			return nil, err
		}
	}

	files = dedupeFiles(files)
	res := &UpdateResult{}
	var results []fileResult
	type fileWork struct {
		idx    int
		src    []byte
		edits  map[*Error]*updateEdit
		errs   []*Error
		policy *updatePolicy
	}
	var work []*fileWork
	var roots []string
	if proj != nil {
		roots = append(roots, proj.RootDir())
	}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("could not read %q: %w", file, err)
		}
		p := proj
		if p == nil {
			if p, err = l.projects.At(file); err != nil {
				return nil, err
			}
		}
		if p != nil {
			roots = append(roots, p.RootDir())
		}
		cfg := l.configFor(p)
		var ucfg UpdateConfig
		if cfg != nil {
			ucfg = cfg.Update
		}
		pol, err := newUpdatePolicy(ucfg, &uo)
		if err != nil {
			return nil, err
		}
		sess, serr := l.onlineSession(cfg)
		if serr != nil {
			return nil, serr
		}
		if sess == nil {
			return nil, &usageError{errors.New("update reads the releases from GitHub, so the online checks must be on (do not pass --no-online)")}
		}
		locked := p != nil && l.lockListsWorkflow(p, file)
		u := &updater{sess: sess, pol: pol, now: now, locked: locked, debug: l.debug}
		sites, err := findUpdateSites(src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		fw := &fileWork{idx: len(results), src: src, edits: map[*Error]*updateEdit{}, policy: pol}
		for _, s := range sites {
			out := u.analyze(src, s)
			if out == nil {
				continue
			}
			e := &Error{
				Message: out.msg, Filepath: file, Line: s.line, Column: s.col, Kind: out.id, ID: out.id, Severity: out.level,
			}
			end := s.col + utf8Len(s.v.spec)
			e.EndLine, e.EndColumn = s.line, end
			fw.errs = append(fw.errs, e)
			if out.edit != nil {
				fw.edits[e] = out.edit
			}
		}
		results = append(results, fileResult{file: file, path: file, src: src, errs: fw.errs})
		work = append(work, fw)
	}

	results = l.filterChanged(results)
	if err := l.changedError(); err != nil {
		return nil, err // an unusable --changed-since must not widen what is updated
	}

	var diff strings.Builder
	for _, fw := range work {
		r := &results[fw.idx]
		var edits []TextEdit
		for _, e := range r.errs {
			switch e.ID {
			case UpdateAvailableID:
				res.Updates++
			case UpdateUnverifiedID, UpdatePolicyID:
				res.Violations++
			}
			if ed := fw.edits[e]; ed != nil {
				edits = append(edits, ed.edits...)
			}
		}
		if len(edits) == 0 {
			continue
		}
		next, err := applyUpdateEdits(fw.src, edits)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.file, err)
		}
		var probe yaml.Node
		if err := yaml.Unmarshal(next, &probe); err != nil {
			return nil, fmt.Errorf("%s: the update would produce invalid YAML, nothing was written: %w", r.file, err)
		}
		name := filepath.ToSlash(r.path)
		diff.WriteString(unifiedDiff("a/"+name, "b/"+name, fw.src, next))
		res.Files = append(res.Files, r.file)
		if uo.Mode == UpdateApply {
			if err := writeFileUnchanged(r.file, fw.src, next); err != nil {
				return nil, err
			}
		}
	}
	res.Diff = diff.String()
	for _, r := range results {
		res.Errors = append(res.Errors, r.errs...)
	}

	out := l.out
	if uo.Mode == UpdateDiff {
		if _, err := fmt.Fprint(l.out, res.Diff); err != nil {
			return nil, err
		}
		out = l.logOut // stdout has the diff
	}
	// --min-severity hides findings from the output only: the edits and the counts above include them
	shown := make([]fileResult, len(results))
	for i, r := range results {
		shown[i] = r
		shown[i].errs = nil
		for _, e := range r.errs {
			if e.Severity >= l.minSeverity {
				shown[i].errs = append(shown[i].errs, e)
			}
		}
	}
	if err := l.printer.print(out, shown, l.runInfo()); err != nil {
		return nil, err
	}
	l.reportChangedNote()
	l.printUpdateSummary(res, uo.Mode)
	if uo.Mode == UpdateApply {
		if err := l.relock(roots, uo); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (l *Linter) printUpdateSummary(res *UpdateResult, mode UpdateMode) {
	w := l.logOut
	switch {
	case res.Updates == 0 && res.Violations == 0:
		fmt.Fprintln(w, "All references are up to date")
	case mode == UpdateApply:
		fmt.Fprintf(w, "Updated %s in %s\n", countNoun(res.Updates, "reference"), countNoun(len(res.Files), "file"))
	case mode == UpdateDiff:
		fmt.Fprintf(w, "Would update %s in %s\n", countNoun(res.Updates, "reference"), countNoun(len(res.Files), "file"))
	default:
		if res.Updates > 0 {
			fmt.Fprintf(w, "%s can be updated; run jactionlint update --diff to preview and --apply to write\n", countNoun(res.Updates, "reference"))
		}
	}
	if res.Violations > 0 {
		fmt.Fprintf(w, "%s violate the update policy or could not be verified\n", countNoun(res.Violations, "reference"))
	}
}

// updateRepositoryFiles lists the workflows and composite actions of the project.
func (l *Linter) updateRepositoryFiles(dir string) ([]string, *Project, error) {
	p, err := l.projects.At(dir)
	if err != nil {
		return nil, nil, err
	}
	if p == nil {
		return nil, nil, fmt.Errorf("no project was found in any parent directories of %q. check workflows directory is put correctly in your Git repository", dir)
	}
	l.log("Detected project:", p.RootDir())
	files, err := projectWorkflowFiles(p.WorkflowsDir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	files = append(files, l.callGraphOf(p).actionPaths()...)
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("no workflow or action file was found in %q", p.RootDir())
	}
	return files, p, nil
}

func utf8Len(s string) int { return len([]rune(s)) }

func applyUpdateEdits(src []byte, edits []TextEdit) ([]byte, error) {
	sorted := slices.Clone(edits)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start > sorted[j].Start })
	out := bytes.Clone(src)
	prev := len(src) + 1
	for _, e := range sorted {
		if e.End > prev || e.Start > e.End || e.End > len(out) {
			return nil, errors.New("the edits overlap")
		}
		out = append(out[:e.Start], append([]byte(e.NewText), out[e.End:]...)...)
		prev = e.Start
	}
	return out, nil
}

// findUpdateSites returns the `uses:` of the jobs (reusable workflows) and of the steps of jobs and composite actions. A
// `uses` key anywhere else, such as an input in `with:`, is data and is not touched.
func findUpdateSites(src []byte) ([]updateSite, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return nil, fmt.Errorf("could not parse the YAML: %w", err)
	}
	starts := lineStarts(src)
	var sites []updateSite
	value := func(n *yaml.Node, key string) *yaml.Node {
		if n == nil || n.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				return n.Content[i+1]
			}
		}
		return nil
	}
	add := func(v *yaml.Node) {
		if v == nil || v.Kind != yaml.ScalarNode || v.Line < 1 || v.Line > len(starts) {
			return
		}
		s, ok := newUpdateSite(src, starts[v.Line-1], v.Line, v.Column)
		if ok && s.v.spec == v.Value && v.Style&(yaml.LiteralStyle|yaml.FoldedStyle) == 0 {
			sites = append(sites, s)
			return
		}
		// Written over several lines, as a block scalar, or with an escape: the reference is real but the line cannot be edited
		sites = append(sites, updateSite{line: v.Line, col: v.Column, unscanned: true, v: usesValueOnLine{spec: v.Value}, ref: ParseUses(strings.TrimSpace(v.Value))})
	}
	steps := func(seq *yaml.Node) {
		if seq == nil || seq.Kind != yaml.SequenceNode {
			return
		}
		for _, step := range seq.Content {
			add(value(step, "uses"))
		}
	}
	n := &root
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if jobs := value(n, "jobs"); jobs != nil && jobs.Kind == yaml.MappingNode {
		for i := 1; i < len(jobs.Content); i += 2 {
			add(value(jobs.Content[i], "uses"))
			steps(value(jobs.Content[i], "steps"))
		}
	}
	steps(value(value(n, "runs"), "steps"))
	return sites, nil
}

func newUpdateSite(src []byte, lineStart, line, col int) (updateSite, bool) {
	v, err := scanUsesValue(src, lineStart, col)
	if err != nil {
		return updateSite{}, false
	}
	s := updateSite{line: line, col: col, lineStart: lineStart, v: v, ref: ParseUses(v.spec)}
	rest := string(v.line[v.tokenEnd:])
	trimmed := strings.TrimLeft(rest, " \t")
	if strings.HasPrefix(trimmed, "#") {
		s.hasHash = true
		s.comment = trimmed[1:]
		s.commentAt = lineStart + v.tokenEnd + (len(rest) - len(trimmed)) + 1
	}
	return s, true
}

// updater holds what the analysis of the references of one file needs.
type updater struct {
	sess   *onlineSession
	pol    *updatePolicy
	now    func() time.Time
	locked bool
	debug  func(format string, args ...any)
}

// analyze returns what the update has to say about the reference, or nil for a value it does not handle or that is up
// to date.
func (u *updater) analyze(src []byte, s updateSite) *updateOutcome {
	ref := s.ref
	if ref.Dynamic || (ref.Kind != UsesAction && ref.Kind != UsesReusableWorkflow) || !ref.HasRef {
		return nil
	}
	if !validGitHubOwner(ref.Owner) || !validGitHubRepo(ref.Repo) || !validGitRefName(ref.Ref) {
		return nil
	}
	name := ref.Owner + "/" + ref.Repo
	if sub := strings.Trim(ref.Subpath, "/"); sub != "" {
		name += "/" + sub
	}
	if len(u.pol.includeActions) > 0 && !anyMatch(u.pol.includeActions, name) {
		return nil
	}
	if anyMatch(u.pol.excludeActions, name) {
		return nil
	}
	cooldown := u.pol.cooldownDays
	subject := s.v.spec
	if c := strings.TrimSpace(s.comment); c != "" {
		subject += " # " + c
	}
	for _, r := range u.pol.rules {
		if !r.re.MatchString(subject) {
			continue
		}
		if r.kind == "ignore" {
			return nil
		}
		cooldown = r.days
		break
	}
	if s.unscanned {
		return &updateOutcome{id: UpdateUnverifiedID, level: SeverityWarning,
			msg: fmt.Sprintf("%s: the value is not a plain or quoted scalar on one line, so it was not checked: check it by hand", name)}
	}

	out, err := u.plan(src, s, name, cooldown)
	if err != nil {
		if errors.Is(err, errOnlineExcluded) {
			return nil
		}
		return &updateOutcome{id: UpdateUnverifiedID, level: SeverityWarning, msg: fmt.Sprintf("%s: %s", name, updateReason(err))}
	}
	return out
}

func updateReason(err error) string {
	var ue *updateReasonError
	if errors.As(err, &ue) {
		return ue.msg
	}
	return "could not be looked up on GitHub: " + err.Error()
}

// updateReasonError is a reason that an update cannot be decided, worded for the user.
type updateReasonError struct{ msg string }

func (e *updateReasonError) Error() string { return e.msg }

func reasonf(format string, args ...any) error {
	return &updateReasonError{fmt.Sprintf(format, args...)}
}

// selection is the release the update chose.
type selection struct {
	tag string
	sha string
	ver advisoryVersion
}

// plan decides the update of one reference. The error is why it cannot be decided; it is reported as unverified.
func (u *updater) plan(src []byte, s updateSite, name string, cooldown int) (*updateOutcome, error) {
	ref := s.ref
	owner, repo := ref.Owner, ref.Repo
	idx, err := u.sess.Tags(owner, repo)
	if err != nil {
		return nil, err
	}

	var (
		cur      *advisoryVersion // the version the reference is at, nil when it is a branch to convert
		curTag   string           // the tag of that version
		curSHA   string           // the commit it is at
		pinned   bool             // the ref is a commit SHA
		curGran  int              // number of components the reference is written with
		wantV    = true
		verified bool
	)
	commentVer, _, _, hasCommentVer := commentVersionAt(s.comment, ref)

	switch {
	case ref.RefKind == RefFullSHA && len(ref.Ref) == 40:
		pinned = true
		curSHA = strings.ToLower(ref.Ref)
		names := idx.bySHA[curSHA]
		if hasCommentVer {
			if tagIn(names, commentVer) || partialIn(names, commentVer) {
				verified = true
			} else if idx.truncated {
				if sha, isTag, err := u.sess.TagCommit(owner, repo, commentVer); err == nil && isTag && strings.EqualFold(sha, curSHA) {
					verified = true
					names = append(slices.Clone(names), commentVer) // the list is cut short and lacks the tag that was just resolved
				}
			}
			if !verified {
				return &updateOutcome{id: UpdateUnverifiedID, level: SeverityWarning,
					msg: fmt.Sprintf("%s: the commit %s is not the commit of the version in its comment (%s), so it was not updated", name, shortSHA(curSHA), commentVer)}, nil
			}
		}
		best, ok := bestStableTag(names, commentVer)
		if !ok {
			return &updateOutcome{id: UpdateUnverifiedID, level: SeverityWarning,
				msg: fmt.Sprintf("%s: no release tag points at the commit %s, so its version is unknown and it was not updated", name, shortSHA(curSHA))}, nil
		}
		curTag = best
		v, _ := parseAdvisoryVersion(best)
		cur = &v
		curGran = len(v.nums)
		wantV = hasVPrefix(best)
	case ref.RefKind == RefSemverTag:
		v, ok := parseAdvisoryVersion(ref.Ref)
		if !ok || !semverTagRegex.MatchString(ref.Ref) {
			return nil, nil // v1.2.3-rc.1 and other tags that are not stable versions are left alone
		}
		if v.pre != "" && !u.pol.excludePre {
			return nil, nil // a prerelease is left alone unless the policy excludes prereleases: then it is updated like any version
		}
		sha, found := idx.byName[ref.Ref]
		if !found {
			if idx.truncated {
				var isTag bool
				if sha, isTag, err = u.sess.TagCommit(owner, repo, ref.Ref); err != nil {
					return nil, err
				}
				found = isTag
			}
			if !found {
				return &updateOutcome{id: UpdateUnverifiedID, level: SeverityWarning,
					msg: fmt.Sprintf("%s: %s is not a tag of the repository, so it was not updated", name, ref.Ref)}, nil
			}
		}
		cur, curTag, curSHA, curGran, wantV = &v, ref.Ref, strings.ToLower(sha), len(v.nums), hasVPrefix(ref.Ref)
	default:
		// A branch, or another name that is not a version
		if !anyMatch(u.pol.branchToTag, ref.Ref) || ref.RefKind == RefShortSHA {
			return nil, nil
		}
	}

	var sel *selection
	var held []string
	if !u.pol.pinOnly {
		sel, held, err = u.latestEligible(idx, owner, repo, cur, isMovingAlias(idx, curSHA, cur), cooldown)
		if err != nil {
			return nil, err
		}
	}

	if sel == nil && cur == nil {
		msg := fmt.Sprintf("%s: no eligible release to replace the branch %s", name, ref.Ref)
		if len(held) > 0 {
			msg += " (" + strings.Join(held, "; ") + ")"
		}
		return &updateOutcome{id: UpdateUnverifiedID, level: SeverityWarning, msg: msg}, nil
	}

	pin := u.pol.freeze || pinned
	var target selection
	switch {
	case sel != nil:
		target = *sel
	default:
		target = selection{tag: curTag, sha: curSHA, ver: *cur}
	}

	var heldNote *updateOutcome
	if sel == nil && len(held) > 0 {
		heldNote = &updateOutcome{id: UpdateHeldID, level: SeverityInfo, msg: fmt.Sprintf("%s: a newer release exists but is not eligible yet (%s)", name, strings.Join(held, "; "))}
	}

	if sel == nil {
		// Nothing newer. The reference stays unless it is to be pinned, or is in violation.
		if cur != nil {
			polTag := curTag
			if best, ok := bestStableTag(idx.bySHA[curSHA], ""); ok && isMovingAlias(idx, curSHA, cur) {
				polTag = best // a moving tag (v1) has no release of its own: the policy applies to the release it points at
			}
			why, err := u.violations(owner, repo, polTag, curSHA, cooldown)
			if err != nil {
				return nil, err
			}
			if why != "" {
				return &updateOutcome{id: UpdatePolicyID, level: SeverityWarning, msg: fmt.Sprintf("%s: %s %s", name, curTag, why)}, nil
			}
		}
		if !u.pol.freeze || (pinned && hasCommentVer) {
			return heldNote, nil
		}
	}

	// The reference text to write
	var newRef, newVer string
	if pin {
		newRef, newVer = target.sha, target.tag
		if sel == nil && !pinned {
			// Pin the release that is referenced: name its most specific version, as the pin fix does
			newVer = pinCommentVersion(u.sess, ref, target.sha, false)
		}
	} else {
		newRef = u.refSpelling(idx, target, curGran, wantV)
		newVer = newRef
	}
	if strings.EqualFold(newRef, ref.Ref) && !(pin && !hasCommentVer) {
		return heldNote, nil
	}
	if cur != nil && sel != nil && target.ver.compare(*cur) < 0 {
		return heldNote, nil // never a downgrade
	}

	if u.locked && ref.Kind == UsesAction {
		return &updateOutcome{id: UpdateUnverifiedID, level: SeverityWarning,
			msg: fmt.Sprintf("%s: %s can be updated, but the workflow is listed in %s, which pins the version: add the entry with `gh actions-lock` instead", name, ref.Ref, actionsLockPath)}, nil
	}

	edit, err := buildUpdateEdit(s, newRef, newVer, pin, pinned, cur != nil)
	if err != nil {
		return &updateOutcome{id: UpdateUnverifiedID, level: SeverityWarning, msg: fmt.Sprintf("%s: %s", name, err.Error())}, nil
	}
	from := ref.Ref
	if pinned {
		from = curTag
	}
	var msg string
	switch {
	case sel != nil:
		msg = fmt.Sprintf("%s can be updated from %s to %s", name, from, target.tag)
		if pin {
			msg += " (" + shortSHA(target.sha) + ")"
		}
	case pinned:
		msg = fmt.Sprintf("%s@%s has no version comment: add # %s", name, shortSHA(target.sha), newVer)
	case pin:
		msg = fmt.Sprintf("%s@%s can be pinned to %s", name, from, shortSHA(target.sha))
	}
	return &updateOutcome{id: UpdateAvailableID, level: SeverityWarning, msg: msg, edit: edit}, nil
}

func tagIn(names []string, v string) bool { return slices.Contains(names, v) }

// partialIn reports whether the comment names a less specific version ("v4") of one of the tags of the commit ("v4.2.2").
func partialIn(names []string, v string) bool {
	cv, ok := parseAdvisoryVersion(v)
	if !ok {
		return false
	}
	for _, n := range names {
		if nv, ok := parseAdvisoryVersion(n); ok && nv.pre == "" && sameLeadingNumbers(nv, cv) {
			return true
		}
	}
	return false
}

// bestStableTag picks the most specific stable version tag of the commit that agrees with the version in the comment.
func bestStableTag(names []string, commentVer string) (string, bool) {
	var stable []string
	for _, n := range names {
		if v, ok := parseAdvisoryVersion(n); ok && v.pre == "" && semverTagRegex.MatchString(n) {
			stable = append(stable, n)
		}
	}
	if len(stable) == 0 {
		return "", false
	}
	if t, ok := specificVersionTag(stable, commentVer); ok {
		return t, true
	}
	// Two versions of the same specificity, or a comment that is no version: the highest
	best := stable[0]
	bv, _ := parseAdvisoryVersion(best)
	for _, n := range stable[1:] {
		if v, _ := parseAdvisoryVersion(n); len(v.nums) > len(bv.nums) || (len(v.nums) == len(bv.nums) && v.compare(bv) > 0) {
			best, bv = n, v
		}
	}
	return best, true
}

// refSpelling returns the ref to write for an unpinned reference: the alias with the granularity of the current ref (v5 for
// v4) when that tag exists and points at the chosen commit, else the full tag.
func (u *updater) refSpelling(idx *tagIndex, t selection, gran int, wantV bool) string {
	if gran > 0 && gran < len(t.ver.nums) {
		parts := make([]string, gran)
		for i := 0; i < gran; i++ {
			parts[i] = strconv.Itoa(t.ver.nums[i])
		}
		alias := strings.Join(parts, ".")
		if wantV {
			alias = "v" + alias
		}
		// A moving tag has no immutable release of its own, and it must pass the tag filters like the release does
		if sha, ok := idx.byName[alias]; ok && strings.EqualFold(sha, t.sha) && !u.pol.requireImmut && u.pol.tagAllowed(alias) {
			return alias
		}
	}
	return t.tag
}

// latestEligible finds the highest stable release newer than cur that the policy allows. held lists why newer releases
// were not eligible. cur is nil for a branch, then every release is newer.
func (u *updater) latestEligible(idx *tagIndex, owner, repo string, cur *advisoryVersion, curAlias bool, cooldown int) (*selection, []string, error) {
	type cand struct {
		tag string
		sha string
		ver advisoryVersion
	}
	var cands []cand
	for name, sha := range idx.byName {
		v, ok := parseAdvisoryVersion(name)
		if !ok || v.pre != "" || !semverTagRegex.MatchString(name) || strings.Contains(name, "+") {
			continue
		}
		if !u.pol.tagAllowed(name) {
			continue
		}
		cands = append(cands, cand{name, sha, v})
	}
	// A moving tag (v4) points at the commit of a more specific tag (v4.2.2) and is not a release of its own
	alias := func(c cand) bool {
		for _, o := range idx.bySHA[c.sha] {
			ov, ok := parseAdvisoryVersion(o)
			if ok && ov.pre == "" && len(ov.nums) > len(c.ver.nums) && sameLeadingNumbers(ov, c.ver) {
				return true
			}
		}
		return false
	}
	var real []cand
	for _, c := range cands {
		if !alias(c) {
			real = append(real, c)
		}
	}
	sort.Slice(real, func(i, j int) bool {
		if d := real[i].ver.compare(real[j].ver); d != 0 {
			return d > 0
		}
		return real[i].tag < real[j].tag
	})

	newer := func(c cand) bool {
		if cur == nil {
			return true
		}
		if curAlias && len(cur.nums) < len(c.ver.nums) {
			// The current ref is a moving tag (v4): a release is newer when a leading number is higher
			trunc := advisoryVersion{nums: c.ver.nums[:len(cur.nums)]}
			return trunc.compare(*cur) > 0
		}
		return c.ver.compare(*cur) > 0
	}

	var held []string
	examined := 0
	for _, c := range real {
		if !newer(c) {
			break
		}
		if examined >= maxUpdateCandidates {
			return nil, nil, reasonf("none of the %d newest releases is eligible and the rest were not looked at, so whether an update exists is unknown", maxUpdateCandidates)
		}
		examined++
		why, err := u.eligible(owner, repo, c.tag, c.sha, cooldown)
		if err != nil {
			return nil, nil, err
		}
		if why != "" {
			if len(held) < 3 {
				held = append(held, c.tag+" "+why)
			}
			continue
		}
		return &selection{tag: c.tag, sha: strings.ToLower(c.sha), ver: c.ver}, nil, nil
	}
	return nil, held, nil
}

// eligible checks the release metadata and the cooldown of a tag. It returns why the release is not eligible, or "". The
// error says the policy could not be checked.
func (u *updater) eligible(owner, repo, tag, sha string, cooldown int) (string, error) {
	if u.pol.excludePre || u.pol.requireImmut {
		rel, err := u.sess.Release(owner, repo, tag)
		switch {
		case err == nil:
			if u.pol.excludePre && rel.Prerelease {
				return "is a prerelease", nil
			}
			if u.pol.requireImmut && !rel.Immutable {
				return "is not an immutable release", nil
			}
		case errors.Is(err, ErrGitHubNotFound):
			if u.pol.requireImmut {
				return "has no GitHub release", nil
			}
		case errors.Is(err, errNoReleaseLookup):
			return "", reasonf("the GitHub client cannot look up releases, so %s cannot be checked against the release policy", tag)
		default:
			return "", err
		}
	}
	if cooldown > 0 {
		age, err := u.age(owner, repo, tag, sha)
		if err != nil {
			return "", err
		}
		if age < time.Duration(cooldown)*24*time.Hour {
			return fmt.Sprintf("was released %s ago (cooldown %d days)", ageWords(age), cooldown), nil
		}
	}
	return "", nil
}

func (u *updater) age(owner, repo, tag, sha string) (time.Duration, error) {
	d, err := u.sess.ReleaseDate(owner, repo, tag, sha)
	if err != nil {
		return 0, err
	}
	t, _, ok := d.Time()
	if !ok {
		return 0, reasonf("the date of %s is not known, so its age cannot be checked", tag)
	}
	return u.now().Sub(t), nil
}

func ageWords(d time.Duration) string {
	if days := int(d.Hours() / 24); days >= 1 {
		return unitCount(days, "day")
	}
	return unitCount(int(d.Hours()), "hour")
}

func unitCount(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// violations says how the release the reference is at breaks the policy, or "". It checks only what is known from GitHub.
func (u *updater) violations(owner, repo, tag, sha string, cooldown int) (string, error) {
	var why []string
	if u.pol.excludePre || u.pol.requireImmut {
		rel, err := u.sess.Release(owner, repo, tag)
		switch {
		case err == nil:
			if u.pol.excludePre && rel.Prerelease {
				why = append(why, "is a prerelease")
			}
			if u.pol.requireImmut && !rel.Immutable {
				why = append(why, "is not an immutable release")
			}
		case errors.Is(err, ErrGitHubNotFound):
			if u.pol.requireImmut {
				why = append(why, "has no GitHub release")
			}
		case errors.Is(err, errNoReleaseLookup):
			return "", reasonf("the GitHub client cannot look up releases, so %s cannot be checked against the release policy", tag)
		default:
			return "", err
		}
	}
	if cooldown > 0 {
		age, err := u.age(owner, repo, tag, sha)
		if err != nil {
			return "", err
		}
		if age < time.Duration(cooldown)*24*time.Hour {
			why = append(why, fmt.Sprintf("was released %s ago (cooldown %d days)", ageWords(age), cooldown))
		}
	}
	if len(why) == 0 {
		return "", nil
	}
	return strings.Join(why, " and ") + ", and no eligible release replaces it", nil
}

// buildUpdateEdit builds the edit of the ref and its version comment.
func buildUpdateEdit(s updateSite, newRef, newVer string, pin, wasPinned, hadVersion bool) (*updateEdit, error) {
	v := s.v
	refStart := s.lineStart + v.valueStart + strings.IndexByte(v.spec, '@') + 1
	refEnd := s.lineStart + v.valueEnd
	text := newRef
	restTrim := strings.TrimSpace(string(v.line[v.tokenEnd:]))

	if !pin {
		// An unpinned ref: the comment is only changed when it names the old ref
		if restTrim != "" && strings.HasPrefix(restTrim, "#") {
			if cv, cs, ce, ok := commentVersionAt(s.comment, s.ref); ok && !commentStillTrue(cv, newVer) {
				return &updateEdit{edits: []TextEdit{
					{Start: refStart, End: refEnd, NewText: text},
					{Start: s.commentAt + cs, End: s.commentAt + ce, NewText: newVer},
				}}, nil
			}
		}
		return &updateEdit{edits: []TextEdit{{Start: refStart, End: refEnd, NewText: text}}}, nil
	}

	switch {
	case restTrim == "":
		// No comment: name the version in one, after the closing quote, and drop the white space at the end of the line
		refEnd = s.lineStart + len(v.line)
		if v.quote != 0 {
			text += string(v.quote)
		}
		text += " # " + newVer
		return &updateEdit{edits: []TextEdit{{Start: refStart, End: refEnd, NewText: text}}}, nil
	case strings.HasPrefix(restTrim, "#"):
		cv, cs, ce, ok := commentVersionAt(s.comment, s.ref)
		if !ok {
			if hadVersion {
				return nil, errors.New("the comment on the line does not name a version, so the update was not written: change the line by hand")
			}
			return nil, errors.New("the comment on the line does not name a version, so the branch was not converted: change the line by hand")
		}
		edits := []TextEdit{{Start: refStart, End: refEnd, NewText: text}}
		if !commentStillTrue(cv, newVer) {
			edits = append(edits, TextEdit{Start: s.commentAt + cs, End: s.commentAt + ce, NewText: newVer})
		}
		return &updateEdit{edits: edits}, nil
	}
	return nil, errors.New("the line has content after the value that is not a comment, so the update was not written")
}

// commentStillTrue reports whether the version in the comment (v4) is a less specific spelling of the new version (v4.3.0),
// which is then kept: the comment stays true after the update.
func commentStillTrue(comment, newVer string) bool {
	cv, ok1 := parseAdvisoryVersion(comment)
	nv, ok2 := parseAdvisoryVersion(newVer)
	if !ok1 || !ok2 || cv.pre != "" || nv.pre != "" {
		return false
	}
	return len(cv.nums) <= len(nv.nums) && sameLeadingNumbers(nv, cv) && hasVPrefix(comment) == hasVPrefix(newVer)
}

// isMovingAlias reports whether the version is a less specific tag (v4) of a more specific one (v4.2.2) at the same commit, which
// is how a moving tag looks. Fixed releases at different commits (v1.2 and v1.2.1) are not aliases.
func isMovingAlias(idx *tagIndex, sha string, v *advisoryVersion) bool {
	if v == nil {
		return false
	}
	for _, o := range idx.bySHA[sha] {
		ov, ok := parseAdvisoryVersion(o)
		if ok && ov.pre == "" && len(ov.nums) > len(v.nums) && sameLeadingNumbers(ov, *v) {
			return true
		}
	}
	return false
}

// dedupeFiles drops the files that are named twice, however they are spelled, so that each is planned and written once.
func dedupeFiles(files []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(files))
	for _, f := range files {
		key := f
		if abs, err := filepath.Abs(f); err == nil {
			key = abs
		}
		if real, err := filepath.EvalSymlinks(key); err == nil {
			key = real
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}

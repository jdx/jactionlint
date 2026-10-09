package jactionlint

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// linterBaseline is the baseline settings and state of a Linter.
type linterBaseline struct {
	// on applies the baseline even when the config does not ask for it. file is its path, resolved
	// against the working directory, or empty for the default file of the repository.
	on   bool
	file string
	// off ignores the baseline even when the config asks for it.
	off bool
	// check reports the entries which match nothing as unused-baseline-entry.
	check bool
	// hideInSARIF leaves the baselined findings out of the SARIF log instead of marking them suppressed.
	hideInSARIF bool
	// writing is set while WriteBaseline lints. Nothing is applied then, so that every finding is seen.
	writing bool

	// infos holds the identity of the findings while WriteBaseline lints: *Error to *baselineInfo.
	infos sync.Map

	mu     sync.Mutex
	states map[string]*baselineState
}

// baselineNamesAFile reports whether the configuration gives the path of a baseline file, which
// -baseline-check then checks, instead of a switch (auto, true, false) or nothing.
func baselineNamesAFile(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Baseline)) {
	case "", "false", "off", "no", "auto", "true", "yes":
		return false
	}
	return true
}

// baselineConfigured reports whether the configuration applies a baseline to every run.
func baselineConfigured(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Baseline)) {
	case "", "false", "off", "no":
		return false
	}
	return true
}

// resolveBaselineSetting returns the path of the baseline to apply and whether it must exist. An
// empty path means no baseline.
func (l *Linter) resolveBaselineSetting(root string, cfg *Config) (path string, required bool) {
	b := &l.baseline
	if b.off || b.writing {
		return "", false
	}
	if b.on || (b.check && !baselineNamesAFile(cfg)) {
		if b.file != "" {
			return b.file, true
		}
		return filepath.Join(root, DefaultBaselineFile), true
	}
	if cfg == nil {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Baseline)) {
	case "", "false", "off", "no":
		return "", false
	case "auto", "true", "yes":
		return filepath.Join(root, DefaultBaselineFile), false
	}
	p := cfg.Baseline
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p, true
}

// baselineFor returns the baseline to apply to the files of the project, or nil when there is none.
// The baseline file is read once.
func (l *Linter) baselineFor(project *Project, cfg *Config) (*baselineState, error) {
	root := l.cwd
	if project != nil {
		root = project.RootDir()
	}
	path, required := l.resolveBaselineSetting(root, cfg)
	if path == "" {
		return nil, nil
	}

	b := &l.baseline
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.states == nil {
		b.states = map[string]*baselineState{}
	}
	key := root + "\x00" + path
	if s, ok := b.states[key]; ok {
		if s == nil {
			return nil, nil
		}
		return s, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			b.states[key] = nil // "auto" without a file
			return nil, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("baseline file %q does not exist. create it with jactionlint -baseline-write", path)
		}
		return nil, fmt.Errorf("could not read baseline file %q: %w", path, err)
	}
	bl, err := ParseBaseline(raw)
	if err != nil {
		return nil, fmt.Errorf("could not parse baseline file %q: %w", path, err)
	}
	s := newBaselineState(path, root, bl)
	s.cfg = cfg
	s.raw = raw
	b.states[key] = s
	l.debug("Loaded baseline %q with %d entries", path, len(bl.Entries))
	return s, nil
}

// baselineStage computes the baseline identity of the errors of one file (sorted by position) and
// marks the errors the baseline accepts. It does nothing without a baseline, unless the baseline is
// being written.
func (l *Linter) baselineStage(path string, content []byte, project *Project, bl *baselineState, errs []*Error) {
	if bl == nil && !l.baseline.writing {
		return
	}
	root := l.cwd
	if project != nil {
		root = project.RootDir()
	}
	key := baselineFileKey(root, l.cwd, path)
	infos := computeBaselineInfo(key, root, content, errs)
	if l.baseline.writing {
		for e, info := range infos {
			l.baseline.infos.Store(e, info)
		}
	}
	if bl != nil {
		l.hintBaseline.Store(true)
		bl.match(key, errs, infos)
		bl.markLinted(key)
	}
}

// WriteBaselineResult describes a baseline file which WriteBaseline wrote.
type WriteBaselineResult struct {
	// Path is the baseline file.
	Path string
	// Entries is the number of findings in the file.
	Entries int
	// Files is the number of files which have findings in the file.
	Files int
	// Changed is false when the file already had this content.
	Changed bool
	// Applied is true when the configuration already applies the baseline ("baseline: auto" or the path of
	// a file), so a plain run uses it. Otherwise only -baseline does.
	Applied bool
	// ConfigFile is the configuration file, relative to the repository, in which "baseline: auto" applies the
	// baseline to every run. It is the file the repository has, else the default .github/jactionlint.yaml
	// that -init-config creates.
	ConfigFile string
	// ConfigValue is the value of "baseline" in ConfigFile which applies this baseline: "auto" for the default
	// file, else the path of the file relative to the repository.
	ConfigValue string
}

// WriteBaseline records the current findings as the baseline. Without files it lints the whole
// repository like LintRepository and replaces the baseline. With files it lints only them: the entries
// of those files are replaced and the entries of the other files that still exist are kept. The
// baseline is written to path, or to .github/jactionlint-baseline.json in the repository when path is
// empty. The file is sorted and does not contain line numbers, so writing it again without a change in
// the findings gives the same bytes. Findings are recorded as they are reported: after the ignores and
// the minimum severity.
func (l *Linter) WriteBaseline(files []string, path string) (*WriteBaselineResult, error) {
	l.baseline.writing = true
	defer func() {
		l.baseline.writing = false
		l.baseline.infos.Clear()
	}()

	var project *Project
	partial := len(files) > 0
	if !partial {
		var p *Project
		var err error
		files, p, err = l.repositoryFiles(l.cwd)
		if err != nil {
			return nil, err
		}
		project = p
	} else {
		p, err := l.projects.At(files[0])
		if err != nil {
			return nil, err
		}
		project = p
	}
	root := l.cwd
	if project != nil {
		root = project.RootDir()
	}
	if path == "" {
		path = filepath.Join(root, DefaultBaselineFile)
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(l.cwd, path)
	}

	results, err := l.lintFilesQuietly(files, project)
	if err != nil {
		return nil, err
	}

	bl := &Baseline{Version: baselineVersion}
	for _, r := range results {
		for _, e := range r.errs {
			v, ok := l.baseline.infos.Load(e)
			if !ok || e.ID == unusedBaselineEntryID {
				continue
			}
			bl.Entries = append(bl.Entries, baselineEntryFor(e, v.(*baselineInfo)))
		}
	}
	keys := map[string]bool{}
	for _, r := range results {
		keys[baselineFileKey(root, l.cwd, r.path)] = true
	}

	if partial {
		old, err := ReadBaselineFile(path)
		switch {
		case err == nil:
			for _, e := range old.Entries {
				if keys[e.File] {
					continue // refreshed by this run
				}
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(e.File))); err != nil {
					continue // the file is gone
				}
				bl.Entries = append(bl.Entries, &BaselineEntry{File: e.File, Rule: e.Rule, Fingerprint: e.Fingerprint, Context: e.Context, Occurrence: e.Occurrence, Message: e.Message})
			}
		case errors.Is(err, os.ErrNotExist):
		default:
			return nil, err
		}
	}
	slices.SortStableFunc(bl.Entries, func(a, b *BaselineEntry) int { return strings.Compare(a.File, b.File) })

	out, err := bl.Marshal()
	if err != nil {
		return nil, err
	}
	res := &WriteBaselineResult{Path: path, Entries: len(bl.Entries), ConfigFile: ".github/jactionlint.yaml", ConfigValue: "auto"}
	if path != filepath.Join(root, DefaultBaselineFile) {
		if rel, err := filepath.Rel(root, path); err == nil {
			res.ConfigValue = filepath.ToSlash(rel)
		} else {
			res.ConfigValue = filepath.ToSlash(path)
		}
	}
	if cfg := l.configFor(project); cfg != nil {
		res.Applied = baselineConfigured(cfg)
		if cfg.Path != "" && project != nil {
			if rel, err := filepath.Rel(project.RootDir(), cfg.Path); err == nil && !strings.HasPrefix(rel, "..") {
				res.ConfigFile = filepath.ToSlash(rel)
			}
		}
	}
	fileSet := map[string]bool{}
	for _, e := range bl.Entries {
		fileSet[e.File] = true
	}
	res.Files = len(fileSet)
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, out) {
		return res, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("could not create the directory of baseline file %q: %w", path, err)
	}
	if err := writeFileKeepingMode(path, out); err != nil {
		return nil, err
	}
	res.Changed = true
	return res, nil
}

// toolUnavailable reports whether the external command of the rule cannot run.
func toolUnavailable(cmd string) bool {
	if cmd == "" {
		return true
	}
	_, err := exec.LookPath(cmd)
	return err != nil
}

// ruleRanAsIs reports whether the findings of the rule can be compared with a baseline in this run:
// the rule is on and what it needs is available. A baseline written with -online, or in a
// different environment, has entries this run cannot reproduce; those are not unused.
func (l *Linter) ruleRanAsIs(id string, cfg *Config) bool {
	// RuleRuns knows the level of the rule and whether it is an online rule while the online checks are off
	// (-online, -online=false, "online" and "online-options" of the configuration all count)
	if !cfg.RuleRuns(id, l.online.enabled || l.online.enabledBy(cfg)) {
		return false
	}
	switch id {
	case "shellcheck":
		return !toolUnavailable(l.shellcheck)
	case "pyflakes":
		return !toolUnavailable(l.pyflakes)
	}
	return true
}

// unusedBaselineEntries returns the entries which matched nothing in this run: the file was linted
// and the finding is gone, or the file does not exist any more.
func (l *Linter) unusedBaselineEntries(s *baselineState) []*BaselineEntry {
	var ret []*BaselineEntry
	for _, e := range s.bl.Entries {
		if s.wasSeen(e) || !l.ruleRanAsIs(e.Rule, s.cfg) {
			continue
		}
		if !s.isLinted(e.File) {
			if _, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(e.File))); err == nil {
				continue // the file exists but was not linted in this run
			}
		}
		ret = append(ret, e)
	}
	return ret
}

// withBaselineResults appends one result per baseline file which lists its unused entries. They are
// reported as errors only with -baseline-check; the summary counts them either way.
func (l *Linter) withBaselineResults(results []fileResult) []fileResult {
	l.baseline.mu.Lock()
	var states []*baselineState
	for _, s := range l.baseline.states {
		if s != nil {
			states = append(states, s)
		}
	}
	l.baseline.mu.Unlock()
	slices.SortFunc(states, func(a, b *baselineState) int { return strings.Compare(a.path, b.path) })

	for _, s := range states {
		unused := l.unusedBaselineEntries(s)
		path := s.path
		if r, err := filepath.Rel(l.cwd, s.path); err == nil && !strings.HasPrefix(r, "..") {
			path = r
		}
		res := fileResult{file: s.path, path: path, src: s.raw, baselineFile: true, baselineEntries: len(s.bl.Entries)}
		var stale []*Error
		for _, e := range unused {
			stale = append(stale, &Error{
				Message: fmt.Sprintf("baseline entry for rule %q in %q matches no finding any more: it was fixed, changed or moved. run jactionlint -baseline-write to remove it", e.Rule, e.File),
				Line:    e.line, Column: 1, Kind: "unused-baseline-entry", ID: "unused-baseline-entry", Filepath: path,
			})
		}
		stale = l.annotateErrors(stale, s.raw, s.cfg)
		kept := stale[:0]
		for _, e := range stale {
			if l.ignorePats.Match(e) || e.Severity < l.minSeverity {
				continue
			}
			kept = append(kept, e)
		}
		res.stale = kept
		if l.baseline.check {
			res.errs = kept
		}
		results = append(results, res)
	}
	return results
}

func (s *baselineState) wasSeen(e *BaselineEntry) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[e]
}

func (s *baselineState) markLinted(key string) {
	s.mu.Lock()
	s.linted[key] = true
	s.mu.Unlock()
}

func (s *baselineState) isLinted(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.linted[key]
}

// newFileResult builds the result of linting one file. The errors which the baseline accepts are kept
// apart: they are neither printed nor counted for the exit status.
func newFileResult(file, path string, src []byte, errs []*Error) fileResult {
	visible, baselined := splitBaselined(errs)
	if visible == nil {
		visible = []*Error{}
	}
	return fileResult{file: file, path: path, src: src, errs: visible, baselined: baselined}
}

// reportBaselineNote tells on the log output that a baseline hid findings. Only the text formats print
// it; the other formats are read by programs.
func (l *Linter) reportBaselineNote(results []fileResult) {
	if _, ok := l.printer.(textPrinter); !ok {
		return
	}
	hidden, unused := 0, 0
	for _, r := range results {
		hidden += len(r.baselined)
		unused += len(r.stale)
	}
	if hidden == 0 && unused == 0 {
		return
	}
	msg := fmt.Sprintf("%d finding(s) are hidden by the baseline", hidden)
	if unused > 0 && !l.baseline.check {
		msg += fmt.Sprintf("; %d baseline entr(ies) match nothing any more (-baseline-check lists them)", unused)
	}
	fmt.Fprintln(l.logOut, "note:", msg)
}

// printOne prints the result of one file, with the baseline file result if the baseline has unused
// entries, and returns the errors to report.
func (l *Linter) printOne(r fileResult) ([]*Error, error) {
	results := l.finishRun([]fileResult{r})
	if err := l.printer.print(l.out, results, l.notifications()); err != nil {
		return nil, err
	}
	l.reportBaselineNote(results)
	l.reportRunHint(results)
	if len(results) == 1 {
		return r.errs, nil
	}
	all := slices.Clone(r.errs)
	for _, x := range results[1:] {
		all = append(all, x.errs...)
	}
	return all, nil
}

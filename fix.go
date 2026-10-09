package jactionlint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// maxFixPasses limits how many times files are fixed and linted again. A fix can make another one
// possible or conflict with another fix of the same pass, so fixing repeats until nothing changes.
const maxFixPasses = 10

// FixResult is the result of fixing files.
type FixResult struct {
	// Fixed is the paths of the files which were rewritten, as they were given to the linter.
	Fixed []string
	// Applied is the number of fixes which were applied.
	Applied int
	// Errors are the errors which remain after fixing, including the ones without a fix. They were
	// printed with the output format of the linter.
	Errors []*Error
}

// applyFixes applies the fixes of the errors to the source. A fix is applied only when it is allowed by
// the mode, all its edits are valid and none of them conflict with the fixes applied before, which
// are the ones of errors earlier in the file. It returns the new source and the number of applied
// fixes. The source is returned as is when nothing was applied.
func applyFixes(src []byte, errs []*Error, mode FixMode) ([]byte, int) {
	var edits []TextEdit
	applied := 0
Fixes:
	for _, e := range errs {
		f := e.Fix
		if f == nil || (f.Unsafe && mode != FixModeUnsafe) || !f.validFor(src) {
			continue
		}
		for _, edit := range f.Edits {
			for _, a := range edits {
				if editsConflict(a, edit) {
					continue Fixes
				}
			}
		}
		for _, edit := range f.Edits {
			if !slices.Contains(edits, edit) {
				edits = append(edits, edit)
			}
		}
		applied++
	}
	if applied == 0 {
		return src, 0
	}

	// Apply from the end so that the offsets of the other edits stay valid
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].Start != edits[j].Start {
			return edits[i].Start > edits[j].Start
		}
		return edits[i].End > edits[j].End
	})
	out := slices.Clone(src)
	for _, e := range edits {
		out = append(out[:e.Start], append([]byte(e.NewText), out[e.End:]...)...)
	}
	return out, applied
}

// FixRepository fixes the files of the nearest project which LintRepository lints: the workflows and the Dependabot configuration.
// When the directory path is empty, the current working directory will be used instead.
func (l *Linter) FixRepository(dir string, mode FixMode) (*FixResult, error) {
	if dir == "" {
		dir = l.cwd
	}
	files, p, err := l.repositoryFiles(dir)
	if err != nil {
		return nil, err
	}
	return l.FixFiles(files, p, mode)
}

// FixFiles applies the automatic fixes of the errors found in the files and writes the files which
// changed. Fixing repeats until no fix is left, so running it again changes nothing. Then the errors
// which remain are printed with the output format of the linter and returned. The project parameter
// can be nil. In the case, a project is detected from the file path.
//
// Only the fixes allowed by the mode are applied: unsafe fixes need FixModeUnsafe. The errors which
// have no fix are never changed, so they remain.
func (l *Linter) FixFiles(filepaths []string, project *Project, mode FixMode) (*FixResult, error) {
	if mode != FixModeSafe && mode != FixModeUnsafe {
		return nil, errors.New("invalid fix mode")
	}
	res := &FixResult{}
	if len(filepaths) == 0 {
		return res, nil
	}

	results, err := l.lintFilesQuietly(filepaths, project)
	if err != nil {
		return nil, err
	}

	fixed := map[string]bool{}
	for pass := 0; pass < maxFixPasses; pass++ {
		var changed []string
		changedIdx := map[string]int{}
		for i, r := range results {
			out, n := applyFixes(r.src, r.errs, mode)
			if n == 0 {
				continue
			}
			if err := writeFileKeepingMode(r.file, out); err != nil {
				return nil, err
			}
			l.log("Applied", n, "fix(es) to", r.path)
			res.Applied += n
			fixed[r.file] = true
			changed = append(changed, r.file)
			changedIdx[r.file] = i
		}
		if len(changed) == 0 {
			break
		}
		again, err := l.lintFilesQuietly(changed, project)
		if err != nil {
			return nil, err
		}
		for _, r := range again {
			results[changedIdx[r.file]] = r
		}
	}

	for _, f := range filepaths {
		if fixed[f] {
			res.Fixed = append(res.Fixed, f)
		}
	}
	for _, r := range results {
		res.Errors = append(res.Errors, r.errs...)
	}
	if err := l.printer.print(l.out, results, l.notifications()); err != nil {
		return nil, err
	}
	if res.Applied > 0 {
		fmt.Fprintf(l.logOut, "Fixed %d problem(s) in %d file(s)\n", res.Applied, len(res.Fixed))
	}
	return res, nil
}

// writeFileKeepingMode overwrites the file with the content and keeps its permission bits.
func writeFileKeepingMode(path string, b []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".jactionlint-fix-*")
	if err != nil {
		return fmt.Errorf("could not write %q: %w", path, err)
	}
	name := tmp.Name()
	defer os.Remove(name) // No-op after the rename
	if _, err := tmp.Write(b); err != nil {
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
		// Renaming over a file can fail where the directory is not writable but the file is
		if werr := os.WriteFile(path, b, mode); werr != nil {
			return fmt.Errorf("could not write %q: %w", path, werr)
		}
	}
	return nil
}

// fixFlag is the value of the -fix flag: -fix applies the safe fixes and -fix=unsafe applies all.
type fixFlag struct {
	mode FixMode
}

func (f *fixFlag) String() string {
	switch f.mode {
	case FixModeSafe:
		return "safe"
	case FixModeUnsafe:
		return "unsafe"
	}
	return "false"
}

// IsBoolFlag lets the flag be given without a value.
func (f *fixFlag) IsBoolFlag() bool { return true }

func (f *fixFlag) Set(v string) error {
	switch strings.ToLower(v) {
	case "true", "safe":
		f.mode = FixModeSafe
	case "unsafe":
		f.mode = FixModeUnsafe
	case "false":
		f.mode = 0
	default:
		return fmt.Errorf("invalid value %q. use -fix or -fix=unsafe", v)
	}
	return nil
}

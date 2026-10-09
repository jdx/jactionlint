package jactionlint

import (
	"io"
	"testing"
)

// lintIDsAt lints the source as a file with the path and returns the errors with the ID.
func lintFileWithConfig(t *testing.T, cfg *Config, path, src string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	errs, err := l.Lint(path, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func errsWithID(errs []*Error, id string) []*Error {
	var ret []*Error
	for _, e := range errs {
		if e.ID == id {
			ret = append(ret, e)
		}
	}
	return ret
}

// wantLines checks that the rule reports exactly at the lines.
func wantLines(t *testing.T, errs []*Error, id string, lines ...int) {
	t.Helper()
	var have []int
	for _, e := range errsWithID(errs, id) {
		have = append(have, e.Line)
	}
	if len(have) != len(lines) {
		t.Fatalf("%s: want errors at lines %v but got %v (%v)", id, lines, have, errs)
	}
	for i := range lines {
		if have[i] != lines[i] {
			t.Fatalf("%s: want errors at lines %v but got %v", id, lines, have)
		}
	}
}

// fixAndLint applies the fixes of the rule to the source and lints the result again.
func fixAndLint(t *testing.T, cfg *Config, path, src string, mode FixMode) (string, []*Error) {
	t.Helper()
	errs := lintFileWithConfig(t, cfg, path, src)
	out, n := applyFixes([]byte(src), errs, mode)
	if n == 0 {
		return src, errs
	}
	return string(out), lintFileWithConfig(t, cfg, path, string(out))
}

const batchAJob = "    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"

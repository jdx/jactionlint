package jactionlint

import (
	"io"
	"testing"
)

func TestRequirePermissionsOptIn(t *testing.T) {
	const job = "    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	tests := []struct {
		what string
		src  string
		want int // number of errors when enabled
	}{
		{"none", "on: push\njobs:\n  a:\n" + job, 1},
		{"workflow level", "on: push\npermissions:\n  contents: read\njobs:\n  a:\n" + job, 0},
		{"workflow level all", "on: push\npermissions: read-all\njobs:\n  a:\n" + job, 0},
		{"workflow level empty", "on: push\npermissions: {}\njobs:\n  a:\n" + job, 0},
		{"job level", "on: push\njobs:\n  a:\n    permissions: {}\n" + job, 0},
		{"one of two jobs", "on: push\njobs:\n  a:\n    permissions: {}\n" + job + "  b:\n" + job, 1},
		{"two jobs none", "on: push\njobs:\n  a:\n" + job + "  b:\n" + job, 2},
		{"workflow call", "on: push\njobs:\n  a:\n    uses: o/r/.github/workflows/w.yml@v1\n", 1},
		{"workflow call with permissions", "on: push\njobs:\n  a:\n    uses: o/r/.github/workflows/w.yml@v1\n    permissions:\n      contents: read\n", 0},
		{"workflow call covered at workflow level", "on: push\npermissions: {}\njobs:\n  a:\n    uses: o/r/.github/workflows/w.yml@v1\n", 0},
	}
	for _, tc := range tests {
		for _, enabled := range []bool{false, true} {
			l, err := NewLinter(io.Discard, &LinterOptions{})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = ruleSwitch("missing-permissions", enabled)
			errs, err := l.Lint("test.yaml", []byte(tc.src), nil)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if enabled {
				want = tc.want
			}
			if len(errs) != want {
				t.Errorf("%s (enabled=%v): want %d errors but got %d: %v", tc.what, enabled, want, len(errs), errs)
			}
		}
	}
}

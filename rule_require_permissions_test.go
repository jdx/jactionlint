package jactionlint

import (
	"io"
	"strings"
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
		{"workflow_call only", "on:\n  workflow_call:\njobs:\n  a:\n" + job, 0},
		{"workflow_call only string form", "on: workflow_call\njobs:\n  a:\n" + job, 0},
		{"workflow_call only calling reusable", "on:\n  workflow_call:\njobs:\n  a:\n    uses: o/r/.github/workflows/w.yml@v1\n", 0},
		{"workflow_call and push", "on:\n  workflow_call:\n  push:\njobs:\n  a:\n" + job, 1},
		{"workflow_call and push with permissions", "on:\n  workflow_call:\n  push:\npermissions: {}\njobs:\n  a:\n" + job, 0},
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

// TestRequirePermissionsReusableMessage checks that a workflow which is also reusable reports a finding
// without a fix and says why, and that a workflow which is only reusable reports nothing.
func TestRequirePermissionsReusableMessage(t *testing.T) {
	const job = "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	lint := func(src string) []*Error {
		l, err := NewLinter(io.Discard, &LinterOptions{})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = ruleSwitch("missing-permissions", true)
		errs, err := l.Lint("test.yaml", []byte(src), nil)
		if err != nil {
			t.Fatal(err)
		}
		return errs
	}
	if errs := lint("on:\n  workflow_call:\n" + job); len(errs) != 0 {
		t.Errorf("workflow_call only: want no errors but got %v", errs)
	}
	errs := lint("on:\n  workflow_call:\n  push:\n" + job)
	if len(errs) != 1 {
		t.Fatalf("workflow_call and push: want 1 error but got %v", errs)
	}
	if errs[0].Fix == nil || !errs[0].Fix.Unsafe {
		t.Errorf("workflow_call and push: want an unsafe fix but got %+v", errs[0].Fix)
	}
	if !strings.Contains(errs[0].Message, "fix is unsafe") || !strings.Contains(errs[0].Message, "workflow_call") {
		t.Errorf("message does not explain the missing fix: %q", errs[0].Message)
	}
	// A workflow that is not reusable keeps its fix and its plain message
	errs = lint("on: push\n" + job)
	if len(errs) != 1 || errs[0].Fix == nil || strings.Contains(errs[0].Message, "workflow_call") {
		t.Errorf("push only: want 1 fixable plain error but got %v", errs)
	}
}

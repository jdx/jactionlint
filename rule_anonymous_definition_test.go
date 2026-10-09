package jactionlint

import (
	"strings"
	"testing"
)

func TestAnonymousDefinition(t *testing.T) {
	cfg := ruleConfig("anonymous-definition")
	tests := []struct {
		what  string
		path  string
		src   string
		lines []int
		fixed string // the source after --fix; empty means no fix
	}{
		{
			what:  "workflow and job",
			path:  ".github/workflows/ci-build.yaml",
			src:   "on: push\njobs:\n  build:\n" + batchAJob,
			lines: []int{1, 3},
			fixed: "name: ci-build\non: push\njobs:\n  build:\n    name: build\n" + batchAJob,
		},
		{
			what:  "named workflow and job",
			path:  "ci.yaml",
			src:   "name: CI\non: push\njobs:\n  build:\n    name: Build\n" + batchAJob,
			lines: nil,
		},
		{
			what:  "after a comment and a document start",
			path:  "ci.yaml",
			src:   "# comment\n---\n\non: push\njobs:\n  b:\n" + batchAJob,
			lines: []int{4, 6},
			fixed: "# comment\n---\n\nname: ci\non: push\njobs:\n  b:\n    name: b\n" + batchAJob,
		},
		{
			what:  "the name of the file is quoted when it must be",
			path:  "on.yaml",
			src:   "on: push\njobs:\n  true:\n    name: x\n" + batchAJob,
			lines: []int{1},
			fixed: "name: \"on\"\non: push\njobs:\n  true:\n    name: x\n" + batchAJob,
		},
		{
			what:  "a name which looks like a number is quoted",
			path:  "2024.yaml",
			src:   "on: push\njobs:\n  a:\n    name: x\n" + batchAJob,
			lines: []int{1},
			fixed: "name: \"2024\"\non: push\njobs:\n  a:\n    name: x\n" + batchAJob,
		},
		{
			what:  "stdin uses the ID of the only job",
			path:  "<stdin>",
			src:   "on: push\njobs:\n  only-job:\n    name: x\n" + batchAJob,
			lines: []int{1},
			fixed: "name: only-job\non: push\njobs:\n  only-job:\n    name: x\n" + batchAJob,
		},
		{
			what:  "stdin with several jobs has no workflow fix",
			path:  "<stdin>",
			src:   "on: push\njobs:\n  a:\n    name: x\n" + batchAJob + "  b:\n    name: y\n" + batchAJob,
			lines: []int{1},
		},
		{
			what:  "job with a comment after the key and a CRLF source",
			path:  "ci.yaml",
			src:   "name: x\r\non: push\r\njobs:\r\n  build: # the build\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - run: echo\r\n",
			lines: []int{4},
			fixed: "name: x\r\non: push\r\njobs:\r\n  build: # the build\r\n    name: build\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - run: echo\r\n",
		},
		{
			what:  "job in the flow style has no fix",
			path:  "ci.yaml",
			src:   "name: x\non: push\njobs:\n  build: {runs-on: ubuntu-latest, steps: [{run: echo}]}\n",
			lines: []int{4},
		},
		{
			what:  "a job which calls a reusable workflow",
			path:  "ci.yaml",
			src:   "name: x\non: push\njobs:\n  call:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n",
			lines: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := lintFileWithConfig(t, cfg, tc.path, tc.src)
			wantLines(t, errs, "anonymous-definition", tc.lines...)
			for _, e := range errsWithID(errs, "anonymous-definition") {
				if tc.fixed == "" && e.Fix != nil && !strings.HasPrefix(e.Message, "job") {
					t.Errorf("unexpected fix %+v", e.Fix)
				}
				if e.Fix != nil && e.Fix.Unsafe {
					t.Errorf("the fix must be safe: %+v", e.Fix)
				}
			}
			if tc.fixed == "" {
				return
			}
			got, after := fixAndLint(t, cfg, tc.path, tc.src, FixModeSafe)
			if got != tc.fixed {
				t.Errorf("fixed source mismatch\nwant: %q\nhave: %q", tc.fixed, got)
			}
			if rest := errsWithID(after, "anonymous-definition"); len(rest) != 0 {
				t.Errorf("still reported after the fix: %v", rest)
			}
			if len(after) != 0 {
				t.Errorf("the fixed source is not clean: %v", after)
			}
		})
	}
}

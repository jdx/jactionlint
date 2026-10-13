package jactionlint

import (
	"strings"
	"testing"
)

func TestDependabotDirectoryGlobs(t *testing.T) {
	tests := []struct {
		entry, dir string
		want       bool
	}{
		{"/", "/", true},
		{"/", "/.github/actions/setup", false},
		{"/.github/actions/setup", "/.github/actions/setup", true},
		{"/.github/actions/setup/", "/.github/actions/setup", true},
		{".github/actions/setup", "/.github/actions/setup", true},
		{"/.github/actions/**/*", "/.github/actions/setup", true},
		{"/.github/actions/**/*", "/.github/actions/a/b", true},
		{"/.github/actions/**/*", "/.github/actions", false},
		{"/.github/actions/**", "/.github/actions/setup", true},
		{"/.github/actions/*", "/.github/actions/setup", true},
		{"/.github/actions/*", "/.github/actions/a/b", false},
		{"/.github/actions/*-ci", "/.github/actions/build-ci", true},
		{"/.github/actions/*-ci", "/.github/actions/build", false},
		{"/**/*", "/tools/custom", true},
		{"/**/*", "/", false},
		{"/.github/**/setup", "/.github/actions/setup", true},
		{"/.github/**/setup", "/.github/setup", true},
		{"/.github/*", "/.github/actions/setup", false},
	}
	for _, tc := range tests {
		if got := dependabotDirectoryCovers(tc.entry, tc.dir); got != tc.want {
			t.Errorf("%q covers %q: want %v, got %v", tc.entry, tc.dir, tc.want, got)
		}
	}
}

// A github-actions update reads the workflows and the action.yml of its directory: the local composite actions which
// use actions need a directory of their own (zizmor#1806).
func TestDependabotMissingActionsUpdateLocalActions(t *testing.T) {
	const npm = "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n"
	ghUpdate := func(dirs string) string {
		return npm + "  - package-ecosystem: github-actions\n" + dirs + "    schedule:\n      interval: weekly\n"
	}
	const composite = "runs:\n  using: composite\n  steps:\n    - uses: actions/setup-node@v4\n    - run: echo\n      shell: bash\n"
	const workflow = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/setup\n      - uses: actions/checkout@v4\n"
	files := map[string]string{
		".github/workflows/ci.yml":          workflow,
		".github/actions/setup/action.yml":  composite,
		".github/actions/other/action.yml":  composite,
		".github/actions/nouses/action.yml": "runs:\n  using: composite\n  steps:\n    - run: echo\n      shell: bash\n",
		".github/actions/local/action.yml":  "runs:\n  using: composite\n  steps:\n    - uses: ./.github/actions/nouses\n    - uses: docker://alpine:3\n",
	}
	cfg := &Config{Profile: ProfilePedantic, Rules: map[string]RuleConfig{"dependabot-cooldown": {Level: SeverityOff}}}
	tests := []struct {
		name  string
		src   string
		want  bool
		fixed string // the source after the unsafe fixes, "" for a finding without a fix
	}{
		{"directory /", ghUpdate("    directory: /\n"), true, ghUpdate("    directories: [\"/\", \"/.github/actions/**/*\"]\n")},
		{"quoted directory", ghUpdate("    directory: \"/\"\n"), true, ghUpdate("    directories: [\"/\", \"/.github/actions/**/*\"]\n")},
		{"directories with the glob", ghUpdate("    directories:\n      - /\n      - /.github/actions/**/*\n"), false, ""},
		{"directories with the names", ghUpdate("    directories: [\"/\", \"/.github/actions/setup\", \"/.github/actions/other\"]\n"), false, ""},
		{"directories with one name", ghUpdate("    directories: [\"/\", \"/.github/actions/setup\"]\n"), true, ""},
		{"a second update for the actions", ghUpdate("    directory: /\n") + "  - package-ecosystem: github-actions\n    directory: /.github/actions/setup\n    schedule:\n      interval: weekly\n", true, ""},
		{"only the actions", ghUpdate("    directory: /.github/actions/*\n"), false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeProject(t, files)
			errs := lintDependabot(t, tc.src, cfg, &Project{root: root})
			var got []*Error
			for _, e := range errs {
				if e.ID == "dependabot-missing-actions-update" {
					got = append(got, e)
				}
			}
			if !tc.want {
				if len(got) != 0 {
					t.Fatalf("want no finding, got %v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want one finding, got %v", got)
			}
			if !strings.Contains(got[0].Message, "local composite actions") || strings.Contains(got[0].Message, "nouses") || strings.Contains(got[0].Message, "/.github/actions/local") {
				t.Errorf("unexpected message %q", got[0].Message)
			}
			if tc.fixed == "" {
				if got[0].Fix != nil || got[0].NoFix == nil {
					t.Errorf("want no fix with a reason: %+v", got[0])
				}
				return
			}
			if got[0].Fix == nil || !got[0].Fix.Unsafe {
				t.Fatalf("want an unsafe fix, got %+v", got[0].Fix)
			}
			if safe, n := applyFixes([]byte(tc.src), got, FixModeSafe); n != 0 || string(safe) != tc.src {
				t.Errorf("the unsafe fix was applied by default: %q", safe)
			}
			fixed, n := applyFixes([]byte(tc.src), got, FixModeUnsafe)
			if n != 1 || string(fixed) != tc.fixed {
				t.Fatalf("want %q, got %q", tc.fixed, fixed)
			}
			again := lintDependabot(t, string(fixed), cfg, &Project{root: root})
			if len(again) != 0 {
				t.Errorf("the fixed source still has findings: %v", again)
			}
		})
	}

	t.Run("no local action with external uses", func(t *testing.T) {
		root := writeProject(t, map[string]string{".github/workflows/ci.yml": workflow, ".github/actions/setup/action.yml": "runs:\n  using: composite\n  steps:\n    - run: echo\n      shell: bash\n"})
		if errs := lintDependabot(t, ghUpdate("    directory: /\n"), cfg, &Project{root: root}); len(errs) != 0 {
			t.Fatal(errs)
		}
	})
	t.Run("renovate", func(t *testing.T) {
		withRenovate := map[string]string{"renovate.json": "{}"}
		for k, v := range files {
			withRenovate[k] = v
		}
		root := writeProject(t, withRenovate)
		if errs := lintDependabot(t, ghUpdate("    directory: /\n"), cfg, &Project{root: root}); len(errs) != 0 {
			t.Fatal(errs)
		}
	})
}

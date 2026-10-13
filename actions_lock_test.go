package jactionlint

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lintLockProject lints the workflows of the repository in testdata/actions_lock/<name> with the
// config and returns the findings.
func lintLockProject(t *testing.T, root string, cfg *Config) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	errs, err := l.LintDir(filepath.Join(root, ".github", "workflows"), &Project{root: root})
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func TestActionsLockUnpinnedUses(t *testing.T) {
	// Each directory is a repository, and its .out file the expected findings of unpinned-uses.
	tests := []struct {
		name string
		cfg  *Config
	}{
		{"covered", fixtureConfig("unpinned-uses")},
		// The option which switches the lock file off reports what a repository without it gets
		{"covered_option_off", mustParseConfigFixture(t, "rules:\n  unpinned-uses:\n    level: error\n    actions-lock: false\n")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.name
			if src == "covered_option_off" {
				src = "covered"
			}
			errs := lintLockProject(t, filepath.Join("testdata", "actions_lock", src), tc.cfg)
			checkErrors(t, filepath.Join("testdata", "actions_lock", tc.name+".out"), errs)
		})
	}
}

func mustParseConfigFixture(t *testing.T, src string) *Config {
	t.Helper()
	return withFixtureRules(mustParseConfig(t, src))
}

// A lock file which is missing, is not YAML, has no usable shape or has a version whose shape is not known
// changes nothing: every tag is reported.
func TestActionsLockIgnoredWhenNotUsable(t *testing.T) {
	const wf = "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v6.0.2\n"
	const entry = "    '.github/workflows/ci.yml':\n        - 'actions/checkout@v6.0.2'\n"
	const dep = "dependencies:\n    'actions/checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: 1\n        repo_id: 2\n"
	tests := []struct {
		name     string
		lock     string // "" for no lock file
		wantPins bool
	}{
		{"valid v0.0.3", "version: 'v0.0.3'\nworkflows:\n" + entry + dep, true},
		{"valid v0.0.2", "version: 'v0.0.2'\nworkflows:\n" + entry + dep, true},
		{"unknown keys are ignored", "version: 'v0.0.3'\nfuture: {a: b}\nworkflows:\n" + entry + dep, true},
		{"no lock file", "", false},
		{"future version", "version: 'v0.1.0'\nworkflows:\n" + entry + dep, false},
		{"v0.0.1 is not read", "version: 'v0.0.1'\nworkflows:\n" + entry + dep, false},
		{"no version", "workflows:\n" + entry + dep, false},
		{"not YAML", "version: 'v0.0.3'\nworkflows: [\n", false},
		{"not a mapping", "- a\n- b\n", false},
		{"empty", "", false},
		{"workflows is a list", "version: 'v0.0.3'\nworkflows:\n    - 'actions/checkout@v6.0.2'\n" + dep, false},
		{"dependencies is a list", "version: 'v0.0.3'\nworkflows:\n" + entry + "dependencies:\n    - a\n", false},
		{"commit is not a digest", "version: 'v0.0.3'\nworkflows:\n" + entry + strings.Replace(dep, "sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd", "sha1-de0fac", 1), false},
		{"no commit", "version: 'v0.0.3'\nworkflows:\n" + entry + strings.Replace(dep, "        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n", "", 1), false},
		{"ref of the dependency differs", "version: 'v0.0.3'\nworkflows:\n" + entry + strings.Replace(dep, "ref: 'v6.0.2'", "ref: 'v6'", 1), false},
		{"no dependency entry", "version: 'v0.0.3'\nworkflows:\n" + entry, false},
		{"workflow is not listed", "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/other.yml':\n        - 'actions/checkout@v6.0.2'\n" + dep, false},
		{"path is not the one of the workflow", "version: 'v0.0.3'\nworkflows:\n    'ci.yml':\n        - 'actions/checkout@v6.0.2'\n" + dep, false},
		// The official parser rejects a repeated key, so the file is not trusted: neither the first nor the last entry wins
		{"a workflow listed twice, the first pins", "version: 'v0.0.3'\nworkflows:\n" + entry + "    '.github/workflows/ci.yml':\n        - 'other/action@v1'\n" + dep, false},
		{"a workflow listed twice, the last pins", "version: 'v0.0.3'\nworkflows:\n    '.github/workflows/ci.yml':\n        - 'other/action@v1'\n" + entry + dep, false},
		{"a dependency listed twice", "version: 'v0.0.3'\nworkflows:\n" + entry + dep + strings.Replace(dep, "dependencies:\n", "", 1), false},
		{"a key twice in a dependency", "version: 'v0.0.3'\nworkflows:\n" + entry + dep + "        commit: 'sha1-0000000000000000000000000000000000000000'\n", false},
		{"version twice", "version: 'v0.0.3'\nversion: 'v0.0.3'\nworkflows:\n" + entry + dep, false},
		{"a dependency under two spellings with other metadata", "version: 'v0.0.3'\nworkflows:\n" + entry + dep + "    'Actions/Checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-0000000000000000000000000000000000000000'\n        owner_id: 1\n        repo_id: 2\n", false},
		{"two spellings, an owner_id missing and 0", "version: 'v0.0.3'\nworkflows:\n" + entry + dep + "    'Actions/Checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        repo_id: 2\n", false},
		{"two spellings, a negative owner_id", "version: 'v0.0.3'\nworkflows:\n" + entry + "dependencies:\n    'actions/checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: 0\n        repo_id: 2\n    'Actions/Checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: -1\n        repo_id: 2\n", false},
		{"two spellings, a non integer repo_id", "version: 'v0.0.3'\nworkflows:\n" + entry + "dependencies:\n    'actions/checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: 1\n        repo_id: 0\n    'Actions/Checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: 1\n        repo_id: abc\n", false},
		{"two spellings, an extra invalid uses item", "version: 'v0.0.3'\nworkflows:\n" + entry + dep + "    'Actions/Checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: 1\n        repo_id: 2\n        uses:\n            - 'not a pin'\n", false},
		{"two spellings, a quoted and a plain id", "version: 'v0.0.3'\nworkflows:\n" + entry + dep + "    'Actions/Checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: '1'\n        repo_id: 2\n", false},
		{"a dependency under two spellings with the same metadata", "version: 'v0.0.3'\nworkflows:\n" + entry + dep + "    'Actions/Checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: 1\n        repo_id: 2\n", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), wf)
			if tc.lock != "" || tc.name == "empty" {
				writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), tc.lock)
			}
			errs := lintLockProject(t, root, fixtureConfig("unpinned-uses"))
			reported := 0
			for _, e := range errs {
				if e.ID == "unpinned-uses" {
					reported++
				}
			}
			if tc.wantPins && reported != 0 {
				t.Errorf("the lock file pins the action but %d findings were reported: %v", reported, errs)
			}
			if !tc.wantPins && reported != 1 {
				t.Errorf("the lock file must not be trusted but %d findings were reported, want 1: %v", reported, errs)
			}
		})
	}
}

// The linter is not given the project of the file by the caller: it finds the repository from the path.
func TestActionsLockFoundFromAPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	src, err := os.ReadFile(filepath.Join("testdata", "actions_lock", "covered", ".github", "workflows", "actions.lock"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), string(src))
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v6.0.2\n      - uses: actions/checkout@v6\n")

	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig("unpinned-uses")
	errs, err := l.LintFile(filepath.Join(root, ".github", "workflows", "ci.yml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].Line != 7 {
		t.Fatalf("only the second step must be reported: %v", errs)
	}
}

// An action's metadata is not a workflow of the lock file, even when the path is listed.
func TestActionsLockDoesNotApplyToActionMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./\n")
	writeFile(t, filepath.Join(root, "action.yml"), "name: x\ndescription: x\nruns:\n  using: composite\n  steps:\n    - uses: actions/checkout@v6.0.2\n")
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), "version: 'v0.0.3'\nworkflows:\n    'action.yml':\n        - 'actions/checkout@v6.0.2'\n"+
		"dependencies:\n    'actions/checkout@v6.0.2':\n        ref: 'v6.0.2'\n        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'\n        owner_id: 1\n        repo_id: 2\n")
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig("unpinned-uses")
	errs, err := l.LintFile(filepath.Join(root, "action.yml"), &Project{root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].ID != "unpinned-uses" {
		t.Fatalf("a step of an action's metadata is not covered by the lock file: %v", errs)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

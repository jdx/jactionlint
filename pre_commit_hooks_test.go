package jactionlint

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

type preCommitHook struct {
	ID                     string   `yaml:"id"`
	Language               string   `yaml:"language"`
	Files                  string   `yaml:"files"`
	Types                  []string `yaml:"types"`
	Entry                  string   `yaml:"entry"`
	AdditionalDependencies []string `yaml:"additional_dependencies"`
}

func readPreCommitHooks(t *testing.T) []preCommitHook {
	t.Helper()
	b, err := os.ReadFile(".pre-commit-hooks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var hooks []preCommitHook
	if err := yaml.Unmarshal(b, &hooks); err != nil {
		t.Fatal(err)
	}
	return hooks
}

// TestPreCommitHooksFiles checks which files trigger the hooks of .pre-commit-hooks.yaml. A hook has to run
// for everything jactionlint lints: the workflows, the metadata files of actions in any directory and
// .github/dependabot.yml. It must not run for look-alikes. pre-commit matches `files` with Python's re.search,
// and the pattern uses only syntax that Go's regexp reads the same way.
func TestPreCommitHooksFiles(t *testing.T) {
	hooks := readPreCommitHooks(t)
	if len(hooks) == 0 {
		t.Fatal("no hooks found")
	}
	linted := []string{
		".github/workflows/ci.yml",
		".github/workflows/ci.yaml",
		".github/workflows/sub/nested.yml",
		"action.yml",
		"action.yaml",
		".github/actions/setup/action.yml",
		"packages/tool/action.yaml",
		".github/dependabot.yml",
		".github/dependabot.yaml",
	}
	ignored := []string{
		"docker-compose.yml",
		"my-action.yml",
		"action.yml.bak",
		"actions.yml",
		"dependabot.yml",
		"src/dependabot.yml",
		".github/dependabot.yml.orig",
		".github/ISSUE_TEMPLATE/config.yml",
		".github/jactionlint.yaml",
		"docs/.github/workflows-notes.yml",
		"src/.github/workflows/ci.yml",
	}
	for _, p := range linted {
		if !IsActionPath(p) && !isDependabotPath(p) && !strings.HasPrefix(p, ".github/workflows/") {
			t.Errorf("jactionlint does not lint %s: fix the test list", p)
		}
	}
	for _, h := range hooks {
		t.Run(h.ID, func(t *testing.T) {
			re, err := regexp.Compile(h.Files)
			if err != nil {
				t.Fatalf("files %q of hook %s: %v", h.Files, h.ID, err)
			}
			for _, p := range linted {
				if !re.MatchString(p) {
					t.Errorf("hook %s does not run for %s", h.ID, p)
				}
			}
			for _, p := range ignored {
				if re.MatchString(p) {
					t.Errorf("hook %s runs for %s", h.ID, p)
				}
			}
			if h.Files != hooks[0].Files {
				t.Errorf("hook %s has files %q, the first hook has %q", h.ID, h.Files, hooks[0].Files)
			}
		})
	}
}

// TestPreCommitShellcheckHook checks that the hook with the bundled shellcheck installs a module that has a
// shellcheck command, with a pinned version (pre-commit runs `go install` for each dependency).
func TestPreCommitShellcheckHook(t *testing.T) {
	var found *preCommitHook
	for _, h := range readPreCommitHooks(t) {
		if h.ID == "jactionlint-shellcheck" {
			h := h
			found = &h
		}
	}
	if found == nil {
		t.Fatal("hook jactionlint-shellcheck is missing")
	}
	if found.Language != "golang" || found.Entry != "jactionlint" {
		t.Errorf("hook must be a golang hook running jactionlint: %+v", found)
	}
	re := regexp.MustCompile(`^github\.com/wasilibs/go-shellcheck/cmd/shellcheck@v\d+\.\d+\.\d+$`)
	if len(found.AdditionalDependencies) != 1 || !re.MatchString(found.AdditionalDependencies[0]) {
		t.Errorf("additional_dependencies = %v", found.AdditionalDependencies)
	}
}

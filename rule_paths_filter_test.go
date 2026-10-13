package jactionlint

import (
	"github.com/google/go-cmp/cmp"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathFilterMatching(t *testing.T) {
	files := &repoPaths{}
	for _, p := range []string{
		"README.md", "main.go", "docs", "docs/index.md", "docs/guide/start.md", "src", "src/js", "src/js/app.js", "src/app.jsx",
		"my-src", "my-src/code", "my-src/code/a.js", "db", "db/migrate-v1.sql", "Dockerfile", "v1.10", "ünï", "ünï/x.txt",
	} {
		files.paths = append(files.paths, strings.ToLower(p))
	}
	sortStrings(files.paths)

	tests := []struct {
		pat  string
		want bool // matches
		skip bool // not evaluated
	}{
		{pat: "README.md", want: true},
		{pat: "readme.md", want: true}, // case is not compared
		{pat: "docs", want: true},      // a directory
		{pat: "docs/", want: false},    // a path never ends with a slash
		{pat: "docs/*", want: true},
		{pat: "docs/**", want: true},
		{pat: "docs/**/*.md", want: true},
		{pat: "docs/*/*.md", want: true},
		{pat: "docs/*/*/*.md", want: false},
		{pat: "docs/**/start.md", want: true},
		{pat: "docs/missing/**", want: false},
		{pat: "documents/**", want: false},
		{pat: "*.go", want: true},
		{pat: "*.rs", want: false},
		{pat: "**.js", want: true},
		{pat: "**/*.js", want: true},
		{pat: "**/README.md", want: true},
		{pat: "**/app.js", want: true},
		{pat: "*.js", want: false}, // only the root
		{pat: "**/*src/**", want: true},
		{pat: "**/migrate-*.sql", want: true},
		{pat: "src/*.jsx?", want: true},
		{pat: "src/*.jsx+", want: true},
		{pat: "src/*.js+x", want: true},
		{pat: "src/*.j+sy", want: false},
		{pat: "v1.1?", want: false}, // v1.1 or v1.
		{pat: "v1.1?0", want: true},
		{pat: "v[0-9].1[0-9]", want: true},
		{pat: "v[2-9].*", want: false},
		{pat: "ünï/*", want: true},
		{pat: "Docker*", want: true},
		{pat: "\\*.md", want: false},
		{pat: "main.g\\o", want: true},
		{pat: ".github/workflows/ci.yml", want: false},
		// Not evaluated
		{pat: "!docs/missing/**", skip: true},
		{pat: "/docs/**", skip: true},
		{pat: "./docs/**", skip: true},
		{pat: "${{ inputs.path }}", skip: true},
		{pat: "docs/[", skip: true},        // invalid
		{pat: "?docs", skip: true},         // invalid
		{pat: "docs/[!a]*.md", skip: true}, // a class that is negated
		{pat: "", skip: true},
	}
	for _, tc := range tests {
		m := compilePathFilter(tc.pat)
		if tc.skip {
			if m != nil {
				t.Errorf("%q should not be evaluated", tc.pat)
			}
			continue
		}
		if m == nil {
			t.Errorf("%q was not compiled", tc.pat)
			continue
		}
		if have := files.matches(m); have != tc.want {
			t.Errorf("%q matches=%v, want %v", tc.pat, have, tc.want)
		}
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// lintPathsFilter writes the files into a new project and lints the workflow of it with the rule on.
func lintPathsFilter(t *testing.T, files map[string]string, workflow string) []*Error {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	files[".github/workflows/ci.yaml"] = workflow
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = mustParseConfig(t, "profile: correctness\nrules:\n  unmatched-paths-filter: warn\n")
	errs, err := l.Lint(filepath.Join(dir, ".github", "workflows", "ci.yaml"), []byte(workflow), &Project{root: dir})
	if err != nil {
		t.Fatal(err)
	}
	var ret []*Error
	for _, e := range errs {
		if e.ID == "unmatched-paths-filter" {
			ret = append(ret, e)
		}
	}
	return ret
}

func TestUnmatchedPathsFilter(t *testing.T) {
	repo := map[string]string{"src/main.go": "package main", "docs/a.md": "a", "Makefile": "all:"}
	copyRepo := func() map[string]string {
		c := map[string]string{}
		for k, v := range repo {
			c[k] = v
		}
		return c
	}

	t.Run("reports the entries which match nothing", func(t *testing.T) {
		errs := lintPathsFilter(t, copyRepo(), `on:
  push:
    paths:
      - 'src/**'
      - 'source/**'
      - 'docs/*.md'
      - '.github/workflows/ci.yaml'
      - '.github/workflows/ci.yml'
  pull_request:
    paths-ignore:
      - 'docs/**'
      - 'doc/**'
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
`)
		checkLines(t, errs, "5", "8", "12")
		if !strings.Contains(errs[0].Message, `the entry "source/**" of "paths" matches no file or directory`) {
			t.Errorf("message: %s", errs[0].Message)
		}
		if !strings.Contains(errs[2].Message, `of "paths-ignore"`) {
			t.Errorf("message: %s", errs[2].Message)
		}
	})

	t.Run("does not report what it cannot decide", func(t *testing.T) {
		errs := lintPathsFilter(t, copyRepo(), `on:
  push:
    paths:
      - '!missing/**'
      - '/missing/**'
      - 'missing/[!a]'
      - '${{ github.x }}'
      - 'src'
      - '**/main.go'
      - '**/*.go'
      - 'Makefile'
      - 'makefile'
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
`)
		checkLines(t, errs)
	})

	t.Run("an empty submodule hides the entries which could be in it", func(t *testing.T) {
		files := copyRepo()
		files[".gitmodules"] = "[submodule \"vendor/lib\"]\n\tpath = vendor/lib\n\turl = https://example.com/lib.git\n"
		files["vendor/readme.txt"] = "x"
		errs := lintPathsFilter(t, files, `on:
  push:
    paths:
      - 'vendor/lib/**'
      - '**/*.c'
      - 'nothing/**'
      - 'vendor/*.txt'
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
`)
		checkLines(t, errs)
	})

	t.Run("a generated file is not looked at", func(t *testing.T) {
		errs := lintPathsFilter(t, copyRepo(), `# This file was automatically generated by gh-aw. DO NOT EDIT.
on:
  push:
    paths:
      - 'missing/**'
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
`)
		checkLines(t, errs)
	})

	t.Run("a file which is not in a project is not looked at", func(t *testing.T) {
		l, err := NewLinter(io.Discard, &LinterOptions{})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = mustParseConfig(t, "profile: correctness\nrules:\n  unmatched-paths-filter: warn\n")
		errs, err := l.Lint("<stdin>", []byte("on:\n  push:\n    paths: ['missing/**']\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range errs {
			if e.ID == "unmatched-paths-filter" {
				t.Errorf("unexpected: %s", e)
			}
		}
	})
}

// TestUnmatchedPathsFilterDocsExample checks the example of docs/checks.md, which scripts/check-checks cannot lint
// because the rule needs the files of a repository.
func TestUnmatchedPathsFilterDocsExample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("docs", "checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	info, ok := LookupRule("unmatched-paths-filter")
	if !ok {
		t.Fatal("the rule is not registered")
	}
	start := strings.Index(doc, `<a id="`+info.DocsAnchor+`"></a>`)
	if start < 0 {
		t.Fatal("the section is missing")
	}
	section := doc[start+1:]
	if i := strings.Index(section, `<a id="`); i >= 0 {
		section = section[:i]
	}
	input, ok1 := docsBlock(section, "Example input, in a repository that has the directories `src` and `docs` (not `source` and not `doc`):\n\n```yaml\n")
	output, ok2 := docsBlock(section, "Output:\n<!-- Skip update output -->\n```\n")
	cfgText, ok3 := docsBlock(section, "```yaml\nrules:\n")
	if !ok1 || !ok2 || !ok3 {
		t.Fatal("the section has no example input, output or rules block")
	}
	dir := t.TempDir()
	for _, f := range []string{"src/main.go", "docs/index.md"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := parseConfig([]byte("rules:\n" + cfgText))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	l, err := NewLinter(&out, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withFixtureRules(cfg)
	if _, err := l.Lint("test.yaml", []byte(input), &Project{root: dir}); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(strings.TrimRight(output, "\n"), strings.TrimRight(out.String(), "\n")); diff != "" {
		t.Errorf("the output in docs/checks.md is outdated (-docs +linter):\n%s", diff)
	}
}

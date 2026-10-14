package jactionlint

import (
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
		"my-src", "my-src/code", "my-src/code/a.js", "db", "db/migrate-v1.sql", "Dockerfile", "v1.10", `back\slash.txt`, "star*.md", "x+y", "ünï", "ünï/x.txt",
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
		{pat: "docs/", want: true},     // read as docs
		{pat: "docs/*", want: true},
		{pat: "docs/**", want: true},
		{pat: "docs/**/*.md", want: true},
		{pat: "docs/*/*.md", want: true},
		{pat: "docs/*/*/*.md", want: false},
		{pat: "docs/**/start.md", want: true},
		{pat: "docs/missing/", want: false},
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
		// A backslash escapes only [ ? * + \ and !; anywhere else it is a character of the path (see glob.go)
		{pat: "main.g\\o", want: false},
		{pat: "back\\slash.txt", want: true},
		{pat: "star\\*.md", want: true},
		{pat: "x\\+y", want: true},
		{pat: "back\\\\slash.txt", want: true}, // an escaped backslash is one backslash
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
      - 'vendor/*.txt'
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
`)
		checkLines(t, errs)
	})

	t.Run("an entry that cannot be decided does not stop the others", func(t *testing.T) {
		files := copyRepo()
		files[".gitmodules"] = "[submodule \"vendor/lib\"]\n\tpath = vendor/lib\n\turl = https://example.com/lib.git\n"
		errs := lintPathsFilter(t, files, `on:
  push:
    paths:
      - '**/*.c'
      - 'nothing/**'
    paths-ignore:
      - 'vendor/lib/**'
      - 'missing.txt'
  pull_request:
    paths:
      - 'also-nothing'
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
`)
		checkLines(t, errs, "5", "8", "11")
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

// TestUnmatchedPathsFilterDoesNotWalkWhenOff checks that the repository is not read when the rule is off, which is
// the case in every run of the default profile.
func TestUnmatchedPathsFilterDoesNotWalkWhenOff(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	src := "on:\n  push:\n    paths: ['missing/**']\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	lint := func(cfg string) {
		l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: dir})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = mustParseConfig(t, cfg)
		if _, err := l.Lint("ci.yaml", []byte(src), &Project{root: dir}); err != nil {
			t.Fatal(err)
		}
	}
	walked := func() bool {
		repoPathsMu.Lock()
		defer repoPathsMu.Unlock()
		_, ok := repoPathsCache[dir]
		return ok
	}
	lint("profile: default\n")
	if walked() {
		t.Fatal("the repository was read although the rule is off in the default profile")
	}
	lint("profile: pedantic\n")
	if !walked() {
		t.Fatal("the repository was not read although the rule is on in the pedantic profile")
	}
}

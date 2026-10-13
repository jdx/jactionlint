package jactionlint

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pedanticSlim = "profile: default\nrules:\n  ubuntu-slim-limits:\n    pedantic: true\n"

func slimJob(extra string) string {
	return "on: push\njobs:\n  j:\n    runs-on: ubuntu-slim\n" + extra + "    steps:\n      - run: echo\n"
}

func slimFindings(errs []*Error) []string {
	var ret []string
	for _, e := range errs {
		if e.ID == "ubuntu-slim-limits" {
			ret = append(ret, e.Message)
		}
	}
	return ret
}

func TestUbuntuSlimLimits(t *testing.T) {
	for _, tc := range []struct {
		what string
		src  string
		want []string // substrings, in order of lines
		cfg  string   // the configuration, "profile: default" when empty
	}{
		{"timeout above 15", slimJob("    timeout-minutes: 16\n"), []string{`"timeout-minutes" is 16, but GitHub terminates a job on a single-CPU runner (ubuntu-slim) after 15 minutes`}, pedanticSlim},
		{"timeout above 15 is only pedantic", slimJob("    timeout-minutes: 16\n"), nil, ""},
		{"timeout above 15 with the option off", slimJob("    timeout-minutes: 16\n"), nil, "profile: pedantic\nrules:\n  ubuntu-slim-limits:\n    pedantic: false\n"},
		{"fractional timeout above 15", slimJob("    timeout-minutes: 15.5\n"), []string{"is 15.5"}, pedanticSlim},
		{"timeout of exactly 15", slimJob("    timeout-minutes: 15\n"), nil, ""},
		{"timeout of a step is not the job", slimJob("") + "      - run: echo\n        timeout-minutes: 60\n", nil, ""},
		{"no timeout", slimJob(""), nil, ""},
		{"timeout expression", slimJob("    timeout-minutes: ${{ fromJSON(vars.T) }}\n"), nil, ""},
		{"container", slimJob("    container: node:22\n"), []string{`"container" cannot be used on ubuntu-slim`}, ""},
		{"container with image", slimJob("    container:\n      image: node:22\n"), []string{`"container" cannot be used on ubuntu-slim`}, ""},
		{"services", slimJob("    services:\n      db:\n        image: postgres\n"), []string{`"services" cannot be used on ubuntu-slim`}, ""},
		{"docker step", "on: push\njobs:\n  j:\n    runs-on: ubuntu-slim\n    steps:\n      - uses: docker://alpine:3.22\n", []string{`Docker container action "docker://alpine:3.22" cannot run on ubuntu-slim`}, ""},
		{"run docker commands is not reported", "on: push\njobs:\n  j:\n    runs-on: ubuntu-slim\n    steps:\n      - run: docker build .\n", nil, ""},
		{"upper case label", "on: push\njobs:\n  j:\n    runs-on: Ubuntu-Slim\n    container: node:22\n    steps:\n      - run: echo\n", []string{"container"}, ""},
		{"list of one label", "on: push\njobs:\n  j:\n    runs-on: [ubuntu-slim]\n    timeout-minutes: 20\n    steps:\n      - run: echo\n", []string{"20"}, pedanticSlim},
		{"matrix of ubuntu-slim only", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    container: node:22\n    strategy:\n      matrix:\n        os: [ubuntu-slim]\n    steps:\n      - run: echo\n", []string{"container"}, ""},
		{"matrix include of ubuntu-slim only", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    container: node:22\n    strategy:\n      matrix:\n        include:\n          - os: ubuntu-slim\n          - os: ubuntu-slim\n            x: 1\n    steps:\n      - run: echo\n", []string{"container"}, ""},

		// The job may get another runner
		{"another runner", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 60\n    container: node:22\n    steps:\n      - run: echo\n", nil, ""},
		{"two labels", "on: push\njobs:\n  j:\n    runs-on: [ubuntu-slim, ubuntu-latest]\n    container: node:22\n    steps:\n      - run: echo\n", nil, ""},
		{"self-hosted with the label", "on: push\njobs:\n  j:\n    runs-on: [self-hosted, ubuntu-slim]\n    container: node:22\n    steps:\n      - run: echo\n", nil, ""},
		{"runner group", "on: push\njobs:\n  j:\n    runs-on:\n      group: slim\n    container: node:22\n    steps:\n      - run: echo\n", nil, ""},
		{"matrix with another runner", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    container: node:22\n    strategy:\n      matrix:\n        os: [ubuntu-slim, ubuntu-latest]\n    steps:\n      - run: echo\n", nil, ""},
		{"matrix include with another runner", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    container: node:22\n    strategy:\n      matrix:\n        os: [ubuntu-slim]\n        include:\n          - os: macos-latest\n    steps:\n      - run: echo\n", nil, ""},
		{"matrix value which is an expression", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    container: node:22\n    strategy:\n      matrix:\n        os: [ubuntu-slim, '${{ vars.OS }}']\n    steps:\n      - run: echo\n", nil, ""},
		{"matrix from JSON", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    container: node:22\n    strategy:\n      matrix: ${{ fromJSON(vars.M) }}\n    steps:\n      - run: echo\n", nil, ""},
		{"matrix row from JSON", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    container: node:22\n    strategy:\n      matrix:\n        os: ${{ fromJSON(vars.OS) }}\n    steps:\n      - run: echo\n", nil, ""},
		{"undefined matrix key", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    container: node:22\n    strategy:\n      matrix:\n        x: [1]\n    steps:\n      - run: echo\n", nil, ""},
		{"expression of vars", "on: push\njobs:\n  j:\n    runs-on: ${{ vars.RUNNER }}\n    container: node:22\n    steps:\n      - run: echo\n", nil, ""},
		{"a reusable workflow call", "on: push\njobs:\n  j:\n    uses: ./.github/workflows/x.yaml\n", nil, ""},
	} {
		t.Run(tc.what, func(t *testing.T) {
			cfg := tc.cfg
			if cfg == "" {
				cfg = "profile: default\n"
			}
			got := slimFindings(lintSlim(t, cfg, tc.src))
			if len(got) != len(tc.want) {
				t.Fatalf("want %d findings but got %q", len(tc.want), got)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("%q is not in %q", w, got[i])
				}
			}
		})
	}
}

func TestUbuntuSlimLimitsProfile(t *testing.T) {
	src := slimJob("    container: node:22\n")
	if got := slimFindings(lintSlim(t, "profile: correctness\n", src)); len(got) != 0 {
		t.Errorf("the correctness profile has the checks of actionlint and the detectors that need no knowledge of the platform limits: %v", got)
	}
	errs := lintSlim(t, "profile: default\n", src)
	n := 0
	for _, e := range errs {
		if e.ID == "ubuntu-slim-limits" {
			n++
			if e.Severity != SeverityError || e.Fix != nil {
				t.Errorf("want an error without a fix but got %v", e)
			}
		}
	}
	if n != 1 {
		t.Errorf("want one finding but got %v", errs)
	}
	if got := slimFindings(lintSlim(t, "profile: default\nrules:\n  ubuntu-slim-limits: off\n", src)); len(got) != 0 {
		t.Errorf("the rule can be turned off: %v", got)
	}
	// The pedantic profile adds the timeout, which the default profile leaves out
	long := slimJob("    timeout-minutes: 60\n")
	if got := slimFindings(lintSlim(t, "profile: default\n", long)); len(got) != 0 {
		t.Errorf("a timeout above 15 is pedantic: %v", got)
	}
	if got := slimFindings(lintSlim(t, "profile: pedantic\n", long)); len(got) != 1 {
		t.Errorf("the pedantic profile reports the timeout: %v", got)
	}
}

func TestUbuntuSlimLimitsLocalDockerAction(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".git/HEAD", "ref: refs/heads/main\n")
	write(".github/actions/dock/action.yml", "name: dock\ndescription: d\nruns:\n  using: docker\n  image: Dockerfile\n")
	write(".github/actions/node/action.yml", "name: node\ndescription: d\nruns:\n  using: node24\n  main: index.js\n")
	write(".github/workflows/ci.yaml", `on: push
jobs:
  slim:
    runs-on: ubuntu-slim
    steps:
      - uses: ./.github/actions/dock
      - uses: ./.github/actions/node
  full:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/dock
`)
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(&Config{Profile: ProfileDefault})
	errs, err := l.LintRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	got := slimFindings(errs)
	if len(got) != 1 || !strings.Contains(got[0], `"./.github/actions/dock" is a Docker container action (runs.using: docker)`) {
		t.Errorf("want one finding for the Docker action of the slim job but got %q (all: %v)", got, errs)
	}
}

func TestMissingTimeoutFixOnUbuntuSlim(t *testing.T) {
	fixed := func(cfg, runsOn string) (string, bool) {
		src := "on: push\njobs:\n  j:\n    runs-on: " + runsOn + "\n    steps:\n      - run: echo\n"
		for _, e := range lintSlimRaw(t, cfg, src) {
			if e.ID != "missing-timeout" {
				continue
			}
			if e.Fix == nil {
				return "", false
			}
			out, n := applyFixes([]byte(src), []*Error{e}, FixModeSafe)
			if n != 1 {
				t.Fatalf("applied %d fixes", n)
			}
			return string(out), true
		}
		t.Fatal("missing-timeout is not reported")
		return "", false
	}

	const opt = "rules:\n  missing-timeout:\n    default-minutes: %d\n"
	for _, tc := range []struct {
		what, cfg, runsOn, want string
	}{
		{"a long default is capped at 15 on ubuntu-slim", strings.Replace(opt, "%d", "60", 1), "ubuntu-slim", "timeout-minutes: 15\n"},
		{"a short default is kept on ubuntu-slim", strings.Replace(opt, "%d", "10", 1), "ubuntu-slim", "timeout-minutes: 10\n"},
		{"15 is kept on ubuntu-slim", strings.Replace(opt, "%d", "15", 1), "[ubuntu-slim]", "timeout-minutes: 15\n"},
		{"a long default is kept on other runners", strings.Replace(opt, "%d", "60", 1), "ubuntu-latest", "timeout-minutes: 60\n"},
		{"a long default is kept when the job can run elsewhere", strings.Replace(opt, "%d", "60", 1), "[ubuntu-slim, ubuntu-latest]", "timeout-minutes: 60\n"},
	} {
		t.Run(tc.what, func(t *testing.T) {
			out, ok := fixed("profile: default\n"+tc.cfg, tc.runsOn)
			if !ok || !strings.Contains(out, tc.want) {
				t.Errorf("want %q in %q (fix=%v)", tc.want, out, ok)
			}
		})
	}

	t.Run("no value is invented without the option", func(t *testing.T) {
		if out, ok := fixed("profile: default\n", "ubuntu-slim"); ok {
			t.Errorf("unexpected fix %q", out)
		}
	})

	t.Run("the fixed workflow has no finding of the slim rule", func(t *testing.T) {
		out, _ := fixed("profile: default\n"+strings.Replace(opt, "%d", "60", 1), "ubuntu-slim")
		if got := slimFindings(lintSlim(t, "profile: default\n", out)); len(got) != 0 {
			t.Errorf("%v", got)
		}
	})
}

func lintSlim(t *testing.T, cfg, src string) []*Error {
	t.Helper()
	return lintSlimConfig(t, withoutMissingTimeout(mustParseConfig(t, cfg)), src)
}

// lintSlimRaw keeps missing-timeout, which the other helper turns off.
func lintSlimRaw(t *testing.T, cfg, src string) []*Error {
	t.Helper()
	return lintSlimConfig(t, mustParseConfig(t, cfg), src)
}

func lintSlimConfig(t *testing.T, c *Config, src string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = c
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

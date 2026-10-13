package jactionlint

import (
	"bytes"
	"strings"
	"testing"
)

// TestNoFixSites checks the reason for each place where a rule that can fix a finding does not: a finding whose fix is
// unavailable says so (a code and a reason), and a finding that the rule never fixes does not.
func TestNoFixSites(t *testing.T) {
	const pedantic = "profile: pedantic\nrules:\n  missing-timeout: error\n  mutable-runner-label: warn\n"
	tests := []struct {
		name string
		file string
		cfg  string
		src  string
		id   string
		// match picks the finding when the rule reports several; empty matches the first one
		match string
		// code is the expected NoFixCode; empty means the finding must have no reason at all (advice only)
		code NoFixCode
	}{
		{
			name: "anonymous workflow with two jobs and no file name",
			file: "<stdin>", cfg: pedantic, id: "anonymous-definition", match: "workflow has no",
			src:  "on: push\njobs:\n  a:\n    name: a\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n  b:\n    name: b\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
			code: NoFixNeedsJudgment,
		},
		{
			name: "anonymous job in a flow mapping",
			file: "w.yaml", cfg: pedantic, id: "anonymous-definition", match: `job "a"`,
			src:  "name: w\non: push\njobs:\n  a: {runs-on: ubuntu-latest, steps: [{run: echo}]}\n",
			code: NoFixUnsupportedShape,
		},
		{
			name: "cancel-in-progress is an expression",
			file: "w.yaml", cfg: "", id: "concurrency-cancels-release",
			src:  "on:\n  push:\n    branches: [main]\npermissions: {}\njobs:\n  docs:\n    runs-on: ubuntu-latest\n    environment: github-pages\n    concurrency:\n      group: pages-${{ github.ref }}\n      cancel-in-progress: ${{ github.ref != 'refs/heads/gh-pages' }}\n    steps:\n      - run: echo deploy\n",
			code: NoFixNeedsJudgment,
		},
		{
			name: "concurrency of a pull request workflow with a job that has its own group",
			file: "w.yaml", cfg: "", id: "concurrency-limits",
			src:  "on: pull_request\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    concurrency: own\n    steps:\n      - run: echo\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
			code: NoFixNeedsJudgment,
		},
		{
			name: "concurrency of a push workflow is advice only",
			file: "w.yaml", cfg: "", id: "concurrency-limits",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
			code: "",
		},
		{
			name: "tee without -a cannot be edited",
			file: "w.yaml", cfg: "", id: "environment-file-redirect",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo A=1 >> $GITHUB_ENV\n          echo B=2 | tee $GITHUB_ENV\n",
			code: NoFixUnsupportedShape,
		},
		{
			name: "mutable runner label without a pin",
			file: "w.yaml", cfg: pedantic, id: "mutable-runner-label",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
			code: NoFixOptionRequired,
		},
		{
			name: "mutable runner label of a matrix",
			file: "w.yaml", cfg: "profile: pedantic\nrules:\n  mutable-runner-label:\n    level: warn\n    pin:\n      ubuntu-latest: ubuntu-24.04\n", id: "mutable-runner-label",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n    steps:\n      - run: echo\n",
			code: NoFixUnsupportedShape,
		},
		{
			name: "uses path that climbs out of the repository",
			file: "w.yaml", cfg: "", id: "obfuscation",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout/../../x@v4\n",
			code: NoFixNeedsJudgment,
		},
		{
			name: "constant expression is advice only",
			file: "w.yaml", cfg: "", id: "obfuscation",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          X: ${{ 'a' }}\n",
			code: "",
		},
		{
			name: "pipeline in a step that runs with sh",
			file: "w.yaml", cfg: "", id: "pipeline-without-pipefail",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - shell: sh\n        run: |\n          curl -s https://example.com | wc -l\n",
			code: NoFixNeedsJudgment,
		},
		{
			name: "pipeline next to one that quits early",
			file: "w.yaml", cfg: "", id: "pipeline-without-pipefail",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          curl -s https://example.com | wc -l\n          yes | head -n1\n",
			code: NoFixNeedsJudgment,
		},
		{
			name: "pipeline in a script that is not a literal block",
			file: "w.yaml", cfg: "", id: "pipeline-without-pipefail",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: curl -s https://example.com | wc -l\n",
			code: NoFixUnsupportedShape,
		},
		{
			name: "expansion in a PowerShell script",
			file: "w.yaml", cfg: "", id: "template-injection",
			src:  "on: issues\npermissions: {}\njobs:\n  a:\n    runs-on: windows-latest\n    steps:\n      - shell: pwsh\n        run: Write-Host \"${{ github.event.issue.title }}\"\n",
			code: NoFixUnsupportedShape,
		},
		{
			name: "expansion that is not a plain reference",
			file: "w.yaml", cfg: "", id: "template-injection",
			src:  "on: issues\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"${{ format('{0}!', github.event.issue.title) }}\"\n",
			code: NoFixUnsupportedShape,
		},
		{
			name: "expansion in the script of github-script is advice only",
			file: "w.yaml", cfg: "", id: "template-injection",
			src:  "on: issues\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/github-script@v7\n        with:\n          script: console.log('${{ github.event.issue.title }}')\n",
			code: "",
		},
		{
			name: "cargo install in a quoted word",
			file: "w.yaml", cfg: "", id: "unlocked-install", match: "cargo install",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: cargo \"install\" ripgrep\n",
			code: NoFixUnsupportedShape,
		},
		{
			name: "npm install is advice only",
			file: "w.yaml", cfg: "", id: "unlocked-install", match: "npm",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm install\n",
			code: "",
		},
		{
			name: "bot condition of a workflow with other events than pull requests",
			file: "w.yaml", cfg: "", id: "bot-conditions",
			src:  "on: [pull_request_target, workflow_dispatch]\npermissions: {}\njobs:\n  a:\n    if: github.actor == 'dependabot[bot]'\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
			code: NoFixNeedsJudgment,
		},
		{
			name: "insecure commands in a flow mapping",
			file: "w.yaml", cfg: "", id: "insecure-commands",
			src:  "on: push\npermissions: {}\nenv: {ACTIONS_ALLOW_UNSECURE_COMMANDS: true, B: x}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
			code: NoFixUnsupportedShape,
		},
		{
			name: "self-repository path with an anchor",
			file: "w.yaml", cfg: "rules:\n  self-repository: warn\n", id: "self-repository",
			src:  "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: &p ./.github/actions/x\n",
			code: NoFixUnsupportedShape,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cfg *Config
			if tc.cfg != "" {
				cfg = mustParseConfig(t, tc.cfg)
			} else {
				cfg = mustParseConfig(t, "profile: pedantic\n")
			}
			l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Config: cfg})
			if err != nil {
				t.Fatal(err)
			}
			errs, err := l.Lint(tc.file, []byte(tc.src), nil)
			if err != nil {
				t.Fatal(err)
			}
			var e *Error
			for _, x := range errs {
				if x.ID == tc.id && strings.Contains(x.Message, tc.match) {
					e = x
					break
				}
			}
			if e == nil {
				t.Fatalf("no %s finding in %v", tc.id, errs)
			}
			if e.Fix != nil {
				t.Fatalf("the finding has a fix: %+v", e.Fix)
			}
			switch {
			case tc.code == "" && e.NoFix != nil:
				t.Errorf("an advice-only finding has a reason: %+v", e.NoFix)
			case tc.code != "" && e.NoFix == nil:
				t.Errorf("no reason, want %s", tc.code)
			case tc.code != "" && e.NoFix.Code != tc.code:
				t.Errorf("code %s (%s), want %s", e.NoFix.Code, e.NoFix.Reason, tc.code)
			case tc.code != "" && e.NoFix.Reason == "":
				t.Errorf("the reason is empty")
			}
		})
	}
}

// The reason of a script that no fix is written for names what stands in the way, not only that it is not bash or sh.
func TestNoFixShellReasons(t *testing.T) {
	const tail = "      - run: echo \"${{ github.event.issue.title }}\"\n"
	tests := []struct {
		name, job, want, notWant string
	}{
		{"pwsh", "    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: pwsh\n    steps:\n", `with the shell "pwsh"`, ""},
		{"windows runner", "    runs-on: windows-latest\n    steps:\n", "runs on Windows", "another shell"},
		{"self-hosted runner", "    runs-on: [self-hosted, gpu]\n    steps:\n", "self-hosted or custom runner", "Windows"},
		{"runs-on expression", "    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n    steps:\n", "chosen by an expression", "Windows"},
		{"shell expression", "    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: ${{ vars.SHELL }}\n    steps:\n", "set by an expression", ""},
		{"bash template", "    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: bash -c \"{0}\"\n    steps:\n", "options or a template that the fix does not understand", "bash and sh only"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "on: issues\npermissions: {}\njobs:\n  a:\n" + tc.job + tail
			l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Config: mustParseConfig(t, "profile: pedantic\n")})
			if err != nil {
				t.Fatal(err)
			}
			errs, err := l.Lint("w.yaml", []byte(src), nil)
			if err != nil {
				t.Fatal(err)
			}
			e := findNoFix(errs, "template-injection")
			if e == nil || e.NoFix == nil {
				t.Fatalf("no reason: %+v", errs)
			}
			if e.NoFix.Code != NoFixUnsupportedShape || !strings.Contains(e.NoFix.Reason, tc.want) || (tc.notWant != "" && strings.Contains(e.NoFix.Reason, tc.notWant)) {
				t.Errorf("reason %q: want %q, not %q", e.NoFix.Reason, tc.want, tc.notWant)
			}
		})
	}
}

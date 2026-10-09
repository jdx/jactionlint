package jactionlint

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allBatchDRules enables every rule of the batch.
func allBatchDRules() *Config {
	c := &Config{Rules: map[string]RuleConfig{}}
	for _, id := range batchDDefaultRules {
		c.Rules[id] = RuleConfig{Level: SeverityError}
	}
	for _, id := range []string{"unlocked-install", "unpinned-tools", "superfluous-actions"} {
		c.Rules[id] = RuleConfig{Level: SeverityError, Options: map[string]any{"pedantic": true}}
	}
	return c
}

func lintBatchD(t *testing.T, name string, src string, cfg *Config) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(cfg)
	errs, err := l.Lint(name, []byte(src), &Project{root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func TestBatchDNoFindings(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "ok", "batch_d_*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range lintBatchD(t, f, string(b), allBatchDRules()) {
			t.Errorf("%s", e)
		}
	}
}

func fixProject(t *testing.T, content string) (root, path string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, "ci.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func newBatchDLinter(t *testing.T, root string) *Linter {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(&Config{Rules: map[string]RuleConfig{"unlocked-install": {Level: SeverityError}}})
	return l
}

// The fix of unlocked-install inserts --locked after "install". It is unsafe, it works in every form of scalar where
// the position of the word is known, and it leaves the other forms alone. Afterwards the file lints clean.
func TestUnlockedInstallFix(t *testing.T) {
	const head = "# こんにちは: multi-byte characters in front\non: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		name string
		step string
		want string // the fixed step, "" when no fix is expected
	}{
		{"plain", "      - run: cargo install ripgrep\n", "      - run: cargo install --locked ripgrep\n"},
		{"double quoted", "      - run: \"cargo install ripgrep\"\n", "      - run: \"cargo install --locked ripgrep\"\n"},
		{"single quoted", "      - run: 'cargo install ripgrep'\n", "      - run: 'cargo install --locked ripgrep'\n"},
		{"literal block", "      - run: |\n          set -e\n          cargo install ripgrep\n", "      - run: |\n          set -e\n          cargo install --locked ripgrep\n"},
		{"literal block, strip", "      - run: |-\n          cargo install ripgrep\n", "      - run: |-\n          cargo install --locked ripgrep\n"},
		{"two commands on a line", "      - run: cargo install a && cargo install b\n", "      - run: cargo install --locked a && cargo install --locked b\n"},
		{"flow mapping", "      - {run: cargo install ripgrep}\n", "      - {run: cargo install --locked ripgrep}\n"},
		{"wrapped in sudo", "      - run: sudo cargo install ripgrep\n", "      - run: sudo cargo install --locked ripgrep\n"},
		{"multi-byte characters in the script", "      - run: |\n          echo こんにちは && cargo install ripgrep\n", "      - run: |\n          echo こんにちは && cargo install --locked ripgrep\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, crlf := range []bool{false, true} {
				in, wantFixed := head+tc.step, head+tc.want
				if crlf {
					in, wantFixed = strings.ReplaceAll(in, "\n", "\r\n"), strings.ReplaceAll(wantFixed, "\n", "\r\n")
				}
				root, path := fixProject(t, in)

				// Not applied unless it is asked for
				res, err := newBatchDLinter(t, root).FixFiles([]string{path}, nil, FixModeSafe)
				if err != nil {
					t.Fatal(err)
				}
				if b, _ := os.ReadFile(path); string(b) != in || res.Applied != 0 {
					t.Fatalf("the safe mode must not change the file (crlf=%v): %q", crlf, b)
				}

				res, err = newBatchDLinter(t, root).FixFiles([]string{path}, nil, FixModeUnsafe)
				if err != nil {
					t.Fatal(err)
				}
				b, _ := os.ReadFile(path)
				if string(b) != wantFixed {
					t.Errorf("crlf=%v: want\n%q\nbut got\n%q", crlf, wantFixed, b)
				}
				if len(res.Errors) != 0 {
					t.Errorf("crlf=%v: the fixed file still has errors: %v", crlf, res.Errors)
				}
			}
		})
	}
}

// Where the position of the word is not known for sure, no fix is attached and the finding stays.
func TestUnlockedInstallNoFix(t *testing.T) {
	const head = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	for name, step := range map[string]string{
		"folded block":              "      - run: >\n          cargo\n          install ripgrep\n",
		"multi-line plain scalar":   "      - run: cargo\n          install ripgrep\n",
		"escape in a quoted scalar": "      - run: \"echo \\\"x\\\" && cargo install ripgrep\"\n",
		"install is quoted":         "      - run: cargo 'install' ripgrep\n",
	} {
		t.Run(name, func(t *testing.T) {
			errs := lintBatchD(t, "test.yaml", head+step, &Config{Rules: map[string]RuleConfig{"unlocked-install": {Level: SeverityError}}})
			found := false
			for _, e := range errs {
				if e.ID == "unlocked-install" {
					found = true
					if e.Fix != nil {
						t.Errorf("a fix was attached though the position is not known: %+v", e.Fix)
					}
				}
			}
			if !found {
				t.Errorf("no finding: %v", errs)
			}
		})
	}
}

func TestExprJudgement(t *testing.T) {
	tests := []struct {
		expr      string
		untrusted bool
		trusted   bool
	}{
		{"github.event.issue.title", true, false},
		{"github.head_ref", true, false},
		{"github.event.commits[0].message", true, false},
		{"github.event.pull_request.head.ref", true, false},
		{"contains(github.event.issue.title, 'x')", false, false},
		{"github.sha", false, true},
		{"github.event.pull_request.head.sha", false, true},
		{"github.event.pull_request.number", false, true},
		{"format('{0}-{1}', github.run_id, runner.os)", false, true},
		{"matrix.os", false, true},
		{"secrets.TOKEN", false, true},
		{"vars.X || 'x'", false, true},
		{"'literal'", false, true},
		{"steps.build.outputs.path", false, false},
		{"github.event.pull_request.user.login", false, false},
		{"github.event.pull_request.title", true, false},
		{"github.event.number || github.event.issue.title", true, false},
		{"github", false, false},
		{"env.FOO", false, false},
		{"not valid ((", false, false},
	}
	for _, tc := range tests {
		if got := exprReadsUntrustedInput(tc.expr); got != tc.untrusted {
			t.Errorf("exprReadsUntrustedInput(%q) = %v, want %v", tc.expr, got, tc.untrusted)
		}
		if got := exprIsTrusted(tc.expr); got != tc.trusted {
			t.Errorf("exprIsTrusted(%q) = %v, want %v", tc.expr, got, tc.trusted)
		}
	}
}

func TestExprsIn(t *testing.T) {
	for in, want := range map[string][]string{
		"no expression":                        nil,
		"${{ a }}":                             {"a"},
		"x ${{ a }} y ${{b}}":                  {"a", "b"},
		"${{ format('}}{0}', github.sha) }} z": {"format('}}{0}', github.sha)"},
		"${{ unterminated":                     nil,
		"${{ 'it''s }}' }}":                    {"'it''s }}'"},
		"${{ a }}\n${{ b\n || c }}":            {"a", "b\n || c"},
	} {
		got := exprsIn(in)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("exprsIn(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every entry of the tables names its source, is in canonical form and appears once.
func TestBatchDTables(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range superfluousActions {
		checkTableEntry(t, "superfluousActions", a.action, a.source, seen)
		if a.instead == "" {
			t.Errorf("%s has no replacement", a.action)
		}
	}
	seen = map[string]bool{}
	for _, a := range floatingToolActions {
		checkTableEntry(t, "floatingToolActions", a.action, a.source, seen)
		if a.input == "" || len(a.floating) == 0 {
			t.Errorf("%s has no input or no floating value", a.action)
		}
		for _, f := range a.floating {
			if f != strings.ToLower(f) {
				t.Errorf("%s: floating value %q must be lower case", a.action, f)
			}
		}
		if a.input != strings.ToLower(a.input) {
			t.Errorf("%s: input %q must be lower case, inputs are matched in lower case", a.action, a.input)
		}
	}
}

func checkTableEntry(t *testing.T, table, action, source string, seen map[string]bool) {
	t.Helper()
	if action != strings.ToLower(action) || strings.Count(action, "/") != 1 {
		t.Errorf("%s: %q must be owner/repo in lower case", table, action)
	}
	if source == "" {
		t.Errorf("%s: %q has no source", table, action)
	}
	if seen[action] {
		t.Errorf("%s: %q is listed twice", table, action)
	}
	seen[action] = true
}

// The label of a finding is its ID, which is what goes into the configuration, not the name of the rule.
func TestBatchDLabelIsID(t *testing.T) {
	src := "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"A=$(cat v.txt)\" >> $GITHUB_ENV\n      - run: cargo install foo\n      - run: npm i foo\n"
	errs := lintBatchD(t, "test.yaml", src, allBatchDRules())
	var ids []string
	for _, e := range errs {
		if e.Kind != e.ID {
			t.Errorf("%s: the label %q must be the ID %q", e.Message, e.Kind, e.ID)
		}
		ids = append(ids, e.ID)
	}
	for _, id := range []string{"github-env", "unlocked-install", "unpinned-tools", "adhoc-packages"} {
		found := false
		for _, i := range ids {
			found = found || i == id
		}
		if !found {
			t.Errorf("%q is not reported: %v", id, ids)
		}
	}
}

// The default shell of a Windows runner is not bash, so its scripts are not read as bash. An explicit shell, or a
// default shell of the job or the workflow, decides.
func TestBatchDShells(t *testing.T) {
	cfg := allBatchDRules()
	count := func(src string) int {
		n := 0
		for _, e := range lintBatchD(t, "test.yaml", src, cfg) {
			if e.ID == "adhoc-packages" {
				n++
			}
		}
		return n
	}
	for name, tc := range map[string]struct {
		src  string
		want int
	}{
		"linux default":                {"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm install foo\n", 1},
		"windows default is pwsh":      {"on: push\njobs:\n  j:\n    runs-on: windows-latest\n    steps:\n      - run: npm install foo\n", 0},
		"windows with bash":            {"on: push\njobs:\n  j:\n    runs-on: windows-latest\n    steps:\n      - run: npm install foo\n        shell: bash\n", 1},
		"windows job default":          {"on: push\njobs:\n  j:\n    runs-on: windows-latest\n    defaults:\n      run:\n        shell: bash\n    steps:\n      - run: npm install foo\n", 1},
		"workflow default pwsh":        {"on: push\ndefaults:\n  run:\n    shell: pwsh\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm install foo\n", 0},
		"custom shell":                 {"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm install foo\n        shell: bash -e {0}\n", 1},
		"python":                       {"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm install foo\n        shell: python\n", 0},
		"shell is an expression":       {"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm install foo\n        shell: ${{ github.sha }}\n", 0},
		"a script that does not parse": {"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: if then npm install foo\n", 0},
		"parallel steps":               {"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - parallel:\n          - run: npm install foo\n          - run: npm install bar\n", 2},
	} {
		if got := count(tc.src); got != tc.want {
			t.Errorf("%s: %d findings, want %d", name, got, tc.want)
		}
	}
}

// Positions are those of the file: with CRLF line breaks, with a literal block, and after multi-byte characters.
func TestBatchDPositions(t *testing.T) {
	src := "on: push\r\njobs:\r\n  j:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - run: |\r\n          echo こんにちは; npm install foo\r\n"
	var got []*Error
	for _, e := range lintBatchD(t, "test.yaml", src, allBatchDRules()) {
		if e.ID == "adhoc-packages" {
			got = append(got, e)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want one finding, got %v", got)
	}
	e := got[0]
	// "          echo こんにちは; " is 10 spaces and 12 characters
	if e.Line != 7 || e.Column != 23 {
		t.Errorf("position is %d:%d, want 7:23", e.Line, e.Column)
	}
	if e.EndLine != 7 || e.EndColumn != 23+len("npm install foo") {
		t.Errorf("end is %d:%d", e.EndLine, e.EndColumn)
	}
}

// A reusable workflow without permissions runs with those of its caller, which may grant id-token: write.
func TestUseTrustedPublishingReusableWorkflow(t *testing.T) {
	count := func(src string) int {
		n := 0
		for _, e := range lintBatchD(t, "test.yaml", src, allBatchDRules()) {
			if e.ID == "use-trusted-publishing" {
				n++
			}
		}
		return n
	}
	const steps = "    runs-on: ubuntu-latest\n    steps:\n      - run: cargo publish\n"
	for name, tc := range map[string]struct {
		src  string
		want int
	}{
		"reusable, permissions unknown":    {"on: workflow_call\njobs:\n  j:\n" + steps, 0},
		"reusable, job without id-token":   {"on: workflow_call\njobs:\n  j:\n    permissions:\n      contents: read\n" + steps, 1},
		"reusable, workflow permissions":   {"on: workflow_call\npermissions: {}\njobs:\n  j:\n" + steps, 1},
		"not reusable":                     {"on: push\njobs:\n  j:\n" + steps, 1},
		"id-token at the workflow":         {"on: push\npermissions:\n  id-token: write\njobs:\n  j:\n" + steps, 0},
		"id-token at the job wins":         {"on: push\npermissions:\n  id-token: write\njobs:\n  j:\n    permissions:\n      contents: read\n" + steps, 1},
		"write-all":                        {"on: push\npermissions: write-all\njobs:\n  j:\n" + steps, 0},
		"read-all does not grant id-token": {"on: push\npermissions: read-all\njobs:\n  j:\n" + steps, 1},
		"id-token: read does not grant it": {"on: push\npermissions:\n  id-token: read\njobs:\n  j:\n" + steps, 1},
	} {
		if got := count(tc.src); got != tc.want {
			t.Errorf("%s: %d findings, want %d", name, got, tc.want)
		}
	}
}

func TestGitHubEnvTriggersAndNames(t *testing.T) {
	cfg := allBatchDRules()
	count := func(src string) int {
		n := 0
		for _, e := range lintBatchD(t, "test.yaml", src, cfg) {
			if e.ID == "github-env" || e.ID == "github-env-untrusted-input" {
				n++
			}
		}
		return n
	}
	const job = "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"A=$(cat v)\" >> \"$GITHUB_ENV\"\n"
	for name, tc := range map[string]struct {
		src  string
		want int
	}{
		"string trigger":                     {"on: pull_request_target\n" + job, 1},
		"list of triggers":                   {"on: [push, workflow_run]\n" + job, 1},
		"mapping with types":                 {"on:\n  workflow_run:\n    workflows: [a]\n" + job, 1},
		"pull_request is not":                {"on: pull_request\n" + job, 0},
		"push is not":                        {"on: push\n" + job, 0},
		"workflow_call is not":               {"on: workflow_call\n" + job, 0},
		"case of an env variable":            {"on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"A=$TITLE\" >> \"$GITHUB_ENV\"\n        env:\n          title: ${{ github.event.issue.title }}\n", 0},
		"same case":                          {"on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"A=$TITLE\" >> \"$GITHUB_ENV\"\n        env:\n          TITLE: ${{ github.event.issue.title }}\n", 1},
		"env of the job":                     {"on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n    env:\n      TITLE: ${{ github.event.issue.title }}\n    steps:\n      - run: echo \"A=$TITLE\" >> \"$GITHUB_ENV\"\n", 1},
		"step env overrides":                 {"on: issues\nenv:\n  TITLE: ${{ github.event.issue.title }}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"A=$TITLE\" >> \"$GITHUB_ENV\"\n        env:\n          TITLE: fixed\n", 0},
		"shell assignment wins":              {"on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          TITLE=fixed\n          echo \"A=$TITLE\" >> \"$GITHUB_ENV\"\n        env:\n          TITLE: ${{ github.event.issue.title }}\n", 0},
		"expression without spaces":          {"on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"A=${{github.event.issue.title}}\" >> \"$GITHUB_ENV\"\n", 1},
		"written to another file":            {"on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"A=${{ github.event.issue.title }}\" >> \"$GITHUB_OUTPUT\"\n", 0},
		"to a name that only starts like it": {"on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"A=${{ github.event.issue.title }}\" >> \"$GITHUB_ENV_FILE\"\n", 0},
		"crlf":                               {strings.ReplaceAll("on: pull_request_target\n"+job, "\n", "\r\n"), 1},
	} {
		if got := count(tc.src); got != tc.want {
			t.Errorf("%s: %d findings, want %d", name, got, tc.want)
		}
	}
}

// The pedantic persona of an audit is the option "pedantic" of the same rule, on by default under the strict
// and all profiles only.
func TestBatchDPedanticOption(t *testing.T) {
	src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm install\n      - uses: stefanzweifel/git-auto-commit-action@v5\n      - run: pip install requests\n"
	count := func(c *Config, id string) int {
		n := 0
		for _, e := range lintBatchD(t, "test.yaml", src, c) {
			if e.ID == id {
				n++
			}
		}
		return n
	}
	for _, id := range []string{"unlocked-install", "superfluous-actions", "unpinned-tools"} {
		on := func(extra map[string]any) *Config {
			return &Config{Rules: map[string]RuleConfig{id: {Level: SeverityError, Options: extra}}}
		}
		if n := count(on(nil), id); n != 0 {
			t.Errorf("%s: %d pedantic findings without the option under the default profile", id, n)
		}
		if n := count(on(map[string]any{"pedantic": true}), id); n == 0 {
			t.Errorf("%s: no pedantic finding with the option", id)
		}
		strict := on(nil)
		strict.Profile = ProfileStrict
		if n := count(strict, id); n == 0 {
			t.Errorf("%s: no pedantic finding under the strict profile", id)
		}
		strictOff := on(map[string]any{"pedantic": false})
		strictOff.Profile = ProfileStrict
		if n := count(strictOff, id); n != 0 {
			t.Errorf("%s: %d pedantic findings with pedantic: false under the strict profile", id, n)
		}
	}
}

func countBatchD(t *testing.T, id, src string) int {
	t.Helper()
	n := 0
	for _, e := range lintBatchD(t, "test.yaml", src, allBatchDRules()) {
		if e.ID == id {
			n++
		}
	}
	return n
}

// The publish commands the analyzer does not know are skipped for a dry run and a registry of your own, like the ones it knows.
func TestUseTrustedPublishingFallbackCommands(t *testing.T) {
	const head = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: "
	for run, want := range map[string]int{
		"pdm publish": 1,
		"pdm publish --repository https://pypi.example.com/":                0,
		"pdm publish -r https://pypi.example.com/":                          0,
		"pdm publish --repository=https://pypi.example.com/":                0,
		"pdm publish --repository pypi":                                     1,
		"pdm publish --dry-run":                                             0,
		"nuget push x.nupkg":                                                1,
		"nuget push x.nupkg -Source https://feed.example.com/v3/index.json": 0,
		"nuget push x.nupkg -Source https://api.nuget.org/v3/index.json":    1,
		"dotnet nuget push x.nupkg":                                         1,
		"dotnet nuget push x.nupkg --source https://feed.example.com":       0,
		"dotnet nuget push x.nupkg -s https://feed.example.com":             0,
		"dotnet nuget push x.nupkg --source ${{ vars.FEED }}":               0,
		"uvx twine upload dist/*":                                           1,
		"uvx twine upload --repository-url https://x.example.com dist/*":    0,
	} {
		if got := countBatchD(t, "use-trusted-publishing", head+run+"\n"); got != want {
			t.Errorf("%q: %d findings, want %d", run, got, want)
		}
	}
}

// Another event next to workflow_call starts the workflow with the default token, which no caller can raise.
func TestUseTrustedPublishingReusableWithOtherEvents(t *testing.T) {
	const steps = "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: cargo publish\n"
	for name, tc := range map[string]struct {
		on   string
		want int
	}{
		"only workflow_call":       {"on: workflow_call\n", 0},
		"workflow_call and push":   {"on: [workflow_call, push]\n", 1},
		"workflow_call, release":   {"on:\n  workflow_call:\n  release:\n    types: [published]\n", 1},
		"workflow_call with input": {"on:\n  workflow_call:\n    inputs:\n      x:\n        type: string\n", 0},
	} {
		if got := countBatchD(t, "use-trusted-publishing", tc.on+steps); got != tc.want {
			t.Errorf("%s: %d findings, want %d", name, got, tc.want)
		}
	}
}

// `${{github.sha}}` is the same expression as `${{ github.sha }}` for the files and the here documents.
func TestGitHubEnvCompactExpressions(t *testing.T) {
	const head = "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	for name, step := range map[string]string{
		"heredoc, spaced":    "      - run: |\n          cat <<EOF >> $GITHUB_ENV\n          SHA=${{ github.sha }}\n          EOF\n",
		"heredoc, compact":   "      - run: |\n          cat <<EOF >> $GITHUB_ENV\n          SHA=${{github.sha}}\n          EOF\n",
		"pwsh line, spaced":  "      - shell: pwsh\n        run: Add-Content $env:GITHUB_ENV \"SHA=${{ github.sha }}\"\n",
		"pwsh line, compact": "      - shell: pwsh\n        run: Add-Content $env:GITHUB_ENV \"SHA=${{github.sha}}\"\n",
		"cmd line, compact":  "      - shell: cmd\n        run: echo SHA=${{github.sha}}>> %GITHUB_ENV%\n",
	} {
		if got := countBatchD(t, "github-env", head+step); got != 0 {
			t.Errorf("%s: %d findings for a trusted value, want 0", name, got)
		}
	}
}

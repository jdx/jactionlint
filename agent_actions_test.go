package jactionlint

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestShellWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{``, nil},
		{`--model opus`, []string{"--model", "opus"}},
		{`--allowedTools "Bash(git diff:*),Edit"  --x`, []string{"--allowedTools", "Bash(git diff:*),Edit", "--x"}},
		{`--a 'it is' b\ c`, []string{"--a", "it is", "b c"}},
		{"--a\n--b\r\n  --c", []string{"--a", "--b", "--c"}},
		{`"" x`, []string{"", "x"}},
		{`--p="a b"`, []string{"--p=a b"}},
		{`"unterminated quote`, []string{"unterminated quote"}},
	}
	for _, tc := range tests {
		if got := shellWords(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("shellWords(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSplitToolList(t *testing.T) {
	got := splitToolList("Bash(git diff:*),Edit Read\n  Bash(a, b)")
	want := []string{"Bash(git diff:*)", "Edit", "Read", "Bash(a, b)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRiskyToolRule(t *testing.T) {
	risky := []string{
		"Bash", "Bash(*)", "Bash(:*)", "bash(*:*)", "Bash()", "Bash(python:*)", "Bash(python3 *)", "Bash(sh*)", "Bash(curl:*)",
		"Bash(xargs:*)", "WebFetch", "WebFetch(*)", "WebFetch(domain:*)",
	}
	for _, r := range risky {
		if riskyToolRule(r, "Bash", "WebFetch") == "" {
			t.Errorf("%q should be risky", r)
		}
	}
	fine := []string{
		"Edit", "Read", "Bash(git diff:*)", "Bash(npm test)", "Bash(python -m pytest:*)", "Bash(python)", "Bash(npx prettier --check:*)",
		"WebFetch(domain:docs.github.com)", "Bashful", "Bash(gh issue view:*)", "mcp__github__get_issue",
	}
	for _, r := range fine {
		if why := riskyToolRule(r, "Bash", "WebFetch"); why != "" {
			t.Errorf("%q should be fine but: %s", r, why)
		}
	}
	if riskyToolRule("WebFetch", "Bash", "") != "" {
		t.Error("the web tool must be ignored when the agent has none")
	}
	if riskyToolRule("shell(bash:*)", "shell", "") == "" || riskyToolRule("run_shell_command", "run_shell_command", "web_fetch") == "" {
		t.Error("the shell tool of other agents is not recognised")
	}
}

func TestAgentTableIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range agentActions {
		if a.Name != strings.ToLower(a.Name) || strings.Contains(a.Name, "@") || strings.Count(a.Name, "/") < 1 {
			t.Errorf("%q is not a lower case owner/repo[/path]", a.Name)
		}
		if seen[a.Name] {
			t.Errorf("%q is listed twice", a.Name)
		}
		seen[a.Name] = true
		if a.Title == "" || !strings.HasPrefix(a.Source, "https://") {
			t.Errorf("%q needs a title and a source", a.Name)
		}
		for _, in := range append(append([]string{}, a.Prompts...), a.Config...) {
			if in != strings.ToLower(in) {
				t.Errorf("%q: input %q must be lower case", a.Name, in)
			}
		}
		for _, g := range a.OpenGate {
			if !a.Gated {
				t.Errorf("%q has open gate inputs but is not gated", a.Name)
			}
			if g.Input != strings.ToLower(g.Input) || g.Wildcard == "" || g.Who == "" {
				t.Errorf("%q: bad open gate %+v", a.Name, g)
			}
		}
		if !a.Gated && len(a.OpenGate) > 0 {
			t.Errorf("%q", a.Name)
		}
	}
	if len(agentActionNames()) != len(agentActions) {
		t.Error("agentActionNames lost an entry")
	}
}

func TestCodeExecTableIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range codeExecTable {
		if e.Action != strings.ToLower(e.Action) || strings.Count(e.Action, "/") != 1 {
			t.Errorf("%q is not a lower case owner/repo", e.Action)
		}
		if seen[e.Action] {
			t.Errorf("%q is listed twice", e.Action)
		}
		seen[e.Action] = true
		if !strings.HasPrefix(e.Source, "https://") || len(e.Inputs) == 0 {
			t.Errorf("%q needs inputs and a source", e.Action)
		}
		for _, in := range e.Inputs {
			if in != strings.ToLower(in) {
				t.Errorf("%q: input %q must be lower case", e.Action, in)
			}
			if !isCodeExecInput(e.Action+"@v1", in) {
				t.Errorf("%q: %q is not found", e.Action, in)
			}
		}
	}
	// An agent action is not in the table: its inputs are prompts, with their own message
	for _, a := range agentActions {
		if seen[a.Name] {
			t.Errorf("%q is both an agent and a code input", a.Name)
		}
	}
}

func lintAgentTest(t *testing.T, src string, enable ...string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig(enable...)
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func messagesOfID(errs []*Error, id string) []string {
	var ret []string
	for _, e := range errs {
		if e.ID == id {
			ret = append(ret, e.Message)
		}
	}
	return ret
}

func TestAgenticActionsGuards(t *testing.T) {
	const head = "on: issues\njobs:\n  a:\n    runs-on: ubuntu-latest\n"
	tests := []struct {
		name string
		body string
		want int
	}{
		{"unguarded", "    steps:\n      - uses: actions/ai-inference@v3\n", 1},
		{"job if on the association", "    if: github.event.issue.author_association == 'OWNER'\n    steps:\n      - uses: actions/ai-inference@v3\n", 0},
		{"step if on the actor", "    steps:\n      - uses: actions/ai-inference@v3\n        if: github.actor == 'octocat'\n", 0},
		{"label", "    if: github.event.label.name == 'ai'\n    steps:\n      - uses: actions/ai-inference@v3\n", 0},
		{"environment", "    environment: ai\n    steps:\n      - uses: actions/ai-inference@v3\n", 0},
		{"permission step", "    steps:\n      - uses: actions-cool/check-user-permission@v2\n      - uses: actions/ai-inference@v3\n", 0},
		{"collaborator check", "    steps:\n      - run: gh api repos/$R/collaborators/$U/permission\n      - uses: actions/ai-inference@v3\n", 0},
		{"a condition that merely mentions the word later is not a guard", "    steps:\n      - uses: actions/ai-inference@v3\n      - run: echo permission\n", 1},
		{"unrelated if", "    if: github.event.issue.title != ''\n    steps:\n      - uses: actions/ai-inference@v3\n", 1},
		{"expression uses is skipped", "    steps:\n      - uses: ${{ github.event.issue.title }}\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := messagesOfID(lintAgentTest(t, head+tc.body), "agentic-actions")
			if len(got) != tc.want {
				t.Fatalf("want %d findings, got %q", tc.want, got)
			}
		})
	}
}

func TestAgenticActionsNeedsJobGuard(t *testing.T) {
	const src = `on: issue_comment
jobs:
  check:
    runs-on: ubuntu-latest
    if: github.event.comment.author_association == 'MEMBER'
    steps:
      - run: echo ok
  agent:
    needs: [Check]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/ai-inference@v3
`
	if got := messagesOfID(lintAgentTest(t, src), "agentic-actions"); len(got) != 0 {
		t.Fatalf("the job it needs restricts the user: %q", got)
	}
}

func TestAgenticActionsPullRequestCheckout(t *testing.T) {
	const head = "on: pull_request_target\njobs:\n  a:\n    runs-on: ubuntu-latest\n    environment: x\n    steps:\n"
	tests := []struct {
		name  string
		steps string
		want  int
	}{
		{"head sha", "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n", 1},
		{"fork repository", "      - uses: actions/checkout@v4\n        with:\n          repository: ${{ github.event.pull_request.head.repo.full_name }}\n", 1},
		{"head ref", "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.head_ref }}\n", 1},
		{"gh pr checkout", "      - run: gh pr checkout ${{ github.event.number }}\n", 1},
		{"subdirectory", "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n          path: pr\n", 0},
		{"base branch", "      - uses: actions/checkout@v4\n", 0},
		{"checkout after the agent", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := head + tc.steps + "      - uses: anthropics/claude-code-action@v1\n"
			if tc.name == "checkout after the agent" {
				src = head + "      - uses: anthropics/claude-code-action@v1\n      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n"
			}
			got := messagesOfID(lintAgentTest(t, src), "agentic-actions")
			if len(got) != tc.want {
				t.Fatalf("want %d findings, got %q", tc.want, got)
			}
		})
	}
}

func TestAgenticActionsCodexOnWindows(t *testing.T) {
	const src = "on: issues\njobs:\n  a:\n    runs-on: [self-hosted, Windows]\n    environment: x\n    steps:\n      - uses: openai/codex-action@v1\n        with:\n          safety-strategy: unsafe\n"
	if got := messagesOfID(lintAgentTest(t, src), "agentic-actions"); len(got) != 0 {
		t.Fatalf("Windows runners need it: %q", got)
	}
	if got := messagesOfID(lintAgentTest(t, strings.Replace(src, "[self-hosted, Windows]", "ubuntu-latest", 1)), "agentic-actions"); len(got) != 1 {
		t.Fatalf("want 1 finding, got %q", got)
	}
}

func TestAgenticActionsOff(t *testing.T) {
	const src = "on: issues\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/ai-inference@v3\n"
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{Rules: map[string]RuleConfig{"agentic-actions": {Level: SeverityOff, levelSet: true}}}
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := messagesOfID(errs, "agentic-actions"); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestTemplateInjectionSinksOff(t *testing.T) {
	const src = "on: issue_comment\njobs:\n  a:\n    runs-on: ubuntu-latest\n    container:\n      image: x\n      options: ${{ github.event.comment.body }}\n    steps:\n      - run: echo\n"
	if got := messagesOfID(lintAgentTest(t, src), "template-injection"); len(got) != 1 {
		t.Fatalf("want 1 finding, got %q", got)
	}
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{Rules: map[string]RuleConfig{"template-injection": {Level: SeverityOff, levelSet: true}}}
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := messagesOfID(errs, "template-injection"); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestAgenticActionsLimitedToken(t *testing.T) {
	const head = "on: issues\njobs:\n  a:\n    runs-on: ubuntu-latest\n"
	const step = "    steps:\n      - uses: openai/codex-action@v1\n        with:\n          allow-users: '*'\n"
	tests := []struct {
		name  string
		perms string
		want  int
	}{
		{"not set", "", 1},
		{"empty", "    permissions: {}\n", 0},
		{"read-all", "    permissions: read-all\n", 0},
		{"write-all", "    permissions: write-all\n", 1},
		{"issues", "    permissions:\n      issues: write\n      pull-requests: write\n      contents: read\n", 0},
		{"contents", "    permissions:\n      contents: write\n", 1},
		{"id-token", "    permissions:\n      id-token: write\n", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := messagesOfID(lintAgentTest(t, head+tc.perms+step), "agentic-actions")
			if len(got) != tc.want {
				t.Fatalf("want %d findings, got %q", tc.want, got)
			}
		})
	}
	// The workflow level counts when the job sets nothing
	src := "on: issues\npermissions:\n  issues: write\njobs:\n  a:\n    runs-on: ubuntu-latest\n" + step
	if got := messagesOfID(lintAgentTest(t, src), "agentic-actions"); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestAgenticActionsWorkflowRun(t *testing.T) {
	const head = "on:\n  workflow_run:\n    workflows: [CI]\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		name  string
		steps string
		want  int
	}{
		// the workflow it follows decides who can reach it, which the rule cannot see
		{"ungated agent", "      - uses: actions/ai-inference@v3\n", 0},
		{"unsafe settings", "      - uses: anthropics/claude-code-action@v1\n        with:\n          claude_args: --dangerously-skip-permissions\n", 0},
		{"open gate", "      - uses: anthropics/claude-code-action@v1\n        with:\n          allowed_non_write_users: '*'\n", 1},
		{"head checkout", "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.workflow_run.head_sha }}\n      - uses: actions/ai-inference@v3\n", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := messagesOfID(lintAgentTest(t, head+tc.steps), "agentic-actions")
			if len(got) != tc.want {
				t.Fatalf("want %d findings, got %q", tc.want, got)
			}
		})
	}
}

func TestAgenticActionsRestricted(t *testing.T) {
	const head = "on: issues\njobs:\n  a:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - uses: google-github-actions/run-gemini-cli@v0\n        with:\n"
	tests := []struct {
		name     string
		settings string
		want     int
	}{
		{"none", "", 1},
		{"core tools", `settings: '{"tools":{"core":["run_shell_command(gh issue edit)"]}}'`, 0},
		{"legacy core tools", `settings: '{"coreTools":["run_shell_command(gh issue edit)"]}'`, 0},
		{"core tools with a shell", `settings: '{"tools":{"core":["run_shell_command"]}}'`, 1},
		{"empty list", `settings: '{"tools":{"core":[]}}'`, 1},
		{"not json", `settings: not-json`, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := head + "          " + tc.settings + "\n"
			if tc.settings == "" {
				src = head + "          prompt: hi\n"
			}
			got := messagesOfID(lintAgentTest(t, src), "agentic-actions")
			if len(got) != tc.want {
				t.Fatalf("want %d findings, got %q", tc.want, got)
			}
		})
	}
}

func TestStripLooseJSON(t *testing.T) {
	var doc struct {
		A []string `json:"a"`
		B string   `json:"b"`
	}
	src := "{\n  // comment\n  \"a\": [\"x,]\", \"y // not a comment\",\n  ], /* block */\n  \"b\": \"q\\\"\",\n}"
	if err := unmarshalLooseJSON(src, &doc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.A, []string{"x,]", "y // not a comment"}) || doc.B != `q"` {
		t.Fatalf("%+v", doc)
	}
}

func TestAgentSettingsWithExpressions(t *testing.T) {
	const head = "on: issues\njobs:\n  a:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - uses: google-github-actions/run-gemini-cli@v0\n        with:\n"
	// an expression in a detail of the settings does not hide the list of tools
	src := head + "          settings: |-\n            {\"telemetry\": {\"enabled\": ${{ vars.X != '' }}}, \"tools\": {\"core\": [\"run_shell_command(echo)\"]}}\n"
	if got := messagesOfID(lintAgentTest(t, src), "agentic-actions"); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
	// an expression is never taken for a risky value
	src = "on: issues\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: openai/codex-action@v1\n        with:\n          safety-strategy: ${{ vars.S }}\n"
	if got := messagesOfID(lintAgentTest(t, src), "agentic-actions"); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

package jactionlint

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// This file is the knowledge about AI agent actions: which inputs are the prompt or the arguments of the
// agent (template-injection reports attacker controlled expressions in them), which inputs open the
// action to users without write access, and which settings turn off the safeguards of the agent
// (agentic-actions reports them). Everything is read from the action.yml and the documentation of the
// action, and Source says where. The list changes often: an action that is not here is not checked.

// agentOpenGate is an input that lets users without write access start the agent.
type agentOpenGate struct {
	Input string
	// Wildcard is the value that opens the gate to everybody.
	Wildcard string
	// Who describes whom the value lets in.
	Who string
}

// agentIssue is a problem with a setting of the agent.
type agentIssue struct {
	Str *String
	Msg string
}

// agentAction is an AI agent action.
type agentAction struct {
	// Name is the lower case owner/repo[/path] of the action.
	Name string
	// Title is how messages call the action.
	Title string
	// Prompts are the lower case names of the inputs with the instructions of the agent.
	Prompts []string
	// Config are the lower case names of the inputs that are the command line arguments or the settings
	// of the agent: an attacker who controls them can add tools, servers or hooks.
	Config []string
	// EnvInputs are the lower case names of the inputs that hold environment variables for the agent.
	EnvInputs []string
	// Gated is whether the action checks that the user who started it has write access, before it runs
	// the agent. An action without the check runs for whoever can trigger the workflow.
	Gated bool
	// OpenGate lists the inputs that switch the check of a gated action off.
	OpenGate []agentOpenGate
	// unsafe finds the settings that turn the safeguards of the agent off. windows is whether the job
	// runs on Windows.
	unsafe func(e *ExecAction, windows bool) []agentIssue
	// restricted reports whether the step limits the agent to a short list of tools or to a read-only
	// sandbox, so that an agent that was steered can do little. The documentation of every agent advises
	// this for a workflow that outsiders can reach.
	restricted func(e *ExecAction) bool
	// Source is where the inputs and the behavior are documented.
	Source string
}

var claudeOpenGate = []agentOpenGate{
	{"allowed_non_write_users", "*", "everybody, including users without write access,"},
	{"allowed_bots", "*", "every bot and GitHub App, whether or not it has write access,"},
}

var agentActions = []*agentAction{
	{
		Name: "anthropics/claude-code-action", Title: "Claude Code Action",
		// direct_prompt, override_prompt, custom_instructions and mcp_config are inputs of the v0 releases.
		Prompts:   []string{"prompt", "direct_prompt", "override_prompt", "custom_instructions"},
		Config:    []string{"claude_args", "settings", "allowed_tools", "disallowed_tools", "mcp_config", "claude_env"},
		EnvInputs: []string{"claude_env"},
		Gated:     true, OpenGate: claudeOpenGate, unsafe: unsafeClaude, restricted: restrictedClaude,
		Source: "https://github.com/anthropics/claude-code-action/blob/main/action.yml and docs/security.md",
	},
	{
		Name: "anthropics/claude-code-base-action", Title: "Claude Code Base Action",
		Prompts:   []string{"prompt", "direct_prompt", "custom_instructions"},
		Config:    []string{"claude_args", "settings", "allowed_tools", "disallowed_tools", "mcp_config", "claude_env"},
		EnvInputs: []string{"claude_env"},
		unsafe:    unsafeClaude, restricted: restrictedClaude,
		Source: "https://github.com/anthropics/claude-code-base-action/blob/main/action.yml",
	},
	{
		Name: "anthropics/claude-code-security-review", Title: "Claude Code Security Reviewer",
		Source: "https://github.com/anthropics/claude-code-security-review/blob/main/action.yml",
	},
	{
		Name: "google-github-actions/run-gemini-cli", Title: "run-gemini-cli",
		Prompts: []string{"prompt"}, Config: []string{"settings", "extensions"}, unsafe: unsafeGemini("settings"), restricted: restrictedGemini("settings"),
		Source: "https://github.com/google-github-actions/run-gemini-cli/blob/main/action.yml",
	},
	{
		Name: "google-gemini/gemini-cli-action", Title: "Gemini CLI Action",
		Prompts: []string{"prompt"}, Config: []string{"settings_json"}, unsafe: unsafeGemini("settings_json"), restricted: restrictedGemini("settings_json"),
		Source: "https://github.com/google-gemini/gemini-cli-action/blob/main/action.yml",
	},
	{
		Name: "openai/codex-action", Title: "Codex Action",
		Prompts: []string{"prompt"}, Config: []string{"codex-args"}, Gated: true,
		OpenGate: []agentOpenGate{{"allow-users", "*", "everybody, including users without write access,"}},
		unsafe:   unsafeCodex, restricted: restrictedCodex,
		Source: "https://github.com/openai/codex-action/blob/main/action.yml and README.md",
	},
	{
		Name: "actions/ai-inference", Title: "AI inference",
		Prompts: []string{"prompt", "system-prompt"}, Config: []string{"copilot-allow-tools"}, unsafe: unsafeAIInference,
		Source: "https://github.com/actions/ai-inference/blob/main/action.yml",
	},
	{
		Name: "factory-ai/droid-action", Title: "Droid",
		Config: []string{"droid_args", "settings"}, Gated: true, OpenGate: claudeOpenGate, unsafe: unsafeDroid,
		Source: "https://github.com/Factory-AI/droid-action/blob/main/action.yml",
	},
	{
		Name: "sst/opencode/github", Title: "opencode", Prompts: []string{"prompt"}, Gated: true,
		Source: "https://github.com/sst/opencode/blob/master/github/action.yml",
	},
	{
		Name: "anomalyco/opencode/github", Title: "opencode", Prompts: []string{"prompt"}, Gated: true,
		Source: "https://github.com/anomalyco/opencode/blob/master/github/action.yml",
	},
	{
		Name: "openhands/openhands-github-action", Title: "OpenHands", Prompts: []string{"prompt"},
		Source: "https://github.com/OpenHands/openhands-github-action/blob/main/action.yml",
	},
	{
		Name: "warpdotdev/oz-agent-action", Title: "Oz agent", Prompts: []string{"prompt"}, Config: []string{"mcp"},
		Source: "https://github.com/warpdotdev/oz-agent-action/blob/main/action.yml",
	},
	{
		Name: "qodo-ai/pr-agent", Title: "PR-Agent", Prompts: []string{"artifact_instructions"},
		Source: "https://github.com/qodo-ai/pr-agent/blob/main/action.yaml",
	},
	{
		Name: "codium-ai/pr-agent", Title: "PR-Agent", Prompts: []string{"artifact_instructions"},
		Source: "https://github.com/qodo-ai/pr-agent/blob/main/action.yaml (the former name of qodo-ai/pr-agent)",
	},
}

var agentByName = func() map[string]*agentAction {
	m := make(map[string]*agentAction, len(agentActions))
	for _, a := range agentActions {
		m[a.Name] = a
	}
	return m
}()

// agentOf returns the AI agent action which the step runs, or nil.
func agentOf(e *ExecAction) *agentAction {
	if e == nil || e.Uses == nil || e.Uses.Value == "" || e.Uses.ContainsExpression() {
		return nil
	}
	u := ParseUses(e.Uses.Value)
	if u.Kind != UsesAction {
		return nil
	}
	return agentByName[u.CanonicalName()]
}

// sinks returns the inputs of the step in which an expression from an attacker is dangerous.
func (a *agentAction) sinks(e *ExecAction) []injectionSink {
	var ret []injectionSink
	add := func(names []string, why, fix string) {
		for _, n := range names {
			in := e.Inputs[n]
			if in == nil || in.Value == nil || !in.Value.ContainsExpression() {
				continue
			}
			ret = append(ret, injectionSink{Str: in.Value, What: fmt.Sprintf("the %q input of %s", n, a.Title), Why: why, Fix: fix})
		}
	}
	add(a.Prompts,
		"the agent reads the prompt as instructions, so text from an attacker can steer the agent, and quoting or escaping cannot prevent that",
		"do not put event data in the prompt: let the agent read it with its own tools, and give the agent only the permissions, secrets and tools that the worst instruction could use")
	add(a.Config,
		"the action hands it to the agent as command line arguments or settings, so text from an attacker can add tools, servers or hooks",
		fixedValueFix)
	return ret
}

// --- settings that turn the safeguards of an agent off -------------------------------------------------

// shellWords splits a command line like a POSIX shell does, without expanding anything.
func shellWords(s string) []string {
	var words []string
	var cur strings.Builder
	in := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case quote == '"':
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < len(s) && strings.IndexByte("\"\\$`", s[i+1]) >= 0:
				i++
				cur.WriteByte(s[i])
			default:
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
			in = true
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			in = true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if in {
				words = append(words, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteByte(c)
			in = true
		}
	}
	if in {
		words = append(words, cur.String())
	}
	return words
}

// splitToolList splits a list of tool rules such as `Bash(git:*),Edit Read` at commas and white space
// that are not inside parentheses.
func splitToolList(s string) []string {
	var ret []string
	depth, start := 0, 0
	flush := func(end int) {
		if t := strings.TrimSpace(s[start:end]); t != "" {
			ret = append(ret, t)
		}
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',', ' ', '\t', '\n', '\r':
			if depth == 0 {
				flush(i)
				start = i + 1
			}
		}
	}
	flush(len(s))
	return ret
}

// anyCodeCommands are the programs that run whatever they are given. An allow rule for one of them with a
// wildcard allows any code.
var anyCodeCommands = map[string]bool{
	"bash": true, "sh": true, "zsh": true, "dash": true, "ksh": true, "fish": true, "eval": true, "exec": true,
	"env": true, "xargs": true, "sudo": true, "python": true, "python3": true, "node": true, "nodejs": true,
	"deno": true, "bun": true, "perl": true, "ruby": true, "php": true, "npx": true, "bunx": true, "pnpx": true,
	"uvx": true, "pipx": true, "docker": true, "pwsh": true, "powershell": true,
}

// networkCommands send data to a host of the caller's choice.
var networkCommands = map[string]bool{"curl": true, "wget": true, "nc": true, "ncat": true, "netcat": true, "ssh": true, "scp": true}

// riskyToolRule says why an allow rule of an agent, like `Bash(python:*)`, gives the agent more than a
// workflow should. shell is the name of the shell tool and web the name of the tool that fetches URLs; web
// can be empty. It returns an empty string for a rule that is fine.
func riskyToolRule(rule, shell, web string) string {
	rule = strings.TrimSpace(rule)
	name, spec := rule, ""
	hasSpec := false
	if i := strings.IndexByte(rule, '('); i >= 0 && strings.HasSuffix(rule, ")") {
		name, spec, hasSpec = strings.TrimSpace(rule[:i]), rule[i+1:len(rule)-1], true
	}
	spec = strings.TrimSpace(spec)
	switch {
	case strings.EqualFold(name, shell):
		wild := strings.Trim(spec, "*: \t")
		hadWild := strings.Contains(spec, "*")
		switch {
		case !hasSpec || spec == "" || (hadWild && wild == ""):
			return fmt.Sprintf("%q lets the agent run any shell command", rule)
		case hadWild && !strings.ContainsAny(wild, " \t"):
			cmd := strings.ToLower(wild)
			if anyCodeCommands[cmd] {
				return fmt.Sprintf("%q lets the agent run any code through %s", rule, cmd)
			}
			if networkCommands[cmd] {
				return fmt.Sprintf("%q lets the agent send data to any host with %s", rule, cmd)
			}
		}
	case web != "" && strings.EqualFold(name, web):
		if !hasSpec || spec == "" || spec == "*" || spec == "domain:*" {
			return fmt.Sprintf("%q lets the agent request any URL, which is a way to send data out", rule)
		}
	}
	return ""
}

// literalInput returns the value of the input with every ${{ }} replaced by 0, so that settings which only use an
// expression for a detail (the telemetry flag of the Gemini CLI, say) can still be read. An expression is never
// taken for a risky value.
func literalInput(e *ExecAction, name string) (*String, bool) {
	in := e.Inputs[name]
	if in == nil || in.Value == nil {
		return nil, false
	}
	if !in.Value.ContainsExpression() {
		return in.Value, true
	}
	spans := scanExprs(in.Value)
	v := in.Value.Value
	for i := len(spans) - 1; i >= 0; i-- {
		v = v[:spans[i].Start] + "0" + v[spans[i].End:]
	}
	c := *in.Value
	c.Value = v
	return &c, true
}

// claudeArgsIssues reads the arguments given to the Claude Code CLI.
func claudeArgsIssues(str *String) []agentIssue {
	var ret []agentIssue
	words := shellWords(str.Value)
	for i := 0; i < len(words); i++ {
		w := words[i]
		flag, val, hasVal := strings.Cut(w, "=")
		switch flag {
		case "--dangerously-skip-permissions":
			ret = append(ret, agentIssue{str, "\"--dangerously-skip-permissions\" turns off every permission check of Claude Code"})
		case "--permission-mode":
			if !hasVal && i+1 < len(words) {
				val = words[i+1]
			}
			if strings.EqualFold(val, "bypassPermissions") {
				ret = append(ret, agentIssue{str, "\"--permission-mode bypassPermissions\" turns off every permission check of Claude Code"})
			}
		case "--allowedTools", "--allowed-tools", "--allowed_tools":
			var rules []string
			if hasVal {
				rules = append(rules, splitToolList(val)...)
			}
			for j := i + 1; j < len(words) && !strings.HasPrefix(words[j], "-"); j++ {
				rules = append(rules, splitToolList(words[j])...)
			}
			for _, r := range rules {
				if why := riskyToolRule(r, "Bash", "WebFetch"); why != "" {
					ret = append(ret, agentIssue{str, "the allowed tools of Claude Code: " + why})
				}
			}
		case "--settings":
			if !hasVal && i+1 < len(words) {
				val = words[i+1]
			}
			if strings.HasPrefix(strings.TrimSpace(val), "{") {
				ret = append(ret, claudeSettingsIssues(&String{Value: val, Pos: str.Pos})...)
			}
		}
	}
	return ret
}

func claudeSettingsIssues(str *String) []agentIssue {
	var doc struct {
		Permissions struct {
			Allow       []string `json:"allow"`
			DefaultMode string   `json:"defaultMode"`
		} `json:"permissions"`
	}
	if unmarshalLooseJSON(str.Value, &doc) != nil {
		return nil // not JSON (a path, or invalid): nothing to read
	}
	var ret []agentIssue
	if strings.EqualFold(doc.Permissions.DefaultMode, "bypassPermissions") {
		ret = append(ret, agentIssue{str, "\"permissions.defaultMode: bypassPermissions\" turns off every permission check of Claude Code"})
	}
	for _, r := range doc.Permissions.Allow {
		if why := riskyToolRule(r, "Bash", "WebFetch"); why != "" {
			ret = append(ret, agentIssue{str, "the allowed tools of Claude Code: " + why})
		}
	}
	return ret
}

// claudeAllowRules returns the tool rules that the step allows in claude_args, in the settings and in the
// allowed_tools input of the v0 releases.
func claudeAllowRules(e *ExecAction) []string {
	var rules []string
	if s, ok := literalInput(e, "claude_args"); ok {
		words := shellWords(s.Value)
		for i := 0; i < len(words); i++ {
			flag, val, hasVal := strings.Cut(words[i], "=")
			if flag != "--allowedTools" && flag != "--allowed-tools" && flag != "--allowed_tools" {
				continue
			}
			if hasVal {
				rules = append(rules, splitToolList(val)...)
			}
			for j := i + 1; j < len(words) && !strings.HasPrefix(words[j], "-"); j++ {
				rules = append(rules, splitToolList(words[j])...)
			}
		}
	}
	if s, ok := literalInput(e, "settings"); ok {
		var doc struct {
			Permissions struct {
				Allow []string `json:"allow"`
			} `json:"permissions"`
		}
		if unmarshalLooseJSON(s.Value, &doc) == nil {
			rules = append(rules, doc.Permissions.Allow...)
		}
	}
	if s, ok := literalInput(e, "allowed_tools"); ok {
		rules = append(rules, splitToolList(s.Value)...)
	}
	return rules
}

// restrictedClaude: Claude Code in a workflow runs only the tools that are allowed, so a list of exact
// rules with nothing risky in it limits a steered agent.
func restrictedClaude(e *ExecAction) bool {
	rules := claudeAllowRules(e)
	for _, r := range rules {
		if riskyToolRule(r, "Bash", "WebFetch") != "" {
			return false
		}
	}
	return len(rules) > 0
}

func unsafeClaude(e *ExecAction, _ bool) []agentIssue {
	var ret []agentIssue
	if s, ok := literalInput(e, "claude_args"); ok {
		ret = append(ret, claudeArgsIssues(s)...)
	}
	if s, ok := literalInput(e, "settings"); ok {
		ret = append(ret, claudeSettingsIssues(s)...)
	}
	// The allowed_tools input of the v0 releases is a comma separated list.
	if s, ok := literalInput(e, "allowed_tools"); ok {
		for _, r := range splitToolList(s.Value) {
			if why := riskyToolRule(r, "Bash", "WebFetch"); why != "" {
				ret = append(ret, agentIssue{s, "the allowed tools of Claude Code: " + why})
			}
		}
	}
	return ret
}

func unsafeGemini(settings string) func(*ExecAction, bool) []agentIssue {
	return func(e *ExecAction, _ bool) []agentIssue {
		s, ok := literalInput(e, settings)
		if !ok {
			return nil
		}
		var doc struct {
			Tools struct {
				Allowed []string `json:"allowed"`
			} `json:"tools"`
		}
		if unmarshalLooseJSON(s.Value, &doc) != nil {
			return nil
		}
		var ret []agentIssue
		for _, r := range doc.Tools.Allowed {
			if why := riskyToolRule(r, "run_shell_command", "web_fetch"); why != "" {
				ret = append(ret, agentIssue{s, "tools.allowed of the Gemini CLI settings skips the confirmation of a tool: " + why})
			}
		}
		return ret
	}
}

// restrictedGemini: tools.core (coreTools in older settings) limits the built-in tools of the Gemini CLI to a
// list.
func restrictedGemini(settings string) func(*ExecAction) bool {
	return func(e *ExecAction) bool {
		s, ok := literalInput(e, settings)
		if !ok {
			return false
		}
		var doc struct {
			CoreTools []string `json:"coreTools"`
			Tools     struct {
				Core []string `json:"core"`
			} `json:"tools"`
		}
		if unmarshalLooseJSON(s.Value, &doc) != nil {
			return false
		}
		rules := append(doc.CoreTools, doc.Tools.Core...)
		for _, r := range rules {
			if riskyToolRule(r, "run_shell_command", "web_fetch") != "" {
				return false
			}
		}
		return len(rules) > 0
	}
}

// restrictedCodex: the read-only sandbox lets Codex read but not change anything.
func restrictedCodex(e *ExecAction) bool {
	for _, in := range []string{"sandbox", "safety-strategy", "permission-profile"} {
		if s, ok := literalInput(e, in); ok && strings.Contains(strings.ToLower(s.Value), "read-only") {
			return true
		}
	}
	return false
}

func unsafeCodex(e *ExecAction, windows bool) []agentIssue {
	var ret []agentIssue
	if s, ok := literalInput(e, "safety-strategy"); ok && strings.EqualFold(strings.TrimSpace(s.Value), "unsafe") && !windows {
		ret = append(ret, agentIssue{s, "\"safety-strategy: unsafe\" runs Codex with the privileges of the runner user, which can read the API key from memory and print it"})
	}
	if s, ok := literalInput(e, "sandbox"); ok && strings.EqualFold(strings.TrimSpace(s.Value), "danger-full-access") {
		ret = append(ret, agentIssue{s, "\"sandbox: danger-full-access\" turns the sandbox of Codex off"})
	}
	if s, ok := literalInput(e, "permission-profile"); ok && strings.Contains(strings.ToLower(s.Value), "danger") {
		ret = append(ret, agentIssue{s, fmt.Sprintf("the permission profile %q turns the sandbox of Codex off", strings.TrimSpace(s.Value))})
	}
	if s, ok := literalInput(e, "codex-args"); ok {
		lower := strings.ToLower(s.Value)
		if strings.Contains(lower, "--dangerously-bypass-approvals-and-sandbox") || strings.Contains(lower, "--yolo") || strings.Contains(lower, "danger-full-access") {
			ret = append(ret, agentIssue{s, "the arguments of codex turn the sandbox and the approvals off"})
		}
	}
	return ret
}

func unsafeAIInference(e *ExecAction, _ bool) []agentIssue {
	s, ok := literalInput(e, "copilot-allow-tools")
	if !ok {
		return nil
	}
	var ret []agentIssue
	for _, r := range splitToolList(s.Value) {
		if why := riskyToolRule(r, "shell", ""); why != "" {
			ret = append(ret, agentIssue{s, "copilot-allow-tools: " + why})
		}
	}
	return ret
}

func unsafeDroid(e *ExecAction, _ bool) []agentIssue {
	s, ok := literalInput(e, "droid_args")
	if !ok {
		return nil
	}
	for _, w := range shellWords(s.Value) {
		if w == "--skip-permissions-unsafe" {
			return []agentIssue{{s, "\"--skip-permissions-unsafe\" skips every permission prompt of Droid"}}
		}
	}
	return nil
}

// agentActionNames returns the canonical names of the agent actions in a stable order, for documentation
// and tests.
func agentActionNames() []string {
	names := make([]string, 0, len(agentActions))
	for _, a := range agentActions {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	return names
}

// unmarshalLooseJSON decodes the settings of an agent. The settings files of the agents tolerate comments and
// trailing commas (the example of the Gemini CLI action has a trailing comma), which encoding/json rejects.
func unmarshalLooseJSON(s string, v any) error {
	return json.Unmarshal(stripLooseJSON([]byte(s)), v)
}

// stripLooseJSON removes comments and trailing commas outside of strings.
func stripLooseJSON(in []byte) []byte {
	out := make([]byte, 0, len(in))
	inStr := false
	for i := 0; i < len(in); i++ {
		c := in[i]
		switch {
		case inStr:
			out = append(out, c)
			if c == '\\' && i+1 < len(in) {
				i++
				out = append(out, in[i])
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(in) && in[i+1] == '/':
			for i < len(in) && in[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case c == '/' && i+1 < len(in) && in[i+1] == '*':
			i += 2
			for i+1 < len(in) && !(in[i] == '*' && in[i+1] == '/') {
				i++
			}
			i++
		case c == '}' || c == ']':
			// drop a comma before the closing bracket
			j := len(out) - 1
			for j >= 0 && (out[j] == ' ' || out[j] == '\t' || out[j] == '\n' || out[j] == '\r') {
				j--
			}
			if j >= 0 && out[j] == ',' {
				out = append(out[:j], out[j+1:]...)
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

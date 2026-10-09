package jactionlint

import (
	"fmt"
	"sort"
	"strings"
)

// This file lists the places other than run: scripts where text becomes code or a command line, and
// finds the attacker controlled expressions in them. The rule template-injection (rule_template_injection.go)
// reports the findings. Three kinds of places are covered:
//
//   - fields that GitHub itself passes to docker: container and service options, image, entrypoint,
//     command and volumes, and the args and entrypoint of a Docker step (shell: takes no expression);
//   - inputs of well-known actions that run their value as code (codeExecTable);
//   - inputs of AI agent actions (agent_actions.go), where the value is a prompt or the arguments and
//     settings of the agent.

// codeExecEntry is a well-known action with inputs whose value is run as code, like the run: of a step.
type codeExecEntry struct {
	// Action is the lower case owner/repo[/path] of the action.
	Action string
	// Inputs are the lower case names of the inputs.
	Inputs []string
	// Source is where the behavior can be checked: the action.yml of the action, or its source when the
	// action.yml does not say what the input does.
	Source string
}

// codeExecTable lists the inputs of well-known actions whose value is run as code. An expression in them
// is expanded into the source code just as in a run: script. Add an entry only after reading the code of
// the action: an input that is a plain argument of a program is not code.
var codeExecTable = []codeExecEntry{
	{"actions/github-script", []string{"script"}, "https://github.com/actions/github-script/blob/HEAD/action.yml"},
	{"amadevus/pwsh-script", []string{"script"}, "https://github.com/Amadevus/pwsh-script/blob/HEAD/action.yml"},
	{"appleboy/ssh-action", []string{"script"}, "https://github.com/appleboy/ssh-action/blob/HEAD/action.yml"},
	{"addnab/docker-run-action", []string{"options", "run"}, "https://github.com/addnab/docker-run-action/blob/HEAD/action.yml"},
	{"azure/cli", []string{"inlinescript"}, "https://github.com/Azure/cli/blob/HEAD/action.yml"},
	{"azure/powershell", []string{"inlinescript"}, "https://github.com/Azure/powershell/blob/HEAD/action.yml"},
	{"borales/actions-yarn", []string{"cmd"}, "https://github.com/Borales/actions-yarn/blob/HEAD/action.yml"},
	{"cardinalby/js-eval-action", []string{"expression"}, "https://github.com/cardinalby/js-eval-action/blob/HEAD/action.yml"},
	{"cloudflare/wrangler-action", []string{"precommands", "postcommands"}, "https://github.com/cloudflare/wrangler-action/blob/HEAD/src/wranglerAction.ts (execCommands runs each line with child_process.exec)"},
	{"cross-the-world/ssh-pipeline", []string{"script"}, "https://github.com/cross-the-world/ssh-pipeline/blob/HEAD/action.yml"},
	{"cypress-io/github-action", []string{"build", "command", "command-prefix", "install-command", "start", "start-windows"}, "https://github.com/cypress-io/github-action/blob/HEAD/action.yml"},
	{"d3rhase/ssh-command-action", []string{"command"}, "https://github.com/D3rHase/ssh-command-action/blob/HEAD/action.yml"},
	{"devcontainers/ci", []string{"runcmd"}, "https://github.com/devcontainers/ci/blob/HEAD/action.yml"},
	{"fifsky/ssh-action", []string{"command"}, "https://github.com/fifsky/ssh-action/blob/HEAD/action.yml"},
	{"gabrielbb/xvfb-action", []string{"run"}, "https://github.com/GabrielBB/xvfb-action/blob/HEAD/action.yml"},
	{"garygrossgarten/github-action-ssh", []string{"command"}, "https://github.com/garygrossgarten/github-action-ssh/blob/HEAD/action.yml"},
	{"google-github-actions/ssh-compute", []string{"command"}, "https://github.com/google-github-actions/ssh-compute/blob/HEAD/action.yml"},
	{"jannekem/run-python-action", []string{"code"}, "https://github.com/jannekem/run-python-action/blob/HEAD/action.yml"},
	{"jimcronqvist/action-ssh", []string{"command"}, "https://github.com/JimCronqvist/action-ssh/blob/HEAD/action.yml"},
	{"mathiasvr/command-output", []string{"run"}, "https://github.com/mathiasvr/command-output/blob/HEAD/action.yml"},
	{"matootie/dokku", []string{"command"}, "https://github.com/matootie/dokku/blob/HEAD/action.yml"},
	{"mikefarah/yq", []string{"cmd"}, "https://github.com/mikefarah/yq/blob/HEAD/github-action/entrypoint.sh (eval \"$1\")"},
	{"nick-fields/retry", []string{"command", "on_retry_command"}, "https://github.com/nick-fields/retry/blob/HEAD/action.yml"},
	{"nick-invision/retry", []string{"command", "on_retry_command"}, "https://github.com/nick-invision/retry/blob/HEAD/action.yml (the former name of nick-fields/retry)"},
	{"wandalen/wretry.action", []string{"command", "pre_retry_command"}, "https://github.com/Wandalen/wretry.action/blob/HEAD/action.yml"},
}

// codeExecInputs maps the action to the inputs of codeExecTable. The keys are lower case owner/repo
// and the values are lower case input names.
var codeExecInputs = func() map[string][]string {
	m := make(map[string][]string, len(codeExecTable))
	for _, e := range codeExecTable {
		m[e.Action] = e.Inputs
	}
	return m
}()

// injectionSink is a string that is read as a command line, as code or as the instructions of an agent.
type injectionSink struct {
	Str *String
	// What names the field with its owner, e.g. "the options of the container of the job".
	What string
	// Why says how the value is interpreted and what an attacker gains.
	Why string
	// Fix says what to do instead.
	Fix string
}

const fixedValueFix = "use a fixed value, or choose one from a fixed list such as a matrix or an input of type choice"

func containerSinks(c *Container, owner string) []injectionSink {
	if c == nil {
		return nil
	}
	var ret []injectionSink
	add := func(s *String, what, why string) {
		if s != nil && s.ContainsExpression() {
			ret = append(ret, injectionSink{Str: s, What: what + " of " + owner, Why: why, Fix: fixedValueFix})
		}
	}
	add(c.Options, "the options", "docker reads them as command line flags, so text from an attacker can add flags such as --privileged, --volume or --entrypoint")
	add(c.Image, "the image", "the job would run an image that an attacker names")
	add(c.Entrypoint, "the entrypoint", "it is the program that the container runs")
	add(c.Command, "the command", "it is the command line that the container runs")
	for _, v := range c.Volumes {
		add(v, "a volume", "an attacker could mount any path of the runner into the container")
	}
	return ret
}

// jobSinks returns the strings of the job that are read as command lines by docker or by a shell.
func jobSinks(j *Job) []injectionSink {
	if j == nil {
		return nil
	}
	ret := containerSinks(j.Container, "the container of the job")
	if j.Services != nil {
		ids := make([]string, 0, len(j.Services.Value))
		for id := range j.Services.Value {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if s := j.Services.Value[id]; s != nil {
				ret = append(ret, containerSinks(s.Container, fmt.Sprintf("the service %q", id))...)
			}
		}
	}
	return ret
}

// stepSinks returns the strings of the step, other than the code that codeStringsOf returns, which docker
// reads as a command line, and the inputs of AI agent actions.
func stepSinks(s *Step) []injectionSink {
	var ret []injectionSink
	if e, ok := s.Exec.(*ExecAction); ok {
		if e.Uses == nil {
			return nil
		}
		if strings.HasPrefix(strings.ToLower(e.Uses.Value), "docker://") {
			if e.Args != nil && e.Args.ContainsExpression() {
				ret = append(ret, injectionSink{Str: e.Args, What: "the args of the Docker step", Why: "they are the command line that the container runs", Fix: fixedValueFix})
			}
			if e.Entrypoint != nil && e.Entrypoint.ContainsExpression() {
				ret = append(ret, injectionSink{Str: e.Entrypoint, What: "the entrypoint of the Docker step", Why: "it is the program that the container runs", Fix: fixedValueFix})
			}
		}
		if agent := agentOf(e); agent != nil {
			ret = append(ret, agent.sinks(e)...)
		}
	}
	return ret
}

// sinkHit is an attacker controlled expression in a sink.
type sinkHit struct {
	Tier   tiTier // tiDirect, tiSubtree or tiEnv
	Ref    *ctxRef
	Source string
}

// sinkHitOf decides whether the expression is attacker controlled. Unlike a script, a sink is not
// reported for an expression whose value is merely free text: a matrix value or an input in the
// options of a container is normal, and only a value from the attacker is the problem.
func (c *tiContext) sinkHitOf(sp *exprSpan) *sinkHit {
	refs := exprContextRefs(sp.Node)
	for i := range refs {
		r := &refs[i]
		if !knownContexts[r.Path[0]] {
			continue
		}
		switch m, leaf := matchUntrusted(r.Path); m {
		case untrustedLeaf:
			return &sinkHit{Tier: tiDirect, Ref: r, Source: leaf}
		case untrustedSubtree:
			return &sinkHit{Tier: tiSubtree, Ref: r, Source: leaf}
		}
	}
	if c.step == nil {
		return nil // the env context is not available where there is no step
	}
	for i := range refs {
		r := &refs[i]
		if r.Path[0] == "env" && len(r.Path) == 2 {
			if t := c.envTaint(r.Path[1], 0); t != "" {
				return &sinkHit{Tier: tiEnv, Ref: r, Source: t}
			}
		}
	}
	return nil
}

// message describes the finding for the sink.
func (h *sinkHit) message(src string, s injectionSink) string {
	name := h.Ref.Display(src)
	switch h.Tier {
	case tiSubtree:
		return fmt.Sprintf("%q includes potentially untrusted properties such as %q and is expanded into %s. %s. %s", name, h.Source, s.What, s.Why, s.Fix)
	case tiEnv:
		return fmt.Sprintf("environment variable %q holds the potentially untrusted input %q and is expanded into %s. %s. %s", name, h.Source, s.What, s.Why, s.Fix)
	}
	return fmt.Sprintf("%q is potentially untrusted and is expanded into %s. %s. %s", name, s.What, s.Why, s.Fix)
}

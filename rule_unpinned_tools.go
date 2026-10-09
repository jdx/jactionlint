package jactionlint

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// floatingToolAction is an action that downloads a tool at run time and uses the newest release of it unless a
// version is asked for. Pinning the action itself does not pin the tool.
type floatingToolAction struct {
	// action is the canonical (lower case) name of the action.
	action string
	// input is the input that selects the version of the tool.
	input string
	// floating are the values of the input (compared in lower case) that select the newest release. The newest
	// release is also selected when the input is not set, because it is the default of the action.
	floating []string
	// pinningInputs are other inputs that fix the version when they are set (a file with the versions).
	pinningInputs []string
	// source says where the entry comes from.
	source string
	// pedantic entries are reported with "unpinned-tools-pedantic" instead of "unpinned-tools".
	pedantic bool
}

// floatingToolActions is the table of the actions known to install the newest version of a tool by default.
// Every entry names where the default is documented. The ones from zizmor are in the default tier, the ones we
// checked in the action.yml of the action are in the pedantic tier because zizmor does not report them.
var floatingToolActions = []floatingToolAction{
	{action: "aquasecurity/setup-trivy", input: "version", floating: []string{"latest"}, source: "zizmor unpinned-tools; action.yml: version defaults to 'latest'"},
	{action: "1password/load-secrets-action", input: "version", floating: []string{"latest"}, source: "zizmor unpinned-tools; action.yml: version defaults to 'latest'"},
	{action: "extractions/setup-just", input: "just-version", floating: []string{"*"}, source: "zizmor unpinned-tools"},
	{action: "extractions/setup-crate", input: "version", floating: []string{"*"}, source: "zizmor unpinned-tools"},
	{action: "hashicorp/setup-terraform", input: "terraform_version", floating: []string{"latest"}, pedantic: true, source: "action.yml: terraform_version defaults to 'latest'"},
	{action: "azure/setup-kubectl", input: "version", floating: []string{"latest"}, pedantic: true, source: "action.yml: version defaults to 'latest'"},
	{action: "azure/setup-helm", input: "version", floating: []string{"latest"}, pinningInputs: []string{"version-file"}, pedantic: true, source: "action.yml: version defaults to 'latest'; version-file overrides it"},
}

// RuleUnpinnedTools detects tools that are installed without an exact version.
//
// "unpinned-tools" reports `uses:` steps of the actions in floatingToolActions that install the newest version
// of their tool because no version is set or the version is "latest". "unpinned-tools-pedantic" reports the
// further actions of that table that zizmor does not report, and `run:` steps that install or run a tool without
// an exact version: `pip install`, `pipx`, `uv tool`, `uvx`, `cargo install` and `cargo binstall`, `go install`,
// global `npm install -g` and `npx --yes`, `pnpm dlx` and the like.
type RuleUnpinnedTools struct {
	RuleBase
	runContext
}

// NewRuleUnpinnedTools creates a new RuleUnpinnedTools instance.
func NewRuleUnpinnedTools() *RuleUnpinnedTools {
	return &RuleUnpinnedTools{
		RuleBase: NewRuleBase("unpinned-tools", "Checks for tools installed without an exact version at \"uses:\" and \"run:\""),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnpinnedTools) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUnpinnedTools) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleUnpinnedTools) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleUnpinnedTools) VisitStep(n *Step) error {
	switch e := n.Exec.(type) {
	case *ExecAction:
		rule.checkAction(e)
	case *ExecRun:
		rule.checkRun(e)
	}
	return nil
}

func (rule *RuleUnpinnedTools) checkAction(e *ExecAction) {
	if e.Uses == nil || e.Uses.ContainsExpression() {
		return
	}
	u := ParseUses(e.Uses.Value)
	if u.Kind != UsesAction {
		return
	}
	name := u.CanonicalName()
	for _, t := range floatingToolActions {
		if t.action != name {
			continue
		}
		report := func(pos *Pos, msg string) {
			if t.pedantic {
				if rule.pedantic("unpinned-tools") {
					rule.errorIDAt("unpinned-tools", pos, msg)
				}
			} else {
				rule.errorIDAt("unpinned-tools", pos, msg)
			}
		}
		if rule.pinnedByOtherInput(e, t) {
			return
		}
		in := e.Inputs[t.input]
		switch {
		case in == nil || in.Value == nil:
			report(e.Uses.Pos, fmt.Sprintf("action %q installs the newest version of its tool because the input %q is not set. set %q to an exact version", e.Uses.Value, t.input, t.input))
		case in.Value.ContainsExpression():
			// the value is not known; it may well be an exact version
		default:
			v := strings.ToLower(strings.TrimSpace(in.Value.Value))
			for _, f := range t.floating {
				if v == f {
					report(in.Value.Pos, fmt.Sprintf("input %q of action %q selects the newest version of its tool with %q. set it to an exact version", t.input, e.Uses.Value, in.Value.Value))
				}
			}
		}
		return
	}
}

func (rule *RuleUnpinnedTools) pinnedByOtherInput(e *ExecAction, t floatingToolAction) bool {
	for _, n := range t.pinningInputs {
		if in := e.Inputs[n]; in != nil && in.Value != nil && in.Value.Value != "" {
			return true
		}
	}
	return false
}

func (rule *RuleUnpinnedTools) checkRun(run *ExecRun) {
	s, origin := rule.script(run)
	if s == nil {
		return
	}
	for _, c := range s.Commands {
		in := c.Installs()
		if in == nil || !unpinnedToolInstall(c, in) {
			continue
		}
		var names []string
		for _, p := range in.Packages {
			if p.Pinned || pinnedByVersion(c, p) || p.Dynamic || p.Local || p.Kind == runscript.KindPath || p.Kind == runscript.KindDynamic || p.Spec == "" {
				continue
			}
			names = append(names, p.Spec)
		}
		if len(names) == 0 {
			continue
		}
		sort.Strings(names)
		start, end := commandRange(s, origin, c)
		if rule.pedantic("unpinned-tools") {
			rule.errorIDAt("unpinned-tools", start, plural(names)+" installed without an exact version, so every run may fetch a different release. pin "+pinExample(in.Ecosystem)).endAt(end)
		}
	}
}

func plural(names []string) string {
	if len(names) == 1 {
		return "tool " + quote(names[0]) + " is"
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = quote(n)
	}
	return "tools " + strings.Join(q, ", ") + " are"
}

func pinExample(ecosystem string) string {
	switch ecosystem {
	case "pypi":
		return "it with `==`, for example `tool==1.2.3`"
	case "crates":
		return "it with `--version`, for example `--version 1.2.3`"
	case "go":
		return "it with a version, for example `tool@v1.2.3`"
	default:
		return "it with a version, for example `tool@1.2.3`"
	}
}

// unpinnedToolInstall reports whether the command fetches a tool from a registry, which is what the rule is about:
// installations of tools and one-shot runs. Installing the packages of a project (`npm install`, `pip install -r`)
// is the business of the lock file, see unlocked-install.
func unpinnedToolInstall(c *runscript.Command, in *runscript.Install) bool {
	switch in.Ecosystem {
	case "pypi", "crates":
		return true
	case "go":
		// `go get` adds a dependency to go.mod, which go.sum binds
		return in.Verb == "install"
	case "npm":
		if in.Run {
			// `npx tsc` runs the binary of the project when there is one. Only the forms that ask for a download
			// (npx -y, dlx) are known to fetch.
			return c.Tool != "npx" && c.Tool != "bunx" || c.HasFlag("-y", "--yes", "-p", "--package")
		}
		return in.Global
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{ID: "unpinned-tools", Group: RuleGroupSecurity, Summary: "An action installs the newest version of its tool because no version is set or it is latest.", DefaultLevel: SeverityWarning, Profile: ProfileDefault, DocsAnchor: "check-unpinned-tools", Options: []RuleOption{pedanticOption}},
	)
	registerRuleFactory("unpinned-tools", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("unpinned-tools") {
			return nil
		}
		return []Rule{NewRuleUnpinnedTools()}
	})
}

var reVersionLike = regexp.MustCompile(`^v?\d+(\.\d+)+([A-Za-z0-9.+-]*)$`)

// pinnedByVersion reports whether a package names a version that is exact although the analyzer does not read it
// as such: `uvx tool@1.2.3` (the analyzer reads `==`), and the Go versions that are not semantic versions (tags such
// as staticcheck@2025.1.1).
func pinnedByVersion(c *runscript.Command, p *runscript.Package) bool {
	switch {
	case c.Tool == "uvx" || c.Tool == "uv":
		i := strings.LastIndex(p.Spec, "@")
		return i > 0 && reVersionLike.MatchString(p.Spec[i+1:])
	case c.Tool == "go":
		return reVersionLike.MatchString(p.Version)
	}
	return false
}

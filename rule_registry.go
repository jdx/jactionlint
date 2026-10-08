package jactionlint

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// RuleGroup classifies a rule by what it protects against.
type RuleGroup string

const (
	// RuleGroupCorrectness is for rules detecting mistakes which make a workflow fail or misbehave.
	RuleGroupCorrectness RuleGroup = "correctness"
	// RuleGroupSecurity is for rules detecting insecure constructs.
	RuleGroupSecurity RuleGroup = "security"
	// RuleGroupPolicy is for rules enforcing good practices which are not mistakes by themselves.
	RuleGroupPolicy RuleGroup = "policy"
	// RuleGroupStyle is for rules which only enforce a code style.
	RuleGroupStyle RuleGroup = "style"
)

// Profile is a named set of rules which are enabled together. A profile includes every rule of the
// profiles before it: ProfileDefault < ProfileStrict < ProfileAll.
type Profile string

const (
	// ProfileDefault enables the correctness checks and the checks which have (almost) no false
	// positives. This is the profile used when none is configured.
	ProfileDefault Profile = "default"
	// ProfileStrict adds the security posture and policy checks.
	ProfileStrict Profile = "strict"
	// ProfileAll adds the style and pedantic checks.
	ProfileAll Profile = "all"
)

// profileRank orders the profiles. The empty profile (never enabled by a profile) has no rank.
var profileRank = map[Profile]int{ProfileDefault: 1, ProfileStrict: 2, ProfileAll: 3}

// ParseProfile parses the name of a profile.
func ParseProfile(s string) (Profile, error) {
	p := Profile(s)
	if _, ok := profileRank[p]; !ok {
		return "", fmt.Errorf("invalid profile %q. available profiles are \"default\", \"strict\" and \"all\"", s)
	}
	return p, nil
}

// Includes reports whether the rules of the profile q are enabled by the profile p.
func (p Profile) Includes(q Profile) bool {
	rq, ok := profileRank[q]
	return ok && profileRank[p] >= rq
}

// RuleOptionKind is the type of the value of a rule option.
type RuleOptionKind string

const (
	// RuleOptionInt is an option taking a non-negative integer.
	RuleOptionInt RuleOptionKind = "int"
	// RuleOptionNumber is an option taking a non-negative number.
	RuleOptionNumber RuleOptionKind = "number"
)

// RuleOption describes one option which can be given to a rule in the "rules" mapping of the
// configuration file.
type RuleOption struct {
	// Name is the key of the option.
	Name string
	// Kind is the type of the option value.
	Kind RuleOptionKind
	// Default is the value used when the rule is enabled without the option. nil means that the rule
	// does nothing until the option is set.
	Default any
	// Summary describes the option.
	Summary string
}

// RuleInfo is the metadata of a rule. One rule implementation (a Rule, which is identified by the
// Kind of its errors) may report several IDs, and each ID has one RuleInfo. IDs are stable: they
// are never renamed or reused, so they are safe to put in configuration files, ignore comments and
// CI annotations.
type RuleInfo struct {
	// ID is the stable kebab-case identifier of the diagnostic, e.g. "unpinned-uses". It appears in
	// Error.ID, in the "rules" mapping of the configuration and in -ignore.
	ID string
	// Group is the category of the rule.
	Group RuleGroup
	// Summary is a one-line description of what the rule reports.
	Summary string
	// DefaultLevel is the severity of the rule when it is enabled without an explicit level.
	DefaultLevel Severity
	// Profile is the first profile which enables the rule. The empty value means that no profile
	// enables the rule; it only runs when the configuration turns it on.
	Profile Profile
	// Online is whether the rule needs network access. Online rules run only with -online and are
	// independent of the profile.
	Online bool
	// Fixable is whether the rule can attach an automatic fix (Error.Fix) to its findings.
	Fixable bool
	// DocsAnchor is the anchor of the section describing the rule in docs/checks.md. It can be empty.
	DocsAnchor string
	// Options are the options the rule accepts in the "rules" mapping of the configuration.
	Options []RuleOption
}

// DocURL returns the URL of the documentation of the rule.
func (r *RuleInfo) DocURL() string {
	return RuleDocURL(r.ID)
}

// RuleDocURL returns the URL of the documentation of the rule with the given ID.
func RuleDocURL(id string) string {
	return "https://jactionlint.jdx.dev/rules#" + id
}

// ruleRegistry is every rule ID jactionlint reports. Keep it sorted by ID. TestRuleIDsAreStable
// guards the IDs: an ID must never be removed or renamed once released.
var ruleRegistry = []RuleInfo{
	{ID: "conflicting-runner-labels", Group: RuleGroupCorrectness, Summary: "The runner labels of a job conflict with each other.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-runner-labels"},
	{ID: "constant-condition", Group: RuleGroupCorrectness, Summary: "An if: condition is a constant expression.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "if-cond-constant"},
	{ID: "context-availability", Group: RuleGroupCorrectness, Summary: "A context or special function is used where it is not available.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "ctx-spfunc-availability"},
	{ID: "cron-too-frequent", Group: RuleGroupCorrectness, Summary: "A scheduled job runs more often than once every 5 minutes.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-cron-syntax-and-timezone"},
	{ID: "cyclic-job-needs", Group: RuleGroupCorrectness, Summary: "Jobs depend on each other in a cycle.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-job-deps"},
	{ID: "deprecated-action-input", Group: RuleGroupCorrectness, Summary: "A deprecated input of an action is used.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "deprecated-inputs-usage"},
	{ID: "deprecated-commands", Group: RuleGroupCorrectness, Summary: "A deprecated workflow command such as ::set-output is used.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-deprecated-workflow-commands"},
	{ID: "duplicate-job-id", Group: RuleGroupCorrectness, Summary: "A job ID is defined more than once.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-job-deps"},
	{ID: "duplicate-job-needs", Group: RuleGroupCorrectness, Summary: "A job ID is listed more than once in needs.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-job-deps"},
	{ID: "duplicate-key", Group: RuleGroupCorrectness, Summary: "A key is defined more than once in a mapping.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-missing-required-duplicate-keys"},
	{ID: "duplicate-step-id", Group: RuleGroupCorrectness, Summary: "A step ID is not unique within its job.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-job-step-ids"},
	{ID: "expression-syntax", Group: RuleGroupCorrectness, Summary: "A ${{ }} expression has a syntax error.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-syntax-expression"},
	{ID: "expression-type", Group: RuleGroupCorrectness, Summary: "A ${{ }} expression has a type error.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-type-check-expression"},
	{ID: "hardcoded-container-credentials", Group: RuleGroupSecurity, Summary: "A password for a container registry is written directly in the workflow.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-hardcoded-credentials"},
	{ID: "if-always-true", Group: RuleGroupCorrectness, Summary: "An if: condition is always true because of the characters around ${{ }}.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "if-cond-constant"},
	{ID: "invalid-activity-type", Group: RuleGroupCorrectness, Summary: "An activity type is not available for the Webhook event.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-webhook-events"},
	{ID: "invalid-cron", Group: RuleGroupCorrectness, Summary: "A cron schedule has an invalid format.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-cron-syntax-and-timezone"},
	{ID: "invalid-env-var-name", Group: RuleGroupCorrectness, Summary: "An environment variable name contains characters which are not allowed.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-env-var-names"},
	{ID: "invalid-event-config", Group: RuleGroupCorrectness, Summary: "An event is configured with options it does not support.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-webhook-events"},
	{ID: "invalid-event-filter", Group: RuleGroupCorrectness, Summary: "An event filter is not available for the event or conflicts with another filter.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-webhook-events"},
	{ID: "invalid-function-call", Group: RuleGroupCorrectness, Summary: "A built-in function is called with wrong arguments.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-contexts-and-builtin-func"},
	{ID: "invalid-glob", Group: RuleGroupCorrectness, Summary: "A glob filter pattern is invalid.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-glob-pattern"},
	{ID: "invalid-id", Group: RuleGroupCorrectness, Summary: "A job or step ID does not follow the naming convention.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "id-naming-convention"},
	{ID: "invalid-ignore-comment", Group: RuleGroupCorrectness, Summary: "An inline ignore comment is invalid.", DefaultLevel: SeverityError, Profile: ProfileDefault},
	{ID: "invalid-label-pattern", Group: RuleGroupCorrectness, Summary: "A runner label pattern in the configuration is not a valid glob.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-runner-labels"},
	{ID: "invalid-local-action", Group: RuleGroupCorrectness, Summary: "A local action cannot be loaded or its metadata is invalid.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "action-metadata-syntax"},
	{ID: "invalid-local-workflow", Group: RuleGroupCorrectness, Summary: "A local reusable workflow cannot be loaded or is invalid.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-reusable-workflows"},
	{ID: "invalid-parallel-step", Group: RuleGroupCorrectness, Summary: "A step is not allowed inside a parallel group or refers to a wrong step.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-parallel-step-refs"},
	{ID: "invalid-permissions", Group: RuleGroupCorrectness, Summary: "A permission scope or its value is invalid.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-permissions"},
	{ID: "invalid-shell-name", Group: RuleGroupCorrectness, Summary: "A shell name is not available on the runner.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-shell-names"},
	{ID: "invalid-timezone", Group: RuleGroupCorrectness, Summary: "A timezone of a schedule is not a valid IANA timezone name.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-cron-syntax-and-timezone"},
	{ID: "invalid-uses", Group: RuleGroupCorrectness, Summary: "A uses: value does not follow the format of an action or a Docker image.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-action-format"},
	{ID: "invalid-workflow-call", Group: RuleGroupCorrectness, Summary: "A reusable workflow call does not follow the format of a reusable workflow.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-reusable-workflows"},
	{ID: "invalid-workflow-call-input", Group: RuleGroupCorrectness, Summary: "An input of the workflow_call event is invalid.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-reusable-workflows"},
	{ID: "invalid-workflow-dispatch-input", Group: RuleGroupCorrectness, Summary: "An input of the workflow_dispatch event is invalid.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-workflow-dispatch-events"},
	{ID: "local-action-checkout", Group: RuleGroupCorrectness, Summary: "A local action is used before any step checks out the repository.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-local-action-checkout"},
	{ID: "matrix-duplicate-value", Group: RuleGroupCorrectness, Summary: "A matrix has a duplicate value.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-matrix-values"},
	{ID: "matrix-invalid-exclude", Group: RuleGroupCorrectness, Summary: "An exclude entry of a matrix does not match the matrix.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-matrix-values"},
	{ID: "max-run-lines", Group: RuleGroupStyle, Summary: "A run: script has more lines than allowed.", DefaultLevel: SeverityError, Profile: ProfileAll, DocsAnchor: "check-run-policy", Options: []RuleOption{{Name: "max", Kind: RuleOptionInt, Default: DefaultMaxRunLines, Summary: "The maximum number of non-blank lines of a run: script."}}},
	{ID: "merge-key", Group: RuleGroupCorrectness, Summary: "The YAML merge key << is used, which GitHub Actions does not support.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "yaml-anchors"},
	{ID: "missing-action-input", Group: RuleGroupCorrectness, Summary: "A required input of an action is not specified.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-local-action-inputs"},
	{ID: "missing-permissions", Group: RuleGroupPolicy, Summary: "Neither the workflow nor the job sets permissions:.", DefaultLevel: SeverityError, Profile: ProfileStrict, DocsAnchor: "permissions"},
	{ID: "missing-timeout", Group: RuleGroupPolicy, Summary: "A job does not set timeout-minutes.", DefaultLevel: SeverityError, Profile: ProfileStrict, DocsAnchor: "check-timeout-minutes"},
	{ID: "missing-workflow-input", Group: RuleGroupCorrectness, Summary: "A required input of a reusable workflow is not specified.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-reusable-workflows"},
	{ID: "missing-workflow-secret", Group: RuleGroupCorrectness, Summary: "A required secret of a reusable workflow is not passed.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-reusable-workflows"},
	{ID: "outdated-action-runner", Group: RuleGroupCorrectness, Summary: "An action runs on a runtime which GitHub Actions no longer supports.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "detect-outdated-popular-actions"},
	{ID: "pyflakes", Group: RuleGroupCorrectness, Summary: "pyflakes reported an issue in a Python script.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-pyflakes-integ"},
	{ID: "recursive-alias", Group: RuleGroupCorrectness, Summary: "A YAML alias refers to itself.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "yaml-anchors"},
	{ID: "require-expression-wrapping", Group: RuleGroupStyle, Summary: "An if: condition is not wrapped in ${{ }}.", DefaultLevel: SeverityError, Profile: ProfileAll, DocsAnchor: "check-require-expression-wrapping"},
	{ID: "require-shell", Group: RuleGroupStyle, Summary: "A run: step does not set the shell explicitly.", DefaultLevel: SeverityError, Profile: ProfileAll, DocsAnchor: "check-run-policy"},
	{ID: "required-actions", Group: RuleGroupPolicy, Summary: "An action listed in required-actions is not used by a workflow.", DefaultLevel: SeverityError},
	{ID: "shellcheck", Group: RuleGroupCorrectness, Summary: "shellcheck reported an issue in a shell script.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-shellcheck-integ"},
	{ID: "template-injection", Group: RuleGroupSecurity, Summary: "A potentially untrusted input is expanded in a script.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "untrusted-inputs"},
	{ID: "timeout-too-long", Group: RuleGroupPolicy, Summary: "timeout-minutes of a job exceeds the configured maximum.", DefaultLevel: SeverityError, DocsAnchor: "check-timeout-minutes", Options: []RuleOption{{Name: "max", Kind: RuleOptionNumber, Summary: "The maximum allowed timeout-minutes. The rule does nothing without it."}}},
	{ID: "undefined-function", Group: RuleGroupCorrectness, Summary: "An undefined function is called in an expression.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-contexts-and-builtin-func"},
	{ID: "undefined-job-needs", Group: RuleGroupCorrectness, Summary: "A job needs a job which does not exist.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-job-deps"},
	{ID: "undefined-property", Group: RuleGroupCorrectness, Summary: "An undefined variable or property is accessed in an expression.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-contexts-and-builtin-func"},
	{ID: "unknown-action-input", Group: RuleGroupCorrectness, Summary: "An input which the action does not define is specified.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-local-action-inputs"},
	{ID: "unknown-event", Group: RuleGroupCorrectness, Summary: "An unknown Webhook event is used.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-webhook-events"},
	{ID: "unknown-runner-label", Group: RuleGroupCorrectness, Summary: "A runner label is unknown.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-runner-labels"},
	{ID: "unknown-workflow-input", Group: RuleGroupCorrectness, Summary: "An input which the reusable workflow does not define is specified.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-reusable-workflows"},
	{ID: "unknown-workflow-secret", Group: RuleGroupCorrectness, Summary: "A secret which the reusable workflow does not define is passed.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-reusable-workflows"},
	{ID: "unpinned-uses", Group: RuleGroupPolicy, Summary: "An action, reusable workflow or Docker image is not pinned to a commit SHA or digest.", DefaultLevel: SeverityError, Profile: ProfileStrict, DocsAnchor: "check-action-format"},
	{ID: "unsound-ternary", Group: RuleGroupCorrectness, Summary: "The a && b || c idiom has a falsy b so it always evaluates to c.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-falsy-ternary"},
	{ID: "unused-anchor", Group: RuleGroupCorrectness, Summary: "A YAML anchor is defined but never used.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "yaml-anchors"},
	{ID: "unused-ignore", Group: RuleGroupPolicy, Summary: "An inline ignore comment did not suppress anything.", DefaultLevel: SeverityError, Profile: ProfileStrict},
	{ID: "workflow-call-permissions", Group: RuleGroupCorrectness, Summary: "A caller job grants fewer permissions than a reusable workflow requires.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-reusable-workflows"},
	{ID: "workflow-input-type", Group: RuleGroupCorrectness, Summary: "The type of a value passed to a reusable workflow does not match its input.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-reusable-workflows"},
	{ID: "workflow-run-names", Group: RuleGroupCorrectness, Summary: "A workflow_run event refers to a workflow which does not exist in the repository.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-workflow-run-names"},
	{ID: "workflow-syntax", Group: RuleGroupCorrectness, Summary: "The workflow does not follow the syntax of GitHub Actions workflows.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-unexpected-keys"},
	{ID: "yaml-syntax", Group: RuleGroupCorrectness, Summary: "The file is not valid YAML.", DefaultLevel: SeverityError, Profile: ProfileDefault},
}

// DefaultMaxRunLines is the maximum number of lines of a run: script which the max-run-lines rule
// allows when it is enabled by a profile without the "max" option.
const DefaultMaxRunLines = 100

var ruleIndex = func() map[string]*RuleInfo {
	m := make(map[string]*RuleInfo, len(ruleRegistry))
	for i := range ruleRegistry {
		m[ruleRegistry[i].ID] = &ruleRegistry[i]
	}
	return m
}()

// Rules returns the metadata of all rules sorted by ID. The returned slice is a copy.
func Rules() []RuleInfo {
	ret := slices.Clone(ruleRegistry)
	slices.SortFunc(ret, func(a, b RuleInfo) int { return strings.Compare(a.ID, b.ID) })
	return ret
}

// LookupRule returns the metadata of the rule with the given ID.
func LookupRule(id string) (RuleInfo, bool) {
	r, ok := ruleIndex[id]
	if !ok {
		return RuleInfo{}, false
	}
	return *r, true
}

func (r *RuleInfo) option(name string) (RuleOption, bool) {
	for _, o := range r.Options {
		if o.Name == name {
			return o, true
		}
	}
	return RuleOption{}, false
}

// ruleContext is what the constructors of the built-in rules need to create rule instances while
// linting one file.
type ruleContext struct {
	path                   string
	project                *Project
	localActions           *LocalActionsCache
	localReusableWorkflows *LocalReusableWorkflowCache
	config                 *Config
	shellcheck             string
	pyflakes               string
	proc                   *concurrentProcess
}

// ruleFactory creates one rule implementation. A factory returns nil when the rule is not needed
// for the file (e.g. its configuration is empty).
type ruleFactory struct {
	// kind is the name of the rule. Errors of the rule have it as Error.Kind.
	kind string
	new  func(ctx *ruleContext) (Rule, error)
}

func plainRule(kind string, f func(ctx *ruleContext) Rule) ruleFactory {
	return ruleFactory{kind, func(ctx *ruleContext) (Rule, error) { return f(ctx), nil }}
}

// ruleFactories is the list of all built-in rules in the order they are applied. The IDs which each
// rule can report are in ruleRegistry.
var ruleFactories = []ruleFactory{
	plainRule("matrix", func(*ruleContext) Rule { return NewRuleMatrix() }),
	plainRule("credentials", func(*ruleContext) Rule { return NewRuleCredentials() }),
	plainRule("shell-name", func(*ruleContext) Rule { return NewRuleShellName() }),
	plainRule("run-policy", func(*ruleContext) Rule { return NewRuleRunPolicy() }),
	plainRule("runner-label", func(*ruleContext) Rule { return NewRuleRunnerLabel() }),
	plainRule("events", func(*ruleContext) Rule { return NewRuleEvents() }),
	plainRule("workflow-run", func(ctx *ruleContext) Rule { return NewRuleWorkflowRun(ctx.project) }),
	plainRule("job-needs", func(*ruleContext) Rule { return NewRuleJobNeeds() }),
	plainRule("parallel-steps", func(*ruleContext) Rule { return NewRuleParallelSteps() }),
	plainRule("action", func(ctx *ruleContext) Rule { return NewRuleAction(ctx.localActions) }),
	plainRule("local-action-checkout", func(*ruleContext) Rule { return NewRuleLocalActionCheckout() }),
	plainRule("env-var", func(*ruleContext) Rule { return NewRuleEnvVar() }),
	plainRule("id", func(*ruleContext) Rule { return NewRuleID() }),
	plainRule("glob", func(*ruleContext) Rule { return NewRuleGlob() }),
	plainRule("permissions", func(*ruleContext) Rule { return NewRulePermissions() }),
	plainRule("timeout-check", func(*ruleContext) Rule { return NewRuleTimeoutCheck() }),
	plainRule("require-permissions", func(*ruleContext) Rule { return NewRuleRequirePermissions() }),
	plainRule("workflow-call", func(ctx *ruleContext) Rule {
		return NewRuleWorkflowCall(ctx.path, ctx.localReusableWorkflows)
	}),
	plainRule("expression", func(ctx *ruleContext) Rule {
		return NewRuleExpression(ctx.localActions, ctx.localReusableWorkflows)
	}),
	plainRule("deprecated-commands", func(*ruleContext) Rule { return NewRuleDeprecatedCommands() }),
	plainRule("if-cond", func(*ruleContext) Rule { return NewRuleIfCond() }),
	{"required-actions", func(ctx *ruleContext) (Rule, error) {
		// Only add the rule if the config has required actions
		if ctx.config == nil || len(ctx.config.RequiredActions) == 0 {
			return nil, nil
		}
		return NewRuleRequiredActions(ctx.config.RequiredActions), nil
	}},
	{"shellcheck", func(ctx *ruleContext) (Rule, error) {
		if ctx.shellcheck == "" {
			return nil, errors.New("shellcheck command name was empty")
		}
		return NewRuleShellcheck(ctx.shellcheck, ctx.proc)
	}},
	{"pyflakes", func(ctx *ruleContext) (Rule, error) {
		if ctx.pyflakes == "" {
			return nil, errors.New("pyflakes command name was empty")
		}
		return NewRulePyflakes(ctx.pyflakes, ctx.proc)
	}},
}

// newBuiltinRules creates the built-in rules for linting one file. Rules which cannot be created
// (e.g. because an external command is missing) are skipped after reporting the reason with log.
func newBuiltinRules(ctx *ruleContext, log func(args ...interface{})) []Rule {
	rules := make([]Rule, 0, len(ruleFactories))
	for _, f := range ruleFactories {
		r, err := f.new(ctx)
		if err != nil {
			log("Rule \"" + f.kind + "\" was disabled: " + err.Error())
			continue
		}
		if r != nil {
			rules = append(rules, r)
		}
	}
	return rules
}

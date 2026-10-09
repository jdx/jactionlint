package jactionlint

import (
	"fmt"
	"slices"
	"strconv"
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
// profiles before it: ProfileCorrectness < ProfileDefault < ProfilePedantic.
type Profile string

const (
	// ProfileCorrectness enables what actionlint checks by default plus the bug detectors of the
	// correctness group. It has no security posture or policy rules, apart from the basic checks
	// actionlint has too (an untrusted input in a script, hard-coded container credentials).
	ProfileCorrectness Profile = "correctness"
	// ProfileDefault adds the security posture and policy rules which are worth failing a build on.
	// It is the profile used when none is configured.
	ProfileDefault Profile = "default"
	// ProfilePedantic adds the noisy and opinionated rules and the pedantic checks of the audits.
	ProfilePedantic Profile = "pedantic"
)

// Names of the profiles of the first v2 releases which still work, mapped to ProfilePedantic, with a
// deprecation warning.
const (
	deprecatedProfileStrict = "strict"
	deprecatedProfileAll    = "all"
)

// profileRank orders the profiles. The empty profile (never enabled by a profile) has no rank.
var profileRank = map[Profile]int{ProfileCorrectness: 1, ProfileDefault: 2, ProfilePedantic: 3}

// ParseProfile parses the name of a profile: "correctness", "default" or "pedantic".
func ParseProfile(s string) (Profile, error) {
	p := Profile(s)
	if _, ok := profileRank[p]; !ok {
		return "", fmt.Errorf("invalid profile %q. available profiles are \"correctness\", \"default\" and \"pedantic\"", s)
	}
	return p, nil
}

// parseConfigProfile parses the value of "profile" in a config file. It also accepts the retired names
// "strict" and "all", which stand for "pedantic": the returned deprecation message is not empty then.
func parseConfigProfile(s string) (Profile, string, error) {
	if s == deprecatedProfileStrict || s == deprecatedProfileAll {
		return ProfilePedantic, fmt.Sprintf("\"profile: %s\" is deprecated and will be removed in a future version. it means \"profile: pedantic\" now. the profiles are now \"correctness\" (what actionlint checks), \"default\" and \"pedantic\"", s), nil
	}
	p, err := ParseProfile(s)
	return p, "", err
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
	// RuleOptionBool is an option taking true or false.
	RuleOptionBool RuleOptionKind = "bool"
	// RuleOptionStringMap is an option taking a mapping from strings to strings.
	RuleOptionStringMap RuleOptionKind = "string-map"
	// RuleOptionStrings is an option taking a list of strings.
	RuleOptionStrings RuleOptionKind = "strings"
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
	// Validate checks the value after it was normalized to the type of Kind. It can be nil.
	Validate func(v any) error
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

// ruleRegistry is every rule ID jactionlint reports, sorted by ID. Rules add themselves from an init
// function in their own file with registerRules; nothing else lists them. TestRuleIDsAreStable
// guards the IDs: an ID must never be removed or renamed once released.
var ruleRegistry []RuleInfo

// ruleIndex looks up a RuleInfo by ID. It is filled by registerRules.
var ruleIndex = map[string]*RuleInfo{}

// registerRules adds rules to the registry. It is meant to be called from init functions and panics
// when a rule ID is registered twice, so a mistake is found by any test run.
func registerRules(infos ...RuleInfo) {
	for _, info := range infos {
		if info.ID == "" {
			panic("jactionlint: rule with empty ID is registered")
		}
		if _, ok := ruleIndex[info.ID]; ok {
			panic("jactionlint: rule ID " + strconv.Quote(info.ID) + " is registered twice")
		}
		i, _ := slices.BinarySearchFunc(ruleRegistry, info.ID, func(r RuleInfo, id string) int { return strings.Compare(r.ID, id) })
		ruleRegistry = slices.Insert(ruleRegistry, i, info)
		p := info
		ruleIndex[info.ID] = &p
	}
}

// RenamedRule is a rule ID which was merged into the ID of another rule before 2.0 was released, when one audit
// was split in several IDs. The old ID is not a rule any more: its findings are the ones of ID with the
// option Option on. Ignores of the old ID (-ignore, "ignore" of "paths" and inline ignore comments) still
// work, matching only the findings that had the old ID, and print a deprecation warning. Everything else
// that takes a rule ID (rules, ignores, fix.rules, -rules) refuses it and names the new place.
type RenamedRule struct {
	// Old is the retired ID.
	Old string
	// ID is the ID of the rule which reports the findings now.
	ID string
	// Option is the option of ID that turns the findings on, or empty when ID reports them always.
	Option string
}

// renamedRules are the retired IDs, sorted by Old. testdata/rule_ids.d/removed-before-release.txt lists them.
var renamedRules = []RenamedRule{
	{Old: "github-env-untrusted-input", ID: "github-env"},
	{Old: "misfeature-custom-shell", ID: "misfeature", Option: "pedantic"},
	{Old: "template-injection-expansion", ID: "template-injection", Option: "pedantic"},
	{Old: "template-injection-trusted", ID: "template-injection", Option: "pedantic"},
}

// RenamedRules returns the retired rule IDs and the rules that report their findings now.
func RenamedRules() []RenamedRule {
	return slices.Clone(renamedRules)
}

// lookupRenamed returns the retired rule with the ID.
func lookupRenamed(id string) (RenamedRule, bool) {
	for _, r := range renamedRules {
		if r.Old == id {
			return r, true
		}
	}
	return RenamedRule{}, false
}

// renamedMessage is the deprecation warning about the use of a retired rule ID in an ignore.
func (r RenamedRule) renamedMessage() string {
	msg := fmt.Sprintf("the rule ID %q was merged into %q before 2.0. the ignore still works and covers only the findings that the old rule reported; %q would ignore every finding of %q", r.Old, r.ID, r.ID, r.ID)
	if r.Option != "" {
		msg += fmt.Sprintf(". to stop reporting them instead, set \"rules: {%s: {%s: false}}\"", r.ID, r.Option)
	}
	return msg
}

// DefaultMaxRunLines is the maximum number of lines of a run: script which the max-run-lines rule
// allows when it is enabled by a profile without the "max" option.
const DefaultMaxRunLines = 100

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

// RuleEnv is what the constructors of the built-in rules need to create rule instances while linting
// one file. A factory registered with registerRuleFactory receives it.
type RuleEnv struct {
	path                   string
	src                    []byte // the source of the file being linted
	project                *Project
	localActions           *LocalActionsCache
	localReusableWorkflows *LocalReusableWorkflowCache
	config                 *Config
	shellcheck             string
	pyflakes               string
	proc                   *concurrentProcess
	online                 *onlineSession // nil unless the online checks are on
	action                 bool           // the file is the metadata of an action (action.yml), see actionRuleScope

	log  func(args ...interface{})
	name string // the factory being run
}

// Source returns the content of the file being linted. A rule which attaches a Fix needs it to turn
// the positions of the syntax tree into byte offsets. The slice must not be modified.
func (e *RuleEnv) Source() []byte {
	return e.src
}

// Skip reports with the debug log that the rule being created is disabled for the reason. A factory
// calls it and returns nil when the rule cannot be created (e.g. an external command is missing).
func (e *RuleEnv) Skip(reason string) {
	if e.log != nil {
		e.log("Rule \"" + e.name + "\" was disabled: " + reason)
	}
}

type ruleFactory struct {
	// name is the name of the rule. Errors of the rule have it as Error.Kind.
	name string
	// new creates the rule instances for one file. It returns nothing when the rule is not needed for
	// the file (e.g. its configuration is empty).
	new func(env *RuleEnv) []Rule
}

// legacyRuleOrder is the order the rules which existed before self-registration are applied in. The
// order decides which of two errors at the same position comes first, so it is frozen to keep the
// output stable. Rules which are not listed run after them, sorted by name. New rules must not be
// added here.
var legacyRuleOrder = []string{
	"matrix", "credentials", "shell-name", "run-policy", "runner-label", "events", "workflow-run",
	"job-needs", "parallel-steps", "action", "local-action-checkout", "env-var", "id", "glob",
	"permissions", "timeout-check", "require-permissions", "workflow-call", "expression",
	"deprecated-commands", "if-cond", "required-actions", "shellcheck", "pyflakes",
}

func ruleFactoryRank(name string) int {
	if i := slices.Index(legacyRuleOrder, name); i >= 0 {
		return i
	}
	return len(legacyRuleOrder)
}

// ruleFactories is the list of all built-in rules in the order they are applied. Rules add themselves
// from an init function in their own file with registerRuleFactory.
var ruleFactories []ruleFactory

// registerRuleFactory registers the constructor of a rule implementation. It is meant to be called
// from init functions. Rules are applied in a deterministic order (see legacyRuleOrder, then by
// name) which does not depend on the order of the files. It panics when the name is registered
// twice. The IDs which the rule reports are registered separately with registerRules.
func registerRuleFactory(name string, f func(env *RuleEnv) []Rule) {
	for _, r := range ruleFactories {
		if r.name == name {
			panic("jactionlint: rule factory " + strconv.Quote(name) + " is registered twice")
		}
	}
	nf := ruleFactory{name, f}
	i, _ := slices.BinarySearchFunc(ruleFactories, nf, func(a, b ruleFactory) int {
		if c := ruleFactoryRank(a.name) - ruleFactoryRank(b.name); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	})
	ruleFactories = slices.Insert(ruleFactories, i, nf)
}

// newBuiltinRules creates the built-in rules for linting one file. Rules which cannot be created
// (e.g. because an external command is missing) are skipped after reporting the reason with log.
func newBuiltinRules(env *RuleEnv, log func(args ...interface{})) []Rule {
	env.log = log
	rules := make([]Rule, 0, len(ruleFactories))
	for _, f := range ruleFactories {
		env.name = f.name
		if env.action && !runsOnActions(f.name) {
			continue
		}
		rules = append(rules, f.new(env)...)
	}
	return rules
}

package jactionlint

import (
	"strconv"
	"strings"
)

// This file is the hand-kept table of inputs of well-known actions which only work together: one needs another,
// exactly one of a group is needed, or two exclude each other. The data set of popular actions (popular_actions.go,
// generated) only knows which inputs exist and which are required, not these rules, so they are written here. Each
// entry comes from the action.yml, the README or the source of the action, cited in a comment. The rule
// action-input-dependencies reads the table.

type inputDependencyKind int

const (
	// inputRequires: when Inputs[0] is given, one of the other Inputs must be given too, or the action ignores it.
	inputRequires inputDependencyKind = iota
	// inputExactlyOne: exactly one of the Inputs must be given. None reports a missing input, two a conflict.
	inputExactlyOne
	// inputExclusive: at most one of the Inputs may be given. Two report a conflict.
	inputExclusive
)

// inputDependency is one rule of an action.
type inputDependency struct {
	Kind   inputDependencyKind
	Inputs []string
	// Effect says what happens when the rule is broken, for the message.
	Effect string
	// Skip says that the step is not judged: the value of the input is the default, for example. It is nil when
	// the rule always holds.
	Skip func(a *ExecAction) bool
}

// actionInputDependencies are the rules of the majors of an action. The rules hold for the major versions from
// MinMajor on, as the sources say; an older version, a branch and a commit without a version comment are not judged.
type actionInputDependencies struct {
	MinMajor int
	Rules    []inputDependency
	Source   string
}

// isDefaultExpression reports whether the value of the input is the expression which is its default.
func isDefaultExpression(a *ExecAction, input, expr string) bool {
	i, ok := a.Inputs[input]
	if !ok || i == nil || i.Value == nil {
		return false
	}
	v := strings.ToLower(strings.Join(strings.Fields(i.Value.Value), ""))
	return v == "${{"+expr+"}}"
}

// inputDependencyTable maps the canonical name of an action to its rules.
var inputDependencyTable = map[string]actionInputDependencies{
	// actions/download-artifact: `run-id` and `repository` are "If github-token is specified, this is the run (the
	// repository) that artifacts will be downloaded from": the source of the action uses them only inside
	// `if (inputs.token)`, so without a token the action downloads from the current run of the current repository
	// and ignores both. `name` and `artifact-ids` "cannot be used together" (the action stops with an error).
	// https://github.com/actions/download-artifact/blob/main/action.yml
	// https://github.com/actions/download-artifact/blob/main/src/download-artifact.ts
	"actions/download-artifact": {
		MinMajor: 4,
		Source:   "https://github.com/actions/download-artifact/blob/main/action.yml",
		Rules: []inputDependency{
			{Kind: inputRequires, Inputs: []string{"run-id", "github-token"}, Effect: "the action ignores it and downloads from the current run", Skip: func(a *ExecAction) bool { return isDefaultExpression(a, "run-id", "github.run_id") }},
			{Kind: inputRequires, Inputs: []string{"repository", "github-token"}, Effect: "the action ignores it and downloads from the current repository", Skip: func(a *ExecAction) bool { return isDefaultExpression(a, "repository", "github.repository") }},
			{Kind: inputExclusive, Inputs: []string{"name", "artifact-ids"}, Effect: "the action stops with an error"},
		},
	},

	// nick-fields/retry: "Must only specify either minutes or seconds"; the source throws "Must specify either
	// timeout_minutes or timeout_seconds inputs" when neither or both are given.
	// https://github.com/nick-fields/retry/blob/master/src/inputs.ts
	"nick-fields/retry": {
		MinMajor: 2,
		Source:   "https://github.com/nick-fields/retry/blob/master/src/inputs.ts",
		Rules: []inputDependency{
			{Kind: inputExactlyOne, Inputs: []string{"timeout_minutes", "timeout_seconds"}, Effect: `the action fails with "Must specify either timeout_minutes or timeout_seconds inputs"`},
		},
	},

	// google-github-actions/auth: `workload_identity_provider` "is mutually exclusive with credentials_json"; the
	// source stops with 'must specify exactly one of "workload_identity_provider" or "credentials_json"'.
	// https://github.com/google-github-actions/auth/blob/main/action.yml
	"google-github-actions/auth": {
		MinMajor: 1,
		Source:   "https://github.com/google-github-actions/auth/blob/main/src/main.ts",
		Rules: []inputDependency{
			{Kind: inputExactlyOne, Inputs: []string{"workload_identity_provider", "credentials_json"}, Effect: `the action fails with "must specify exactly one of"`},
		},
	},

	// actions/dependency-review-action: "The allow-licenses and deny-licenses options are mutually exclusive; an
	// error will be raised if you provide both."
	// https://github.com/actions/dependency-review-action#configuration-options
	"actions/dependency-review-action": {
		MinMajor: 4,
		Source:   "https://github.com/actions/dependency-review-action#configuration-options",
		Rules: []inputDependency{
			{Kind: inputExclusive, Inputs: []string{"allow-licenses", "deny-licenses"}, Effect: "the action raises an error"},
		},
	},
}

// knownInputDependencies reports whether the data above judges the key "owner/repo@vN" (the form
// resolvePopularSpec derives from a tag or a version comment).
func knownInputDependencies(key string) bool {
	name, ref, ok := strings.Cut(key, "@")
	if !ok || !strings.HasPrefix(ref, "v") {
		return false
	}
	major, err := strconv.Atoi(ref[1:])
	if err != nil {
		return false
	}
	e, ok := inputDependencyTable[strings.ToLower(name)]
	return ok && major >= e.MinMajor
}

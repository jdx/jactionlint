package jactionlint

// actionKeyAvailability is the availability of contexts and special functions at the keys of an action.yml
// (action metadata), which the table generated from the documentation (WorkflowKeyAvailability) does not have.
// The values are lower case, like the ones of the generated table.
var actionKeyAvailability = map[string]struct{ contexts, functions []string }{
	// The default of an input is evaluated by the runner with the definition "input-default-context" of its
	// action_yaml.json, and the language service has the same list. A workflow context such as `env` or `inputs`
	// is not there, so the runner stops with "Unrecognized named-value".
	// https://github.com/actions/runner/blob/main/src/Runner.Worker/action_yaml.json
	// https://github.com/actions/languageservices/blob/main/workflow-parser/src/action-v1.0.json
	actionInputDefaultKey: {
		contexts:  []string{"github", "job", "matrix", "runner", "strategy"},
		functions: []string{"hashfiles"},
	},
	// `pre-if` and `post-if` are stored as a string by the runner and evaluated later with the contexts of the
	// job, so any context of a job can be in them. The language service lists a narrower set, which the runner
	// does not enforce, so only a name which is no context at all is reported (languageservices#317).
	actionRunsIfKey: {
		contexts:  []string{"env", "github", "inputs", "job", "matrix", "needs", "runner", "secrets", "steps", "strategy", "vars"},
		functions: []string{"always", "cancelled", "failure", "hashfiles", "success"},
	},
}

const (
	// actionInputDefaultKey is the key of the default of an input in an action.yml.
	actionInputDefaultKey = "inputs.<input_id>.default"
	// actionRunsIfKey is the key of `runs.pre-if` and `runs.post-if` in an action.yml.
	actionRunsIfKey = "runs.pre-if"
)

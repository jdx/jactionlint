package jactionlint

import (
	"regexp"
	"strings"
)

// This file is the table of actions which do not fail by default when the thing they exist for did not happen:
// no file was uploaded, no test result was found. The step is then green although the build is broken. Each entry
// names the input that makes the action fail, the default of the input and where both are documented, from the
// action.yml of the action. The rule fail-fast-action-inputs reads the table.

// failFastInput is an input which makes an action fail instead of going on.
type failFastInput struct {
	// Name is the input.
	Name string
	// Value is the value that makes the action fail.
	Value string
	// Default is what the action does when the input is not given, as the documentation says it.
	Default string
	// Problem says what goes unnoticed, for the message.
	Problem string
	// Relevant says whether the input matters for the step: some inputs only act on another input. It is nil
	// when it always does.
	Relevant func(a *ExecAction) bool
	// NoInput says that the ref of the action does not have the input (the first major version, say), and the
	// step is not reported. The comment is the one at the end of the line of `uses:`.
	NoInput func(ref *UsesRef, comment string) bool
	// Source is the documentation of the input.
	Source string
}

// hasInput reports whether the step gives the input, with any value (even a deliberate "warn").
func hasInput(a *ExecAction, name string) bool {
	i, ok := a.Inputs[strings.ToLower(name)]
	return ok && i != nil
}

// failFastTable maps the canonical name of an action to the input to set. It lists only inputs whose default and
// meaning are in the action.yml of the action.
var failFastTable = map[string]failFastInput{
	// actions/upload-artifact: `if-no-files-found`, default `warn`: "Output a warning but do not fail the action".
	// The first major version has no such input.
	// https://github.com/actions/upload-artifact/blob/main/action.yml
	"actions/upload-artifact": {
		Name: "if-no-files-found", Value: "error", Default: "warn",
		Problem: `the step only warns when "path" matches no file, so a build which produced nothing stays green and the artifact is missing`,
		NoInput: func(ref *UsesRef, comment string) bool { return majorOne(ref, comment) },
		Source:  "https://github.com/actions/upload-artifact/blob/main/action.yml",
	},
	// softprops/action-gh-release: `fail_on_unmatched_files`, "Fails if any of the `files` globs match nothing.
	// Defaults to false". It matters only when `files` is given.
	// https://github.com/softprops/action-gh-release/blob/master/action.yml
	"softprops/action-gh-release": {
		Name: "fail_on_unmatched_files", Value: "true", Default: "false",
		Problem:  `the release is created without the files when a "files" pattern matches nothing`,
		Relevant: func(a *ExecAction) bool { return hasInput(a, "files") },
		Source:   "https://github.com/softprops/action-gh-release/blob/master/action.yml",
	},
	// ncipollo/release-action: `artifactErrorsFailBuild`, "An optional flag which indicates if artifact read or
	// upload errors should fail the build", empty (false) by default. It matters only with `artifacts`.
	// https://github.com/ncipollo/release-action/blob/main/action.yml
	"ncipollo/release-action": {
		Name: "artifactErrorsFailBuild", Value: "true", Default: "false",
		Problem:  `the release is created without the artifacts when one of them cannot be read or uploaded`,
		Relevant: func(a *ExecAction) bool { return hasInput(a, "artifacts") },
		Source:   "https://github.com/ncipollo/release-action/blob/main/action.yml",
	},
	// EnricoMi/publish-unit-test-result-action: `action_fail_on_inconclusive`, "When set "true", the action itself
	// fails when tests are inconclusive (no test results)", false by default. The variants of the action share
	// the input.
	// https://github.com/EnricoMi/publish-unit-test-result-action/blob/master/action.yml
	"enricomi/publish-unit-test-result-action":              enricoMiEntry,
	"enricomi/publish-unit-test-result-action/composite":    enricoMiEntry,
	"enricomi/publish-unit-test-result-action/linux":        enricoMiEntry,
	"enricomi/publish-unit-test-result-action/macos":        enricoMiEntry,
	"enricomi/publish-unit-test-result-action/windows":      enricoMiEntry,
	"enricomi/publish-unit-test-result-action/windows/bash": enricoMiEntry,

	// mikepenz/action-junit-report: `require_tests`, "Fail if no test are found", false by default.
	// https://github.com/mikepenz/action-junit-report/blob/main/action.yml
	"mikepenz/action-junit-report": {
		Name: "require_tests", Value: "true", Default: "false",
		Problem: `the step passes when no test result was found, so a test run which did not run stays green`,
		Source:  "https://github.com/mikepenz/action-junit-report/blob/main/action.yml",
	},
}

var enricoMiEntry = failFastInput{
	Name: "action_fail_on_inconclusive", Value: "true", Default: "false",
	Problem: `the step passes when no test result was found, so a test run which did not run stays green`,
	Source:  "https://github.com/EnricoMi/publish-unit-test-result-action/blob/master/action.yml",
}

// v1RefRe matches the refs of the first major version.
var v1RefRe = regexp.MustCompile(`^v?1(\.[0-9]+)*$`)

// majorOne reports whether the step uses the first major version of the action: its ref is v1 or v1.x, or it is
// pinned to a commit and the comment after it says so.
func majorOne(ref *UsesRef, comment string) bool {
	if v1RefRe.MatchString(ref.Ref) {
		return true
	}
	if ref.RefKind == RefFullSHA && comment != "" {
		v, ok := commentVersion(comment)
		return ok && v1RefRe.MatchString(v)
	}
	return false
}

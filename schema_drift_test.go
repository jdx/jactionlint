package jactionlint

import (
	"slices"
	"strings"
	"testing"
)

// Keys and values which the workflow schema of GitHub accepts but its documentation does not list (yet) must not be
// reported by the rules of the correctness profile.

func TestWebhookTypesOfTheSchemaAreKnown(t *testing.T) {
	for _, hook := range []string{"pull_request", "pull_request_target"} {
		if !slices.Contains(AllWebhookTypes[hook], "stacked") {
			t.Errorf("%q does not accept the type \"stacked\": %v", hook, AllWebhookTypes[hook])
		}
	}
	for _, ty := range []string{"created", "ready", "deleted"} {
		if !slices.Contains(AllWebhookTypes["image_version"], ty) {
			t.Errorf("image_version does not accept the type %q: %v", ty, AllWebhookTypes["image_version"])
		}
	}
}

func TestSnapshotIfContextsAndFunctions(t *testing.T) {
	ctxs, funcs := WorkflowKeyAvailability("jobs.<job_id>.snapshot.if")
	for _, c := range []string{"github", "inputs", "vars", "needs", "strategy", "matrix", "steps", "job", "runner", "env"} {
		if !slices.Contains(ctxs, c) {
			t.Errorf("context %q is not available in snapshot.if: %v", c, ctxs)
		}
	}
	for _, c := range []string{"secrets", "jobs"} {
		if slices.Contains(ctxs, c) {
			t.Errorf("context %q must not be available in snapshot.if: %v", c, ctxs)
		}
	}
	for _, f := range []string{"always", "cancelled", "failure", "success", "hashfiles"} {
		if !slices.Contains(funcs, f) {
			t.Errorf("function %q is not available in snapshot.if: %v", f, funcs)
		}
		if !slices.Contains(SpecialFunctionNames[f], "jobs.<job_id>.snapshot.if") {
			t.Errorf("SpecialFunctionNames[%q] does not have snapshot.if: %v", f, SpecialFunctionNames[f])
		}
	}
}

func TestSnapshotIfUsesStepsOfTheJob(t *testing.T) {
	src := `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    snapshot:
      image-name: img
      if: steps.build.outcome == 'success' && steps.missing.outcome == 'success'
    steps:
      - id: build
        run: echo
`
	errs := errsWithID(lintFileWithConfig(t, fixtureConfig(), "test.yaml", src), "undefined-property")
	if len(errs) != 1 || !strings.Contains(errs[0].Message, `"missing"`) || errs[0].Line != 7 {
		t.Fatalf("want only the step which does not exist to be reported at line 7: %v", errs)
	}
}

func TestExcessivePermissionsOfScopesWithoutWriteLevel(t *testing.T) {
	// Every scope which allows `write` has the description that the message of the rule needs
	for scope, levels := range allPermissionScopes {
		if slices.Contains(levels, "write") && scopeWriteImpact[scope] == "" {
			t.Errorf("scope %q allows write but has no description in scopeWriteImpact", scope)
		}
	}

	// `models: write` is not a valid level, which the permissions rule reports. The rule for excessive
	// permissions must not repeat it with a message which has no description of the scope.
	src := `on: push
permissions:
  models: write
  drives: write
jobs:
  a:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - run: echo
  b:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - run: echo
`
	errs := lintFileWithConfig(t, ruleConfig("excessive-permissions", "permissions"), "test.yaml", src)
	excessive := errsWithID(errs, "excessive-permissions")
	if len(excessive) != 1 || !strings.Contains(excessive[0].Message, `"drives: write"`) || !strings.Contains(excessive[0].Message, "GitHub Drives") {
		t.Fatalf("want only drives to be reported: %v", excessive)
	}
	for _, e := range excessive {
		if strings.Contains(e.Message, "lets any of them ,") || strings.Contains(e.Message, "lets the job ,") {
			t.Errorf("the message has no description of the scope: %q", e.Message)
		}
	}
	wantLines(t, errs, "invalid-permissions", 3)
}

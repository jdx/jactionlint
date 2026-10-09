package jactionlint

import "slices"

// actionScope says whether a rule applies to the metadata of an action (action.yml) and how. The
// steps of a composite action are checked like the steps of a workflow job, but the rest of a
// workflow (triggers, permissions, runners, job settings) does not exist in an action: it belongs to
// the workflow which calls the action.
type actionScope int

const (
	// actionNotApplicable means that the rule is about something an action does not have, or that only
	// the caller can decide. It is not run for action.yml files.
	actionNotApplicable actionScope = iota
	// actionApplies means that the rule checks the steps of a composite action (or other parts of the
	// metadata) the same way as the steps of a workflow.
	actionApplies
	// actionCallerDependent means that the rule applies, and that its result depends on the context of the
	// run (the events which trigger the workflow). It judges the steps with the context of the local
	// workflows which call the action, see ActionCallers.
	actionCallerDependent
)

// actionRuleScope is how the rules treat action.yml files. The key is the name of the rule
// implementation (the name it registers its factory with), and except lists the IDs of that rule which are
// not reported for actions. A rule which is not listed does not run for actions: a rule added later
// must decide, and TestActionScopeIsComplete fails until it does.
var actionRuleScope = map[string]struct {
	scope  actionScope
	except []string
}{
	"action":                      {scope: actionApplies},
	"adhoc-packages":              {scope: actionApplies},         // package installs in the scripts of the steps
	"agentic-actions":             {scope: actionCallerDependent}, // an agent is steerable depending on the events of the callers
	"checkout-static-credentials": {scope: actionApplies},         // the inputs of actions/checkout steps
	"concurrency-cancels-prs":     {scope: actionNotApplicable},   // concurrency: belongs to the workflow
	"concurrency-cancels-release": {scope: actionNotApplicable},   // concurrency: belongs to the workflow
	"continue-on-error":           {scope: actionNotApplicable},   // a job setting; an action has no jobs
	"duplicate-triggers":          {scope: actionNotApplicable},   // on: belongs to the workflow
	"gate-job-skipped-on-failure": {scope: actionNotApplicable},   // needs: between the jobs of a workflow
	"github-env":                  {scope: actionCallerDependent}, // privileged only for the events of the callers; its findings about untrusted input do not depend on them
	"insecure-ssh-keyscan":        {scope: actionApplies},         // the scripts of the steps
	"insecure-url-scheme":         {scope: actionApplies},         // the scripts and inputs of the steps
	"mutable-runner-label":        {scope: actionNotApplicable},   // runs-on: belongs to the calling job
	"pipeline-without-pipefail":   {scope: actionApplies},         // the scripts of the steps, with the shell of each step
	"superfluous-actions":         {scope: actionApplies},         // the actions the steps use
	"unlocked-install":            {scope: actionApplies},         // the scripts of the steps
	"unpinned-tools":              {scope: actionApplies},         // the inputs of the actions the steps use
	"unsound-prefix-match":        {scope: actionApplies},         // the if: conditions and expressions of the steps
	"untrusted-artifact":          {scope: actionCallerDependent}, // privileged only when a caller runs on workflow_run
	"untrusted-checkout":          {scope: actionCallerDependent}, // privileged only when a caller runs on pull_request_target or workflow_run
	"unused-job-output":           {scope: actionNotApplicable},   // outputs between the jobs of a workflow
	"unused-needs":                {scope: actionNotApplicable},   // needs: between the jobs of a workflow
	"unused-workflow-input":       {scope: actionNotApplicable},   // inputs of workflow_dispatch and workflow_call
	"unverified-download":         {scope: actionApplies},         // the scripts of the steps
	"use-trusted-publishing":      {scope: actionApplies},         // the publishing steps; the id-token permission is the caller's
	"anonymous-definition":        {scope: actionNotApplicable},
	"archived-uses":               {scope: actionApplies},
	"artipacked":                  {scope: actionApplies},
	"bot-conditions":              {scope: actionCallerDependent},
	"cache-poisoning":             {scope: actionCallerDependent},
	"concurrency-limits":          {scope: actionNotApplicable},
	"credentials":                 {scope: actionNotApplicable},
	"dangerous-triggers":          {scope: actionNotApplicable},
	"deprecated-commands":         {scope: actionApplies},
	"env-var":                     {scope: actionApplies},
	"events":                      {scope: actionNotApplicable},
	"excessive-permissions":       {scope: actionNotApplicable},
	"expression":                  {scope: actionApplies},
	"forbidden-uses":              {scope: actionApplies},
	"github-app":                  {scope: actionApplies},
	"glob":                        {scope: actionNotApplicable},
	"id":                          {scope: actionApplies},
	"if-cond":                     {scope: actionApplies},
	"impostor-commit":             {scope: actionApplies},
	"insecure-commands":           {scope: actionApplies},
	"job-needs":                   {scope: actionNotApplicable},
	"known-vulnerable-actions":    {scope: actionApplies},
	"local-action-checkout":       {scope: actionNotApplicable},
	"matrix":                      {scope: actionNotApplicable},
	"misfeature":                  {scope: actionApplies},
	"obfuscation":                 {scope: actionApplies},
	"overprovisioned-secrets":     {scope: actionNotApplicable},
	"parallel-steps":              {scope: actionApplies},
	"permissions":                 {scope: actionNotApplicable},
	"pyflakes":                    {scope: actionApplies},
	"ref-confusion":               {scope: actionApplies},
	"ref-version-mismatch":        {scope: actionApplies},
	"require-permissions":         {scope: actionNotApplicable},
	"required-actions":            {scope: actionNotApplicable},
	"run-policy":                  {scope: actionApplies, except: []string{"require-shell"}},
	"runner-label":                {scope: actionNotApplicable},
	"secrets-inherit":             {scope: actionNotApplicable},
	"secrets-outside-env":         {scope: actionNotApplicable},
	"self-hosted-runner":          {scope: actionNotApplicable},
	"self-repository":             {scope: actionApplies},
	"shell-name":                  {scope: actionApplies},
	"shellcheck":                  {scope: actionApplies},
	"stale-action-refs":           {scope: actionApplies},
	"timeout-check":               {scope: actionNotApplicable},
	"typosquat-uses":              {scope: actionApplies},
	"undocumented-permissions":    {scope: actionNotApplicable},
	"unpinned-images":             {scope: actionApplies},
	"unredacted-secrets":          {scope: actionNotApplicable},
	"unsound-contains":            {scope: actionApplies},
	"workflow-call":               {scope: actionNotApplicable},
	"workflow-run":                {scope: actionNotApplicable},
}

// runsOnActions reports whether the rule implementation is run for the metadata of an action.
func runsOnActions(name string) bool {
	return actionRuleScope[name].scope != actionNotApplicable
}

// dropsOnActions reports whether the rule ID is not reported for the metadata of an action although the
// rule runs for it.
func dropsOnActions(factory, id string) bool {
	return slices.Contains(actionRuleScope[factory].except, id)
}

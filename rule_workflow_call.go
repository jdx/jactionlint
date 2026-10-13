package jactionlint

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// RuleWorkflowCall is a rule checker to check workflow call at jobs.<job_id>.
type RuleWorkflowCall struct {
	RuleBase
	workflowCallEventPos *Pos
	workflowPath         string
	cache                *LocalReusableWorkflowCache
	workflow             *Workflow
	curJob               *Job
	// online reads the workflows of other repositories; nil when the online checks are off.
	online *onlineSession
}

// NewRuleWorkflowCall creates a new RuleWorkflowCall instance. 'workflowPath' is a file path to
// the workflow which is relative to a project root directory or an absolute path.
func NewRuleWorkflowCall(workflowPath string, cache *LocalReusableWorkflowCache) *RuleWorkflowCall {
	return &RuleWorkflowCall{
		RuleBase: RuleBase{
			name: "workflow-call",
			desc: "Checks for reusable workflow calls. Inputs and outputs of called reusable workflow are checked",
		},
		workflowCallEventPos: nil,
		workflowPath:         workflowPath,
		cache:                cache,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleWorkflowCall) VisitWorkflowPre(n *Workflow) error {
	rule.workflow = n
	for _, e := range n.On {
		if e, ok := e.(*WorkflowCallEvent); ok {
			rule.workflowCallEventPos = e.Pos
			// Register this reusable workflow in cache so that it does not need to parse this workflow
			// file again when this workflow is called by other workflows.
			rule.cache.WriteWorkflowCallEventFromWorkflow(rule.workflowPath, e, n)
			break
		}
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleWorkflowCall) VisitJobPre(n *Job) error {
	rule.curJob = n
	if n.WorkflowCall == nil {
		return nil
	}

	u := n.WorkflowCall.Uses
	if u == nil || u.Value == "" || u.ContainsExpression() {
		return nil
	}

	ref := ParseUses(u.Value)

	if msg := selfRepositoryProblem(u.Value); msg != "" {
		if s, ok := canonLocalUsesSpec(u.Value); ok {
			// Do not report that the file cannot be read in addition
			rule.cache.writeCache(s, nil)
		}
		rule.ReportID("invalid-workflow-call", u.Pos, msg)
		return nil
	}

	if ref.isLocalWorkflowCall() {
		rule.checkWorkflowCallUsesLocal(n.WorkflowCall)
		return nil
	}

	if ref.isRepoWorkflowCall() {
		if msg := unpinnedUsesMessage(rule.Config(), ref, true); msg != "" {
			rule.ReportID("unpinned-uses", u.Pos, msg)
		}
		rule.checkWorkflowCallUsesRemote(n.WorkflowCall, ref)
		return nil
	}

	if s, ok := canonLocalUsesSpec(u.Value); ok {
		// When the specification is invalid and it is local reusable workflow call, remember it caused
		// an error by setting `nil` to cache. This can prevent redundant 'could not read workflow call'
		// error.
		rule.cache.writeCache(s, nil)
	}

	rule.ReportIDf(
		"invalid-workflow-call",
		u.Pos,
		"reusable workflow call %q at \"uses\" is not following the format \"owner/repo/path/to/workflow.yml@ref\" nor \"./path/to/workflow.yml\" nor \"$/path/to/workflow.yml\". see https://docs.github.com/en/actions/learn-github-actions/reusing-workflows for more details",
		u.Value,
	)
	return nil
}

func (rule *RuleWorkflowCall) checkWorkflowCallUsesLocal(call *WorkflowCall) {
	u := call.Uses
	m, err := rule.cache.FindMetadata(u.Value)
	if err != nil {
		rule.ReportID("invalid-local-workflow", u.Pos, err.Error())
		return
	}
	if m == nil {
		rule.Debug("Skip workflow call %q since no metadata was found", u.Value)
		return
	}

	rule.checkWorkflowCallInputsAndSecrets(call, m, strconv.Quote(u.Value)+" reusable workflow")
	rule.checkWorkflowCallPermissions(call, m)

	rule.Debug("Validated reusable workflow %q", u.Value)
}

// remoteWorkflowLookup returns whether GitHub can be asked for the reusable workflow of the `uses:` value:
// {owner}/{repo}/{path}.yml@{ref} (or .yaml) with plain names and no expression.
func remoteWorkflowLookup(ref *UsesRef) bool {
	if !ref.isRepoWorkflowCall() || ref.Dynamic || !ref.HasRef || ref.RefKind == RefShortSHA {
		return false
	}
	if !validGitHubOwner(ref.Owner) || !validGitHubRepo(ref.Repo) || !validGitRefName(ref.Ref) {
		return false
	}
	return isWorkflowFilePath(ref.Subpath) && validRepoFilePath(ref.Subpath)
}

// checkWorkflowCallUsesRemote checks a call of a reusable workflow of another repository against the workflow
// file at the pinned ref, read from GitHub with --online. When the file cannot be read nothing is reported.
func (rule *RuleWorkflowCall) checkWorkflowCallUsesRemote(call *WorkflowCall, ref *UsesRef) {
	if rule.online == nil || !remoteWorkflowLookup(ref) {
		return
	}
	m, err := rule.online.RemoteWorkflow(ref.Owner, ref.Repo, ref.Subpath, ref.Ref)
	if err != nil || m == nil {
		rule.Debug("%s: no workflow metadata to check the call against: %v", ref.Raw, err)
		return
	}
	rule.checkWorkflowCallInputsAndSecrets(call, m, strconv.Quote(ref.Raw)+" reusable workflow (read from GitHub with --online)")
	rule.Debug("Validated reusable workflow %q read from GitHub", ref.Raw)
}

// checkWorkflowCallInputsAndSecrets compares the inputs and the secrets of a call with the ones the workflow declares.
// what names the workflow in the messages.
func (rule *RuleWorkflowCall) checkWorkflowCallInputsAndSecrets(call *WorkflowCall, m *ReusableWorkflowMetadata, what string) {
	u := call.Uses

	// Validate inputs
	for n, i := range m.Inputs {
		if i != nil && i.Required {
			if _, ok := call.Inputs[n]; !ok {
				rule.ReportIDf("missing-workflow-input", u.Pos, "input %q is required by %s", i.Name, what)
			}
		}
	}
	for n, i := range call.Inputs {
		if _, ok := m.Inputs[n]; !ok {
			note := "no input is defined"
			if len(m.Inputs) > 0 {
				is := make([]string, 0, len(m.Inputs))
				for _, i := range m.Inputs {
					is = append(is, i.Name)
				}
				if len(is) == 1 {
					note = fmt.Sprintf("defined input is %q", is[0])
				} else {
					note = "defined inputs are " + sortedQuotes(is)
				}
			}
			rule.ReportIDf("unknown-workflow-input", i.Name.Pos, "input %q is not defined in %s. %s", i.Name.Value, what, note)
		}
	}

	// Validate secrets
	if !call.InheritSecrets {
		for n, s := range m.Secrets {
			if s.Required {
				if _, ok := call.Secrets[n]; !ok {
					rule.ReportIDf("missing-workflow-secret", u.Pos, "secret %q is required by %s", s.Name, what)
				}
			}
		}
		for n, s := range call.Secrets {
			if _, ok := m.Secrets[n]; !ok {
				note := "no secret is defined"
				if len(m.Secrets) > 0 {
					ss := make([]string, 0, len(m.Secrets))
					for _, s := range m.Secrets {
						ss = append(ss, s.Name)
					}
					if len(ss) == 1 {
						note = fmt.Sprintf("defined secret is %q", ss[0])
					} else {
						note = "defined secrets are " + sortedQuotes(ss)
					}
				}
				rule.ReportIDf("unknown-workflow-secret", s.Name.Pos, "secret %q is not defined in %s. %s", s.Name.Value, what, note)
			}
		}
	}
}

// checkWorkflowCallPermissions compares each callee job's effective `permissions:` requirement
// against the caller job's effective grant. Emits an error per missing scope. The check ignores
// `if:` on callee jobs because GitHub validates permissions at workflow load time regardless of
// the runtime gate.
func (rule *RuleWorkflowCall) checkWorkflowCallPermissions(call *WorkflowCall, m *ReusableWorkflowMetadata) {
	if len(m.JobPermissions) == 0 {
		return
	}

	// The token of a caller that sets no permissions has the permissions of the repository setting (Settings, Actions,
	// General), which a workflow file does not tell. Unless the configuration says which one to assume
	// ("assume-default-permissions"), the benefit of the doubt goes to the caller: only what no default token has
	// (id-token, attestations written by a restricted token, ...) is missing.
	cfg := rule.Config()
	mode := AssumeDefaultPermissionsPermissive
	assumed := ""
	if cfg != nil && cfg.AssumeDefaultPermissions != nil {
		mode = *cfg.AssumeDefaultPermissions
		assumed = mode
	}

	// Caller's effective permissions: job-level wins over workflow-level. A nil callerPerm means
	// the caller declared no `permissions:` anywhere; per-scope levels then come from
	// silentDefaultLevel rather than the explicit-block "absent = none" rule.
	var callerPerm *ReusableWorkflowPermissions
	if rule.curJob != nil && rule.curJob.Permissions != nil {
		callerPerm = convertASTPermissions(rule.curJob.Permissions)
	} else if rule.workflow != nil && rule.workflow.Permissions != nil {
		callerPerm = convertASTPermissions(rule.workflow.Permissions)
	} else if rule.workflowCallEventPos != nil {
		// The caller is itself a reusable workflow without any `permissions:`. GitHub then hands it the
		// token permissions of its own (unknown) upstream caller, not the repository default, so there
		// is nothing reliable to compare against.
		rule.Debug("Skip permissions check since the caller is a reusable workflow without permissions")
		return
	}

	callerLevel := func(scope string) int {
		if callerPerm == nil {
			return silentDefaultLevel(mode, scope)
		}
		return effectiveLevel(callerPerm, scope)
	}

	// Iterate callee jobs in a stable order so error messages are deterministic.
	ids := make([]string, 0, len(m.JobPermissions))
	for id := range m.JobPermissions {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	u := call.Uses
	for _, id := range ids {
		jp := m.JobPermissions[id]
		if jp == nil {
			continue
		}
		required := requiredScopeLevels(jp)

		scopes := make([]string, 0, len(required))
		for s := range required {
			scopes = append(scopes, s)
		}
		slices.Sort(scopes)
		// Report all insufficient scopes of a job in one error to avoid flooding the output
		// (e.g. a callee with `read-all` called by a caller granting a single scope).
		var wants, haves []string
		for _, scope := range scopes {
			want := required[scope]
			have := callerLevel(scope)
			if have < want {
				wants = append(wants, strconv.Quote(scope+": "+permissionLevelName(want)))
				haves = append(haves, strconv.Quote(scope+": "+permissionLevelName(have)))
			}
		}
		if len(wants) > 0 {
			msg := fmt.Sprintf(
				"nested job %q of %q requires %s but the calling job grants %s",
				id, u.Value,
				strings.Join(wants, ", "),
				strings.Join(haves, ", "),
			)
			if callerPerm == nil {
				if assumed == "" {
					msg += ". neither the calling job nor its workflow sets \"permissions:\", so the token has the default of the repository, and no default token has these permissions"
				} else {
					msg += fmt.Sprintf(". neither the calling job nor its workflow sets \"permissions:\", so the token has the default of the repository, which is assumed to be %s by \"assume-default-permissions\"", assumed)
				}
			}
			rule.ReportID("workflow-call-permissions", u.Pos, msg)
		}
	}
}

// requiredScopeLevels returns the scopes the callee actually requires (level > none), mapped to
// the minimum permission level the caller must grant. Both `read-all`/`write-all` and per-scope
// values are clamped to what each scope actually allows (e.g. `id-token: read` collapses to
// `none` because id-token only supports write).
func requiredScopeLevels(jp *ReusableWorkflowPermissions) map[string]int {
	required := map[string]int{}
	if jp.All != "" {
		level := permLevelNone
		switch jp.All {
		case "write-all":
			level = permLevelWrite
		case "read-all":
			level = permLevelRead
		}
		if level > permLevelNone {
			for scope := range allPermissionScopes {
				if l := clampLevelForScope(scope, level); l > permLevelNone {
					required[scope] = l
				}
			}
		}
		return required
	}
	for scope, val := range jp.Scopes {
		if _, ok := allPermissionScopes[scope]; !ok {
			continue
		}
		if l := clampLevelForScope(scope, permissionLevel(val)); l > permLevelNone {
			required[scope] = l
		}
	}
	return required
}

func init() {
	registerRules(
		RuleInfo{ID: "invalid-local-workflow", Group: RuleGroupCorrectness, Summary: "A local reusable workflow cannot be loaded or is invalid.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-reusable-workflows"},
		RuleInfo{ID: "invalid-workflow-call", Group: RuleGroupCorrectness, Summary: "A reusable workflow call does not follow the format of a reusable workflow.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-reusable-workflows"},
		RuleInfo{ID: "missing-workflow-input", Group: RuleGroupCorrectness, Summary: "A required input of a reusable workflow is not specified.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-reusable-workflows"},
		RuleInfo{ID: "missing-workflow-secret", Group: RuleGroupCorrectness, Summary: "A required secret of a reusable workflow is not passed.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-reusable-workflows"},
		RuleInfo{ID: "unknown-workflow-input", Group: RuleGroupCorrectness, Summary: "An input which the reusable workflow does not define is specified.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-reusable-workflows"},
		RuleInfo{ID: "unknown-workflow-secret", Group: RuleGroupCorrectness, Summary: "A secret which the reusable workflow does not define is passed.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-reusable-workflows"},
		RuleInfo{ID: "workflow-call-permissions", Group: RuleGroupCorrectness, Summary: "A caller job grants fewer permissions than a reusable workflow requires.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-reusable-workflows"},
	)
	registerRuleFactory("workflow-call", func(env *RuleEnv) []Rule {
		r := NewRuleWorkflowCall(env.path, env.localReusableWorkflows)
		r.online = env.online
		return []Rule{r}
	})
}

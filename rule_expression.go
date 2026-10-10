package jactionlint

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

//go:generate go run ./scripts/generate-availability ./availability.go

type typedExpr struct {
	ty  ExprType
	pos Pos
}

// RuleExpression is a rule checker to check expression syntax in string values of workflow syntax.
// It checks syntax and semantics of the expressions including type checks and functions/contexts
// definitions. For more details see
// - https://docs.github.com/en/actions/learn-github-actions/contexts
// - https://docs.github.com/en/actions/learn-github-actions/expressions
type RuleExpression struct {
	RuleBase
	origin           *exprOrigin // where the expression being checked comes from
	matrixTy         *ObjectType
	stepsTy          *ObjectType
	needsTy          *ObjectType
	secretsTy        *ObjectType
	inputsTy         *ObjectType
	dispatchInputsTy *ObjectType
	jobsTy           *ObjectType
	workflow         *Workflow
	localActions     *LocalActionsCache
	localWorkflows   *LocalReusableWorkflowCache

	// The following fields let a template-injection finding carry a fix (see template_injection.go).
	src       *sourceIndex
	curJob    *Job
	tiMatrix  map[string]bool // template injection: the matrix of curJob is looked at once (see tiContext.matrix)
	curStep   *Step
	scriptRun *ExecRun // the step when the script being checked is its run: script
	scriptStr *String  // the script being checked
	fixPlan   *tiFixPlan
	planned   bool
}

// NewRuleExpression creates new RuleExpression instance.
func NewRuleExpression(actionsCache *LocalActionsCache, workflowCache *LocalReusableWorkflowCache) *RuleExpression {
	return &RuleExpression{
		RuleBase: RuleBase{
			name: "expression",
			desc: "Syntax and semantics checks for expressions embedded with ${{ }} syntax",
		},
		matrixTy:         nil,
		stepsTy:          nil,
		needsTy:          nil,
		secretsTy:        nil,
		inputsTy:         nil,
		dispatchInputsTy: nil,
		jobsTy:           nil,
		workflow:         nil,
		localActions:     actionsCache,
		localWorkflows:   workflowCache,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleExpression) VisitWorkflowPre(n *Workflow) error {
	if n.Action != nil {
		rule.workflow = n
		rule.visitActionPre(n.Action)
		return nil
	}
	rule.checkString(n.Name, "")

	// Declared workflow_call secrets are exhaustive only when no other event can trigger the workflow.
	workflowCallOnly := true
	for _, e := range n.On {
		if _, ok := e.(*WorkflowCallEvent); !ok {
			workflowCallOnly = false
			break
		}
	}

	for _, e := range n.On {
		switch e := e.(type) {
		case *WebhookEvent:
			rule.checkStrings(e.Types, "")
			rule.checkWebhookEventFilter(e.Branches)
			rule.checkWebhookEventFilter(e.BranchesIgnore)
			rule.checkWebhookEventFilter(e.Tags)
			rule.checkWebhookEventFilter(e.TagsIgnore)
			rule.checkWebhookEventFilter(e.Paths)
			rule.checkWebhookEventFilter(e.PathsIgnore)
			rule.checkStrings(e.Workflows, "")
		case *ScheduledEvent:
			for _, s := range e.Schedules {
				rule.checkString(s.Cron, "")
				rule.checkString(s.Timezone, "")
			}
		case *WorkflowDispatchEvent:
			ity := NewEmptyStrictObjectType()
			for id, i := range e.Inputs {
				rule.checkString(i.Description, "")
				rule.checkString(i.Default, "")
				rule.checkBool(i.Required, "")
				rule.checkStrings(i.Options, "")

				var ty ExprType
				switch i.Type {
				case WorkflowDispatchEventInputTypeBoolean:
					ty = BoolType{}
				case WorkflowDispatchEventInputTypeNumber:
					ty = NumberType{}
				case WorkflowDispatchEventInputTypeString, WorkflowDispatchEventInputTypeChoice, WorkflowDispatchEventInputTypeEnvironment:
					ty = StringType{}
				default:
					ty = AnyType{}
				}
				ity.Props[id] = ty
			}
			rule.dispatchInputsTy = ity
		case *RepositoryDispatchEvent:
			rule.checkStrings(e.Types, "")
		case *WorkflowCallEvent:
			ity := NewEmptyStrictObjectType()

			// Set `inputs` context object before checking inputs since input's default values can refer `inputs` context.
			//   inputs:
			//     input1:
			//       type: string
			//     input2:
			//       type: string
			//       default: ${{ inputs.input1 }}
			rule.inputsTy = ity

			for _, i := range e.Inputs {
				rule.checkString(i.Description, "")
				// Check default value before setting type to `ity` because referring myself should cause an error.
				//   inputs:
				//     recursive:
				//       type: string
				//       default: ${{ inputs.recursive }}
				ts := rule.checkString(i.Default, "on.workflow_call.inputs.<inputs_id>.default")

				var ty ExprType
				switch i.Type {
				case WorkflowCallEventInputTypeString:
					ty = StringType{}
				case WorkflowCallEventInputTypeBoolean:
					ty = BoolType{}
					if len(ts) == 1 && i.Default.IsExpressionAssigned() {
						switch ts[0].ty.(type) {
						case BoolType, AnyType:
							// ok
						default:
							rule.ReportIDf("expression-type", i.Default.Pos, "type of input %q must be bool but found type %s", i.Name.Value, ts[0].ty.String())
						}
					}
				case WorkflowCallEventInputTypeNumber:
					ty = NumberType{}
					if len(ts) == 1 && i.Default.IsExpressionAssigned() {
						switch ts[0].ty.(type) {
						case NumberType, AnyType:
							// ok
						default:
							rule.ReportIDf("expression-type", i.Default.Pos, "type of input %q must be number but found type %s", i.Name.Value, ts[0].ty.String())
						}
					}
				default:
					ty = AnyType{}
				}
				ity.Props[i.ID] = ty
			}

			// When no secret is passed, secrets may be inherited from a caller of the workflow.
			// So `secrets` context must be typed as { string => string }. `e.Secrets` is nil when `secrets:` does not
			// exist. When `e.Secrets` is an empty map, `secrets:` exists but it has no child.
			if e.Secrets != nil {
				sty := NewEmptyStrictObjectType()
				for id, s := range e.Secrets {
					sty.Props[id] = StringType{}
					rule.checkString(s.Description, "")
				}
				if workflowCallOnly && !n.inheritedSecrets {
					rule.secretsTy = sty
				}
			}

			for _, o := range e.Outputs {
				rule.checkString(o.Description, "")
				// o.Value will be checked in VisitWorkflowPost
			}
		case *ImageVersionEvent:
			rule.checkStrings(e.Names, "")
			rule.checkStrings(e.Versions, "")
		}
	}

	rule.checkString(n.RunName, "run-name")
	rule.checkEnv(n.Env, "env")

	rule.checkDefaults(n.Defaults, "")
	rule.checkConcurrency(n.Concurrency, "concurrency")

	rule.workflow = n
	return nil
}

// VisitWorkflowPost is callback when visiting Workflow node after visiting its children
func (rule *RuleExpression) VisitWorkflowPost(n *Workflow) error {
	if n.Action != nil {
		rule.workflow = nil
		return nil
	}
	if e, ok := n.FindWorkflowCallEvent(); ok {
		rule.checkWorkflowCallOutputs(e.Outputs, n.Jobs)
	}
	rule.workflow = nil
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleExpression) VisitJobPre(n *Job) error {
	rule.curJob, rule.tiMatrix = n, map[string]bool{}
	// Type of needs must be resolved before resolving type of matrix because `needs` context can
	// be used in matrix configuration.
	rule.needsTy = rule.calcNeedsType(n)

	// Set matrix type at start of VisitJobPre() because matrix values are available in
	// jobs.<job_id> section. For example:
	//   jobs:
	//     foo:
	//       strategy:
	//         matrix:
	//           os: [ubuntu-latest, macos-latest, windows-latest]
	//       runs-on: ${{ matrix.os }}
	if n.Strategy != nil && n.Strategy.Matrix != nil {
		// Check and guess type of the matrix
		rule.matrixTy = rule.checkMatrix(n.Strategy.Matrix)
	}

	rule.checkString(n.Name, "jobs.<job_id>.name")
	rule.checkStrings(n.Needs, "")

	if n.RunsOn != nil {
		if n.RunsOn.LabelsExpr != nil {
			if ty := rule.checkOneExpression(n.RunsOn.LabelsExpr, "runner label at \"runs-on\" section", "jobs.<job_id>.runs-on"); ty != nil {
				switch ty.(type) {
				case *ArrayType, StringType, AnyType:
					// OK
				default:
					rule.ReportIDf("expression-type", n.RunsOn.LabelsExpr.Pos, "type of expression at \"runs-on\" must be string or array but found type %q", ty.String())
				}
			}
		} else {
			for _, l := range n.RunsOn.Labels {
				rule.checkString(l, "jobs.<job_id>.runs-on")
			}
		}
		rule.checkString(n.RunsOn.Group, "jobs.<job_id>.runs-on")
	}

	rule.checkConcurrency(n.Concurrency, "jobs.<job_id>.concurrency")

	rule.checkEnv(n.Env, "jobs.<job_id>.env")

	rule.checkDefaults(n.Defaults, "jobs.<job_id>.defaults.run")
	rule.checkIfCondition(n.If, "jobs.<job_id>.if")

	if n.Strategy != nil {
		// Note: Types in "jobs.<job_id>.strategy.matrix" were checked `checkMatrix`
		rule.checkBool(n.Strategy.FailFast, "jobs.<job_id>.strategy")
		rule.checkInt(n.Strategy.MaxParallel, "jobs.<job_id>.strategy")
	}

	rule.checkBool(n.ContinueOnError, "jobs.<job_id>.continue-on-error")
	rule.checkFloat(n.TimeoutMinutes, "jobs.<job_id>.timeout-minutes")
	rule.checkContainer(n.Container, "jobs.<job_id>.container", "")

	if n.Services != nil {
		rule.checkObjectExpression(n.Services.Expression, "services", "jobs.<job_id>.services")
		for _, s := range n.Services.Value {
			rule.checkContainer(s.Container, "jobs.<job_id>.services", "<service_id>")
		}
	}

	rule.checkWorkflowCall(n.WorkflowCall)
	rule.checkSnapshot(n.Snapshot)

	rule.stepsTy = NewEmptyStrictObjectType()

	if n.Composite {
		// The matrix and the needs of the job which calls the action are not known here
		rule.matrixTy = NewEmptyObjectType()
		rule.needsTy = NewEmptyObjectType()
	}

	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children
func (rule *RuleExpression) VisitJobPost(n *Job) error {
	rule.curJob, rule.tiMatrix = nil, nil
	// 'environment' and 'outputs' sections are evaluated after all steps are run
	if n.Environment != nil {
		rule.checkString(n.Environment.Name, "jobs.<job_id>.environment")
		rule.checkString(n.Environment.URL, "jobs.<job_id>.environment.url")
		rule.checkBool(n.Environment.Deployment, "jobs.<job_id>.environment")
	}
	for _, output := range n.Outputs {
		rule.checkString(output.Value, "jobs.<job_id>.outputs.<output_id>")
	}
	if n.Composite && rule.workflow != nil && rule.workflow.Action != nil {
		// The outputs of a composite action are evaluated after all of its steps ran
		for _, o := range rule.workflow.Action.Outputs {
			rule.checkString(o.Value, "jobs.<job_id>.steps.run")
		}
	}

	rule.matrixTy = nil
	rule.stepsTy = nil
	rule.needsTy = nil

	return nil
}

// stepParallelKey is the workflow key used to look up context availability for the 'background', 'wait',
// and 'cancel' step fields. The context availability table does not list these new keys yet, so they
// are treated like their sibling 'continue-on-error' field.
const stepParallelKey = "jobs.<job_id>.steps.continue-on-error"

// VisitStep is callback when visiting Step node.
func (rule *RuleExpression) VisitStep(n *Step) error {
	rule.curStep = n
	defer func() { rule.curStep = nil }()
	rule.checkString(n.Name, "jobs.<job_id>.steps.name")
	rule.checkIfCondition(n.If, "jobs.<job_id>.steps.if")

	var spec *String
	switch e := n.Exec.(type) {
	case *ExecRun:
		rule.scriptRun = e
		rule.checkScriptString(e.Run, "jobs.<job_id>.steps.run")
		rule.scriptRun = nil
		rule.checkString(e.Shell, "")
		rule.checkString(e.WorkingDirectory, "jobs.<job_id>.steps.working-directory")
	case *ExecAction:
		rule.checkString(e.Uses, "")
		for n, i := range e.Inputs {
			if e.Uses != nil && isCodeExecInput(e.Uses.Value, n) {
				rule.checkScriptString(i.Value, "jobs.<job_id>.steps.with")
			} else {
				rule.checkString(i.Value, "jobs.<job_id>.steps.with")
			}
		}
		rule.checkString(e.Entrypoint, "jobs.<job_id>.steps.with")
		rule.checkString(e.Args, "jobs.<job_id>.steps.with")
		spec = e.Uses
	case *ExecWait:
		for _, name := range e.Names {
			rule.checkString(name, stepParallelKey)
		}
	case *ExecCancel:
		rule.checkString(e.Name, stepParallelKey)
	}

	rule.checkEnv(n.Env, "jobs.<job_id>.steps.env") // env: at step level can refer 'env' context (#158)
	rule.checkBool(n.ContinueOnError, "jobs.<job_id>.steps.continue-on-error")
	rule.checkFloat(n.TimeoutMinutes, "jobs.<job_id>.steps.timeout-minutes")
	rule.checkBool(n.Background, stepParallelKey)

	if n.ID != nil {
		if n.ID.ContainsExpression() {
			rule.checkString(n.ID, "")
			rule.stepsTy.Loose()
		}
		// Step ID is case insensitive
		id := strings.ToLower(n.ID.Value)
		rule.stepsTy.Props[id] = NewStrictObjectType(map[string]ExprType{
			"outputs":    rule.getActionOutputsType(spec),
			"conclusion": StringType{},
			"outcome":    StringType{},
		})
	}

	return nil
}

// Get type of `outputs.<output name>`
func (rule *RuleExpression) getActionOutputsType(spec *String) *ObjectType {
	if spec == nil {
		return NewMapObjectType(StringType{})
	}

	if _, ok := canonLocalUsesSpec(spec.Value); ok {
		// A broken metadata file is reported by the action rule at every use of the action (see
		// RuleAction.reportOnce). Reporting it here too made the report depend on which rule searched first.
		meta, err := rule.localActions.Lookup(spec.Value)
		if err != nil || meta == nil {
			return NewMapObjectType(StringType{})
		}

		return typeOfActionOutputs(meta)
	}

	// github-script action allows to set any outputs through calling `core.setOutput` directly.
	// So any `outputs.*` properties should be accepted (#104)
	if strings.HasPrefix(spec.Value, "actions/github-script@") {
		return NewEmptyObjectType()
	}

	// When the action run at this step is a popular action, we know what outputs are set by it.
	// Set the output names to `steps.{step_id}.outputs.{name}`.
	if meta, ok := PopularActions[spec.Value]; ok {
		return typeOfActionOutputs(meta)
	}

	return NewMapObjectType(StringType{})
}

func (rule *RuleExpression) getWorkflowCallOutputsType(call *WorkflowCall) *ObjectType {
	if call.Uses == nil {
		return NewMapObjectType(StringType{})
	}

	m, err := rule.localWorkflows.FindMetadata(call.Uses.Value)
	if err != nil {
		rule.ReportID("invalid-local-workflow", call.Uses.Pos, err.Error())
		return NewMapObjectType(StringType{})
	}
	if m == nil {
		return NewMapObjectType(StringType{})
	}

	p := make(map[string]ExprType, len(m.Outputs))
	for n := range m.Outputs {
		p[n] = StringType{}
	}
	return NewStrictObjectType(p)
}

func (rule *RuleExpression) checkOneExpression(s *String, what, workflowKey string) ExprType {
	// checkString is not available since it checks types for embedding values into a string
	if s == nil {
		return nil
	}

	ts, ok := rule.checkExprsIn(s, false, workflowKey)
	if !ok {
		return nil
	}

	if len(ts) != 1 {
		// This case should be unreachable since only one ${{ }} is included is checked by parser
		rule.ReportIDf("expression-type", s.Pos, "one ${{ }} expression should be included in %q value but got %d expressions", what, len(ts))
		return nil
	}

	return ts[0].ty
}

func (rule *RuleExpression) checkObjectTy(ty ExprType, pos *Pos, what string) ExprType {
	if ty == nil {
		return nil
	}
	switch ty.(type) {
	case *ObjectType, AnyType:
		return ty
	default:
		rule.ReportIDf("expression-type", pos, "type of expression at %q must be object but found type %s", what, ty.String())
		return nil
	}
}

func (rule *RuleExpression) checkArrayTy(ty ExprType, pos *Pos, what string) ExprType {
	if ty == nil {
		return nil
	}
	switch ty.(type) {
	case *ArrayType, AnyType:
		return ty
	default:
		rule.ReportIDf("expression-type", pos, "type of expression at %q must be array but found type %s", what, ty.String())
		return nil
	}
}

func (rule *RuleExpression) checkNumberTy(ty ExprType, pos *Pos, what string) ExprType {
	if ty == nil {
		return nil
	}
	switch ty.(type) {
	case NumberType, AnyType:
		return ty
	default:
		rule.ReportIDf("expression-type", pos, "type of expression at %q must be number but found type %s", what, ty.String())
		return nil
	}
}

func (rule *RuleExpression) checkObjectExpression(s *String, what, workflowKey string) ExprType {
	ty := rule.checkOneExpression(s, what, workflowKey)
	if ty == nil {
		return nil
	}
	return rule.checkObjectTy(ty, s.Pos, what)
}

func (rule *RuleExpression) checkArrayExpression(s *String, what, workflowKey string) ExprType {
	ty := rule.checkOneExpression(s, what, workflowKey)
	if ty == nil {
		return nil
	}
	return rule.checkArrayTy(ty, s.Pos, what)
}

func (rule *RuleExpression) checkNumberExpression(s *String, what, workflowKey string) ExprType {
	ty := rule.checkOneExpression(s, what, workflowKey)
	if ty == nil {
		return nil
	}
	return rule.checkNumberTy(ty, s.Pos, what)
}

func (rule *RuleExpression) checkEnv(env *Env, workflowKey string) {
	if env == nil {
		return
	}

	if env.Vars != nil {
		for _, e := range env.Vars {
			rule.checkString(e.Name, workflowKey)
			rule.checkString(e.Value, workflowKey)
		}
		return
	}

	// When form of "env: ${{...}}"
	rule.checkObjectExpression(env.Expression, "env", workflowKey)
}

func (rule *RuleExpression) checkContainer(c *Container, workflowKey, childWorkflowKeyPrefix string) {
	if c == nil {
		return
	}
	childWorkflowKey := workflowKey
	if childWorkflowKeyPrefix != "" {
		childWorkflowKey += "." + childWorkflowKeyPrefix
	}
	rule.checkString(c.Image, workflowKey)
	if c.Credentials != nil {
		k := childWorkflowKey + ".credentials" // e.g. jobs.<job_id>.container.credentials
		if c.Credentials.Expression != nil {
			rule.checkObjectExpression(c.Credentials.Expression, "credentials", k)
		} else {
			rule.checkString(c.Credentials.Username, k)
			rule.checkString(c.Credentials.Password, k)
		}
	}
	rule.checkEnv(c.Env, childWorkflowKey+".env.<env_id>") // e.g. jobs.<job_id>.container.env.<env_id>
	rule.checkStrings(c.Ports, workflowKey)
	rule.checkStrings(c.Volumes, workflowKey)
	rule.checkString(c.Options, workflowKey)
	rule.checkString(c.Command, workflowKey)
	rule.checkString(c.Entrypoint, workflowKey)
}

func (rule *RuleExpression) checkConcurrency(c *Concurrency, workflowKey string) {
	if c == nil {
		return
	}
	rule.checkString(c.Group, workflowKey)
	rule.checkBool(c.CancelInProgress, workflowKey)
	rule.checkString(c.Queue, workflowKey)
}

func (rule *RuleExpression) checkDefaults(d *Defaults, workflowKey string) {
	if d == nil || d.Run == nil {
		return
	}
	rule.checkString(d.Run.Shell, workflowKey)
	rule.checkString(d.Run.WorkingDirectory, workflowKey)
}

func (rule *RuleExpression) checkWorkflowCall(c *WorkflowCall) {
	if c == nil || c.Uses == nil {
		return
	}

	rule.checkString(c.Uses, "")

	m, err := rule.localWorkflows.FindMetadata(c.Uses.Value)
	if err != nil {
		rule.ReportID("invalid-local-workflow", c.Uses.Pos, err.Error())
	}

	for n, i := range c.Inputs {
		ts := rule.checkString(i.Value, "jobs.<job_id>.with.<with_id>")

		if m == nil {
			continue
		}

		mi, ok := m.Inputs[n]
		if !ok || mi == nil {
			continue
		}
		if _, ok := mi.Type.(AnyType); ok {
			continue
		}

		v := strings.TrimSpace(i.Value.Value)

		var ty ExprType = StringType{}
		switch len(ts) {
		case 0:
			switch {
			case i.Value.Quoted:
				// A quoted YAML scalar is a string even when its text reads like a bool, a number
				// or null: "true" passed to a string input is fine.
			case v == "null":
				ty = NullType{}
			case v == "true" || v == "false":
				ty = BoolType{}
			default:
				if _, err := strconv.ParseFloat(v, 64); err == nil {
					ty = NumberType{}
				}
			}
		case 1:
			if i.Value.IsExpressionAssigned() {
				ty = ts[0].ty
			}
		}

		if !mi.Type.Assignable(ty) {
			rule.ReportIDf(
				"workflow-input-type",
				i.Value.Pos,
				"input %q is typed as %s by reusable workflow %q. %s value cannot be assigned",
				mi.Name,
				mi.Type.String(),
				c.Uses.Value,
				ty.String(),
			)
		}
	}

	for _, s := range c.Secrets {
		rule.checkString(s.Value, "jobs.<job_id>.secrets.<secrets_id>")
	}
}

func (rule *RuleExpression) checkSnapshot(s *Snapshot) {
	if s == nil {
		return
	}
	rule.checkIfCondition(s.If, "jobs.<job_id>.snapshot.if")
}

func (rule *RuleExpression) checkWebhookEventFilter(f *WebhookEventFilter) {
	if f == nil {
		return
	}
	rule.checkStrings(f.Values, "")
}

func (rule *RuleExpression) checkStrings(ss []*String, workflowKey string) {
	for _, s := range ss {
		rule.checkString(s, workflowKey)
	}
}

func (rule *RuleExpression) checkIfCondition(str *String, workflowKey string) {
	if str == nil {
		return
	}

	// Note:
	// https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idif
	//
	// > When you use expressions in an if conditional, you may omit the expression syntax (${{ }})
	// > because GitHub automatically evaluates the if conditional as an expression, unless the
	// > expression contains any operators. If the expression contains any operators, the expression
	// > must be contained within ${{ }} to explicitly mark it for evaluation.
	//
	// This document is actually wrong. I confirmed that any strings without surrounding in ${{ }}
	// are evaluated.
	//
	// - run: echo 'run'
	//   if: '!false'
	// - run: echo 'not run'
	//   if: '!true'
	// - run: echo 'run'
	//   if: false || true
	// - run: echo 'run'
	//   if: true && true
	// - run: echo 'not run'
	//   if: true && false

	var condTy ExprType
	if str.ContainsExpression() {
		ts := rule.checkString(str, workflowKey)

		if len(ts) == 1 {
			if str.IsExpressionAssigned() {
				condTy = ts[0].ty
			}
		}
	} else {
		src := str.Value + "}}" // }} is necessary since lexer lexes it as end of tokens
		rule.origin = &exprOrigin{str: str, text: src}
		defer func() { rule.origin = nil }()
		line, col := rule.originPos(0)

		expr, err := str.condExpr()
		if err != nil {
			rule.exprError(err, line, col)
			return
		}

		if ty, ok := rule.checkSemanticsOfExprNode(expr, line, col, false, workflowKey); ok {
			condTy = ty
		}
	}

	if condTy != nil && !(BoolType{}).Assignable(condTy) {
		rule.ReportIDf("expression-type", str.Pos, "\"if\" condition should be type \"bool\" but got type %q", condTy.String())
	}
}

func (rule *RuleExpression) checkTemplateEvaluatedType(ts []typedExpr) {
	for _, t := range ts {
		switch t.ty.(type) {
		case *ObjectType, *ArrayType, NullType:
			rule.ReportIDf("expression-type", &t.pos, "object, array, and null values should not be evaluated in template with ${{ }} but evaluating the value of type %s", t.ty)
		}
	}
}

func (rule *RuleExpression) checkString(str *String, workflowKey string) []typedExpr {
	if str == nil {
		return nil
	}

	ts, ok := rule.checkExprsIn(str, false, workflowKey)
	if !ok {
		return nil
	}

	rule.checkTemplateEvaluatedType(ts)
	return ts
}

func (rule *RuleExpression) checkScriptString(str *String, workflowKey string) {
	if str == nil {
		return
	}

	rule.scriptStr, rule.fixPlan, rule.planned = str, nil, false
	defer func() { rule.scriptStr, rule.fixPlan, rule.planned = nil, nil, false }()
	ts, ok := rule.checkExprsIn(str, true, workflowKey)
	if !ok {
		return
	}

	rule.checkTemplateEvaluatedType(ts)
}

func (rule *RuleExpression) checkBool(b *Bool, workflowKey string) {
	if b == nil || b.Expression == nil {
		return
	}

	ty := rule.checkOneExpression(b.Expression, "bool value", workflowKey)
	if ty == nil {
		return
	}

	switch ty.(type) {
	case BoolType, AnyType:
		// ok
	default:
		rule.ReportIDf("expression-type", b.Expression.Pos, "type of expression must be bool but found type %s", ty.String())
	}
}

func (rule *RuleExpression) checkInt(i *Int, workflowKey string) {
	if i == nil {
		return
	}
	rule.checkNumberExpression(i.Expression, "integer value", workflowKey)
}

func (rule *RuleExpression) checkFloat(f *Float, workflowKey string) {
	if f == nil {
		return
	}
	rule.checkNumberExpression(f.Expression, "float number value", workflowKey)
}

func (rule *RuleExpression) checkExprsIn(str *String, checkUntrusted bool, workflowKey string) ([]typedExpr, bool) {
	// Positions inside the string are mapped back to the source through the position of the string and
	// the way it is written (see exprOrigin).
	full := str.Value
	ts := []typedExpr{}
	defer func() { rule.origin = nil }()
	// The expressions are parsed once for all the rules which look at the string.
	parsed := str.placeholders()
	for _, p := range parsed.list {
		offset := p.after
		rule.origin = &exprOrigin{str: str, base: offset, text: full[offset:]}
		l, c := rule.originPos(0)

		nerrs := len(rule.errs)
		ty, ok := rule.checkSemanticsOfExprNode(p.node, l, c, checkUntrusted, workflowKey)
		rule.attachTemplateInjectionFix(nerrs, offset-len("${{"))
		if !ok {
			return nil, false
		}
		if ty == nil || p.n == 0 {
			return nil, true
		}
		dl, dc := rule.originPos(-len("${{"))
		ts = append(ts, typedExpr{ty, Pos{dl, dc}})
	}
	if parsed.failed {
		offset := parsed.errAfter
		rule.origin = &exprOrigin{str: str, base: offset, text: full[offset:]}
		l, c := rule.originPos(0)
		nerrs := len(rule.errs)
		if parsed.err != nil {
			rule.exprError(parsed.err, l, c)
		}
		rule.attachTemplateInjectionFix(nerrs, offset-len("${{"))
		return nil, false
	}

	return ts, true
}

// exprOrigin is where the text of the expression which is checked comes from: it starts at the byte
// offset base of the value of the string, and text is the rest of the value from there.
type exprOrigin struct {
	str  *String
	base int
	text string
}

// originPos returns the source position of the byte at the offset delta from the start of the
// expression text.
func (rule *RuleExpression) originPos(delta int) (line, col int) {
	o := rule.origin
	return rule.src.valuePosition(o.str, min(max(o.base+delta, 0), len(o.str.Value)))
}

// exprTextOffset converts the line and the column (both 1-based, the column in characters) in the text
// of an expression to a byte offset of the text. A position past the end gives the end of the text.
func exprTextOffset(text string, line, col int) int {
	off := 0
	for l := 1; l < line; l++ {
		i := strings.IndexByte(text[off:], '\n')
		if i < 0 {
			return len(text)
		}
		off += i + 1
	}
	for c := 1; c < col && off < len(text) && text[off] != '\n'; c++ {
		_, w := utf8.DecodeRuneInString(text[off:])
		off += w
	}
	return off
}

// attachTemplateInjectionFix puts the fix of the expansion which starts at the offset of the script on
// the template injection findings reported since the nth error.
func (rule *RuleExpression) attachTemplateInjectionFix(nerrs, start int) {
	if rule.scriptStr == nil || rule.scriptRun == nil || rule.src == nil {
		return
	}
	for _, e := range rule.errs[nerrs:] {
		if e.ID != "template-injection" {
			continue
		}
		if !rule.planned {
			rule.planned = true
			rule.fixPlan = planTemplateInjectionFixes(tiFixInput{
				idx: rule.src,
				ctx: tiContext{wf: rule.workflow, job: rule.curJob, step: rule.curStep, matrix: rule.tiMatrix},
				cfg: rule.config,
				str: rule.scriptStr,
				run: rule.scriptRun,
			})
		}
		rule.fixPlan.apply(e, start)
	}
}

// exprPos converts a position in an expression to a position in the source. The line and the column
// are relative to the text of the expression. A multi-line scalar (a block scalar, or a plain or quoted
// scalar over several lines) is followed line by line, so the position is on the line of the text in
// the file. The escapes of a quoted scalar and the folding of lines make it approximate (see
// sourceIndex.valuePosition). lineBase and colBase are the position of the start of the expression,
// which is used when the origin of the text is not known.
func (rule *RuleExpression) exprPos(line, col, lineBase, colBase int) *Pos {
	if o := rule.origin; o != nil {
		l, c := rule.originPos(exprTextOffset(o.text, line, col))
		return &Pos{Line: l, Col: c}
	}
	return convertExprLineColToPos(line, col, lineBase, colBase)
}

func (rule *RuleExpression) exprError(err *ExprError, lineBase, colBase int) {
	rule.ReportID(err.ID, rule.exprPos(err.Line, err.Column, lineBase, colBase), err.Message)
}

func (rule *RuleExpression) checkSemanticsOfExprNode(expr ExprNode, line, col int, checkUntrusted bool, workflowKey string) (ExprType, bool) {
	var v []string
	var s []string
	if rule.config != nil {
		v = rule.config.ConfigVariables
		s = rule.config.ConfigSecrets
	}
	c := NewExprSemanticsChecker(checkUntrusted, v, s)
	if rule.matrixTy != nil {
		c.UpdateMatrix(rule.matrixTy)
	}
	if rule.stepsTy != nil {
		c.UpdateSteps(rule.stepsTy)
	}
	if rule.needsTy != nil {
		c.UpdateNeeds(rule.needsTy)
	}
	if rule.secretsTy != nil {
		c.UpdateSecrets(rule.secretsTy)
	}
	if rule.inputsTy != nil {
		c.UpdateInputs(rule.inputsTy)
	}
	if rule.dispatchInputsTy != nil {
		c.UpdateDispatchInputs(rule.dispatchInputsTy)
	}
	if rule.jobsTy != nil {
		c.UpdateJobs(rule.jobsTy)
	}
	if workflowKey != "" {
		ctx, sp := WorkflowKeyAvailability(workflowKey)
		if len(ctx) == 0 {
			rule.Debug("No context availability was found for workflow key %q", workflowKey)
		}
		if rule.workflow != nil && rule.workflow.IsComposite() {
			ctx = compositeContexts(ctx)
		}
		c.SetContextAvailability(ctx)
		c.SetSpecialFunctionAvailability(sp)
	}

	ty, errs := c.Check(expr)
	if rule.workflow != nil && rule.workflow.IsComposite() {
		errs = compositeExprErrors(errs)
	}
	ok := true
	for _, err := range errs {
		if err.ID == "template-injection" {
			if note := rule.workflow.callerWarning(); note != "" {
				err.Message += ". " + note
			}
		}
		rule.exprError(err, line, col)
		// A potentially untrusted input is a finding about the script, not an error of the expression.
		// The other expressions of the script are still checked.
		if err.ID != "template-injection" {
			ok = false
		}
	}

	if rule.config.RuleEnabled("unsound-ternary") {
		rule.checkFalsyTernary(expr, line, col)
	}
	if rule.config.RuleEnabled("obfuscation") {
		rule.checkObfuscation(expr, line, col, workflowKey)
	}

	return ty, ok
}

// checkFalsyTernary reports `cond && <falsy literal> || other`. The `a && b || c` idiom works as a ternary
// operator only when `b` is truthy. When `b` is a literal which is always falsy (`”`, `0`, `false`, `null`),
// the expression always evaluates to `c`. (rhysd/actionlint#440)
func (rule *RuleExpression) checkFalsyTernary(expr ExprNode, line, col int) {
	VisitExprNode(expr, func(n, _ ExprNode, entering bool) {
		if !entering {
			return
		}
		or, ok := n.(*LogicalOpNode)
		if !ok || or.Kind != LogicalOpNodeKindOr {
			return
		}
		and, ok := or.Left.(*LogicalOpNode)
		if !ok || and.Kind != LogicalOpNodeKindAnd {
			return
		}
		// `a && b && c` may be parsed as `a && (b && c)`. The last operand decides the value when all are truthy.
		last := and.Right
		for {
			a, ok := last.(*LogicalOpNode)
			if !ok || a.Kind != LogicalOpNodeKindAnd {
				break
			}
			last = a.Right
		}
		if !isFalsyLiteral(last) {
			return
		}
		tok := last.Token()
		rule.ReportIDf(
			"unsound-ternary",
			rule.exprPos(tok.Line, tok.Column, line, col),
			"value %q after && is always falsy so the expression always evaluates to the value after ||. \"a && b || c\" works as a ternary only when b is truthy",
			tok.Value,
		)
	})
}

func isFalsyLiteral(n ExprNode) bool {
	switch n := n.(type) {
	case *NullNode:
		return true
	case *BoolNode:
		return !n.Value
	case *IntNode:
		return n.Value == 0
	case *FloatNode:
		return n.Value == 0
	case *StringNode:
		return n.Value == ""
	}
	return false
}

func (rule *RuleExpression) calcNeedsType(job *Job) *ObjectType {
	// https://docs.github.com/en/actions/learn-github-actions/contexts#needs-context
	o := NewEmptyStrictObjectType()
	rule.populateDependantNeedsTypes(o, job, job)
	return o
}

func (rule *RuleExpression) populateDependantNeedsTypes(out *ObjectType, job *Job, root *Job) {
	for _, id := range job.Needs {
		i := strings.ToLower(id.Value) // ID is case insensitive
		if i == root.ID.Value {
			continue // When cyclic dependency exists. This does not happen normally.
		}
		if _, ok := out.Props[i]; ok {
			continue // Already added
		}

		j, ok := rule.workflow.Jobs[i]
		if !ok {
			continue
		}

		var outputs *ObjectType
		if j.WorkflowCall == nil {
			outputs = NewEmptyStrictObjectType()
			for name := range j.Outputs {
				outputs.Props[name] = StringType{}
			}
		} else {
			outputs = rule.getWorkflowCallOutputsType(j.WorkflowCall)
		}

		out.Props[i] = NewStrictObjectType(map[string]ExprType{
			"outputs": outputs,
			"result":  StringType{},
		})

		// Do not collect outputs type from parent of parent recursively. (#151)
	}
}

func (rule *RuleExpression) checkMatrixExpression(expr *String) *ObjectType {
	ty := rule.checkObjectExpression(expr, "matrix", "jobs.<job_id>.strategy")
	if ty == nil {
		return NewEmptyObjectType()
	}
	matTy, ok := ty.(*ObjectType)
	if !ok {
		return NewEmptyObjectType()
	}

	// Consider properties in include section elements since 'include' section adds matrix values
	incTy, ok := matTy.Props["include"]
	if ok {
		delete(matTy.Props, "include")
		if a, ok := incTy.(*ArrayType); ok {
			if o, ok := a.Elem.(*ObjectType); ok {
				for n, p := range o.Props {
					t, ok := matTy.Props[n]
					if !ok {
						matTy.Props[n] = p
						continue
					}
					matTy.Props[n] = t.Merge(p)
				}
			}
		}
	}

	delete(matTy.Props, "exclude")

	return matTy
}

func (rule *RuleExpression) checkMatrix(m *Matrix) *ObjectType {
	if m.Expression != nil {
		return rule.checkMatrixExpression(m.Expression)
	}

	// Check types of "exclude" but they are not used to guess type of matrix
	if m.Exclude != nil {
		if m.Exclude.Expression != nil {
			if ty, ok := rule.checkArrayExpression(m.Exclude.Expression, "exclude", "jobs.<job_id>.strategy").(*ArrayType); ok {
				rule.checkObjectTy(ty.Elem, m.Exclude.Expression.Pos, "exclude")
			}
		} else {
			for _, combi := range m.Exclude.Combinations {
				if combi.Expression != nil {
					rule.checkObjectExpression(combi.Expression, "exclude", "jobs.<job_id>.strategy")
					continue
				}
				for _, a := range combi.Assigns {
					rule.checkRawYAMLValue(a.Value)
				}
			}
		}
	}

	o := NewEmptyStrictObjectType()

	for n, r := range m.Rows {
		o.Props[n] = rule.checkMatrixRow(r)
	}

	if m.Include == nil {
		return o
	}

	if m.Include.Expression != nil {
		if a, ok := rule.checkOneExpression(m.Include.Expression, "include", "jobs.<job_id>.strategy").(*ArrayType); ok {
			if ret, ok := o.Merge(a.Elem).(*ObjectType); ok {
				return ret
			}
		}
		return NewEmptyObjectType()
	}

	for _, combi := range m.Include.Combinations {
		if combi.Expression != nil {
			ty := rule.checkOneExpression(m.Include.Expression, "matrix combination at element of include section", "jobs.<job_id>.strategy")
			if ty == nil {
				continue
			}
			if merged, ok := o.Merge(ty).(*ObjectType); ok {
				o = merged
			} else {
				o.Loose()
			}
			continue
		}

		for n, assign := range combi.Assigns {
			ty := rule.checkRawYAMLValue(assign.Value)
			if t, ok := o.Props[n]; ok {
				// When the combination exists in 'matrix' section, merge type with existing one
				ty = t.Merge(ty)
			}
			o.Props[n] = ty
		}
	}

	return o
}

func (rule *RuleExpression) checkMatrixRow(r *MatrixRow) ExprType {
	if r.Expression != nil {
		if a, ok := rule.checkArrayExpression(r.Expression, "matrix row", "jobs.<job_id>.strategy").(*ArrayType); ok {
			return a.Elem
		}
		return AnyType{}
	}

	var ty ExprType
	for _, v := range r.Values {
		t := rule.checkRawYAMLValue(v)
		if ty == nil {
			ty = t
		} else {
			ty = ty.Merge(t)
		}
	}

	if ty == nil {
		return AnyType{} // No element
	}

	return ty
}

func (rule *RuleExpression) checkWorkflowCallOutputs(outputs map[string]*WorkflowCallEventOutput, jobs map[string]*Job) {
	if len(outputs) == 0 || len(jobs) == 0 {
		return
	}

	props := make(map[string]ExprType, len(jobs))
	for n, j := range jobs {
		var o *ObjectType
		if j.WorkflowCall != nil {
			// Outputs are not defined in jobs.<job_id> section when it is reusable workflow call.
			o = NewEmptyObjectType()
		} else {
			p := make(map[string]ExprType, len(j.Outputs))
			for n := range j.Outputs {
				p[n] = StringType{}
			}
			o = NewStrictObjectType(p)
		}
		props[n] = NewStrictObjectType(map[string]ExprType{
			"outputs": o,
			"result":  StringType{},
		})
	}
	rule.jobsTy = NewStrictObjectType(props)

	for _, o := range outputs {
		rule.checkString(o.Value, "on.workflow_call.outputs.<output_id>.value")
	}
}

func (rule *RuleExpression) checkRawYAMLValue(v RawYAMLValue) ExprType {
	switch v := v.(type) {
	case *RawYAMLObject:
		m := make(map[string]ExprType, len(v.Props))
		for k, p := range v.Props {
			m[k] = rule.checkRawYAMLValue(p)
		}
		return NewStrictObjectType(m)
	case *RawYAMLArray:
		if len(v.Elems) == 0 {
			return &ArrayType{AnyType{}, false}
		}
		elem := rule.checkRawYAMLValue(v.Elems[0])
		for _, v := range v.Elems[1:] {
			elem = elem.Merge(rule.checkRawYAMLValue(v))
		}
		return &ArrayType{elem, false}
	case *RawYAMLString:
		return rule.checkRawYAMLString(v)
	default:
		panic("unreachable")
	}
}

func (rule *RuleExpression) checkRawYAMLString(y *RawYAMLString) ExprType {
	ts, ok := rule.checkExprsIn(&String{Value: y.Value, Pos: y.Pos()}, false, "jobs.<job_id>.strategy")

	if isExprAssigned(y.Value) {
		if !ok || len(ts) != 1 {
			return AnyType{}
		}
		return ts[0].ty
	}

	// A scalar tagged as `!!str` is a string even when it looks like a number or a boolean. (#250)
	if y.StringTag {
		return StringType{}
	}

	s := strings.TrimSpace(y.Value)
	// Note that keywords are case sensitive. TRUE, FALSE, NULL are invalid named value.
	if s == "true" || s == "false" {
		return BoolType{}
	}
	if s == "null" {
		return NullType{}
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return NumberType{}
	}
	return StringType{}
}

func convertExprLineColToPos(line, col, lineBase, colBase int) *Pos {
	// Line and column in ExprError are 1-based
	return &Pos{
		Line: line - 1 + lineBase,
		Col:  col - 1 + colBase,
	}
}

func typeOfActionOutputs(meta *ActionMetadata) *ObjectType {
	// Some action sets outputs dynamically. Such outputs are not defined in action.yml. jactionlint
	// cannot check such outputs statically so it allows any props (#18)
	if meta.SkipOutputs {
		return NewEmptyObjectType()
	}
	props := make(map[string]ExprType, len(meta.Outputs))
	for n := range meta.Outputs {
		props[strings.ToLower(n)] = StringType{}
	}
	return NewStrictObjectType(props)
}

func init() {
	registerRules(
		RuleInfo{ID: "expression-type", Group: RuleGroupCorrectness, Summary: "A ${{ }} expression has a type error.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-type-check-expression"},
		RuleInfo{ID: "unsound-ternary", Group: RuleGroupCorrectness, Summary: "The a && b || c idiom has a falsy b so it always evaluates to c.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-falsy-ternary"},
		RuleInfo{ID: "workflow-input-type", Group: RuleGroupCorrectness, Summary: "The type of a value passed to a reusable workflow does not match its input.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-reusable-workflows"},
	)
	registerRuleFactory("expression", func(env *RuleEnv) []Rule {
		r := NewRuleExpression(env.localActions, env.localReusableWorkflows)
		if env.src != nil {
			r.src = newSourceIndex(env.src)
		}
		return []Rule{r, NewRuleTemplateInjection(env.src)}
	})
}

// compositeContexts narrows the contexts available to a step of a workflow job to those available in a
// composite action. GitHub documents only one difference: the secrets context is not available, so
// a secret must be passed as an input. The other contexts are left alone because the runner does not
// document whether it passes them on.
// https://docs.github.com/en/actions/reference/workflows-and-actions/contexts
func compositeContexts(ctx []string) []string {
	return slices.DeleteFunc(slices.Clone(ctx), func(c string) bool { return c == "secrets" })
}

// visitActionPre checks the parts of an action.yml which are not steps: the defaults of the inputs. It
// also sets the type of the "inputs" context for the steps: every input of an action is a string.
func (rule *RuleExpression) visitActionPre(a *ActionFile) {
	// The default of an input is evaluated before the inputs exist
	rule.inputsTy = NewEmptyStrictObjectType()
	for _, in := range a.Inputs {
		rule.checkString(in.Default, "jobs.<job_id>.steps.run")
	}

	ity := NewEmptyStrictObjectType()
	for _, in := range a.Inputs {
		ity.Props[strings.ToLower(in.ID.Value)] = StringType{}
	}
	rule.inputsTy = ity
}

// compositeExprErrors tells what to do instead when a composite action reads the secrets context.
func compositeExprErrors(errs []*ExprError) []*ExprError {
	ret := make([]*ExprError, 0, len(errs))
	for _, e := range errs {
		if rest, ok := strings.CutPrefix(e.Message, `context "secrets" is not allowed here. `); ok && e.ID == "context-availability" {
			c := *e
			c.Message = `context "secrets" is not allowed in a composite action because secrets are not passed to it. declare an input and let the workflow pass the secret with "with:". ` + rest
			e = &c
		}
		ret = append(ret, e)
	}
	return ret
}

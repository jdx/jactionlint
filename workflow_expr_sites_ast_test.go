package jactionlint

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// astImplementations are the implementations of the interfaces which the AST holds. The coverage test
// builds one workflow per index, so every implementation is populated.
var astImplementations = map[reflect.Type][]reflect.Type{
	reflect.TypeOf((*Event)(nil)).Elem(): {
		reflect.TypeOf(&WebhookEvent{}), reflect.TypeOf(&ScheduledEvent{}), reflect.TypeOf(&WorkflowDispatchEvent{}),
		reflect.TypeOf(&RepositoryDispatchEvent{}), reflect.TypeOf(&WorkflowCallEvent{}), reflect.TypeOf(&ImageVersionEvent{}),
	},
	reflect.TypeOf((*Exec)(nil)).Elem(): {
		reflect.TypeOf(&ExecRun{}), reflect.TypeOf(&ExecAction{}), reflect.TypeOf(&ExecWait{}),
		reflect.TypeOf(&ExecCancel{}), reflect.TypeOf(&ExecParallel{}),
	},
}

// notExpressions are the fields of the AST which hold a string that is never an expression. The test
// fails for every other field workflowExprSites does not emit.
var notExpressions = map[string]string{
	"DispatchInput.Required.Expression":           "the declaration of an input",
	"WorkflowCallEventInput.Required.Expression":  "the declaration of an input",
	"WorkflowCallEventSecret.Required.Expression": "the declaration of a secret",
	"DispatchInput.Default":                       "the declaration of an input of workflow_dispatch",
	"DispatchInput.Description":                   "the declaration of an input",
	"DispatchInput.Name":                          "the declaration of an input",
	"DispatchInput.Options":                       "the declaration of an input",
	"EnvVar.Name":                                 "the key of a variable",
	"ExecCancel.Name":                             "the id of a background step",
	"ExecWait.Names":                              "ids of background steps",
	"ImageVersionEvent.Names":                     "names of an image event",
	"ImageVersionEvent.Versions":                  "versions of an image event",
	"Input.Name":                                  "the key of an input",
	"Job.ID":                                      "the key of the job",
	"Job.Needs":                                   "ids of jobs",
	"MatrixAssign.Key":                            "the key of a matrix value",
	"MatrixRow.Name":                              "the key of a matrix row",
	"Output.Name":                                 "the key of an output",
	"RepositoryDispatchEvent.Types":               "the types of an event",
	"ScheduleEntry.Cron":                          "a cron schedule",
	"ScheduleEntry.Timezone":                      "a time zone of a schedule",
	"Service.Name":                                "the key of a service",
	"Step.ID":                                     "the id of a step",
	"WebhookEvent.Hook":                           "the name of an event",
	"WebhookEvent.Types":                          "the activity types of an event",
	"WebhookEvent.Workflows":                      "names of the workflows of workflow_run",
	"WebhookEventFilter.Name":                     "the name of a filter",
	"WebhookEventFilter.Values":                   "the patterns of a filter",
	"WorkflowCallEventInput.Description":          "the declaration of an input",
	"WorkflowCallEventInput.Name":                 "the declaration of an input",
	"WorkflowCallEventOutput.Description":         "the declaration of an output",
	"WorkflowCallEventOutput.Name":                "the declaration of an output",
	"WorkflowCallEventSecret.Description":         "the declaration of a secret",
	"WorkflowCallEventSecret.Name":                "the declaration of a secret",
	"WorkflowCallInput.Name":                      "the key of an input",
	"WorkflowCallSecret.Name":                     "the key of a secret",
}

// skippedASTFields are the fields whose expressions are not visited through workflowExprSites on purpose.
var skippedASTFields = map[string]bool{
	// the metadata of an action (action.yml) is visited by the expression rule's visitActionPre; the steps
	// of a composite action are also in Jobs
	"Workflow.Action": true,
}

type astFields struct {
	values map[string]string // the marker of a string to the field which holds it
}

// populate fills v with strings, each of which carries its own marker.
func (a *astFields) populate(v reflect.Value, field string, variant int, stack map[reflect.Type]bool) {
	t := v.Type()
	switch {
	case t == reflect.TypeOf((*String)(nil)):
		a.add(v, field)
	case t == reflect.TypeOf((*Pos)(nil)):
	case t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct:
		if stack[t.Elem()] {
			return
		}
		stack[t.Elem()] = true
		defer delete(stack, t.Elem())
		n := reflect.New(t.Elem())
		v.Set(n)
		name := t.Elem().Name()
		if t == reflect.TypeOf((*Bool)(nil)) || t == reflect.TypeOf((*Int)(nil)) || t == reflect.TypeOf((*Float)(nil)) {
			name = field // the expression of "continue-on-error:" is named by the field which holds it
		}
		for i := 0; i < t.Elem().NumField(); i++ {
			f := t.Elem().Field(i)
			if !f.IsExported() {
				continue
			}
			if skippedASTFields[name+"."+f.Name] {
				continue
			}
			a.populate(n.Elem().Field(i), name+"."+f.Name, variant, stack)
		}
	case t.Kind() == reflect.Slice:
		e := reflect.New(t.Elem()).Elem()
		a.populate(e, field, variant, stack)
		if !isNilValue(e) {
			v.Set(reflect.Append(reflect.MakeSlice(t, 0, 1), e))
		}
	case t.Kind() == reflect.Map && t.Key().Kind() == reflect.String:
		e := reflect.New(t.Elem()).Elem()
		a.populate(e, field, variant, stack)
		if !isNilValue(e) {
			m := reflect.MakeMap(t)
			m.SetMapIndex(reflect.ValueOf("k"), e)
			v.Set(m)
		}
	case t == reflect.TypeOf((*RawYAMLValue)(nil)).Elem():
		marker := a.mark(field)
		v.Set(reflect.ValueOf(&RawYAMLString{Value: marker, pos: &Pos{Line: 1, Col: 1}}))
	case t.Kind() == reflect.Interface:
		impls := astImplementations[t]
		if len(impls) == 0 {
			return
		}
		e := reflect.New(impls[variant%len(impls)]).Elem()
		a.populate(e, field, variant, stack)
		v.Set(e)
	}
}

func (a *astFields) mark(field string) string {
	m := fmt.Sprintf("${{ M%d }}", len(a.values))
	a.values[m] = field
	return m
}

func (a *astFields) add(v reflect.Value, field string) {
	v.Set(reflect.ValueOf(&String{Value: a.mark(field), Pos: &Pos{Line: 1, Col: 1}}))
}

func isNilValue(v reflect.Value) bool {
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) && v.IsNil()
}

// Every string of the AST which can hold an expression must be visited by workflowExprSites, because
// the rules which look at the place of an expression (sensitiveContexts, secrets-outside-env, ...) are
// blind to what it does not emit. Adding a field to ast.go without emitting it fails here.
func TestWorkflowExprSitesEmitEveryASTString(t *testing.T) {
	missing := map[string]bool{}
	for variant := 0; variant < 6; variant++ {
		a := &astFields{values: map[string]string{}}
		var w *Workflow
		a.populate(reflect.ValueOf(&w).Elem(), "Workflow", variant, map[reflect.Type]bool{})
		seen := map[string]bool{}
		workflowExprSites(w, func(site exprSite) { seen[site.Str.Value] = true })
		for marker, field := range a.values {
			if !seen[marker] && notExpressions[field] == "" {
				missing[field] = true
			}
		}
	}
	var fields []string
	for f := range missing {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	if len(fields) > 0 {
		t.Errorf("workflowExprSites does not emit these fields of the AST (emit them, or list them in notExpressions with the reason): %s", strings.Join(fields, ", "))
	}
}

// availabilityFields maps each key of the table of context availability (availability.go) to a field of
// the AST which holds the key. A key of the table accepts expressions, so its field must be emitted and
// must not be explained away in notExpressions.
var availabilityFields = map[string]string{
	"concurrency":                                      "Concurrency.Group",
	"env":                                              "EnvVar.Value",
	"jobs.<job_id>.concurrency":                        "Concurrency.Group",
	"jobs.<job_id>.container":                          "Container.Image",
	"jobs.<job_id>.container.credentials":              "Credentials.Username",
	"jobs.<job_id>.container.env.<env_id>":             "EnvVar.Value",
	"jobs.<job_id>.container.image":                    "Container.Image",
	"jobs.<job_id>.continue-on-error":                  "Job.ContinueOnError.Expression",
	"jobs.<job_id>.defaults.run":                       "DefaultsRun.Shell",
	"jobs.<job_id>.env":                                "EnvVar.Value",
	"jobs.<job_id>.environment":                        "Environment.Name",
	"jobs.<job_id>.environment.url":                    "Environment.URL",
	"jobs.<job_id>.if":                                 "Job.If",
	"jobs.<job_id>.name":                               "Job.Name",
	"jobs.<job_id>.outputs.<output_id>":                "Output.Value",
	"jobs.<job_id>.runs-on":                            "Runner.LabelsExpr",
	"jobs.<job_id>.secrets.<secrets_id>":               "WorkflowCallSecret.Value",
	"jobs.<job_id>.services":                           "Services.Expression",
	"jobs.<job_id>.services.<service_id>.credentials":  "Credentials.Password",
	"jobs.<job_id>.services.<service_id>.env.<env_id>": "EnvVar.Value",
	"jobs.<job_id>.snapshot.if":                        "Snapshot.If",
	"jobs.<job_id>.steps.continue-on-error":            "Step.ContinueOnError.Expression",
	"jobs.<job_id>.steps.env":                          "EnvVar.Value",
	"jobs.<job_id>.steps.if":                           "Step.If",
	"jobs.<job_id>.steps.name":                         "Step.Name",
	"jobs.<job_id>.steps.run":                          "ExecRun.Run",
	"jobs.<job_id>.steps.timeout-minutes":              "Step.TimeoutMinutes.Expression",
	"jobs.<job_id>.steps.with":                         "Input.Value",
	"jobs.<job_id>.steps.working-directory":            "ExecRun.WorkingDirectory",
	"jobs.<job_id>.strategy":                           "Strategy.FailFast.Expression",
	"jobs.<job_id>.timeout-minutes":                    "Job.TimeoutMinutes.Expression",
	"jobs.<job_id>.with.<with_id>":                     "WorkflowCallInput.Value",
	"on.workflow_call.inputs.<inputs_id>.default":      "WorkflowCallEventInput.Default",
	"on.workflow_call.outputs.<output_id>.value":       "WorkflowCallEventOutput.Value",
	"run-name": "Workflow.RunName",
}

// notExpressions and the table of context availability must not disagree: every key of availability.go is
// in availabilityFields, and none of their fields is declared to never hold an expression.
func TestNotExpressionsAgreeWithAvailability(t *testing.T) {
	src, err := os.ReadFile("availability.go")
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(string(src[strings.Index(string(src), "switch key"):]), -1) {
		if !strings.HasPrefix(m[1], "hashfiles") && strings.ToLower(m[1]) == m[1] && !isContextName(m[1]) {
			keys[m[1]] = true
		}
	}
	for k := range keys {
		field, ok := availabilityFields[k]
		if !ok {
			t.Errorf("availability.go has the key %q, which availabilityFields does not map to a field of the AST", k)
			continue
		}
		if why := notExpressions[field]; why != "" {
			t.Errorf("%s holds %q, which accepts expressions, but notExpressions says: %s", field, k, why)
		}
	}
	for k := range availabilityFields {
		if ctx, _ := WorkflowKeyAvailability(k); ctx == nil {
			t.Errorf("availabilityFields maps %q, which availability.go does not know", k)
		}
	}
}

// isContextName reports whether a string literal of availability.go names a context or a function
// instead of a workflow key.
func isContextName(s string) bool {
	return !strings.ContainsAny(s, ".<") && s != "concurrency" && s != "env" && s != "run-name"
}

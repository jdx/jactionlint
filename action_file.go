package jactionlint

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v4"
)

// actionSyntaxID is the ID of the syntax errors of action.yml files.
const actionSyntaxID = "action-syntax" // also written as a literal where it is reported, see TestRuleIDsInSourceMatchRegistry

// ActionFile is the syntax tree of the metadata file of an action (action.yml or action.yaml). It is
// not the same as ActionMetadata, which only has what is needed to check the inputs of a step which
// uses the action. ActionFile keeps positions and the steps of a composite action so that the rules
// which check steps can check them too.
//
// ParseAction returns a Workflow with ActionFile in its Action field. The steps of a composite action
// are in one synthetic job of the workflow (see Job.Composite), so the rules which visit steps visit
// them like the steps of a workflow job. Actions of other kinds ("node" and "docker") have no job.
//
// https://docs.github.com/en/actions/reference/workflows-and-actions/metadata-syntax
type ActionFile struct {
	// Name is the "name" of the action. It can be nil.
	Name *String
	// Author is the "author" of the action. It can be nil.
	Author *String
	// Description is the "description" of the action. It can be nil.
	Description *String
	// Inputs are the "inputs" of the action in the order of the source. Their IDs are case-insensitive.
	Inputs []*ActionFileInput
	// Outputs are the "outputs" of the action in the order of the source.
	Outputs []*ActionFileOutput
	// Runs is the "runs" section. It is nil only when the section is missing.
	Runs *ActionFileRuns
	// Pos is the position of the file.
	Pos *Pos
	// Callers tells which local workflows run the action. The linter sets it when it knows the
	// repository; it is nil when it does not (for example when linting standard input) and then
	// the rules which depend on the caller assume nothing about it. See ActionCallers.
	Callers *ActionCallers
}

// ActionFileInput is one input in the "inputs" section of an action.
type ActionFileInput struct {
	// ID is the name of the input.
	ID *String
	// Description is the "description" of the input.
	Description *String
	// Required is the "required" of the input.
	Required *Bool
	// Default is the "default" of the input. It is nil when the input has no default.
	Default *String
	// DeprecationMessage is the "deprecationMessage" of the input.
	DeprecationMessage *String
}

// ActionFileOutput is one output in the "outputs" section of an action.
type ActionFileOutput struct {
	// ID is the name of the output.
	ID *String
	// Description is the "description" of the output.
	Description *String
	// Value is the "value" of the output of a composite action. It is nil for other actions.
	Value *String
}

// ActionFileRuns is the "runs" section of an action.
type ActionFileRuns struct {
	// Using is the "using" key: "composite", "docker" or "node20" and the like.
	Using *String
	// Pos is the position of the "runs" key.
	Pos *Pos
	// Steps are the "steps" of a composite action.
	Steps []*Step
	// Main is the "main" script of a JavaScript action.
	Main *String
	// Image is the "image" of a Docker action: "Dockerfile" or "docker://image:tag".
	Image *String
}

// IsComposite reports whether the action runs steps like a job: "runs.using" is "composite".
func (r *ActionFileRuns) IsComposite() bool {
	return r != nil && r.Using != nil && strings.EqualFold(r.Using.Value, "composite")
}

// nodeUsingRegex matches the runtimes of JavaScript actions. Versions are not enumerated because
// GitHub adds a new one every few years.
var nodeUsingRegex = regexp.MustCompile(`^node[0-9]+$`)

// compositeJobID is the ID of the synthetic job which holds the steps of a composite action.
const compositeJobID = "composite-action"

// ParseAction parses the content of an action metadata file (action.yml). It returns all errors
// detected while parsing the input. Like Parse, detecting one error does not stop parsing. The
// returned workflow is nil only when the input is not valid YAML.
func ParseAction(b []byte) (*Workflow, []*Error) {
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, handleYAMLUnmarshalError(err)
	}

	p := &parser{lines: strings.Split(string(b), "\n"), syntaxID: actionSyntaxID}
	w := p.parseAction(&n)
	w.Comments = NewCommentIndex(b)
	w.Source = b
	return w, p.errors
}

func (p *parser) parseAction(n *yaml.Node) *Workflow {
	p.resolveAliases(n)

	a := &ActionFile{Pos: &Pos{1, 1}}
	w := &Workflow{Action: a, Jobs: map[string]*Job{}}

	if n.Line == 0 {
		n.Line = 1
	}
	if n.Column == 0 {
		n.Column = 1
	}
	if len(n.Content) == 0 {
		p.errorID("action-syntax", n, "action metadata is empty")
		return w
	}

	for e := range p.parseSectionMapping("action", n.Content[0], false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "name":
			a.Name = p.parseString(v, true)
		case "author":
			a.Author = p.parseString(v, true)
		case "description":
			a.Description = p.parseString(v, true)
		case "inputs":
			a.Inputs = p.parseActionInputs(v)
		case "outputs":
			a.Outputs = p.parseActionOutputs(v)
		case "runs":
			a.Runs = p.parseActionRuns(k.Pos, v)
		case "branding":
			// Only decorates the Marketplace page.
		default:
			p.unexpectedKey(k, "action", []string{"name", "author", "description", "inputs", "outputs", "runs", "branding"})
		}
	}

	if a.Runs == nil {
		p.error(n.Content[0], "\"runs\" section is missing in action metadata")
		return w
	}

	if a.Runs.IsComposite() && a.Runs.Steps != nil {
		w.Jobs[compositeJobID] = &Job{
			ID:        &String{Value: compositeJobID, Pos: a.Runs.Pos},
			Steps:     a.Runs.Steps,
			Pos:       a.Runs.Pos,
			Composite: true,
		}
	}
	return w
}

func (p *parser) parseActionInputs(n *yaml.Node) []*ActionFileInput {
	var ret []*ActionFileInput
	for e := range p.parseSectionMapping("inputs", n, false, false) {
		in := &ActionFileInput{ID: e.key}
		ret = append(ret, in)
		for f := range p.parseMappingAt(fmt.Sprintf("input %q", e.key.Value), e.val, true, true) {
			switch f.id {
			case "description":
				in.Description = p.parseString(f.val, true)
			case "required":
				in.Required = p.parseBool(f.val)
			case "default":
				in.Default = p.parseString(f.val, true)
			case "deprecationMessage":
				in.DeprecationMessage = p.parseString(f.val, true)
			case "type", "options":
				// Copied from workflow_dispatch, where inputs are typed. GitHub ignores them for actions.
				p.errorfAt(f.key.Pos, "%q has no effect on the input %q of an action, whose inputs are always strings. remove it or document the accepted values in \"description\"", f.key.Value, e.key.Value)
			default:
				p.unexpectedKey(f.key, fmt.Sprintf("input %q", e.key.Value), []string{"description", "required", "default", "deprecationMessage"})
			}
		}
	}
	return ret
}

func (p *parser) parseActionOutputs(n *yaml.Node) []*ActionFileOutput {
	var ret []*ActionFileOutput
	for e := range p.parseSectionMapping("outputs", n, false, false) {
		out := &ActionFileOutput{ID: e.key}
		ret = append(ret, out)
		for f := range p.parseMappingAt(fmt.Sprintf("output %q", e.key.Value), e.val, true, true) {
			switch f.id {
			case "description":
				out.Description = p.parseString(f.val, true)
			case "value":
				out.Value = p.parseString(f.val, false)
			default:
				p.unexpectedKey(f.key, fmt.Sprintf("output %q", e.key.Value), []string{"description", "value"})
			}
		}
	}
	return ret
}

func (p *parser) parseActionRuns(pos *Pos, n *yaml.Node) *ActionFileRuns {
	r := &ActionFileRuns{Pos: pos}
	var stepsKey *String
	var otherKeys []*String
	for e := range p.parseSectionMapping("runs", n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "using":
			r.Using = p.parseString(v, false)
		case "steps":
			stepsKey = k
			r.Steps = p.parseSteps("steps", v)
		case "main":
			otherKeys = append(otherKeys, k)
			r.Main = p.parseString(v, false)
		case "image":
			otherKeys = append(otherKeys, k)
			r.Image = p.parseString(v, false)
		case "pre", "post", "pre-if", "post-if", "args", "entrypoint", "pre-entrypoint", "post-entrypoint", "env":
			otherKeys = append(otherKeys, k)
		default:
			p.unexpectedKey(k, "runs", []string{
				"using", "steps", "main", "pre", "pre-if", "post", "post-if", "image", "args", "entrypoint",
				"pre-entrypoint", "post-entrypoint", "env",
			})
		}
	}

	if r.Using == nil {
		p.error(n, "\"using\" is missing in \"runs\" section. it must be \"composite\", \"docker\" or a Node.js runtime such as \"node24\"")
		return r
	}
	if r.Using.Value == "" {
		return r // reported as an empty string
	}
	using := strings.ToLower(r.Using.Value)
	switch {
	case using == "composite":
		if stepsKey == nil {
			p.errorAt(r.Using.Pos, "\"steps\" is missing in \"runs\" section of the composite action")
		}
		for _, k := range otherKeys {
			p.errorfAt(k.Pos, "%q is not available in \"runs\" section of the composite action. it is for JavaScript and Docker actions", k.Value)
		}
		p.checkCompositeShells(r.Steps)
	case using == "docker":
		if r.Image == nil {
			p.errorAt(r.Using.Pos, "\"image\" is missing in \"runs\" section of the Docker action")
		}
		if stepsKey != nil {
			p.errorAt(stepsKey.Pos, "\"steps\" is only available in the composite action but \"using\" is \"docker\"")
		}
	case nodeUsingRegex.MatchString(using):
		if r.Main == nil {
			p.errorfAt(r.Using.Pos, "\"main\" is missing in \"runs\" section of the %s action", using)
		}
		if stepsKey != nil {
			p.errorfAt(stepsKey.Pos, "\"steps\" is only available in the composite action but \"using\" is %q", r.Using.Value)
		}
	default:
		p.errorfAt(r.Using.Pos, "\"using\" %q is invalid. it must be \"composite\", \"docker\" or a Node.js runtime such as \"node24\"", r.Using.Value)
	}
	return r
}

// checkCompositeShells reports the "run" steps of a composite action which have no "shell". Unlike a
// workflow job, a composite action has no default shell, so GitHub rejects the action.
func (p *parser) checkCompositeShells(steps []*Step) {
	walkSteps(steps, func(s *Step) {
		run, ok := s.Exec.(*ExecRun)
		if !ok || run.Shell != nil {
			return
		}
		pos := run.RunPos
		if pos == nil {
			pos = s.Pos
		}
		p.errorAt(pos, "\"shell\" is required for a \"run\" step of a composite action, which has no default shell. for example, add \"shell: bash\"")
	})
}

// IsActionPath reports whether the path is the metadata file of an action: a file named action.yml or
// action.yaml that is not under ".github/workflows" (where every YAML file is a workflow).
func IsActionPath(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	switch path.Base(p) {
	case "action.yml", "action.yaml":
	default:
		return false
	}
	return !isWorkflowsDir(path.Dir(p))
}

// isWorkflowsDir reports whether the slash-separated directory is ".github/workflows" or in it.
func isWorkflowsDir(dir string) bool {
	parts := strings.Split(dir, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == ".github" && parts[i+1] == "workflows" {
			return true
		}
	}
	return false
}

// actionFileNames are the names GitHub reads the metadata of an action from.
var actionFileNames = []string{"action.yml", "action.yaml"}

// IsComposite reports whether the workflow is the metadata of a composite action.
func (w *Workflow) IsComposite() bool {
	return w != nil && w.Action != nil && w.Action.Runs.IsComposite()
}

func init() {
	registerRules(
		RuleInfo{ID: actionSyntaxID, Group: RuleGroupCorrectness, Summary: "The action metadata does not follow the syntax of action.yml.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-composite-action-syntax"},
	)
}

package jactionlint

import (
	"fmt"
	"io"
)

// RuleBase is a struct to be a base of rule structs. Embed this struct to define default methods
// automatically
type RuleBase struct {
	name   string
	desc   string
	errs   []*Error
	dbg    io.Writer
	config *Config
}

// NewRuleBase creates a new RuleBase instance. It should be embedded to your own
// rule instance.
func NewRuleBase(name string, desc string) RuleBase {
	return RuleBase{
		name: name,
		desc: desc,
	}
}

// VisitStep is callback when visiting Step node.
func (r *RuleBase) VisitStep(node *Step) error { return nil }

// VisitJobPre is callback when visiting Job node before visiting its children.
func (r *RuleBase) VisitJobPre(node *Job) error { return nil }

// VisitJobPost is callback when visiting Job node after visiting its children.
func (r *RuleBase) VisitJobPost(node *Job) error { return nil }

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (r *RuleBase) VisitWorkflowPre(node *Workflow) error { return nil }

// VisitWorkflowPost is callback when visiting Workflow node after visiting its children.
func (r *RuleBase) VisitWorkflowPost(node *Workflow) error { return nil }

// Error creates a new error from the source position and the error message and stores it in the
// rule instance. The errors can be accessed by Errs method. The ID of the error is the name of the
// rule. Rules which should be configurable per diagnostic report with ReportID instead.
func (r *RuleBase) Error(pos *Pos, msg string) {
	r.ReportID(r.name, pos, msg)
}

// Errorf reports a new error with the source position and the formatted error message and stores it
// in the rule instance. The errors can be accessed by Errs method. The ID of the error is the name
// of the rule.
func (r *RuleBase) Errorf(pos *Pos, format string, args ...interface{}) {
	r.ReportIDf(r.name, pos, format, args...)
}

// ReportID reports a new error with the stable diagnostic ID, the source position and the error
// message and stores it in the rule instance. One rule can report several IDs. IDs of the
// built-in rules are listed in Rules. The errors can be accessed by Errs method.
func (r *RuleBase) ReportID(id string, pos *Pos, msg string) {
	r.errs = append(r.errs, errorAt(pos, r.name, id, msg))
}

// ReportIDf is like ReportID but takes a format string and its arguments.
func (r *RuleBase) ReportIDf(id string, pos *Pos, format string, args ...interface{}) {
	r.errs = append(r.errs, errorfAt(pos, r.name, id, format, args...))
}

// ReportRange is like ReportID but also tells where the problematic region ends. The end position
// is exclusive: it is the position just after the last character of the region.
func (r *RuleBase) ReportRange(id string, pos *Pos, end *Pos, msg string) {
	err := errorAt(pos, r.name, id, msg)
	err.EndLine = end.Line
	err.EndColumn = end.Col
	r.errs = append(r.errs, err)
}

// Debug prints debug log to the output. The output is specified by the argument of EnableDebug method.
// By default, no output is set so debug log is not printed.
func (r *RuleBase) Debug(format string, args ...interface{}) {
	if r.dbg == nil {
		return
	}
	format = fmt.Sprintf("[%s] %s\n", r.name, format)
	fmt.Fprintf(r.dbg, format, args...)
}

// Errs returns errors found by the rule.
func (r *RuleBase) Errs() []*Error {
	return r.errs
}

// Name returns the name of the rule.
func (r *RuleBase) Name() string {
	return r.name
}

// Description returns the description of the rule.
func (r *RuleBase) Description() string {
	return r.desc
}

// EnableDebug enables debug output from the rule. Given io.Writer instance is used to print debug
// information to console. Setting nil means disabling debug output.
func (r *RuleBase) EnableDebug(out io.Writer) {
	r.dbg = out
}

// SetConfig populates user configuration of jactionlint to the rule. When no config is set, rules
// should behave as if the default configuration is set.
func (r *RuleBase) SetConfig(cfg *Config) {
	r.config = cfg
}

// Config returns the user configuration of jactionlint. When no config was set to this rule by SetConfig,
// this method returns nil.
func (r *RuleBase) Config() *Config {
	return r.config
}

// Rule is an interface which all rule structs must meet.
type Rule interface {
	Pass
	Errs() []*Error
	Name() string
	Description() string
	EnableDebug(out io.Writer)
	SetConfig(cfg *Config)
	Config() *Config
}

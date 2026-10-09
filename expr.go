package jactionlint

import "fmt"

// ExprError is an error type caused by lexing/parsing expression syntax. For more details, see
// https://docs.github.com/en/actions/learn-github-actions/expressions
type ExprError struct {
	// Message is an error message
	Message string
	// Offset is byte offset position which caused the error
	Offset int
	// Offset is line number position which caused the error. Note that this value is 1-based.
	Line int
	// Column is column number position which caused the error. Note that this value is 1-based.
	Column int
	// ID is the stable ID of the diagnostic such as "expression-syntax" or "undefined-property".
	ID string
}

func (e *ExprError) Error() string {
	return fmt.Sprintf("%d:%d:%d: %s", e.Line, e.Column, e.Offset, e.Message)
}

func (e *ExprError) String() string {
	return e.Error()
}

func init() {
	registerRules(
		RuleInfo{ID: "expression-syntax", Group: RuleGroupCorrectness, Summary: "A ${{ }} expression has a syntax error.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-syntax-expression"},
	)
}

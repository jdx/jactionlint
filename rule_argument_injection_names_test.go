package jactionlint

import "testing"

func TestExprIsName(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"github.head_ref", true},
		{"github.event.pull_request.head.ref", true},
		{"github.event.workflow_run.pull_requests[0].head.ref", true},
		{"github.event.workflow_run.head_branch", true},
		{"github.event.issue.title", false},
		{"github.event.comment.body", false},
		{"github.event.pull_request.title || github.head_ref", false},
		{"github.repository", false},
	} {
		if got := exprIsName(tc.expr); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.expr, got, tc.want)
		}
	}
}

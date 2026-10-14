package jactionlint

import (
	"strings"
	"testing"
)

func TestMissingVersionCommentReasonOfAFailedLookup(t *testing.T) {
	c := &failingClient{err: &GitHubStatusError{Status: 500}}
	errs, _ := lintOnline(t, c, mustParseConfig(t, "rules:\n  missing-version-comment: warning\n"), workflowWith("uses: "+mvcCheckout))
	var found bool
	for _, e := range errs {
		if e.ID != "missing-version-comment" {
			continue
		}
		found = true
		if e.Fix != nil || e.NoFix == nil || e.NoFix.Code != NoFixLookupFailed ||
			!strings.Contains(e.NoFix.Reason, "adding the version") || strings.Contains(e.NoFix.Reason, "pinning") {
			t.Errorf("the reason is about the comment, not about pinning: %+v", e.NoFix)
		}
	}
	if !found {
		t.Error("no finding")
	}
}

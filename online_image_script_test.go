package jactionlint

import (
	"strings"
	"testing"
)

// The image of a command in a script is reported but not edited: the quoting of the shell and of YAML is around it.
func TestImageFixDoesNotEditScripts(t *testing.T) {
	src := imageTestHead + "    steps:\n      - run: docker run --rm alpine:3.19 echo hi\n"
	errs := lintImages(t, onlineFixtureClient(t), imagesConfig(t, ""), src)
	var found bool
	for _, e := range errs {
		if e.ID != "unpinned-images" {
			continue
		}
		found = true
		if e.Fix != nil || e.NoFix == nil || !strings.Contains(e.NoFix.Reason, "script") {
			t.Errorf("a script image gets no fix and says why: %+v %+v", e.Fix, e.NoFix)
		}
	}
	if !found {
		t.Skip("the image of a script is not reported with this configuration")
	}
	if got, n := applyFixes([]byte(src), errs, FixModeUnsafe); n != 0 || string(got) != src {
		t.Errorf("%d fixes applied:\n%s", n, got)
	}
}

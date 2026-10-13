package jactionlint

import "testing"

func TestPinFixCommentKeepsCRLF(t *testing.T) {
	src := "on: push\r\njobs:\r\n  test:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - uses: example/tags@v1\r\n"
	errs, _ := lintOnline(t, tagsClient(t), unpinnedConfig(), src)
	got, n := applyFixes([]byte(src), errs, FixModeSafe)
	want := "on: push\r\njobs:\r\n  test:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - uses: example/tags@" + tagsSHA1 + " # v1.2.3\r\n"
	if n != 1 || string(got) != want {
		t.Errorf("got %d fixes:\n%q\nwant:\n%q", n, got, want)
	}
}

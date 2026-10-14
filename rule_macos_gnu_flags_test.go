package jactionlint

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMacosGnuFlagsOKFixture runs the rule on the workflow of testdata/ok, which the golden test does not do because
// the rule is pedantic.
func TestMacosGnuFlagsOKFixture(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "ok", "macos_gnu_flags.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range lintBatchH(t, pedanticCfg, string(b), "macos-gnu-flags") {
		t.Errorf("unexpected: %s", e)
	}
}

func TestMacosGnuFlagsIsPedantic(t *testing.T) {
	src := "on: push\njobs:\n  a:\n    runs-on: macos-latest\n    steps:\n      - run: sed -i 's/a/b/' f\n"
	if errs := lintBatchH(t, "", src, "macos-gnu-flags"); len(errs) != 0 {
		t.Errorf("the rule is on without the pedantic profile: %v", errs)
	}
	errs := lintBatchH(t, pedanticCfg, src, "macos-gnu-flags")
	if len(errs) != 1 {
		t.Fatalf("want one finding, have %v", errs)
	}
	if errs[0].Severity != SeverityWarning {
		t.Errorf("level %v, want warning", errs[0].Severity)
	}
}

func TestIsMacOSRunnerLabel(t *testing.T) {
	for l, want := range map[string]bool{
		"macos-latest": true, "xcode-27": true, "xcode-27-xlarge": true, "macos-15": true, "macOS-14": true, "macos-15-intel": true, "macos-26-xlarge": true,
		// Labels of runners of the owner, and other systems
		"macOS-ARM64": false, "macos": false, "macos-custom": false, "ubuntu-latest": false, "windows-latest": false, "self-hosted": false,
	} {
		if have := isMacOSRunnerLabel(l); have != want {
			t.Errorf("%q: have %v, want %v", l, have, want)
		}
	}
}

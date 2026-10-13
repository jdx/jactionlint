package jactionlint

import "testing"

func TestMacosLabelOfSeveralLabels(t *testing.T) {
	for runsOn, want := range map[string]string{
		"macos-14":                    "macos-14",
		"[macos-14]":                  "macos-14",
		"[macos-14, ARM64]":           "macos-14",
		"[macos-latest, macOS, X64]":  "macos-latest",
		"[macos-14, my-own-runner]":   "", // a label of GitHub does not give this one: the runner of the owner
		"[macos-14, gpu]":             "",
		"[self-hosted, macos-14]":     "",
		"[self-hosted, macOS, ARM64]": "",
		"[ubuntu-latest, macos-14]":   "", // no runner has both labels
		"ubuntu-latest":               "",
		"[xcode-27, ARM64]":           "xcode-27",
		"[xcode-27, some-pool]":       "",
		// A label that is not known makes the runner unknown
		"[macos-latest, '${{ inputs.extra }}']":     "",
		"[macos-latest, '${{ fromJSON(vars.X) }}']": "",
		"'${{ fromJSON(vars.X) }}'":                 "",
		"'${{ inputs.runner }}'":                    "",
	} {
		w, _ := Parse([]byte("on:\n  workflow_dispatch:\n    inputs:\n      extra:\n        type: string\n      runner:\n        type: string\njobs:\n  a:\n    runs-on: " + runsOn + "\n    steps:\n      - run: echo\n"))
		if w == nil || w.Jobs["a"] == nil {
			t.Fatalf("no job for %q", runsOn)
		}
		if have := macosLabelOf(w.Jobs["a"]); have != want {
			t.Errorf("runs-on %s: have %q, want %q", runsOn, have, want)
		}
	}
}

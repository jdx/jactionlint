package jactionlint

import "testing"

func TestMisfeatureShells(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  misfeature:\n    level: warn\n    pedantic: true\n")
	tests := []struct {
		shell string
		want  string // the ID of the finding, if any
	}{
		{"bash", ""},
		{"sh", ""},
		{"pwsh", ""},
		{"powershell", ""},
		{"python", ""},
		{"BASH", ""},
		{"bash -eo pipefail {0}", ""},
		{"cmd", "misfeature"},
		{"CMD", "misfeature"},
		{"cmd.exe /c {0}", "misfeature"},
		{"perl {0}", "misfeature-custom-shell"},
		{"zsh", "misfeature-custom-shell"},
		{"${{ matrix.shell }}", ""},
		// A path to a shell is the shell
		{"/usr/bin/bash {0}", ""},
		{"/bin/sh -e {0}", ""},
		{"bash.exe", ""},
		{"C:\\Windows\\System32\\cmd.exe /c", "misfeature"},
		{"/usr/bin/cmd", "misfeature"},
		{"C:\\Program\\PWSH.EXE", ""},
		{"./custom-shell", "misfeature-custom-shell"},
		{"/opt/tools/zsh {0}", "misfeature-custom-shell"},
	}
	for _, tc := range tests {
		t.Run(tc.shell, func(t *testing.T) {
			src := "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [a]\n        shell: [b]\n    steps:\n      - run: echo\n        shell: '" + tc.shell + "'\n"
			var got []string
			for _, e := range lintWithConfig(t, cfg, src) {
				if e.ID == "misfeature" {
					got = append(got, findingName(e))
				}
			}
			switch {
			case tc.want == "" && len(got) != 0, tc.want != "" && (len(got) != 1 || got[0] != tc.want):
				t.Errorf("want %q but got %v", tc.want, got)
			}
		})
	}
}

func TestMisfeaturePipInstall(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  misfeature: warn\n")
	for uses, want := range map[string]int{
		"actions/setup-python@v6": 1,
		"Actions/Setup-Python@v6": 1,
		"actions/setup-node@v4":   0,
	} {
		src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n        with:\n          pip-install: x\n"
		n := 0
		for _, e := range lintWithConfig(t, cfg, src) {
			if e.ID == "misfeature" {
				n++
			}
		}
		if n != want {
			t.Errorf("%s: want %d findings but got %d", uses, want, n)
		}
	}
}

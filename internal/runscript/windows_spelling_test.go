package runscript

import "testing"

// A command written the way Windows runners write it is the same tool: the case and the suffix do not matter.
func TestToolSpellingOnWindows(t *testing.T) {
	tests := []struct {
		script, tool string
		global       bool
		eco          string
	}{
		{"npm install -g npm@latest", "npm", true, "npm"},
		{"npm.cmd i -g npm@11", "npm", true, "npm"},
		{"NPM install --global npm", "npm", true, "npm"},
		{"NPM.CMD ci", "npm", false, "npm"},
		{"Npm.Cmd install lodash", "npm", false, "npm"},
		{"Pip.exe install twine", "pip", false, "pypi"},
		{"PIP3.EXE install -r requirements.txt", "pip", false, "pypi"},
		{"yarn.cmd add left-pad", "yarn", false, "npm"},
		{"pnpm.CMD install", "pnpm", false, "npm"},
		{"Python.exe -m pip install requests", "pip", false, "pypi"},
	}
	for _, tc := range tests {
		s, err := Analyze(tc.script, "bash")
		if err != nil || len(s.Commands) != 1 {
			t.Fatalf("%q: %v %v", tc.script, err, s)
		}
		c := s.Commands[0]
		if c.Tool != tc.tool {
			t.Errorf("%q: tool %q, want %q", tc.script, c.Tool, tc.tool)
			continue
		}
		in := c.Installs()
		if in == nil || in.Global != tc.global || in.Ecosystem != tc.eco {
			t.Errorf("%q: install %+v, want global=%v ecosystem=%s", tc.script, in, tc.global, tc.eco)
		}
	}
}

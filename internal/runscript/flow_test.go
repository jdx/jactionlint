package runscript

import "testing"

func TestGuards(t *testing.T) {
	tests := []struct {
		what   string
		script string
		want   int // number of guards
	}{
		{"bare test", `[[ "$v" =~ ^[a-z]+$ ]]`, 1},
		{"bare negated test", `[[ ! "$v" =~ ^[a-z]+$ ]]`, 0},
		{"bare test with set +e", "set +e\n[[ \"$v\" =~ ^[a-z]+$ ]]", 0},
		{"or exit", `[[ "$v" =~ ^[a-z]+$ ]] || exit 1`, 1},
		{"or exit in a group", `[[ "$v" =~ ^[a-z]+$ ]] || { echo bad; exit 1; }`, 1},
		{"or exit in a subshell", `[[ "$v" =~ ^[a-z]+$ ]] || (exit 1)`, 0},
		{"or echo", `[[ "$v" =~ ^[a-z]+$ ]] || echo bad`, 0},
		{"and exit", `[[ "$v" =~ ^[a-z]+$ ]] && exit 1`, 0},
		{"negated and exit", `[[ ! "$v" =~ ^[a-z]+$ ]] && exit 1`, 1},
		{"negated or exit", `[[ ! "$v" =~ ^[a-z]+$ ]] || exit 1`, 0},
		{"if then exit", "if [[ \"$v\" =~ ^[a-z]+$ ]]; then exit 1; fi", 0},
		{"negated if then exit", "if [[ ! \"$v\" =~ ^[a-z]+$ ]]; then exit 1; fi", 1},
		{"if else exit", "if [[ \"$v\" =~ ^[a-z]+$ ]]; then :; else exit 1; fi", 1},
		{"quoted pattern is no regex", `[[ "$v" =~ "^a$" ]] || exit 1`, 0},
		{"equality", `[[ "$v" == "stable" ]] || exit 1`, 1},
		{"inequality", `[[ "$v" != "stable" ]] && exit 1`, 1},
		{"glob equality", `[[ "$v" == st* ]] || exit 1`, 0},
		{"case with default exit", "case \"$v\" in\n a|b) ;;\n *) exit 1 ;;\nesac", 1},
		{"case without default", "case \"$v\" in\n a|b) ;;\nesac", 0},
		{"case with glob", "case \"$v\" in\n a*) ;;\n *) exit 1 ;;\nesac", 0},
		{"inside an if", "if [ -n \"$CI\" ]; then\n[[ \"$v\" =~ ^[a-z]+$ ]] || exit 1\nfi", 0},
		{"two variables", `[[ "$v$w" =~ ^[a-z]+$ ]] || exit 1`, 0},
		{"background", `[[ "$v" =~ ^[a-z]+$ ]] || exit 1 &`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			s, err := Analyze(tc.script, "bash")
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Guards) != tc.want {
				t.Errorf("%d guards, want %d", len(s.Guards), tc.want)
			}
		})
	}
}

func TestCond(t *testing.T) {
	s, err := Analyze("A=1\nif x; then B=1; fi\nfor i in 1; do C=1; done\nx && D=1\nf() { E=1; }\n(F=1)\nG=$(H=1)\nx | read I", "bash")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"A": false, "B": true, "C": true, "D": true, "E": true, "F": true, "G": false, "H": true}
	for _, a := range s.Assignments {
		if w, ok := want[a.Name]; ok && a.Cond != w {
			t.Errorf("%s: Cond=%v, want %v", a.Name, a.Cond, w)
		}
	}
	if len(s.ForVars) != 1 || s.ForVars[0].Name != "i" {
		t.Errorf("for vars: %+v", s.ForVars)
	}
}

func TestTotals(t *testing.T) {
	tests := []struct {
		what, script string
		want         int
	}{
		{"case with default", "case $x in\n a) V=1 ;;\n *) V=2 ;;\nesac", 1},
		{"case with default that leaves", "case $x in\n a) V=1 ;;\n *) exit 1 ;;\nesac", 1},
		{"case without default", "case $x in\n a) V=1 ;;\nesac", 0},
		{"case with a branch that does not set it", "case $x in\n a) V=1 ;;\n *) echo ;;\nesac", 0},
		{"if else", "if x; then V=1; else V=2; fi", 1},
		{"if without else", "if x; then V=1; fi", 0},
		{"if elif else", "if x; then V=1; elif y; then V=2; else V=3; fi", 1},
		{"if elif", "if x; then V=1; elif y; then V=2; fi", 0},
		{"append", "if x; then V+=1; else V+=2; fi", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			s, err := Analyze(tc.script, "bash")
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Totals) != tc.want {
				t.Errorf("%d totals, want %d", len(s.Totals), tc.want)
			}
		})
	}
}

package runscript

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func mustAnalyze(t *testing.T, src string) *Script {
	t.Helper()
	s, err := Analyze(src, "bash")
	if err != nil {
		t.Fatalf("Analyze(%q): %v", src, err)
	}
	return s
}

func TestSupportsShell(t *testing.T) {
	for shell, want := range map[string]bool{
		"": true, "bash": true, "sh": true, "bash -e {0}": true, "bash --noprofile --norc -eo pipefail {0}": true,
		"/bin/bash": true, "/usr/bin/env bash": false, "sh -e {0}": true, "bash.exe": true,
		"pwsh": false, "powershell": false, "cmd": false, "cmd /D /E:ON /V:OFF /S /C \"CALL \"{0}\"\"": false,
		"python": false, "python {0}": false, "node": false, "zsh": false, "ruby {0}": false, "bashism": false,
	} {
		if got := SupportsShell(shell); got != want {
			t.Errorf("SupportsShell(%q) = %v, want %v", shell, got, want)
		}
	}
}

func TestAnalyzeErrors(t *testing.T) {
	if s, err := Analyze("echo hi", "pwsh"); s != nil || !errors.Is(err, ErrUnsupportedShell) {
		t.Errorf("pwsh: %v %v", s, err)
	}
	for _, src := range []string{"if true; then", "echo \"x", "echo ${{ x", "fi", "((", "echo $(", "cat <<EOF\nx", "case x in"} {
		s, err := Analyze(src, "bash")
		var pe *ParseError
		if s != nil || !errors.As(err, &pe) {
			t.Errorf("%q: want ParseError, got %v %v", src, s, err)
		} else if pe.Unwrap() == nil || !strings.Contains(err.Error(), "runscript") {
			t.Errorf("%q: bad error %v", src, err)
		}
	}
	for _, src := range []string{"", "\n", "# only a comment", "   ", "echo ${{ never closed"[:4]} {
		if _, err := Analyze(src, ""); err != nil && src != "echo" {
			// "echo" is fine, the others too
			t.Errorf("%q: %v", src, err)
		}
	}
}

func TestExpressions(t *testing.T) {
	s := mustAnalyze(t, "echo ${{ a }}${{b}} \"x${{ format('}}', c) }}\"\nfoo ${{\n d\n}}")
	var got []string
	for _, e := range s.Exprs {
		got = append(got, e.Text)
	}
	if want := []string{"a", "b", "format('}}', c)", "d"}; !reflect.DeepEqual(got, want) {
		t.Errorf("exprs %q, want %q", got, want)
	}
	w := s.Commands[0].Args[0]
	if w.Value != "${{ a }}${{b}}" || !reflect.DeepEqual(w.Exprs, []string{"a", "b"}) || w.IsExpr() || w.Literal() || !w.Dynamic() {
		t.Errorf("word %#v", w)
	}
	if w := s.Commands[0].Args[1]; w.Value != "x${{ format('}}', c) }}" || !w.Quoted {
		t.Errorf("quoted word %#v", w)
	}
	if w := s.Commands[1].Args[0]; !w.IsExpr() || w.Line != 2 || w.Col != 5 {
		t.Errorf("multi-line word %#v", w)
	}
	// an expression is not confused with real text that looks like its placeholder
	s = mustAnalyze(t, "echo X0___ ${{ a }} X0_____")
	if v := s.Commands[0].Args; v[0].Value != "X0___" || v[1].Value != "${{ a }}" || v[2].Value != "X0_____" {
		t.Errorf("placeholder lookalikes: %q %q %q", v[0].Value, v[1].Value, v[2].Value)
	}
}

func TestManyExpressions(t *testing.T) {
	// more expressions than the placeholder of the shortest ones can number
	src := strings.Repeat("echo ${{}} ${{ x }}\n", 1500)
	s := mustAnalyze(t, src)
	if len(s.Exprs) != 3000 || len(s.Commands) != 1500 {
		t.Fatalf("%d exprs, %d commands", len(s.Exprs), len(s.Commands))
	}
	for _, c := range s.Commands[:3] {
		if c.Args[1].Value != "${{ x }}" {
			t.Errorf("%q", c.Args[1].Value)
		}
	}
}

func TestWords(t *testing.T) {
	tests := []struct {
		src                string
		value              string
		quoted, glob, subs bool
		varName            string
		vars               []string
	}{
		{`echo plain`, "plain", false, false, false, "", nil},
		{`echo 'single $X'`, "single $X", true, false, false, "", nil},
		{`echo "dq $X end"`, "dq $X end", true, false, false, "", []string{"X"}},
		{`echo "$X"`, `$X`, true, false, false, "X", []string{"X"}},
		{`echo "${X}"`, `${X}`, true, false, false, "X", []string{"X"}},
		{`echo ${X}`, `${X}`, false, false, false, "X", []string{"X"}},
		{`echo ${X:-d}`, `${X:-d}`, false, false, false, "", []string{"X"}},
		{`echo ${#X}`, `${#X}`, false, false, false, "", []string{"X"}},
		{`echo $X/y`, `$X/y`, false, false, false, "", []string{"X"}},
		{`echo $X_Y`, `$X_Y`, false, false, false, "X_Y", []string{"X_Y"}},
		{`echo a\ b`, "a b", false, false, false, "", nil},
		{`echo "a\"b\$c\d"`, `a"b$c\d`, true, false, false, "", nil},
		{`echo *.go`, "*.go", false, true, false, "", nil},
		{`echo "*.go"`, "*.go", true, false, false, "", nil},
		{`echo ~/x`, "~/x", false, true, false, "", nil},
		{`echo $(date)`, "$(date)", false, false, true, "", nil},
		{"echo `date`", "`date`", false, false, true, "", nil},
		{`echo $((1+2))`, "$((1+2))", false, false, true, "", nil},
		{`echo a"b"'c'`, "abc", true, false, false, "", nil},
	}
	for _, tc := range tests {
		s := mustAnalyze(t, tc.src)
		w := s.Commands[0].Args[0]
		name, ok := w.Var()
		if w.Value != tc.value || w.Quoted != tc.quoted || w.Glob != tc.glob || w.Subst != tc.subs || name != tc.varName || ok != (tc.varName != "") || !reflect.DeepEqual(w.Vars, tc.vars) {
			t.Errorf("%q: value=%q quoted=%v glob=%v subst=%v var=%q vars=%q", tc.src, w.Value, w.Quoted, w.Glob, w.Subst, name, w.Vars)
		}
		if tc.src == `echo plain` && (!w.Literal() || w.Dynamic()) {
			t.Error("plain must be literal")
		}
	}
}

func TestWordsInSubstitutions(t *testing.T) {
	s := mustAnalyze(t, `echo "$(curl -s https://x | head -1) $(date)" <(cat f)`)
	if len(s.Commands) != 5 { // echo curl head date cat
		t.Fatalf("%d commands", len(s.Commands))
	}
	echo := s.Commands[0]
	if got := len(echo.Args[0].Subs); got != 3 {
		t.Errorf("subs of word 0: %d", got)
	}
	if w := echo.Args[1]; !w.ProcSubst || len(w.Subs) != 1 || w.Subs[0].Name != "cat" {
		t.Errorf("proc subst %#v", w)
	}
	if len(s.Pipelines) != 1 || len(s.Pipelines[0].Stages) != 2 {
		t.Errorf("pipelines %#v", s.Pipelines)
	}
}

func TestFlags(t *testing.T) {
	s := mustAnalyze(t, `pip install -yq --index-url=U --extra-index-url V -rreq.txt -c c.txt -- -weird pkg`)
	c := s.Commands[0]
	var got []string
	for _, f := range c.Flags {
		v := ""
		if f.Value != nil {
			v = f.Value.Value
		}
		got = append(got, f.Name+"="+v)
	}
	want := []string{"-yq=", "--index-url=U", "--extra-index-url=V", "-r=req.txt", "-c=c.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("flags %q, want %q", got, want)
	}
	var pos []string
	for _, w := range c.Positional {
		pos = append(pos, w.Value)
	}
	if want := []string{"install", "-weird", "pkg"}; !reflect.DeepEqual(pos, want) {
		t.Errorf("positional %q, want %q", pos, want)
	}
	for _, n := range []string{"-y", "-q", "-yq", "--index-url", "-r", "-c"} {
		if !c.HasFlag(n) {
			t.Errorf("HasFlag(%q) = false", n)
		}
	}
	for _, n := range []string{"-x", "--yes", "-i", "--index", "-u"} {
		if c.HasFlag(n) {
			t.Errorf("HasFlag(%q) = true", n)
		}
	}
	if !c.HasFlag("--nope", "-q") || c.Flag("-q") == nil {
		t.Error("HasFlag with several names")
	}
	if vs := c.FlagValues("--index-url", "--extra-index-url"); len(vs) != 2 {
		t.Errorf("FlagValues %v", vs)
	}
	if c.Verb() != "install" || c.Sub(1) != "-weird" || c.Sub(9) != "" {
		t.Errorf("Verb/Sub: %q %q", c.Verb(), c.Sub(1))
	}
	// a value flag at the end of a cluster takes the next word
	s = mustAnalyze(t, `pip install -qr requirements.txt pkg`)
	if r := s.Commands[0].Flag("-qr"); r == nil || r.Value == nil || r.Value.Value != "requirements.txt" || s.Commands[0].Sub(1) != "pkg" {
		t.Errorf("cluster with value: %#v", r)
	}
	// --version of tools is a flag, not a package
	for _, src := range []string{"pip --version", "npm --version", "cargo --version", "go --version", "node --version"} {
		s := mustAnalyze(t, src)
		if in := s.Commands[0].Installs(); in != nil || len(s.Commands[0].Positional) != 0 {
			t.Errorf("%q: %#v", src, in)
		}
	}
}

func TestWrappersAndTools(t *testing.T) {
	tests := []struct {
		src      string
		name     string
		tool     string
		wrappers []string
		args     string
	}{
		{"pip install x", "pip", "pip", nil, "install x"},
		{"pip3.11 install x", "pip3.11", "pip", nil, "install x"},
		{"/usr/local/bin/pip3 install x", "pip3", "pip", nil, "install x"},
		{"python -m pip install x", "python", "pip", nil, "install x"},
		{"python3.12 -u -m pip install x", "python3.12", "pip", nil, "install x"},
		{"py -m pip install x", "py", "pip", nil, "install x"},
		{"python -m twine upload d/*", "python", "twine", nil, "upload d/*"},
		{"python -m build", "python", "", nil, "-m build"},
		{"python script.py", "python", "", nil, "script.py"},
		{"sudo -E -u root apt-get install x", "apt-get", "apt", []string{"sudo"}, "install x"},
		{"sudo env A=1 B=2 npm i x", "npm", "npm", []string{"sudo", "env"}, "i x"},
		{"env -u FOO -- npm i x", "npm", "npm", []string{"env"}, "i x"},
		{"timeout 30 cargo install x", "cargo", "cargo", []string{"timeout"}, "install x"},
		{"time -p go install x", "go", "go", nil, "install x"},
		{"nice -n 5 gem install x", "gem", "gem", []string{"nice"}, "install x"},
		{"command pip install x", "pip", "pip", []string{"command"}, "install x"},
		{"cargo-binstall x", "cargo-binstall", "cargo", nil, "binstall x"},
		{"unknown install x", "unknown", "", nil, "install x"},
		{"pip.exe install x", "pip.exe", "pip", nil, "install x"},
	}
	for _, tc := range tests {
		s := mustAnalyze(t, tc.src)
		c := s.Commands[0]
		var args []string
		for _, a := range c.Args {
			args = append(args, a.Value)
		}
		if c.Name != tc.name || c.Tool != tc.tool || !reflect.DeepEqual(c.Wrappers, tc.wrappers) || strings.Join(args, " ") != tc.args {
			t.Errorf("%q: name=%q tool=%q wrappers=%q args=%q", tc.src, c.Name, c.Tool, c.Wrappers, args)
		}
	}
	// a dynamic command name
	s := mustAnalyze(t, `${{ matrix.tool }} install x`)
	if c := s.Commands[0]; c.Name != "" || c.NameWord == nil || !c.NameWord.IsExpr() || c.Tool != "" || c.Installs() != nil {
		t.Errorf("%#v", c)
	}
	s = mustAnalyze(t, `command -v pip`)
	if c := s.Commands[0]; c.Tool != "" {
		t.Errorf("command -v: %#v", c)
	}
}

func TestAssignments(t *testing.T) {
	s := mustAnalyze(t, "A=1\nB+=x C=$(date) cmd arg\nexport D=2 E\ndeclare -a F=(a b)\nreadonly G=3")
	var got []string
	for _, a := range s.Assignments {
		got = append(got, fmt.Sprintf("%s:%v:%v:%v", a.Name, a.Append, a.Array, a.Cmd != nil))
	}
	want := []string{"A:false:false:false", "B:true:false:true", "C:false:false:true", "D:false:false:true", "E:false:false:true", "F:false:true:true", "G:false:false:true"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("assignments %q, want %q", got, want)
	}
	var decl []string
	for _, c := range s.Commands {
		if c.Decl {
			decl = append(decl, c.Name)
		}
	}
	if !reflect.DeepEqual(decl, []string{"export", "declare", "readonly"}) {
		t.Errorf("decl %q", decl)
	}
	if v := s.Commands[len(s.Commands)-1]; v.Name != "readonly" {
		_ = v
	}
}

func TestPipelines(t *testing.T) {
	s := mustAnalyze(t, "a | b |& c\n! d | e\nf && g | h\n{ i | j; } | k\nl")
	var shape []string
	for _, p := range s.Pipelines {
		shape = append(shape, fmt.Sprintf("%d%v", len(p.Stages), p.Negated))
	}
	if want := []string{"3false", "2true", "2false", "2false", "2false"}; !reflect.DeepEqual(shape, want) {
		t.Errorf("pipelines %q, want %q", shape, want)
	}
	for _, c := range s.Commands {
		if c.Name == "l" && c.Pipeline != nil {
			t.Error("l is not piped")
		}
		if c.Name == "b" && (c.Stage != 1 || c.Pipeline == nil) {
			t.Errorf("b stage %d", c.Stage)
		}
	}
	// the nested pipeline wins for the commands in it
	for _, c := range s.Commands {
		if c.Name == "i" && len(c.Pipeline.Stages) != 2 {
			t.Error("i")
		}
	}
	// commands in substitutions are not stages of the pipeline of the command holding them
	s = mustAnalyze(t, "echo $(a | b) | c")
	if len(s.Pipelines) != 2 {
		t.Fatalf("pipelines: %d", len(s.Pipelines))
	}
	outer := s.Pipelines[0]
	if len(outer.Stages[0].Commands) != 1 || outer.Stages[0].Commands[0].Name != "echo" {
		t.Errorf("stage 0 %#v", outer.Stages[0].Commands)
	}
}

func TestRedirects(t *testing.T) {
	type r struct {
		op, fd, target     string
		appendF, write     bool
		group, tee, heredc bool
	}
	tests := []struct {
		src  string
		want []r
	}{
		{`echo >> f`, []r{{">>", "", "f", true, true, false, false, false}}},
		{`echo > f`, []r{{">", "", "f", false, true, false, false, false}}},
		{`echo >| f`, []r{{">|", "", "f", false, true, false, false, false}}},
		{`echo &> f`, []r{{"&>", "", "f", false, true, false, false, false}}},
		{`echo &>> f`, []r{{"&>>", "", "f", true, true, false, false, false}}},
		{`echo 2>&1`, []r{{">&", "2", "1", false, false, false, false, false}}},
		{`echo >&2`, []r{{">&", "", "2", false, false, false, false, false}}},
		{`echo >&f`, []r{{">&", "", "f", false, true, false, false, false}}},
		{`echo 2> err`, []r{{">", "2", "err", false, true, false, false, false}}},
		{`cat < in`, []r{{"<", "", "in", false, false, false, false, false}}},
		{"cat <<E\nx\nE", []r{{"<<", "", "E", false, false, false, false, true}}},
		{"cat <<-E\n\tx\n\tE", []r{{"<<-", "", "E", false, false, false, false, true}}},
		{`cat <<< "x"`, []r{{"<<<", "", "x", false, false, false, false, false}}},
		{`{ a; } >> f`, []r{{">>", "", "f", true, true, true, false, false}}},
		{`( a ) > f`, []r{{">", "", "f", false, true, true, false, false}}},
		{`while a; do b; done > f`, []r{{">", "", "f", false, true, true, false, false}}},
		{`>> f`, []r{{">>", "", "f", true, true, false, false, false}}},
		{`a | tee -a f g`, []r{{"tee", "", "f", true, true, false, true, false}, {"tee", "", "g", true, true, false, true, false}}},
		{`a | tee f`, []r{{"tee", "", "f", false, true, false, true, false}}},
		{`a | tee`, nil},
		{`a | tee --append f`, []r{{"tee", "", "f", true, true, false, true, false}}},
	}
	for _, tc := range tests {
		s := mustAnalyze(t, tc.src)
		var got []r
		for _, x := range s.Redirects {
			tg := ""
			if x.Target != nil {
				tg = x.Target.Value
			}
			got = append(got, r{x.Op, x.Fd, tg, x.Append, x.Write, x.Group, x.Tee, x.Heredoc != nil})
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q:\n got  %v\n want %v", tc.src, got, tc.want)
		}
	}
	s := mustAnalyze(t, "cat <<-'EOF' > f\n\t${{ a }} $B\n\tEOF\n")
	var h *Heredoc
	for _, x := range s.Redirects {
		if x.Heredoc != nil {
			h = x.Heredoc
		}
	}
	if h == nil || h.Delim != "EOF" || !h.Quoted || !h.Tabs || h.Body != "\t${{ a }} $B\n" || !reflect.DeepEqual(h.Exprs, []string{"a"}) {
		t.Errorf("heredoc %#v", h)
	}
}

func TestWritesTo(t *testing.T) {
	tests := []struct {
		name string
		src  string
		vars []string
		want int
	}{
		{"plain", `echo a >> $GITHUB_ENV`, []string{"GITHUB_ENV"}, 1},
		{"quoted", `echo a >> "$GITHUB_ENV"`, []string{"GITHUB_ENV"}, 1},
		{"braced", `echo a >> ${GITHUB_ENV}`, []string{"GITHUB_ENV"}, 1},
		{"quoted braced", `echo a >> "${GITHUB_ENV}"`, []string{"GITHUB_ENV"}, 1},
		{"truncate", `echo a > $GITHUB_ENV`, []string{"GITHUB_ENV"}, 1},
		{"no space", `echo a >>$GITHUB_ENV`, []string{"GITHUB_ENV"}, 1},
		{"prefix var", `echo a >> $GITHUB_ENV_x`, []string{"GITHUB_ENV"}, 0},
		{"longer var", `echo a >> $GITHUB_ENVIRONMENT`, []string{"GITHUB_ENV"}, 0},
		{"suffix var", `echo a >> $MY_GITHUB_ENV`, []string{"GITHUB_ENV"}, 0},
		{"dir join", `echo a >> $RUNNER_TEMP/$GITHUB_ENV`, []string{"GITHUB_ENV"}, 0},
		{"path suffix", `echo a >> "$GITHUB_ENV/x"`, []string{"GITHUB_ENV"}, 0},
		{"single quotes", `echo a >> '$GITHUB_ENV'`, []string{"GITHUB_ENV"}, 0},
		{"escaped", `echo a >> \$GITHUB_ENV`, []string{"GITHUB_ENV"}, 0},
		{"default value", `echo a >> ${GITHUB_ENV:-/dev/null}`, []string{"GITHUB_ENV"}, 0},
		{"stdin redirect", `cat < $GITHUB_ENV`, []string{"GITHUB_ENV"}, 0},
		{"dup or file", `echo a >&$GITHUB_ENV`, []string{"GITHUB_ENV"}, 1},
		{"other file", `echo a >> $GITHUB_PATH`, []string{"GITHUB_ENV"}, 0},
		{"several names", "echo a >> $GITHUB_PATH\necho a >> $GITHUB_OUTPUT\necho a >> $GITHUB_ENV", []string{"GITHUB_PATH", "GITHUB_OUTPUT", "GITHUB_STATE"}, 2},
		{"tee -a", `echo a | tee -a $GITHUB_ENV`, []string{"GITHUB_ENV"}, 1},
		{"sudo tee", `echo a | sudo tee -a "$GITHUB_ENV" >/dev/null`, []string{"GITHUB_ENV"}, 1},
		{"tee other", `echo a | tee -a out.txt`, []string{"GITHUB_ENV"}, 0},
		{"group", `{ echo a; echo b; } >> $GITHUB_ENV`, []string{"GITHUB_ENV"}, 1},
		{"subshell", `( echo a ) >> $GITHUB_ENV`, []string{"GITHUB_ENV"}, 1},
		{"loop", `for i in 1 2; do echo $i; done >> $GITHUB_ENV`, []string{"GITHUB_ENV"}, 1},
		{"heredoc", "cat <<EOF >> $GITHUB_ENV\nA=1\nEOF", []string{"GITHUB_ENV"}, 1},
		{"heredoc first", "cat >> $GITHUB_ENV <<EOF\nA=1\nEOF", []string{"GITHUB_ENV"}, 1},
		{"alias", "F=$GITHUB_ENV\necho a >> \"$F\"", []string{"GITHUB_ENV"}, 1},
		{"alias chain", "F=$GITHUB_ENV\nG=\"$F\"\necho a >> $G", []string{"GITHUB_ENV"}, 1},
		{"export alias", "export F=$GITHUB_ENV; echo a >> $F", []string{"GITHUB_ENV"}, 1},
		{"not an alias", "F=$GITHUB_ENV/x\necho a >> $F", []string{"GITHUB_ENV"}, 0},
		{"in if", `if x; then echo a >> $GITHUB_ENV; fi`, []string{"GITHUB_ENV"}, 1},
		{"in function", `f() { echo a >> $GITHUB_ENV; }; f`, []string{"GITHUB_ENV"}, 1},
		{"in subst", `x=$(echo a >> $GITHUB_ENV)`, []string{"GITHUB_ENV"}, 1},
		{"no names", `echo a >> $GITHUB_ENV`, nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := mustAnalyze(t, tc.src)
			ws := s.WritesTo(tc.vars...)
			if len(ws) != tc.want {
				t.Fatalf("%q: %d writes, want %d", tc.src, len(ws), tc.want)
			}
			for _, w := range ws {
				if w.Redirect == nil || w.Var == "" {
					t.Errorf("incomplete write %#v", w)
				}
			}
		})
	}
}

func TestWritesToDetails(t *testing.T) {
	s := mustAnalyze(t, "echo \"A=${{ github.event.title }}\" | tee -a $GITHUB_ENV\n"+
		"{ echo a; echo \"B=${{ inputs.b }}\"; } >> \"$GITHUB_OUTPUT\"\n"+
		"cat <<EOF >> $GITHUB_PATH\n${{ matrix.p }}\nEOF\n"+
		"tee -a $GITHUB_ENV <<< \"C=${{ env.c }}\"\n"+
		"echo a > $GITHUB_STATE")
	ws := s.WritesTo("GITHUB_ENV", "GITHUB_OUTPUT", "GITHUB_PATH", "GITHUB_STATE")
	var got []string
	for _, w := range ws {
		got = append(got, fmt.Sprintf("%s append=%v producers=%d exprs=%q heredoc=%v tee=%v", w.Var, w.Append, len(w.Producers), w.Exprs, w.Heredoc != nil, w.Redirect.Tee))
	}
	want := []string{
		`GITHUB_ENV append=true producers=2 exprs=["github.event.title"] heredoc=false tee=true`,
		`GITHUB_OUTPUT append=true producers=2 exprs=["inputs.b"] heredoc=false tee=false`,
		`GITHUB_PATH append=true producers=1 exprs=["matrix.p"] heredoc=true tee=false`,
		`GITHUB_ENV append=true producers=1 exprs=["env.c"] heredoc=false tee=true`,
		`GITHUB_STATE append=false producers=1 exprs=[] heredoc=false tee=false`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestInstalls(t *testing.T) {
	type pkg struct {
		spec, name, ver, kind string
		pinned                bool
	}
	p := func(spec, name, ver, kind string, pinned bool) pkg { return pkg{spec, name, ver, kind, pinned} }
	tests := []struct {
		src      string
		tool     string
		verb     string
		run      bool
		global   bool
		locked   bool
		manifest bool
		reqs     int
		pkgs     []pkg
	}{
		// pip
		{"pip install requests", "pip", "install", false, false, false, false, 0, []pkg{p("requests", "requests", "", KindRegistry, false)}},
		{"pip install requests==2.0 'a>=1' 'b~=1.0' c==1.*", "pip", "install", false, false, false, false, 0, []pkg{
			p("requests==2.0", "requests", "==2.0", KindRegistry, true), p("a>=1", "a", ">=1", KindRegistry, false),
			p("b~=1.0", "b", "~=1.0", KindRegistry, false), p("c==1.*", "c", "==1.*", KindRegistry, false)}},
		{"pip install 'x[extra]==1.0; python_version<\"3.9\"'", "pip", "install", false, false, false, false, 0, []pkg{p(`x[extra]==1.0; python_version<"3.9"`, "x", "==1.0", KindRegistry, true)}},
		{"pip install -r r.txt --require-hashes", "pip", "install", false, false, true, false, 1, nil},
		{"pip install --upgrade pip", "pip", "install", false, false, false, false, 0, []pkg{p("pip", "pip", "", KindRegistry, false)}},
		{"pip install -e . ./x.whl ../y.tgz /z.zip dist/*.whl", "pip", "install", false, false, false, false, 0, []pkg{
			p("./x.whl", "./x.whl", "", KindPath, false), p("../y.tgz", "../y.tgz", "", KindPath, false), p("/z.zip", "/z.zip", "", KindPath, false),
			p("dist/*.whl", "dist/*.whl", "", KindPath, false), p(".", ".", "", KindPath, false)}},
		{"pip install git+https://github.com/o/r.git@v1.0 git+https://github.com/o/r@main git+https://github.com/o/r@0123abc git+ssh://git@github.com/o/r.git", "pip", "install", false, false, false, false, 0, []pkg{
			p("git+https://github.com/o/r.git@v1.0", "", "git+https://github.com/o/r.git@v1.0", KindGit, true),
			p("git+https://github.com/o/r@main", "", "git+https://github.com/o/r@main", KindGit, false),
			p("git+https://github.com/o/r@0123abc", "", "git+https://github.com/o/r@0123abc", KindGit, true),
			p("git+ssh://git@github.com/o/r.git", "", "git+ssh://git@github.com/o/r.git", KindGit, false)}},
		{"pip install https://x/y.whl#sha256=ab https://x/z.whl", "pip", "install", false, false, false, false, 0, []pkg{
			p("https://x/y.whl#sha256=ab", "", "https://x/y.whl#sha256=ab", KindURL, true), p("https://x/z.whl", "", "https://x/z.whl", KindURL, false)}},
		{"pip install 'p==$V' $OTHER ${{ inputs.p }} 'q==${{ inputs.v }}'", "pip", "install", false, false, false, false, 0, []pkg{
			p("p==$V", "p", "==$V", KindRegistry, true), p("$OTHER", "", "", KindDynamic, false), p("${{ inputs.p }}", "", "", KindDynamic, false),
			p("q==${{ inputs.v }}", "q", "==${{ inputs.v }}", KindRegistry, true)}},
		{"pip install --index-url https://i/simple --extra-index-url https://j pkg", "pip", "install", false, false, false, false, 0, []pkg{p("pkg", "pkg", "", KindRegistry, false)}},
		{"python3 -m pip install -U pip==24.0", "pip", "install", false, false, false, false, 0, []pkg{p("pip==24.0", "pip", "==24.0", KindRegistry, true)}},
		{"sudo pip3 install x", "pip", "install", false, false, false, false, 0, []pkg{p("x", "x", "", KindRegistry, false)}},
		// uv, pipx
		{"uv pip install --system a==1 b", "uv", "install", false, false, false, false, 0, []pkg{p("a==1", "a", "==1", KindRegistry, true), p("b", "b", "", KindRegistry, false)}},
		{"uv tool install ruff", "uv", "install", false, false, false, false, 0, []pkg{p("ruff", "ruff", "", KindRegistry, false)}},
		{"uv tool install --from 'ruff==0.4' ruff", "uv", "install", false, false, false, false, 0, []pkg{p("ruff==0.4", "ruff", "==0.4", KindRegistry, true)}},
		{"uv tool run --from ruff==1 ruff check", "uv", "run", true, false, false, false, 0, []pkg{p("ruff==1", "ruff", "==1", KindRegistry, true)}},
		{"uvx ruff check .", "uvx", "run", true, false, false, false, 0, []pkg{p("ruff", "ruff", "", KindRegistry, false)}},
		{"uv add pytest", "uv", "add", false, false, false, false, 0, []pkg{p("pytest", "pytest", "", KindRegistry, false)}},
		{"uv sync --locked", "uv", "sync", false, false, true, true, 0, nil},
		{"uv run pytest", "", "", false, false, false, false, 0, nil},
		{"pipx install black", "pipx", "install", false, false, false, false, 0, []pkg{p("black", "black", "", KindRegistry, false)}},
		{"pipx install --python python3.12 black==24", "pipx", "install", false, false, false, false, 0, []pkg{p("black==24", "black", "==24", KindRegistry, true)}},
		{"pipx run --spec ruff==1 ruff", "pipx", "run", true, false, false, false, 0, []pkg{p("ruff==1", "ruff", "==1", KindRegistry, true)}},
		{"pipx run ruff", "pipx", "run", true, false, false, false, 0, []pkg{p("ruff", "ruff", "", KindRegistry, false)}},
		// node
		{"npm install", "npm", "install", false, false, false, true, 0, nil},
		{"npm ci", "npm", "ci", false, false, true, true, 0, nil},
		{"npm i -g typescript@5.4.2 eslint@^8 prettier@latest jest@29 @s/p@1.0.0 @s/q", "npm", "i", false, true, false, false, 0, []pkg{
			p("typescript@5.4.2", "typescript", "5.4.2", KindRegistry, true), p("eslint@^8", "eslint", "^8", KindRegistry, false),
			p("prettier@latest", "prettier", "latest", KindRegistry, false), p("jest@29", "jest", "29", KindRegistry, false),
			p("@s/p@1.0.0", "@s/p", "1.0.0", KindRegistry, true), p("@s/q", "@s/q", "", KindRegistry, false)}},
		{"npm install foo@1.2.3-beta.1 bar@v2.0.0 baz@${{ matrix.v }}", "npm", "install", false, false, false, false, 0, []pkg{
			p("foo@1.2.3-beta.1", "foo", "1.2.3-beta.1", KindRegistry, true), p("bar@v2.0.0", "bar", "v2.0.0", KindRegistry, true),
			p("baz@${{ matrix.v }}", "baz", "${{ matrix.v }}", KindRegistry, true)}},
		{"npm install ./a ../b.tgz file:../c d.tgz", "npm", "install", false, false, false, false, 0, []pkg{
			p("./a", "./a", "", KindPath, false), p("../b.tgz", "../b.tgz", "", KindPath, false), p("file:../c", "file:../c", "", KindPath, false), p("d.tgz", "d.tgz", "", KindPath, false)}},
		{"npm install o/r o/r#v1.0.0 github:o/r#0123abcd o/r#main git+https://h/r.git#semver:^1", "npm", "install", false, false, false, false, 0, []pkg{
			p("o/r", "o/r", "", KindGit, false), p("o/r#v1.0.0", "o/r", "v1.0.0", KindGit, true), p("github:o/r#0123abcd", "github:o/r", "0123abcd", KindGit, true),
			p("o/r#main", "o/r", "main", KindGit, false), p("git+https://h/r.git#semver:^1", "git+https://h/r.git", "semver:^1", KindGit, false)}},
		{"npm install --prefix sub --registry https://r pkg", "npm", "install", false, false, false, false, 0, []pkg{p("pkg", "pkg", "", KindRegistry, false)}},
		{"npm run build", "", "", false, false, false, false, 0, nil},
		{"npm exec -- foo", "npm", "exec", true, false, false, false, 0, []pkg{p("foo", "foo", "", KindRegistry, false)}},
		{"npx --yes cra@5.0.1 app", "npx", "run", true, false, false, false, 0, []pkg{p("cra@5.0.1", "cra", "5.0.1", KindRegistry, true)}},
		{"npx -p typescript@5 tsc", "npx", "run", true, false, false, false, 0, []pkg{p("typescript@5", "typescript", "5", KindRegistry, false)}},
		{"pnpm install --frozen-lockfile", "pnpm", "install", false, false, true, true, 0, nil},
		{"pnpm add -D --filter w vitest@1.0.0", "pnpm", "add", false, false, false, false, 0, []pkg{p("vitest@1.0.0", "vitest", "1.0.0", KindRegistry, true)}},
		{"pnpm dlx cowsay@1.6.0", "pnpm", "dlx", true, false, false, false, 0, []pkg{p("cowsay@1.6.0", "cowsay", "1.6.0", KindRegistry, true)}},
		{"yarn", "yarn", "install", false, false, false, true, 0, nil},
		{"yarn install --immutable", "yarn", "install", false, false, true, true, 0, nil},
		{"yarn add x@1.0.0", "yarn", "add", false, false, false, false, 0, []pkg{p("x@1.0.0", "x", "1.0.0", KindRegistry, true)}},
		{"yarn global add serve", "yarn", "add", false, true, false, false, 0, []pkg{p("serve", "serve", "", KindRegistry, false)}},
		{"yarn build", "", "", false, false, false, false, 0, nil},
		{"bun add zod@3.22.4", "bun", "add", false, false, false, false, 0, []pkg{p("zod@3.22.4", "zod", "3.22.4", KindRegistry, true)}},
		{"bun install --frozen-lockfile", "bun", "install", false, false, true, true, 0, nil},
		{"bunx cowsay", "bunx", "run", true, false, false, false, 0, []pkg{p("cowsay", "cowsay", "", KindRegistry, false)}},
		{"aube add lodash@4.17.21", "aube", "add", false, false, false, false, 0, []pkg{p("lodash@4.17.21", "lodash", "4.17.21", KindRegistry, true)}},
		{"aube install", "aube", "install", false, false, false, true, 0, nil},
		// cargo
		{"cargo install ripgrep", "cargo", "install", false, false, false, false, 0, []pkg{p("ripgrep", "ripgrep", "", KindRegistry, false)}},
		{"cargo install --locked ripgrep --version 14.1.0", "cargo", "install", false, false, true, false, 0, []pkg{p("ripgrep", "ripgrep", "14.1.0", KindRegistry, true)}},
		{"cargo install --version 14 ripgrep", "cargo", "install", false, false, false, false, 0, []pkg{p("ripgrep", "ripgrep", "14", KindRegistry, false)}},
		{"cargo install ripgrep@14.1.0 fd-find", "cargo", "install", false, false, false, false, 0, []pkg{p("ripgrep@14.1.0", "ripgrep", "14.1.0", KindRegistry, true), p("fd-find", "fd-find", "", KindRegistry, false)}},
		{"cargo install --git https://g/r --rev 0123abc", "cargo", "install", false, false, false, false, 0, []pkg{p("https://g/r", "https://g/r", "", KindGit, true)}},
		{"cargo install --git https://g/r --branch main tool", "cargo", "install", false, false, false, false, 0, []pkg{p("tool", "tool", "", KindGit, false)}},
		{"cargo install --path . --force", "cargo", "install", false, false, false, false, 0, []pkg{p(".", ".", "", KindPath, false)}},
		{"cargo binstall -y cargo-deny@0.14.0", "cargo", "binstall", false, false, false, false, 0, []pkg{p("cargo-deny@0.14.0", "cargo-deny", "0.14.0", KindRegistry, true)}},
		{"cargo build --release", "", "", false, false, false, false, 0, nil},
		// go
		{"go install golang.org/x/tools/cmd/goimports@latest", "go", "install", false, false, false, false, 0, []pkg{p("golang.org/x/tools/cmd/goimports@latest", "golang.org/x/tools/cmd/goimports", "latest", KindModule, false)}},
		{"go install a.io/b@v1.2.3 c.io/d@0123456789ab e.io/f@master g.io/h", "go", "install", false, false, false, false, 0, []pkg{
			p("a.io/b@v1.2.3", "a.io/b", "v1.2.3", KindModule, true), p("c.io/d@0123456789ab", "c.io/d", "0123456789ab", KindModule, true),
			p("e.io/f@master", "e.io/f", "master", KindModule, false), p("g.io/h", "g.io/h", "", KindModule, false)}},
		{"go install ./cmd/x ./...", "go", "install", false, false, false, false, 0, []pkg{p("./cmd/x", "./cmd/x", "", KindPath, false), p("./...", "./...", "", KindPath, false)}},
		{"go get -u a.io/b", "go", "get", false, false, false, false, 0, []pkg{p("a.io/b", "a.io/b", "", KindModule, false)}},
		{"go build ./... && go test ./...", "", "", false, false, false, false, 0, nil},
		// gem
		{"gem install bundler", "gem", "install", false, false, false, false, 0, []pkg{p("bundler", "bundler", "", KindRegistry, false)}},
		{"gem install bundler -v 2.5.3", "gem", "install", false, false, false, false, 0, []pkg{p("bundler", "bundler", "2.5.3", KindRegistry, true)}},
		{"gem install rake --version '~> 13'", "gem", "install", false, false, false, false, 0, []pkg{p("rake", "rake", "~> 13", KindRegistry, false)}},
		{"gem install a:1.0 ./b.gem", "gem", "install", false, false, false, false, 0, []pkg{p("a:1.0", "a", "1.0", KindRegistry, true), p("./b.gem", "./b.gem", "", KindPath, false)}},
		// apt, brew
		{"sudo apt-get install -y --no-install-recommends curl=7.88 git", "apt", "install", false, false, false, false, 0, []pkg{p("curl=7.88", "curl", "7.88", KindRegistry, true), p("git", "git", "", KindRegistry, false)}},
		{"apt install -t bookworm-backports foo ./x.deb", "apt", "install", false, false, false, false, 0, []pkg{p("foo", "foo", "", KindRegistry, false), p("./x.deb", "./x.deb", "", KindPath, false)}},
		{"apt-get update", "", "", false, false, false, false, 0, nil},
		{"apt-get remove -y foo", "", "", false, false, false, false, 0, nil},
		{"brew install jq python@3.12 ./x.rb", "brew", "install", false, false, false, false, 0, []pkg{p("jq", "jq", "", KindRegistry, false), p("python@3.12", "python", "3.12", KindRegistry, true), p("./x.rb", "./x.rb", "", KindPath, false)}},
		{"brew update", "", "", false, false, false, false, 0, nil},
		// not installs
		{"echo pip install x", "", "", false, false, false, false, 0, nil},
		{"mise install node", "", "", false, false, false, false, 0, nil},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			s := mustAnalyze(t, tc.src)
			var in *Install
			for _, c := range s.Commands {
				if i := c.Installs(); i != nil {
					in = i
				}
			}
			if tc.tool == "" {
				if in != nil {
					t.Fatalf("not an install, got %#v", in)
				}
				return
			}
			if in == nil {
				t.Fatal("no install")
			}
			if in.Tool != tc.tool || in.Verb != tc.verb || in.Run != tc.run || in.Global != tc.global || in.Locked != tc.locked || in.FromManifest != tc.manifest || len(in.Requirements) != tc.reqs {
				t.Errorf("install: tool=%s verb=%s run=%v global=%v locked=%v manifest=%v reqs=%d", in.Tool, in.Verb, in.Run, in.Global, in.Locked, in.FromManifest, len(in.Requirements))
			}
			var got []pkg
			for _, x := range in.Packages {
				got = append(got, pkg{x.Spec, x.Name, x.Version, x.Kind, x.Pinned})
				if x.Local != (x.Kind == KindPath) {
					t.Errorf("%s: Local=%v for kind %s", x.Spec, x.Local, x.Kind)
				}
				if x.Word == nil || x.Word.Value == "" {
					t.Errorf("%s: no word", x.Spec)
				}
			}
			if !reflect.DeepEqual(got, tc.pkgs) {
				t.Errorf("packages:\n got  %v\n want %v", got, tc.pkgs)
			}
		})
	}
}

func TestPackageDynamic(t *testing.T) {
	s := mustAnalyze(t, `pip install "a==${{ x }}" b "$C"`)
	pk := s.Commands[0].Installs().Packages
	if !pk[0].Dynamic || pk[1].Dynamic || !pk[2].Dynamic {
		t.Errorf("dynamic: %v %v %v", pk[0].Dynamic, pk[1].Dynamic, pk[2].Dynamic)
	}
}

func TestPublishes(t *testing.T) {
	tests := []struct {
		src      string
		tool     string
		verb     string
		kind     string
		dry      bool
		registry string
	}{
		{"twine upload dist/*", "twine", "upload", "package", false, ""},
		{"python3 -m twine upload --repository-url https://test.pypi.org/legacy/ dist/*", "twine", "upload", "package", false, "https://test.pypi.org/legacy/"},
		{"twine upload -r testpypi d/*", "twine", "upload", "package", false, "testpypi"},
		{"twine check dist/*", "", "", "", false, ""},
		{"cargo publish", "cargo", "publish", "package", false, ""},
		{"cargo publish --dry-run --registry my", "cargo", "publish", "package", true, "my"},
		{"cargo build", "", "", "", false, ""},
		{"npm publish --provenance", "npm", "publish", "package", false, ""},
		{"npm publish --dry-run --registry=https://r", "npm", "publish", "package", true, "https://r"},
		{"npm pack", "", "", "", false, ""},
		{"pnpm publish --dry-run", "pnpm", "publish", "package", true, ""},
		{"poetry publish -n", "poetry", "publish", "package", false, ""},
		{"gh release create v1 -n notes", "gh", "release create", "release", false, ""},
		{"yarn npm publish", "yarn", "npm publish", "package", false, ""},
		{"yarn publish", "yarn", "publish", "package", false, ""},
		{"bun publish", "bun", "publish", "package", false, ""},
		{"gem push a.gem", "gem", "push", "package", false, ""},
		{"uv publish", "uv", "publish", "package", false, ""},
		{"poetry publish --build", "poetry", "publish", "package", false, ""},
		{"gh release create v1 d/*", "gh", "release create", "release", false, ""},
		{"gh release upload v1 d/*", "gh", "release upload", "release", false, ""},
		{"gh -R o/r release create v1", "gh", "release create", "release", false, ""},
		{"gh release view v1", "", "", "", false, ""},
		{"gh pr create", "", "", "", false, ""},
		{"echo cargo publish", "", "", "", false, ""},
		{"sudo npm publish", "npm", "publish", "package", false, ""},
	}
	for _, tc := range tests {
		s := mustAnalyze(t, tc.src)
		p := s.Commands[len(s.Commands)-1].Publishes()
		if tc.tool == "" {
			if p != nil {
				t.Errorf("%q: unexpected %#v", tc.src, p)
			}
			continue
		}
		if p == nil {
			t.Errorf("%q: no publish", tc.src)
			continue
		}
		reg := ""
		if p.Registry != nil {
			reg = p.Registry.Value
		}
		if p.Tool != tc.tool || p.Verb != tc.verb || p.Kind != tc.kind || p.DryRun != tc.dry || reg != tc.registry {
			t.Errorf("%q: %#v registry=%q", tc.src, p, reg)
		}
	}
}

func TestShellPipes(t *testing.T) {
	tests := []struct {
		src  string
		want []string // form:downloader>shell url
	}{
		{"curl -fsSL https://x/i.sh | sh", []string{"pipe:curl>sh https://x/i.sh"}},
		{"curl -fsSL https://x/i.sh | bash", []string{"pipe:curl>bash https://x/i.sh"}},
		{"curl -sSf https://sh.rustup.rs | sh -s -- -y", []string{"pipe:curl>sh https://sh.rustup.rs"}},
		{"wget -qO- https://x/i.sh | sudo bash", []string{"pipe:wget>bash https://x/i.sh"}},
		{"wget -O - https://x/i.sh | /bin/sh -", []string{"pipe:wget>sh https://x/i.sh"}},
		{"curl --proto '=https' -H 'A: b' -o /dev/null https://x | sh", []string{"pipe:curl>sh https://x"}},
		{"curl --url https://x/u | sh", []string{"pipe:curl>sh https://x/u"}},
		{"curl -fsSL https://x | tee f | sh", []string{"pipe:curl>sh https://x"}},
		{"curl https://x | sudo -E bash -e", []string{"pipe:curl>bash https://x"}},
		{"curl -fsSL ${{ inputs.url }} | sh", []string{"pipe:curl>sh ${{ inputs.url }}"}},
		{"bash <(curl -fsSL https://x/i.sh)", []string{"subst:curl>bash https://x/i.sh"}},
		{`sh -c "$(curl -fsSL https://x/i.sh)"`, []string{"subst:curl>sh https://x/i.sh"}},
		{`eval "$(curl -fsSL https://x/e.sh)"`, []string{"subst:curl>eval https://x/e.sh"}},
		{`source <(curl -s https://x/e.sh)`, []string{"subst:curl>source https://x/e.sh"}},
		{"curl https://x | sh\nwget -qO- https://y | bash", []string{"pipe:curl>sh https://x", "pipe:wget>bash https://y"}},
		// not reported
		{"curl https://x | jq .", nil},
		{"curl https://x | bash script.sh", nil},
		{"curl https://x | sh -c 'cat'", nil},
		{"curl -o i.sh https://x && sh i.sh", nil},
		{"curl https://x -o i.sh; bash i.sh", nil},
		{"sh -c \"$(date)\"", nil},
		{"bash script.sh $(curl -s https://x)", nil},
		{"echo curl https://x | sh", nil},
		{"sh <(echo hi)", nil},
		{"sh | curl https://x", nil},
	}
	for _, tc := range tests {
		s := mustAnalyze(t, tc.src)
		var got []string
		for _, p := range s.ShellPipes() {
			u := ""
			if p.URL != nil {
				u = p.URL.Value
			}
			got = append(got, fmt.Sprintf("%s:%s>%s %s", p.Form, p.Downloader.Name, p.Shell.Name, u))
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q:\n got  %q\n want %q", tc.src, got, tc.want)
		}
	}
}

func TestPositions(t *testing.T) {
	lit := Origin{Line: 10, Col: 14, Literal: true, Indent: 10}
	s := mustAnalyze(t, "echo a\n  pip install x >> $GITHUB_ENV\n")
	r := s.Redirects[0]
	if got := s.Position(lit, r.Offset); got != (Position{12, 10 + 17, true}) {
		t.Errorf("literal: %+v", got)
	}
	if got := s.Position(lit, 0); got != (Position{11, 11, true}) {
		t.Errorf("literal first: %+v", got)
	}
	if got := lit.Map(r.Line, r.Col); got != (Position{12, 27, true}) {
		t.Errorf("Map: %+v", got)
	}
	// literal block with unknown indentation: the line is right, the column is not claimed to be
	if got := (Origin{Line: 10, Col: 14, Literal: true}).Map(2, 3); got.Exact || got.Line != 12 {
		t.Errorf("unknown indent: %+v", got)
	}
	// single-line plain and quoted scalars
	one := mustAnalyze(t, "pip install x")
	if got := one.Position(Origin{Line: 5, Col: 12}, 4); got != (Position{5, 16, false}) {
		t.Errorf("plain: %+v", got)
	}
	if got := one.Position(Origin{Line: 5, Col: 12, Quoted: true}, 4); got != (Position{5, 17, false}) {
		t.Errorf("quoted: %+v", got)
	}
	// folded and multi-line plain: approximate by construction, lines follow the value
	if got := s.Position(Origin{Line: 5, Col: 12}, r.Offset); got.Line != 6 || got.Exact {
		t.Errorf("folded: %+v", got)
	}
	// columns count runes
	u := mustAnalyze(t, "echo ✓✓ >> $GITHUB_ENV")
	if r := u.Redirects[0]; r.Col != 9 {
		t.Errorf("rune column: %d", r.Col)
	}
	// multi-line expressions keep the lines of the following commands
	m := mustAnalyze(t, "echo ${{\n a\n}}\necho >> $GITHUB_ENV")
	if r := m.Redirects[0]; r.Line != 4 || m.Position(lit, r.Offset).Line != 14 {
		t.Errorf("after multi-line expression: line %d, %+v", r.Line, m.Position(lit, r.Offset))
	}
	// out of range offsets do not panic
	_ = s.Position(lit, -5)
	_ = s.Position(lit, 1<<30)
}

func TestLocOffsets(t *testing.T) {
	src := "echo   ${{ a }}  foo\nbar \"b z\"\n"
	s := mustAnalyze(t, src)
	for _, c := range s.Commands {
		for _, w := range c.Words {
			if got := src[w.Offset:w.End]; got != w.Raw {
				t.Errorf("Raw %q != source %q", w.Raw, got)
			}
		}
	}
	if c := s.Commands[0]; src[c.Offset:c.End] != "echo   ${{ a }}  foo" {
		t.Errorf("command source %q", src[c.Offset:c.End])
	}
	if e := s.Exprs[0]; src[e.Loc.Offset:e.Loc.End] != "${{ a }}" || e.Raw != "${{ a }}" {
		t.Errorf("expr %#v", e)
	}
}

func TestCRLF(t *testing.T) {
	s := mustAnalyze(t, "echo a >> $GITHUB_ENV\r\nnpm i x\r\n")
	if len(s.WritesTo("GITHUB_ENV")) != 1 || s.Commands[1].Installs() == nil {
		t.Error("CRLF script not understood")
	}
}

func TestFlagClusterEndingInValueFlag(t *testing.T) {
	c := mustAnalyze(t, `pip install -qr req.txt pkg`).Commands[0]
	for _, n := range []string{"-q", "-r"} {
		if !c.HasFlag(n) {
			t.Errorf("-qr has no %s", n)
		}
	}
	if vs := c.FlagValues("-r"); len(vs) != 1 || vs[0].Value != "req.txt" {
		t.Errorf("value of -r in -qr req.txt: %v", vs)
	}
	if vs := c.FlagValues("-q"); len(vs) != 0 {
		t.Errorf("-q takes no value but has %v", vs)
	}
	if got := c.Positional; len(got) != 2 || got[0].Value != "install" || got[1].Value != "pkg" {
		t.Errorf("positional arguments %v", got)
	}
	// `sh -ec "..."`: the script is the value of -c
	c = mustAnalyze(t, `bash -ec 'echo hi'`).Commands[0]
	if !c.HasFlag("-e") || !c.HasFlag("-c") {
		t.Errorf("bash -ec has -e and -c: %v", c.Flags)
	}
}

func TestNodeInstallFlags(t *testing.T) {
	for _, tc := range []struct {
		script string
		global bool
		locked bool
	}{
		{"npm install --location global typescript", true, false},
		{"npm install --location=global typescript", true, false},
		{"npm install --location project typescript", false, false},
		{"pnpm install --prefer-frozen-lockfile=false", false, false},
		{"pnpm install --frozen-lockfile", false, true},
	} {
		in := mustAnalyze(t, tc.script).Commands[0].Installs()
		if in == nil {
			t.Errorf("%q is not an install", tc.script)
			continue
		}
		if in.Global != tc.global || in.Locked != tc.locked {
			t.Errorf("%q: global=%v locked=%v, want global=%v locked=%v", tc.script, in.Global, in.Locked, tc.global, tc.locked)
		}
		if tc.global {
			if len(in.Packages) != 1 || in.Packages[0].Name != "typescript" {
				t.Errorf("%q: packages %v, want only typescript", tc.script, in.Packages)
			}
		}
	}
}

func TestRefPinned(t *testing.T) {
	for ref, want := range map[string]bool{
		"0123abcd":       true,
		"v1.2.3":         true,
		"1.2.3-rc.1":     true,
		"v2.0":           true,
		"v1":             false, // moving major tag
		"2024-release":   false,
		"v1-nightly":     false,
		"main":           false,
		"release/v1.2.3": false,
		"1.0-nightly":    true, // looks like a version, so it is taken as one
	} {
		if have := refPinned(ref); have != want {
			t.Errorf("refPinned(%q) = %v, want %v", ref, have, want)
		}
	}
}

func TestExpressionsInsideExpansions(t *testing.T) {
	for _, script := range []string{
		`echo "x=${A:-${{ github.head_ref }}}" >> $GITHUB_ENV`,
		`echo "x=$(echo ${{ github.head_ref }})" >> $GITHUB_ENV`,
		`echo "x=$((${{ github.run_number }} + 1))" >> $GITHUB_ENV`,
		`cat <(echo ${{ github.head_ref }}) >> $GITHUB_ENV`,
	} {
		s := mustAnalyze(t, script)
		found := false
		for _, c := range s.Commands {
			for _, a := range c.Args {
				if len(a.Exprs) > 0 {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("no word of %q has its expression", script)
		}
		ws := s.WritesTo("GITHUB_ENV")
		if len(ws) != 1 || len(ws[0].Exprs) == 0 {
			t.Errorf("WritesTo of %q has no producer expression: %+v", script, ws)
		}
	}
}

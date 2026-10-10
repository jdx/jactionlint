package jactionlint

import (
	"strings"
	"testing"
)

func TestAnalyzeShellPlaceholders(t *testing.T) {
	// Each "@" in the script stands for one placeholder.
	tests := []struct {
		name   string
		script string
		want   []shPlace
	}{
		{"unquoted", "echo @", []shPlace{{Cannot: true, Split: "x"}}},
		{"double quoted", `echo "@"`, []shPlace{{Quote: shDouble}}},
		{"double quoted in text", `echo "a @ b"`, []shPlace{{Quote: shDouble}}},
		{"single quoted", `echo '@'`, []shPlace{{Quote: shSingle}}},
		{"single quote closed", `echo 'a' @`, []shPlace{{Cannot: true, Split: "x"}}},
		{"single quote inside double quotes", `echo "it's @"`, []shPlace{{Quote: shDouble}}},
		{"double quote inside single quotes", `echo 'say "@"'`, []shPlace{{Quote: shSingle}}},
		{"escaped quote in double quotes", `echo "a\" @"`, []shPlace{{Quote: shDouble}}},
		{"backslash before placeholder", `echo \@`, []shPlace{{Cannot: true}}},
		{"comment", "echo hi # @", []shPlace{{Cannot: true}}},
		{"hash inside a word is not a comment", "echo a#b @", []shPlace{{Cannot: true, Split: "x"}}},
		{"comment ends at newline", "# c\necho \"@\"", []shPlace{{Quote: shDouble}}},
		{"command substitution", `echo "$(echo @)"`, []shPlace{{Cannot: true}}},
		{"backticks", "echo `echo @`", []shPlace{{Cannot: true}}},
		{"heredoc", "cat <<EOF\n@\nEOF", []shPlace{{Cannot: true}}},
		{"here-string", "cat <<< \"@\"", []shPlace{{Cannot: true}}},
		{"process substitution", "diff <(echo @) b", []shPlace{{Cannot: true}}},
		{"ansi-c quote", "echo $'a' \"@\"", []shPlace{{Cannot: true}}},
		{"case", "case \"@\" in a) ;; esac", []shPlace{{Cannot: true}}},
		{"unterminated quote", `echo "@`, []shPlace{{Cannot: true}}},
		{"double brackets", `[[ "@" == a ]]`, []shPlace{{Quote: shDouble, Unsafe: "x"}}},
		{"double brackets closed", `[[ a == b ]]; echo "@"`, []shPlace{{Quote: shDouble}}},
		{"two placeholders", `echo "@" '@' @`, []shPlace{{Quote: shDouble}, {Quote: shSingle}, {Cannot: true, Split: "x"}}},
		{"parameter expansion is fine", `echo "${A:-@}"`, []shPlace{{Quote: shDouble}}},
		{"for list", "for f in @; do echo \"$f\"; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list after other words", "for f in a @ b; do :; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list in a block", "if x; then\n  for f in @\n  do :; done\nfi", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list quoted is fine", "for f in \"@\"; do :; done", []shPlace{{Quote: shDouble}}},
		{"for body is not the list", "for f in a b; do echo @; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"select list", "select f in @; do break; done", []shPlace{{Cannot: true}}},
		{"array assignment", "files=(a @)", []shPlace{{Cannot: true, Split: "x"}}},
		{"after an array assignment", "files=(a b); echo @", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list after a braced variable", "for f in ${A} @; do :; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list in a brace group", "{ for f in @; do :; done; }", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list after a braced variable in a group", "{ for f in ${A}; do :; done; for g in ${B} @; do :; done; }", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list with a prefix assignment", "IFS=, for f in @; do :; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list with a parameter expansion before", "for f in ${A:-x} @; do :; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list word glued to a variable", "for f in ${A}@; do :; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list over lines", "for f in a \\\n  @; do :; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"for list on a pipeline line", "echo x | while read -r l; do for f in ${l} @; do :; done; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"select list after a braced variable", "select f in ${A} @; do break; done", []shPlace{{Cannot: true}}},
		{"array assignment after braced variable", "files=(${A} @)", []shPlace{{Cannot: true, Split: "x"}}},
		{"array assignment with declare", "declare -a files=(a @)", []shPlace{{Cannot: true, Split: "x"}}},
		{"array append", "files+=(@)", []shPlace{{Cannot: true, Split: "x"}}},
		{"array assignment over lines", "files=(\n  a\n  @\n)", []shPlace{{Cannot: true, Split: "x"}}},
		{"for body after braced list is fine", "for f in ${A}; do echo @; done", []shPlace{{Cannot: true, Split: "x"}}},
		{"assignment value", "A=@", []shPlace{{Quote: shUnquoted, Unsafe: "x"}}},
		{"assignment value after text", "A=pre@", []shPlace{{Quote: shUnquoted, Unsafe: "x"}}},
		{"assignment value after another assignment", "A=1 B=@ cmd", []shPlace{{Quote: shUnquoted, Unsafe: "x"}}},
		{"assignment value after an assignment with a placeholder", "A=@ B=@ cmd", []shPlace{{Quote: shUnquoted, Unsafe: "x"}, {Quote: shUnquoted, Unsafe: "x"}}},
		{"assignment value after a quoted assignment", "A=\"x\" B=@ cmd", []shPlace{{Quote: shUnquoted, Unsafe: "x"}}},
		{"argument after an assignment with a placeholder", "A=@ echo @", []shPlace{{Quote: shUnquoted, Unsafe: "x"}, {Cannot: true, Split: "x"}}},
		{"redirection target that looks like an assignment", "A=1 > FILE=@", []shPlace{{Cannot: true, Split: "x"}}},
		{"input redirection target that looks like an assignment", "cat < FILE=@", []shPlace{{Cannot: true, Split: "x"}}},
		{"assignment after a redirection target", "echo > out B=@", []shPlace{{Cannot: true, Split: "x"}}},
		{"assignment-looking argument after a braced variable", "echo ${A} B=@", []shPlace{{Cannot: true, Split: "x"}}},
		{"assignment after a brace group", "{ echo a; }; B=@", []shPlace{{Quote: shUnquoted, Unsafe: "x"}}},
		{"assignment prefix of a command is not a single word argument", "A=1 echo @", []shPlace{{Cannot: true, Split: "x"}}},
		{"argument that looks like an assignment", "echo A=@", []shPlace{{Cannot: true, Split: "x"}}},
		{"export is not a plain assignment", "export A=@", []shPlace{{Cannot: true, Split: "x"}}},
		{"redirection target", "echo hi > @", []shPlace{{Cannot: true, Split: "x"}}},
		{"line continuation", "echo \\\n  \"@\"", []shPlace{{Quote: shDouble}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var spans []shSpan
			var b strings.Builder
			for i := 0; i < len(tc.script); i++ {
				if tc.script[i] == '@' {
					start := b.Len()
					b.WriteString("${{ x }}")
					spans = append(spans, shSpan{start, b.Len()})
					continue
				}
				b.WriteByte(tc.script[i])
			}
			got := analyzeShellPlaceholders(b.String(), spans)
			if len(got) != len(tc.want) {
				t.Fatalf("want %d places but got %d", len(tc.want), len(got))
			}
			for i, w := range tc.want {
				g := got[i]
				if g.Cannot != w.Cannot {
					t.Errorf("place %d: Cannot want %v got %v", i, w.Cannot, g.Cannot)
				}
				if (g.Split != "") != (w.Split != "") {
					t.Errorf("place %d: Split want %q got %q", i, w.Split, g.Split)
				}
				if w.Cannot {
					continue
				}
				if g.Quote != w.Quote {
					t.Errorf("place %d: Quote want %v got %v", i, w.Quote, g.Quote)
				}
				if (g.Unsafe != "") != (w.Unsafe != "") {
					t.Errorf("place %d: Unsafe want %q got %q", i, w.Unsafe, g.Unsafe)
				}
			}
		})
	}
}

func FuzzAnalyzeShellPlaceholders(f *testing.F) {
	for _, s := range []string{`echo "a" '${{ x }}' ${{ y }}`, "cat <<EOF\n${{ x }}\nEOF", "echo $(echo ${{ x }})", `echo "\${{ x }}"`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, script string) {
		var spans []shSpan
		pos := 0
		for {
			i := strings.Index(script[pos:], "${{")
			if i < 0 {
				break
			}
			start := pos + i
			j := strings.Index(script[start:], "}}")
			if j < 0 {
				break
			}
			spans = append(spans, shSpan{start, start + j + 2})
			pos = start + j + 2
		}
		got := analyzeShellPlaceholders(script, spans)
		if len(got) != len(spans) {
			t.Fatalf("want %d places but got %d", len(spans), len(got))
		}
		for i, g := range got {
			if !g.Cannot && g.Quote == shUnquoted && g.Unsafe == "" {
				t.Errorf("place %d is unquoted but neither unsafe nor unknown: %q", i, script)
			}
		}
	})
}

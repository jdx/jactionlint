package runscript

import (
	"strings"
	"testing"
)

func TestSyntaxError(t *testing.T) {
	reported := []struct {
		name, script, want string
		offset             int
	}{
		{"unclosed double quote", "echo \"a\n", "reached EOF without closing quote", 5},
		{"unclosed single quote", "echo 'a\n", "reached EOF without closing quote", 5},
		{"if without fi", "echo a\nif true; then\n  echo b\n", "`if` statement must end with `fi`", 7},
		{"stray fi", "echo a\nfi\n", "`fi` can only be used to end an `if`", 7},
		{"then outside an if", "then\n", "`then` can only be used in an `if`", 0},
		{"do outside a loop", "do\n", "`do` can only be used in a loop", 0},
		{"stray done", "done\n", "`done` can only be used to end a loop", 0},
		{"for without done", "for i in 1 2; do\n  echo $i\n", "`for` statement must end with `done`", 0},
		{"case without esac", "case $x in\n  a) echo a ;;\n", "`case` statement must end with `esac`", 0},
		{"then without statement", "if true; then\nfi\n", "`then` must be followed by a statement list", 9},
		{"unclosed command substitution", "x=$(echo a\n", "reached EOF without matching `$(` with `)`", 2},
		{"unclosed subshell", "(echo a\n", "reached EOF without matching `(` with `)`", 0},
		{"unclosed brace group", "{ echo a\n", "reached EOF without matching `{` with `}`", 0},
		{"unclosed test", "[[ -n $x\n", "reached EOF without matching `[[` with `]]`", 0},
		{"pipe without command", "echo a |\n", "`|` must be followed by a statement", 7},
		{"redirect without word", "echo a >\n", "`>` must be followed by a word", 7},
		{"before a here-document", "if true; then\ncat <<EOF\nx\nEOF\n", "`if` statement must end with `fi`", 0},
	}
	for _, tc := range reported {
		t.Run(tc.name, func(t *testing.T) {
			p := SyntaxError(tc.script)
			if p == nil {
				t.Fatalf("want an error for %q", tc.script)
			}
			if !strings.Contains(p.Text, tc.want) {
				t.Errorf("text = %q, want %q", p.Text, tc.want)
			}
			if p.Offset != tc.offset {
				t.Errorf("offset = %d, want %d", p.Offset, tc.offset)
			}
		})
	}

	// Scripts which bash accepts, or whose problem it only finds when it runs the command, or which the parser cannot
	// read the way bash does
	silent := []struct{ name, script string }{
		{"valid script", "set -eo pipefail\nf() { echo \"$1\"; }\nf a | while read -r l; do echo \"$l\"; done\n"},
		{"double bracket", "[[ $a == b* && $x =~ ^(a|b)+$ ]] && echo ok\n"},
		{"process substitution", "diff <(sort a) <(sort b)\ntee >(cat) < /dev/null\n"},
		{"arrays", "arr=(a b)\ndeclare -A m=([k]=v)\necho \"${arr[@]}\" \"${#arr[@]}\" \"${m[k]}\"\n"},
		{"here-string", "read -r a b <<< \"x y\"\n"},
		{"case fallthrough", "case $x in\n  a) echo a ;;&\n  b) echo b ;&\n  *) : ;;\nesac\n"},
		{"function with keyword", "function f { echo a; }\nf\n"},
		{"here-document", "cat <<EOF\nit's \"$(not closed\nEOF\n"},
		{"here-document with a dash", "if true; then\n\tcat <<-EOF\n\tx\n\tEOF\nfi\n"},
		{"here-document that is not closed (a warning for bash)", "cat <<EOF\nbody\n"},
		{"error in a here-document body", "cat <<EOF\n${x<}\nEOF\n"},
		{"bad substitution", "echo ${x<}\n"},
		{"bad substitution in quotes", "echo \"${|X}/a\"\n"},
		{"bad index", "echo \"${a[0}\"\n"},
		{"bad offset", "echo ${a:(-4} x\n"},
		{"arithmetic", "echo $(( 1 + ))\n"},
		{"arithmetic with a missing operator", "echo $(( ($(date +%s)  $(date +%s)) / 86400 ))\n"},
		{"error after a backquote", "x=`date`\nif true; then\n"},
		{"error in a backquote", "x=`if true; then`\n"},
		{"error after arithmetic", "(( i++ ))\nif true; then\n"},
		{"error after a here-document", "cat <<EOF\nx\nEOF\nif true; then\n"},
		{"negation after a redirection", "[ ! -f a ] && <[ ! -f b ]\n"},
		{"operator of a test without its operand", "[[ a =~&& b ]]\n"},
		{"a function name that is not a name", "a[b]\n"},
		{"fi inside a string", "echo \"fi\"\n"},
	}
	for _, tc := range silent {
		t.Run(tc.name, func(t *testing.T) {
			if p := SyntaxError(tc.script); p != nil {
				t.Fatalf("want no error for %q, got %+v", tc.script, p)
			}
		})
	}
}

// The parser must not panic or hang on odd input.
func TestSyntaxErrorOddInput(t *testing.T) {
	for _, s := range []string{"", "\x00", "\xff\xfe", strings.Repeat("(", 5000), strings.Repeat("$(", 5000), strings.Repeat("`", 3), "<<", "<<<", "${", "$(("} {
		SyntaxError(s)
	}
}

package jactionlint

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

func commentSummary(src string) []string {
	var ret []string
	for _, c := range NewCommentIndex([]byte(src)).All() {
		kind := "own"
		if c.Inline {
			kind = "inline"
		}
		ret = append(ret, fmt.Sprintf("%d:%d:%s:%s", c.Line, c.Column, kind, c.Text))
	}
	return ret
}

func TestCommentIndexPlacement(t *testing.T) {
	tests := []struct {
		what string
		src  string
		want []string
	}{
		{"no comment", "a: 1\nb: 2\n", nil},
		{"above a key", "# above\na: 1\n", []string{"1:1:own:above"}},
		{"indented above a key", "a:\n  # above b\n  b: 1\n", []string{"2:3:own:above b"}},
		{"trailing", "a: 1 # trailing\n", []string{"1:6:inline:trailing"}},
		{"trailing after key only", "a: # trailing\n  b: 1\n", []string{"1:4:inline:trailing"}},
		{"trailing after sequence dash", "- # c\n  a: 1\n", []string{"1:3:inline:c"}},
		{"hash without space is content", "a: b#c\nd: e #f\n", []string{"2:6:inline:f"}},
		{"hash starting a plain scalar needs a space", "a: #c\n", []string{"1:4:inline:c"}},
		{"between sequence items", "s:\n  - a\n  # between\n  - b\n", []string{"3:3:own:between"}},
		{"after a sequence item", "s:\n  - a # one\n  - b # two\n", []string{"2:7:inline:one", "3:7:inline:two"}},
		{"flow mapping", "e: {A: 1, # in flow\n  B: 2} # after\n", []string{"1:11:inline:in flow", "2:9:inline:after"}},
		{"comment line inside a flow sequence", "e: [a,\n  # own line\n  b]\n", []string{"2:3:own:own line"}},
		{"hash in flow is content", "e: {A: x#y, B: 'z # w'}\n", nil},
		{"anchor", "a: &anc # on anchor\n  b: 1\n", []string{"1:9:inline:on anchor"}},
		{"anchor on a sequence item", "- &x # c\n  a: 1\n", []string{"1:6:inline:c"}},
		{"tag", "a: !!str # c\n", []string{"1:10:inline:c"}},
		{"double quoted", "a: \"x # y\" # z\n", []string{"1:12:inline:z"}},
		{"single quoted with escape", "a: 'it''s # not' # yes\n", []string{"1:18:inline:yes"}},
		{"double quoted with escape", "a: \"q\\\" # not\" # yes\n", []string{"1:16:inline:yes"}},
		{"multi-line quoted", "a: \"one\n  # not a comment\n  two\" # yes\n", []string{"3:8:inline:yes"}},
		{"quote inside a plain scalar", "a: say \"hi # there\"\n", []string{"1:12:inline:there\""}},
		{"literal block", "r: |\n  echo # no\n  # no\nb: 1 # yes\n", []string{"4:6:inline:yes"}},
		{"literal block header comment", "r: | # header\n  echo # no\n", []string{"1:6:inline:header"}},
		{"literal block with indicators", "r: |-2 # h\n    x # no\n", []string{"1:8:inline:h"}},
		{"folded block in sequence", "- run: >\n    x # no\n  # after\n", []string{"3:3:own:after"}},
		{"block scalar item", "- |\n  x # no\n# after\n", []string{"3:1:own:after"}},
		{"blank lines in block scalar", "r: |\n  a\n\n  # inside\n  b\n# out\n", []string{"6:1:own:out"}},
		{"document start", "--- # doc\na: 1\n", []string{"1:5:inline:doc"}},
		{"crlf", "# a\r\nb: 1 # c\r\n", []string{"1:1:own:a", "2:6:inline:c"}},
		{"unicode column", "k: \"é\" # c\n", []string{"1:8:inline:c"}},
		{"empty comment", "#\na: 1 #\n", []string{"1:1:own:", "2:6:inline:"}},
		{"comment at end of file without newline", "a: 1\n# end", []string{"2:1:own:end"}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			have := commentSummary(tc.src)
			if !reflect.DeepEqual(have, tc.want) {
				t.Fatalf("source:\n%s\nwant: %q\nhave: %q", tc.src, tc.want, have)
			}
		})
	}
}

func TestCommentIndexLookups(t *testing.T) {
	src := `# detached

# head 1
# head 2
permissions:
  contents: read # why
  # above issues
  issues: write
  # foot

jobs: {}
`
	idx := NewCommentIndex([]byte(src))

	texts := func(cs []Comment) string {
		var s []string
		for _, c := range cs {
			s = append(s, c.Text)
		}
		return strings.Join(s, "|")
	}

	if have := texts(idx.Before(5)); have != "head 1|head 2" {
		t.Errorf("Before(permissions) = %q", have)
	}
	if have := texts(idx.Before(3)); have != "" {
		t.Errorf("a blank line must end the block, Before(3) = %q", have)
	}
	if have := texts(idx.Before(1)); have != "" {
		t.Errorf("Before(1) = %q", have)
	}
	if c := idx.Inline(6); c == nil || c.Text != "why" {
		t.Errorf("Inline(6) = %v", c)
	}
	if c := idx.Inline(8); c != nil {
		t.Errorf("Inline(8) = %v", c)
	}
	if have := texts(idx.Before(6)); have != "" {
		t.Errorf("an own-line block above contents is missing, Before(6) = %q", have)
	}
	if have := texts(idx.Before(8)); have != "above issues" {
		t.Errorf("Before(8) = %q", have)
	}
	if have := texts(idx.After(8)); have != "foot" {
		t.Errorf("After(8) = %q", have)
	}
	if have := texts(idx.After(6)); have != "above issues" {
		t.Errorf("After(6) = %q", have)
	}
	if have := texts(idx.After(9)); have != "" {
		t.Errorf("After(9) = %q", have)
	}
	if !idx.Documented(5) || !idx.Documented(6) || !idx.Documented(8) || idx.Documented(11) {
		t.Errorf("Documented: 5=%v 6=%v 8=%v 11=%v", idx.Documented(5), idx.Documented(6), idx.Documented(8), idx.Documented(11))
	}
	// An inline comment of the previous line does not document the next line
	if idx.Documented(7) {
		t.Error("a comment line documents the line below it, not itself")
	}
	if c := idx.At(7); c == nil || c.Inline {
		t.Errorf("At(7) = %v", c)
	}
	if idx.At(0) != nil || idx.At(1000) != nil {
		t.Error("At must be nil out of range")
	}

	var nilIdx *CommentIndex
	if nilIdx.All() != nil || nilIdx.At(1) != nil || nilIdx.Inline(1) != nil || nilIdx.Before(2) != nil || nilIdx.After(1) != nil || nilIdx.Documented(1) {
		t.Error("nil index must have no comments")
	}
}

func TestParseKeepsComments(t *testing.T) {
	src := `on: push
# the permissions
permissions:
  contents: read # needed to checkout
  issues: write
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # check out
      - uses: actions/checkout@v4 # pinned later
`
	w, errs := Parse([]byte(src))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if w.Comments == nil {
		t.Fatal("Comments is nil")
	}
	p := w.Permissions
	if !w.Comments.Documented(p.Pos.Line) {
		t.Errorf("permissions at line %d is not documented", p.Pos.Line)
	}
	var documented []string
	for name, s := range p.Scopes {
		if w.Comments.Documented(s.Name.Pos.Line) {
			documented = append(documented, name)
		}
	}
	if !reflect.DeepEqual(documented, []string{"contents"}) {
		t.Errorf("documented scopes: %v", documented)
	}
	step := w.Jobs["test"].Steps[0]
	if !w.Comments.Documented(step.Pos.Line) {
		t.Errorf("step at line %d is not documented", step.Pos.Line)
	}
	if c := w.Comments.Inline(step.Exec.(*ExecAction).Uses.Pos.Line); c == nil || c.Text != "pinned later" {
		t.Errorf("uses comment: %v", c)
	}
}

// The YAML library attaches comments to nodes. Both implementations must find the same comments in
// the real files, which is the best check of the scanner we have.
func TestCommentIndexAgreesWithYAMLLibrary(t *testing.T) {
	var files []string
	for _, pat := range []string{"testdata/ok/*.yaml", "testdata/err/*.yaml", "testdata/examples/*.yaml", "testdata/projects/*/.github/workflows/*.y*ml"} {
		m, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) < 10 {
		t.Fatalf("too few files: %v", files)
	}

	checked := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var n yaml.Node
		if yaml.Unmarshal(b, &n) != nil {
			continue // not valid YAML
		}
		want := map[string]int{}
		var walk func(*yaml.Node)
		walk = func(n *yaml.Node) {
			for _, c := range []string{n.HeadComment, n.LineComment, n.FootComment} {
				for _, l := range strings.Split(c, "\n") {
					if l = strings.TrimSpace(l); strings.HasPrefix(l, "#") {
						want[strings.TrimSpace(l[1:])]++
					}
				}
			}
			for _, c := range n.Content {
				walk(c)
			}
		}
		walk(&n)
		have := map[string]int{}
		for _, c := range NewCommentIndex(b).All() {
			have[c.Text]++
		}
		// The library drops comments in some positions, so only what it found must be found by the scanner
		for text, cnt := range want {
			if have[text] < cnt {
				t.Errorf("%s: the YAML library found comment %q %d times but the scanner %d times", f, text, cnt, have[text])
			}
		}
		checked++
	}
	if checked < 10 {
		t.Fatalf("only %d files were compared", checked)
	}
}

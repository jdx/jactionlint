package jactionlint

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestYAMLSiteInsert(t *testing.T) {
	tests := []struct {
		name string
		site YAMLSite
		in   string
		want string
		ok   bool
	}{
		{"plain", YAMLSite{Context: YAMLPlain}, `"${X}"`, `"${X}"`, true},
		{"plain with a comment start", YAMLSite{Context: YAMLPlain}, `a #b`, "", false},
		{"plain with a mapping indicator", YAMLSite{Context: YAMLPlain}, `a: b`, "", false},
		{"plain ending with a colon", YAMLSite{Context: YAMLPlain}, `a:`, "", false},
		{"plain with a hash inside a word", YAMLSite{Context: YAMLPlain}, `a#b`, `a#b`, true},
		{"plain at the start with an indicator", YAMLSite{Context: YAMLPlain, AtStart: true}, `"${X}"`, "", false},
		{"plain at the start with a dollar", YAMLSite{Context: YAMLPlain, AtStart: true}, `$X`, `$X`, true},
		{"flow plain with a comma", YAMLSite{Context: YAMLFlowPlain}, `a,b`, "", false},
		{"flow plain", YAMLSite{Context: YAMLFlowPlain}, `a b`, `a b`, true},
		{"double quoted", YAMLSite{Context: YAMLDoubleQuoted}, `say "hi" \o/`, `say \"hi\" \\o/`, true},
		{"double quoted control", YAMLSite{Context: YAMLDoubleQuoted}, "a\nb\tc\x01", `a\nb\tc\x01`, true},
		{"single quoted", YAMLSite{Context: YAMLSingleQuoted}, `it's "x"`, `it''s "x"`, true},
		{"single quoted newline", YAMLSite{Context: YAMLSingleQuoted}, "a\nb", "", false},
		{"block scalar", YAMLSite{Context: YAMLBlockScalar, Indent: 4}, "a\nb", "a\n    b", true},
		{"block scalar one line", YAMLSite{Context: YAMLBlockScalar, Indent: 4}, `echo "$X"`, `echo "$X"`, true},
		{"invalid UTF-8", YAMLSite{Context: YAMLDoubleQuoted}, "\xff", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.site.Insert(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Errorf("want %q (%v) but got %q (%v)", tc.want, tc.ok, got, ok)
			}
		})
	}
}

// Whatever Insert returns must read back as the inserted text when it is put into a scalar of the context.
func TestYAMLSiteInsertRoundTrip(t *testing.T) {
	texts := []string{`x`, `"`, `'`, `\`, `a: b`, ` # c`, `it's "q" \n`, "tab\there", `${{ a == 'b: c' }}`, `{a}`, `[b]`, `é日本`, `%`, `@`}
	frames := []struct {
		site          YAMLSite
		before, after string
	}{
		{YAMLSite{Context: YAMLPlain}, "k: pre ", " post\n"},
		{YAMLSite{Context: YAMLFlowPlain}, "k: [pre ", " post]\n"},
		{YAMLSite{Context: YAMLSingleQuoted}, "k: 'pre ", " post'\n"},
		{YAMLSite{Context: YAMLDoubleQuoted}, `k: "pre `, " post\"\n"},
		{YAMLSite{Context: YAMLBlockScalar, Indent: 2}, "k: |\n  pre ", " post\n"},
	}
	for _, f := range frames {
		for _, text := range texts {
			ins, ok := f.site.Insert(text)
			if !ok {
				continue
			}
			var m map[string]any
			src := f.before + ins + f.after
			if err := yaml.Unmarshal([]byte(src), &m); err != nil {
				t.Errorf("%v %q: %q does not parse: %v", f.site.Context, text, src, err)
				continue
			}
			var got string
			switch v := m["k"].(type) {
			case string:
				got = v
			case []any:
				got, _ = v[0].(string)
			}
			want := "pre " + text + " post"
			if f.site.Context == YAMLBlockScalar {
				want += "\n"
			}
			if got != want {
				t.Errorf("%v %q: %q reads back as %q, not %q", f.site.Context, text, src, got, want)
			}
		}
	}
}

func TestRenderYAMLValue(t *testing.T) {
	tests := map[string]string{
		"build":             "build",
		"Build and test":    "Build and test",
		"true":              `"true"`,
		"On":                `"On"`,
		"null":              `"null"`,
		"~":                 `"~"`,
		"123":               `"123"`,
		"1.0":               `"1.0"`,
		"0x1f":              `"0x1f"`,
		"":                  `""`,
		" lead":             `" lead"`,
		"trail ":            `"trail "`,
		"a: b":              `"a: b"`,
		"a #b":              `"a #b"`,
		"# c":               `"# c"`,
		"- x":               `"- x"`,
		"*alias":            `"*alias"`,
		"${{ github.ref }}": "${{ github.ref }}",
		"${{ f('a: b') }}":  `"${{ f('a: b') }}"`,
		"say \"hi\"":        `say "hi"`,
		"line\nbreak":       `"line\nbreak"`,
		"{x}":               `"{x}"`,
		"é":                 "é",
	}
	for in, want := range tests {
		got := RenderYAMLValue(in)
		if got != want {
			t.Errorf("RenderYAMLValue(%q) = %s, want %s", in, got, want)
		}
		// It reads back as the same string
		var m map[string]string
		if err := yaml.Unmarshal([]byte("k: "+got+"\n"), &m); err != nil || m["k"] != in {
			t.Errorf("RenderYAMLValue(%q) = %s reads back as %q (%v)", in, got, m["k"], err)
		}
	}
}

func TestYAMLSiteAt(t *testing.T) {
	src := "a: plain value\n" + // 0
		"b: 'single'\n" +
		"c: \"double\"\n" +
		"d: |\n    block\n    text\n" +
		"e: [x, y]\n" +
		"f:\n  - g: h\n"
	at := func(sub string, nth int) int {
		off := 0
		for i := 0; i <= nth; i++ {
			j := strings.Index(src[off:], sub)
			if j < 0 {
				t.Fatalf("%q not found", sub)
			}
			off += j
			if i < nth {
				off++
			}
		}
		return off
	}
	tests := []struct {
		name string
		off  int
		want YAMLSite
		ok   bool
	}{
		{"plain", at("value", 0), YAMLSite{Context: YAMLPlain}, true},
		{"plain start", at("plain", 0), YAMLSite{Context: YAMLPlain, AtStart: true}, true},
		{"single", at("ingle", 0), YAMLSite{Context: YAMLSingleQuoted}, true},
		{"double", at("ouble", 0), YAMLSite{Context: YAMLDoubleQuoted}, true},
		{"block", at("text", 0), YAMLSite{Context: YAMLBlockScalar, Indent: 4}, true},
		{"flow", at("y]", 0), YAMLSite{Context: YAMLFlowPlain, AtStart: true}, true},
		{"nested", at("h\n", 0), YAMLSite{Context: YAMLPlain, AtStart: true}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := YAMLSiteAt([]byte(src), tc.off)
			if ok != tc.ok || got != tc.want {
				t.Errorf("want %+v (%v) but got %+v (%v)", tc.want, tc.ok, got, ok)
			}
		})
	}
	if _, ok := YAMLSiteAt([]byte("a: [unclosed"), 4); ok {
		t.Error("a source which does not parse has no site")
	}
	if _, ok := YAMLSiteAt([]byte(src), 1000); ok {
		t.Error("an offset outside the source has no site")
	}
}

// The lines of a text inserted into a block scalar of a CRLF file end with CRLF too.
func TestYAMLSiteInsertKeepsCRLF(t *testing.T) {
	src := []byte("a: |\r\n  one\r\n  two\r\n")
	site, ok := YAMLSiteAt(src, strings.Index(string(src), "one"))
	if !ok || site.Context != YAMLBlockScalar || !site.CRLF {
		t.Fatalf("site: %+v %v", site, ok)
	}
	if got, ok := site.Insert("x\ny"); !ok || got != "x\r\n  y" {
		t.Errorf("got %q %v", got, ok)
	}
	lf := []byte("a: |\n  one\n")
	site, _ = YAMLSiteAt(lf, strings.Index(string(lf), "one"))
	if got, _ := site.Insert("x\ny"); got != "x\n  y" {
		t.Errorf("LF file: %q", got)
	}
}

// A text that starts with "#" can follow a space and then starts a comment.
func TestYAMLSiteInsertRefusesALeadingHash(t *testing.T) {
	src := []byte("a: x y\n")
	site, ok := YAMLSiteAt(src, strings.Index(string(src), "y"))
	if !ok || site.Context != YAMLPlain {
		t.Fatalf("site: %+v %v", site, ok)
	}
	if got, ok := site.Insert("#z"); ok {
		t.Errorf("a leading # must be refused: %q", got)
	}
	if got, ok := site.Insert("z#"); !ok || got != "z#" {
		t.Errorf("a # inside is fine: %q %v", got, ok)
	}
}

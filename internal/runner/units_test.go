package runner

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseCommand(t *testing.T) {
	for _, tc := range []struct {
		line string
		want command
		ok   bool
	}{
		{"::error::boom", command{Name: "error", Props: map[string]string{}, Message: "boom"}, true},
		{"::warning file=a.go,line=3,col=4,title=T::msg", command{Name: "warning", Props: map[string]string{"file": "a.go", "line": "3", "col": "4", "title": "T"}, Message: "msg"}, true},
		{"::ERROR::case", command{Name: "error", Props: map[string]string{}, Message: "case"}, true},
		{"::set-output name=x::a%0Ab%25", command{Name: "set-output", Props: map[string]string{"name": "x"}, Message: "a\nb%"}, true},
		{"::notice file=a%2Cb%3Ac::m", command{Name: "notice", Props: map[string]string{"file": "a,b:c"}, Message: "m"}, true},
		{"::endgroup::", command{Name: "endgroup", Props: map[string]string{}}, true},
		{"::group::Title with spaces", command{Name: "group", Props: map[string]string{}, Message: "Title with spaces"}, true},
		{"plain text", command{}, false},
		{"::no-end", command{}, false},
		{":: ::x", command{}, false},
		{"  ::error::indented", command{}, false},
	} {
		got, ok := parseCommand(tc.line)
		if ok != tc.ok || (ok && !reflect.DeepEqual(got, tc.want)) {
			t.Errorf("%q: got %+v, %v; want %+v, %v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParseKeyValues(t *testing.T) {
	keys, vals, err := parseKeyValues("A=1\nB=two=2\nC<<EOF\nline1\n\nline3\nEOF\nA=override\nD=\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(keys, ",") != "A,B,C,D" || vals["A"] != "override" || vals["B"] != "two=2" || vals["C"] != "line1\n\nline3" || vals["D"] != "" {
		t.Fatalf("%v %v", keys, vals)
	}
	for _, bad := range []string{"novalue", "=x", "K<<EOF\nnever closed", "K<<\nx\n", "<<EOF\nx\nEOF"} {
		if _, _, err := parseKeyValues(bad); err == nil {
			t.Errorf("%q must fail", bad)
		}
	}
	_, vals, _ = parseKeyValues("K<<EOF\r\nwin\r\nEOF\r\n")
	if vals["K"] != "win" {
		t.Errorf("CRLF: %q", vals["K"])
	}
}

func TestLineSink(t *testing.T) {
	var lines []string
	s := &lineSink{handle: func(l string) { lines = append(lines, l) }}
	s.Write([]byte("one\ntw"))
	s.Write([]byte("o\r\nthree"))
	s.Close()
	if strings.Join(lines, "|") != "one|two|three" {
		t.Fatalf("%q", lines)
	}
}

func TestMasker(t *testing.T) {
	var m Masker
	m.Add("hunter2")
	m.Add("multi\nline-secret\n")
	m.Add("  ")
	m.Add("")
	m.Add("hunter2-long")
	for in, want := range map[string]string{
		"pw hunter2 ok":        "pw *** ok",
		"hunter2-long":         "***",
		"multi":                "***",
		"a line-secret b":      "a *** b",
		"nothing":              "nothing",
		"hunter2 hunter2-long": "*** ***",
	} {
		if got := m.Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveShell(t *testing.T) {
	has := func(names ...string) func(string) bool {
		return func(n string) bool {
			for _, x := range names {
				if x == n {
					return true
				}
			}
			return false
		}
	}
	for _, tc := range []struct {
		shell  string
		have   func(string) bool
		want   string
		wantEr bool
	}{
		{"", has("bash"), "bash --noprofile --norc -eo pipefail S", false},
		{"", has(), "sh -e S", false},
		{"bash", has(), "bash --noprofile --norc -eo pipefail S", false},
		{"sh", has(), "sh -e S", false},
		{"python", has(), "python S", false},
		{"pwsh", has(), "pwsh -command . 'S'", false},
		{"bash {0}", has(), "bash S", false},
		{"perl -w {0}", has(), "perl -w S", false},
		{"zsh", has(), "", true},
		{"bash -x", has(), "", true},
	} {
		spec, err := resolveShell(tc.shell, tc.have)
		if (err != nil) != tc.wantEr {
			t.Errorf("%q: err = %v", tc.shell, err)
			continue
		}
		if err == nil {
			if got := strings.Join(spec.command("S"), " "); got != tc.want {
				t.Errorf("%q: %q, want %q", tc.shell, got, tc.want)
			}
		}
	}
}

func TestTargetOS(t *testing.T) {
	for in, want := range map[string]string{
		"ubuntu-latest": "linux", "ubuntu-24.04-arm": "linux", "macos-14": "darwin", "macos-latest": "darwin", "windows-2022": "windows",
		"self-hosted": "", "my-runner": "", "": "",
	} {
		if got := labelOS(in); got != want {
			t.Errorf("labelOS(%q) = %q, want %q", in, got, want)
		}
	}
	if got := targetOS([]string{"self-hosted", "macOS", "ARM64"}); got != "darwin" {
		t.Errorf("targetOS(self-hosted macOS) = %q", got)
	}
}

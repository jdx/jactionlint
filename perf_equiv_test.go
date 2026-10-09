package jactionlint

import (
	"bufio"
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

// The faster lookups must answer like the straightforward code they replaced. These tests keep the
// straightforward code as the reference.

func TestEditSetMatchesPairwiseConflicts(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 300; round++ {
		var set editSet
		var kept []TextEdit
		for i := 0; i < 40; i++ {
			s := rng.Intn(30)
			e := s
			if rng.Intn(3) > 0 {
				e = s + rng.Intn(6)
			}
			edit := TextEdit{Start: s, End: e, NewText: string(rune('a' + rng.Intn(2)))}
			want := false
			for _, k := range kept {
				if editsConflict(k, edit) {
					want = true
					break
				}
			}
			if got := set.conflicts(edit); got != want {
				t.Fatalf("round %d: conflicts(%+v) = %v, want %v with %+v", round, edit, got, want, kept)
			}
			if !want {
				set.add(edit)
				if !slicesContainsEdit(kept, edit) {
					kept = append(kept, edit)
				}
			}
		}
	}
}

func slicesContainsEdit(edits []TextEdit, e TextEdit) bool {
	for _, x := range edits {
		if x == e {
			return true
		}
	}
	return false
}

func referenceGetLine(e *Error, source []byte) (string, bool) {
	s := bufio.NewScanner(bytes.NewReader(source))
	l := 0
	for s.Scan() {
		l++
		if l == e.Line {
			return s.Text(), true
		}
	}
	return "", false
}

func referenceOffsetPosition(src []byte, off int) (line, col int) {
	line, col = 1, 1
	for i := 0; i < off; {
		r, w := utf8.DecodeRune(src[i:])
		switch {
		case r == '\n':
			line++
			col = 1
		case r == '\r' && i+1 < len(src) && src[i+1] == '\n':
		default:
			col++
		}
		i += w
	}
	return line, col
}

func TestLineLookupsMatchScanning(t *testing.T) {
	sources := []string{
		"", "\n", "a", "a\n", "a\nb", "a\nb\n", "\n\n", "a\r\nb\r\n", "a\r\n\r\nb", "é\n日本語\n😀 x\n", "x\r", "a\n\nb\n\n",
	}
	for _, src := range sources {
		b := []byte(src)
		for line := 0; line <= strings.Count(src, "\n")+3; line++ {
			e := &Error{Line: line}
			got, gok := e.getLine(b)
			want, wok := referenceGetLine(e, b)
			if got != want || gok != wok {
				t.Errorf("getLine(%q, %d) = %q, %v; want %q, %v", src, line, got, gok, want, wok)
			}
		}
		for off := 0; off <= len(b); off++ {
			if off > 0 && off < len(b) && !utf8.RuneStart(b[off]) {
				continue
			}
			gl, gc := offsetPosition(b, off)
			wl, wc := referenceOffsetPosition(b, off)
			if gl != wl || gc != wc {
				t.Errorf("offsetPosition(%q, %d) = %d:%d; want %d:%d", src, off, gl, gc, wl, wc)
			}
		}
	}
}

// A different source of the same length must not be answered from the cache of the previous one.
func TestLineStartsCacheKeysOnTheSource(t *testing.T) {
	a := []byte("a\nb\nc\n")
	b := []byte("aa\nbb\n")
	for i := 0; i < 2; i++ {
		if got, _ := (&Error{Line: 2}).getLine(a); got != "b" {
			t.Fatalf("a: %q", got)
		}
		if got, _ := (&Error{Line: 2}).getLine(b); got != "bb" {
			t.Fatalf("b: %q", got)
		}
	}
}

// Every offset of documents of all kinds finds the same scalar through the bisecting index and through
// the scan of all scalars.
func TestYAMLSiteIndexFindsTheScalarOfTheScan(t *testing.T) {
	docs := []string{
		"a: b\nc: [d, e, {f: g}]\n",
		"run: |\n  echo ${{ x }}\n  y\nname: 'q'\nother: \"z\"\n",
		"- &a x\n- *a\n- ? k\n  : v\n- >-\n  folded\n  text\n",
		"---\na: 1\n---\nb: 2\n",
		"a:\n  - b: c\n    d: e # comment\n  - f\n",
		"k: multi\n  line plain\nj: 'single\n  quoted'\n",
		"{a: b, c: [d]}\n",
		"x: !!str tagged\ny: &anc anchored\n",
	}
	for _, src := range docs {
		b := []byte(src)
		x := newYAMLSiteIndex(b)
		pd, err := parseYAMLDocs(b)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		tab := newNodeTable(b, pd)
		for off := 0; off <= len(b); off++ {
			var best *yaml.Node
			for _, d := range pd {
				tab.eachScalar(d, func(n *yaml.Node) {
					if s, e := tab.extent(n, n); off >= s && off < e {
						best = n
					}
				})
			}
			_, got := x.at(off)
			if got != (best != nil) {
				t.Errorf("%q offset %d: index found=%v, scan found=%v", src, off, got, best != nil)
			}
		}
	}
}

func TestFindRange(t *testing.T) {
	ranges := [][2]int{{2, 4}, {5, 5}, {7, 9}}
	at := func(i int) (int, int) { return ranges[i][0], ranges[i][1] }
	if !rangesOrdered(len(ranges), at) {
		t.Fatal("ranges are ordered")
	}
	for line := 0; line < 12; line++ {
		fast := findRange(len(ranges), true, at, line)
		slow := findRange(len(ranges), false, at, line)
		if len(fast) != len(slow) || (len(fast) == 1 && fast[0] != slow[0]) {
			t.Errorf("line %d: bisect %v, scan %v", line, fast, slow)
		}
	}
	overlap := [][2]int{{1, 5}, {3, 8}}
	if rangesOrdered(2, func(i int) (int, int) { return overlap[i][0], overlap[i][1] }) {
		t.Error("overlapping ranges are not ordered")
	}
	if got := findRange(2, false, func(i int) (int, int) { return overlap[i][0], overlap[i][1] }, 4); len(got) != 2 {
		t.Errorf("line 4 is in both ranges: %v", got)
	}
}

// The workflows next to a workflow are read once for all the files of a run, however many of them have
// a workflow_run event (pytorch has 130), and a lookup without the shared value reads them again.
func TestSiblingWorkflowsAreReadOnce(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	up := filepath.Join(wfDir, "up.yaml")
	if err := os.WriteFile(up, []byte("name: Up\non: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := NewProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	we := &WebhookEvent{Workflows: []*String{{Value: "Up"}}}

	sib := &siblingWorkflows{}
	if !upstreamWorkflowsTrusted(project, sib, we) {
		t.Fatal("the upstream workflow runs on push only, so it is trusted")
	}
	// The file changes, but the run has read it already
	if err := os.WriteFile(up, []byte("name: Up\non: pull_request\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !upstreamWorkflowsTrusted(project, sib, we) {
		t.Error("the files were read again")
	}
	if upstreamWorkflowsTrusted(project, nil, we) {
		t.Error("without the shared value the file is read again and pull_request is not trusted")
	}
	// The cache of a run hands out one value
	c := NewLocalActionsCache(project, nil)
	if c.siblings() == nil || c.siblings() != c.siblings() {
		t.Error("one value per cache")
	}
	var nilCache *LocalActionsCache
	if nilCache.siblings() != nil {
		t.Error("a nil cache has none")
	}
}

func referenceSrcLineStart(src []byte, line int) int {
	if line < 1 {
		return -1
	}
	off := 0
	for i := 1; i < line; i++ {
		j := bytes.IndexByte(src[off:], '\n')
		if j < 0 {
			return -1
		}
		off += j + 1
	}
	return off
}

func TestSrcLineStartMatchesScanning(t *testing.T) {
	for _, src := range []string{"", "\n", "a", "a\n", "a\nb", "a\nb\n", "\n\n", "a\r\nb\r\n", "x\n\ny"} {
		b := []byte(src)
		for line := -1; line <= strings.Count(src, "\n")+3; line++ {
			if got, want := srcLineStart(b, line), referenceSrcLineStart(b, line); got != want {
				t.Errorf("srcLineStart(%q, %d) = %d, want %d", src, line, got, want)
			}
		}
	}
}

package jactionlint

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The top of docs/checks.md lists every check by topic. The list is written by hand, and the old one had left out
// fourteen sections and subsections (the permissions checks among them), so a reader who looked for a check there did
// not find it. This keeps the list complete: every section, and every subsection that has an anchor of its own, is
// linked from the part of the page above the first anchor.
func TestChecksIndexListsEverySection(t *testing.T) {
	b, err := os.ReadFile("docs/checks.md")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")

	first := -1
	for i, l := range lines {
		if reAnchorLine.MatchString(l) {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatal("docs/checks.md has no section anchors")
	}
	index := strings.Join(lines[:first], "\n")
	linked := map[string]bool{}
	for _, m := range regexp.MustCompile(`\]\(#([^)\s]+)\)`).FindAllStringSubmatch(index, -1) {
		linked[m[1]] = true
	}

	// A run of anchor lines directly above a heading are the names of one section: one link is enough.
	fence := false
	var block []string
	flush := func(heading string) {
		if len(block) == 0 {
			return
		}
		ok := false
		for _, id := range block {
			ok = ok || linked[id]
		}
		if !ok {
			t.Errorf("the index at the top of docs/checks.md does not link %q (anchors %v)", heading, block)
		}
		block = nil
	}
	for _, l := range lines[first:] {
		if strings.HasPrefix(l, "```") {
			fence = !fence
		}
		if fence {
			continue
		}
		if m := reAnchorLine.FindStringSubmatch(l); m != nil {
			block = append(block, m[1])
			continue
		}
		if strings.HasPrefix(l, "#") {
			flush(l)
		} else if strings.TrimSpace(l) != "" {
			block = nil // an anchor that is not above a heading names part of the text above it
		}
	}
}

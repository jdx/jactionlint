package jactionlint

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// summaryPrinter prints how many findings there are per rule and per file, split into the new ones
// and the ones the baseline accepts. It is for CI logs and for planning the adoption of a stricter
// configuration: the findings themselves are not printed.
type summaryPrinter struct{}

type summaryCount struct{ fresh, baselined int }

func (c summaryCount) total() int { return c.fresh + c.baselined }

func (summaryPrinter) print(w io.Writer, results []fileResult, _ []string) error {
	byRule := map[string]*summaryCount{}
	byFile := map[string]*summaryCount{}
	var total summaryCount
	files, filesWithFindings := 0, 0
	unused, baselineSeen := 0, false

	add := func(m map[string]*summaryCount, key string, baselined bool) {
		c := m[key]
		if c == nil {
			c = &summaryCount{}
			m[key] = c
		}
		if baselined {
			c.baselined++
		} else {
			c.fresh++
		}
	}
	for _, r := range results {
		if r.baselineFile {
			baselineSeen = true
			unused += len(r.stale)
			continue
		}
		files++
		n := 0
		for _, e := range r.errs {
			add(byRule, e.ID, false)
			add(byFile, r.path, false)
			total.fresh++
			n++
		}
		for _, e := range r.baselined {
			add(byRule, e.ID, true)
			add(byFile, r.path, true)
			total.baselined++
			n++
		}
		if n > 0 {
			filesWithFindings++
		}
	}
	showBaseline := baselineSeen || total.baselined > 0
	fmt.Fprintf(w, "%s in %d of %d files", countNoun(total.total(), "finding"), filesWithFindings, files)
	if showBaseline {
		fmt.Fprintf(w, ": %d new, %d baselined", total.fresh, total.baselined)
	}
	fmt.Fprintln(w)
	if baselineSeen {
		verb := "match"
		if unused == 1 {
			verb = "matches"
		}
		fmt.Fprintf(w, "%s %s nothing any more\n", countNoun(unused, "baseline entry"), verb)
	}
	if total.total() == 0 {
		return nil
	}

	table := func(title string, m map[string]*summaryCount) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b string) int {
			if d := m[b].total() - m[a].total(); d != 0 {
				return d
			}
			return strings.Compare(a, b)
		})
		width := len(title)
		for _, k := range keys {
			width = max(width, len(k))
		}
		fmt.Fprintln(w)
		if showBaseline {
			fmt.Fprintf(w, "%-*s  %5s  %9s  %5s\n", width, title, "new", "baselined", "total")
		} else {
			fmt.Fprintf(w, "%-*s  %8s\n", width, title, "findings")
		}
		for _, k := range keys {
			c := m[k]
			if showBaseline {
				fmt.Fprintf(w, "%-*s  %5d  %9d  %5d\n", width, k, c.fresh, c.baselined, c.total())
			} else {
				fmt.Fprintf(w, "%-*s  %8d\n", width, k, c.total())
			}
		}
	}
	table("by rule", byRule)
	table("by file", byFile)
	return nil
}

func countNoun(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	if strings.HasSuffix(noun, "y") {
		return fmt.Sprintf("%d %sies", n, strings.TrimSuffix(noun, "y"))
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

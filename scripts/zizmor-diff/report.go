package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
)

// RepoInput is what was collected for one repository.
type RepoInput struct {
	Name string
	Dir  string
	// Skipped is set when the repository was not analyzed at all, for example the directory is missing.
	Skipped          string
	JactionlintError string
	ZizmorError      string
	Jactionlint      []Finding
	Zizmor           []Finding
}

// RepoResult is the per repository summary in the report.
type RepoResult struct {
	Name             string `json:"name"`
	Dir              string `json:"dir"`
	Skipped          string `json:"skipped,omitempty"`
	JactionlintError string `json:"jactionlint_error,omitempty"`
	ZizmorError      string `json:"zizmor_error,omitempty"`
	Zizmor           int    `json:"zizmor"`
	Jactionlint      int    `json:"jactionlint"`
	Shared           int    `json:"shared"`
	// OutOfScope counts zizmor findings in files jactionlint does not check (action.yml, dependabot.yml, ...).
	OutOfScope int `json:"zizmor_out_of_scope"`
}

// AuditRow is one zizmor audit.
type AuditRow struct {
	Audit    string `json:"audit"`
	Coverage string `json:"coverage"` // full, partial, or "unmapped"
	Notes    string `json:"notes,omitempty"`
	// Rules are the jactionlint rule IDs the audit maps to.
	Rules   []string `json:"jactionlint_rules,omitempty"`
	Zizmor  int      `json:"zizmor"`
	Matched int      `json:"also_reported_by_jactionlint"`
	Repos   int      `json:"repos"`
}

// Missed is the number of zizmor findings jactionlint does not report.
func (a AuditRow) Missed() int { return a.Zizmor - a.Matched }

// RuleRow groups the findings only jactionlint reports.
type RuleRow struct {
	Rule     string    `json:"rule"`
	Count    int       `json:"count"`
	Examples []Finding `json:"examples"`
}

// Report is the whole comparison.
type Report struct {
	ZizmorVersion   string       `json:"zizmor_version,omitempty"`
	MappingZizmor   string       `json:"mapping_zizmor"`
	LineTolerance   int          `json:"line_tolerance"`
	Repos           []RepoResult `json:"repos"`
	Audits          []AuditRow   `json:"audits"`
	JactionlintOnly []RuleRow    `json:"jactionlint_only"`
	Unmapped        []AuditRow   `json:"unmapped_audits"`
}

const maxExamples = 5

// Build compares the findings of each repository and aggregates them.
func Build(inputs []RepoInput, m *Mapping, lineTolerance int) *Report {
	rep := &Report{MappingZizmor: m.Zizmor, LineTolerance: lineTolerance}

	mapped := map[string]*AuditRow{}
	for name, a := range m.Audits {
		var rules []string
		for _, j := range a.Jactionlint {
			if !slices.Contains(rules, j.Rule) {
				rules = append(rules, j.Rule)
			}
		}
		mapped[name] = &AuditRow{Audit: name, Coverage: a.Coverage, Notes: a.Notes, Rules: rules}
	}
	unmapped := map[string]*AuditRow{}
	only := map[string]*RuleRow{}

	for _, in := range inputs {
		res := RepoResult{
			Name: in.Name, Dir: in.Dir, Skipped: in.Skipped,
			JactionlintError: in.JactionlintError, ZizmorError: in.ZizmorError,
		}
		if in.Skipped != "" {
			rep.Repos = append(rep.Repos, res)
			continue
		}
		res.Jactionlint = len(in.Jactionlint)
		shared := make([]bool, len(in.Jactionlint))
		seen := map[string]bool{} // audit -> counted this repo
		for _, z := range in.Zizmor {
			if !isWorkflowFile(z.File) {
				res.OutOfScope++
				continue
			}
			res.Zizmor++
			row, ok := mapped[z.Rule]
			if !ok {
				if row, ok = unmapped[z.Rule]; !ok {
					row = &AuditRow{Audit: z.Rule, Coverage: "unmapped"}
					unmapped[z.Rule] = row
				}
			}
			row.Zizmor++
			if !seen[z.Rule] {
				seen[z.Rule] = true
				row.Repos++
			}
			a, ok := m.Audits[z.Rule]
			if !ok {
				continue
			}
			hit := false
			for i, j := range in.Jactionlint {
				if j.File == z.File && abs(j.Line-z.Line) <= lineTolerance && a.covers(j) {
					shared[i] = true
					hit = true
				}
			}
			if hit {
				row.Matched++
			}
		}
		for i, j := range in.Jactionlint {
			if shared[i] {
				res.Shared++
				continue
			}
			r := only[j.Rule]
			if r == nil {
				r = &RuleRow{Rule: j.Rule}
				only[j.Rule] = r
			}
			r.Count++
			if len(r.Examples) < maxExamples {
				r.Examples = append(r.Examples, j)
			}
		}
		rep.Repos = append(rep.Repos, res)
	}

	for _, r := range mapped {
		rep.Audits = append(rep.Audits, *r)
	}
	for _, r := range unmapped {
		rep.Unmapped = append(rep.Unmapped, *r)
	}
	for _, r := range only {
		rep.JactionlintOnly = append(rep.JactionlintOnly, *r)
	}
	slices.SortFunc(rep.Audits, func(a, b AuditRow) int {
		return cmp.Or(cmp.Compare(b.Zizmor, a.Zizmor), cmp.Compare(a.Audit, b.Audit))
	})
	slices.SortFunc(rep.Unmapped, func(a, b AuditRow) int {
		return cmp.Or(cmp.Compare(b.Zizmor, a.Zizmor), cmp.Compare(a.Audit, b.Audit))
	})
	slices.SortFunc(rep.JactionlintOnly, func(a, b RuleRow) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Rule, b.Rule))
	})
	return rep
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// WriteMarkdown renders the report.
func (r *Report) WriteMarkdown(w io.Writer) error {
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	p("# jactionlint vs zizmor\n\n")
	if r.ZizmorVersion != "" {
		p("zizmor %s (`--offline --persona pedantic`), mapping written for zizmor %s. ", r.ZizmorVersion, r.MappingZizmor)
	}
	p("A zizmor finding counts as also reported when jactionlint reports a mapped rule in the same file within %d line(s). Only workflow files (`.github/workflows/*.y(a)ml`) are compared.\n\n", r.LineTolerance)

	p("## Mapped audits\n\n")
	p("| zizmor audit | coverage | jactionlint rules | zizmor | also reported | missed | repos |\n|---|---|---|--:|--:|--:|--:|\n")
	var zt, mt int
	for _, a := range r.Audits {
		p("| %s | %s | %s | %d | %d | %d | %d |\n", a.Audit, a.Coverage, codeList(a.Rules), a.Zizmor, a.Matched, a.Missed(), a.Repos)
		zt += a.Zizmor
		mt += a.Matched
	}
	p("| **total** | | | %d | %d | %d | |\n\n", zt, mt, zt-mt)

	p("## jactionlint only\n\n")
	p("Findings with no matching zizmor finding. Either zizmor does not check it (a correctness bug) or the two disagree.\n\n")
	if len(r.JactionlintOnly) == 0 {
		p("None.\n\n")
	} else {
		p("| rule | findings | examples |\n|---|--:|---|\n")
		for _, o := range r.JactionlintOnly {
			ex := make([]string, len(o.Examples))
			for i, e := range o.Examples {
				ex[i] = fmt.Sprintf("`%s/%s:%d`", e.Repo, e.File, e.Line)
			}
			p("| %s | %d | %s |\n", o.Rule, o.Count, strings.Join(ex, ", "))
		}
		p("\n")
	}

	p("## Unmapped zizmor audits\n\n")
	p("zizmor reports these but `mapping.json` has no jactionlint rule for them.\n\n")
	if len(r.Unmapped) == 0 {
		p("None.\n\n")
	} else {
		p("| zizmor audit | findings | repos |\n|---|--:|--:|\n")
		for _, a := range r.Unmapped {
			p("| %s | %d | %d |\n", a.Audit, a.Zizmor, a.Repos)
		}
		p("\n")
	}

	p("## Repositories\n\n")
	p("| repo | zizmor | jactionlint | shared | zizmor out of scope | notes |\n|---|--:|--:|--:|--:|---|\n")
	for _, rr := range r.Repos {
		var notes []string
		if rr.Skipped != "" {
			notes = append(notes, "skipped: "+rr.Skipped)
		}
		if rr.JactionlintError != "" {
			notes = append(notes, "jactionlint failed: "+oneLine(rr.JactionlintError))
		}
		if rr.ZizmorError != "" {
			notes = append(notes, "zizmor failed: "+oneLine(rr.ZizmorError))
		}
		p("| %s | %d | %d | %d | %d | %s |\n", rr.Name, rr.Zizmor, rr.Jactionlint, rr.Shared, rr.OutOfScope, strings.Join(notes, "; "))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func codeList(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = "`" + s + "`"
	}
	return strings.Join(out, ", ")
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return strings.ReplaceAll(s, "|", "\\|")
}

package jactionlint

import (
	"fmt"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleUntrustedArtifact reports workflow_run workflows that download an artifact of the run that
// triggered them and then run it, extract it or put its content in the environment of later steps
// without checking it. The workflow that made the artifact runs the code of a pull request, so the
// artifact is whatever the author of the pull request wanted it to be.
type RuleUntrustedArtifact struct {
	RuleBase
	project  *Project
	siblings *siblingWorkflows // nil reads the workflows of the project for each file
	active   bool
}

// NewRuleUntrustedArtifact creates a new RuleUntrustedArtifact instance. The project can be nil.
func NewRuleUntrustedArtifact(project *Project) *RuleUntrustedArtifact {
	return &RuleUntrustedArtifact{
		RuleBase: RuleBase{
			name: "untrusted-artifact",
			desc: "Checks that artifacts of the triggering run are validated before they are used",
		},
		project: project,
	}
}

// artifactDownload is a step that downloads artifacts of another run.
type artifactDownload struct {
	step *Step
	pos  *Pos
	// prefixes are the paths in the workspace that the artifact is put in: the directory or the name
	// of the artifact. They are what later commands are linked to the artifact by. It is empty when
	// the artifact lands in the root of the workspace under unknown names.
	prefixes []string
	// raw is true when the artifact comes as an archive that the download did not extract.
	raw bool
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUntrustedArtifact) VisitWorkflowPre(n *Workflow) error {
	rule.active = false
	if !rule.Config().RuleEnabled("untrusted-artifact") {
		return nil
	}
	for _, e := range webhookEvents(n, "workflow_run") {
		if !upstreamWorkflowsTrusted(rule.project, rule.siblings, e) {
			rule.active = true
		}
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUntrustedArtifact) VisitJobPre(n *Job) error {
	if !rule.active || (n.Environment != nil && n.Environment.Name != nil) || conditionIsGuard(n.If) {
		return nil
	}
	steps := flattenSteps(n.Steps)
	for i, s := range steps {
		if conditionIsGuard(s.If) {
			continue
		}
		dl := rule.downloadOf(s)
		if dl == nil {
			continue
		}
		if scriptValidates(s) {
			continue
		}
		for _, later := range steps[i+1:] {
			if scriptValidates(later) {
				break // everything from here on may rely on the check
			}
			if conditionIsGuard(later.If) {
				continue // a step a maintainer has to allow does not use the artifact on its own
			}
			if how, ok := artifactUse(later, dl); ok {
				line := 0
				if later.Pos != nil {
					line = later.Pos.Line
				}
				rule.ReportIDf(
					"untrusted-artifact",
					dl.pos,
					"this step downloads an artifact of the run that triggered the workflow, which ran the code of a pull request, and the step at line %d %s without validating its content first. the artifact is whatever the pull request wanted, and this workflow has a write token and secrets. match the content against a strict pattern (for example digits only) before you use it, and never run or extract it",
					line, how,
				)
				break
			}
		}
	}
	return nil
}

// downloadOf returns the download of artifacts from another run that the step makes.
func (rule *RuleUntrustedArtifact) downloadOf(s *Step) *artifactDownload {
	switch e := s.Exec.(type) {
	case *ExecAction:
		if e.Uses == nil || e.Uses.ContainsExpression() {
			return nil
		}
		u := ParseUses(e.Uses.Value)
		if u.Kind != UsesAction {
			return nil
		}
		dl := &artifactDownload{step: s, pos: e.Uses.Pos}
		switch strings.ToLower(u.CanonicalName()) {
		case "actions/download-artifact":
			in := e.Inputs["run-id"]
			if in == nil || in.Value == nil {
				return nil // the artifacts of the current run
			}
			refs, _ := stringExprRefs(in.Value.Value)
			if !refsRead(refs, []string{"github", "event", "workflow_run"}) {
				return nil
			}
		case "dawidd6/action-download-artifact":
		case "actions/github-script":
			script, ok := e.input("script")
			if !ok || !strings.Contains(script, "listWorkflowRunArtifacts") || !strings.Contains(script, "downloadArtifact") {
				return nil
			}
			dl.raw = true
			return dl
		default:
			return nil
		}
		if p, ok := e.input("path"); ok && normalizeDir(p) != "" {
			dl.prefixes = append(dl.prefixes, normalizeDir(p))
		} else if n, ok := e.input("name"); ok && n != "" && !ContainsExpression(n) && !strings.ContainsAny(n, "*?") {
			dl.prefixes = append(dl.prefixes, n)
		}
		return dl
	case *ExecRun:
		script, _ := analyzeRun(e)
		if script == nil {
			return nil
		}
		for _, c := range script.Commands {
			if c.Name != "gh" || c.Verb() != "run" || c.Sub(1) != "download" {
				continue
			}
			pos := s.Pos
			if e.RunPos != nil {
				pos = e.RunPos
			}
			// `gh run download` without a run ID, or with the ID of the current run, does not fetch what the triggering
			// run uploaded
			if !downloadsOtherRun(c) {
				continue
			}
			dl := &artifactDownload{step: s, pos: pos}
			for _, w := range c.FlagValues("-D", "--dir") {
				if d := normalizeDir(w.Value); d != "" {
					dl.prefixes = append(dl.prefixes, d)
				}
			}
			if len(dl.prefixes) == 0 {
				for _, w := range c.FlagValues("-n", "--name") {
					if !w.Dynamic() {
						dl.prefixes = append(dl.prefixes, w.Value)
					}
				}
			}
			return dl
		}
	}
	return nil
}

// downloadsOtherRun reports whether `gh run download` names a run other than the current one: its run ID is
// the first word after "download" (the other words are flags and their values).
func downloadsOtherRun(c *runscript.Command) bool {
	if len(c.Positional) < 3 {
		return false
	}
	id := c.Positional[2]
	raw := strings.ToLower(id.Raw)
	return !(strings.Contains(raw, "github.run_id") || strings.Contains(raw, "github_run_id"))
}

func refsRead(refs []exprRef, target []string) bool {
	for _, r := range refs {
		if refCovers(r.chain, target) {
			return true
		}
	}
	return false
}

// artifactUse reports whether the step runs or extracts something from the artifact, or writes its
// content to the environment of later steps, and describes how.
func artifactUse(s *Step, dl *artifactDownload) (string, bool) {
	if a, ok := s.Exec.(*ExecAction); ok {
		// an action below the download directory is code of the artifact
		if a.Uses == nil || a.Uses.ContainsExpression() || ParseUses(a.Uses.Value).Kind != UsesLocal {
			return "", false
		}
		v := normalizeDir(a.Uses.Value)
		for _, p := range dl.prefixes {
			if v == p || strings.HasPrefix(v, p+"/") {
				return "runs the local action " + a.Uses.Value, true
			}
		}
		return "", false
	}
	e, ok := s.Exec.(*ExecRun)
	if !ok || e.Run == nil {
		return "", false
	}
	script, _ := analyzeRun(e)
	if script == nil {
		return "", false
	}
	under := func(c *runscript.Command) bool {
		for _, p := range dl.prefixes {
			if commandTouches(c, p) {
				return true
			}
		}
		return false
	}
	wd := stepWorkdir(s)
	inDir := false
	for _, p := range dl.prefixes {
		if wd == p || strings.HasPrefix(wd, p+"/") {
			inDir = true
		}
	}
	for _, c := range script.Commands {
		if c.Name == "cd" || c.Name == "pushd" {
			if len(c.Positional) > 0 {
				for _, p := range dl.prefixes {
					if wordIsUnder(c.Positional[0], p) {
						inDir = true
					}
				}
			}
			continue
		}
		if isExtraction(c) && (dl.raw || under(c) || inDir) {
			return fmt.Sprintf("extracts it with %q", c.Name), true
		}
		if commandRunsCode(c) && (under(c) || inDir) {
			return fmt.Sprintf("runs it with %q", c.Name), true
		}
	}
	for _, w := range script.WritesTo("GITHUB_ENV", "GITHUB_PATH", "GITHUB_OUTPUT") {
		for _, p := range w.Producers {
			if under(p) || (inDir && readsRelativeFile(p)) {
				return fmt.Sprintf("writes its content to $%s", w.Var), true
			}
			for _, word := range p.Words {
				for _, sub := range word.Subs {
					if under(sub) || (inDir && readsRelativeFile(sub)) {
						return fmt.Sprintf("writes its content to $%s", w.Var), true
					}
				}
			}
		}
		// read VAR < file; echo "VAR=$VAR" >> $GITHUB_ENV
		for _, r := range script.Redirects {
			if r.Op == "<" && r.Target != nil {
				if inDir && !strings.HasPrefix(r.Target.Value, "/") && !strings.HasPrefix(r.Target.Value, "~") && !r.Target.Dynamic() {
					return fmt.Sprintf("writes its content to $%s", w.Var), true
				}
				for _, p := range dl.prefixes {
					if wordIsUnder(r.Target, p) {
						return fmt.Sprintf("writes its content to $%s", w.Var), true
					}
				}
			}
		}
	}
	return "", false
}

// readsRelativeFile reports whether the command is one that reads a file named by a relative path, which
// in the download directory is a file of the artifact.
func readsRelativeFile(c *runscript.Command) bool {
	switch c.Name {
	case "cat", "head", "tail", "jq", "yq", "grep", "egrep", "fgrep", "sed", "awk", "cut", "tr", "sort", "uniq", "wc", "xargs", "tac", "rev", "nl":
	default:
		return false
	}
	for _, w := range c.Positional {
		if w == nil {
			continue
		}
		if v := w.Value; v != "" && !strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "~") && !strings.HasPrefix(v, "-") {
			return true
		}
	}
	return false
}

// isExtraction reports whether the command unpacks an archive.
func isExtraction(c *runscript.Command) bool {
	switch c.Name {
	case "unzip", "bsdtar", "ditto", "gunzip", "7z", "7za", "7zr", "unrar":
		return c.Name != "7z" || c.Verb() == "x" || c.Verb() == "e"
	case "tar":
		return c.HasFlag("-x", "--extract", "--get") || (c.Verb() != "" && strings.HasPrefix(c.Verb(), "x"))
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{ID: "untrusted-artifact", Group: RuleGroupSecurity, Summary: "A workflow_run workflow uses an artifact of the triggering run without validating it.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-untrusted-artifact"},
	)
	registerRuleFactory("untrusted-artifact", func(env *RuleEnv) []Rule {
		r := NewRuleUntrustedArtifact(env.project)
		r.siblings = env.localActions.siblings()
		return []Rule{r}
	})
}

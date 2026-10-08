package runscript

import (
	"fmt"
	"strings"
)

// dump renders everything the analyzer found in a stable text form for golden files.
func dump(s *Script) string {
	var sb strings.Builder
	pf := func(f string, a ...any) { fmt.Fprintf(&sb, f+"\n", a...) }
	word := func(w *Word) string {
		if w == nil {
			return "<nil>"
		}
		var fl []string
		if len(w.Exprs) > 0 {
			fl = append(fl, "expr")
		}
		if len(w.Vars) > 0 {
			fl = append(fl, "var:"+strings.Join(w.Vars, ","))
		}
		if w.Subst {
			fl = append(fl, "subst")
		}
		if w.Glob {
			fl = append(fl, "glob")
		}
		if w.Quoted {
			fl = append(fl, "quoted")
		}
		if w.IsExpr() {
			fl = append(fl, "whole-expr")
		}
		if len(fl) > 0 {
			return fmt.Sprintf("%q[%s]", w.Value, strings.Join(fl, " "))
		}
		return fmt.Sprintf("%q", w.Value)
	}
	words := func(ws []*Word) string {
		out := make([]string, len(ws))
		for i, w := range ws {
			out[i] = word(w)
		}
		return strings.Join(out, " ")
	}
	for _, e := range s.Exprs {
		pf("expr %d:%d %q", e.Loc.Line, e.Loc.Col, e.Text)
	}
	for _, c := range s.Commands {
		line := fmt.Sprintf("cmd %d:%d name=%q", c.Line, c.Col, c.Name)
		if c.Tool != "" {
			line += " tool=" + c.Tool
		}
		if len(c.Wrappers) > 0 {
			line += " wrappers=" + strings.Join(c.Wrappers, ",")
		}
		if c.Pipeline != nil {
			line += fmt.Sprintf(" pipe=%d/%d", c.Stage+1, len(c.Pipeline.Stages))
		}
		pf("%s", line)
		for _, a := range c.Assigns {
			pf("  assign %s=%s", a.Name, word(a.Value))
		}
		if len(c.Flags) > 0 {
			var fs []string
			for _, f := range c.Flags {
				if f.Value != nil {
					fs = append(fs, f.Name+"="+word(f.Value))
				} else {
					fs = append(fs, f.Name)
				}
			}
			pf("  flags %s", strings.Join(fs, " "))
		}
		if len(c.Positional) > 0 {
			pf("  pos %s", words(c.Positional))
		}
		if in := c.Installs(); in != nil {
			pf("  install tool=%s eco=%s verb=%s run=%v global=%v locked=%v manifest=%v reqs=%d", in.Tool, in.Ecosystem, in.Verb, in.Run, in.Global, in.Locked, in.FromManifest, len(in.Requirements))
			for _, p := range in.Packages {
				pf("    pkg %q name=%q ver=%q kind=%s pinned=%v dyn=%v local=%v", p.Spec, p.Name, p.Version, p.Kind, p.Pinned, p.Dynamic, p.Local)
			}
		}
		if p := c.Publishes(); p != nil {
			pf("  publish tool=%s verb=%q kind=%s dry=%v registry=%s", p.Tool, p.Verb, p.Kind, p.DryRun, word(p.Registry))
		}
	}
	for _, p := range s.Pipelines {
		pf("pipeline %d:%d stages=%d negated=%v", p.Line, p.Col, len(p.Stages), p.Negated)
	}
	for _, r := range s.Redirects {
		line := fmt.Sprintf("redirect %d:%d op=%q fd=%q target=%s append=%v write=%v", r.Line, r.Col, r.Op, r.Fd, word(r.Target), r.Append, r.Write)
		if r.Group {
			line += fmt.Sprintf(" group inner=%d", len(r.Inner))
		}
		if r.Tee {
			line += " tee"
		}
		if r.Heredoc != nil {
			line += fmt.Sprintf(" heredoc(delim=%q quoted=%v tabs=%v exprs=%d lines=%d)", r.Heredoc.Delim, r.Heredoc.Quoted, r.Heredoc.Tabs, len(r.Heredoc.Exprs), strings.Count(r.Heredoc.Body, "\n"))
		}
		pf("%s", line)
	}
	for _, a := range s.Assignments {
		pf("assignment %d:%d %s value=%s append=%v array=%v", a.Line, a.Col, a.Name, word(a.Value), a.Append, a.Array)
	}
	for _, name := range []string{"GITHUB_ENV", "GITHUB_PATH", "GITHUB_OUTPUT", "GITHUB_STATE"} {
		for _, w := range s.WritesTo(name) {
			pf("writes %s at %d:%d append=%v producers=%d exprs=%q", name, w.Redirect.Line, w.Redirect.Col, w.Append, len(w.Producers), w.Exprs)
		}
	}
	for _, sp := range s.ShellPipes() {
		pf("shellpipe form=%s downloader=%q shell=%q url=%s", sp.Form, sp.Downloader.Name, sp.Shell.Name, word(sp.URL))
	}
	return sb.String()
}

package runscript

import "fmt"

// CheckInvariants verifies the structural guarantees of an analyzed script and exercises every helper. It is
// meant for fuzzers and tests: it returns an error describing the first violated guarantee.
func CheckInvariants(s *Script) error {
	n := len(s.Source)
	loc := func(what string, l Loc) error {
		if l.Offset < 0 || l.Offset > l.End || l.End > n || l.Line < 1 || l.Col < 1 {
			return fmt.Errorf("%s: bad location %+v for a script of %d bytes", what, l, n)
		}
		return nil
	}
	word := func(what string, w *Word) error {
		if w == nil {
			return nil
		}
		if err := loc(what, w.Loc); err != nil {
			return err
		}
		if w.Raw != s.Source[w.Offset:w.End] {
			return fmt.Errorf("%s: raw %q is not the source %q", what, w.Raw, s.Source[w.Offset:w.End])
		}
		return nil
	}
	for _, e := range s.Exprs {
		if err := loc("expr", e.Loc); err != nil {
			return err
		}
		if e.Raw != s.Source[e.Loc.Offset:e.Loc.End] {
			return fmt.Errorf("expr: raw %q is not the source", e.Raw)
		}
	}
	for _, c := range s.Commands {
		if err := loc("command", c.Loc); err != nil {
			return err
		}
		for _, w := range c.Words {
			if err := word("command word", w); err != nil {
				return err
			}
		}
		for _, f := range c.Flags {
			if f.Word == nil {
				return fmt.Errorf("flag %q without word", f.Name)
			}
		}
		if c.Pipeline != nil && (c.Stage < 0 || c.Stage >= len(c.Pipeline.Stages)) {
			return fmt.Errorf("command %q: bad stage %d", c.Name, c.Stage)
		}
		if in := c.Installs(); in != nil {
			if in.Cmd != c {
				return fmt.Errorf("install of another command")
			}
			for _, p := range in.Packages {
				if p.Word == nil || p.Kind == "" || p.Local != (p.Kind == KindPath) {
					return fmt.Errorf("bad package %#v", p)
				}
			}
		}
		_ = c.Publishes()
		_ = c.Verb()
		_ = c.HasFlag("-a", "--all")
	}
	for _, r := range s.Redirects {
		if err := loc("redirect", r.Loc); err != nil {
			return err
		}
		if err := word("redirect target", r.Target); err != nil {
			return err
		}
	}
	for _, p := range s.Pipelines {
		if err := loc("pipeline", p.Loc); err != nil {
			return err
		}
		if len(p.Stages) < 2 {
			return fmt.Errorf("pipeline of %d stages", len(p.Stages))
		}
	}
	for _, a := range s.Assignments {
		if err := loc("assignment", a.Loc); err != nil {
			return err
		}
		if err := word("assignment value", a.Value); err != nil {
			return err
		}
	}
	for _, w := range s.WritesTo("GITHUB_ENV", "GITHUB_PATH", "GITHUB_OUTPUT", "GITHUB_STATE") {
		if w.Redirect == nil {
			return fmt.Errorf("write without redirect")
		}
	}
	for _, p := range s.ShellPipes() {
		if p.Downloader == nil || p.Shell == nil {
			return fmt.Errorf("incomplete shell pipe")
		}
	}
	o := Origin{Line: 3, Col: 7, Literal: true, Indent: 4}
	for _, off := range []int{-1, 0, n / 2, n, n + 10} {
		if p := s.Position(o, off); p.Line < 1 || p.Col < 1 {
			return fmt.Errorf("position %+v for offset %d", p, off)
		}
	}
	return nil
}

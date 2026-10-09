package runscript

import (
	"slices"
	"strings"
)

// ShellPipe is a download whose result is run by a shell without being stored or checked.
type ShellPipe struct {
	// Downloader is the curl or wget command, Shell the command which runs the download.
	Downloader, Shell *Command
	// Form is how the download reaches the shell:
	//   "pipe"  curl ... | sh
	//   "subst" sh -c "$(curl ...)", sh <(curl ...), eval "$(curl ...)", source <(curl ...)
	Form string
	// URL is the first positional argument of the downloader: the URL (or, with --url, its value).
	URL *Word
	// Pipeline is set for Form "pipe".
	Pipeline *Pipeline
}

func isDownloader(c *Command) bool { return c.Tool == "curl" || c.Tool == "wget" }

// isShell returns whether the command runs a script from stdin or from its arguments: a shell, eval or source.
func isShell(c *Command) bool {
	return shells[c.Tool] || c.Tool == "eval" || c.Tool == "source" || c.Tool == "."
}

func (c *Command) downloadURL() *Word {
	if vs := c.FlagValues("--url"); len(vs) > 0 {
		return vs[0]
	}
	for _, w := range c.Positional {
		if strings.Contains(w.Value, "://") || strings.HasPrefix(w.Value, "$") || !strings.HasPrefix(w.Value, "-") {
			return w
		}
	}
	return nil
}

// ShellPipes returns the downloads which are piped or substituted into a shell, in source order.
//
//	curl -fsSL https://example.com/install.sh | sh
//	wget -qO- https://example.com/install.sh | sudo bash -s -- -y
//	bash -c "$(curl -fsSL https://example.com/install.sh)"
//	bash <(curl -fsSL https://example.com/install.sh)
//
// A pipe into a shell which reads a file (`curl ... | bash script.sh`) or only runs a command
// (`curl ... | sh -c 'cat'`) is not reported.
func (s *Script) ShellPipes() []*ShellPipe {
	var out []*ShellPipe
	for _, p := range s.Pipelines {
		for i := 1; i < len(p.Stages); i++ {
			for _, sh := range p.Stages[i].Commands {
				if !isShell(sh) || sh.Pipeline != p || sh.Stage != i || !readsStdin(sh) {
					continue
				}
				for _, st := range p.Stages[:i] {
					for _, d := range st.Commands {
						if isDownloader(d) {
							out = append(out, &ShellPipe{Downloader: d, Shell: sh, Form: "pipe", URL: d.downloadURL(), Pipeline: p})
						}
					}
				}
			}
		}
	}
	for _, c := range s.Commands {
		if !isShell(c) {
			continue
		}
		dash := c.HasFlag("-c")
		for _, w := range c.Words {
			if !w.Subst || (!w.ProcSubst && !dash && c.Tool != "eval" && c.Tool != "source" && c.Tool != ".") {
				continue
			}
			for _, d := range w.Subs {
				if isDownloader(d) {
					out = append(out, &ShellPipe{Downloader: d, Shell: c, Form: "subst", URL: d.downloadURL()})
				}
			}
		}
	}
	sortShellPipes(out)
	return out
}

// readsStdin: `sh`, `bash -s`, `bash -` and `sh -e` run the script from stdin, `bash file` and `bash -c cmd` do
// not. eval and source never read stdin (source /dev/stdin is the exception, and handled as a file).
func readsStdin(c *Command) bool {
	switch c.Tool {
	case "eval":
		return false
	case "source", ".":
		for _, w := range c.Positional {
			if w.Value == "/dev/stdin" || w.Value == "/dev/fd/0" {
				return true
			}
		}
		return false
	}
	if c.HasFlag("-c") {
		return false
	}
	pos := c.Positional
	if c.Name == "busybox" || c.Name == "toybox" {
		// The first word is the applet: `busybox sh` is a shell, `busybox cat` is not
		if len(pos) == 0 {
			return false
		}
		switch pos[0].Value {
		case "sh", "ash", "bash":
			pos = pos[1:]
		default:
			return false
		}
	}
	for _, w := range pos {
		if w.Value == "-" {
			return true
		}
	}
	return len(pos) == 0 || c.HasFlag("-s")
}

func sortShellPipes(ps []*ShellPipe) {
	slices.SortStableFunc(ps, func(a, b *ShellPipe) int { return a.Shell.Offset - b.Shell.Offset })
}

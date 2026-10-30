package runner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdx/jactionlint/v2/internal/runner/cache"
	"github.com/spf13/pflag"
)

// IsCacheCommand reports whether the arguments (including the program name) ask for `jactionlint cache`.
func IsCacheCommand(args []string) bool {
	return len(args) > 1 && args[1] == "cache"
}

const cacheHelp = `Usage: jactionlint cache COMMAND [OPTIONS]

Manage the local actions cache that 'jactionlint run' uses.

Commands:
  ls              list the entries of this repository's cache (--all: of every repository)
  rm KEY          remove the entries with this key (KEY* removes by prefix)
  prune           remove expired entries and apply the size limit
  path            print the directory of the cache

Options:
`

// CacheMain runs `jactionlint cache` and returns the exit status. args are the arguments after "cache".
func CacheMain(args []string, stdout, stderr io.Writer) int {
	fs := pflag.NewFlagSet("cache", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var dir, size string
	var all, help bool
	fs.StringVar(&dir, "cache-dir", "", "the cache directory (default ~/.cache/jactionlint/cache)")
	fs.StringVar(&size, "cache-size", "", "size limit for prune, such as 2G (default 10G)")
	fs.BoolVar(&all, "all", false, "with ls, rm and prune: every repository, not the one in the working directory")
	fs.BoolVarP(&help, "help", "h", false, "print this help")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "jactionlint cache: %s (try jactionlint cache --help)\n", err)
		return ExitUsage
	}
	if help || fs.NArg() == 0 {
		fmt.Fprint(stdout, cacheHelp)
		fmt.Fprint(stdout, fs.FlagUsages())
		if help {
			return ExitOK
		}
		return ExitUsage
	}
	var maxBytes int64
	if size != "" {
		n, err := parseSize(size)
		if err != nil {
			fmt.Fprintf(stderr, "jactionlint cache: --cache-size: %v\n", err)
			return ExitUsage
		}
		maxBytes = n
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint cache: %v\n", err)
		return ExitError
	}
	cfg := &Config{CacheDir: dir, In: &Inputs{Repo: readRepoInfo(findRepoRoot(cwd))}}
	base := dir
	if base == "" {
		state, err := StateDir()
		if err != nil {
			fmt.Fprintf(stderr, "jactionlint cache: %v\n", err)
			return ExitError
		}
		base = filepath.Join(state, "cache")
	}
	repoDir, err := cfg.repoCacheDir()
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint cache: %v\n", err)
		return ExitError
	}
	dirs := []string{repoDir}
	if all {
		dirs = nil
		ents, _ := os.ReadDir(base)
		for _, e := range ents {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(base, e.Name()))
			}
		}
	}
	cmd := fs.Arg(0)
	switch cmd {
	case "path":
		fmt.Fprintln(stdout, repoDir)
		return ExitOK
	case "ls", "rm", "prune":
	default:
		fmt.Fprintf(stderr, "jactionlint cache: unknown command %q (ls, rm, prune, path)\n", cmd)
		return ExitUsage
	}
	if cmd == "rm" && fs.NArg() != 2 {
		fmt.Fprintln(stderr, "jactionlint cache rm: give the key to remove")
		return ExitUsage
	}
	status := ExitOK
	total := 0
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			continue
		}
		st, err := cache.Open(d)
		if err != nil {
			fmt.Fprintf(stderr, "jactionlint cache: %v\n", err)
			status = ExitError
			continue
		}
		if maxBytes > 0 {
			st.MaxBytes = maxBytes
		}
		switch cmd {
		case "ls":
			es, err := st.List()
			if err != nil {
				fmt.Fprintf(stderr, "jactionlint cache: %v\n", err)
				status = ExitError
				continue
			}
			if all && len(es) > 0 {
				fmt.Fprintf(stdout, "%s\n", filepath.Base(d))
			}
			for _, e := range es {
				fmt.Fprintf(stdout, "%10s  %-8s  %-22s  %s\n", humanSize(e.Size), age(time.Since(e.LastUsed)), strings.TrimPrefix(e.Scope, "refs/heads/"), e.Key)
			}
			total += len(es)
		case "rm":
			n, err := st.Remove(fs.Arg(1))
			if err != nil {
				fmt.Fprintf(stderr, "jactionlint cache: %v\n", err)
				status = ExitError
			}
			total += n
		case "prune":
			n, err := st.Prune()
			if err != nil {
				fmt.Fprintf(stderr, "jactionlint cache: %v\n", err)
				status = ExitError
			}
			total += n
		}
	}
	switch cmd {
	case "ls":
		if total == 0 {
			fmt.Fprintf(stdout, "the cache is empty (%s)\n", repoDir)
		}
	case "rm":
		fmt.Fprintf(stdout, "removed %d entr%s\n", total, plural(total, "y", "ies"))
		if total == 0 {
			return ExitFailure
		}
	case "prune":
		fmt.Fprintf(stdout, "removed %d file%s\n", total, plural(total, "", "s"))
	}
	return status
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

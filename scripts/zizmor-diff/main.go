// Command zizmor-diff runs jactionlint and zizmor over a corpus of repositories and reports which of
// zizmor's findings jactionlint also reports. See README.md.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Runner runs a command in dir and returns its stdout and exit code. A non-zero exit code is not an error.
// The error is set when the command could not be started.
type Runner func(ctx context.Context, dir string, argv []string) (stdout, stderr []byte, code int, err error)

func execRunner(ctx context.Context, dir string, argv []string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.Bytes(), errOut.Bytes(), ee.ExitCode(), nil
	}
	return out.Bytes(), errOut.Bytes(), 0, err
}

// Repo is an entry of the corpus.
type Repo struct{ Name, Dir string }

// parseCorpus reads one repository per line. Blank lines and lines starting with # are ignored, as are
// trailing comments. A line is `path` or `label=path`, and `~` expands to the home directory.
func parseCorpus(r io.Reader, home string) ([]Repo, error) {
	var repos []Repo
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, " #"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		repos = append(repos, parseRepoSpec(line, home))
	}
	return repos, sc.Err()
}

func parseRepoSpec(spec, home string) Repo {
	name, dir := "", spec
	if i := strings.Index(spec, "="); i > 0 && !strings.ContainsAny(spec[:i], "/~") {
		name, dir = spec[:i], spec[i+1:]
	}
	if dir == "~" {
		dir = home
	} else if strings.HasPrefix(dir, "~/") {
		dir = filepath.Join(home, dir[2:])
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(dir), "-jactionlint")
	}
	return Repo{Name: name, Dir: dir}
}

// Config is the resolved command line.
type Config struct {
	Repos           []Repo
	Jactionlint     []string // command prefix
	JactionlintArgs []string // extra flags, before the output flags
	Zizmor          []string // command prefix
	Mapping         *Mapping
	LineTolerance   int
	Jobs            int
}

// analyze runs both tools on one repository. Failures of one tool are recorded and do not stop the rest.
func analyze(ctx context.Context, run Runner, c *Config, repo Repo) (RepoInput, string) {
	in := RepoInput{Name: repo.Name, Dir: repo.Dir}
	if st, err := os.Stat(repo.Dir); err != nil || !st.IsDir() {
		in.Skipped = "directory not found"
		return in, ""
	}

	jl := append(append(append([]string{}, c.Jactionlint...), c.JactionlintArgs...), jactionlintArgs...)
	out, stderr, code, err := run(ctx, repo.Dir, jl)
	switch {
	case err != nil:
		in.JactionlintError = err.Error()
	case code != 0 && code != 1:
		in.JactionlintError = fmt.Sprintf("exit %d: %s", code, stderr)
	default:
		if in.Jactionlint, err = parseJactionlint(repo.Name, repo.Dir, out); err != nil {
			in.JactionlintError = err.Error()
		}
	}

	zz := append(append(append([]string{}, c.Zizmor...), zizmorArgs...), ".")
	out, stderr, code, err = run(ctx, repo.Dir, zz)
	version := ""
	switch {
	case err != nil:
		in.ZizmorError = err.Error()
	case code != 0 && (code < 10 || code > 14):
		in.ZizmorError = fmt.Sprintf("exit %d: %s", code, stderr)
	default:
		if in.Zizmor, version, err = parseZizmor(repo.Name, repo.Dir, out); err != nil {
			in.ZizmorError = err.Error()
		}
	}
	// A repository where either tool failed would skew the totals, so keep its findings out of them.
	if in.ZizmorError != "" || in.JactionlintError != "" {
		in.Zizmor, in.Jactionlint = nil, nil
	}
	return in, version
}

// Run analyzes the whole corpus with up to c.Jobs repositories at a time and returns the report.
func Run(ctx context.Context, run Runner, c *Config, progress io.Writer) *Report {
	inputs := make([]RepoInput, len(c.Repos))
	versions := make([]string, len(c.Repos))
	sem := make(chan struct{}, max(c.Jobs, 1))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, repo := range c.Repos {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			inputs[i], versions[i] = analyze(ctx, run, c, repo)
			mu.Lock()
			fmt.Fprintf(progress, "analyzed %s\n", repo.Name)
			mu.Unlock()
		}()
	}
	wg.Wait()
	rep := Build(inputs, c.Mapping, c.LineTolerance)
	for _, v := range versions {
		if v != "" {
			rep.ZizmorVersion = v
			break
		}
	}
	return rep
}

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(s string) error { *l = append(*l, s); return nil }

func main() { os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr)) }

func realMain(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("zizmor-diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var repoFlags listFlag
	corpusFile := fs.String("corpus-file", "", "file listing repository directories, one per line (default: scripts/zizmor-diff/corpus.txt)")
	fs.Var(&repoFlags, "repos", "repository directory, or label=directory. Repeatable. Replaces the corpus file")
	jl := fs.String("jactionlint", "jactionlint", "jactionlint executable")
	jlConfig := fs.String("jactionlint-config", "", "jactionlint config file used instead of each repository's own (passed as -config-file)")
	var jlArgs listFlag
	fs.Var(&jlArgs, "jactionlint-arg", "extra jactionlint argument. Repeatable")
	zz := fs.String("zizmor", "mise x zizmor@1.30.1 -- zizmor", "zizmor command (split on spaces)")
	mapPath := fs.String("mapping", "", "audit mapping file (default: the embedded mapping.json)")
	tol := fs.Int("line-tolerance", 0, "lines of distance allowed between a zizmor finding and the jactionlint finding covering it")
	jobs := fs.Int("jobs", 4, "repositories analyzed in parallel")
	mdOut := fs.String("markdown", "", "write the markdown report to this file (default: stdout)")
	jsonOut := fs.String("json", "", "write the JSON report to this file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	fail := func(err error) int {
		fmt.Fprintf(stderr, "zizmor-diff: %v\n", err)
		return 1
	}
	home, _ := os.UserHomeDir()

	var repos []Repo
	if len(repoFlags) > 0 {
		for _, s := range repoFlags {
			repos = append(repos, parseRepoSpec(s, home))
		}
	} else {
		path := *corpusFile
		if path == "" {
			path = filepath.Join("scripts", "zizmor-diff", "corpus.txt")
		}
		f, err := os.Open(path)
		if err != nil {
			return fail(fmt.Errorf("read corpus: %w", err))
		}
		defer f.Close()
		if repos, err = parseCorpus(f, home); err != nil {
			return fail(err)
		}
	}
	if len(repos) == 0 {
		return fail(errors.New("the corpus is empty"))
	}

	m, err := loadMapping(*mapPath)
	if err != nil {
		return fail(err)
	}
	jlCmd := []string{*jl}
	if strings.ContainsRune(*jl, filepath.Separator) {
		// The tool runs inside each repository, so a relative path would stop resolving.
		if jlCmd[0], err = filepath.Abs(*jl); err != nil {
			return fail(err)
		}
	}
	extra := append([]string{}, jlArgs...)
	if *jlConfig != "" {
		abs, err := filepath.Abs(*jlConfig)
		if err != nil {
			return fail(err)
		}
		extra = append(extra, "-config-file", abs)
	}
	zzCmd := strings.Fields(*zz)
	if len(zzCmd) == 0 {
		return fail(errors.New("-zizmor is empty"))
	}

	rep := Run(context.Background(), execRunner, &Config{
		Repos: repos, Jactionlint: jlCmd, JactionlintArgs: extra, Zizmor: zzCmd, Mapping: m, LineTolerance: *tol, Jobs: *jobs,
	}, stderr)

	if *jsonOut != "" {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(*jsonOut, append(b, '\n'), 0o644); err != nil {
			return fail(err)
		}
	}
	w := stdout
	if *mdOut != "" {
		f, err := os.Create(*mdOut)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		w = f
	}
	if err := rep.WriteMarkdown(w); err != nil {
		return fail(err)
	}
	return 0
}

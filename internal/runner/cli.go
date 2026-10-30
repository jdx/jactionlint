package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jdx/jactionlint/v2"
	"github.com/spf13/pflag"
)

// Exit statuses, the same as the other commands of jactionlint.
const (
	ExitOK      = 0
	ExitFailure = 1 // a step or job failed
	ExitUsage   = 2
	ExitError   = 3 // the run could not start
)

// IsRunCommand reports whether the arguments (including the program name) ask for `jactionlint run`.
func IsRunCommand(args []string) bool {
	return len(args) > 1 && args[1] == "run"
}

type options struct {
	ro           runOptions
	list         bool
	json         bool
	eventFile    string
	inputs       []string
	jobs         []string
	event        string
	dir          string
	help         bool
	checkSandbox bool
	noLint       bool
}

func newFlagSet(o *options) *pflag.FlagSet {
	fs := pflag.NewFlagSet("run", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.SortFlags = false
	fs.BoolVarP(&o.list, "list", "l", false, "print the jobs and matrix cells that would run, without running them")
	fs.BoolVar(&o.json, "json", false, "print --list as JSON")
	fs.StringArrayVarP(&o.jobs, "job", "j", nil, "run only this job and the jobs it needs (repeatable)")
	fs.StringVarP(&o.event, "event", "e", "push", "name of the event that triggers the workflow")
	fs.StringVar(&o.eventFile, "event-file", "", "JSON file with the event payload (github.event)")
	fs.StringArrayVarP(&o.inputs, "input", "i", nil, "workflow_dispatch input as NAME=VALUE (repeatable)")
	fs.StringVarP(&o.dir, "directory", "C", "", "run as if started in this directory")
	fs.BoolVar(&o.noLint, "no-lint", false, "do not refuse a workflow that jactionlint reports errors for")
	fs.StringVar(&o.ro.sandbox, "sandbox", "on", "on: run every step in the OS sandbox; off: run without it")
	fs.StringVar(&o.ro.net, "net", "open", "network of the steps: open, none (no internet sockets) or proxy (not available yet)")
	fs.BoolVar(&o.ro.inPlace, "in-place", false, "work in your checkout instead of a copy (steps can change your files)")
	fs.StringArrayVar(&o.ro.passEnv, "pass-env", nil, "let a variable of your environment reach the steps (repeatable)")
	fs.StringArrayVar(&o.ro.allowRead, "allow-read", nil, "let steps read and run files under this directory (repeatable)")
	fs.StringArrayVar(&o.ro.allowWrite, "allow-write", nil, "let steps write under this directory (repeatable)")
	fs.StringArrayVar(&o.ro.secrets, "secret", nil, "a secret as NAME=VALUE (repeatable)")
	fs.StringVar(&o.ro.secretFile, "secret-file", "", "a file of NAME=VALUE secrets")
	fs.StringArrayVar(&o.ro.vars, "var", nil, "a configuration variable as NAME=VALUE for vars.NAME (repeatable)")
	fs.StringVar(&o.ro.envFile, "env-file", "", "a file of NAME=VALUE that sets the default env of the workflow")
	fs.BoolVar(&o.ro.trust, "trust-actions", false, "run actions that are not pinned to a commit SHA (tags and branches are resolved now)")
	fs.StringArrayVar(&o.ro.noShim, "no-shim", nil, "run the real action instead of the built-in replacement, such as actions/checkout (repeatable)")
	fs.BoolVar(&o.ro.debug, "debug", false, "show ::debug:: output and set RUNNER_DEBUG")
	fs.BoolVar(&o.ro.timings, "timings", false, "show how long each step took")
	fs.BoolVar(&o.ro.keep, "keep", false, "keep the run directory under .jactionlint/run")
	fs.BoolVar(&o.checkSandbox, "check-sandbox", false, "report what the sandbox can do on this machine and test it")
	fs.BoolVarP(&o.help, "help", "h", false, "print this help")
	return fs
}

const helpText = `Usage: jactionlint run [OPTIONS] [WORKFLOW]

Run the jobs of a GitHub Actions workflow on this machine.

WORKFLOW is a path or the name of a file in .github/workflows (the extension is optional).
It can be left out when the repository has one workflow, or with --list to show them all.

Options:
`

// Main runs `jactionlint run` and returns the exit status. args are the arguments after "run".
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var o options
	fs := newFlagSet(&o)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %s (try jactionlint run --help)\n", strings.TrimPrefix(err.Error(), "unknown flag: "))
		return ExitUsage
	}
	if o.help {
		fmt.Fprint(stdout, helpText)
		fmt.Fprint(stdout, fs.FlagUsages())
		return ExitOK
	}
	if o.checkSandbox {
		return checkSandbox(stdout, stderr)
	}
	if o.json && !o.list {
		fmt.Fprintln(stderr, "jactionlint run: --json can only be used with --list")
		return ExitUsage
	}
	if len(fs.Args()) > 1 {
		fmt.Fprintln(stderr, "jactionlint run: give one workflow at a time")
		return ExitUsage
	}
	cwd := o.dir
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
			return ExitError
		}
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitError
	}
	root := findRepoRoot(cwd)

	var paths []string
	if len(fs.Args()) == 1 {
		p, err := resolveWorkflow(cwd, root, fs.Arg(0))
		if err != nil {
			fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
			return ExitUsage
		}
		paths = []string{p}
	} else {
		paths, err = listWorkflows(root)
		if err != nil {
			fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
			return ExitError
		}
		if len(paths) == 0 {
			fmt.Fprintf(stderr, "jactionlint run: no workflows in %s\n", filepath.Join(root, ".github", "workflows"))
			return ExitError
		}
		if !o.list && len(paths) > 1 {
			fmt.Fprintf(stderr, "jactionlint run: the repository has %d workflows, name one (jactionlint run --list shows them)\n", len(paths))
			return ExitUsage
		}
	}

	if !o.list {
		return execute(&o, &o.ro, paths[0], root, stdout, stderr)
	}

	in, err := o.inputsFor(root)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitUsage
	}
	var plans []*Plan
	status := ExitOK
	missingJob := false
	for _, p := range paths {
		errw := stderr
		if len(paths) > 1 {
			errw = &missingJobFilter{w: stderr, missing: &missingJob}
		}
		plan, code := loadPlan(p, root, &o, in, errw)
		if code != ExitOK {
			if len(paths) == 1 {
				return code
			}
			// with -j and no workflow named, a workflow that lacks the job is not listed; the others are
			if code != ExitUsage {
				status = code
			}
			continue
		}
		plans = append(plans, plan)
	}
	if len(plans) == 0 && missingJob {
		fmt.Fprintf(stderr, "jactionlint run: no workflow has the job %s\n", strings.Join(o.jobs, ", "))
		return ExitUsage
	}
	if o.json {
		if err := writeJSON(stdout, plans, root, &o); err != nil {
			fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
			return ExitError
		}
		return status
	}
	for i, plan := range plans {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		writeText(stdout, plan, root, &o)
	}
	return status
}

func loadPlan(path, root string, o *options, in *Inputs, stderr io.Writer) (*Plan, int) {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return nil, ExitError
	}
	w, errs := jactionlint.Parse(b)
	if w == nil || len(errs) > 0 {
		for _, e := range errs {
			e.Filepath = relPath(root, path)
			fmt.Fprintln(stderr, e.Error())
		}
		fmt.Fprintf(stderr, "jactionlint run: %s is not a valid workflow\n", relPath(root, path))
		return nil, ExitError
	}
	wf := *in
	wf.applyDispatch(w)
	wf.WorkflowPath = relPath(root, path)
	wf.Workflow = wf.WorkflowPath
	if w.Name != nil {
		wf.Workflow = w.Name.Value
	}
	plan, err := BuildPlan(w, path, o.jobs, BaseEnv(&wf))
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %s: %v\n", relPath(root, path), err)
		if strings.Contains(err.Error(), "has no job") {
			return nil, ExitUsage
		}
		return nil, ExitError
	}
	return plan, ExitOK
}

func relPath(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return p
}

// findRepoRoot walks up from dir to the first directory with a .git entry or a .github/workflows directory.
func findRepoRoot(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if st, err := os.Stat(filepath.Join(d, ".github", "workflows")); err == nil && st.IsDir() {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

func listWorkflows(root string) ([]string, error) {
	dir := filepath.Join(root, ".github", "workflows")
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if ext := filepath.Ext(e.Name()); ext == ".yml" || ext == ".yaml" {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// resolveWorkflow finds the file that the argument names: a path as given, or a file of .github/workflows with or
// without its extension.
func resolveWorkflow(cwd, root, arg string) (string, error) {
	cands := []string{arg}
	if !filepath.IsAbs(arg) {
		cands = []string{filepath.Join(cwd, arg)}
		base := filepath.Join(root, ".github", "workflows", arg)
		cands = append(cands, base, base+".yml", base+".yaml")
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(c)
			if err != nil {
				return "", err
			}
			return abs, nil
		}
	}
	return "", fmt.Errorf("no workflow %q (looked for the path and in .github/workflows)", arg)
}

// missingJobFilter drops the "has no job" message of a workflow when several are listed, and remembers that it happened.
type missingJobFilter struct {
	w       io.Writer
	missing *bool
}

func (f *missingJobFilter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "has no job") {
		*f.missing = true
		return len(p), nil
	}
	return f.w.Write(p)
}

// inputsFor builds the inputs of the contexts from the options.
func (o *options) inputsFor(root string) (*Inputs, error) {
	in := &Inputs{Repo: readRepoInfo(root), Event: o.event, Workspace: root}
	if o.eventFile != "" {
		ev, err := loadEvent(o.eventFile)
		if err != nil {
			return nil, err
		}
		in.EventData = ev
	}
	if len(o.inputs) > 0 {
		in.Inputs = map[string]any{}
		in.Passed = map[string]string{}
		for _, kv := range o.inputs {
			k, v, ok := strings.Cut(kv, "=")
			if !ok || k == "" {
				return nil, fmt.Errorf("--input %q: want NAME=VALUE", kv)
			}
			in.Inputs[k] = v
			in.Passed[k] = v
		}
	}
	return in, nil
}

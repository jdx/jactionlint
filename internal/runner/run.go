package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

// runOptions are the flags of `run` that only matter when steps are executed.
type runOptions struct {
	sandbox    string // on, off
	net        string // open, none, proxy
	inPlace    bool
	passEnv    []string
	allowRead  []string
	allowWrite []string
	secrets    []string
	secretFile string
	vars       []string
	envFile    string
	debug      bool
	timings    bool
	keep       bool
	trust      bool
	noShim     []string
}

// parseKV turns NAME=VALUE flags into a map.
func parseKV(flag string, kvs []string) (map[string]string, error) {
	m := map[string]string{}
	for _, kv := range kvs {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--%s %q: want NAME=VALUE", flag, kv)
		}
		m[k] = v
	}
	return m, nil
}

// parseDotenv reads KEY=VALUE lines; blank lines and # comments are skipped, and a value may be quoted.
func parseDotenv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("%s:%d: want KEY=VALUE", path, n)
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		m[strings.TrimSpace(k)] = v
	}
	return m, sc.Err()
}

// lintWorkflow refuses a workflow that has the errors actionlint reports, with the diagnostics of the linter.
func lintWorkflow(path, root string, stderr io.Writer) bool {
	project, err := jactionlint.NewProject(root)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return false
	}
	l, err := jactionlint.NewLinter(io.Discard, &jactionlint.LinterOptions{Profile: jactionlint.ProfileCorrectness, MinSeverity: jactionlint.SeverityError})
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return false
	}
	errs, err := l.LintFile(path, project)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return false
	}
	bad := 0
	for _, e := range errs {
		if e.Severity == jactionlint.SeverityError {
			e.Filepath = relPath(root, path)
			fmt.Fprintln(stderr, e.String())
			bad++
		}
	}
	if bad > 0 {
		fmt.Fprintf(stderr, "jactionlint run: %s has %d error(s), so it would not run on GitHub either. Fix them, or pass --no-lint\n", relPath(root, path), bad)
		return false
	}
	return true
}

// execute runs the selected jobs of one workflow.
func execute(o *options, ro *runOptions, path, root string, stdout, stderr io.Writer) int {
	secrets, err := parseKV("secret", ro.secrets)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitUsage
	}
	if ro.secretFile != "" {
		fm, err := parseDotenv(ro.secretFile)
		if err != nil {
			fmt.Fprintf(stderr, "jactionlint run: --secret-file: %v\n", err)
			return ExitUsage
		}
		for k, v := range fm {
			if _, ok := secrets[k]; !ok {
				secrets[k] = v
			}
		}
	}
	vars, err := parseKV("var", ro.vars)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitUsage
	}
	var envFile map[string]string
	if ro.envFile != "" {
		if envFile, err = parseDotenv(ro.envFile); err != nil {
			fmt.Fprintf(stderr, "jactionlint run: --env-file: %v\n", err)
			return ExitUsage
		}
	}
	var net sandbox.Net
	switch ro.net {
	case "open", "":
		net = sandbox.NetOpen
	case "none":
		net = sandbox.NetNone
	case "proxy":
		fmt.Fprintln(stderr, "jactionlint run: --net=proxy is not available in this build yet, use --net=open or --net=none")
		return ExitUsage
	default:
		fmt.Fprintf(stderr, "jactionlint run: --net must be open, none or proxy, not %q\n", ro.net)
		return ExitUsage
	}
	useSandbox := true
	switch ro.sandbox {
	case "on", "":
	case "off":
		useSandbox = false
	default:
		fmt.Fprintf(stderr, "jactionlint run: --sandbox must be on or off, not %q\n", ro.sandbox)
		return ExitUsage
	}
	if useSandbox {
		if err := sandbox.Available(net); err != nil {
			fmt.Fprintf(stderr, "jactionlint run: steps cannot run isolated on this machine: %v\n", err)
			fmt.Fprintln(stderr, "jactionlint run: nothing was run. To run without isolation (the steps get your environment and can read your files), pass --sandbox=off")
			return ExitError
		}
	}
	if !o.noLint && !lintWorkflow(path, root, stderr) {
		return ExitError
	}

	in, err := o.inputsFor(root)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitUsage
	}
	in.Vars = map[string]any{}
	for k, v := range vars {
		in.Vars[k] = v
	}
	rd := filepath.Join(root, ".jactionlint")
	_ = rd
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitError
	}
	w, errs := jactionlint.Parse(b)
	if w == nil || len(errs) > 0 {
		for _, e := range errs {
			e.Filepath = relPath(root, path)
			fmt.Fprintln(stderr, e.Error())
		}
		fmt.Fprintf(stderr, "jactionlint run: %s is not a valid workflow\n", relPath(root, path))
		return ExitError
	}
	in.applyDispatch(w)
	in.WorkflowPath = relPath(root, path)
	in.Workflow = in.WorkflowPath
	if w.Name != nil {
		in.Workflow = w.Name.Value
	}
	plan, err := BuildPlan(w, path, o.jobs, BaseEnv(in))
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %s: %v\n", relPath(root, path), err)
		return ExitUsage
	}
	if !listensTo(w, o.event) {
		fmt.Fprintf(stdout, "note: the workflow has no %q trigger, GitHub would not run it for that event\n", o.event)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := &Config{
		Root: root, In: in, Secrets: secrets, EnvFile: envFile,
		Sandbox: useSandbox, Net: net, InPlace: ro.inPlace, PassEnv: ro.passEnv, AllowRead: ro.allowRead, AllowWrite: ro.allowWrite,
		TrustActions: ro.trust, NoShim: ro.noShim, Debug: ro.debug, Timings: ro.timings, Keep: ro.keep, ParentEnv: os.Environ(), Out: stdout,
	}
	if !useSandbox {
		fmt.Fprintln(stdout, "note: running without the sandbox (--sandbox=off): steps can read and write what you can")
	}
	eng := NewEngine(cfg, plan)
	ok, err := eng.Run(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitError
	}
	if !ok {
		return ExitFailure
	}
	return ExitOK
}

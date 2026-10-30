package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/eval"
	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

// fileSet is the set of files a step writes to talk to the runner.
type fileSet struct {
	dir                               string
	env, path, output, state, summary string
	event                             string
}

func (f fileSet) writable() []string {
	return []string{f.env, f.path, f.output, f.state, f.summary}
}

func (jr *jobRun) newFileSet(n int) (fileSet, error) {
	dir := filepath.Join(jr.rd.Files, fmt.Sprintf("step-%d", n))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fileSet{}, err
	}
	f := fileSet{dir: dir, event: filepath.Join(jr.rd.Files, "event.json")}
	f.env, f.path, f.output = filepath.Join(dir, "env"), filepath.Join(dir, "path"), filepath.Join(dir, "output")
	f.state, f.summary = filepath.Join(dir, "state"), filepath.Join(dir, "summary.md")
	for _, p := range f.writable() {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			return f, err
		}
	}
	if _, err := os.Stat(f.event); err != nil {
		ev := jr.env.Contexts["github"].(map[string]any)["event"]
		b, _ := json.Marshal(ev)
		if err := os.WriteFile(f.event, b, 0o644); err != nil {
			return f, err
		}
	}
	return f, nil
}

// effects is what a step changes in the job, collected while it runs and applied when it is joined.
type effects struct {
	env      map[string]string
	envKeys  []string
	path     []string
	outputs  map[string]string
	state    map[string]string
	summary  string
	warnings []string
	hint     string
	errors   int
	notices  int
}

func (f *effects) warn(format string, a ...any) {
	f.warnings = append(f.warnings, fmt.Sprintf(format, a...))
}

func (f *effects) report(pr *Printer, indent int) {
	for _, w := range f.warnings {
		pr.Note(indent, "warning: %s", w)
	}
}

// cmdState is the state of the workflow-command parser of a step.
type cmdState struct {
	stop     string // between ::stop-commands::TOKEN and ::TOKEN::
	denied   bool   // the output mentions a permission error
	notFound bool
}

func (jr *jobRun) execRun(n int, step *jactionlint.Step, ex *jactionlint.ExecRun, ev *eval.Env, emit func(string), ctx context.Context) (stepResult, *effects) {
	e := jr.e
	cfg := e.cfg
	eff := &effects{env: map[string]string{}, outputs: map[string]string{}, state: map[string]string{}}
	fail := func(note string) (stepResult, *effects) {
		emit("error: " + note)
		return stepResult{Outcome: "failure", Note: note}, eff
	}

	script, err := jr.interp(ev, ex.Run.Value)
	if err != nil {
		return fail(err.Error())
	}
	files, err := jr.newFileSet(n)
	if err != nil {
		return fail("cannot prepare the step files: " + err.Error())
	}

	// environment: workflow < job < GITHUB_ENV < step
	merged := map[string]string{}
	for k, v := range jr.baseEnv {
		merged[k] = v
	}
	for k, v := range jr.ghEnv {
		merged[k] = v
	}
	stepEnv := map[string]string{}
	if err := jr.addEnv(stepEnv, step.Env, ev); err != nil {
		return fail(err.Error())
	}
	for k, v := range stepEnv {
		merged[k] = v
	}
	for _, v := range merged {
		_ = v
	}

	extra := jr.rd.BaseEnv()
	for k, v := range githubEnv(cfg.In, jr, files) {
		extra[k] = v
	}
	if cfg.Debug {
		extra["RUNNER_DEBUG"] = "1"
	}
	for k, v := range merged {
		extra[k] = v
	}
	parentPath := ""
	for _, kv := range cfg.ParentEnv {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			parentPath = v
		}
	}
	if v, ok := extra["PATH"]; ok {
		parentPath = v
	}
	finalPath := jr.pathWith(parentPath)
	extra["PATH"] = finalPath
	env := sandbox.ScrubEnv(cfg.ParentEnv, append(append([]string{}, sandbox.DefaultPassEnv...), cfg.PassEnv...), extra)

	// shell and working directory
	shellName := jr.defaults.shell
	if ex.Shell != nil {
		shellName = ex.Shell.Value
	}
	if shellName, err = jr.interp(ev, shellName); err != nil {
		return fail(err.Error())
	}
	spec, err := resolveShell(shellName, onPath(sandbox.PathDirs(finalPath)))
	if err != nil {
		return fail(err.Error())
	}
	wd := jr.defaults.workdir
	if ex.WorkingDirectory != nil {
		wd = ex.WorkingDirectory.Value
	}
	if wd, err = jr.interp(ev, wd); err != nil {
		return fail(err.Error())
	}
	dir := jr.work
	if wd != "" {
		dir = wd
		if !filepath.IsAbs(wd) {
			dir = filepath.Join(jr.work, wd)
		}
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fail(fmt.Sprintf("the working directory %s does not exist", wd))
	}

	scriptPath := filepath.Join(jr.rd.Temp, fmt.Sprintf("step-%d%s", n, spec.ext))
	if err := os.WriteFile(scriptPath, []byte(spec.prelude+script+spec.postlude), 0o600); err != nil {
		return fail("cannot write the script: " + err.Error())
	}

	pol := jr.policyFor(files, dir, finalPath)
	pol.Argv = spec.command(scriptPath)
	pol.Env = env
	pol.Dir = dir

	timeout := time.Duration(0)
	if step.TimeoutMinutes != nil && step.TimeoutMinutes.Expression == nil && step.TimeoutMinutes.Value > 0 {
		timeout = time.Duration(step.TimeoutMinutes.Value * float64(time.Minute))
	}
	sctx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		sctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cs := &cmdState{}
	maskedEmit := func(l string) { emit(e.masker.Mask(l)) }
	stdout := &lineSink{handle: func(l string) { jr.onStdout(cs, l, maskedEmit, eff) }}
	stderr := &lineSink{handle: func(l string) { noteDenied(cs, l); maskedEmit(l) }}

	var code int
	if cfg.Sandbox {
		code, err = sandbox.Run(sctx, pol, sandbox.IO{Stdout: stdout, Stderr: stderr})
	} else {
		code, err = sandbox.RunUnrestricted(sctx, pol, sandbox.IO{Stdout: stdout, Stderr: stderr})
	}
	stdout.Close()
	stderr.Close()

	res := stepResult{Outcome: "success"}
	var se *sandbox.SetupError
	switch {
	case errors.As(err, &se):
		res = stepResult{Outcome: "failure", Note: se.Error()}
		emit("error: " + se.Error())
		if cfg.Sandbox {
			eff.hint = "the step did not start because the sandbox could not be applied. Run jactionlint run --check-sandbox, or use --sandbox=off to run without it"
		}
	case err != nil:
		res = stepResult{Outcome: "failure", Note: err.Error()}
	case sctx.Err() == context.DeadlineExceeded && ctx.Err() == nil && timeout > 0:
		res = stepResult{Outcome: "failure", Note: fmt.Sprintf("timed out after %s minutes", eval.FormatNumber(timeout.Minutes()))}
	case ctx.Err() != nil:
		res = stepResult{Outcome: "cancelled"}
	case code != 0:
		res = stepResult{Outcome: "failure", Note: fmt.Sprintf("exit status %d", code)}
	}
	if res.Outcome == "failure" && eff.hint == "" {
		switch {
		case cs.denied && cfg.Sandbox:
			eff.hint = "the sandbox only lets a step write to its workspace copy, home, temp directories and /tmp, and read the system directories and PATH. Add a directory with --allow-read DIR or --allow-write DIR, or use --sandbox=off"
		case cs.notFound:
			eff.hint = "a program was not found. Steps see only PATH from your shell plus the tools that the steps install; run the setup step, or check that the tool is on PATH (--check-sandbox lists what a step may use)"
		}
	}

	jr.collect(files, eff)
	return res, eff
}

func noteDenied(cs *cmdState, line string) {
	l := strings.ToLower(line)
	if strings.Contains(l, "permission denied") || strings.Contains(l, "operation not permitted") || strings.Contains(l, "read-only file system") {
		cs.denied = true
	}
	if strings.Contains(l, "command not found") {
		cs.notFound = true
	}
}

// onStdout interprets a line of the standard output of a step: a workflow command, or text to show.
func (jr *jobRun) onStdout(cs *cmdState, line string, emit func(string), eff *effects) {
	noteDenied(cs, line)
	if cs.stop != "" {
		if line == "::"+cs.stop+"::" {
			cs.stop = ""
			return
		}
		emit(line)
		return
	}
	c, ok := parseCommand(line)
	if !ok {
		emit(line)
		return
	}
	e := jr.e
	switch c.Name {
	case "debug":
		if e.cfg.Debug {
			emit("debug: " + c.Message)
		}
	case "error", "warning", "notice":
		a := annotation{Level: c.Name, Title: c.Props["title"], File: c.Props["file"], Line: c.Props["line"], Col: c.Props["col"], Msg: c.Message}
		emit(a.String())
		if c.Name == "error" {
			eff.errors++
		} else {
			eff.notices++
		}
	case "group":
		emit("▼ " + c.Message)
	case "endgroup":
	case "add-mask":
		e.masker.Add(c.Message)
	case "stop-commands":
		cs.stop = c.Message
	case "set-output":
		emit("warning: the set-output command is deprecated, write name=value to the file $GITHUB_OUTPUT instead")
		eff.outputs[c.Props["name"]] = c.Message
	case "save-state":
		eff.state[c.Props["name"]] = c.Message
	case "set-env", "add-path":
		emit(fmt.Sprintf("error: the %s command is disabled, write to the file $GITHUB_ENV or $GITHUB_PATH instead", c.Name))
	case "echo", "add-matcher", "remove-matcher":
	default:
		emit(line)
	}
}

// collect reads the files a finished step wrote.
func (jr *jobRun) collect(files fileSet, eff *effects) {
	read := func(path string) string {
		s, err := readFileCommand(path)
		if err != nil {
			eff.warn("%v", err)
			return ""
		}
		return s
	}
	if text := read(files.env); text != "" {
		keys, vals, err := parseKeyValues(text)
		if err != nil {
			eff.warn("GITHUB_ENV: %v", err)
		}
		for _, k := range keys {
			if sandbox.IsDangerousEnv(k) {
				eff.warn("GITHUB_ENV: %s is not allowed to be set for later steps (it changes what code a program loads)", k)
				continue
			}
			if _, ok := eff.env[k]; !ok {
				eff.envKeys = append(eff.envKeys, k)
			}
			eff.env[k] = vals[k]
		}
	}
	if text := read(files.path); text != "" {
		for _, p := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(jr.work, p)
			}
			if !jr.pathAllowed(p) {
				eff.warn("GITHUB_PATH: %s is outside the directories a step may run programs from, so it was not added", p)
				continue
			}
			eff.path = append(eff.path, filepath.Clean(p))
		}
	}
	if text := read(files.output); text != "" {
		keys, vals, err := parseKeyValues(text)
		if err != nil {
			eff.warn("GITHUB_OUTPUT: %v", err)
		}
		for _, k := range keys {
			eff.outputs[k] = vals[k]
		}
	}
	if text := read(files.state); text != "" {
		keys, vals, _ := parseKeyValues(text)
		for _, k := range keys {
			eff.state[k] = vals[k]
		}
	}
	eff.summary = read(files.summary)
	if eff.summary != "" {
		for k, v := range map[string]string{"summary": eff.summary} {
			_ = k
			jr.e.masker.Mask(v)
		}
		eff.summary = jr.e.masker.Mask(eff.summary)
	}
}

// pathAllowed reports whether a step could run programs from the directory: it is under the workspace, the home,
// the temp or tool cache directories, or a directory the default policy lets it execute from.
func (jr *jobRun) pathAllowed(p string) bool {
	pol := sandbox.DefaultPolicy(sandbox.Options{
		Workspace: jr.work, Home: jr.rd.Home, Temp: jr.rd.Temp, ToolCache: jr.rd.ToolCache,
		PathDirs: sandbox.PathDirs(os.Getenv("PATH")), RealHome: userHome(), ExtraRead: jr.e.cfg.AllowRead, ExtraWrite: jr.e.cfg.AllowWrite,
	})
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	for _, list := range [][]string{pol.Exec, pol.Write} {
		for _, d := range list {
			if r, err := filepath.EvalSymlinks(d); err == nil {
				d = r
			}
			if inDir(d, p) {
				return true
			}
		}
	}
	return false
}

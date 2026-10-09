package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

// These variables might be modified by ldflags on building release binaries by GoReleaser. Do not modify manually
var (
	version       = ""
	installedFrom = "installed by building from source"
)

const (
	// ExitStatusSuccessNoProblem is the exit status when the command ran successfully with no problem found.
	ExitStatusSuccessNoProblem = 0
	// ExitStatusSuccessProblemFound is the exit status when the command ran successfully with some problem found.
	ExitStatusSuccessProblemFound = 1
	// ExitStatusInvalidCommandOption is the exit status when parsing command line options failed or the value of
	// an option is invalid (an unknown --profile, --format or --min-severity, a broken --ignore regular expression).
	ExitStatusInvalidCommandOption = 2
	// ExitStatusFailure is the exit status when the command stopped due to some fatal error while checking workflows
	// (no project, an unreadable file or config).
	ExitStatusFailure = 3
)

func getCommandVersion() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "unknown" // Reaches only when jactionlint package is built outside module
	}

	return info.Main.Version
}

// Command represents entire jactionlint command. Given stdin/stdout/stderr are used for input/output.
type Command struct {
	// Stdin is a reader to read input from stdin
	Stdin io.Reader
	// Stdout is a writer to write output to stdout
	Stdout io.Writer
	// Stderr is a writer to write output to stderr
	Stderr io.Writer

	// onRulesCreated is passed to LinterOptions.OnRulesCreated. Tests use it to add rules with fixes.
	onRulesCreated func([]Rule) []Rule
}

// usageError is an error caused by the value of a command line flag (--format, --ignore, ...). The command exits
// with ExitStatusInvalidCommandOption for it, like it does for a value which it validates itself (--profile).
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// fixRequest is what --fix, --diff and --fix-rules ask for.
type fixRequest struct {
	mode   FixMode
	diff   bool
	rules  []string
	result *FixResult
}

func (cmd *Command) runLinter(args []string, opts *LinterOptions, initConfig, migrateConfig, migrateIgnores bool, fix *fixRequest, baselineWrite *optionalValueFlag, onlineFailed *int) ([]*Error, error) {
	l, err := NewLinter(cmd.Stdout, opts)
	if err != nil {
		return nil, err
	}
	defer func() {
		if l.OnlineFailed() {
			*onlineFailed = l.OnlineSkipped()
		}
	}()

	if initConfig {
		return nil, l.GenerateDefaultConfig("")
	}
	if migrateConfig {
		return nil, l.MigrateConfig("")
	}

	if baselineWrite.set {
		if len(args) == 1 && args[0] == "-" {
			return nil, &usageError{errors.New("--baseline-write cannot be used with stdin because the baseline would not know the file")}
		}
		if fix.mode != 0 {
			return nil, &usageError{errors.New("--baseline-write cannot be combined with --fix")}
		}
		if args == nil {
			args = []string{}
		}
		res, err := l.WriteBaseline(args, baselineWrite.value)
		if err != nil {
			return nil, err
		}
		state := "Wrote"
		if !res.Changed {
			state = "Baseline is up to date:"
		}
		fmt.Fprintf(cmd.Stdout, "%s %s for %s in %s\n", state, countNoun(res.Entries, "entry"), countNoun(res.Files, "file"), displayPath(opts.WorkingDir, res.Path))
		if !res.Applied {
			// A plain run does not read the baseline unless the configuration or --baseline says so
			fmt.Fprintf(cmd.Stdout, "A plain run does not use the baseline yet. Pass --baseline, or put this line in %s so that every run and hook does:\n\n  baseline: %s\n", res.ConfigFile, res.ConfigValue)
		}
		return nil, nil
	}
	if migrateIgnores {
		return nil, l.MigrateIgnores(args)
	}

	if fix.mode != 0 {
		if len(args) == 1 && args[0] == "-" {
			return nil, &usageError{errors.New("--fix cannot be used with stdin because the fixed file would not be saved")}
		}
		fo := FixOptions{Mode: fix.mode, Rules: fix.rules, DryRun: fix.diff}
		var res *FixResult
		if len(args) == 0 {
			res, err = l.FixRepositoryWithOptions("", fo)
		} else {
			res, err = l.FixFilesWithOptions(args, nil, fo)
		}
		if err != nil {
			return nil, err
		}
		fix.result = res
		return res.Errors, nil
	}

	if len(args) == 0 {
		return l.LintRepository("")
	}

	if len(args) == 1 && args[0] == "-" {
		return l.LintStdin(cmd.Stdin)
	}

	return l.LintFiles(args, nil)
}

// Main is main function of jactionlint. It takes command line arguments as string slice and returns
// exit status. The args should be entire arguments including the program name, usually given via
// os.Args.
func (cmd *Command) Main(args []string) int {
	f := newCommandFlags()
	if err := f.parse(args[1:]); err != nil {
		fmt.Fprintf(cmd.Stderr, "jactionlint: %s (try --help)\n", err)
		return ExitStatusInvalidCommandOption
	}
	if f.help {
		printHelp(cmd.Stdout)
		return ExitStatusSuccessNoProblem
	}
	opts := f.opts
	flags := f.fs
	ver := f.ver
	minSeverity, profileName, strictExit := f.minSeverity, f.profileName, f.strictExit
	baseline, baselineWrite := f.baseline, f.baselineWrite
	diff, fix := f.diff, f.fix
	online, onlineAllow, onlineDeny := f.online, f.onlineAllow, f.onlineDeny
	onlineTTL, onlineMaxWait := f.onlineTTL, f.onlineMaxWait

	if ver {
		fmt.Fprintf(
			cmd.Stdout,
			"%s\n%s\nbuilt with %s compiler for %s/%s\n",
			getCommandVersion(),
			installedFrom,
			runtime.Version(),
			runtime.GOOS,
			runtime.GOARCH,
		)
		return ExitStatusSuccessNoProblem
	}

	sev, err := ParseSeverity(minSeverity)
	if err != nil || sev == SeverityOff {
		fmt.Fprintf(cmd.Stderr, "invalid value %q for --min-severity. available values are \"info\", \"warn\" and \"error\"\n", minSeverity)
		return ExitStatusInvalidCommandOption
	}
	opts.MinSeverity = sev
	if profileName != "" {
		p, err := ParseProfile(profileName)
		if err != nil {
			fmt.Fprintf(cmd.Stderr, "invalid value %q for --profile. available values are \"correctness\", \"default\" and \"pedantic\"\n", profileName)
			return ExitStatusInvalidCommandOption
		}
		opts.Profile = p
	}
	if baselineWrite.set && (baseline.set || f.noBaseline || opts.BaselineCheck) {
		fmt.Fprintln(cmd.Stderr, "--baseline-write cannot be combined with --baseline or --baseline-check: it records every finding")
		return ExitStatusInvalidCommandOption
	}
	opts.Baseline = baseline.set
	opts.BaselineFile = baseline.value
	opts.NoBaseline = f.noBaseline
	if baseline.set && f.noBaseline {
		fmt.Fprintln(cmd.Stderr, "--baseline and --no-baseline cannot be combined")
		return ExitStatusInvalidCommandOption
	}
	if online.set && f.noOnline {
		fmt.Fprintln(cmd.Stderr, "--online and --no-online cannot be combined")
		return ExitStatusInvalidCommandOption
	}
	opts.IgnorePatterns = f.ignorePats
	opts.OnRulesCreated = cmd.onRulesCreated
	opts.RunHints = !f.noHints && !HintsDisabledByEnv()
	opts.LogWriter = cmd.Stderr
	if flags.Changed("online-cache-ttl") {
		opts.OnlineOptions.CacheTTL = &onlineTTL
	}
	if flags.Changed("online-max-wait") {
		opts.OnlineOptions.MaxRateLimitWait = &onlineMaxWait
	}
	if f.noOnline {
		opts.OnlineOff = true
	}
	if online.set {
		opts.Online = true
		opts.OnlineOptions.Mode = online.mode
		opts.OnlineOptions.ModeSet = true
	}
	if len(onlineAllow) > 0 {
		opts.OnlineOptions.Allow = onlineAllow
	}
	if len(onlineDeny) > 0 {
		opts.OnlineOptions.Deny = onlineDeny
	}
	if err := opts.OnlineOptions.validate(); err != nil {
		fmt.Fprintf(cmd.Stderr, "invalid online option: %s\n", err)
		return ExitStatusInvalidCommandOption
	}
	if opts.Online || opts.OnlineOptions.Mode != OnlineModeDefault {
		// Stop the lookups, instead of killing the process in the middle of a cache write, on Ctrl-C
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		opts.Context = ctx
	}

	if f.color.set {
		opts.Color = f.color.kind
	}
	if f.noColor {
		opts.Color = ColorOptionKindNever
	}

	req := &fixRequest{mode: fix.mode, diff: diff}
	if diff && req.mode == 0 {
		req.mode = FixModeSafe
	}
	if f.fixRules != "" {
		if req.mode == 0 {
			fmt.Fprintln(cmd.Stderr, "--fix-rules can be used only with --fix or --diff")
			return ExitStatusInvalidCommandOption
		}
		for _, id := range strings.Split(f.fixRules, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := ruleIndex[id]; !ok {
				fmt.Fprintf(cmd.Stderr, "unknown rule ID %q in --fix-rules%s\n", id, suggestRuleID(id))
				return ExitStatusInvalidCommandOption
			}
			req.rules = append(req.rules, id)
		}
	}
	var onlineFailed int
	errs, err := cmd.runLinter(flags.Args(), &opts, f.initConfig, f.migrateConfig, f.migrateIgnores, req, &baselineWrite, &onlineFailed)
	if err != nil {
		fmt.Fprintln(cmd.Stderr, err.Error())
		var ue *usageError
		if errors.As(err, &ue) {
			return ExitStatusInvalidCommandOption // the value of a flag is wrong, not the workflows
		}
		return ExitStatusFailure
	}
	if req.result != nil && len(req.result.Failures) > 0 {
		return ExitStatusFailure
	}
	if onlineFailed > 0 {
		fmt.Fprintf(cmd.Stderr, "online=strict: %d GitHub lookups were skipped, so the online checks are incomplete\n", onlineFailed)
		return ExitStatusFailure
	}
	if req.result != nil && diff && req.result.Diff != "" {
		return ExitStatusSuccessProblemFound
	}
	return exitStatusOf(errs, strictExit)
}

// exitStatusOf returns the exit status for the errors found. Only errors of error level make the
// status 1 unless strict is true, which counts every error.
func exitStatusOf(errs []*Error, strict bool) int {
	for _, e := range errs {
		if strict || e.Severity >= SeverityError {
			return ExitStatusSuccessProblemFound // Linter found some issues, yay!
		}
	}
	return ExitStatusSuccessNoProblem
}

// displayPath shows a path relative to the working directory when it is inside it.
func displayPath(wd, p string) string {
	if wd == "" {
		wd, _ = os.Getwd()
	}
	if r, err := filepath.Rel(wd, p); wd != "" && err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

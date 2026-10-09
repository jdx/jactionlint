package jactionlint

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
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
	// an option is invalid (an unknown -profile, -format or -min-severity, a broken -ignore regular expression).
	ExitStatusInvalidCommandOption = 2
	// ExitStatusFailure is the exit status when the command stopped due to some fatal error while checking workflows
	// (no project, an unreadable file or config).
	ExitStatusFailure = 3
)

func printUsageHeader(out io.Writer) {
	v := getCommandVersion()
	b := "main"
	if regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(v) {
		b = "v" + v
	}

	fmt.Fprintf(out, `Usage: jactionlint [FLAGS] [FILES...] [-]

  jactionlint is a linter for GitHub Actions workflow files.

  To check all YAML files in current repository, just run jactionlint without
  arguments. It automatically finds the nearest '.github/workflows' directory:

    $ jactionlint

  To check specific files, pass the file paths as arguments:

    $ jactionlint file1.yaml file2.yaml

  To check content which is not saved in file yet (e.g. output from some
  command), pass - argument. It reads stdin and checks it as workflow file:

    $ jactionlint -

  To serialize errors, use -format option. json, jsonl, sarif (for code scanning
  and tools like hk), gcc and github (GitHub Actions annotations) are available:

    $ jactionlint -format sarif

  A Go template also allows to format error messages flexibly:

    $ jactionlint -format '{{json .}}'

Documents:

  - List of checks: https://github.com/jdx/jactionlint/tree/%s/docs/checks.md
  - Rule IDs:       https://github.com/jdx/jactionlint/tree/%s/docs/rules.md
  - Usage:          https://github.com/jdx/jactionlint/tree/%s/docs/usage.md
  - Configuration:  https://github.com/jdx/jactionlint/tree/%s/docs/config.md

Flags:
`, b, b, b, b)
}

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

// usageError is an error caused by the value of a command line flag (-format, -ignore, ...). The command exits
// with ExitStatusInvalidCommandOption for it, like it does for a value which it validates itself (-profile).
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// fixRequest is what -fix, -diff and -rules ask for.
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
			return nil, &usageError{errors.New("-baseline-write cannot be used with stdin because the baseline would not know the file")}
		}
		if fix.mode != 0 {
			return nil, &usageError{errors.New("-baseline-write cannot be combined with -fix")}
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
			// A plain run does not read the baseline unless the configuration or -baseline says so
			fmt.Fprintf(cmd.Stdout, "A plain run does not use the baseline yet. Pass -baseline, or put this line in %s so that every run and hook does:\n\n  baseline: %s\n", res.ConfigFile, res.ConfigValue)
		}
		return nil, nil
	}
	if migrateIgnores {
		return nil, l.MigrateIgnores(args)
	}

	if fix.mode != 0 {
		if len(args) == 1 && args[0] == "-" {
			return nil, &usageError{errors.New("-fix cannot be used with stdin because the fixed file would not be saved")}
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

// optionalValueFlag is a flag which can be given without a value (-baseline), with one
// (-baseline=FILE) or turned off (-baseline=false).
type optionalValueFlag struct {
	set   bool
	off   bool
	value string
}

func (f *optionalValueFlag) String() string {
	switch {
	case f.off:
		return "false"
	case f.set && f.value != "":
		return f.value
	case f.set:
		return "true"
	}
	return ""
}

// IsBoolFlag lets the flag be given without a value.
func (f *optionalValueFlag) IsBoolFlag() bool { return true }

func (f *optionalValueFlag) Set(v string) error {
	switch v {
	case "true":
		*f = optionalValueFlag{set: true}
	case "false":
		*f = optionalValueFlag{off: true}
	case "":
		return errors.New("the value must not be empty. omit it to use the default file")
	default:
		*f = optionalValueFlag{set: true, value: v}
	}
	return nil
}

// onlineFlag is the -online flag: a boolean flag which also takes a mode, -online=cache or -online=strict.
type onlineFlag struct {
	set  bool
	off  bool // -online=false: the configuration cannot turn the checks on either
	mode OnlineMode
}

func (f *onlineFlag) String() string {
	if f == nil || !f.set {
		return "false"
	}
	if f.mode == OnlineModeDefault {
		return "true"
	}
	return string(f.mode)
}

func (f *onlineFlag) Set(v string) error {
	switch strings.ToLower(v) {
	case "false", "off", "0":
		*f = onlineFlag{off: true}
		return nil
	}
	m, err := ParseOnlineMode(v)
	if err != nil {
		return err
	}
	*f = onlineFlag{set: true, mode: m}
	return nil
}

// IsBoolFlag makes a bare -online mean -online=true.
func (f *onlineFlag) IsBoolFlag() bool { return true }

// stringListFlag is a repeatable string flag. A value may also hold several comma separated items.
type stringListFlag []string

func (l *stringListFlag) String() string { return strings.Join(*l, ",") }
func (l *stringListFlag) Set(v string) error {
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			*l = append(*l, s)
		}
	}
	return nil
}

type ignorePatternFlags []string

func (i *ignorePatternFlags) String() string {
	return "option for ignore patterns"
}
func (i *ignorePatternFlags) Set(v string) error {
	*i = append(*i, v)
	return nil
}

// Main is main function of jactionlint. It takes command line arguments as string slice and returns
// exit status. The args should be entire arguments including the program name, usually given via
// os.Args.
func (cmd *Command) Main(args []string) int {
	var ver bool
	var opts LinterOptions
	var ignorePats ignorePatternFlags
	var initConfig bool
	var fix fixFlag
	var diff bool
	var fixRules string
	var migrateConfig bool
	var migrateIgnores bool
	var noColor bool
	var noHints bool
	var color bool
	var minSeverity string
	var profileName string
	var strictExit bool
	var onlineTTL time.Duration
	var baseline, baselineWrite optionalValueFlag

	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(cmd.Stderr)
	flags.Var(&ignorePats, "ignore", "Rule ID (e.g. unpinned-uses) or regular expression matching to error messages you want to ignore. This flag is repeatable")
	flags.StringVar(&minSeverity, "min-severity", "info", "Hide the errors less severe than this level: info, warn or error")
	flags.StringVar(&profileName, "profile", "", "Rule profile: correctness (what actionlint checks), default or pedantic. It overrides \"profile\" of the config file. Each profile includes the rules of the one before it. Without this flag the config file decides, else the default profile applies, which is stricter than actionlint. See the rules documentation")
	flags.BoolVar(&strictExit, "strict-exit", false, "Exit with status 1 also when only errors of warn or info level are found. By default only errors of error level make the exit status 1")
	flags.StringVar(&opts.Shellcheck, "shellcheck", "shellcheck", "Command name or file path of \"shellcheck\" external command. If empty, shellcheck integration will be disabled")
	flags.StringVar(&opts.Pyflakes, "pyflakes", "pyflakes", "Command name or file path of \"pyflakes\" external command. If empty, pyflakes integration will be disabled")
	flags.BoolVar(&opts.Oneline, "oneline", false, "Use one line per error, without the source excerpt. The same as -format oneline")
	flags.StringVar(&opts.Format, "format", "", "Output format: text (default), oneline, json, jsonl, sarif, gcc, github or summary. A custom template in Go template syntax which has {{ }} is also accepted. See the usage documentation for more details")
	flags.BoolVar(&opts.ShowRuleIDs, "rule-ids", false, "Show the stable rule ID such as unpinned-uses at the end of each error in the text format instead of the kind. The ID is used in the rules of the config file and in -ignore")
	flags.StringVar(&opts.ConfigFile, "config-file", "", "File path to config file")
	flags.BoolVar(&initConfig, "init-config", false, "Generate default config file at .github/jactionlint.yaml in current project")
	flags.Var(&fix, "fix", "Apply the safe automatic fixes to the files and report what remains. -fix=unsafe also applies the fixes which may change the behavior of the workflow. The files are rewritten in place")
	flags.BoolVar(&diff, "diff", false, "Print the changes -fix would make as a unified diff on stdout and do not write the files (implies -fix). The errors which remain go to stderr. Exits with 1 when there is a diff or an error remains")
	flags.StringVar(&fixRules, "rules", "", "Only with -fix or -diff: apply only the fixes of these rule IDs, separated by commas (e.g. -fix -rules missing-timeout,artipacked). It does not choose which rules run (use -profile or \"rules\" in the config file for that). The \"fix.rules\" key of the config file does the same")
	flags.BoolVar(&migrateConfig, "migrate-config", false, "Rewrite the deprecated keys of the config file (.github/jactionlint.yaml or the file of -config-file) into the \"rules\" mapping")
	flags.BoolVar(&migrateIgnores, "migrate-ignores", false, "Rewrite the trailing \"# zizmor: ignore[...]\" comments of the files (the workflows of the project by default) into \"# jactionlint ignore=...\" comments. jactionlint also honors the zizmor comments as they are")
	var online onlineFlag
	var onlineAllow, onlineDeny stringListFlag
	var onlineMaxWait time.Duration
	flags.Var(&online, "online", "Enable the checks which query the GitHub API (impostor-commit, known-vulnerable-actions, ref-confusion, stale-action-refs, archived-uses, ref-version-mismatch) and let -fix pin tags to commit SHAs. -online=cache uses only the answers in the disk cache and never the network, -online=strict also fails (exit status 3) when a lookup had to be skipped. A lookup which fails (404, 403, server error, timeout, rate limit) is skipped with one warning per kind and does not change the exit status. The token is read from -online-token-env, -online-token-file, GITHUB_TOKEN, GH_TOKEN or \"gh auth token\". Nothing uses the network without this flag")
	flags.DurationVar(&onlineTTL, "online-cache-ttl", defaultOnlineCacheTTL, "How long -online uses an answer of the GitHub API from the cache in $XDG_CACHE_HOME/jactionlint without asking GitHub whether it changed. 0 checks every answer")
	flags.StringVar(&opts.OnlineOptions.APIURL, "online-api-url", "", "URL of the REST API of a GitHub Enterprise Server such as https://ghe.example.com/api/v3. The default is $GITHUB_API_URL, else derived from $GITHUB_SERVER_URL or $GH_HOST, else api.github.com. The token is sent only to this host")
	flags.StringVar(&opts.OnlineOptions.TokenEnv, "online-token-env", "", "Name of the environment variable which holds the GitHub token, read before GITHUB_TOKEN and GH_TOKEN")
	flags.StringVar(&opts.OnlineOptions.TokenFile, "online-token-file", "", "File which holds the GitHub token, read before GITHUB_TOKEN and GH_TOKEN")
	flags.Var(&onlineAllow, "online-allow", "Look up only the repositories matching this \"owner/repo\" pattern (\"*\" is a wildcard, e.g. \"actions/*\"). This flag is repeatable")
	flags.Var(&onlineDeny, "online-deny", "Never look up the repositories matching this \"owner/repo\" pattern, e.g. private or internal actions. This flag is repeatable")
	flags.DurationVar(&onlineMaxWait, "online-max-wait", defaultOnlineMaxWait, "The longest to wait for a GitHub rate limit to reset. A limit which resets later skips the lookups. 0 never waits")
	flags.Var(&baseline, "baseline", "Hide the findings recorded in the baseline file (default "+DefaultBaselineFile+" in the repository). -baseline=FILE reads another file and -baseline=false ignores a baseline that the config enables. See -baseline-write")
	flags.Var(&baselineWrite, "baseline-write", "Record the current findings as the baseline (default file "+DefaultBaselineFile+") and exit with status 0. -baseline-write=FILE writes another file. With file arguments only the entries of those files are refreshed. Run it in the same environment as the CI (rules, -online, shellcheck). A plain run applies the baseline only when the config file says \"baseline: auto\" or -baseline is given; the command prints the line to add")
	flags.BoolVar(&opts.BaselineCheck, "baseline-check", false, "Report the baseline entries which match no finding any more as unused-baseline-entry (info; set its level to error in \"rules\" to fail on them). Implies -baseline")
	flags.BoolVar(&opts.SARIFHideBaselined, "sarif-hide-baselined", false, "Leave the findings accepted by the baseline out of -format sarif. By default they are in the log as suppressed results. Use it for tools like hk which do not read suppressions")
	flags.BoolVar(&noHints, "no-hints", false, "Do not print the line at the end of a text run with many findings that says how to count them, adopt them gradually and get the checks of actionlint only. It is printed only to a terminal or in CI. JACTIONLINT_NO_HINTS=1 does the same")
	flags.BoolVar(&noColor, "no-color", false, "Disable colorful output")
	flags.BoolVar(&color, "color", false, "Always enable colorful output. This is useful to force colorful outputs")
	flags.BoolVar(&opts.Verbose, "verbose", false, "Enable verbose output")
	flags.BoolVar(&opts.Debug, "debug", false, "Enable debug output (for development)")
	flags.BoolVar(&ver, "version", false, "Show version and how this binary was installed")
	flags.StringVar(&opts.StdinFileName, "stdin-filename", "<stdin>", "File name when reading input from stdin")
	flags.Usage = func() {
		printUsageHeader(cmd.Stderr)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			// When -h or -help
			return ExitStatusSuccessNoProblem
		}
		return ExitStatusInvalidCommandOption
	}

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
		fmt.Fprintf(cmd.Stderr, "invalid value %q for -min-severity. available values are \"info\", \"warn\" and \"error\"\n", minSeverity)
		return ExitStatusInvalidCommandOption
	}
	opts.MinSeverity = sev
	if profileName != "" {
		p, err := ParseProfile(profileName)
		if err != nil {
			fmt.Fprintf(cmd.Stderr, "invalid value %q for -profile. available values are \"correctness\", \"default\" and \"pedantic\"\n", profileName)
			return ExitStatusInvalidCommandOption
		}
		opts.Profile = p
	}
	if baselineWrite.set && (baseline.set || baseline.off || opts.BaselineCheck) {
		fmt.Fprintln(cmd.Stderr, "-baseline-write cannot be combined with -baseline or -baseline-check: it records every finding")
		return ExitStatusInvalidCommandOption
	}
	opts.Baseline = baseline.set
	opts.BaselineFile = baseline.value
	opts.NoBaseline = baseline.off
	opts.IgnorePatterns = ignorePats
	opts.OnRulesCreated = cmd.onRulesCreated
	opts.RunHints = !noHints && !HintsDisabledByEnv()
	opts.LogWriter = cmd.Stderr
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "online-cache-ttl":
			opts.OnlineOptions.CacheTTL = &onlineTTL
		case "online-max-wait":
			opts.OnlineOptions.MaxRateLimitWait = &onlineMaxWait
		}
	})
	if online.off {
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

	if color {
		opts.Color = ColorOptionKindAlways
	}
	if noColor {
		opts.Color = ColorOptionKindNever
	}

	req := &fixRequest{mode: fix.mode, diff: diff}
	if diff && req.mode == 0 {
		req.mode = FixModeSafe
	}
	if fixRules != "" {
		if req.mode == 0 {
			fmt.Fprintln(cmd.Stderr, "-rules can be used only with -fix or -diff")
			return ExitStatusInvalidCommandOption
		}
		for _, id := range strings.Split(fixRules, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := ruleIndex[id]; !ok {
				fmt.Fprintf(cmd.Stderr, "unknown rule ID %q in -rules%s\n", id, suggestRuleID(id))
				return ExitStatusInvalidCommandOption
			}
			req.rules = append(req.rules, id)
		}
	}
	var onlineFailed int
	errs, err := cmd.runLinter(flags.Args(), &opts, initConfig, migrateConfig, migrateIgnores, req, &baselineWrite, &onlineFailed)
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

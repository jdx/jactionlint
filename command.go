package jactionlint

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
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
	// ExitStatusInvalidCommandOption is the exit status when parsing command line options failed.
	ExitStatusInvalidCommandOption = 2
	// ExitStatusFailure is the exit status when the command stopped due to some fatal error while checking workflows.
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

// fixRequest is what -fix, -diff and -rules ask for.
type fixRequest struct {
	mode   FixMode
	diff   bool
	rules  []string
	result *FixResult
}

func (cmd *Command) runLinter(args []string, opts *LinterOptions, initConfig, migrateConfig bool, fix *fixRequest) ([]*Error, error) {
	l, err := NewLinter(cmd.Stdout, opts)
	if err != nil {
		return nil, err
	}

	if initConfig {
		return nil, l.GenerateDefaultConfig("")
	}
	if migrateConfig {
		return nil, l.MigrateConfig("")
	}

	if fix.mode != 0 {
		if len(args) == 1 && args[0] == "-" {
			return nil, errors.New("-fix cannot be used with stdin because the fixed file would not be saved")
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
	var noColor bool
	var color bool
	var minSeverity string
	var strictExit bool
	var onlineTTL time.Duration

	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(cmd.Stderr)
	flags.Var(&ignorePats, "ignore", "Rule ID (e.g. unpinned-uses) or regular expression matching to error messages you want to ignore. This flag is repeatable")
	flags.StringVar(&minSeverity, "min-severity", "info", "Hide the errors less severe than this level: info, warn or error")
	flags.BoolVar(&strictExit, "strict-exit", false, "Exit with status 1 also when only errors of warn or info level are found. By default only errors of error level make the exit status 1")
	flags.StringVar(&opts.Shellcheck, "shellcheck", "shellcheck", "Command name or file path of \"shellcheck\" external command. If empty, shellcheck integration will be disabled")
	flags.StringVar(&opts.Pyflakes, "pyflakes", "pyflakes", "Command name or file path of \"pyflakes\" external command. If empty, pyflakes integration will be disabled")
	flags.BoolVar(&opts.Oneline, "oneline", false, "Use one line per one error. Useful for reading error messages from programs")
	flags.StringVar(&opts.Format, "format", "", "Output format: text (default), oneline, json, jsonl, sarif, gcc or github. A custom template in Go template syntax which has {{ }} is also accepted. See the usage documentation for more details")
	flags.BoolVar(&opts.ShowRuleIDs, "rule-ids", false, "Show the stable rule ID such as unpinned-uses at the end of each error in the text format instead of the kind. The ID is used in the rules of the config file and in -ignore")
	flags.StringVar(&opts.ConfigFile, "config-file", "", "File path to config file")
	flags.BoolVar(&initConfig, "init-config", false, "Generate default config file at .github/jactionlint.yaml in current project")
	flags.Var(&fix, "fix", "Apply the safe automatic fixes to the files and report what remains. -fix=unsafe also applies the fixes which may change the behavior of the workflow. The files are rewritten in place")
	flags.BoolVar(&diff, "diff", false, "Print the changes -fix would make as a unified diff on stdout and do not write the files (implies -fix). The errors which remain go to stderr. Exits with 1 when there is a diff or an error remains")
	flags.StringVar(&fixRules, "rules", "", "With -fix or -diff, apply only the fixes of these rule IDs, separated by commas (e.g. -fix -rules missing-timeout,artipacked). The \"fix.rules\" key of the config file does the same")
	flags.BoolVar(&migrateConfig, "migrate-config", false, "Rewrite the deprecated keys of the config file (.github/jactionlint.yaml or the file of -config-file) into the \"rules\" mapping")
	flags.BoolVar(&opts.Online, "online", false, "Enable the checks which query the GitHub API (impostor-commit, known-vulnerable-actions, ref-confusion, stale-action-refs, archived-uses, ref-version-mismatch) and let -fix pin tags to commit SHAs. The token is read from GITHUB_TOKEN or GH_TOKEN. Nothing uses the network without this flag")
	flags.DurationVar(&onlineTTL, "online-cache-ttl", time.Hour, "How long -online uses an answer of the GitHub API from the cache in $XDG_CACHE_HOME/jactionlint without asking GitHub whether it changed. 0 checks every answer")
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
	opts.IgnorePatterns = ignorePats
	opts.OnRulesCreated = cmd.onRulesCreated
	opts.LogWriter = cmd.Stderr
	if onlineTTL <= 0 {
		opts.OnlineCacheTTL = -1
	} else {
		opts.OnlineCacheTTL = onlineTTL
	}
	if opts.Online {
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
	errs, err := cmd.runLinter(flags.Args(), &opts, initConfig, migrateConfig, req)
	if err != nil {
		fmt.Fprintln(cmd.Stderr, err.Error())
		return ExitStatusFailure
	}
	if req.result != nil {
		if len(req.result.Failures) > 0 {
			return ExitStatusFailure
		}
		if diff && req.result.Diff != "" {
			return ExitStatusSuccessProblemFound
		}
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

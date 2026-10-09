package jactionlint

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// This file is the command line surface of jactionlint: the POSIX/GNU option table, the parser setup and the
// help text. Command.Main in command.go runs what the options ask for. The library API (Command.Main,
// LinterOptions) does not depend on the spelling of the options.
//
// The options follow the GNU conventions: --long-name (kebab-case) with --opt=value or --opt value, short
// options which bundle (-fq) and take a value attached or separate (-cfile, -c file), "--" which ends the
// options and a lone "-" operand which means stdin. The single-dash long options of v1 (-format, -fix, ...) are
// not accepted any more; legacyOptions turns them into an error which names the replacement.

// optionalValueSentinel is what a bare --baseline or --baseline-write stores: no value was given.
const optionalValueSentinel = "\x00default"

// cliOption describes one command line option for the help text, the manual and the tests.
type cliOption struct {
	Group    string // heading in the help output
	Long     string // name without the leading dashes
	Short    string // one letter or empty
	ArgName  string // metavar of the value, empty for a boolean option
	Optional bool   // the value can only be given as --long=VALUE
	Replaces string // the v1 spelling, "" for a new option
	Help     string // text of --help
}

// cliGroups are the headings of the help output in order.
var cliGroups = []string{"General", "Output", "Rules and configuration", "Fixing", "Baseline", "GitHub API (online checks)", "External tools"}

// cliOptions is the table of all options in the order of the help output.
var cliOptions = []cliOption{
	{"General", "help", "h", "", false, "-help", "Show this help and exit"},
	{"General", "version", "V", "", false, "-version", "Show the version and how this binary was installed, then exit"},
	{"General", "verbose", "v", "", false, "-verbose", "Print verbose logs to stderr"},
	{"General", "debug", "", "", false, "-debug", "Print debug logs to stderr (for development)"},
	{"General", "stdin-filename", "", "NAME", false, "-stdin-filename", "File name used in the output when reading stdin (default <stdin>)"},

	{"Output", "format", "f", "FORMAT", false, "-format", "Output format: text (default), oneline, json, jsonl, sarif, gcc, github, summary, or a Go template containing {{ }}"},
	{"Output", "oneline", "", "", false, "-oneline", "One line per finding; same as --format oneline"},
	{"Output", "rule-ids", "", "", false, "-rule-ids", "Accepted for compatibility. The text output always shows the rule ID at the end of each finding"},
	{"Output", "color", "", "WHEN", true, "-color", "Colorize the output: always, never or auto (default). Bare --color means always"},
	{"Output", "no-color", "", "", false, "-no-color", "Same as --color=never"},
	{"Output", "no-hints", "", "", false, "-no-hints", "Do not print the hint line after a text run with many findings"},
	{"Output", "min-severity", "", "LEVEL", false, "-min-severity", "Hide findings less severe than LEVEL: info (default), warn or error"},
	{"Output", "strict-exit", "", "", false, "-strict-exit", "Exit with status 1 also for findings of level warn and info"},

	{"Rules and configuration", "profile", "p", "NAME", false, "-profile", "Rule profile: correctness, default or pedantic. Overrides \"profile\" of the config file"},
	{"Rules and configuration", "config-file", "c", "FILE", false, "-config-file", "Use this config file instead of .github/jactionlint.yaml"},
	{"Rules and configuration", "ignore", "i", "PATTERN", false, "-ignore", "Ignore findings whose rule ID or message matches PATTERN (a rule ID or a regular expression). Repeatable"},
	{"Rules and configuration", "init-config", "", "", false, "-init-config", "Write a default config file .github/jactionlint.yaml and exit"},
	{"Rules and configuration", "migrate-config", "", "", false, "-migrate-config", "Rewrite the deprecated keys of the config file and exit"},
	{"Rules and configuration", "migrate-ignores", "", "", false, "-migrate-ignores", "Rewrite \"# zizmor: ignore[...]\" comments into \"# jactionlint ignore=...\" comments and exit"},

	{"Fixing", "fix", "", "unsafe", true, "-fix", "Apply the safe automatic fixes in place. --fix=unsafe also applies the fixes that may change behavior"},
	{"Fixing", "diff", "", "", false, "-diff", "Print what --fix would change as a unified diff and write nothing (implies --fix)"},
	{"Fixing", "fix-rules", "", "RULES", false, "-rules", "With --fix or --diff: apply only the fixes of these comma separated rule IDs"},

	{"Baseline", "baseline", "", "FILE", true, "-baseline", "Hide the findings recorded in the baseline (default " + DefaultBaselineFile + "). --baseline=FILE reads another file"},
	{"Baseline", "no-baseline", "", "", false, "-baseline=false", "Ignore the baseline even when the config file enables it"},
	{"Baseline", "baseline-write", "", "FILE", true, "-baseline-write", "Record the current findings as the baseline and exit 0. --baseline-write=FILE writes another file"},
	{"Baseline", "baseline-check", "", "", false, "-baseline-check", "Report baseline entries which match no finding any more (implies --baseline)"},
	{"Baseline", "sarif-hide-baselined", "", "", false, "-sarif-hide-baselined", "Leave baselined findings out of --format sarif instead of marking them suppressed"},

	{"GitHub API (online checks)", "online", "", "MODE", true, "-online", "Run the checks which query the GitHub API. MODE: cache (cache only, no network), strict (fail when a lookup is skipped) or cache,strict"},
	{"GitHub API (online checks)", "no-online", "", "", false, "-online=false", "Never use the network, even when the config file enables the online checks"},
	{"GitHub API (online checks)", "online-api-url", "", "URL", false, "-online-api-url", "REST API URL of a GitHub Enterprise Server, e.g. https://ghe.example.com/api/v3"},
	{"GitHub API (online checks)", "online-token-env", "", "NAME", false, "-online-token-env", "Name of the environment variable that holds the GitHub token"},
	{"GitHub API (online checks)", "online-token-file", "", "FILE", false, "-online-token-file", "File that holds the GitHub token"},
	{"GitHub API (online checks)", "online-allow", "", "PATTERN", false, "-online-allow", "Look up only repositories matching owner/repo PATTERN (\"*\" is a wildcard). Repeatable"},
	{"GitHub API (online checks)", "online-deny", "", "PATTERN", false, "-online-deny", "Never look up repositories matching owner/repo PATTERN. Repeatable"},
	{"GitHub API (online checks)", "online-cache-ttl", "", "DURATION", false, "-online-cache-ttl", "How long a cached GitHub answer is used without revalidation (default 1h; 0 always revalidates)"},
	{"GitHub API (online checks)", "online-max-wait", "", "DURATION", false, "-online-max-wait", "The longest to wait for a GitHub rate limit to reset (default 30s; 0 never waits)"},

	{"External tools", "shellcheck", "", "CMD", false, "-shellcheck", "Command or path of shellcheck (default shellcheck). Empty disables the integration"},
	{"External tools", "pyflakes", "", "CMD", false, "-pyflakes", "Command or path of pyflakes (default pyflakes). Empty disables the integration"},
}

// legacyOnlyReplacements are v1 option names which have no row of their own in cliOptions.
var legacyOnlyReplacements = map[string]string{
	"help": "--help",
}

// legacyOptions maps every v1 single-dash long option to its replacement.
func legacyOptions() map[string]string {
	m := map[string]string{}
	for _, o := range cliOptions {
		if o.Replaces == "" || !strings.HasPrefix(o.Replaces, "-") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(o.Replaces, "-"), "=")
		if len(name) < 2 {
			continue
		}
		// -baseline=false and -online=false are rows of their own but the bare -baseline is --baseline
		if _, dup := m[name]; !dup {
			m[name] = "--" + o.Long
		}
	}
	for k, v := range legacyOnlyReplacements {
		m[k] = v
	}
	return m
}

// legacyHint returns the message for an argument which is a v1 option, or "" when it is not one.
func legacyHint(arg string) string {
	if len(arg) < 3 || arg[0] != '-' || arg[1] == '-' {
		return ""
	}
	name, val, hasVal := strings.Cut(arg[1:], "=")
	repl, ok := legacyOptions()[name]
	if !ok {
		return ""
	}
	if hasVal && (name == "baseline" || name == "online") {
		switch strings.ToLower(val) {
		case "false", "off", "0":
			repl = "--no-" + name
		}
	}
	return fmt.Sprintf("unknown option -%s; did you mean %s?", name, repl)
}

// optionalValueFlag is the value of --baseline and --baseline-write: given bare (the default file) or as
// --baseline=FILE.
type optionalValueFlag struct {
	set   bool
	value string
}

func (f *optionalValueFlag) String() string { return f.value }
func (f *optionalValueFlag) Type() string   { return "file" }
func (f *optionalValueFlag) Set(v string) error {
	switch v {
	case optionalValueSentinel:
		*f = optionalValueFlag{set: true}
	case "":
		return errors.New("the value must not be empty. omit it to use the default file")
	default:
		*f = optionalValueFlag{set: true, value: v}
	}
	return nil
}

// fixFlag is the value of --fix: bare --fix applies the safe fixes and --fix=unsafe applies all.
type fixFlag struct {
	mode FixMode
}

func (f *fixFlag) Type() string { return "mode" }
func (f *fixFlag) String() string {
	switch f.mode {
	case FixModeSafe:
		return "safe"
	case FixModeUnsafe:
		return "unsafe"
	}
	return ""
}

func (f *fixFlag) Set(v string) error {
	switch strings.ToLower(v) {
	case "safe":
		f.mode = FixModeSafe
	case "unsafe":
		f.mode = FixModeUnsafe
	default:
		return fmt.Errorf("invalid value %q. use --fix or --fix=unsafe", v)
	}
	return nil
}

// onlineFlag is the value of --online: bare --online, or a mode: --online=cache, --online=strict.
type onlineFlag struct {
	set  bool
	mode OnlineMode
}

func (f *onlineFlag) Type() string { return "mode" }
func (f *onlineFlag) String() string {
	if f == nil || !f.set || f.mode == OnlineModeDefault {
		return ""
	}
	return string(f.mode)
}

func (f *onlineFlag) Set(v string) error {
	m, err := ParseOnlineMode(v)
	if err != nil {
		return err
	}
	*f = onlineFlag{set: true, mode: m}
	return nil
}

// stringListFlag is a repeatable string option. A value may also hold several comma separated items.
type stringListFlag []string

func (l *stringListFlag) Type() string   { return "pattern" }
func (l *stringListFlag) String() string { return strings.Join(*l, ",") }
func (l *stringListFlag) Set(v string) error {
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			*l = append(*l, s)
		}
	}
	return nil
}

// colorFlag is the value of --color.
type colorFlag struct {
	set  bool
	kind ColorOptionKind
}

func (f *colorFlag) Type() string { return "when" }
func (f *colorFlag) String() string {
	if !f.set {
		return ""
	}
	switch f.kind {
	case ColorOptionKindAlways:
		return "always"
	case ColorOptionKindNever:
		return "never"
	}
	return "auto"
}

func (f *colorFlag) Set(v string) error {
	switch strings.ToLower(v) {
	case "always", "yes", "true":
		*f = colorFlag{true, ColorOptionKindAlways}
	case "never", "no", "false":
		*f = colorFlag{true, ColorOptionKindNever}
	case "auto":
		*f = colorFlag{true, ColorOptionKindAuto}
	default:
		return fmt.Errorf("invalid value %q. use always, never or auto", v)
	}
	return nil
}

// commandFlags holds the parsed values of the command line.
type commandFlags struct {
	help, ver                bool
	opts                     LinterOptions
	ignorePats               []string
	initConfig               bool
	fix                      fixFlag
	diff                     bool
	fixRules                 string
	migrateConfig            bool
	migrateIgnores           bool
	noColor                  bool
	color                    colorFlag
	noHints                  bool
	minSeverity              string
	profileName              string
	strictExit               bool
	onlineTTL, onlineMaxWait time.Duration
	baseline, baselineWrite  optionalValueFlag
	noBaseline, noOnline     bool
	online                   onlineFlag
	onlineAllow, onlineDeny  stringListFlag
	fs                       *pflag.FlagSet
}

// newCommandFlags registers every option of cliOptions on a new FlagSet.
func newCommandFlags() *commandFlags {
	f := &commandFlags{}
	fs := pflag.NewFlagSet("jactionlint", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.SortFlags = false
	f.fs = fs
	o := &f.opts
	short := map[string]string{}
	for _, c := range cliOptions {
		short[c.Long] = c.Short
	}
	sh := func(name string) string { return short[name] }
	opt := func(name string, nodef string) {
		if nodef != "" {
			fs.Lookup(name).NoOptDefVal = nodef
		}
	}

	fs.BoolVarP(&f.help, "help", sh("help"), false, "")
	fs.BoolVarP(&f.ver, "version", sh("version"), false, "")
	fs.BoolVarP(&o.Verbose, "verbose", sh("verbose"), false, "")
	fs.BoolVar(&o.Debug, "debug", false, "")
	fs.StringVar(&o.StdinFileName, "stdin-filename", "<stdin>", "")

	fs.StringVarP(&o.Format, "format", sh("format"), "", "")
	fs.BoolVar(&o.Oneline, "oneline", false, "")
	fs.BoolVar(&o.ShowRuleIDs, "rule-ids", false, "")
	fs.Var(&f.color, "color", "")
	opt("color", "always")
	fs.BoolVar(&f.noColor, "no-color", false, "")
	fs.BoolVar(&f.noHints, "no-hints", false, "")
	fs.StringVar(&f.minSeverity, "min-severity", "info", "")
	fs.BoolVar(&f.strictExit, "strict-exit", false, "")

	fs.StringVarP(&f.profileName, "profile", sh("profile"), "", "")
	fs.StringVarP(&o.ConfigFile, "config-file", sh("config-file"), "", "")
	fs.StringArrayVarP(&f.ignorePats, "ignore", sh("ignore"), nil, "")
	fs.BoolVar(&f.initConfig, "init-config", false, "")
	fs.BoolVar(&f.migrateConfig, "migrate-config", false, "")
	fs.BoolVar(&f.migrateIgnores, "migrate-ignores", false, "")

	fs.Var(&f.fix, "fix", "")
	opt("fix", "safe")
	fs.BoolVar(&f.diff, "diff", false, "")
	fs.StringVar(&f.fixRules, "fix-rules", "", "")

	fs.Var(&f.baseline, "baseline", "")
	opt("baseline", optionalValueSentinel)
	fs.BoolVar(&f.noBaseline, "no-baseline", false, "")
	fs.Var(&f.baselineWrite, "baseline-write", "")
	opt("baseline-write", optionalValueSentinel)
	fs.BoolVar(&o.BaselineCheck, "baseline-check", false, "")
	fs.BoolVar(&o.SARIFHideBaselined, "sarif-hide-baselined", false, "")

	fs.Var(&f.online, "online", "")
	opt("online", "true")
	fs.BoolVar(&f.noOnline, "no-online", false, "")
	fs.StringVar(&o.OnlineOptions.APIURL, "online-api-url", "", "")
	fs.StringVar(&o.OnlineOptions.TokenEnv, "online-token-env", "", "")
	fs.StringVar(&o.OnlineOptions.TokenFile, "online-token-file", "", "")
	fs.Var(&f.onlineAllow, "online-allow", "")
	fs.Var(&f.onlineDeny, "online-deny", "")
	fs.DurationVar(&f.onlineTTL, "online-cache-ttl", defaultOnlineCacheTTL, "")
	fs.DurationVar(&f.onlineMaxWait, "online-max-wait", defaultOnlineMaxWait, "")

	fs.StringVar(&o.Shellcheck, "shellcheck", "shellcheck", "")
	fs.StringVar(&o.Pyflakes, "pyflakes", "pyflakes", "")
	return f
}

// takesSeparateValue reports whether the option consumes the next argument when it has no "=value".
func (f *commandFlags) takesSeparateValue(fl *pflag.Flag) bool {
	return fl != nil && fl.NoOptDefVal == "" && fl.Value.Type() != "bool"
}

// parse parses the arguments after the program name. A returned error is a usage error whose text is one line.
func (f *commandFlags) parse(args []string) error {
	// v1 options are checked first: -fix would otherwise be read as the bundle -f -i -x
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if len(a) < 2 || a[0] != '-' {
			continue
		}
		if a[1] == '-' {
			name, _, hasVal := strings.Cut(a[2:], "=")
			if !hasVal && f.takesSeparateValue(f.fs.Lookup(name)) {
				i++ // the next argument is its value, whatever it looks like
			}
			continue
		}
		if msg := legacyHint(a); msg != "" {
			return errors.New(msg)
		}
		for j := 1; j < len(a); j++ {
			fl := f.fs.ShorthandLookup(a[j : j+1])
			if fl == nil || !f.takesSeparateValue(fl) {
				continue
			}
			if j == len(a)-1 {
				i++
			}
			break // the rest of the argument is the value
		}
	}
	if err := f.fs.Parse(args); err != nil {
		return usageMessage(err)
	}
	return nil
}

// usageMessage words the error of pflag like the other messages of the command.
func usageMessage(err error) error {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "unknown flag: "):
		msg = "unknown option " + strings.TrimPrefix(msg, "unknown flag: ")
	case strings.HasPrefix(msg, "unknown shorthand flag: "):
		msg = "unknown option " + strings.TrimPrefix(msg, "unknown shorthand flag: ")
	case strings.HasPrefix(msg, "flag needs an argument: "):
		msg = "option requires a value: " + strings.TrimPrefix(msg, "flag needs an argument: ")
	case strings.HasPrefix(msg, "bad flag syntax: "):
		msg = "bad option syntax: " + strings.TrimPrefix(msg, "bad flag syntax: ")
	}
	return errors.New(msg)
}

// printHelp writes the help text, which --help prints to stdout.
func printHelp(out io.Writer) {
	v := getCommandVersion()
	b := "main"
	if isReleaseVersion(v) {
		b = "v" + v
	}
	fmt.Fprint(out, `Usage: jactionlint [OPTIONS] [FILES...] [-]

jactionlint is a linter for GitHub Actions workflow files and Dependabot configuration.

With no FILES it checks the workflows of the nearest '.github/workflows'
directory. A lone '-' reads stdin. '--' ends the options.

Examples:
  jactionlint                               check the repository
  jactionlint ci.yaml release.yaml          check files
  jactionlint -                             check stdin
  jactionlint --fix                         apply the safe fixes
  jactionlint --format sarif                SARIF for code scanning and hk
  jactionlint --profile correctness         what actionlint checks
  jactionlint --online=cache                online checks from the cache only
`)
	for _, g := range cliGroups {
		fmt.Fprintf(out, "\n%s:\n", g)
		for _, o := range cliOptions {
			if o.Group == g {
				writeOptionHelp(out, o)
			}
		}
	}
	fmt.Fprintf(out, `
Options with an optional value (--color, --fix, --baseline, --baseline-write, --online) take it only as
--option=VALUE; '--option VALUE' treats VALUE as a file.

Exit status: 0 no problem, 1 problems found, 2 invalid command line, 3 failure.

Documents:
  Usage          https://github.com/jdx/jactionlint/tree/%[1]s/docs/usage.md
  Checks         https://github.com/jdx/jactionlint/tree/%[1]s/docs/checks.md
  Rule IDs       https://github.com/jdx/jactionlint/tree/%[1]s/docs/rules.md
  Configuration  https://github.com/jdx/jactionlint/tree/%[1]s/docs/config.md
`, b)
}

func isReleaseVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return false
		}
	}
	return true
}

// optionSyntax is how the option is written in the help, e.g. "-c, --config-file=FILE".
func optionSyntax(o cliOption) string {
	s := "    --" + o.Long
	if o.Short != "" {
		s = "-" + o.Short + ", --" + o.Long
		s = "  " + s
	}
	switch {
	case o.ArgName != "" && o.Optional:
		s += "[=" + o.ArgName + "]"
	case o.ArgName != "":
		s += "=" + o.ArgName
	}
	return s
}

const helpColumn = 34
const helpWidth = 100

func writeOptionHelp(out io.Writer, o cliOption) {
	syn := optionSyntax(o)
	lines := wrapWords(o.Help, helpWidth-helpColumn)
	if len(syn) >= helpColumn-1 {
		fmt.Fprintln(out, syn)
		syn = ""
	}
	for i, l := range lines {
		if i == 0 {
			fmt.Fprintf(out, "%-*s%s\n", helpColumn, syn, l)
		} else {
			fmt.Fprintf(out, "%-*s%s\n", helpColumn, "", l)
		}
	}
}

func wrapWords(s string, width int) []string {
	var lines []string
	cur := ""
	for _, w := range strings.Fields(s) {
		if cur != "" && len(cur)+1+len(w) > width {
			lines = append(lines, cur)
			cur = w
			continue
		}
		if cur != "" {
			cur += " "
		}
		cur += w
	}
	return append(lines, cur)
}

package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-colorable"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// LogLevel is log level of logger used in Linter instance.
type LogLevel int

const (
	// LogLevelNone does not output any log output.
	LogLevelNone LogLevel = 0
	// LogLevelVerbose shows verbose log output. This is equivalent to specifying --verbose option
	// to jactionlint command.
	LogLevelVerbose = 1
	// LogLevelDebug shows all log output including debug information.
	LogLevelDebug = 2
)

// ColorOptionKind is kind of colorful output behavior.
type ColorOptionKind int

const (
	// ColorOptionKindAuto is kind to determine to colorize errors output automatically. It is
	// determined based on pty and $NO_COLOR environment variable. See document of fatih/color
	// for more details.
	ColorOptionKindAuto ColorOptionKind = iota
	// ColorOptionKindAlways is kind to always colorize errors output.
	ColorOptionKindAlways
	// ColorOptionKindNever is kind never to colorize errors output.
	ColorOptionKindNever
)

// LinterOptions is set of options for Linter instance. This struct is used for NewLinter factory
// function call. The zero value LinterOptions{} represents the default behavior.
type LinterOptions struct {
	// Verbose is flag if verbose log output is enabled.
	Verbose bool
	// Debug is flag if debug log output is enabled.
	Debug bool
	// LogWriter is io.Writer object to use to print log outputs. Note that error outputs detected
	// by the linter are not included in the log outputs.
	LogWriter io.Writer
	// Color is option for colorizing error outputs. See ColorOptionKind document for each enum values.
	Color ColorOptionKind
	// Oneline is flag if one line output is enabled. When enabling it, one error is output per one
	// line. It is useful when reading outputs from programs. It is the same as setting Format to
	// "oneline" and it only affects the text format.
	Oneline bool
	// ShowRuleIDs is accepted for compatibility. The text format always shows the stable rule ID such as
	// "unpinned-uses" at the end of each error, since that is what the configuration, the ignore
	// comments and ignore patterns accept; the kind such as "action" is shared by several rules.
	ShowRuleIDs bool
	// Shellcheck is executable for running shellcheck external command. It can be command name like
	// "shellcheck" or file path like "/path/to/shellcheck", "path/to/shellcheck". When this value
	// is empty, shellcheck won't run to check scripts in workflow file.
	Shellcheck string
	// Pyflakes is executable for running pyflakes external command. It can be command name like "pyflakes"
	// or file path like "/path/to/pyflakes", "path/to/pyflakes". When this value is empty, pyflakes
	// won't run to check scripts in workflow file.
	Pyflakes string
	// IgnorePatterns is list of regular expression to filter errors. The pattern is applied to error
	// messages. When an error is matched, the error is ignored.
	IgnorePatterns []string
	// ConfigFile is a path to config file. Empty string means no config file path is given. In
	// the case, jactionlint will try to read config from the repository's .github/jactionlint.yaml,
	// then from $XDG_CONFIG_HOME/jactionlint/jactionlint.yaml ($HOME/.config when unset).
	ConfigFile string
	// Config is a configuration to use instead of reading one from a file or the repository. It is for
	// callers which have no file system (the playground) or want fixed rules. ConfigFile takes precedence
	// when both are set.
	Config *Config
	// Format selects the output format. It is one of "text" (the default when empty), "oneline", "json",
	// "jsonl", "sarif", "gcc" and "github", or a custom template to format error messages. A template
	// must follow Go Template format and contain at least one {{ }} placeholder. When the format is
	// not a native one, it is regarded as a template. https://pkg.go.dev/text/template
	Format string
	// StdinFileName is a file name when reading input from stdin. When this value is empty, "<stdin>"
	// is used as the default value.
	StdinFileName string
	// WorkingDir is a file path to the current working directory. When this value is empty, os.Getwd
	// will be used to get a working directory.
	WorkingDir string
	// MinSeverity hides the errors less severe than it. The zero value shows every error. For example
	// SeverityWarning hides the errors of info level.
	MinSeverity Severity
	// Online turns on the rules which query the GitHub API: impostor-commit, known-vulnerable-actions,
	// ref-confusion, stale-action-refs, archived-uses and ref-version-mismatch (and the pinning
	// fix of unpinned-uses, see Error.Fix). Nothing reaches the network when it is false. The token is
	// read from $GITHUB_TOKEN or $GH_TOKEN; without one the API allows very few requests. When the
	// API cannot be used (rate limit, no network) the online rules stop with one warning. It
	// is an error in builds without network access such as the WebAssembly one, unless
	// GitHubClient is set. The "online" key of the configuration file turns it on for the files it
	// applies to.
	Online bool
	// Profile selects the profile (ProfileCorrectness, ProfileDefault or ProfilePedantic) for every file,
	// whatever the configuration says: it is what --profile asks for. The empty value leaves the choice to
	// the configuration.
	Profile Profile
	// OnlineOff turns the online checks off although the configuration file turns them on: it is what
	// --no-online asks for. It wins over Online and over the configuration.
	OnlineOff bool
	// GitHubClient replaces the built-in client of the GitHub API, which sends REST requests and caches
	// the answers in $XDG_CACHE_HOME/jactionlint. It is used by tests (see NewFixtureGitHubClient)
	// and implies nothing by itself: the online rules need Online or the "online" configuration.
	GitHubClient GitHubClient
	// OnlineCacheTTL is how long the built-in client uses a cached answer without asking GitHub
	// whether it changed. Zero means one hour. A negative value revalidates every answer (which
	// costs no rate limit when nothing changed).
	OnlineCacheTTL time.Duration
	// OnlineOptions tunes the online checks (mode, API URL, token source, allow and deny lists, cache,
	// retries). It wins over the "online-options" of the configuration file. A Mode other than the
	// default turns the online checks on. See OnlineOptions.
	OnlineOptions OnlineOptions
	// Context stops the online lookups when it is canceled, for example on interruption. Nil means
	// context.Background.
	Context context.Context
	// Now returns the current time. It is used to decide whether an entry of "ignores" in the config
	// file has expired. Nil means time.Now. It is for tests.
	Now func() time.Time
	// OnRulesCreated is a hook to add or remove the check rules. This function is called on checking
	// every workflow files. Rules created by Linter instance are passed to the argument and the
	// function should return the modified rules.
	// Note that syntax errors may be reported even if this function returns nil or an empty slice.
	OnRulesCreated func([]Rule) []Rule
	// OnDependabotRulesCreated is like OnRulesCreated but for the rules which check Dependabot
	// configuration files (.github/dependabot.yml).
	OnDependabotRulesCreated func([]DependabotRule) []DependabotRule
	// Baseline applies the baseline file (BaselineFile, or .github/jactionlint-baseline.json in the
	// repository when empty) even if the configuration does not ask for it: the findings it accepts
	// are not returned or printed. The file must exist.
	Baseline bool
	// BaselineFile is the path of the baseline file to apply with Baseline. A relative path is relative
	// to the working directory.
	BaselineFile string
	// NoBaseline ignores the baseline even if the configuration asks for it.
	NoBaseline bool
	// BaselineCheck reports the baseline entries which match no finding as unused-baseline-entry. It
	// implies Baseline unless the configuration selects a baseline.
	BaselineCheck bool
	// SARIFHideBaselined leaves the findings which the baseline accepts out of the SARIF log. By default
	// they are in it as results with a suppression of the kind "external".
	SARIFHideBaselined bool
	// RunHints lets a run of the text format that finds many findings print one line to LogWriter about how
	// to count them, adopt them gradually and get the checks of actionlint only. It is printed only when
	// LogWriter is a terminal or the process runs in CI, and never with another format. The jactionlint
	// command sets it unless --no-hints or JACTIONLINT_NO_HINTS is given.
	RunHints bool
	// More options will come here
}

// Linter is struct to lint workflow files.
type Linter struct {
	shared     sharedFindings
	projects   *Projects
	out        io.Writer
	logOut     io.Writer
	logLevel   LogLevel
	printer    printer
	shellcheck string
	pyflakes   string
	ignorePats IgnorePatterns
	// now returns the current time. It is time.Now unless LinterOptions.Now is set.
	now func() time.Time
	// ignoreRun is the state of the config "ignores" in the current run.
	ignoreRun      ignoreRun
	stdin          string
	defaultConfig  *Config
	globalConfig   *Config
	errFmt         *ErrorFormatter
	cwd            string
	onRulesCreated func([]Rule) []Rule
	onDependabot   func([]DependabotRule) []DependabotRule
	configFile     string
	minSeverity    Severity
	online         onlineSettings
	baseline       linterBaseline
	warnedOnce     sync.Map    // string -> struct{}: the messages warnOnce printed
	profile        Profile     // the --profile override, empty when the configuration decides
	profiled       sync.Map    // *Config -> *Config: the configs with the profile override applied
	warned         sync.Map    // *Config -> struct{}: configs whose deprecations were already reported
	graphs         sync.Map    // root directory -> *sync.Once-guarded *callGraph, see callGraphOf
	runHints       bool        // LinterOptions.RunHints
	hintBaseline   atomic.Bool // a baseline was applied to a file
	// hintBeyondCorrectness is true when a file was linted with a profile above correctness
	hintBeyondCorrectness atomic.Bool
	notesMu               sync.Mutex
	notes                 []string // deprecation warnings found while linting
}

// NewLinter creates a new Linter instance.
// The out parameter is used to output errors from Linter instance. Set io.Discard if you don't
// want the outputs.
// The opts parameter is LinterOptions instance which configures behavior of linting.
func NewLinter(out io.Writer, opts *LinterOptions) (*Linter, error) {
	level := LogLevelNone
	if opts.Verbose {
		level = LogLevelVerbose
	} else if opts.Debug {
		level = LogLevelDebug
	}

	if opts.Color == ColorOptionKindNever {
		color.NoColor = true
	} else {
		if opts.Color == ColorOptionKindAlways {
			color.NoColor = false
		}
		// Allow colorful output on Windows
		if f, ok := out.(*os.File); ok {
			out = colorable.NewColorable(f)
		}
	}

	lout := io.Discard
	if opts.LogWriter != nil {
		lout = opts.LogWriter
	}

	var cfg *Config
	if opts.ConfigFile != "" {
		c, err := ReadConfigFile(opts.ConfigFile)
		if err != nil {
			return nil, err
		}
		c.userOwned = true
		cfg = c
	} else if opts.Config != nil {
		cfg = opts.Config
		cfg.userOwned = true
	}

	// Load the user-global config as a fallback for projects which have no
	// .github/jactionlint.yaml. The --config-file option takes precedence over it.
	var globalCfg *Config
	var globalCfgPath string
	if opts.ConfigFile == "" && opts.Config == nil {
		c, p, err := loadGlobalConfig()
		if err != nil {
			return nil, err
		}
		globalCfg, globalCfgPath = c, p
		if globalCfg != nil {
			globalCfg.userOwned = true
		}
	}

	if err := opts.OnlineOptions.validate(); err != nil {
		return nil, &usageError{fmt.Errorf("invalid online options: %w", err)}
	}

	ignore := make(IgnorePatterns, 0, len(opts.IgnorePatterns))
	for _, s := range opts.IgnorePatterns {
		r, err := ParseIgnorePattern(s)
		if err != nil {
			return nil, &usageError{fmt.Errorf("invalid regular expression for ignore pattern %q: %s", s, err.Error())}
		}
		ignore = append(ignore, r)
	}

	var formatter *ErrorFormatter
	if isTemplateFormat(opts.Format) {
		f, err := NewErrorFormatter(opts.Format)
		if err != nil {
			return nil, &usageError{err}
		}
		formatter = f
	}
	prn, err := newPrinter(opts.Format, opts.Oneline, opts.SARIFHideBaselined, formatter)
	if err != nil {
		return nil, &usageError{err}
	}

	cwd := "."
	if opts.WorkingDir != "" {
		cwd = opts.WorkingDir
	} else if d, err := os.Getwd(); err == nil {
		cwd = d
	}

	stdin := "<stdin>"
	if opts.StdinFileName != "" {
		stdin = opts.StdinFileName
	}

	l := &Linter{
		projects:       NewProjects(),
		out:            out,
		logOut:         lout,
		logLevel:       level,
		printer:        prn,
		shellcheck:     opts.Shellcheck,
		pyflakes:       opts.Pyflakes,
		ignorePats:     ignore,
		now:            opts.Now,
		stdin:          stdin,
		defaultConfig:  cfg,
		globalConfig:   globalCfg,
		errFmt:         formatter,
		cwd:            cwd,
		onRulesCreated: opts.OnRulesCreated,
		runHints:       opts.RunHints,
		onDependabot:   opts.OnDependabotRulesCreated,
		configFile:     opts.ConfigFile,
		minSeverity:    opts.MinSeverity,
		online:         onlineSettings{off: opts.OnlineOff, enabled: !opts.OnlineOff && (opts.Online || opts.OnlineOptions.Mode != OnlineModeDefault), client: opts.GitHubClient, ttl: opts.OnlineCacheTTL, ctx: opts.Context, opts: opts.OnlineOptions},
	}
	l.warnRetiredIgnores(ignore...)
	if opts.Profile != "" {
		p, err := ParseProfile(string(opts.Profile))
		if err != nil {
			return nil, err
		}
		l.profile = p
	}
	l.baseline.on = opts.Baseline
	l.baseline.off = opts.NoBaseline
	l.baseline.check = opts.BaselineCheck
	l.baseline.hideInSARIF = opts.SARIFHideBaselined
	if opts.BaselineFile != "" {
		f := opts.BaselineFile
		if !filepath.IsAbs(f) {
			f = filepath.Join(cwd, f)
		}
		l.baseline.file = f
		l.baseline.on = true
	}
	if l.online.enabled && opts.GitHubClient == nil && !onlineSupported {
		return nil, errOnlineUnsupported
	}

	l.debug("Create a Linter instance with option %#v", opts)
	if globalCfgPath != "" {
		l.debug("Loaded user-global config from %q", globalCfgPath)
	}
	return l, nil
}

func (l *Linter) log(args ...interface{}) {
	if l.logLevel < LogLevelVerbose {
		return
	}
	fmt.Fprint(l.logOut, "verbose: ")
	fmt.Fprintln(l.logOut, args...)
}

func (l *Linter) debug(format string, args ...interface{}) {
	if l.logLevel < LogLevelDebug {
		return
	}
	format = "[Linter] " + format + "\n"
	fmt.Fprintf(l.logOut, format, args...)
}

func (l *Linter) debugWriter() io.Writer {
	if l.logLevel < LogLevelDebug {
		return nil
	}
	return l.logOut
}

// GenerateDefaultConfig generates default config file at ".github/jactionlint.yaml" in the project
// which the given directory path belongs to. When the directory path is empty, the current directory
// will be used instead.
func (l *Linter) GenerateDefaultConfig(dir string) error {
	if dir == "" {
		dir = l.cwd
	}

	l.log("Generating default jactionlint.yaml in repository:", dir)

	proj, err := l.projects.At(dir)
	if err != nil {
		return err
	}
	if proj == nil {
		return errors.New("project is not found. check current project is initialized as Git repository and \".github/workflows\" directory exists")
	}

	d := filepath.Join(proj.RootDir(), ".github")
	for _, f := range configFileNames {
		p := filepath.Join(d, f)
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("config file already exists at %q", p)
		}
	}

	p := filepath.Join(d, "jactionlint.yaml")
	if err := writeDefaultConfigFile(p); err != nil {
		return err
	}

	fmt.Fprintf(l.out, "Config file was generated at %q\n", p)
	return nil
}

// LintRepository lints YAML workflow files and outputs the errors to given writer. It finds the
// nearest `.github/workflows` directory based on `dir` and applies lint rules to all YAML workflow
// files under the directory. When the directory path is empty, the current working directory will
// be used instead.
func (l *Linter) LintRepository(dir string) ([]*Error, error) {
	if dir == "" {
		dir = l.cwd
	}

	l.log("Linting all workflow files and Dependabot configuration in repository:", dir)

	files, p, err := l.repositoryFiles(dir)
	if err != nil {
		return nil, err
	}
	l.log("Collected", len(files), "YAML files")
	return l.LintFiles(files, p)
}

// repositoryFiles finds the nearest project of dir and returns its files which are linted: the workflow
// files and the Dependabot configuration. LintRepository and FixRepository both use it, so what is fixed is always
// what is linted.
func (l *Linter) repositoryFiles(dir string) ([]string, *Project, error) {
	p, err := l.projects.At(dir)
	if err != nil {
		return nil, nil, err
	}
	if p == nil {
		return nil, nil, fmt.Errorf("no project was found in any parent directories of %q. check workflows directory is put correctly in your Git repository", dir)
	}

	l.log("Detected project:", p.RootDir())
	files, err := projectWorkflowFiles(p.WorkflowsDir())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err // a repository with only actions has no workflows directory
	}
	files = append(files, p.DependabotFiles()...)
	files = append(files, l.callGraphOf(p).actionPaths()...)
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("no YAML file was found in %q", p.WorkflowsDir())
	}
	return files, p, nil
}

// projectWorkflowFiles returns the paths of the YAML files which are located directly in the workflows
// directory of a project, in sorted order. GitHub loads only these as workflows, so a YAML file in a
// subdirectory (test data, scripts, configuration for tools) is not a workflow and is not linted as one
// by the repository mode. A file given explicitly on the command line is linted anyway.
func projectWorkflowFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("could not read files in %q: %w", dir, err)
	}
	files := []string{}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".yml") && !strings.HasSuffix(n, ".yaml") {
			continue
		}
		path := filepath.Join(dir, n)
		if e.IsDir() {
			continue
		}
		if e.Type()&fs.ModeSymlink != 0 {
			if s, err := os.Stat(path); err != nil || s.IsDir() {
				continue // a dangling link or a link to a directory is no workflow file
			}
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

// collectWorkflowFiles returns the paths of all YAML files in the directory recursively in sorted order.
// It is an error that the directory has no YAML file.
func collectWorkflowFiles(dir string) ([]string, error) {
	files, err := walkWorkflowFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no YAML file was found in %q", dir)
	}
	return files, nil
}

// walkWorkflowFiles returns the paths of all YAML files in the directory recursively in sorted order.
func walkWorkflowFiles(dir string) ([]string, error) {
	files := []string{}
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("could not read files in %q: %w", dir, err)
	}

	// To make output deterministic, sort order of file paths
	sort.Strings(files)
	return files, nil
}

// LintDir lints all YAML workflow files in the given directory recursively.
func (l *Linter) LintDir(dir string, project *Project) ([]*Error, error) {
	files, err := collectWorkflowFiles(dir)
	if err != nil {
		return nil, err
	}
	l.log("Collected", len(files), "YAML files")

	return l.LintFiles(files, project)
}

// LintFiles lints YAML workflow files and outputs the errors to given writer. It applies lint
// rules to all given files. The project parameter can be nil. In the case, a project is detected
// from the file path.
func (l *Linter) LintFiles(filepaths []string, project *Project) ([]*Error, error) {
	n := len(filepaths)
	switch n {
	case 0:
		return []*Error{}, nil
	case 1:
		return l.LintFile(filepaths[0], project)
	}

	results, err := l.lintFilesQuietly(filepaths, project)
	if err != nil {
		return nil, err
	}

	results = l.finishRun(results)
	total := 0
	for _, r := range results {
		total += len(r.errs)
	}
	all := make([]*Error, 0, total)
	for _, r := range results {
		all = append(all, r.errs...)
	}
	if err := l.printer.print(l.out, results, l.notifications()); err != nil {
		return nil, err
	}
	l.reportBaselineNote(results)
	l.reportRunHint(results)

	l.log("Found", total, "errors in", n, "files")

	return all, nil
}

// finishRun adds the results which belong to the run and not to a file: the problems of the ignores of
// the config files (expired and unused entries) and the unused entries of the baseline files.
func (l *Linter) finishRun(results []fileResult) []fileResult {
	return l.withBaselineResults(l.finishIgnoreRun(results))
}

// lintFilesQuietly lints the files in parallel and returns the results without printing them. The
// results are in the order of the file paths.
func (l *Linter) lintFilesQuietly(filepaths []string, project *Project) ([]fileResult, error) {
	n := len(filepaths)
	l.log("Linting", n, "files")

	cwd := l.cwd
	cpus := runtime.NumCPU()
	proc := newConcurrentProcess(cpus)
	sema := semaphore.NewWeighted(int64(cpus))
	ctx := context.Background()
	dbg := l.debugWriter()
	acf := NewLocalActionsCacheFactory(dbg)
	rwcf := NewLocalReusableWorkflowCacheFactory(cwd, dbg)

	ws := make([]fileResult, 0, len(filepaths))
	for _, p := range filepaths {
		ws = append(ws, fileResult{file: p, path: p})
	}

	eg := errgroup.Group{}
	for i := range ws {
		// Each element of ws is accessed by single goroutine so mutex is unnecessary
		w := &ws[i]
		proj := project
		if proj == nil {
			// This method modifies state of l.projects so it cannot be called in parallel.
			// Before entering goroutine, resolve project instance.
			p, err := l.projects.At(w.path)
			if err != nil {
				return nil, err
			}
			proj = p
		}
		ac := acf.GetCache(proj) // #173
		rwc := rwcf.GetCache(proj)

		eg.Go(func() error {
			// Bound concurrency on reading files to avoid "too many files to open" error (issue #3)
			sema.Acquire(ctx, 1)
			src, err := os.ReadFile(w.path)
			sema.Release(1)
			if err != nil {
				return fmt.Errorf("could not read %q: %w", w.path, err)
			}

			if cwd != "" {
				if r, err := filepath.Rel(cwd, w.path); err == nil {
					w.path = r // Use relative path if possible
				}
			}
			errs, err := l.check(w.path, src, proj, proc, ac, rwc)
			if err != nil {
				return fmt.Errorf("fatal error while checking %s: %w", w.path, err)
			}
			*w = newFileResult(w.file, w.path, src, errs)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	l.shared.dropRepeatedInFiles(ws)

	// Ensure that all processes finish. `proc.wait()` must be called after `eg.Wait()`.
	// Calling `WaitGroup.Add` after `WaitGroup.Wait` can cause a race condition (specifically when
	// increasing the group count from 0 to 1 and calling `Wait` and at the same time).
	// `WaitGroup.Add` is called in `proc.run()` and `WaitGroup.Wait` is called in `proc.wait()`.
	// After traversing all workflows, `proc.run()` is no longer called so `proc.wait()` can be
	// called safely.
	proc.wait()

	return ws, nil
}

// sharedFindings are the findings which describe a thing many places share (a local action with a broken
// metadata file) and not the place which reported them (see RuleAction.reportOnce). Every use reports such a
// finding, so that the report does not depend on which file a goroutine happened to lint first, and only the
// first one in the order of the files is kept.
type sharedFindings struct{ errs sync.Map }

// mark remembers the findings.
func (s *sharedFindings) mark(errs []*Error) {
	for _, e := range errs {
		s.errs.Store(e, struct{}{})
	}
}

// dropper returns a function which reports whether a finding is a repeat of an earlier shared finding with the
// same ID and message, and so is to be dropped.
func (s *sharedFindings) dropper() func(*Error) bool {
	var seen map[string]bool
	return func(e *Error) bool {
		if _, ok := s.errs.Load(e); !ok {
			return false
		}
		k := e.ID + "\x00" + e.Message
		if seen[k] {
			return true
		}
		if seen == nil {
			seen = map[string]bool{}
		}
		seen[k] = true
		return false
	}
}

// dropRepeated removes the repeats from the sorted findings of one file.
func (s *sharedFindings) dropRepeated(errs []*Error) []*Error {
	return slices.DeleteFunc(errs, s.dropper())
}

// dropRepeatedInFiles keeps only the first shared finding of all files, in the order of the results. The
// findings which the baseline accepts count as the first, too: the repeat of an accepted finding is the same
// finding.
func (s *sharedFindings) dropRepeatedInFiles(ws []fileResult) {
	drop := s.dropper()
	for i := range ws {
		// A file has at most one of them (finishCheck dropped the repeats), either accepted or reported
		ws[i].baselined = slices.DeleteFunc(ws[i].baselined, drop)
		ws[i].errs = slices.DeleteFunc(ws[i].errs, drop)
	}
}

// LintFile lints one YAML workflow file and outputs the errors to given writer. The project
// parameter can be nil. In the case, the project is detected from the given path.
func (l *Linter) LintFile(path string, project *Project) ([]*Error, error) {
	if project == nil {
		p, err := l.projects.At(path)
		if err != nil {
			return nil, err
		}
		project = p
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %q: %w", path, err)
	}

	origPath := path
	if l.cwd != "" {
		if r, err := filepath.Rel(l.cwd, path); err == nil {
			path = r
		}
	}

	proc := newConcurrentProcess(runtime.NumCPU())
	dbg := l.debugWriter()
	localActions := NewLocalActionsCache(project, dbg)
	localReusableWorkflows := NewLocalReusableWorkflowCache(project, l.cwd, dbg)
	errs, err := l.check(path, src, project, proc, localActions, localReusableWorkflows)
	proc.wait()
	if err != nil {
		return nil, err
	}

	return l.printOne(newFileResult(origPath, path, src, errs))
}

// LintStdin lints the content read from STDIN. The stdin parameter is a reader to read from STDIN,
// which is usually os.Stdin. The file name is determined by LinterOptions.StdinFileName. When the
// option is empty, "<stdin>" is the default value.
func (l *Linter) LintStdin(stdin io.Reader) ([]*Error, error) {
	l.log("Reading the input from stdin")
	b, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("could not read stdin: %w", err)
	}
	return l.Lint(l.stdin, b, nil)
}

// Lint lints YAML workflow file content given as byte slice. The path parameter is used as file
// path where the content came from.
// When nil is passed to the project parameter, it tries to find the project from the path parameter.
func (l *Linter) Lint(path string, content []byte, project *Project) ([]*Error, error) {
	if project == nil && path != "<stdin>" {
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			p, err := l.projects.At(path)
			if err != nil {
				return nil, err
			}
			project = p
		}
	}
	proc := newConcurrentProcess(runtime.NumCPU())
	dbg := l.debugWriter()
	localActions := NewLocalActionsCache(project, dbg)
	localReusableWorkflows := NewLocalReusableWorkflowCache(project, l.cwd, dbg)
	errs, err := l.check(path, content, project, proc, localActions, localReusableWorkflows)
	proc.wait()
	if err != nil {
		return nil, err
	}
	return l.printOne(newFileResult(path, path, content, errs))
}

func (l *Linter) check(
	path string,
	content []byte,
	project *Project,
	proc *concurrentProcess,
	localActions *LocalActionsCache,
	localReusableWorkflows *LocalReusableWorkflowCache,
) ([]*Error, error) {
	// Note: This method is called to check multiple files in parallel.
	// It must be thread safe assuming fields of Linter are not modified while running.

	var start time.Time
	if l.logLevel >= LogLevelVerbose {
		start = time.Now()
	}

	l.log("Linting", path)
	if project != nil {
		l.log("Using project at", project.RootDir())
	}

	// Config priority: --config-file option, then repository config, then user-global config
	cfg := l.configFor(project)
	if cfg != nil {
		l.debug("Config: %#v", cfg)
		l.warnDeprecations(cfg)
	} else {
		l.debug("No config was found")
	}

	bl, err := l.baselineFor(project, cfg)
	if err != nil {
		return nil, err
	}

	if l.isDependabotFile(path) {
		return l.checkDependabot(path, content, project, cfg, bl, start)
	}

	isAction := l.isActionFile(path)
	var w *Workflow
	var all []*Error
	if isAction {
		w, all = ParseAction(content)
		if w != nil && project != nil {
			w.Action.Callers = l.actionCallers(project, l.absFilePath(path))
		}
	} else {
		w, all = Parse(content)
	}

	if l.logLevel >= LogLevelVerbose {
		elapsed := time.Since(start)
		l.log("Found", len(all), "parse errors in", elapsed.Milliseconds(), "ms for", path)
	}

	if w != nil {
		dbg := l.debugWriter()

		sess, err := l.onlineSession(cfg)
		if err != nil {
			return nil, err
		}
		rules := newBuiltinRules(&RuleEnv{
			online:                 sess,
			path:                   path,
			src:                    content,
			action:                 isAction,
			project:                project,
			localActions:           localActions,
			localReusableWorkflows: localReusableWorkflows,
			config:                 cfg,
			shellcheck:             l.shellcheck,
			pyflakes:               l.pyflakes,
			proc:                   proc,
		}, l.log)
		if l.onRulesCreated != nil {
			rules = l.onRulesCreated(rules)
		}

		v := NewVisitor()
		for _, rule := range rules {
			v.AddPass(rule)
		}
		if dbg != nil {
			v.EnableDebug(dbg)
			for _, r := range rules {
				r.EnableDebug(dbg)
			}
		}
		if cfg != nil {
			for _, r := range rules {
				r.SetConfig(cfg)
			}
		}

		if err := v.Visit(w); err != nil {
			l.debug("Error occurred while visiting workflow syntax tree: %v", err)
			return nil, err
		}

		for _, rule := range rules {
			if sr, ok := rule.(interface{ sharedErrs() []*Error }); ok {
				l.shared.mark(sr.sharedErrs())
			}
			errs := rule.Errs()
			// A reusable workflow which cannot be loaded is a problem of that file, which every call reports
			l.shared.mark(slices.DeleteFunc(slices.Clone(errs), func(e *Error) bool { return e.ID != "invalid-local-workflow" }))
			l.debug("%s found %d errors", rule.Name(), len(errs))
			if isAction {
				errs = slices.DeleteFunc(slices.Clone(errs), func(e *Error) bool { return dropsOnActions(e.Kind, e.ID) })
			}
			all = append(all, errs...)
		}

		if l.errFmt != nil {
			for _, rule := range rules {
				l.errFmt.RegisterRule(rule)
			}
		}
	}

	return l.finishCheck(path, content, all, cfg, bl, start, w != nil, &ignoreContext{project: project, scopes: newScopeIndex(w, content)}), nil
}

// isActionFile reports whether the path is the metadata file of an action (see IsActionPath). The path
// is resolved against the working directory first, so that "action.yml" linted from inside ".github/workflows"
// is still a workflow. The name for STDIN is used as it is.
func (l *Linter) isActionFile(p string) bool {
	if p != l.stdin {
		p = l.absFilePath(p)
	}
	return IsActionPath(p)
}

// finishCheck post-processes the errors found in one file: it fills the fields derived from the
// rule IDs, applies the ignores and the minimum severity, and sorts the errors. isWorkflow tells that
// the file is a workflow, for which the online pin fixes are attached.
func (l *Linter) finishCheck(path string, content []byte, all []*Error, cfg *Config, bl *baselineState, start time.Time, isWorkflow bool, ic *ignoreContext) []*Error {
	l.noteRunProfile(cfg)
	all = append(all, checkSourceRules(content, cfg)...)
	if ic != nil {
		all = dropUnreachable(all, ic.scopes)
	}
	all = l.annotateErrors(all, content, cfg)

	// The ignores of the config file are matched first, without removing anything, so that the inline
	// ignores see every error as well and neither is reported as unused for covering the same error.
	var cfgHit map[*Error]bool
	if ic != nil {
		l.trackIgnoreConfig(cfg, ic.project, path)
		cfgHit = l.matchConfigIgnores(all, cfg, ic.project, path, ic.scopes)
	}

	// Inline ignores are applied first so that every pattern sees all errors, which tells whether it
	// is used. The order of the filters does not change which errors remain.
	inlineIgnores, orphans, ignoreErrs := parseInlineIgnoresWithOrphans(content)
	for _, ig := range inlineIgnores {
		for _, e := range ig.entries {
			l.warnRetiredIgnores(e.pat)
		}
	}
	for _, e := range orphans {
		l.warnRetiredIgnores(e.pat)
	}
	var inlinePats IgnorePatterns
	for _, ig := range inlineIgnores {
		for _, e := range ig.entries {
			inlinePats = append(inlinePats, e.pat)
		}
	}
	l.warnKindPatterns(all, inlinePats)
	inlineIgnores = append(inlineIgnores, parseZizmorIgnores(content)...)
	all = l.filterInlineIgnores(all, inlineIgnores)
	all = dropIgnored(all, cfgHit)
	unused := unusedInlineIgnores(inlineIgnores, orphans, cfg, l.online.enabled || (!l.online.off && cfg != nil && cfg.Online))
	dropFixesChangingYAML(content, unused)
	all = append(all, l.annotateErrors(unused, content, cfg)...)

	all = l.filterErrors(all, cfg.PathConfigs(path))
	all = append(all, l.annotateErrors(ignoreErrs, content, cfg)...)
	if isWorkflow {
		if sess, _ := l.onlineSession(cfg); sess != nil {
			l.attachPinFixes(sess, content, all)
		}
	}

	for _, err := range all {
		err.Filepath = path // Populate filename in the error
	}

	slices.SortFunc(all, compareErrors)
	all = slices.CompactFunc(all, equalsErrors) // Alias may duplicate errors
	all = l.shared.dropRepeated(all)

	// The baseline identifies findings by their order among identical ones, so it sees all of them,
	// also those below the minimum severity
	var project *Project
	if ic != nil {
		project = ic.project
	}
	l.baselineStage(path, content, project, bl, all)

	if l.minSeverity > SeverityInfo {
		kept := all[:0]
		for _, err := range all {
			if err.Severity >= l.minSeverity {
				kept = append(kept, err)
			}
		}
		all = kept
	}

	if l.logLevel >= LogLevelVerbose {
		elapsed := time.Since(start)
		l.log("Found total", len(all), "errors in", elapsed.Milliseconds(), "ms for", path)
	}

	return all
}

func (l *Linter) filterErrors(errs []*Error, cfgs []PathConfig) []*Error {
	if len(l.ignorePats) == 0 && len(cfgs) == 0 {
		return errs
	}

	l.warnKindPatterns(errs, l.ignorePats)
	for _, c := range cfgs {
		l.warnKindPatterns(errs, c.Ignore)
	}

	filtered := make([]*Error, 0, len(errs))
Loop:
	for _, err := range errs {
		if l.ignorePats.Match(err) {
			l.debug("Error %q is ignored due to --ignore command line option", err.Message)
			continue Loop
		}
		for _, c := range cfgs {
			if c.Ignore.Match(err) {
				l.debug("Error %q is ignored due to the \"ignore\" config in the config file", err.Message)
				continue Loop
			}
		}
		filtered = append(filtered, err)
	}
	if len(filtered) != len(errs) {
		l.log("Filtered", len(errs)-len(filtered), "error(s) due to \"--ignore\" command line option and \"ignore\" configuration")
	}
	return filtered
}

var kindWordRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// warnKindPatterns warns about an ignore pattern which is the legacy kind of some findings ("timeout-check",
// "action") instead of a rule ID. Such a pattern is a regular expression on the messages, which the kind is
// not part of, so it ignores nothing, and the kind is what former versions printed at the end of a finding.
func (l *Linter) warnKindPatterns(errs []*Error, pats IgnorePatterns) {
	for _, p := range pats {
		if p.Regexp == nil || !kindWordRe.MatchString(p.Regexp.String()) {
			continue
		}
		kind := p.Regexp.String()
		var ids []string
		for _, e := range errs {
			if e.Kind == kind && e.ID != "" && e.ID != kind && !slices.Contains(ids, e.ID) {
				ids = append(ids, e.ID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		slices.Sort(ids)
		l.warnOnce(fmt.Sprintf("the ignore pattern %q is a legacy kind, not a rule ID, so it is a regular expression on the messages, and it matches only a message that contains this text. use a rule ID: %s", kind, strings.Join(ids, ", ")))
	}
}

// warnDeprecations reports the deprecated keys of the config to the log output. It reports each
// config only once even if many files are linted with it.
func (l *Linter) warnDeprecations(cfg *Config) {
	notices := cfg.Notices
	if l.profile == "" {
		// --profile decides the profile, so a note about the profile of a config file that sets none is wrong then
		notices = append(slices.Clone(notices), cfg.profileNotices...)
	}
	if len(cfg.Deprecations) == 0 && len(notices) == 0 {
		return
	}
	if _, loaded := l.warned.LoadOrStore(cfg, struct{}{}); loaded {
		return
	}
	l.notesMu.Lock()
	l.notes = append(l.notes, cfg.Deprecations...)
	l.notes = append(l.notes, notices...)
	l.notesMu.Unlock()
	if structured(l.printer) {
		return // The warnings are in the output document
	}
	for _, d := range cfg.Deprecations {
		fmt.Fprintln(l.logOut, "warning:", d)
	}
	for _, n := range notices {
		fmt.Fprintln(l.logOut, "note:", n)
	}
}

// warnOnce reports a warning about the run once, however many files cause it.
func (l *Linter) warnOnce(msg string) {
	if _, loaded := l.warnedOnce.LoadOrStore(msg, struct{}{}); loaded {
		return
	}
	l.notesMu.Lock()
	l.notes = append(l.notes, msg)
	l.notesMu.Unlock()
	if !structured(l.printer) {
		fmt.Fprintln(l.logOut, "warning:", msg)
	}
}

// warnRetiredIgnores warns about the ignore patterns written with a retired rule ID.
func (l *Linter) warnRetiredIgnores(pats ...IgnorePattern) {
	for _, p := range pats {
		if m := p.deprecation(); m != "" {
			l.warnOnce(m)
		}
	}
}

// notifications returns the warnings about the run itself (the deprecated keys of the configs which were
// used) in sorted order so that the output does not depend on the order files were checked in.
func (l *Linter) notifications() []string {
	l.notesMu.Lock()
	defer l.notesMu.Unlock()
	ret := slices.Clone(l.notes)
	slices.Sort(ret)
	return slices.Compact(ret)
}

// annotateErrors fills the fields of the errors which are derived from the diagnostic ID: the
// severity, the documentation URL and the end position of the region. Errors of rules which the
// config turns off are removed.
func (l *Linter) annotateErrors(errs []*Error, src []byte, cfg *Config) []*Error {
	lines := sourceLines(src)
	kept := errs[:0]
	for _, err := range errs {
		if err.ID == "" {
			err.ID = err.Kind
		}
		err.Severity = cfg.RuleLevel(err.ID)
		if err.Severity == SeverityOff {
			l.debug("Error %q is dropped because rule %q is off", err.Message, err.ID)
			continue
		}
		if info, ok := ruleIndex[err.ID]; ok {
			err.DocURL = info.DocURL()
		}
		err.fillRegion(lines)
		kept = append(kept, err)
	}
	return kept
}

// lazyCallGraph builds the call graph of a repository once, however many files ask for it in parallel.
type lazyCallGraph struct {
	once  sync.Once
	graph *callGraph
}

// callGraphOf returns the call graph of the project. The linter builds it when the first action.yml (or
// the list of the actions of the repository) is needed and keeps it for the rest of the run, so that
// every workflow and action is read once.
func (l *Linter) callGraphOf(p *Project) *callGraph {
	v, _ := l.graphs.LoadOrStore(p.root, &lazyCallGraph{})
	lg := v.(*lazyCallGraph)
	lg.once.Do(func() { lg.graph = newCallGraph(p.root) })
	return lg.graph
}

// actionCallers returns the local workflows which run the action defined in the file, or nil when the
// file is not in the project.
func (l *Linter) actionCallers(p *Project, file string) *ActionCallers {
	rel, err := filepath.Rel(absPath(p.root), absPath(file))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	return l.callGraphOf(p).callersOf(filepath.ToSlash(filepath.Dir(rel)))
}

package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
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
	// LogLevelVerbose shows verbose log output. This is equivalent to specifying -verbose option
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
	// ShowRuleIDs makes the text format show the stable rule ID such as "unpinned-uses" at the end of
	// each error instead of the kind such as "action".
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
	// GitHubClient replaces the built-in client of the GitHub API, which sends REST requests and caches
	// the answers in $XDG_CACHE_HOME/jactionlint. It is used by tests (see NewFixtureGitHubClient)
	// and implies nothing by itself: the online rules need Online or the "online" configuration.
	GitHubClient GitHubClient
	// OnlineCacheTTL is how long the built-in client uses a cached answer without asking GitHub
	// whether it changed. Zero means one hour. A negative value revalidates every answer (which
	// costs no rate limit when nothing changed).
	OnlineCacheTTL time.Duration
	// Context stops the online lookups when it is canceled, for example on interruption. Nil means
	// context.Background.
	Context context.Context
	// OnRulesCreated is a hook to add or remove the check rules. This function is called on checking
	// every workflow files. Rules created by Linter instance are passed to the argument and the
	// function should return the modified rules.
	// Note that syntax errors may be reported even if this function returns nil or an empty slice.
	OnRulesCreated func([]Rule) []Rule
	// OnDependabotRulesCreated is like OnRulesCreated but for the rules which check Dependabot
	// configuration files (.github/dependabot.yml).
	OnDependabotRulesCreated func([]DependabotRule) []DependabotRule
	// More options will come here
}

// Linter is struct to lint workflow files.
type Linter struct {
	projects       *Projects
	out            io.Writer
	logOut         io.Writer
	logLevel       LogLevel
	printer        printer
	shellcheck     string
	pyflakes       string
	ignorePats     IgnorePatterns
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
	warned         sync.Map // *Config -> struct{}: configs whose deprecations were already reported
	notesMu        sync.Mutex
	notes          []string // deprecation warnings found while linting
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
		cfg = c
	} else if opts.Config != nil {
		cfg = opts.Config
	}

	// Load the user-global config as a fallback for projects which have no
	// .github/jactionlint.yaml. The -config-file option takes precedence over it.
	var globalCfg *Config
	var globalCfgPath string
	if opts.ConfigFile == "" && opts.Config == nil {
		c, p, err := loadGlobalConfig()
		if err != nil {
			return nil, err
		}
		globalCfg, globalCfgPath = c, p
	}

	ignore := make(IgnorePatterns, 0, len(opts.IgnorePatterns))
	for _, s := range opts.IgnorePatterns {
		r, err := ParseIgnorePattern(s)
		if err != nil {
			return nil, fmt.Errorf("invalid regular expression for ignore pattern %q: %s", s, err.Error())
		}
		ignore = append(ignore, r)
	}

	var formatter *ErrorFormatter
	if isTemplateFormat(opts.Format) {
		f, err := NewErrorFormatter(opts.Format)
		if err != nil {
			return nil, err
		}
		formatter = f
	}
	prn, err := newPrinter(opts.Format, opts.Oneline, opts.ShowRuleIDs, formatter)
	if err != nil {
		return nil, err
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
		stdin:          stdin,
		defaultConfig:  cfg,
		globalConfig:   globalCfg,
		errFmt:         formatter,
		cwd:            cwd,
		onRulesCreated: opts.OnRulesCreated,
		onDependabot:   opts.OnDependabotRulesCreated,
		configFile:     opts.ConfigFile,
		minSeverity:    opts.MinSeverity,
		online:         onlineSettings{enabled: opts.Online, client: opts.GitHubClient, ttl: opts.OnlineCacheTTL, ctx: opts.Context},
	}
	if opts.Online && opts.GitHubClient == nil && !onlineSupported {
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

	p, err := l.projects.At(dir)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("no project was found in any parent directories of %q. check workflows directory is put correctly in your Git repository", dir)
	}

	l.log("Detected project:", p.RootDir())
	files, err := walkWorkflowFiles(p.WorkflowsDir())
	if err != nil {
		return nil, err
	}
	files = append(files, p.DependabotFiles()...)
	if len(files) == 0 {
		return nil, fmt.Errorf("no YAML file was found in %q", p.WorkflowsDir())
	}
	l.log("Collected", len(files), "YAML files")
	return l.LintFiles(files, p)
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

	l.log("Found", total, "errors in", n, "files")

	return all, nil
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
			w.src = src
			w.errs = errs
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// Ensure that all processes finish. `proc.wait()` must be called after `eg.Wait()`.
	// Calling `WaitGroup.Add` after `WaitGroup.Wait` can cause a race condition (specifically when
	// increasing the group count from 0 to 1 and calling `Wait` and at the same time).
	// `WaitGroup.Add` is called in `proc.run()` and `WaitGroup.Wait` is called in `proc.wait()`.
	// After traversing all workflows, `proc.run()` is no longer called so `proc.wait()` can be
	// called safely.
	proc.wait()

	return ws, nil
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

	if err := l.printer.print(l.out, []fileResult{{file: origPath, path: path, src: src, errs: errs}}, l.notifications()); err != nil {
		return nil, err
	}
	return errs, nil
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
	if err := l.printer.print(l.out, []fileResult{{file: path, path: path, src: content, errs: errs}}, l.notifications()); err != nil {
		return nil, err
	}
	return errs, nil
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

	// Config priority: -config-file option, then repository config, then user-global config
	var cfg *Config
	if l.defaultConfig != nil {
		cfg = l.defaultConfig
	} else if project != nil && project.Config() != nil {
		cfg = project.Config()
	} else {
		cfg = l.globalConfig
	}
	if cfg != nil {
		l.debug("Config: %#v", cfg)
		l.warnDeprecations(cfg)
	} else {
		l.debug("No config was found")
	}

	if l.isDependabotFile(path) {
		return l.checkDependabot(path, content, project, cfg, start)
	}

	w, all := Parse(content)

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
			errs := rule.Errs()
			l.debug("%s found %d errors", rule.Name(), len(errs))
			all = append(all, errs...)
		}

		if l.errFmt != nil {
			for _, rule := range rules {
				l.errFmt.RegisterRule(rule)
			}
		}
	}

	return l.finishCheck(path, content, all, cfg, start, w != nil), nil
}

// finishCheck post-processes the errors found in one file: it fills the fields derived from the
// rule IDs, applies the ignores and the minimum severity, and sorts the errors. isWorkflow tells that
// the file is a workflow, for which the online pin fixes are attached.
func (l *Linter) finishCheck(path string, content []byte, all []*Error, cfg *Config, start time.Time, isWorkflow bool) []*Error {
	all = l.annotateErrors(all, content, cfg)

	// Inline ignores are applied first so that every pattern sees all errors, which tells whether it
	// is used. The order of the filters does not change which errors remain.
	inlineIgnores, orphans, ignoreErrs := parseInlineIgnoresWithOrphans(content)
	all = l.filterInlineIgnores(all, inlineIgnores)
	unused := unusedInlineIgnores(inlineIgnores, orphans, cfg)
	dropFixesChangingYAML(content, unused)
	all = append(all, l.annotateErrors(unused, content, cfg)...)

	all = l.filterErrors(all, cfg.PathConfigs(path))
	all = append(all, l.annotateErrors(ignoreErrs, content, cfg)...)
	if isWorkflow {
		if sess, _ := l.onlineSession(cfg); sess != nil {
			l.attachPinFixes(sess, content, all)
		}
	}

	if l.minSeverity > SeverityInfo {
		kept := all[:0]
		for _, err := range all {
			if err.Severity >= l.minSeverity {
				kept = append(kept, err)
			}
		}
		all = kept
	}

	for _, err := range all {
		err.Filepath = path // Populate filename in the error
	}

	slices.SortFunc(all, compareErrors)
	all = slices.CompactFunc(all, equalsErrors) // Alias may duplicate errors

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

	filtered := make([]*Error, 0, len(errs))
Loop:
	for _, err := range errs {
		if l.ignorePats.Match(err) {
			l.debug("Error %q is ignored due to -ignore command line option", err.Message)
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
		l.log("Filtered", len(errs)-len(filtered), "error(s) due to \"-ignore\" command line option and \"ignore\" configuration")
	}
	return filtered
}

// warnDeprecations reports the deprecated keys of the config to the log output. It reports each
// config only once even if many files are linted with it.
func (l *Linter) warnDeprecations(cfg *Config) {
	if len(cfg.Deprecations) == 0 {
		return
	}
	if _, loaded := l.warned.LoadOrStore(cfg, struct{}{}); loaded {
		return
	}
	l.notesMu.Lock()
	l.notes = append(l.notes, cfg.Deprecations...)
	l.notesMu.Unlock()
	if structured(l.printer) {
		return // The warnings are in the output document
	}
	for _, d := range cfg.Deprecations {
		fmt.Fprintln(l.logOut, "warning:", d)
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

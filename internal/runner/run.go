package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
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
	cache      string
	noCache    bool
	cacheDir   string
	cacheSize  string
	shims      []string
	allowNet   []string
	learn      bool

	parallel               int
	dryRun                 bool
	container              string // "" off, "default" for --container, or an image
	images                 []string
	platform               string
	engine                 string
	noContainers           bool
	trustImages            bool
	unsafeContainerOptions bool
	noShim                 []string
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
	if ro.container != "" && ro.noContainers {
		fmt.Fprintln(stderr, "jactionlint run: --container and --no-containers contradict each other")
		return ExitUsage
	}
	if ro.parallel < 1 {
		fmt.Fprintln(stderr, "jactionlint run: --parallel must be at least 1")
		return ExitUsage
	}
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
	if ro.learn && ro.net != "proxy" && ro.net != "open" && ro.net != "" {
		fmt.Fprintln(stderr, "jactionlint run: --learn needs --net=proxy")
		return ExitUsage
	}
	if ro.learn {
		ro.net = "proxy"
	}
	if len(ro.allowNet) > 0 && ro.net != "proxy" {
		fmt.Fprintln(stderr, "jactionlint run: --allow-net only applies to --net=proxy")
		return ExitUsage
	}
	var net sandbox.Net
	switch ro.net {
	case "open", "":
		net = sandbox.NetOpen
	case "none":
		net = sandbox.NetNone
	case "proxy":
		net = sandbox.NetProxy
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
	if useSandbox && !ro.dryRun {
		if err := sandbox.Available(net); err != nil {
			ref := explainRef("sandbox-unavailable")
			if runtime.GOOS == "windows" {
				ref = explainRef("windows")
			}
			fmt.Fprintf(stderr, "jactionlint run: steps cannot run isolated on this machine: %v%s\n", err, ref)
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
	images, err := parseKV("image", ro.images)
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitUsage
	}
	for k, v := range images {
		images[strings.ToLower(k)] = v
	}
	containerImage := ""
	if ro.container != "" && ro.container != "default" {
		containerImage = ro.container
	}
	cacheMode := ro.cache
	if ro.noCache {
		cacheMode = "off"
	}
	switch cacheMode {
	case "on", "read-only", "off":
	case "":
		cacheMode = "on"
	default:
		fmt.Fprintf(stderr, "jactionlint run: --cache must be on, read-only or off, not %q\n", ro.cache)
		return ExitUsage
	}
	var cacheSize int64
	if ro.cacheSize != "" {
		n, err := parseSize(ro.cacheSize)
		if err != nil {
			fmt.Fprintf(stderr, "jactionlint run: --cache-size: %v\n", err)
			return ExitUsage
		}
		cacheSize = n
	}
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

	var proxy *Proxy
	if net == sandbox.NetProxy {
		var hosts []string
		for _, h := range ro.allowNet {
			hosts = append(hosts, strings.Split(h, ",")...)
		}
		if proxy, err = StartProxy(hosts, ro.learn); err != nil {
			fmt.Fprintf(stderr, "jactionlint run: cannot start the proxy: %v\n", err)
			return ExitError
		}
		defer proxy.Close()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := &Config{
		Root: root, In: in, Secrets: secrets, EnvFile: envFile,
		Sandbox: useSandbox, Net: net, InPlace: ro.inPlace, PassEnv: ro.passEnv, AllowRead: ro.allowRead, AllowWrite: ro.allowWrite,
		ContainerEngine: ro.engine, NoContainers: ro.noContainers, ContainerMode: ro.container != "", ContainerImage: containerImage,
		Images: images, Platform: ro.platform, TrustImages: ro.trustImages, UnsafeContainerOptions: ro.unsafeContainerOptions,
		Parallel: ro.parallel, DryRun: ro.dryRun,
		Proxy: proxy, TrustActions: ro.trust, NoShim: ro.noShim, EnableShim: ro.shims, Cache: cacheMode, CacheDir: ro.cacheDir, CacheSize: cacheSize, Debug: ro.debug, Timings: ro.timings, Keep: ro.keep, ParentEnv: os.Environ(), Out: stdout,
	}
	if !useSandbox && !ro.dryRun {
		fmt.Fprintln(stdout, "note: running without the sandbox (--sandbox=off): steps can read and write what you can")
	}
	eng := NewEngine(cfg, plan)
	ok, err := eng.Run(ctx)
	if proxy != nil {
		reportProxy(stdout, proxy, ro.learn)
	}
	if err != nil {
		fmt.Fprintf(stderr, "jactionlint run: %v\n", err)
		return ExitError
	}
	if !ok {
		return ExitFailure
	}
	return ExitOK
}

// parseSize reads sizes such as 500M, 2G or a number of bytes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := int64(1)
	for suffix, m := range map[string]int64{"K": 1 << 10, "M": 1 << 20, "G": 1 << 30, "T": 1 << 40} {
		if strings.HasSuffix(s, suffix) || strings.HasSuffix(s, suffix+"B") {
			s = strings.TrimSuffix(strings.TrimSuffix(s, "B"), suffix)
			mult = m
			break
		}
	}
	var n float64
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%g", &n); err != nil || n < 0 {
		return 0, fmt.Errorf("%q is not a size (use 500M, 2G or bytes)", s)
	}
	return int64(n * float64(mult)), nil
}

// reportProxy tells what the proxy saw: the hosts to allow (--learn) or the hosts that were refused.
func reportProxy(w io.Writer, p *Proxy, learn bool) {
	if learn {
		hosts := p.Hosts()
		if len(hosts) == 0 {
			fmt.Fprintln(w, "\nnetwork: the steps contacted no host through the proxy")
			return
		}
		fmt.Fprintf(w, "\nnetwork: the steps contacted %d host(s) through the proxy:\n", len(hosts))
		for _, h := range hosts {
			fmt.Fprintf(w, "  %s\n", h)
		}
		fmt.Fprintf(w, "to run with --net=proxy and allow them: --allow-net %s\n", strings.Join(hosts, ","))
		return
	}
	if denied := p.Denied(); len(denied) > 0 {
		fmt.Fprintf(w, "\nnetwork: --net=proxy refused %d host(s): %s\nallow them with --allow-net %s%s\n", len(denied), strings.Join(denied, ", "), strings.Join(denied, ","), explainRef("net-proxy-refused"))
	}
}

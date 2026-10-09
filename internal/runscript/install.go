package runscript

import (
	"regexp"
	"strings"
)

// Package kinds.
const (
	// KindRegistry is a package from the default registry (PyPI, npm, crates.io, rubygems, apt, Homebrew).
	KindRegistry = "registry"
	// KindModule is a Go module path.
	KindModule = "module"
	// KindGit is a git repository (`git+https://...@ref`, `github:o/r#ref`, `--git`).
	KindGit = "git"
	// KindURL is a download URL.
	KindURL = "url"
	// KindPath is a local path or archive.
	KindPath = "path"
	// KindDynamic is a word which is entirely an expression, variable or substitution: nothing is known.
	KindDynamic = "dynamic"
)

// Install describes a command which installs packages or runs them without installing (`npx`).
//
// Pinning is judged from the command line only. "Pinned" means an exact version (or a commit/tag for git
// sources, a digest for URLs) is named. Ranges (`>=1`, `^1`, `~=1.2`), tags (`latest`) and nothing at all are
// not pinned. A version which is a variable or an expression (`pkg==$V`, `pkg@${{ matrix.v }}`) counts as
// pinned: the author chose a version explicitly, and flagging it would be noise.
type Install struct {
	Cmd *Command
	// Tool is the tool of the command (Command.Tool). `uv pip install` is Tool "uv".
	Tool string
	// Ecosystem is "pypi", "npm", "crates", "go", "rubygems", "apt" or "brew".
	Ecosystem string
	// Verb is the subcommand: install, add, ci, get, binstall, run, dlx, exec, ...
	Verb string
	// Run is whether the packages are only run, not installed (npx, pnpm dlx, pipx run, uvx, ...).
	Run bool
	// Global is whether the installation is explicitly global (`-g`, `--global`, `yarn global`).
	Global bool
	// Packages are the packages named on the command line.
	Packages []*Package
	// Requirements are the requirement files (`pip install -r requirements.txt`). Their content is unknown.
	Requirements []*Word
	// Locked is whether the installation is bound to a lock file or hashes: `npm ci`, `--frozen-lockfile`,
	// `--immutable`, `--locked`, `--frozen`, `--require-hashes`, `uv sync`.
	Locked bool
	// FromManifest is whether no package is named: the project's manifest and lock file decide (`npm install`,
	// `pip install -r file`, `uv sync`).
	FromManifest bool
}

// Package is one package argument of an install.
type Package struct {
	// Word is the argument. For packages named by a flag (`--version`, `--from`) it is the word of the name.
	Word *Word
	// Spec is the argument as given, `requests==2.0` or `left-pad@1.3.0`.
	Spec string
	// Name is the package name without version, empty for dynamic arguments.
	Name string
	// Version is the version or requirement part, empty if there is none.
	Version string
	// Kind is one of the Kind constants.
	Kind string
	// Pinned is whether an exact version is named, see [Install].
	Pinned bool
	// Dynamic is whether the argument contains an expression, variable or substitution.
	Dynamic bool
	// Local is whether the package is a path on disk.
	Local bool
}

var (
	reExactNPM   = regexp.MustCompile(`^=?v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
	reExactCargo = regexp.MustCompile(`^=?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
	reExactGem   = regexp.MustCompile(`^(=\s*)?\d+(\.[0-9A-Za-z]+)*$`)
	reHex        = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
	// A version tag has at least a minor version. "v1" is a moving major tag, and "2024-release" or
	// "v1-nightly" are names, not versions.
	reTagLike = regexp.MustCompile(`^v?\d+(\.\d+)+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
	rePyName  = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)(\[[^\]]*\])?\s*(.*)$`)
	reDigest  = regexp.MustCompile(`(sha256|sha384|sha512)[=:]|@sha256:`)
)

// installOf classifies the command, nil if it is not an install.
func installOf(c *Command) *Install {
	if c.Tool == "" || c.NameWord == nil {
		return nil
	}
	in := &Install{Cmd: c, Tool: c.Tool}
	var ok bool
	switch c.Tool {
	case "pip":
		ok = in.pip(c, c.Positional, 0)
	case "uv":
		ok = in.uv(c)
	case "uvx":
		in.Ecosystem, in.Run, in.Verb = "pypi", true, "run"
		in.runTarget(c, c.Positional, "--from", pypiPackage)
		ok = true
	case "pipx":
		ok = in.pipx(c)
	case "npm", "pnpm", "yarn", "bun", "aube":
		ok = in.node(c)
	case "npx", "bunx", "pnpm-dlx":
		in.Ecosystem, in.Run, in.Verb = "npm", true, "run"
		in.runTarget(c, c.Positional, "--package", npmPackage, "-p")
		ok = true
	case "cargo":
		ok = in.cargo(c)
	case "go":
		ok = in.golang(c)
	case "gem":
		ok = in.gem(c)
	case "apt":
		ok = in.apt(c)
	case "brew":
		ok = in.brew(c)
	}
	if !ok {
		return nil
	}
	if len(in.Packages) == 0 && len(in.Requirements) == 0 && !in.Run {
		in.FromManifest = true
	}
	return in
}

func (in *Install) add(w *Word, parse func(*Word) *Package) {
	in.Packages = append(in.Packages, parse(w))
}

func (in *Install) addAll(ws []*Word, parse func(*Word) *Package) {
	for _, w := range ws {
		in.add(w, parse)
	}
}

// runTarget handles `npx pkg args...`: the package is the first positional, or the values of the package flags.
func (in *Install) runTarget(c *Command, pos []*Word, flag string, parse func(*Word) *Package, alt ...string) {
	names := append([]string{flag}, alt...)
	if vs := c.FlagValues(names...); len(vs) > 0 {
		in.addAll(vs, parse)
		return
	}
	if len(pos) > 0 {
		in.add(pos[0], parse)
	}
}

func dynamicPackage(w *Word) *Package {
	return &Package{Word: w, Spec: w.Value, Kind: KindDynamic, Dynamic: true}
}

// entirelyDynamic returns whether the word starts with an expansion: its package name is not known.
func entirelyDynamic(w *Word) bool {
	return w.ProcSubst || strings.HasPrefix(w.Value, "$") || strings.HasPrefix(w.Value, "`")
}

// versionIsDynamic returns whether the version comes from an expression or variable.
func versionIsDynamic(v string) bool { return strings.Contains(v, "${{") || strings.Contains(v, "$") }

func isLocalPath(v string) bool {
	return v == "." || v == ".." || strings.HasPrefix(v, "./") || strings.HasPrefix(v, "../") || strings.HasPrefix(v, "/") ||
		strings.HasPrefix(v, "~") || strings.HasPrefix(v, `.\`) || strings.HasPrefix(v, `..\`)
}

// refPinned: a git ref which names a commit or a version tag.
func refPinned(ref string) bool {
	return reHex.MatchString(ref) || reTagLike.MatchString(ref) || versionIsDynamic(ref)
}

func finish(p *Package, w *Word) *Package {
	p.Word, p.Spec = w, w.Value
	p.Dynamic = w.Dynamic()
	if p.Kind == KindPath {
		p.Local = true
	}
	return p
}

// pypiPackage parses a requirement specifier (PEP 508 subset).
func pypiPackage(w *Word) *Package {
	if entirelyDynamic(w) {
		return dynamicPackage(w)
	}
	v := strings.TrimSpace(w.Value)
	if i := strings.Index(v, ";"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	p := &Package{Kind: KindRegistry}
	switch {
	case isLocalPath(v) || (w.Glob && strings.Contains(v, "/")) || (!strings.Contains(v, "://") && !strings.Contains(v, " @ ") && archiveExt(v)):
		p.Kind, p.Name = KindPath, v
	case strings.Contains(v, "://") || strings.HasPrefix(v, "git+") || strings.Contains(v, " @ "):
		name, url := "", v
		if i := strings.Index(v, " @ "); i >= 0 {
			name, url = strings.TrimSpace(v[:i]), strings.TrimSpace(v[i+3:])
		}
		p.Name, p.Version = name, url
		p.Kind = KindURL
		if strings.HasPrefix(url, "git+") || strings.HasSuffix(strings.SplitN(url, "#", 2)[0], ".git") || strings.Contains(strings.SplitN(url, "@", 2)[0], "git+") {
			p.Kind = KindGit
			p.Pinned = urlRefPinned(url)
		} else {
			p.Pinned = reDigest.MatchString(url)
		}
	default:
		m := rePyName.FindStringSubmatch(v)
		if m == nil {
			p.Kind, p.Name = KindDynamic, v
			break
		}
		p.Name, p.Version = m[1], strings.TrimSpace(m[3])
		if i := strings.Index(p.Version, "=="); i >= 0 {
			rest := p.Version[i+2:]
			p.Pinned = !strings.HasSuffix(strings.TrimSpace(rest), "*") // == 1.* is a range
		}
		if strings.HasPrefix(p.Name, "$") {
			p.Kind = KindDynamic
		}
	}
	return finish(p, w)
}

func archiveExt(v string) bool {
	for _, e := range []string{".whl", ".tar.gz", ".tgz", ".zip", ".tar.bz2", ".tar.xz", ".tar"} {
		if strings.HasSuffix(v, e) {
			return true
		}
	}
	return false
}

// urlRefPinned: `scheme://host/path@ref#fragment`. Only a ref after the host counts.
func urlRefPinned(url string) bool {
	if i := strings.Index(url, "#"); i >= 0 {
		url = url[:i]
	}
	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
	}
	// strip user@ before the host
	slash := strings.Index(url, "/")
	if slash < 0 {
		return false
	}
	path := url[slash:]
	at := strings.LastIndex(path, "@")
	return at >= 0 && refPinned(path[at+1:])
}

// npmPackage parses an npm/yarn/pnpm/bun package argument.
func npmPackage(w *Word) *Package {
	if entirelyDynamic(w) {
		return dynamicPackage(w)
	}
	v := w.Value
	p := &Package{Kind: KindRegistry}
	switch {
	case isLocalPath(v) || w.Glob || strings.HasPrefix(v, "file:") || strings.HasPrefix(v, "link:") || strings.HasPrefix(v, "workspace:") ||
		(!strings.Contains(v, "://") && archiveExt(v) && !strings.HasPrefix(v, "@")):
		p.Kind, p.Name = KindPath, v
	case strings.HasPrefix(v, "git+") || strings.HasPrefix(v, "git://") || strings.HasPrefix(v, "github:") || strings.HasPrefix(v, "gitlab:") ||
		strings.HasPrefix(v, "bitbucket:") || strings.HasPrefix(v, "gist:") || strings.HasPrefix(v, "ssh://") || isGithubShorthand(v):
		p.Kind, p.Name = KindGit, v
		if i := strings.LastIndex(v, "#"); i >= 0 {
			p.Name, p.Version = v[:i], v[i+1:]
			ref := strings.TrimPrefix(p.Version, "semver:")
			p.Pinned = refPinned(ref) && !strings.ContainsAny(ref, "^~*<>|xX ") || reHex.MatchString(ref)
		}
	case strings.Contains(v, "://"):
		p.Kind, p.Name = KindURL, v
		p.Pinned = reDigest.MatchString(v)
	default:
		name, ver := v, ""
		if i := strings.LastIndex(v, "@"); i > 0 {
			name, ver = v[:i], v[i+1:]
		}
		p.Name, p.Version = name, ver
		p.Pinned = ver != "" && (reExactNPM.MatchString(ver) || versionIsDynamic(ver))
		if strings.HasPrefix(ver, "npm:") { // alias: name@npm:other@1.2.3
			if i := strings.LastIndex(ver, "@"); i > 4 {
				p.Pinned = reExactNPM.MatchString(ver[i+1:])
			}
		}
	}
	return finish(p, w)
}

// isGithubShorthand reports `owner/repo` and `owner/repo#ref` (npm treats them as GitHub repositories).
func isGithubShorthand(v string) bool {
	if strings.HasPrefix(v, "@") || strings.HasPrefix(v, ".") || strings.HasPrefix(v, "/") {
		return false
	}
	base := v
	if i := strings.Index(v, "#"); i >= 0 {
		base = v[:i]
	}
	return strings.Count(base, "/") == 1 && !strings.Contains(base, "@") && !strings.Contains(base, ":")
}

// pip implements `pip install` for the positional words starting at pos[0] being the verb.
func (in *Install) pip(c *Command, pos []*Word, _ int) bool {
	in.Ecosystem = "pypi"
	if len(pos) == 0 || pos[0].Dynamic() || pos[0].Value != "install" {
		return false
	}
	in.Verb = "install"
	in.addAll(pos[1:], pypiPackage)
	in.addAll(c.FlagValues("-e", "--editable"), pypiPackage)
	in.Requirements = append(in.Requirements, c.FlagValues("-r", "--requirement")...)
	in.Locked = c.HasFlag("--require-hashes")
	return true
}

func (in *Install) uv(c *Command) bool {
	in.Ecosystem = "pypi"
	pos := c.Positional
	if len(pos) == 0 || pos[0].Dynamic() {
		return false
	}
	switch pos[0].Value {
	case "pip":
		if len(pos) < 2 || pos[1].Dynamic() {
			return false
		}
		switch pos[1].Value {
		case "install":
			in.Locked = c.HasFlag("--require-hashes")
			in.Verb = "install"
			in.addAll(pos[2:], pypiPackage)
			in.addAll(c.FlagValues("-e", "--editable"), pypiPackage)
			in.Requirements = append(in.Requirements, c.FlagValues("-r", "--requirement")...)
			return true
		case "sync":
			in.Verb, in.Locked = "sync", true
			return true
		}
	case "tool":
		if len(pos) < 2 || pos[1].Dynamic() {
			return false
		}
		switch pos[1].Value {
		case "install":
			in.Verb = "install"
			in.fromOr(c, pos[2:], pypiPackage)
			return true
		case "run":
			in.Verb, in.Run = "run", true
			in.runTarget(c, pos[2:], "--from", pypiPackage)
			return true
		}
	case "add":
		in.Verb = "add"
		in.addAll(pos[1:], pypiPackage)
		in.Requirements = append(in.Requirements, c.FlagValues("-r", "--requirements")...)
		return true
	case "sync":
		in.Verb, in.Locked = "sync", true
		in.Locked = true
		return true
	}
	return false
}

// fromOr: `--from SPEC` names the package of a tool, the positional is then the executable.
func (in *Install) fromOr(c *Command, pos []*Word, parse func(*Word) *Package) {
	if vs := c.FlagValues("--from"); len(vs) > 0 {
		in.addAll(vs, parse)
		return
	}
	in.addAll(pos, parse)
}

func (in *Install) pipx(c *Command) bool {
	in.Ecosystem = "pypi"
	pos := c.Positional
	if len(pos) == 0 || pos[0].Dynamic() {
		return false
	}
	switch pos[0].Value {
	case "install":
		in.Verb = "install"
		in.fromOr(c, pos[1:], pypiPackage)
		if vs := c.FlagValues("--spec"); len(vs) > 0 {
			in.Packages = nil
			in.addAll(vs, pypiPackage)
		}
		return true
	case "run":
		in.Verb, in.Run = "run", true
		if vs := c.FlagValues("--spec"); len(vs) > 0 {
			in.addAll(vs, pypiPackage)
		} else if len(pos) > 1 {
			in.add(pos[1], pypiPackage)
		}
		return true
	case "inject":
		in.Verb = "inject"
		if len(pos) > 2 {
			in.addAll(pos[2:], pypiPackage)
		}
		return true
	}
	return false
}

var (
	nodeInstallVerbs = set("install i add in ins inst insta instal isnt isnta isntal isntall a")
	nodeCIVerbs      = set("ci clean-install cit clean-install-test install-ci-test")
)

// node handles npm, pnpm, yarn, bun and aube.
func (in *Install) node(c *Command) bool {
	in.Ecosystem = "npm"
	pos := c.Positional
	global := c.HasFlag("-g", "--global") || c.Flag("--location") != nil && c.Flag("--location").Value != nil && c.Flag("--location").Value.Value == "global"
	if c.Tool == "yarn" && len(pos) > 0 && !pos[0].Dynamic() && pos[0].Value == "global" {
		global, pos = true, pos[1:]
	}
	in.Global = global
	in.Locked = c.HasFlag("--frozen-lockfile", "--immutable", "--ci") && !c.HasFlag("--no-frozen-lockfile")
	if len(pos) == 0 {
		switch c.Tool {
		case "yarn": // `yarn` installs
			in.Verb = "install"
			return true
		}
		return false
	}
	if pos[0].Dynamic() {
		return false
	}
	verb := pos[0].Value
	switch {
	case verb == "dlx" && (c.Tool == "pnpm" || c.Tool == "yarn"), verb == "x" && c.Tool == "bun", (verb == "exec" || verb == "x") && c.Tool == "npm":
		in.Verb, in.Run = verb, true
		in.runTarget(c, pos[1:], "--package", npmPackage, "-p")
		return true
	case nodeCIVerbs[verb]:
		in.Verb, in.Locked = "ci", true
		return true
	case nodeInstallVerbs[verb]:
		in.Verb = verb
		if verb == "a" && c.Tool != "bun" && c.Tool != "aube" {
			return false
		}
		if c.Tool == "bun" || c.Tool == "aube" || c.Tool == "pnpm" || c.Tool == "yarn" || c.Tool == "npm" {
			in.addAll(pos[1:], npmPackage)
		}
		return true
	}
	return false
}

func (in *Install) cargo(c *Command) bool {
	in.Ecosystem = "crates"
	pos := c.Positional
	if len(pos) == 0 || pos[0].Dynamic() || (pos[0].Value != "install" && pos[0].Value != "binstall") {
		return false
	}
	in.Verb = pos[0].Value
	in.Locked = c.HasFlag("--locked", "--frozen")
	version := ""
	if vs := c.FlagValues("--version", "--vers"); len(vs) > 0 {
		version = vs[len(vs)-1].Value
	}
	gitPinned := false
	if c.HasFlag("--git") {
		for _, w := range c.FlagValues("--rev", "--tag") {
			gitPinned = gitPinned || refPinned(w.Value)
		}
	}
	for _, w := range pos[1:] {
		if entirelyDynamic(w) {
			in.Packages = append(in.Packages, dynamicPackage(w))
			continue
		}
		p := &Package{Kind: KindRegistry, Name: w.Value}
		if i := strings.Index(w.Value, "@"); i > 0 {
			p.Name, p.Version = w.Value[:i], w.Value[i+1:]
		} else {
			p.Version = version
		}
		p.Pinned = p.Version != "" && (reExactCargo.MatchString(p.Version) || versionIsDynamic(p.Version))
		if c.HasFlag("--git") {
			p.Kind = KindGit
			p.Pinned = p.Pinned || gitPinned
		}
		in.Packages = append(in.Packages, finish(p, w))
	}
	for _, w := range c.FlagValues("--path") {
		in.Packages = append(in.Packages, finish(&Package{Kind: KindPath, Name: w.Value}, w))
	}
	if len(pos) == 1 && !c.HasFlag("--path") {
		if gits := c.FlagValues("--git"); len(gits) > 0 {
			in.Packages = append(in.Packages, finish(&Package{Kind: KindGit, Name: gits[0].Value, Pinned: gitPinned}, gits[0]))
		}
	}
	return true
}

// reGoExactVersion matches a version of a Go module which names one release: vMAJOR.MINOR.PATCH with an optional
// prerelease and build part, which includes the pseudo-versions (v0.0.0-20240101120000-0123456789ab). Queries
// such as v1, v1.2, latest, upgrade, patch or a branch name resolve to whatever matches at the time.
var reGoExactVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func (in *Install) golang(c *Command) bool {
	in.Ecosystem = "go"
	pos := c.Positional
	if len(pos) == 0 || pos[0].Dynamic() || (pos[0].Value != "install" && pos[0].Value != "get") {
		return false
	}
	in.Verb = pos[0].Value
	for _, w := range pos[1:] {
		if entirelyDynamic(w) {
			in.Packages = append(in.Packages, dynamicPackage(w))
			continue
		}
		p := &Package{Kind: KindModule, Name: w.Value}
		if i := strings.LastIndex(w.Value, "@"); i > 0 {
			p.Name, p.Version = w.Value[:i], w.Value[i+1:]
			p.Pinned = reGoExactVersion.MatchString(p.Version) || reHex.MatchString(p.Version) || versionIsDynamic(p.Version)
		}
		if isLocalPath(w.Value) || w.Glob {
			p.Kind = KindPath
		}
		in.Packages = append(in.Packages, finish(p, w))
	}
	return true
}

func (in *Install) gem(c *Command) bool {
	in.Ecosystem = "rubygems"
	pos := c.Positional
	if len(pos) == 0 || pos[0].Dynamic() || pos[0].Value != "install" {
		return false
	}
	in.Verb = "install"
	version := ""
	for _, v := range c.FlagValues("-v", "--version") {
		version = v.Value
	}
	for _, w := range pos[1:] {
		if entirelyDynamic(w) {
			in.Packages = append(in.Packages, dynamicPackage(w))
			continue
		}
		p := &Package{Kind: KindRegistry, Name: w.Value, Version: version}
		if i := strings.Index(w.Value, ":"); i > 0 {
			p.Name, p.Version = w.Value[:i], w.Value[i+1:]
		}
		if isLocalPath(w.Value) || strings.HasSuffix(w.Value, ".gem") {
			p.Kind = KindPath
		}
		p.Pinned = p.Version != "" && (reExactGem.MatchString(strings.TrimSpace(p.Version)) || versionIsDynamic(p.Version))
		in.Packages = append(in.Packages, finish(p, w))
	}
	return true
}

func (in *Install) apt(c *Command) bool {
	in.Ecosystem = "apt"
	pos := c.Positional
	if len(pos) == 0 || pos[0].Dynamic() || (pos[0].Value != "install" && pos[0].Value != "reinstall") {
		return false
	}
	in.Verb = pos[0].Value
	for _, w := range pos[1:] {
		if entirelyDynamic(w) {
			in.Packages = append(in.Packages, dynamicPackage(w))
			continue
		}
		p := &Package{Kind: KindRegistry, Name: w.Value}
		switch {
		case isLocalPath(w.Value) || strings.HasSuffix(w.Value, ".deb"):
			p.Kind = KindPath
		default:
			if i := strings.Index(w.Value, "="); i > 0 {
				p.Name, p.Version = w.Value[:i], w.Value[i+1:]
				p.Pinned = p.Version != "" && !strings.Contains(p.Version, "*")
			}
			if i := strings.Index(p.Name, "/"); i > 0 { // pkg/release
				p.Name = p.Name[:i]
			}
		}
		in.Packages = append(in.Packages, finish(p, w))
	}
	return true
}

func (in *Install) brew(c *Command) bool {
	in.Ecosystem = "brew"
	pos := c.Positional
	if len(pos) == 0 || pos[0].Dynamic() || (pos[0].Value != "install" && pos[0].Value != "reinstall") {
		return false
	}
	in.Verb = pos[0].Value
	for _, w := range pos[1:] {
		if entirelyDynamic(w) {
			in.Packages = append(in.Packages, dynamicPackage(w))
			continue
		}
		p := &Package{Kind: KindRegistry, Name: w.Value}
		switch {
		case isLocalPath(w.Value) || strings.HasSuffix(w.Value, ".rb"):
			p.Kind = KindPath
		case strings.Contains(w.Value, "://"):
			p.Kind = KindURL
		default:
			// versioned formulae (`python@3.12`) are the closest thing to a pin Homebrew has
			if i := strings.LastIndex(w.Value, "@"); i > 0 {
				p.Name, p.Version = w.Value[:i], w.Value[i+1:]
				p.Pinned = true
			}
		}
		in.Packages = append(in.Packages, finish(p, w))
	}
	return true
}

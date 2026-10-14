package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/exampleconfig"
	"go.yaml.in/yaml/v4"
)

// exampleDay is the day the outputs of the examples of the document are made on.
var exampleDay = time.Date(2026, time.October, 13, 0, 0, 0, 0, time.UTC)

// requiresOnlineMarker is the first line of the examples of the online checks. Those checks need the
// GitHub API, so the example is run with the recorded answers in testdata/online/github.json
// instead (the same ones the tests of the checks use).
const requiresOnlineMarker = "# requires --online"

// withoutShellcheckMarker is a line of the head of the example of a check that runs only when shellcheck does not: the
// example is run with `--shellcheck=`, whether or not the machine has the command.
const withoutShellcheckMarker = "# without shellcheck"

// selectMarker and profileMarker start a line of the head of an example input which runs the example under
// other rules than the example configuration: `# select: id[,id]` runs only those rules (the option --select) and
// `# profile: pedantic` runs the profile (the option --profile).
const (
	selectMarker  = "# select:"
	profileMarker = "# profile:"
	// fileMarker names the file of the example in the project, `# file: .github/workflows/ci.yml`. The findings name it
	// and the project has it, so a rule that reads the workflows of the repository sees the example too.
	fileMarker = "# file:"
)

// example is what the tool needs to lint one example: the input, the kind of file it is, and what it runs with.
type example struct {
	src     []byte
	heading string
	// config is the YAML of an `Example config:` block, merged over the example configuration, or empty.
	config string
	// files are the `Example file:` blocks, by the path in the project.
	files map[string]string
}

// markers are the options that the head of an example input sets.
type markers struct {
	online  bool
	noShell bool
	select_ []string
	profile string
	file    string
}

// parseMarkers reads the marker lines at the head of an example input. The head ends at the first line which is
// not a marker, so a comment of the example that comes after it stays a comment.
func parseMarkers(src []byte) (markers, error) {
	var m markers
	for _, l := range strings.Split(string(src), "\n") {
		switch {
		case l == requiresOnlineMarker:
			m.online = true
		case l == withoutShellcheckMarker:
			m.noShell = true
		case strings.HasPrefix(l, selectMarker):
			for _, id := range strings.Split(strings.TrimPrefix(l, selectMarker), ",") {
				if id = strings.TrimSpace(id); id != "" {
					m.select_ = append(m.select_, id)
				}
			}
			if len(m.select_) == 0 {
				return m, fmt.Errorf("%q lists no rule ID", l)
			}
		case strings.HasPrefix(l, fileMarker):
			m.file = strings.TrimSpace(strings.TrimPrefix(l, fileMarker))
			if err := checkExampleFilePath(m.file); err != nil {
				return m, err
			}
		case strings.HasPrefix(l, profileMarker):
			m.profile = strings.TrimSpace(strings.TrimPrefix(l, profileMarker))
			if m.profile == "" {
				return m, fmt.Errorf("%q names no profile", l)
			}
		default:
			return m, nil
		}
	}
	return m, nil
}

// mergeConfig returns the example configuration with the configuration of the example merged over it: a key of
// the example replaces the key of the example configuration, and a rule of `rules` replaces the rule of the same
// ID, so the other rules keep their level.
func mergeConfig(base, over string) ([]byte, error) {
	if strings.TrimSpace(over) == "" {
		return []byte(base), nil
	}
	var b, o map[string]any
	if err := yaml.Unmarshal([]byte(base), &b); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal([]byte(over), &o); err != nil {
		return nil, fmt.Errorf("the example config is not YAML: %w", err)
	}
	for k, v := range o {
		if k == "rules" {
			br, _ := b[k].(map[string]any)
			or, ok := v.(map[string]any)
			if br != nil && ok {
				for id, rc := range or {
					br[id] = rc
				}
				continue
			}
		}
		b[k] = v
	}
	return yaml.Marshal(b)
}

// Actionlint lints the example in a project of its own, which has the files of the example and nothing else. The
// examples in the sections whose headings start with "Dependabot" are Dependabot configuration files, and the ones
// in the sections whose headings start with "Composite action" are the metadata of an action. The others are
// workflows.
func Actionlint(ex example) ([]byte, error) {
	var out bytes.Buffer

	m, err := parseMarkers(ex.src)
	if err != nil {
		return nil, err
	}

	cfgYAML, err := mergeConfig(exampleconfig.YAML, ex.config)
	if err != nil {
		return nil, err
	}
	cfg, err := jactionlint.ParseConfig(cfgYAML)
	if err != nil {
		return nil, fmt.Errorf("the example config is invalid: %w", err)
	}

	name := "test.yaml"
	if strings.HasPrefix(ex.heading, "Dependabot") {
		name = ".github/dependabot.yml"
	} else if strings.HasPrefix(ex.heading, "Composite action") {
		name = ".github/actions/example/action.yml"
	}

	if m.file != "" {
		name = m.file
	}

	opts := &jactionlint.LinterOptions{
		Config:        cfg,
		StdinFileName: name,
		Shellcheck:    "shellcheck",
		Pyflakes:      "pyflakes",
		Color:         jactionlint.ColorOptionKindNever,
		Select:        m.select_,
		Profile:       jactionlint.Profile(m.profile),
		// The outputs of the examples must not change when a date of the checks passes (the retirement of a runner
		// label changes the list of the available labels), so they are made as of this day
		Now: func() time.Time { return exampleDay },
	}

	if m.noShell {
		opts.Shellcheck = ""
	}

	// The tool runs in the root of this repository, whose testdata the online examples read
	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if m.online {
		b, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "online", "github.json"))
		if err != nil {
			return nil, err
		}
		c, err := jactionlint.NewFixtureGitHubClient(b)
		if err != nil {
			return nil, err
		}
		opts.Online = true
		opts.GitHubClient = c
		// The repository whose environments the checks look at, which testdata/online/github.json has
		os.Setenv("GITHUB_REPOSITORY", "example/environments")
	}

	// The project of the example. A message that names a file or a directory of the repository (a local action,
	// a path filter) is about these files, not about the ones of this repository.
	dir, err := os.MkdirTemp("", "check-checks-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	// The temporary directory may be a link (macOS), and the project reports the real path
	if d, err := filepath.EvalSymlinks(dir); err == nil {
		dir = d
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		return nil, err
	}
	files := map[string]string{}
	for path, content := range ex.files {
		files[path] = content
	}
	if m.file != "" {
		if _, ok := files[m.file]; ok {
			return nil, fmt.Errorf("the example is the file %q, which an `Example file:` block also gives", m.file)
		}
		files[m.file] = string(ex.src)
	}
	for _, path := range sortedKeys(files) {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(full, []byte(files[path]), 0o644); err != nil {
			return nil, err
		}
	}
	if err := os.Chdir(dir); err != nil {
		return nil, err
	}
	defer os.Chdir(repoRoot)

	p, err := jactionlint.NewProjects().At(".")
	if err != nil {
		return nil, err
	}

	l, err := jactionlint.NewLinter(&out, opts)
	if err != nil {
		return nil, err
	}
	errs, err := l.Lint(name, ex.src, p)
	if err != nil {
		return nil, err
	}
	if len(errs) == 0 {
		return nil, errors.New("the input example caused no error")
	}

	// Some error message contains absolute file paths. Replace them not to make the document depend
	// on the current file system.
	b := bytes.ReplaceAll(out.Bytes(), []byte(p.RootDir()), []byte("/path/to/repo"))

	return b, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// exampleConfigHeader and exampleFileHeader start the blocks that follow an example input and before its output: the
// configuration to lint with (merged over internal/exampleconfig) and a file of the project to lint in.
const (
	exampleConfigHeader = "Example config:"
	exampleFileHeader   = "Example file: "
)

type state int

const (
	stateInit state = iota
	stateAnchor
	stateHeading
	stateInputHeader
	stateInputBlock
	stateAfterInput
	stateExtraHeader
	stateExtraBlock
	stateOutputHeader
	stateAfterOutput
	stateOutputBlock
	stateEnd
)

func (s state) String() string {
	switch s {
	case stateAnchor:
		return "anchor"
	case stateHeading:
		return "heading"
	case stateInputHeader:
		return "input header"
	case stateInputBlock:
		return "input block"
	case stateAfterInput:
		return "after input"
	case stateExtraHeader:
		return "example config or file header"
	case stateExtraBlock:
		return "example config or file block"
	case stateOutputHeader:
		return "output header"
	case stateAfterOutput:
		return "after output"
	case stateOutputBlock:
		return "output block"
	case stateEnd:
		return "end"
	default:
		return "init"
	}
}

type Updater struct {
	prev, cur state
	lines     *bufio.Scanner
	out       bytes.Buffer
	heading   string
	input     bytes.Buffer
	// config and files are the `Example config:` and `Example file:` blocks of the example being read
	config    string
	files     map[string]string
	extra     bytes.Buffer
	extraPath string // the path of the `Example file:` block being read, empty for an `Example config:` block
	lnum      int
	ids       map[string]int
	headings  map[string]int
	firstErr  error
}

func NewUpdater(in []byte) *Updater {
	return &Updater{
		lines:    bufio.NewScanner(bytes.NewReader(in)),
		ids:      map[string]int{},
		headings: map[string]int{},
	}
}

func (u *Updater) err(err error) {
	if u.firstErr == nil && err != nil {
		u.firstErr = fmt.Errorf("error at line %d while generating section %q: %w", u.lnum, u.heading, err)
	}
}

func (u *Updater) End() ([]byte, error) {
	u.err(u.lines.Err())
	if u.cur != stateEnd {
		u.err(fmt.Errorf("unexpected state %q after generating all. this happens when a block is unclosed or some part was missing", u.cur))
	}
	log.Printf("Scanned %d lines and %d sections", u.lnum, len(u.headings))
	return u.out.Bytes(), u.firstErr
}

func (u *Updater) PlaygroundLink(src []byte) string {
	var out bytes.Buffer

	b64 := base64.NewEncoder(base64.StdEncoding, &out)
	comp, _ := zlib.NewWriterLevel(b64, zlib.BestCompression)

	scan := bufio.NewScanner(bytes.NewReader(src))
	first := true
	for scan.Scan() {
		l := scan.Bytes()
		if bytes.HasPrefix(bytes.TrimSpace(l), []byte("#")) {
			continue
		}
		if first {
			first = false
		} else {
			comp.Write([]byte{'\n'})
		}
		comp.Write(l)
	}
	u.err(scan.Err())

	u.err(comp.Close())
	u.err(b64.Close())

	return fmt.Sprintf("[Playground](https://jactionlint.jdx.dev/#%s)", out.Bytes())
}

func (u *Updater) state(s state, reason string) {
	log.Printf("%s at line %d in section %q (%q -> %q)", reason, u.lnum, u.heading, u.cur, s)
	u.prev, u.cur = u.cur, s
}

func (u *Updater) Scan() bool {
	if u.firstErr != nil || !u.lines.Scan() {
		return false
	}
	u.lnum++
	return true
}

func (u *Updater) expect(states ...state) {
	for _, s := range states {
		if s == u.cur {
			return
		}
	}
	u.err(fmt.Errorf("unexpected state %q. expected %q", u.cur, states))
}

// resetExample forgets the example that was read, when the next one starts.
func (u *Updater) resetExample() {
	u.input.Reset()
	u.config = ""
	u.files = nil
}

// checkExampleFilePath rejects a path that is not inside the project of the example.
func checkExampleFilePath(path string) error {
	if path == "" {
		return errors.New("`Example file:` needs the path of the file in the project")
	}
	if strings.HasPrefix(path, "/") || strings.Contains(path, `\`) {
		return fmt.Errorf("the path %q of `Example file:` must be relative and use slashes", path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." || part == ".git" {
			return fmt.Errorf("the path %q of `Example file:` must not have an empty, \".\", \"..\" or \".git\" part", path)
		}
	}
	return nil
}

func (u *Updater) Update() {
	l := u.lines.Text()

	isHeading := strings.HasPrefix(l, "## ")
	isInputHeader := l == "Example input:"
	isOutputHeader := l == "Output:"
	isConfigHeader := l == exampleConfigHeader
	isFileHeader := strings.HasPrefix(l, exampleFileHeader)
	isSkipOutput := l == "<!-- Skip update output -->"
	isSkipPlaygroundLink := l == "<!-- Skip playground link -->"
	isPlaygroundLink := strings.HasPrefix(l, "[Playground](") && strings.HasSuffix(l, ")")

	// Validation
	switch {
	case isHeading:
		u.expect(stateAnchor)
	case isInputHeader:
		u.expect(stateHeading, stateEnd)
	case isOutputHeader, isConfigHeader, isFileHeader:
		u.expect(stateAfterInput)
	case isSkipOutput:
		u.expect(stateOutputHeader)
	case isSkipPlaygroundLink, isPlaygroundLink:
		u.expect(stateAfterOutput)
	}

	// State transition
	//
	//    backtrack
	//    ┌───────┐
	// ┌──▼─┐  ┌──┴───┐  ┌───────┐  ┌──────┐  ┌─────┐  ┌─────┐
	// │init│─►│anchor│─►│heading│─►│input │─►│input│─►│after│────┐
	// └────┘  │      │  │       │  │header│  │block│  │input│    │
	//         └──▲┬──┘  └───────┘  └──▲───┘  └─────┘  └─────┘ ┌──▼───┐
	//     next   ││            more   │            skip       │output│
	//     section││            example│        ┌──────────────│header│
	//            ││backtrack        ┌───┐  ┌───▼──┐  ┌──────┐ └──┬───┘
	//            │└────────────────►│end│◄─│after │◄─│output│◄───┘
	// ┌────┐     └──────────────────│   │  │output│  │block │
	// │done│◄───────────────────────│   │  └──────┘  └──────┘
	// └────┘                        └───┘
	//
	switch u.cur {
	case stateInit, stateEnd:
		if u.cur == stateEnd && isInputHeader {
			u.state(stateInputHeader, "Found more example input")
			break
		}
		if strings.HasPrefix(l, `<a id="`) && strings.HasSuffix(l, `"></a>`) {
			id := strings.TrimSuffix(strings.TrimPrefix(l, `<a id="`), `"></a>`)
			if len(id) == 0 {
				u.err(errors.New("id for <a> tag is empty"))
				return
			}
			if n, ok := u.ids[id]; ok {
				u.err(fmt.Errorf("id %q was already used at line %d", id, n))
				return
			}
			u.ids[id] = u.lnum
			u.state(stateAnchor, "Found new <a> ID "+id)
		}
	case stateAnchor:
		if isHeading {
			h := strings.TrimPrefix(l, "## ")
			if n, ok := u.headings[h]; ok {
				u.err(fmt.Errorf("heading %q was already used at line %d", h, n))
				return
			}
			u.headings[h] = u.lnum
			u.heading = h
			u.state(stateHeading, "Entering new section")
		} else {
			u.state(u.prev, "Back to previous state because the next line to <a> is not a section heading")
			u.Update()
			return
		}
	case stateHeading:
		if isInputHeader {
			u.state(stateInputHeader, "Found example input header")
		}
	case stateInputHeader:
		if l == "```yaml" {
			u.state(stateInputBlock, "Start code block for input example")
		}
	case stateInputBlock:
		if l == "```" {
			if u.input.Len() == 0 {
				u.err(errors.New("empty example input is not allowed"))
				return
			}
			u.state(stateAfterInput, "End code block for input example")
		} else {
			u.input.WriteString(l)
			u.input.WriteByte('\n')
		}
	case stateAfterInput:
		switch {
		case isOutputHeader:
			u.state(stateOutputHeader, "Found example output header")
		case isConfigHeader:
			if u.config != "" {
				u.err(errors.New("an example has one `Example config:` block"))
				return
			}
			u.extraPath = ""
			u.state(stateExtraHeader, "Found example config header")
		case isFileHeader:
			path := strings.TrimSpace(strings.TrimPrefix(l, exampleFileHeader))
			if err := checkExampleFilePath(path); err != nil {
				u.err(err)
				return
			}
			if _, ok := u.files[path]; ok {
				u.err(fmt.Errorf("the file %q of the example is given twice", path))
				return
			}
			u.extraPath = path
			u.state(stateExtraHeader, "Found example file header")
		}
	case stateExtraHeader:
		// The fence of a config is yaml. A file may be of any kind, so its fence may name any language
		if (u.extraPath == "" && l == "```yaml") || (u.extraPath != "" && strings.HasPrefix(l, "```") && l != "```") {
			u.extra.Reset()
			u.state(stateExtraBlock, "Start code block for the example config or file")
		}
	case stateExtraBlock:
		if l != "```" {
			u.extra.WriteString(l)
			u.extra.WriteByte('\n')
			break
		}
		if u.extraPath == "" && strings.TrimSpace(u.extra.String()) == "" {
			u.err(errors.New("an empty `Example config:` block is not allowed"))
			return
		}
		if u.extraPath == "" {
			u.config = u.extra.String()
		} else {
			if u.files == nil {
				u.files = map[string]string{}
			}
			u.files[u.extraPath] = u.extra.String()
		}
		u.state(stateAfterInput, "End code block for the example config or file")
	case stateOutputHeader:
		if isSkipOutput {
			u.state(stateAfterOutput, "Skip updating output due to the comment")
		} else if l == "```" {
			u.state(stateOutputBlock, "Start code block for output")
		}
	case stateOutputBlock:
		if l != "```" {
			return // Output block will be generated by jactionlint
		}
		out, err := Actionlint(example{src: u.input.Bytes(), heading: u.heading, config: u.config, files: u.files})
		u.err(err)
		u.out.Write(out)
		u.state(stateAfterOutput, "End code block for output and generate jactionlint output")
	case stateAfterOutput:
		if isSkipPlaygroundLink {
			u.resetExample()
			u.state(stateEnd, "Skip updating playground link due to the comment")
		} else if isPlaygroundLink {
			ln := u.PlaygroundLink(u.input.Bytes())
			u.out.WriteString(ln)
			u.out.WriteByte('\n')
			u.resetExample()
			u.state(stateEnd, "Generate playground link "+ln)
			return
		}
	}

	u.out.WriteString(l)
	u.out.WriteByte('\n')
}

func Update(in []byte) ([]byte, error) {
	u := NewUpdater(in)
	for u.Scan() {
		u.Update()
	}
	return u.End()
}

var stderr io.Writer = os.Stderr

func Main(args []string) error {
	var fix bool
	var quiet bool
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.BoolVar(&fix, "fix", false, "fix the outdated document automatically")
	flags.BoolVar(&quiet, "quiet", false, "disable trace logs")
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: check-checks [FLAGS] FILE\n\nFlags:")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("this command should take exact one file path but got %v", flags.Args())
	}
	path := flags.Arg(0)

	if quiet {
		log.SetOutput(io.Discard)
	}

	in, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not read the document file: %w", err)
	}
	log.Printf("Read %d bytes from %q", len(in), path)

	out, err := Update(in)
	if err != nil {
		return err
	}

	if bytes.Equal(in, out) {
		log.Printf("Do nothing because there is no update in %q", path)
		return nil
	}

	if !fix {
		return fmt.Errorf("checks document has some update. run `go run ./scripts/check-checks -fix %s` and commit the changes on Linux or macOS. the diff:\n\n%s", path, cmp.Diff(in, out))
	}

	log.Printf("Overwrite the file with the updated content (%d bytes) at %q", len(out), path)
	return os.WriteFile(path, out, 0666)
}

func main() {
	if err := Main(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

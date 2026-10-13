package jactionlint

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"go.yaml.in/yaml/v4"
)

// This file reads GitHub's Actions lock file, `.github/workflows/actions.lock` (a technical preview).
// The format is written by `gh actions-lock` and defined by github/actions-lockfile, which is pre-1.0
// and may change without notice:
//
//	version: 'v0.0.3'
//	workflows:                               # the workflows which are locked, with the uses: they write
//	    '.github/workflows/ci.yml':
//	        - 'actions/checkout@v6.0.2'
//	dependencies:                            # the graph of every locked action, by "owner/repo@ref"
//	    'actions/checkout@v6.0.2':
//	        ref: 'v6.0.2'
//	        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'
//	        owner_id: 44036562
//	        repo_id: 197814629
//	        uses: ['owner/repo@ref']             # only composite actions: their own direct uses
//
// What is read here is what was verified in the sources of the format and of the tools which read it
// (see docs/checks.md, "GitHub Actions lock file"). Keys which are not known are ignored, and the
// versions v0.0.2 and v0.0.3, whose shape is the one above, are the only ones trusted: a lock file of
// another version is treated like a missing one, so a change of the format never makes a finding
// disappear. v0.0.1 (which has tag and branch instead of ref and keys with a ":sha1-..." suffix) is
// not read.

// actionsLockPath is where the lock file is, relative to the root of the repository.
const actionsLockPath = ".github/workflows/actions.lock"

// actionsLockMaxSize is the largest lock file which is read. The tool which writes it refuses a
// larger one too.
const actionsLockMaxSize = 1 << 20

// actionsLockVersions are the versions of the format whose shape is the one this file reads.
var actionsLockVersions = map[string]bool{"v0.0.2": true, "v0.0.3": true}

var (
	reLockCommit = regexp.MustCompile(`^(?:sha1-[0-9A-Fa-f]{40}|sha256-[0-9A-Fa-f]{64})$`)
	// The grammar of a pin as the format defines it: owner/repo@ref, with no slash, "@" or ":" in the
	// owner and the repo, and no ":" in the ref.
	reLockPin = regexp.MustCompile(`^([^/@:\s]+)/([^/@:\s]+)@([^:\s]+)$`)
)

// lockPin is the "owner/repo@ref" which names a dependency in the lock file.
type lockPin struct {
	Raw   string
	Owner string // as written
	Repo  string // as written
	Ref   string
	// Key is the canonical form: the owner and the repo in lower case, the ref as written, because
	// git refs are case sensitive.
	Key       string
	Line, Col int
}

func parseLockPin(raw string, line, col int) (lockPin, bool) {
	m := reLockPin.FindStringSubmatch(raw)
	if m == nil {
		return lockPin{Raw: raw, Line: line, Col: col}, false
	}
	return lockPin{
		Raw: raw, Owner: m[1], Repo: m[2], Ref: m[3], Line: line, Col: col,
		Key: strings.ToLower(m[1]+"/"+m[2]) + "@" + m[3],
	}, true
}

// lockWorkflow is an item of "workflows": a locked workflow and the actions its file uses.
type lockWorkflow struct {
	Key       string // the path as written in the file, relative to the root of the repository
	Pins      []lockPin
	Line, Col int
}

// lockDependency is an item of "dependencies".
type lockDependency struct {
	Pin       lockPin
	Ref       string // "ref:", the ref the commit was resolved from
	Commit    string // "commit:", "sha1-<hex>" or "sha256-<hex>"
	OwnerID   int64  // 0 when missing or not a positive integer
	RepoID    int64
	Hostname  string
	Uses      []lockPin
	HasRef    bool
	HasCommit bool
	// The positions of the keys, which are 0 when the key is missing.
	RefLine, RefCol       int
	CommitLine, CommitCol int
	OwnerIDLine           int
	RepoIDLine            int
	OwnerIDCol, RepoIDCol int
	OwnerIDBad, RepoIDBad bool
	Line, Col             int
}

// lockProblem is something in the lock file which does not follow the format.
type lockProblem struct {
	Line, Col int
	Msg       string
	// Fatal means that nothing in the file can be trusted: it is not YAML or has not the shape of the format.
	Fatal bool
}

// actionsLock is the content of an actions.lock file.
type actionsLock struct {
	// Path is the absolute path of the file.
	Path    string
	Version string
	// VersionLine and VersionCol are the position of the value of "version:", 0 when it is missing.
	VersionLine, VersionCol int
	// Workflows are the locked workflows in the order of the file.
	Workflows []*lockWorkflow
	// Dependencies are the locked actions in the order of the file.
	Dependencies []*lockDependency
	Problems     []lockProblem

	workflows map[string]*lockWorkflow   // by Key
	deps      map[string]*lockDependency // by canonical key
	fatal     bool
}

// usable reports whether the lock file may be trusted to decide something about a workflow: it is
// YAML of the shape of the format and its version is one whose shape is known.
func (l *actionsLock) usable() bool {
	return l != nil && !l.fatal && actionsLockVersions[l.Version]
}

// listsWorkflow reports whether the workflow (a path relative to the root of the repository, with
// slashes) is locked: only the workflows which are listed are held to the lock file.
func (l *actionsLock) listsWorkflow(key string) bool {
	if !l.usable() {
		return false
	}
	_, ok := l.workflows[key]
	return ok
}

// pinsAction reports whether the lock file pins the repository action u to a commit for the workflow:
// the workflow lists "owner/repo@ref" (a sub action such as actions/cache/save is pinned by its
// repository), and the dependency of that key has the same ref and a full commit digest. The owner and
// the repo are compared case insensitively, the ref is not. Reusable workflows and Docker images are
// not part of the lock file.
func (l *actionsLock) pinsAction(workflowKey string, u *UsesRef) bool {
	if !l.usable() || u == nil || u.Kind != UsesAction || u.Dynamic {
		return false
	}
	w := l.workflows[workflowKey]
	if w == nil {
		return false
	}
	key := strings.ToLower(u.Owner+"/"+u.Repo) + "@" + u.Ref
	for _, p := range w.Pins {
		if p.Key != key {
			continue
		}
		d := l.deps[key]
		return d != nil && d.HasRef && d.Ref == p.Ref && reLockCommit.MatchString(d.Commit)
	}
	return false
}

// readActionsLock reads the lock file at path. It returns nil when there is no such file. A file
// which cannot be read or parsed gives a lock with a fatal problem, so that the caller can report it
// and trust nothing from it.
func readActionsLock(path string) *actionsLock {
	l := &actionsLock{Path: path, workflows: map[string]*lockWorkflow{}, deps: map[string]*lockDependency{}}
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		l.fail(0, 0, "the lock file cannot be read: "+err.Error())
		return l
	}
	if fi.IsDir() {
		return nil
	}
	if fi.Size() > actionsLockMaxSize {
		l.fail(1, 1, "the lock file is larger than 1 MiB, which GitHub's tools do not read")
		return l
	}
	b, err := os.ReadFile(path)
	if err != nil {
		l.fail(0, 0, "the lock file cannot be read: "+err.Error())
		return l
	}
	l.parse(b)
	return l
}

// parseActionsLock parses the content of a lock file, which was not read from the disk (the linter was given it).
func parseActionsLock(path string, b []byte) *actionsLock {
	l := &actionsLock{Path: path, workflows: map[string]*lockWorkflow{}, deps: map[string]*lockDependency{}}
	if len(b) > actionsLockMaxSize {
		l.fail(1, 1, "the lock file is larger than 1 MiB, which GitHub's tools do not read")
		return l
	}
	l.parse(b)
	return l
}

// lockState says what the lock file has for an action which a workflow uses.
type lockState int

const (
	// lockMissing: the lock file lists no entry of the repository for the workflow.
	lockMissing lockState = iota
	// lockOtherRef: it lists the repository, but at another ref.
	lockOtherRef
	// lockListed: it lists the pin (or, for a commit SHA, an entry which was resolved to that commit).
	lockListed
)

// lookup tells what the entry of the workflow has for the action u, the way gh actions-lock decides it: the pin
// owner/repo@ref is listed, or, when the ref is a full commit SHA, an entry of the repository whose commit is that
// SHA is (the workflow was pinned by hand and the entry has the tag it was found by). It returns the dependency of the
// pin which was found, or the refs of the repository which are listed instead.
func (l *actionsLock) lookup(workflowKey string, u *UsesRef) (lockState, *lockDependency, []string) {
	w := l.workflows[workflowKey]
	if w == nil {
		return lockMissing, nil, nil
	}
	nwo := strings.ToLower(u.Owner + "/" + u.Repo)
	want := nwo + "@" + u.Ref
	sha := ""
	if u.RefKind == RefFullSHA {
		sha = strings.ToLower(u.Ref)
	}
	var others []string
	for _, p := range w.Pins {
		if p.Key == want {
			return lockListed, l.deps[p.Key], nil
		}
		if strings.ToLower(p.Owner+"/"+p.Repo) != nwo {
			continue
		}
		if d := l.deps[p.Key]; d != nil && sha != "" && strings.EqualFold(lockCommitHex(d.Commit), sha) {
			return lockListed, d, nil
		}
		others = append(others, p.Ref)
	}
	if len(others) > 0 {
		return lockOtherRef, nil, others
	}
	return lockMissing, nil, nil
}

// lockCommitHex returns the hex digits of a commit digest such as "sha1-ab12...", or "" when it is not one.
func lockCommitHex(commit string) string {
	_, hex, ok := strings.Cut(commit, "-")
	if !ok || !reLockCommit.MatchString(commit) {
		return ""
	}
	return hex
}

func (l *actionsLock) fail(line, col int, msg string) {
	l.fatal = true
	l.Problems = append(l.Problems, lockProblem{Line: line, Col: col, Msg: msg, Fatal: true})
}

func (l *actionsLock) problem(n *yaml.Node, msg string) {
	p := lockProblem{Msg: msg}
	if n != nil {
		p.Line, p.Col = n.Line, n.Column
	}
	l.Problems = append(l.Problems, p)
}

func (l *actionsLock) parse(b []byte) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		l.fail(1, 1, "the lock file is not valid YAML: "+firstLine(err.Error()))
		return
	}
	root := &doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		l.fail(1, 1, "the lock file must be a mapping with the keys \"version\", \"workflows\" and \"dependencies\"")
		return
	}

	var versionNode, workflowsNode, depsNode *yaml.Node
	seenTop := map[string]bool{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		if seenTop[k.Value] {
			l.fail(k.Line, k.Column, "the key "+strconv.Quote(k.Value)+" is repeated, which the lock file does not allow: the file is not read")
			return
		}
		seenTop[k.Value] = true
		switch k.Value { // Other keys are ignored: the format may add some
		case "version":
			versionNode = v
		case "workflows":
			workflowsNode = v
		case "dependencies":
			depsNode = v
		}
	}

	switch {
	case versionNode == nil:
		l.fail(root.Line, root.Column, "the lock file has no \"version\"")
		return
	case versionNode.Kind != yaml.ScalarNode || versionNode.Value == "":
		l.fail(versionNode.Line, versionNode.Column, "\"version\" must be a string such as \"v0.0.3\"")
		return
	}
	l.Version = versionNode.Value
	l.VersionLine, l.VersionCol = versionNode.Line, versionNode.Column
	if !actionsLockVersions[l.Version] {
		// The shape of another version is not known, so nothing is read from it.
		return
	}

	if workflowsNode != nil && !l.parseWorkflows(workflowsNode) {
		return
	}
	if depsNode != nil && !l.parseDependencies(depsNode) {
		return
	}
}

// parseWorkflows reads the "workflows" mapping. It returns false when the node is not a mapping,
// which makes the whole file unusable.
func (l *actionsLock) parseWorkflows(n *yaml.Node) bool {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		return true // "workflows:" with nothing: no workflow is locked
	}
	if n.Kind != yaml.MappingNode {
		l.fail(n.Line, n.Column, "\"workflows\" must be a mapping from the path of a workflow to the list of the actions it uses")
		return false
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		w := &lockWorkflow{Key: k.Value, Line: k.Line, Col: k.Column}
		if k.Kind != yaml.ScalarNode || k.Value == "" {
			l.problem(k, "the path of a workflow must be a non-empty string")
			continue
		}
		if _, dup := l.workflows[w.Key]; dup {
			// The official parser rejects a repeated key: no entry is the effective one
			l.fail(k.Line, k.Column, "the workflow "+strconv.Quote(w.Key)+" is listed twice, which the lock file does not allow: the file is not read")
			return false
		}
		switch {
		case v.Kind == yaml.ScalarNode && v.Tag == "!!null":
			// A workflow without dependencies is recorded with an empty list
		case v.Kind != yaml.SequenceNode:
			l.problem(v, "the actions of the workflow "+strconv.Quote(w.Key)+" must be a list of \"owner/repo@ref\"")
		default:
			for _, item := range v.Content {
				if item.Kind != yaml.ScalarNode {
					l.problem(item, "an action of the workflow "+strconv.Quote(w.Key)+" must be a string like \"owner/repo@ref\"")
					continue
				}
				p, ok := parseLockPin(item.Value, item.Line, item.Column)
				if !ok {
					l.problem(item, strconv.Quote(item.Value)+" is not a valid dependency, which must look like \"owner/repo@ref\"")
					continue
				}
				w.Pins = append(w.Pins, p)
			}
		}
		l.Workflows = append(l.Workflows, w)
		l.workflows[w.Key] = w
	}
	return true
}

// parseDependencies reads the "dependencies" mapping.
func (l *actionsLock) parseDependencies(n *yaml.Node) bool {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		return true
	}
	if n.Kind != yaml.MappingNode {
		l.fail(n.Line, n.Column, "\"dependencies\" must be a mapping from \"owner/repo@ref\" to what was resolved")
		return false
	}
	seenDeps := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		pin, ok := parseLockPin(k.Value, k.Line, k.Column)
		if k.Kind != yaml.ScalarNode || !ok {
			l.problem(k, strconv.Quote(k.Value)+" is not a valid dependency, which must look like \"owner/repo@ref\"")
			continue
		}
		if seenDeps[pin.Raw] {
			// The official parser rejects a repeated key: no entry is the effective one
			l.fail(k.Line, k.Column, "the dependency "+strconv.Quote(pin.Raw)+" is listed twice, which the lock file does not allow: the file is not read")
			return false
		}
		seenDeps[pin.Raw] = true
		d := &lockDependency{Pin: pin, Line: k.Line, Col: k.Column}
		if v.Kind != yaml.MappingNode {
			l.problem(v, "the dependency "+strconv.Quote(pin.Raw)+" must be a mapping with \"ref\", \"commit\", \"owner_id\" and \"repo_id\"")
			l.addDependency(d)
			continue
		}
		seenField := map[string]bool{}
		for j := 0; j+1 < len(v.Content); j += 2 {
			fk, fv := v.Content[j], v.Content[j+1]
			if seenField[fk.Value] {
				l.fail(fk.Line, fk.Column, "the key "+strconv.Quote(fk.Value)+" of the dependency "+strconv.Quote(pin.Raw)+" is repeated, which the lock file does not allow: the file is not read")
				return false
			}
			seenField[fk.Value] = true
			switch fk.Value { // Other keys are ignored
			case "ref":
				d.Ref, d.HasRef = scalarOf(fv)
				d.RefLine, d.RefCol = fv.Line, fv.Column
			case "commit":
				d.Commit, d.HasCommit = scalarOf(fv)
				d.CommitLine, d.CommitCol = fv.Line, fv.Column
			case "hostname":
				d.Hostname, _ = scalarOf(fv)
			case "owner_id":
				d.OwnerID, d.OwnerIDBad = positiveInt(fv)
				d.OwnerIDLine, d.OwnerIDCol = fv.Line, fv.Column
			case "repo_id":
				d.RepoID, d.RepoIDBad = positiveInt(fv)
				d.RepoIDLine, d.RepoIDCol = fv.Line, fv.Column
			case "uses":
				if fv.Kind != yaml.SequenceNode {
					l.problem(fv, "\"uses\" of the dependency "+strconv.Quote(pin.Raw)+" must be a list of \"owner/repo@ref\"")
					continue
				}
				for _, item := range fv.Content {
					p, ok := parseLockPin(item.Value, item.Line, item.Column)
					if item.Kind != yaml.ScalarNode || !ok {
						l.problem(item, strconv.Quote(item.Value)+" is not a valid dependency, which must look like \"owner/repo@ref\"")
						continue
					}
					d.Uses = append(d.Uses, p)
				}
			}
		}
		if !l.addDependency(d) {
			return false
		}
	}
	return true
}

// addDependency records the dependency. The key is canonical (the owner and the repository in lower case), so two
// spellings of one key are one entry: the official parser accepts them when they say the same and rejects the
// file when they do not. It returns false when the file is rejected.
func (l *actionsLock) addDependency(d *lockDependency) bool {
	if prev, dup := l.deps[d.Pin.Key]; dup {
		if !sameLockDependency(prev, d) {
			l.fail(d.Line, d.Col, "the dependency "+strconv.Quote(d.Pin.Raw)+" is listed twice with other contents, which the lock file does not allow: the file is not read")
			return false
		}
		return true
	}
	l.Dependencies = append(l.Dependencies, d)
	l.deps[d.Pin.Key] = d
	return true
}

func sameLockDependency(a, b *lockDependency) bool {
	if a.Ref != b.Ref || a.Commit != b.Commit || a.OwnerID != b.OwnerID || a.RepoID != b.RepoID || a.Hostname != b.Hostname || len(a.Uses) != len(b.Uses) {
		return false
	}
	for i := range a.Uses {
		if a.Uses[i].Key != b.Uses[i].Key {
			return false
		}
	}
	return true
}

// scalarOf returns the string of a scalar node and whether it is a non-empty scalar.
func scalarOf(n *yaml.Node) (string, bool) {
	if n.Kind != yaml.ScalarNode || n.Tag == "!!null" || n.Value == "" {
		return "", false
	}
	return n.Value, true
}

// positiveInt returns the value of a positive integer node. bad is true when the node is something
// else.
func positiveInt(n *yaml.Node) (v int64, bad bool) {
	if n.Kind != yaml.ScalarNode {
		return 0, true
	}
	v, err := strconv.ParseInt(n.Value, 10, 64)
	if err != nil || v <= 0 {
		return 0, true
	}
	return v, false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// actionsLockOf returns the lock file of the project, which the linter reads the first time a file needs it and keeps
// for the rest of its run, or nil when the repository has none. It lives and dies with the Linter, like the call graph.
func (l *Linter) actionsLockOf(p *Project) *actionsLock {
	if p == nil {
		return nil
	}
	v, _ := l.locks.LoadOrStore(p.root, &actionsLockSlot{})
	slot := v.(*actionsLockSlot)
	slot.once.Do(func() {
		slot.lock = readActionsLock(filepath.Join(p.root, filepath.FromSlash(actionsLockPath)))
	})
	return slot.lock
}

type actionsLockSlot struct {
	once sync.Once
	lock *actionsLock
}

// workflowKeyOf returns the path of the file relative to the root of the project, with slashes, which is
// how the lock file names a workflow. It returns "" when the file is outside the project.
func (p *Project) workflowKeyOf(file string) string {
	if p == nil {
		return ""
	}
	root := absPath(p.root)
	if !isPathInDir(root, file) {
		return ""
	}
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// lockScope returns the usable lock file of the repository and the workflows it has to say something about for the file
// being linted: the workflow itself, or for the metadata of a local action the workflows which run it. It returns nil
// when there is none: the repository has no lock file or it cannot be trusted (see actionsLock.usable), or no
// workflow runs the action. The workflows are not necessarily listed in the lock file.
func (e *RuleEnv) lockScope() *lockScope {
	if e == nil || e.project == nil {
		return nil
	}
	lock := e.actionsLock
	if !lock.usable() {
		return nil
	}
	var keys []string
	if e.action {
		keys = e.lockWorkflows
	} else {
		p := e.path
		if !filepath.IsAbs(p) {
			p = filepath.Join(e.cwd, p)
		}
		if k := e.project.workflowKeyOf(absPath(p)); k != "" {
			keys = []string{k}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return &lockScope{lock: lock, keys: keys}
}

// lockScope is the lock file and the workflows which it is read for.
type lockScope struct {
	lock *actionsLock
	keys []string
}

// workflowLock returns the lock file as it holds the file being linted to itself, or nil when there is nothing to
// apply: see lockScope, and every workflow must be listed in the lock file.
//
// The file is a workflow, or the metadata of a local action. The lock file records the actions which an
// action uses under the workflows which run it with `uses: $/path` (or `./path`), so the steps of an action are
// pinned when every workflow which runs it, directly or through other local actions, lists the pin. An action
// which no workflow of the repository runs is not locked.
func (e *RuleEnv) workflowLock() *workflowLock {
	sc := e.lockScope()
	if sc == nil {
		return nil
	}
	for _, k := range sc.keys {
		if !sc.lock.listsWorkflow(k) {
			return nil
		}
	}
	return &workflowLock{lock: sc.lock, keys: sc.keys}
}

// workflowLock is the lock file as it applies to a file: the workflows which it lists and which run it.
type workflowLock struct {
	lock *actionsLock
	keys []string
}

// pins reports whether the lock file pins the action to a commit for every workflow of the lock.
func (w *workflowLock) pins(u *UsesRef) bool {
	if w == nil {
		return false
	}
	for _, k := range w.keys {
		if !w.lock.pinsAction(k, u) {
			return false
		}
	}
	return true
}

// lockWorkflowsOf returns the workflows (paths relative to the root of the repository) which run the local action
// in the file, for an action file of a project whose lock file is usable.
func (l *Linter) lockWorkflowsOf(p *Project, file string) []string {
	if p == nil || !l.actionsLockOf(p).usable() {
		return nil
	}
	rel := p.workflowKeyOf(l.absFilePath(file))
	if rel == "" {
		return nil
	}
	return l.callGraphOf(p).workflowsRunning(path.Dir(rel))
}

// workflowsRunning returns the workflows whose own steps run the action in the directory (relative to the root
// with "/"), directly or through other local actions, sorted. A reusable workflow in between is not followed:
// it is a workflow of the lock file of its own.
func (g *callGraph) workflowsRunning(dir string) []string {
	dir = path.Clean(dir)
	start := graphNode{'a', dir}
	seen := map[graphNode]bool{start: true}
	queue := []graphNode{start}
	var out []string
	g.mu.Lock()
	defer g.mu.Unlock()
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, from := range g.callers[n] {
			if seen[from] {
				continue
			}
			seen[from] = true
			if from.kind == 'w' {
				out = append(out, from.path)
			} else {
				queue = append(queue, from)
			}
		}
	}
	slices.Sort(out)
	return out
}

// lockListsWorkflow reports whether the Actions lock file of the project holds the file to itself: a workflow
// which it lists, or an action which a workflow that it lists runs.
func (l *Linter) lockListsWorkflow(project *Project, file string) bool {
	if project == nil || !l.actionsLockOf(project).usable() {
		return false
	}
	if l.isActionFile(file) {
		for _, k := range l.lockWorkflowsOf(project, file) {
			if l.actionsLockOf(project).listsWorkflow(k) {
				return true
			}
		}
		return false
	}
	return l.actionsLockOf(project).listsWorkflow(project.workflowKeyOf(l.absFilePath(file)))
}

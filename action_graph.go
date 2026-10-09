package jactionlint

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

// This file finds which local workflows run a composite action. The rules which depend on the
// context of a run (the event which triggered the workflow) cannot tell it from the action.yml, so they
// ask the repository: every `uses: ./path` of a workflow step (or of another local action, or of a
// reusable workflow called by a workflow) is an edge of a graph, and the callers of an action are the
// workflows which reach it.

// ActionCaller is one local workflow which (directly or through other local actions and reusable
// workflows) runs an action.
type ActionCaller struct {
	// Workflow is the path of the workflow file, relative to the root of the repository with "/" as the
	// separator.
	Workflow string
	// Via lists the local actions and reusable workflows between the workflow and the action, outermost
	// first, as paths relative to the root of the repository. It is empty when the workflow runs the
	// action in one of its own steps.
	Via []string
	// Events are the events which trigger the workflow, without workflow_call. They are empty when the
	// workflow is only called by other workflows, which is not known.
	Events []Event
	// Calls are the steps of the workflow that run the action, when the workflow runs it in one of its own steps
	// (Via is empty). They tell what the workflow passes to the action. They are nil for a call through
	// another action or reusable workflow.
	Calls []*ExecAction
}

// ActionCallers are the local workflows which run an action. It lets the rules which depend on the
// context of the run (the trigger) judge the steps of a composite action.
//
// The context is the most dangerous one among the callers: an action called by a "pull_request" workflow
// and by a "pull_request_target" workflow is checked as if it ran in the second one. When no local
// workflow calls the action (it is published for other repositories, or only used by workflows
// which are not in the repository), Callers is empty and the rules assume nothing about the trigger:
// they judge the action by its own steps.
type ActionCallers struct {
	// Dir is the directory of the action relative to the root of the repository with "/" as the
	// separator. It is "." for an action.yml in the root.
	Dir string
	// Callers are sorted by the path of the workflow.
	Callers []*ActionCaller
}

// Known reports whether a local workflow runs the action.
func (c *ActionCallers) Known() bool {
	return c != nil && len(c.Callers) > 0
}

// Events returns the events of all callers.
func (c *ActionCallers) Events() []Event {
	if c == nil {
		return nil
	}
	var ret []Event
	for _, cl := range c.Callers {
		ret = append(ret, cl.Events...)
	}
	return ret
}

// Dangerous returns the caller whose trigger is the most dangerous, and the name of the trigger. It
// returns nil when no caller has an event.
func (c *ActionCallers) Dangerous() (*ActionCaller, string) {
	if c == nil {
		return nil, ""
	}
	var best *ActionCaller
	bestName := ""
	bestRank := -1
	for _, cl := range c.Callers {
		for _, e := range cl.Events {
			if r := triggerDanger(e.EventName()); r > bestRank {
				best, bestName, bestRank = cl, e.EventName(), r
			}
		}
	}
	return best, bestName
}

// RunsOn reports whether any caller runs on the event.
func (c *ActionCallers) RunsOn(event string) (*ActionCaller, bool) {
	if c == nil {
		return nil, false
	}
	for _, cl := range c.Callers {
		for _, e := range cl.Events {
			if e.EventName() == event {
				return cl, true
			}
		}
	}
	return nil, false
}

// Describe tells in a message where the context comes from, for example
// "called from .github/workflows/release.yml, which runs on pull_request_target". When no workflow calls
// the action it says so. It is "" when the action is not a composite action.
func (c *ActionCallers) Describe() string {
	if !c.Known() {
		return "no local workflow calls this action, so only its own steps are considered"
	}
	cl, trigger := c.Dangerous()
	if cl == nil {
		cl = c.Callers[0]
		return "called from " + cl.describe()
	}
	return fmt.Sprintf("called from %s, which runs on %q", c.who(cl), trigger)
}

// who names the caller and counts the other callers.
func (c *ActionCallers) who(cl *ActionCaller) string {
	if n := len(c.Callers) - 1; n > 0 {
		return fmt.Sprintf("%s and %d other workflow(s)", cl.describe(), n)
	}
	return cl.describe()
}

func (cl *ActionCaller) describe() string {
	if len(cl.Via) == 0 {
		return cl.Workflow
	}
	return cl.Workflow + " (through " + strings.Join(cl.Via, " and ") + ")"
}

// triggerDanger ranks the events by how much an attacker controls of a run and by the privileges of the
// run. A higher number is more dangerous.
func triggerDanger(event string) int {
	switch event {
	case "pull_request_target":
		return 6
	case "workflow_run", "issue_comment":
		return 5
	case "issues", "pull_request_review_comment", "pull_request_review", "discussion", "discussion_comment":
		return 4
	case "pull_request":
		return 3
	case "push", "fork", "watch", "release", "create":
		return 2
	case "workflow_dispatch", "repository_dispatch", "schedule":
		return 1
	}
	return 0
}

// TriggerEvents returns the events which decide the context a workflow-level rule judges. They are the events of
// the workflow, or, for the metadata of an action, those of the workflows which call it (see
// ActionCallers).
func (w *Workflow) TriggerEvents() []Event {
	if w.Action != nil {
		return w.Action.Callers.Events()
	}
	return w.On
}

// graphNode identifies a workflow or an action in the call graph.
type graphNode struct {
	// kind is 'w' for a workflow file and 'a' for the directory of an action.
	kind byte
	path string
}

// callGraph is the call graph of the workflows and actions of one repository. It is built on first
// use and then read-only, except for the memoized callers.
type callGraph struct {
	root string

	// callees maps a node to the nodes it calls. An action which does not exist on disk has no entry.
	callees map[graphNode][]graphNode
	// workflows are the parsed workflows by path.
	workflows map[string]*Workflow
	// actions are the parsed actions by directory, nil for a directory which has none.
	actions map[string]*Workflow
	// actionFiles are the files the actions were read from, relative to the root with "/".
	actionFiles map[string]string
	// callers is the reverse of callees.
	callers map[graphNode][]graphNode
	// calls are the steps of a node that run the local action of another node.
	calls map[[2]graphNode][]*ExecAction
	// notInheriting holds the edges from a workflow to a reusable workflow that has a job calling it without
	// "secrets: inherit".
	notInheriting map[[2]graphNode]bool

	mu   sync.Mutex
	memo map[string]*ActionCallers
}

// localTarget turns the value of a `uses:` into the node it calls, if it is a local action or
// workflow of the repository.
func localTarget(value string) (graphNode, bool) {
	spec, ok := canonLocalUsesSpec(value)
	if !ok || strings.Contains(spec, "${{") {
		return graphNode{}, false
	}
	p := path.Clean(strings.TrimPrefix(spec, "./"))
	if p == ".." || strings.HasPrefix(p, "../") {
		return graphNode{}, false
	}
	if strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml") {
		return graphNode{'w', p}, true
	}
	return graphNode{'a', p}, true
}

func newCallGraph(root string) *callGraph {
	g := &callGraph{
		root:        root,
		callees:     map[graphNode][]graphNode{},
		workflows:   map[string]*Workflow{},
		actions:     map[string]*Workflow{},
		actionFiles: map[string]string{},
		callers:     map[graphNode][]graphNode{},
		calls:       map[[2]graphNode][]*ExecAction{},

		notInheriting: map[[2]graphNode]bool{},
		memo:          map[string]*ActionCallers{},
	}

	wfDir := filepath.Join(root, ".github", "workflows")
	files, _ := projectWorkflowFiles(wfDir) // a missing directory has no workflows
	var queue []graphNode
	for _, f := range files {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		w, _ := Parse(src)
		if w == nil {
			continue
		}
		g.workflows[rel] = w
		n := graphNode{'w', rel}
		for _, j := range w.Jobs {
			if j == nil {
				continue
			}
			if j.WorkflowCall != nil && j.WorkflowCall.Uses != nil {
				if t, ok := localTarget(j.WorkflowCall.Uses.Value); ok {
					g.addEdge(n, t, &queue)
					if !j.WorkflowCall.InheritSecrets {
						g.notInheriting[[2]graphNode{n, t}] = true
					}
				}
			}
			g.addStepEdges(n, j.Steps, &queue)
		}
	}

	// Actions in the usual places have no caller necessarily, but they are nodes of the graph.
	for _, d := range standardActionDirs(root) {
		g.addNode(graphNode{'a', d}, &queue)
	}

	// Load the actions the workflows call, and the ones those call, until nothing is new. This also
	// terminates on a cycle because every directory is loaded once.
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n.kind == 'a' {
			g.loadAction(n, &queue)
		}
	}

	for from, tos := range g.callees {
		for _, to := range tos {
			g.callers[to] = append(g.callers[to], from)
		}
	}
	for _, froms := range g.callers {
		sort.Slice(froms, func(i, j int) bool { return froms[i].path < froms[j].path })
	}
	return g
}

func (g *callGraph) addNode(n graphNode, queue *[]graphNode) {
	if n.kind != 'a' {
		return
	}
	if _, ok := g.actions[n.path]; ok {
		return
	}
	g.actions[n.path] = nil // mark as seen
	*queue = append(*queue, n)
}

func (g *callGraph) addEdge(from, to graphNode, queue *[]graphNode) {
	if from == to || slices.Contains(g.callees[from], to) {
		return
	}
	g.callees[from] = append(g.callees[from], to)
	g.addNode(to, queue)
}

func (g *callGraph) addStepEdges(from graphNode, steps []*Step, queue *[]graphNode) {
	walkSteps(steps, func(s *Step) {
		a, ok := s.Exec.(*ExecAction)
		if !ok || a.Uses == nil {
			return
		}
		if t, ok := localTarget(a.Uses.Value); ok {
			g.addEdge(from, t, queue)
			g.calls[[2]graphNode{from, t}] = append(g.calls[[2]graphNode{from, t}], a)
		}
	})
}

// loadAction reads and parses the metadata of the action in the directory, if there is any.
func (g *callGraph) loadAction(n graphNode, queue *[]graphNode) {
	for _, name := range actionFileNames {
		rel := path.Join(n.path, name)
		src, err := os.ReadFile(filepath.Join(g.root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		// A file that does not parse is still linted, which reports why, and has no steps to follow
		g.actionFiles[n.path] = rel
		w, _ := ParseAction(src)
		if w == nil {
			return
		}
		g.actions[n.path] = w
		for _, j := range w.Jobs {
			g.addStepEdges(n, j.Steps, queue)
		}
		return
	}
}

// standardActionDirs lists the directories of the actions in the places GitHub repositories keep them:
// the root and any directory under .github/actions.
func standardActionDirs(root string) []string {
	var dirs []string
	for _, name := range actionFileNames {
		if s, err := os.Stat(filepath.Join(root, name)); err == nil && !s.IsDir() {
			dirs = append(dirs, ".")
			break
		}
	}
	base := filepath.Join(root, ".github", "actions")
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if n := d.Name(); n == "action.yml" || n == "action.yaml" {
			if rel, err := filepath.Rel(root, filepath.Dir(p)); err == nil {
				dirs = append(dirs, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	return dirs
}

// actionPaths returns the metadata files of all actions of the repository: the ones in the usual places
// and the ones which a local `uses:` refers to. The paths are absolute and sorted.
func (g *callGraph) actionPaths() []string {
	var ret []string
	for _, rel := range g.actionFiles {
		ret = append(ret, filepath.Join(g.root, filepath.FromSlash(rel)))
	}
	sort.Strings(ret)
	return ret
}

// callersOf returns the workflows which run the action in the directory (relative to the root with "/").
func (g *callGraph) callersOf(dir string) *ActionCallers {
	dir = path.Clean(dir)
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.memo[dir]; ok {
		return c
	}

	// Breadth first search upwards. visited makes a cycle (an action which calls itself, directly or
	// not) harmless, and the first path found to a workflow is a shortest one.
	type item struct {
		node graphNode
		via  []string // nodes between the item and the action, outermost first
	}
	start := graphNode{'a', dir}
	visited := map[graphNode]bool{start: true}
	queue := []item{{start, nil}}
	found := map[string]*ActionCaller{}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		via := it.via
		if it.node != start {
			via = append([]string{it.node.path}, it.via...)
		}
		for _, from := range g.callers[it.node] {
			if visited[from] {
				continue
			}
			visited[from] = true
			if from.kind != 'w' {
				queue = append(queue, item{from, via})
				continue
			}
			w := g.workflows[from.path]
			called := len(g.callers[from]) > 0
			var events []Event
			for _, e := range w.On {
				if _, ok := e.(*WorkflowCallEvent); !ok {
					events = append(events, e)
				}
			}
			if len(events) > 0 || !called {
				found[from.path] = &ActionCaller{Workflow: from.path, Via: via, Events: events}
				if it.node == start {
					found[from.path].Calls = g.calls[[2]graphNode{from, start}]
				}
			}
			// A reusable workflow runs in the context of the workflow calling it.
			if called {
				queue = append(queue, item{from, via})
			}
		}
	}

	ret := &ActionCallers{Dir: dir}
	for _, c := range found {
		ret.Callers = append(ret.Callers, c)
	}
	sort.Slice(ret.Callers, func(i, j int) bool { return ret.Callers[i].Workflow < ret.Callers[j].Workflow })
	g.memo[dir] = ret
	return ret
}

// callerWarning tells that the metadata of an action is called from a workflow with a trigger that
// an outsider controls (see triggerDanger), for the findings that get worse because of it. It is ""
// for a workflow, and for an action which no local workflow calls or which only runs on the triggers
// of the repository's own people.
func (w *Workflow) callerWarning() string {
	if w == nil || w.Action == nil {
		return ""
	}
	cl, trigger := w.Action.Callers.Dangerous()
	if cl == nil || triggerDanger(trigger) < 4 {
		return ""
	}
	return fmt.Sprintf("this action is called from %s, which runs on %q", w.Action.Callers.who(cl), trigger)
}

// allCallersInheritSecrets reports whether the workflow at the path (relative to the root with "/") is called
// by at least one local workflow and every job that calls it passes "secrets: inherit".
func (g *callGraph) allCallersInheritSecrets(rel string) bool {
	to := graphNode{'w', rel}
	froms := g.callers[to]
	for _, from := range froms {
		if g.notInheriting[[2]graphNode{from, to}] {
			return false
		}
	}
	return len(froms) > 0
}

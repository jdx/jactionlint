package jactionlint

import (
	"regexp"
	"strings"

	"go.yaml.in/yaml/v4"
)

// gitCredentialUseRegex matches a command of a `run:` script that talks to a git remote or creates
// history, which is what persisted checkout credentials are for.
var gitCredentialUseRegex = regexp.MustCompile(`(?m)\bgit\b[^\n]*\b(push|pull|fetch|clone|remote|submodule|lfs|ls-remote|commit|tag)\b`)

// gitPushingActions are actions which push to the repository and rely on the credentials that
// actions/checkout leaves in the git config unless they are given a token of their own.
var gitPushingActions = []string{
	"stefanzweifel/git-auto-commit-action",
	"endbug/add-and-commit",
	"peter-evans/create-pull-request",
	"ad-m/github-push-action",
	"jamesives/github-pages-deploy-action",
	"peaceiris/actions-gh-pages",
	"crazy-max/ghaction-github-pages",
	"googleapis/release-please-action",
	"marcoieni/release-plz-action",
	"actions-js/push",
	"github-actions-x/commit",
	"cpina/github-action-push-to-another-repository",
}

// RuleArtipacked is a rule checker which reports actions/checkout steps that leave the GITHUB_TOKEN
// in the git config of the workspace. Anything that later copies the workspace, such as an artifact
// upload, then publishes the credential (the "ArtiPACKED" attack), and even without that the
// credential is on disk for every later step to read. Setting `persist-credentials: false` removes
// it.
//
// An explicit `persist-credentials: true` is a decision that the credential is needed, so it is not
// reported.
//
// The fix adds `persist-credentials: false`. It is safe only when no later step of the job runs git commands that
// talk to a remote or uses a known pushing action; otherwise the job may be relying on the
// credential and the fix is unsafe. Scripts which push without showing a git command in the workflow
// cannot be seen, so run the workflow after applying the fix.
type RuleArtipacked struct {
	RuleBase
	src []byte
	idx *sourceIndex
	doc *yaml.Node
	bad bool
	// steps maps the position of the `uses` value of each step mapping of the document to the step.
	steps map[[2]int]stepMapping
	// pending are the fixes which are not verified yet. VisitWorkflowPost checks them together so that a
	// file with many checkouts is parsed once more, not once more for each.
	pending []pendingPersistFix
	wf      *Workflow
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleArtipacked) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	return nil
}

// stepMapping is a mapping node of the document with the path of child indexes from the document to it.
type stepMapping struct {
	path []int
	node *yaml.Node
}

type pendingPersistFix struct {
	err  *Error
	edit TextEdit
	path []int
}

// NewRuleArtipacked creates a new RuleArtipacked instance. The source of the file is needed to attach
// fixes. It can be nil, in which case findings carry no fix.
func NewRuleArtipacked(src []byte) *RuleArtipacked {
	return &RuleArtipacked{
		RuleBase: RuleBase{
			name: "artipacked",
			desc: "Checks that actions/checkout does not persist the GITHUB_TOKEN credential in the git config",
		},
		src: src,
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleArtipacked) VisitJobPre(n *Job) error {
	if !rule.Config().RuleEnabled("artipacked") {
		return nil
	}
	steps := flattenSteps(n.Steps)
	var suffix *stepSuffixFlags
	uploads := false
	for i, s := range steps {
		a, ref := stepAction(s)
		if a == nil || !ref.isRepoAction("actions/checkout") {
			continue
		}
		if _, set := a.input("persist-credentials"); set {
			// false is the fix, true is a decision that the credential is needed, and an expression
			// cannot be judged
			continue
		}
		if checkoutV1Re.MatchString(ref.Ref) || rule.pinnedToV1(a, ref) {
			// actions/checkout@v1 has no such input and always leaves the credential. The outdated runner
			// check reports the version, and nothing here could be done (zizmor#1098)
			continue
		}
		if suffix == nil {
			// Whether some later step pushes or relies on the credential, for each position: a job with
			// many checkouts must not scan its steps for each of them.
			suffix = suffixFlags(steps)
			uploads = uploadsWorkspace(steps)
		}
		if suffix.pushes[i+1] && !uploads {
			// A later step pushes, and it does so with the credential the checkout left. That is what the
			// credential is for and nothing uploads the workspace, so there is nothing to leak (zizmor#1043)
			continue
		}
		rule.ReportID("artipacked", a.Uses.Pos, "actions/checkout leaves the GITHUB_TOKEN in the git config of the workspace, where a later step such as an artifact upload can publish it. set \"persist-credentials: false\" under \"with:\" unless a later step needs to push")
		rule.attachFix(a, suffix.needs[i+1])
	}
	return nil
}

// pinnedToV1 reports whether the checkout is pinned to a commit and the version comment after it names the
// first major version: `actions/checkout@<sha> # v1`.
func (rule *RuleArtipacked) pinnedToV1(a *ExecAction, ref *UsesRef) bool {
	if ref.RefKind != RefFullSHA || rule.wf == nil || rule.wf.Comments == nil || a.Uses.Pos == nil {
		return false
	}
	c := rule.wf.Comments.Inline(a.Uses.Pos.Line)
	if c == nil {
		return false
	}
	v, ok := commentVersion(c.Text)
	return ok && checkoutV1Re.MatchString(v)
}

// checkoutV1Re matches the refs of the first major version of actions/checkout.
var checkoutV1Re = regexp.MustCompile(`^v?1(\.[0-9]+)*$`)

// gitPushRe matches a git command that pushes.
var gitPushRe = regexp.MustCompile(`(?m)\bgit\b[^\n]*\bpush\b`)

// pushesWithCredentials reports whether one of the steps pushes with the credential of the checkout: a `git push` in
// a script or an action that pushes. Another remote operation (fetch, clone, tag) or a push with a credential of its
// own is not told from a push here, so only the push counts.
func pushesWithCredentials(steps []*Step) bool {
	for _, s := range steps {
		if isStaticallyFalse(s.If) {
			continue // a step that never runs pushes and relies on nothing
		}
		switch e := s.Exec.(type) {
		case *ExecRun:
			if e.Run != nil && gitPushRe.MatchString(e.Run.Value) {
				return true
			}
		case *ExecAction:
			if e.Uses == nil {
				continue
			}
			name := strings.ToLower(ParseUses(e.Uses.Value).CanonicalName())
			for _, p := range gitPushingActions {
				if name == p || strings.HasPrefix(name, p+"/") {
					return true
				}
			}
		}
	}
	return false
}

// uploadsWorkspace reports whether a step uploads an artifact of the whole workspace (or its parent), which
// publishes the git config of the checkout with the credential in it.
func uploadsWorkspace(steps []*Step) bool {
	for _, s := range steps {
		a, ref := stepAction(s)
		if a == nil || !ref.isRepoAction("actions/upload-artifact") {
			continue
		}
		path, ok := a.input("path")
		if !ok {
			continue
		}
		for _, p := range strings.FieldsFunc(path, func(r rune) bool { return r == '\n' || r == ' ' }) {
			if p == "." || p == "./" || p == ".." || p == "../" || strings.Contains(p, "github.workspace") {
				return true
			}
		}
	}
	return false
}

// artipackedNoFix is the reason for a checkout whose YAML the fix does not edit.
var artipackedNoFix = &NoFix{Code: NoFixUnsupportedShape, Reason: "the step is not written in a way the fix can edit exactly, add persist-credentials: false under with: by hand"}

func (rule *RuleArtipacked) attachFix(a *ExecAction, unsafe bool) {
	if rule.src == nil {
		return
	}
	e := rule.errs[len(rule.errs)-1]
	if rule.idx == nil {
		rule.idx = newSourceIndex(rule.src)
	}
	if rule.doc == nil && !rule.bad {
		var doc yaml.Node
		if err := yaml.Unmarshal(rule.src, &doc); err != nil {
			rule.bad = true
		} else {
			rule.doc = &doc
		}
	}
	if rule.bad {
		e.NoFix = artipackedNoFix
		return
	}
	if rule.steps == nil {
		rule.steps = map[[2]int]stepMapping{}
		indexStepMappings(rule.doc, nil, rule.steps)
	}
	edit, path, ok := persistCredentialsEdit(rule.steps, rule.src, rule.idx, a)
	if !ok {
		e.NoFix = artipackedNoFix
		return
	}
	e.Fix = &Fix{
		Description: "Set persist-credentials: false",
		Unsafe:      unsafe,
		Edits:       []TextEdit{edit},
	}
	rule.pending = append(rule.pending, pendingPersistFix{err: e, edit: edit, path: path})
}

// VisitWorkflowPost is callback when visiting Workflow node after visiting its children. It verifies
// the fixes: the file must still be YAML with persist-credentials: false in the same step.
func (rule *RuleArtipacked) VisitWorkflowPost(n *Workflow) error {
	pending := rule.pending
	rule.pending = nil
	if len(pending) == 0 {
		return nil
	}
	var check func(set []pendingPersistFix)
	check = func(set []pendingPersistFix) {
		if persistFixesHold(rule.src, set) {
			return
		}
		if len(set) == 1 {
			set[0].err.Fix = nil
			set[0].err.NoFix = artipackedNoFix
			return
		}
		check(set[:len(set)/2])
		check(set[len(set)/2:])
	}
	check(pending)
	return nil
}

// persistFixesHold applies the edits of the fixes together and reports whether the result is YAML in
// which every step has persist-credentials: false.
func persistFixesHold(src []byte, fixes []pendingPersistFix) bool {
	edits := make([]TextEdit, len(fixes))
	for i, f := range fixes {
		edits[i] = f.edit
	}
	out := applyEdits(src, edits)
	var ndoc yaml.Node
	if err := yaml.Unmarshal(out, &ndoc); err != nil {
		return false
	}
	for _, f := range fixes {
		nstep := followPath(&ndoc, f.path)
		if nstep == nil {
			return false
		}
		_, nwith := mappingEntry(nstep, "with")
		if nwith == nil || nwith.Kind != yaml.MappingNode {
			return false
		}
		_, nv := mappingEntry(nwith, "persist-credentials")
		if nv == nil || nv.Kind != yaml.ScalarNode || nv.Value != "false" {
			return false
		}
	}
	return true
}

// stepSuffixFlags tells for the position i whether some step from i on pushes with the credential of
// the checkout (pushes) or may rely on it (needs). The slices have one more element than there are steps.
type stepSuffixFlags struct {
	pushes, needs []bool
}

func suffixFlags(steps []*Step) *stepSuffixFlags {
	f := &stepSuffixFlags{pushes: make([]bool, len(steps)+1), needs: make([]bool, len(steps)+1)}
	for i := len(steps) - 1; i >= 0; i-- {
		one := steps[i : i+1]
		f.pushes[i] = f.pushes[i+1] || pushesWithCredentials(one)
		f.needs[i] = f.needs[i+1] || needsPersistedCredentials(one)
	}
	return f
}

// needsPersistedCredentials reports whether the steps may rely on the credential that actions/checkout
// persisted.
func needsPersistedCredentials(steps []*Step) bool {
	for _, s := range steps {
		if isStaticallyFalse(s.If) {
			continue // a step that never runs pushes and relies on nothing
		}
		switch e := s.Exec.(type) {
		case *ExecRun:
			if e.Run != nil && gitCredentialUseRegex.MatchString(e.Run.Value) {
				return true
			}
		case *ExecAction:
			if e.Uses == nil {
				continue
			}
			name := strings.ToLower(ParseUses(e.Uses.Value).CanonicalName())
			for _, p := range gitPushingActions {
				if name == p || strings.HasPrefix(name, p+"/") {
					return true
				}
			}
		}
	}
	return false
}

// persistCredentialsEdit computes the edit which makes the checkout step persist-credentials: false,
// with the path to the step in the document. It returns false when the YAML is written in a way that
// cannot be edited with certainty. The caller verifies the edited file (persistFixesHold).
func persistCredentialsEdit(steps map[[2]int]stepMapping, src []byte, idx *sourceIndex, a *ExecAction) (TextEdit, []int, bool) {
	if a.Uses == nil || a.Uses.Pos == nil || !idx.valid {
		return TextEdit{}, nil, false
	}
	found, ok := steps[[2]int{a.Uses.Pos.Line, a.Uses.Pos.Col}]
	step, path := found.node, found.path
	if !ok || step == nil {
		return TextEdit{}, nil, false
	}
	usesKey, usesVal := mappingEntry(step, "uses")
	if usesKey == nil {
		return TextEdit{}, nil, false
	}
	if step.Style&yaml.FlowStyle != 0 {
		edit, ok := persistCredentialsFlowStepEdit(src, idx, step)
		return edit, path, ok
	}
	nl := idx.newline()
	withKey, withVal := mappingEntry(step, "with")

	var edit TextEdit
	switch {
	case withKey == nil:
		if usesVal.Line != usesKey.Line || strings.Contains(usesVal.Value, "\n") {
			return TextEdit{}, nil, false
		}
		at := idx.lineEnd(usesVal.Line)
		pad := strings.Repeat(" ", usesKey.Column-1)
		edit = TextEdit{Start: at, End: at, NewText: nl + pad + "with:" + nl + pad + "  persist-credentials: false"}
	case withVal.Kind == yaml.ScalarNode && withVal.Tag == "!!null" && withVal.Value == "":
		if withVal.Line != withKey.Line {
			return TextEdit{}, nil, false
		}
		at := idx.lineEnd(withKey.Line)
		pad := strings.Repeat(" ", withKey.Column-1+2)
		edit = TextEdit{Start: at, End: at, NewText: nl + pad + "persist-credentials: false"}
	case withVal.Kind == yaml.MappingNode && withVal.Style&yaml.FlowStyle == 0 && len(withVal.Content) > 0:
		first := withVal.Content[0]
		if first.Line == withKey.Line {
			return TextEdit{}, nil, false
		}
		if k, _ := mappingEntry(withVal, "persist-credentials"); k != nil {
			return TextEdit{}, nil, false
		}
		// Insert before the first entry but after the `with:` line, so that comments above the first
		// entry stay with it.
		if withKey.Line+1 > len(idx.lineStarts) {
			return TextEdit{}, nil, false
		}
		at := idx.lineStarts[withKey.Line] // the start of the line after `with:`
		pad := strings.Repeat(" ", first.Column-1)
		edit = TextEdit{Start: at, End: at, NewText: pad + "persist-credentials: false" + nl}
	case withVal.Kind == yaml.MappingNode && withVal.Style&yaml.FlowStyle != 0:
		// with: { fetch-depth: 0 }, also over several lines. The entry goes first, in front of the first key
		// (so that comments and line breaks between the entries stay where they are)
		var ok bool
		if edit, ok = persistCredentialsFlowEdit(src, idx, withVal, "persist-credentials: false"); !ok {
			return TextEdit{}, nil, false
		}
	default:
		return TextEdit{}, nil, false
	}

	return edit, path, true
}

// persistCredentialsFlowEdit computes the edit which puts the entry first in the flow mapping, in front of its first key,
// or right after the braces of an empty one.
func persistCredentialsFlowEdit(src []byte, idx *sourceIndex, m *yaml.Node, entry string) (TextEdit, bool) {
	open, ok := idx.offset(m.Line, m.Column)
	if !ok || open >= len(src) || src[open] != '{' || m.Anchor != "" {
		return TextEdit{}, false
	}
	if len(m.Content) == 0 {
		return TextEdit{Start: open + 1, End: open + 1, NewText: " " + entry + " "}, true
	}
	first := m.Content[0]
	at, ok := idx.offset(first.Line, first.Column)
	if !ok || at <= open || first.Anchor != "" || first.Kind != yaml.ScalarNode || first.Tag != "!!str" {
		return TextEdit{}, false
	}
	return TextEdit{Start: at, End: at, NewText: entry + ", "}, true
}

// persistCredentialsFlowStepEdit is the edit for a step written as a flow mapping, `- {uses: actions/checkout@v4}`. A
// flow mapping is the whole step then, and a `with:` of it is a flow mapping too.
func persistCredentialsFlowStepEdit(src []byte, idx *sourceIndex, step *yaml.Node) (TextEdit, bool) {
	withKey, withVal := mappingEntry(step, "with")
	switch {
	case withKey == nil:
		return persistCredentialsFlowEdit(src, idx, step, "with: {persist-credentials: false}")
	case withVal.Kind == yaml.MappingNode && withVal.Style&yaml.FlowStyle != 0:
		return persistCredentialsFlowEdit(src, idx, withVal, "persist-credentials: false")
	}
	return TextEdit{}, false
}

// mappingEntry returns the key and the value node of the key in a mapping node, or nils.
func mappingEntry(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// indexStepMappings records the mapping nodes which have a `uses` key by the position of its value, with
// the path of child indexes from the document to the node.
func indexStepMappings(n *yaml.Node, path []int, into map[[2]int]stepMapping) {
	if n.Kind == yaml.MappingNode {
		if k, v := mappingEntry(n, "uses"); k != nil {
			key := [2]int{v.Line, v.Column}
			if _, dup := into[key]; !dup {
				into[key] = stepMapping{path: path, node: n}
			}
		}
	}
	for i, c := range n.Content {
		indexStepMappings(c, append(path[:len(path):len(path)], i), into)
	}
}

func followPath(n *yaml.Node, path []int) *yaml.Node {
	for _, i := range path {
		if i < 0 || i >= len(n.Content) {
			return nil
		}
		n = n.Content[i]
	}
	return n
}

func init() {
	registerRules(
		RuleInfo{ID: "artipacked", Group: RuleGroupSecurity, Summary: "actions/checkout persists the GITHUB_TOKEN credential in the git config.", DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-artipacked"},
	)
	registerRuleFactory("artipacked", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleArtipacked(env.Source())}
	})
}

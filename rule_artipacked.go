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
		rule.ReportID("artipacked", a.Uses.Pos, "actions/checkout leaves the GITHUB_TOKEN in the git config of the workspace, where a later step such as an artifact upload can publish it. set \"persist-credentials: false\" under \"with:\" unless a later step needs to push")
		rule.attachFix(a, steps[i+1:])
	}
	return nil
}

func (rule *RuleArtipacked) attachFix(a *ExecAction, later []*Step) {
	if rule.src == nil {
		return
	}
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
		return
	}
	edit, ok := persistCredentialsEdit(rule.doc, rule.src, rule.idx, a)
	if !ok {
		return
	}
	unsafe := needsPersistedCredentials(later)
	rule.errs[len(rule.errs)-1].Fix = &Fix{
		Description: "Set persist-credentials: false",
		Unsafe:      unsafe,
		Edits:       []TextEdit{edit},
	}
}

// needsPersistedCredentials reports whether the steps may rely on the credential that actions/checkout
// persisted.
func needsPersistedCredentials(steps []*Step) bool {
	for _, s := range steps {
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

// persistCredentialsEdit computes the edit which makes the checkout step persist-credentials: false. It
// returns false when the YAML is written in a way that cannot be edited with certainty, and verifies
// the result by parsing the edited file.
func persistCredentialsEdit(doc *yaml.Node, src []byte, idx *sourceIndex, a *ExecAction) (TextEdit, bool) {
	if a.Uses == nil || a.Uses.Pos == nil || !idx.valid {
		return TextEdit{}, false
	}
	path, step := findStepMapping(doc, nil, a.Uses.Pos.Line, a.Uses.Pos.Col)
	if step == nil || step.Style&yaml.FlowStyle != 0 {
		return TextEdit{}, false
	}
	usesKey, usesVal := mappingEntry(step, "uses")
	if usesKey == nil {
		return TextEdit{}, false
	}
	nl := idx.newline()
	withKey, withVal := mappingEntry(step, "with")

	var edit TextEdit
	switch {
	case withKey == nil:
		if usesVal.Line != usesKey.Line || strings.Contains(usesVal.Value, "\n") {
			return TextEdit{}, false
		}
		at := idx.lineEnd(usesVal.Line)
		pad := strings.Repeat(" ", usesKey.Column-1)
		edit = TextEdit{Start: at, End: at, NewText: nl + pad + "with:" + nl + pad + "  persist-credentials: false"}
	case withVal.Kind == yaml.ScalarNode && withVal.Tag == "!!null":
		if withVal.Line != withKey.Line {
			return TextEdit{}, false
		}
		at := idx.lineEnd(withKey.Line)
		pad := strings.Repeat(" ", withKey.Column-1+2)
		edit = TextEdit{Start: at, End: at, NewText: nl + pad + "persist-credentials: false"}
	case withVal.Kind == yaml.MappingNode && withVal.Style&yaml.FlowStyle == 0 && len(withVal.Content) > 0:
		first := withVal.Content[0]
		if first.Line == withKey.Line {
			return TextEdit{}, false
		}
		if k, _ := mappingEntry(withVal, "persist-credentials"); k != nil {
			return TextEdit{}, false
		}
		// Insert before the first entry but after the `with:` line, so that comments above the first
		// entry stay with it.
		if withKey.Line+1 > len(idx.lineStarts) {
			return TextEdit{}, false
		}
		at := idx.lineStarts[withKey.Line] // the start of the line after `with:`
		pad := strings.Repeat(" ", first.Column-1)
		edit = TextEdit{Start: at, End: at, NewText: pad + "persist-credentials: false" + nl}
	default:
		return TextEdit{}, false
	}

	// Check the result: it must still be YAML with persist-credentials: false in the same step.
	out := append(append(append([]byte{}, src[:edit.Start]...), edit.NewText...), src[edit.End:]...)
	var ndoc yaml.Node
	if err := yaml.Unmarshal(out, &ndoc); err != nil {
		return TextEdit{}, false
	}
	nstep := followPath(&ndoc, path)
	if nstep == nil {
		return TextEdit{}, false
	}
	_, nwith := mappingEntry(nstep, "with")
	if nwith == nil || nwith.Kind != yaml.MappingNode {
		return TextEdit{}, false
	}
	_, nv := mappingEntry(nwith, "persist-credentials")
	if nv == nil || nv.Kind != yaml.ScalarNode || nv.Value != "false" {
		return TextEdit{}, false
	}
	return edit, true
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

// findStepMapping looks for the mapping node which has a `uses` key whose value starts at the position.
// It returns the node with the path of child indexes from the document to it.
func findStepMapping(n *yaml.Node, path []int, line, col int) ([]int, *yaml.Node) {
	if n.Kind == yaml.MappingNode {
		if k, v := mappingEntry(n, "uses"); k != nil && v.Line == line && v.Column == col {
			return path, n
		}
	}
	for i, c := range n.Content {
		if p, found := findStepMapping(c, append(path[:len(path):len(path)], i), line, col); found != nil {
			return p, found
		}
	}
	return nil, nil
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

package jactionlint

import (
	"fmt"
	"sort"
	"strings"
)

// RuleForbiddenUses is a rule to restrict the actions and reusable workflows a repository may use. With
// an "allow" list only the listed ones are accepted. With a "deny" list the listed ones are rejected.
// The rule does nothing without one of them.
//
// The lists "images-allow" and "images-deny" do the same for container images: the image of `container:` and
// `services:`, of `uses: docker://` and of `docker run`, `docker create` and `docker pull` in a script.
type RuleForbiddenUses struct {
	RuleBase
	runContext
	allow, deny             []UsesPattern
	imagesAllow, imagesDeny []string
	// urlUses reads `uses: https://host/owner/repo@ref` as owner/repo@ref (the platform is Forgejo or Gitea)
	urlUses bool
}

// NewRuleForbiddenUses creates a new RuleForbiddenUses instance from the patterns of the allow and
// deny lists. It returns nil when both lists are empty.
func NewRuleForbiddenUses(allow, deny []string) *RuleForbiddenUses {
	return newRuleForbiddenUses(allow, deny, nil, nil)
}

// newRuleForbiddenUses is NewRuleForbiddenUses with the lists of images. It returns nil when all four are empty.
func newRuleForbiddenUses(allow, deny, imagesAllow, imagesDeny []string) *RuleForbiddenUses {
	if len(allow) == 0 && len(deny) == 0 && len(imagesAllow) == 0 && len(imagesDeny) == 0 {
		return nil
	}
	r := &RuleForbiddenUses{
		RuleBase: RuleBase{
			name: "forbidden-uses",
			desc: "Checks `uses:` against the allow and deny lists of the configuration",
		},
		imagesAllow: imagesAllow,
		imagesDeny:  imagesDeny,
	}
	for _, s := range allow {
		r.allow = append(r.allow, ParseUsesPattern(s))
	}
	for _, s := range deny {
		r.deny = append(r.deny, ParseUsesPattern(s))
	}
	return r
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleForbiddenUses) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	if n.Action != nil && n.Action.Runs != nil && n.Action.Runs.Image != nil {
		if ref, ok := strings.CutPrefix(n.Action.Runs.Image.Value, "docker://"); ok {
			rule.checkImage(ref, n.Action.Runs.Image.Pos, "image")
		}
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleForbiddenUses) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	if n.WorkflowCall != nil {
		rule.check(n.WorkflowCall.Uses)
	}
	walkSteps(n.Steps, func(s *Step) {
		if a, ok := s.Exec.(*ExecAction); ok && a != nil {
			rule.check(a.Uses)
		}
	})
	if rule.imagesActive() {
		rule.checkContainer(n.Container, "container image")
		if n.Services != nil && n.Services.Expression == nil {
			ids := make([]string, 0, len(n.Services.Value))
			for id := range n.Services.Value {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				if sv := n.Services.Value[id]; sv != nil {
					rule.checkContainer(sv.Container, fmt.Sprintf("image of service %q", id))
				}
			}
		}
	}
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleForbiddenUses) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node. It checks the images of `docker run` and the like in a script.
func (rule *RuleForbiddenUses) VisitStep(n *Step) error {
	if !rule.imagesActive() {
		return nil
	}
	run, ok := n.Exec.(*ExecRun)
	if !ok {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	for _, c := range s.Commands {
		if img, ok := dockerImagesOf(c); ok {
			rule.checkImage(img.Ref, scriptPos(s, origin, img.Word.Offset), fmt.Sprintf("image of %q", img.Command))
		}
	}
	return nil
}

func (rule *RuleForbiddenUses) imagesActive() bool {
	return len(rule.imagesAllow) > 0 || len(rule.imagesDeny) > 0
}

func (rule *RuleForbiddenUses) checkContainer(c *Container, what string) {
	if c == nil || c.Image == nil || c.Image.Value == "" || c.Image.ContainsExpression() {
		return
	}
	rule.checkImage(c.Image.Value, c.Image.Pos, what)
}

// checkImage reports an image that the lists "images-deny" and "images-allow" do not accept.
func (rule *RuleForbiddenUses) checkImage(ref string, pos *Pos, what string) {
	if !rule.imagesActive() || pos == nil || ref == "" || strings.Contains(ref, "${{") {
		return
	}
	if p, ok := imagePatternMatches(rule.imagesDeny, ref); ok {
		rule.ReportIDf("forbidden-uses", pos, "%s %q is forbidden by pattern %q in the \"images-deny\" list of the \"forbidden-uses\" rule. remove it or change the configuration", what, ref, p)
		return
	}
	if len(rule.imagesAllow) == 0 {
		return
	}
	if _, ok := imagePatternMatches(rule.imagesAllow, ref); !ok {
		rule.ReportIDf("forbidden-uses", pos, "%s %q is not allowed because it matches no pattern in the \"images-allow\" list of the \"forbidden-uses\" rule (%s). use an allowed one or add a pattern to the configuration", what, ref, quoteJoin(rule.imagesAllow))
	}
}

func (rule *RuleForbiddenUses) check(uses *String) {
	if uses == nil || uses.Pos == nil {
		return
	}
	value := uses.Value
	if rule.urlUses {
		value = repoOfURLUses(value)
	}
	u := ParseUses(value)
	if u.Kind == UsesDocker && !u.Dynamic {
		rule.checkImage(strings.TrimPrefix(uses.Value, "docker://"), uses.Pos, "image")
		return
	}
	if !u.IsRepo() || u.Dynamic {
		return
	}
	for _, p := range rule.deny {
		if p.Match(u) {
			rule.ReportIDf("forbidden-uses", uses.Pos, "%s %q is forbidden by pattern %q in the \"deny\" list of the \"forbidden-uses\" rule. remove it or change the configuration", usesNoun(u), uses.Value, p)
			return
		}
	}
	if len(rule.allow) == 0 {
		return
	}
	for _, p := range rule.allow {
		if p.Match(u) {
			return
		}
	}
	rule.ReportIDf("forbidden-uses", uses.Pos, "%s %q is not allowed because it matches no pattern in the \"allow\" list of the \"forbidden-uses\" rule (%s). use an allowed one or add a pattern to the configuration", usesNoun(u), uses.Value, quoteJoin(patternStrings(rule.allow)))
}

// usesNoun names what a reference points to in a message.
func usesNoun(u *UsesRef) string {
	if u.Kind == UsesReusableWorkflow {
		return "reusable workflow"
	}
	return "action"
}

func patternStrings(ps []UsesPattern) []string {
	ss := make([]string, len(ps))
	for i, p := range ps {
		ss[i] = p.String()
	}
	return ss
}

func init() {
	registerRules(
		RuleInfo{
			ID: "forbidden-uses", Group: RuleGroupPolicy, Summary: "An action or reusable workflow is not allowed or is denied by the configuration.", DefaultLevel: SeverityError,
			DocsAnchor: "check-forbidden-uses",
			Options: []RuleOption{
				{Name: "allow", Kind: RuleOptionStrings, Summary: "Patterns of the only actions and reusable workflows which may be used, e.g. \"actions/*\". The rule does nothing without allow or deny."},
				{Name: "deny", Kind: RuleOptionStrings, Summary: "Patterns of actions and reusable workflows which must not be used."},
				{Name: "images-allow", Kind: RuleOptionStrings, Validate: validateImagePatterns, Summary: "Glob patterns of the only container images which may be used (container:, services:, docker:// and docker run in a script), e.g. \"ghcr.io/my-org/*\" or \"node:20\". Docker Hub names can leave out the registry (\"node\" is \"docker.io/library/node\")."},
				{Name: "images-deny", Kind: RuleOptionStrings, Validate: validateImagePatterns, Summary: "Glob patterns of container images which must not be used."},
			},
		},
	)
	registerRuleFactory("forbidden-uses", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("forbidden-uses") {
			return nil
		}
		r := newRuleForbiddenUses(env.config.ruleOptionStrings("forbidden-uses", "allow"), env.config.ruleOptionStrings("forbidden-uses", "deny"), env.config.ruleOptionStrings("forbidden-uses", "images-allow"), env.config.ruleOptionStrings("forbidden-uses", "images-deny"))
		if r == nil {
			return nil
		}
		r.urlUses = env.config.allowsURLUses()
		return []Rule{r}
	})
}

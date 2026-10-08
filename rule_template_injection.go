package jactionlint

// RuleTemplateInjection finds ${{ }} expansions in scripts which the check of the expression rule
// does not cover: objects which hold attacker controlled properties, environment variables which
// are set from attacker controlled properties, and every other expansion. Expansions of the
// properties which are known to be attacker controlled are reported by RuleExpression because it
// already checks every expression.
type RuleTemplateInjection struct {
	RuleBase
	src *sourceIndex
	wf  *Workflow
	job *Job
}

// NewRuleTemplateInjection creates a new RuleTemplateInjection instance. The source of the file lets
// the rule attach fixes to its findings. It can be nil.
func NewRuleTemplateInjection(src []byte) *RuleTemplateInjection {
	r := &RuleTemplateInjection{
		RuleBase: RuleBase{
			name: "expression", // the findings are of the same kind as the ones of RuleExpression
			desc: "Checks for ${{ }} expansions in scripts which let attacker controlled text become code",
		},
	}
	if src != nil {
		r.src = newSourceIndex(src)
	}
	return r
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleTemplateInjection) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleTemplateInjection) VisitJobPre(n *Job) error {
	rule.job = n
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleTemplateInjection) VisitJobPost(n *Job) error {
	rule.job = nil
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleTemplateInjection) VisitStep(n *Step) error {
	cfg := rule.Config()
	if !cfg.RuleEnabled("template-injection") && !cfg.RuleEnabled("template-injection-expansion") && !cfg.RuleEnabled("template-injection-trusted") {
		return nil
	}
	ctx := tiContext{wf: rule.wf, job: rule.job, step: n}
	for _, code := range codeStringsOf(n) {
		spans := rule.src.scanExprs(code.Str)
		var plan map[int]*Fix
		planned := false
		for i := range spans {
			sp := &spans[i]
			cl := ctx.classify(sp)
			// The expression rule reports a direct property (cl.Ref is nil then). One which it did not report,
			// like github.event.issue['Title'], which matchUntrusted reads case-insensitively, is ours.
			if (cl.Tier == tiDirect && cl.Ref == nil) || cl.Tier == tiNone || !tiEnabled(cfg, cl.Tier) {
				continue
			}
			if !planned {
				plan = planTemplateInjectionFixes(tiFixInput{idx: rule.src, ctx: ctx, cfg: cfg, str: code.Str, run: code.Run})
				planned = true
			}
			pos := sp.TokPos(cl.Ref.Node.Token())
			fix := plan[sp.Start]
			switch cl.Tier {
			case tiDirect:
				rule.ReportIDf("template-injection", pos, "%q is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details", cl.Ref.Display(sp.Src))
			case tiSubtree:
				rule.ReportIDf("template-injection", pos, "%q includes potentially untrusted properties such as %q. avoid expanding it in inline scripts. instead, pass the properties you need through environment variables", cl.Ref.Display(sp.Src), cl.Source)
			case tiEnv:
				rule.ReportIDf("template-injection", pos, "environment variable %q holds the potentially untrusted input %q. expanding it with ${{ }} in an inline script is as dangerous as using the input directly. instead, read it as a variable of the shell", cl.Ref.Display(sp.Src), cl.Source)
			case tiTrusted:
				rule.ReportIDf("template-injection-trusted", pos, "%q is expanded with ${{ }} into an inline script. its value is not controlled by an attacker, but an expansion in a script is easy to get wrong when the script changes. instead, read it as a variable of the shell", cl.Ref.Display(sp.Src))
			default:
				rule.ReportIDf("template-injection-expansion", pos, "%q is expanded with ${{ }} into an inline script, so a value with shell syntax changes what the script does. instead, pass it through an environment variable and read it as a variable of the shell", cl.Ref.Display(sp.Src))
			}
			if fix != nil {
				rule.errs[len(rule.errs)-1].Fix = fix
			}
		}
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "template-injection-trusted", Group: RuleGroupStyle, Summary: "A ${{ }} expansion in a script is of a value that an attacker cannot control.", DefaultLevel: SeverityInfo, Profile: ProfileAll, Fixable: true, DocsAnchor: "check-template-injection-expansion"},
		RuleInfo{ID: "template-injection-expansion", Group: RuleGroupSecurity, Summary: "A ${{ }} expansion in a script is not an environment variable.", DefaultLevel: SeverityWarning, Profile: ProfileStrict, Fixable: true, DocsAnchor: "check-template-injection-expansion"},
	)
}

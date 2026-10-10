package jactionlint

import (
	"fmt"
	"strconv"
)

// dependabotImplicitCooldownDays is the cooldown Dependabot applies when "default-days" is not set.
// https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference#cooldown-
const dependabotImplicitCooldownDays = 3

// dependabotDefaultMinCooldownDays is the minimum "default-days" required when the rule is not
// configured with the "days" option.
const dependabotDefaultMinCooldownDays = 7

// dependabotMaxCooldownDays is the largest number of days that Dependabot accepts in a cooldown.
const dependabotMaxCooldownDays = 90

// RuleDependabotCooldown is a rule to check that the updates of a Dependabot configuration wait for a
// new version to age before proposing it.
type RuleDependabotCooldown struct {
	DependabotRuleBase
	src *dependabotSource
	// why is the reason the current update gets no fix from its options. It is nil when the options allow one.
	why *NoFix
}

// NewRuleDependabotCooldown creates a new RuleDependabotCooldown instance. src is the content of the
// file, which is used to build automatic fixes. It can be nil.
func NewRuleDependabotCooldown(src []byte) *RuleDependabotCooldown {
	r := &RuleDependabotCooldown{
		DependabotRuleBase: NewDependabotRuleBase("dependabot-cooldown", "Checks that updates of dependabot.yml set a cooldown of at least the minimum number of days"),
	}
	if src != nil {
		r.src = newDependabotSource(src)
	}
	return r
}

// VisitDependabotUpdate is callback when visiting an item of "updates".
func (r *RuleDependabotCooldown) VisitDependabotUpdate(u *DependabotUpdate) error {
	if dependabotUpdatesDisabled(u) {
		return nil // a cooldown delays pull requests that are never opened
	}
	cfg := r.Config()
	minDays := dependabotDefaultMinCooldownDays
	if v, ok := cfg.ruleOptionNumber("dependabot-cooldown", "days"); ok {
		minDays = int(v)
	}

	// A "default-days" which is not a valid number is a syntax error of its own. Reporting a missing key and adding a
	// second one would only leave two keys, of which the last one wins.
	if u.Cooldown != nil && u.Cooldown.HasDefaultDays && u.Cooldown.DefaultDays == nil {
		return nil
	}

	days := dependabotImplicitCooldownDays
	if u.Cooldown != nil && u.Cooldown.DefaultDays != nil {
		days = u.Cooldown.DefaultDays.Value
	}
	if days >= minDays {
		return nil
	}

	// The fix needs a number of days chosen by the user. There is no default on purpose.
	fixDays := -1
	var why *NoFix
	if v, ok := cfg.ruleOptionNumber("dependabot-cooldown", "default-days"); !ok {
		why = &NoFix{Code: NoFixOptionRequired, Option: "dependabot-cooldown.default-days", Reason: "set the dependabot-cooldown option default-days to enable the fix, jactionlint does not choose a number of days"}
	} else if int(v) < minDays || int(v) > dependabotMaxCooldownDays {
		why = &NoFix{Code: NoFixOptionInvalid, Option: "dependabot-cooldown.default-days", Reason: fmt.Sprintf("the dependabot-cooldown option default-days must be between %d and %d for the fix to use it", minDays, dependabotMaxCooldownDays)}
	} else {
		fixDays = int(v)
	}
	r.why = why
	hint := ""
	if why != nil && why.Code == NoFixOptionRequired {
		hint = ". set \"default-days\" in the \"dependabot-cooldown\" rule options of the configuration to turn on the --fix fix"
	}

	switch {
	case u.Cooldown == nil:
		r.report(u.Pos, r.addCooldown(u, fixDays), "\"cooldown\" is not set in this update, so Dependabot applies its implicit cooldown of %d days. set \"cooldown.default-days\" to at least %d to avoid updating to a version right after its release%s", dependabotImplicitCooldownDays, minDays, hint)
	case u.Cooldown.DefaultDays == nil:
		r.report(u.Cooldown.Pos, r.addDefaultDays(u, fixDays), "\"cooldown\" does not set \"default-days\", so Dependabot applies its implicit cooldown of %d days. set \"default-days\" to at least %d%s", dependabotImplicitCooldownDays, minDays, hint)
	default:
		d := u.Cooldown.DefaultDays
		r.report(d.Pos, r.raiseDefaultDays(u, fixDays), "\"cooldown.default-days\" is %d, which is less than the minimum %d days. set it to at least %d%s", d.Value, minDays, minDays, hint)
	}
	return nil
}

// dependabotUpdatesDisabled reports whether the version updates of the entry are turned off: "open-pull-requests-limit: 0"
// or an ignore of every dependency and every kind of update. A cooldown only delays version updates, so it has no
// purpose there.
func dependabotUpdatesDisabled(u *DependabotUpdate) bool {
	if l := u.OpenPullRequestsLimit; l != nil && l.Expression == nil && l.Value == 0 {
		return true
	}
	for _, i := range u.Ignore {
		if i == nil || i.DependencyName == nil || i.DependencyName.Value != "*" || len(i.Versions) > 0 {
			continue
		}
		if len(i.UpdateTypes) == 0 {
			return true
		}
		all := map[string]bool{}
		for _, t := range i.UpdateTypes {
			all[t.Value] = true
		}
		if all["version-update:semver-major"] && all["version-update:semver-minor"] && all["version-update:semver-patch"] {
			return true
		}
	}
	return false
}

func (r *RuleDependabotCooldown) report(pos *Pos, fix *Fix, format string, args ...any) {
	r.ReportIDf("dependabot-cooldown", pos, format, args...)
	e := r.errs[len(r.errs)-1]
	switch {
	case fix != nil:
		e.Fix = fix
	case r.why != nil:
		e.NoFix = r.why
	default:
		e.NoFix = &NoFix{Code: NoFixUnsupportedShape, Reason: "the update is not written in a way the fix can edit"}
	}
}

// addCooldown makes the fix adding the "cooldown" section to an update without one. The section is
// inserted after the "package-ecosystem" line, which is always a single line.
func (r *RuleDependabotCooldown) addCooldown(u *DependabotUpdate, days int) *Fix {
	if days < 0 || r.src == nil || u.PackageEcosystem == nil || u.PackageEcosystem.Pos == nil {
		return nil
	}
	e := u.PackageEcosystem
	indent, end, eol, text, ok := r.src.keyOfValueAt(e.Pos, "package-ecosystem")
	if !ok {
		return nil
	}
	if _, ok := scalarToken(text, e.Value, e.Quoted); !ok {
		return nil
	}
	return &Fix{
		Description: fmt.Sprintf("Add cooldown with default-days: %d", days),
		Edits:       []TextEdit{r.src.insertAfter(end, eol, indent, "cooldown:", "  default-days: "+strconv.Itoa(days))},
	}
}

// addDefaultDays makes the fix adding "default-days" to a "cooldown" section which has none.
func (r *RuleDependabotCooldown) addDefaultDays(u *DependabotUpdate, days int) *Fix {
	if days < 0 || r.src == nil {
		return nil
	}
	pos := u.Cooldown.Pos
	indent, end, eol, rest, ok := r.src.keyAt(pos, "cooldown")
	if !ok || !dependabotLineRest.Match(rest) {
		return nil // e.g. the flow mapping "cooldown: {}"
	}
	child, ok := r.src.childIndent(pos.Line, indent)
	if !ok {
		child = indent + 2
	}
	return &Fix{
		Description: fmt.Sprintf("Add default-days: %d to cooldown", days),
		Edits:       []TextEdit{r.src.insertAfter(end, eol, child, "default-days: "+strconv.Itoa(days))},
	}
}

// raiseDefaultDays makes the fix replacing a too small "default-days".
func (r *RuleDependabotCooldown) raiseDefaultDays(u *DependabotUpdate, days int) *Fix {
	if days < 0 || r.src == nil {
		return nil
	}
	d := u.Cooldown.DefaultDays
	edit, ok := r.src.replaceInt(d.Pos, d.Value, days)
	if !ok {
		return nil
	}
	return &Fix{
		Description: fmt.Sprintf("Set default-days to %d", days),
		Edits:       []TextEdit{edit},
	}
}

func init() {
	registerRules(RuleInfo{
		ID: "dependabot-cooldown", Group: RuleGroupSecurity,
		Summary:      "An update in dependabot.yml has no cooldown or a cooldown shorter than the minimum.",
		DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-dependabot-cooldown",
		Options: []RuleOption{
			{Name: "days", Kind: RuleOptionInt, Default: dependabotDefaultMinCooldownDays, Summary: "The minimum number of days \"cooldown.default-days\" must be. Defaults to 7."},
			{Name: "default-days", Kind: RuleOptionInt, Summary: "The number of days --fix writes as \"cooldown.default-days\". It must be at least \"days\". There is no default: without it findings have no fix."},
		},
	})
	dependabotRuleFactories = append(dependabotRuleFactories, dependabotRuleFactory{
		kind: "dependabot-cooldown",
		new: func(ctx *dependabotRuleContext) (DependabotRule, error) {
			if !ctx.config.RuleEnabled("dependabot-cooldown") {
				return nil, nil
			}
			return NewRuleDependabotCooldown(ctx.src), nil
		},
	})
}

package jactionlint

import (
	"fmt"
	"strings"
	"time"
)

// actionReleaseCooldownDefaultDays is the cooldown when the rule is enabled without the option "days". It is
// the one of dependabot-cooldown.
const actionReleaseCooldownDefaultDays = 7

// releaseNow is the clock of the rule. The tests fix it.
var releaseNow = time.Now

// RuleActionReleaseCooldown reports an action pinned to a version which was released less than a number of days
// ago. A compromised release of an action is usually found and taken down within days, and a pin that
// waits for a release to age is not hit by it. It is the check of pinact's --min-age, and the twin of
// dependabot-cooldown, which asks the same of the updates that Dependabot proposes.
//
// The date is the one of the release of GitHub which the tag names (published_at). A tag without a release has
// the date of its commit, which is older than the tag when the tag was moved to an older commit, and so can
// hide a fresh release. It does not report a branch.
type RuleActionReleaseCooldown struct {
	onlineUsesRule
	days int
}

// NewRuleActionReleaseCooldown creates a new RuleActionReleaseCooldown instance.
func NewRuleActionReleaseCooldown(sess *onlineSession, days int) *RuleActionReleaseCooldown {
	return &RuleActionReleaseCooldown{
		onlineUsesRule: newOnlineUsesRule("action-release-cooldown", "Checks that the version of an action is older than the cooldown", sess),
		days:           days,
	}
}

// VisitWorkflowPost implements Pass.
func (r *RuleActionReleaseCooldown) VisitWorkflowPost(*Workflow) error {
	cooldown := time.Duration(r.days) * 24 * time.Hour
	for _, s := range r.sites {
		sha, tag, ok := r.versionOf(s)
		if !ok {
			continue
		}
		d, err := r.sess.ReleaseDate(s.ref.Owner, s.ref.Repo, tag, sha)
		if err != nil {
			r.skipped(s, "the release date", err)
			continue
		}
		at, release, ok := d.Time()
		if !ok {
			r.Debug("%s: no date for %s", s.ref.Raw, firstNonEmpty(tag, shortSHA(sha)))
			continue
		}
		now := releaseNow()
		age := now.Sub(at)
		if age >= cooldown {
			continue
		}
		var what, caveat string
		switch {
		case release && tag != "":
			what = fmt.Sprintf("%s was released %s", tag, ago(age, at))
		case tag != "":
			what = fmt.Sprintf("%s has no release on GitHub, and its commit %s was made %s", tag, shortSHA(sha), ago(age, at))
		default:
			what = fmt.Sprintf("the commit %s was made %s", shortSHA(sha), ago(age, at))
		}
		if !release {
			caveat = " (the date of a commit is not the one of a release: a tag moved to an older commit looks older than it is)"
		}
		r.ReportIDf("action-release-cooldown", s.pos,
			"%s %q is at a version that is younger than the cooldown of %d days: %s%s. wait for the version to age before you use it: a compromised release is usually found within days",
			s.what(), s.ref.Raw, r.days, what, caveat)
	}
	return nil
}

// versionOf finds the commit and the tag whose date to ask for. The tag is the most specific version tag of the
// commit: the one a pin names in its comment, and the one of a release. It is empty when the commit has none.
// The boolean is false for what has no date: a branch, an abbreviated SHA, a tag which does not exist.
func (r *RuleActionReleaseCooldown) versionOf(s usesSite) (sha, tag string, ok bool) {
	idx, err := r.sess.Tags(s.ref.Owner, s.ref.Repo)
	if err != nil {
		r.skipped(s, "the tags", err)
		return "", "", false
	}
	switch s.ref.RefKind {
	case RefFullSHA:
		sha = strings.ToLower(s.ref.Ref)
	case RefSemverTag, RefOther:
		c, isTag, err := r.sess.TagCommit(s.ref.Owner, s.ref.Repo, s.ref.Ref)
		if err != nil {
			r.skipped(s, "the tag "+s.ref.Ref, err)
			return "", "", false
		}
		if !isTag {
			return "", "", false // A branch moves by design, and has no release
		}
		sha, tag = c, s.ref.Ref
	default:
		return "", "", false
	}
	tagged := idx.bySHA[sha]
	if s.ref.RefKind == RefFullSHA {
		if t, ok := pickVersionTag(tagged, "", true); ok {
			tag = t
		}
	} else if t, ok := specificVersionTag(tagged, s.ref.Ref); ok {
		tag = t
	}
	return sha, tag, true
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ago says how long ago something happened: "3 days ago, on 2026-10-10", "5 hours ago".
func ago(age time.Duration, at time.Time) string {
	date := at.UTC().Format("2006-01-02")
	switch {
	case age < 0:
		return "in the future, on " + date
	case age < time.Hour:
		return fmt.Sprintf("less than an hour ago, on %s", date)
	case age < 48*time.Hour:
		return fmt.Sprintf("%d hours ago, on %s", int(age.Hours()), date)
	}
	return fmt.Sprintf("%d days ago, on %s", int(age.Hours()/24), date)
}

func validateReleaseCooldownDays(v any) error {
	if n, ok := v.(int); ok && n < 1 {
		return fmt.Errorf("it must be at least 1 day")
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{
			ID: "action-release-cooldown", Group: RuleGroupSecurity, Summary: "An action is pinned to a version released less than the cooldown ago.",
			DefaultLevel: SeverityWarning, Online: true, DocsAnchor: "check-action-release-cooldown",
			Options: []RuleOption{{
				Name: "days", Kind: RuleOptionInt, Default: actionReleaseCooldownDefaultDays, Validate: validateReleaseCooldownDays,
				Summary: "How many days a version must be old. The date is the publication of its release on GitHub, else the date of its commit. The rule runs only when \"rules\" has an entry for it, and only with the online checks.",
			}},
		},
	)
	registerRuleFactory("action-release-cooldown", func(env *RuleEnv) []Rule {
		// Opt-in: a number of days is a policy, so the rule runs only when the configuration names it
		if env.online == nil || !env.config.ruleConfigured("action-release-cooldown") || !env.config.RuleEnabled("action-release-cooldown") {
			return nil
		}
		days := actionReleaseCooldownDefaultDays
		if v, ok := env.config.ruleOptionNumber("action-release-cooldown", "days"); ok {
			days = int(v)
		}
		return []Rule{NewRuleActionReleaseCooldown(env.online, days)}
	})
}

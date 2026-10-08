package jactionlint

import (
	"slices"
	"strings"
)

// RuleKnownVulnerableActions reports actions whose version is affected by a published GitHub
// security advisory (GHSA) for the GitHub Actions ecosystem.
type RuleKnownVulnerableActions struct {
	onlineUsesRule
	allow []string
}

// NewRuleKnownVulnerableActions creates a new RuleKnownVulnerableActions instance. The advisories
// whose IDs are in allow are not reported.
func NewRuleKnownVulnerableActions(sess *onlineSession, allow []string) *RuleKnownVulnerableActions {
	return &RuleKnownVulnerableActions{newOnlineUsesRule("known-vulnerable-actions", "Checks that actions are not affected by known security advisories", sess), allow}
}

// VisitWorkflowPost implements Pass.
func (r *RuleKnownVulnerableActions) VisitWorkflowPost(*Workflow) error {
	for _, s := range r.sites {
		advs, err := r.sess.Advisories(s.ref.Owner, s.ref.Repo)
		if err != nil {
			r.skipped(s, "the advisories", err)
			continue
		}
		if len(advs) == 0 {
			continue // Most actions: no need to work out the version
		}
		v, ok, err := r.sess.versionOf(s.ref)
		if err != nil {
			r.skipped(s, "the version", err)
			continue
		}
		if !ok {
			r.Debug("%s: the version cannot be determined", s.ref.Raw)
			continue
		}
		for _, a := range advs {
			if slices.Contains(r.allow, a.ID) {
				continue
			}
			vuln, found := affectedBy(a, s.ref, v)
			if !found {
				continue
			}
			fix := "no patched version is available, so stop using the action"
			if vuln.FirstPatched != "" {
				fix = "upgrade to " + vuln.FirstPatched + " or later"
			}
			sev := ""
			if a.Severity != "" {
				sev = a.Severity + " severity, "
			}
			r.ReportIDf("known-vulnerable-actions", s.pos,
				"%s %q (version %s) is affected by %s (%s%s): %s. %s. to accept the risk add %q to the \"allow\" option of this rule",
				s.what(), s.ref.Raw, v, a.ID, sev, a.URL, strings.TrimRight(a.Summary, "."), fix, a.ID)
		}
	}
	return nil
}

// affectedBy returns the vulnerability of the advisory which covers the version of the action.
func affectedBy(a GitHubAdvisory, ref *UsesRef, v advisoryVersion) (GitHubVulnerability, bool) {
	name := strings.ToLower(ref.Owner + "/" + ref.Repo)
	full := name
	if ref.Subpath != "" && ref.Kind == UsesAction {
		full += "/" + strings.ToLower(strings.TrimRight(ref.Subpath, "/"))
	}
	for _, vuln := range a.Vulnerabilities {
		p := strings.ToLower(vuln.Package)
		if p != name && p != full {
			continue
		}
		rng, err := parseVersionRange(vuln.VulnerableRange)
		if err != nil {
			continue // A range which is not understood is not guessed at
		}
		if rng.contains(v) {
			return vuln, true
		}
	}
	return GitHubVulnerability{}, false
}

// versionOf works out which version of the action a `uses:` value runs. A version tag is the version. A
// partial tag ("v4") or a commit SHA is looked up among the tags of the repository and the most
// specific version tag on the same commit is the version. A branch has no version.
func (s *onlineSession) versionOf(ref *UsesRef) (advisoryVersion, bool, error) {
	if ref.RefKind == RefSemverTag || ref.RefKind == RefOther {
		if v, ok := parseAdvisoryVersion(ref.Ref); ok && len(v.nums) >= 3 {
			return v, true, nil // Fully specified: no need to ask
		}
	}
	idx, err := s.Tags(ref.Owner, ref.Repo)
	if err != nil {
		return advisoryVersion{}, false, err
	}
	var sha string
	switch ref.RefKind {
	case RefFullSHA:
		sha = strings.ToLower(ref.Ref)
	case RefSemverTag, RefOther:
		var ok bool
		sha, ok, err = s.TagCommit(ref.Owner, ref.Repo, ref.Ref)
		if err != nil || !ok {
			return advisoryVersion{}, false, err // A branch, or no such tag
		}
	default:
		return advisoryVersion{}, false, nil
	}
	if v, ok := mostSpecificVersion(idx.bySHA[sha]); ok {
		return v, true, nil
	}
	if ref.RefKind != RefFullSHA {
		if v, ok := parseAdvisoryVersion(ref.Ref); ok {
			return v, true, nil // "v4": the best that is known
		}
	}
	return advisoryVersion{}, false, nil
}

// mostSpecificVersion picks from tag names the one with the most components, and the highest of those.
func mostSpecificVersion(names []string) (advisoryVersion, bool) {
	var best advisoryVersion
	found := false
	for _, n := range names {
		v, ok := parseAdvisoryVersion(n)
		if !ok {
			continue
		}
		if !found || len(v.nums) > len(best.nums) || (len(v.nums) == len(best.nums) && v.compare(best) > 0) {
			best, found = v, true
		}
	}
	return best, found
}

func init() {
	registerRules(
		RuleInfo{
			ID: "known-vulnerable-actions", Group: RuleGroupSecurity, Summary: "An action version is affected by a published GitHub security advisory.",
			DefaultLevel: SeverityError, Online: true, DocsAnchor: "check-known-vulnerable-actions",
			Options: []RuleOption{{Name: "allow", Kind: RuleOptionStringList, Summary: "Advisory IDs (GHSA-...) which are not reported."}},
		},
	)
	registerRuleFactory("known-vulnerable-actions", func(env *RuleEnv) []Rule {
		if env.online == nil || !env.config.RuleEnabled("known-vulnerable-actions") {
			return nil
		}
		allow, _ := env.config.RuleOptionStrings("known-vulnerable-actions", "allow")
		return []Rule{NewRuleKnownVulnerableActions(env.online, allow)}
	})
}

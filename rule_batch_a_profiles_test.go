package jactionlint

import "testing"

// The profiles of the rules are part of their contract: a rule must not start to run in a profile
// that was chosen by the corpus review.
func TestBatchAProfiles(t *testing.T) {
	want := map[string]struct {
		profile Profile
		level   Severity
	}{
		"anonymous-definition":    {ProfileStrict, SeverityWarning},
		"concurrency-limits":      {ProfileStrict, SeverityWarning},
		"dangerous-triggers":      {ProfileStrict, SeverityWarning},
		"forbidden-uses":          {"", SeverityError},
		"insecure-commands":       {ProfileDefault, SeverityError},
		"overprovisioned-secrets": {ProfileStrict, SeverityWarning},
		"secrets-inherit":         {ProfileDefault, SeverityWarning},
		"secrets-outside-env":     {ProfileAll, SeverityWarning},
		"self-hosted-runner":      {ProfileAll, SeverityInfo},
		"typosquat-uses":          {ProfileStrict, SeverityWarning},
		"unredacted-secrets":      {ProfileStrict, SeverityWarning},
		"unsound-contains":        {ProfileDefault, SeverityWarning},
	}
	for id, w := range want {
		info, ok := LookupRule(id)
		if !ok {
			t.Errorf("rule %q is not registered", id)
			continue
		}
		if info.Profile != w.profile || info.DefaultLevel != w.level {
			t.Errorf("rule %q: want profile %q and level %v but got %q and %v", id, w.profile, w.level, info.Profile, info.DefaultLevel)
		}
	}
}

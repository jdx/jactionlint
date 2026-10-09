package jactionlint

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var updateProfiles = flag.Bool("update-profiles", false, "rewrite testdata/profiles.txt from the rule registry")

// profileSnapshotLine is the line of testdata/profiles.txt for a rule: "id profile group level". The profile
// is "online" for the rules which only run with -online and "configured" for the ones no profile enables.
func profileSnapshotLine(r RuleInfo) string {
	profile := string(r.Profile)
	switch {
	case r.Online:
		profile = "online"
	case profile == "":
		profile = "configured"
	}
	return strings.Join([]string{r.ID, profile, string(r.Group), r.DefaultLevel.String()}, " ")
}

// The profile and the level of every rule are a decision: they decide which workflows fail in a
// repository that follows a profile. testdata/profiles.txt records them, so changing one needs a change of
// the snapshot that a reviewer sees, and a new rule needs its line. Run
//
//	go test -run TestRuleProfilesSnapshot -update-profiles
//
// to write the file, and put the table of the changes in the description of the pull request.
func TestRuleProfilesSnapshot(t *testing.T) {
	path := filepath.Join("testdata", "profiles.txt")
	var want []string
	for _, r := range Rules() {
		want = append(want, profileSnapshotLine(r))
	}
	if *updateProfiles {
		if err := os.WriteFile(path, []byte(strings.Join(want, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			have = append(have, l)
		}
	}
	for _, l := range have {
		if !slices.Contains(want, l) {
			t.Errorf("testdata/profiles.txt has %q, which no rule has any more (profile, group or level changed, or the rule is gone). run the test with -update-profiles after deciding it", l)
		}
	}
	for _, l := range want {
		if !slices.Contains(have, l) {
			t.Errorf("testdata/profiles.txt lacks %q. a new rule needs the decision of its profile; run the test with -update-profiles", l)
		}
	}
}

// Each profile includes the one before it, the rules of the default profile report errors, and the
// correctness profile has no security posture or policy rule except the documented ones.
func TestRuleProfilesInvariants(t *testing.T) {
	// Rules of the policy or security group that the correctness profile has anyway
	correctnessExceptions := map[string]string{
		"template-injection":              "actionlint reports the untrusted inputs in a script too",
		"hardcoded-container-credentials": "actionlint has the check (credentials)",
		"expired-ignore":                  "it reports the config's own ignores that expired, which the user asked for with until",
		"unused-baseline-entry":           "it only runs with -baseline-check, which the user asked for",
	}
	// Rules of the correctness and default profiles that are not errors
	notError := map[string]string{
		"unused-baseline-entry": "an info finding by design; only -baseline-check produces it",
	}
	for _, r := range Rules() {
		if r.Online || r.Profile == "" {
			continue
		}
		if r.Profile == ProfileCorrectness && r.Group != RuleGroupCorrectness {
			if _, ok := correctnessExceptions[r.ID]; !ok {
				t.Errorf("rule %q (%s) is in the correctness profile, which has only the correctness group", r.ID, r.Group)
			}
		}
		if r.Profile != ProfilePedantic && r.DefaultLevel != SeverityError {
			if _, ok := notError[r.ID]; !ok {
				t.Errorf("rule %q of the %s profile reports %v but the rules of the correctness and default profiles are errors", r.ID, r.Profile, r.DefaultLevel)
			}
		}
	}
	if !ProfilePedantic.Includes(ProfileDefault) || !ProfileDefault.Includes(ProfileCorrectness) || ProfileCorrectness.Includes(ProfileDefault) {
		t.Error("each profile must include the one before it")
	}
}

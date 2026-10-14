package jactionlint

import "testing"

// An opt-in rule says so in its registration: RuleLevel and RuleRuns, which the consumers of "did the rule run"
// (unused baseline entries, unused ignore comments) ask, report "not running" until the configuration has an entry for it.
func TestActionReleaseCooldownIsOffUntilConfigured(t *testing.T) {
	const id = "action-release-cooldown"
	for _, cfg := range []*Config{nil, {}, mustParseConfig(t, "profile: pedantic\n"), mustParseConfig(t, "online: true\n")} {
		if cfg.RuleEnabled(id) || cfg.RuleRuns(id, true) || cfg.RuleLevel(id) != SeverityOff {
			t.Errorf("%+v: the rule is off without an entry in rules: level %v", cfg, cfg.RuleLevel(id))
		}
	}
	for src, want := range map[string]bool{
		"rules:\n  action-release-cooldown: warning\n":         true,
		"rules:\n  action-release-cooldown:\n    days: 7\n":    true,
		"rules:\n  action-release-cooldown: off\n":             false,
		"rules:\n  action-release-cooldown:\n    level: off\n": false,
	} {
		cfg := mustParseConfig(t, src)
		if got := cfg.RuleRuns(id, true); got != want {
			t.Errorf("%q: runs online = %v, want %v", src, got, want)
		}
		if cfg.RuleRuns(id, false) {
			t.Errorf("%q: an online rule does not run offline", src)
		}
	}
}

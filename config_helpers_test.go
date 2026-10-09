package jactionlint

import (
	"os"
	"testing"
)

// ruleSwitch returns a config which sets the rule to error or turns it off.
func ruleSwitch(id string, enabled bool) *Config {
	lv := SeverityOff
	if enabled {
		lv = SeverityError
	}
	return withoutMissingTimeout(&Config{Rules: map[string]RuleConfig{id: {Level: lv}}})
}

// ruleConfig returns a config which enables the rules as errors.
func ruleConfig(ids ...string) *Config {
	c := withoutMissingTimeout(&Config{Rules: map[string]RuleConfig{}})
	for _, id := range ids {
		c.Rules[id] = RuleConfig{Level: SeverityError}
	}
	return c
}

func mustParseConfig(t testing.TB, src string) *Config {
	t.Helper()
	c, err := ParseConfig([]byte(src))
	if err != nil {
		t.Fatalf("could not parse config %q: %v", src, err)
	}
	return c
}

// maxRunLinesConfig returns a config which allows at most n lines in a run: script. Zero disables the rule.
func maxRunLinesConfig(n int) *Config {
	if n == 0 {
		return withoutMissingTimeout(&Config{})
	}
	return withoutMissingTimeout(&Config{Rules: map[string]RuleConfig{"max-run-lines": {Level: SeverityError, Options: map[string]any{"max": n}}}})
}

// fixtureConfig returns the config for linting the files in testdata. The golden files test one check
// at a time so the rules which are enabled by default for finding bugs in workflows with local
// actions or unknown workflow_run names are turned off, unless the test enables them by name.
// The rules named in enable run as errors.
func fixtureConfig(enable ...string) *Config {
	c := withFixtureRules(&Config{})
	for _, id := range enable {
		c.Rules[id] = RuleConfig{Level: SeverityError}
	}
	return c
}

// withFixtureRules turns off the rules enabled by default for finding bugs in workflows unless the
// config sets them explicitly.
func withFixtureRules(c *Config) *Config {
	if c.Rules == nil {
		c.Rules = map[string]RuleConfig{}
	}
	for _, id := range []string{"local-action-checkout", "workflow-run-names", "unsound-ternary", "missing-timeout", "insecure-commands", "secrets-inherit", "unsound-contains"} {
		if _, ok := c.Rules[id]; !ok {
			c.Rules[id] = RuleConfig{Level: SeverityOff}
		}
	}
	for _, id := range batchHFixtureOffRules {
		if _, ok := c.Rules[id]; !ok {
			c.Rules[id] = RuleConfig{Level: SeverityOff}
		}
	}
	turnOffBatchDRules(c)
	return c
}

// withoutMissingTimeout turns off the missing-timeout rule, which the default profile enables, unless the
// config sets it explicitly or selects the strict or all profile, which test it on purpose. Tests whose workflows do not set timeout-minutes use it to look at
// the rule they are about.
func withoutMissingTimeout(c *Config) *Config {
	if c.Profile == ProfileStrict || c.Profile == ProfileAll {
		return c
	}
	if c.Rules == nil {
		c.Rules = map[string]RuleConfig{}
	}
	if _, ok := c.Rules["missing-timeout"]; !ok {
		c.Rules["missing-timeout"] = RuleConfig{Level: SeverityOff}
	}
	return c
}

// fixtureConfigFile reads the configuration which is put next to a fixture in testdata (the fixture
// "x.yaml" has "x.config") and returns it with the fixture rules turned off like fixtureConfig. It
// returns nil when the fixture has no configuration file.
func fixtureConfigFile(t testing.TB, path string) *Config {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	c, err := ParseConfig(b)
	if err != nil {
		t.Fatalf("invalid configuration %q: %v", path, err)
	}
	return withFixtureRules(c)
}

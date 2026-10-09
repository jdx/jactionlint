package jactionlint

// batchDDefaultRules are the rules of the run-script batch which the default profile enables. The golden
// files of the other checks must not show their findings, so fixtures turn them off unless a fixture
// enables them in its own x.config file.
var batchDDefaultRules = []string{
	"github-env", "github-env-untrusted-input", "adhoc-packages", "unlocked-install", "unpinned-tools",
	"use-trusted-publishing", "superfluous-actions",
}

// turnOffBatchDRules turns off the default rules of the batch unless the config sets them explicitly.
func turnOffBatchDRules(c *Config) {
	for _, id := range batchDDefaultRules {
		if _, ok := c.Rules[id]; !ok {
			c.Rules[id] = RuleConfig{Level: SeverityOff}
		}
	}
}

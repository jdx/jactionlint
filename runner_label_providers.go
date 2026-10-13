package jactionlint

import (
	"fmt"
	"slices"
	"strings"
)

// runnerProvider is a service that hosts runners for GitHub Actions and names them with labels of its own: a job
// says `runs-on: blacksmith-4vcpu-ubuntu-2404` and the service starts a runner for it. The labels are not in the table of
// the labels of GitHub, so without this table every one of them would be an unknown runner label, and a workflow that runs
// fine would be an error of the correctness profile.
type runnerProvider struct {
	// name is the name in the configuration ("self-hosted-runner.providers").
	name string
	// docs is where the label format is documented.
	docs string
	// exact lists labels that are the whole label.
	exact []string
	// prefixes lists the starts of labels, in lower case. A label must have something after the prefix.
	prefixes []string
}

// runnerProviders is the one place that knows the labels of the hosted-runner providers. The prefixes are the ones
// that the documentation of each provider names, as narrow as the documented formats allow (checked 2026-10-13): a
// typo of a GitHub label (`ubunt-latest`, `ubuntu-24.4`) must stay an unknown label.
//
// BuildJet is left out on purpose: the service has been shut down since 2026-03-31, so a `buildjet-*` label gets no runner
// and is rightly unknown (https://buildjet.com/for-github-actions/blog/we-are-shutting-down).
var runnerProviders = []runnerProvider{
	{
		name: "blacksmith", docs: "https://docs.blacksmith.sh/blacksmith-runners/overview",
		prefixes: []string{"blacksmith-"}, // blacksmith-4vcpu-ubuntu-2404, blacksmith-6vcpu-macos-latest, blacksmith-2vcpu-windows-2025
	},
	{
		name: "depot", docs: "https://depot.dev/docs/github-actions/runner-types",
		prefixes: []string{"depot-ubuntu-", "depot-windows-", "depot-macos-"}, // depot-ubuntu-24.04-4, depot-ubuntu-latest, depot-macos-26
	},
	{
		name: "namespace", docs: "https://namespace.so/docs/solutions/github-actions",
		prefixes: []string{"namespace-profile-", "nscloud-", "namespace-features:"}, // namespace-profile-big-apple, nscloud-ubuntu-22.04-amd64-8x16
	},
	{
		name: "runson", docs: "https://runs-on.com/configuration/job-labels/",
		// runs-on=<run id>/runner=2cpu-linux-x64 as one label, or one label for each key=value
		prefixes: []string{"runs-on=", "runs-on/", "runner=", "family=", "image=", "cpu=", "ram=", "spot=", "extras=", "pool=", "volume=", "retry=", "ami=", "sticky="},
	},
	{
		name: "ubicloud", docs: "https://ubicloud.com/docs/github-actions-integration/runner-types",
		exact: []string{"ubicloud", "ubicloud-arm"},
		// The families of https://github.com/ubicloud/ubicloud/blob/main/config/github_runner_labels.yml are standard and
		// premium. It has no gpu label.
		prefixes: []string{"ubicloud-standard-", "ubicloud-premium-"}, // ubicloud-standard-4-arm-ubuntu-2204, ubicloud-premium-8
	},
	{
		name: "warpbuild", docs: "https://www.warpbuild.com/docs/ci/cloud-runners",
		prefixes: []string{"warp-"}, // warp-ubuntu-latest-x64-4x, warp-macos-26-arm64-6x
	},
}

// runnerProviderNames returns the names that "self-hosted-runner.providers" accepts.
func runnerProviderNames() []string {
	ret := make([]string, 0, len(runnerProviders))
	for _, p := range runnerProviders {
		ret = append(ret, p.name)
	}
	return ret
}

// validateRunnerProviders returns an error for a name that is not a provider.
func validateRunnerProviders(names []string) error {
	known := runnerProviderNames()
	for _, n := range names {
		if !slices.Contains(known, strings.ToLower(n)) {
			return fmt.Errorf("unknown provider %q in \"self-hosted-runner.providers\". available providers are %s", n, quotes(known))
		}
	}
	return nil
}

func (p *runnerProvider) matches(lower string) bool {
	if slices.Contains(p.exact, lower) {
		return true
	}
	for _, pre := range p.prefixes {
		if len(lower) > len(pre) && strings.HasPrefix(lower, pre) {
			return true
		}
	}
	return false
}

// runnerProviderOf returns the provider whose label the string is, among the enabled ones. enabled is the value of
// "self-hosted-runner.providers": nil means every provider, an empty list none.
func runnerProviderOf(label string, enabled []string) (string, bool) {
	l := strings.ToLower(label)
	for i := range runnerProviders {
		p := &runnerProviders[i]
		if enabled != nil && !containsFold(enabled, p.name) {
			continue
		}
		if p.matches(l) {
			return p.name, true
		}
	}
	return "", false
}

func containsFold(ss []string, s string) bool {
	return slices.ContainsFunc(ss, func(x string) bool { return strings.EqualFold(x, s) })
}

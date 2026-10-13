package jactionlint

import (
	"slices"
	"strings"
	"testing"
)

func unknownLabels(t *testing.T, cfg, runsOn string) []string {
	t.Helper()
	src := "on: push\njobs:\n  j:\n    runs-on: " + runsOn + "\n    steps:\n      - run: echo\n"
	var ret []string
	c := &Config{}
	if cfg != "" {
		c = mustParseConfig(t, cfg)
	}
	for _, e := range lintWithConfig(t, c, src) {
		if e.ID == "unknown-runner-label" || e.ID == "conflicting-runner-labels" {
			ret = append(ret, e.Message)
		}
	}
	return ret
}

func TestRunnerProviderLabels(t *testing.T) {
	for _, label := range []string{
		// https://docs.blacksmith.sh/blacksmith-runners/overview
		"blacksmith-4vcpu-ubuntu-2404", "blacksmith-2vcpu-ubuntu-2204-arm", "blacksmith-6vcpu-macos-latest", "blacksmith-2vcpu-windows-2025",
		// https://depot.dev/docs/github-actions/runner-types
		"depot-ubuntu-24.04", "depot-ubuntu-24.04-arm-8", "depot-ubuntu-latest", "depot-windows-2022-4", "depot-macos-26",
		// https://ubicloud.com/docs/github-actions-integration/runner-types
		"ubicloud", "ubicloud-arm", "ubicloud-standard-2", "ubicloud-standard-4-arm-ubuntu-2204", "ubicloud-premium-8", "ubicloud-premium-16-arm-ubuntu-2404",
		// https://www.warpbuild.com/docs/ci/cloud-runners
		"warp-ubuntu-latest-x64-4x", "warp-ubuntu-2404-arm64-8x", "warp-macos-26-arm64-6x", "warp-windows-2025-vs2026-x64-32x",
		// https://namespace.so/docs/solutions/github-actions
		"namespace-profile-big-apple", "nscloud-ubuntu-22.04-amd64-8x16-with-cache", "namespace-features:container.privileged=true",
		// https://runs-on.com/configuration/job-labels/
		"runner=64cpu-linux-x64", "family=m8+c8", "image=ubuntu24-full-x64", "cpu=2+8", "ram=16+64", "spot=false", "extras=s3-cache",
		"runs-on=12345/runner=2cpu-linux-x64", "Blacksmith-4vcpu-Ubuntu-2404",
	} {
		t.Run(label, func(t *testing.T) {
			if got := unknownLabels(t, "", label); len(got) != 0 {
				t.Errorf("a label of a provider is unknown: %v", got)
			}
		})
	}
}

func TestRunnerProviderLabelsNotAccepted(t *testing.T) {
	for _, label := range []string{
		"buildjet-2vcpu-ubuntu-2204", // the service was shut down on 2026-03-31
		"blacksmith", "blacksmith-", "depot-", "depot-gpu", "warp", "warp-", "ubicloud-gpu", "ubicloud-premium-", "ubicloud-premium", "ubicloud-", "namespace", "namespace-profile-",
		"ubunt-latest", "ubuntu-24.4", "linux-latest", "gpu", "runner", "runs-on", "cpu", "image=",
		"macos-latest-blacksmith", "xblacksmith-4vcpu",
	} {
		t.Run(label, func(t *testing.T) {
			got := unknownLabels(t, "", label)
			if len(got) != 1 || !strings.Contains(got[0], "is unknown") {
				t.Errorf("want an unknown label but got %v", got)
			}
		})
	}
}

func TestRunnerProviderLabelsInLists(t *testing.T) {
	// RunsOn: one label for each key. The first one has an expression and is not checked, the others are provider labels
	if got := unknownLabels(t, "", "[\"runs-on=${{ github.run_id }}\", runner=64cpu-linux-x64, extras=s3-cache]"); len(got) != 0 {
		t.Errorf("%v", got)
	}
	// A provider label has no operating system that could conflict with the others
	if got := unknownLabels(t, "", "[ubuntu-latest, blacksmith-4vcpu-ubuntu-2404]"); len(got) != 0 {
		t.Errorf("%v", got)
	}
	// A label which is not known stays unknown next to a provider label
	if got := unknownLabels(t, "", "[blacksmith-4vcpu-ubuntu-2404, gpu]"); len(got) != 1 {
		t.Errorf("%v", got)
	}
	// Matrix values
	src := "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest, depot-ubuntu-24.04-4, warp-ubuntu-latest-x64-4x, nope]\n    steps:\n      - run: echo\n"
	n := 0
	for _, e := range lintWithConfig(t, &Config{}, src) {
		if e.ID == "unknown-runner-label" {
			n++
			if !strings.Contains(e.Message, `"nope"`) {
				t.Errorf("%v", e)
			}
		}
	}
	if n != 1 {
		t.Errorf("want only nope to be unknown, got %d findings", n)
	}
}

func TestRunnerProvidersOption(t *testing.T) {
	const bs = "blacksmith-4vcpu-ubuntu-2404"
	for _, tc := range []struct {
		what, cfg, label string
		unknown          bool
	}{
		{"all by default", "", bs, false},
		{"listed", "self-hosted-runner:\n  providers: [blacksmith]\n", bs, false},
		{"listed in upper case", "self-hosted-runner:\n  providers: [Blacksmith]\n", bs, false},
		{"another provider is listed", "self-hosted-runner:\n  providers: [depot]\n", bs, true},
		{"the provider is listed with others", "self-hosted-runner:\n  providers: [depot, blacksmith]\n", "depot-ubuntu-24.04", false},
		{"none", "self-hosted-runner:\n  providers: []\n", bs, true},
		{"labels of the config win over the list", "self-hosted-runner:\n  providers: []\n  labels: [blacksmith-*]\n", bs, false},
		{"strict labels accept only the listed ones", "self-hosted-runner:\n  strict-labels: true\n  labels: [foo]\n", bs, true},
		{"strict labels with the label listed", "self-hosted-runner:\n  strict-labels: true\n  labels: [blacksmith-*]\n", bs, false},
	} {
		t.Run(tc.what, func(t *testing.T) {
			got := unknownLabels(t, tc.cfg, tc.label)
			if tc.unknown != (len(got) == 1) || len(got) > 1 {
				t.Errorf("unknown=%v but got %v", tc.unknown, got)
			}
		})
	}

	t.Run("an empty list is not the default", func(t *testing.T) {
		if c := mustParseConfig(t, "self-hosted-runner:\n  providers: []\n"); c.SelfHostedRunner.Providers == nil {
			t.Error("the empty list must stay an empty list, which accepts no provider")
		}
		if c := mustParseConfig(t, "self-hosted-runner:\n  labels: [x]\n"); c.SelfHostedRunner.Providers != nil {
			t.Error("not setting the option must accept every provider")
		}
	})

	t.Run("a name that is not a provider", func(t *testing.T) {
		_, err := ParseConfig([]byte("self-hosted-runner:\n  providers: [blacksmth]\n"))
		if err == nil || !strings.Contains(err.Error(), `unknown provider "blacksmth"`) || !strings.Contains(err.Error(), `"blacksmith"`) {
			t.Errorf("unexpected error %v", err)
		}
	})

	t.Run("extends", func(t *testing.T) {
		base := mustParseConfig(t, "self-hosted-runner:\n  providers: [depot]\n")
		over := mustParseConfig(t, "self-hosted-runner:\n  labels: [x]\n")
		base.merge(over)
		if !slices.Equal(base.SelfHostedRunner.Providers, []string{"depot"}) {
			t.Errorf("a file that does not set it keeps the list: %v", base.SelfHostedRunner.Providers)
		}
		base.merge(mustParseConfig(t, "self-hosted-runner:\n  providers: []\n"))
		if base.SelfHostedRunner.Providers == nil || len(base.SelfHostedRunner.Providers) != 0 {
			t.Errorf("a file that sets it wins: %v", base.SelfHostedRunner.Providers)
		}
	})
}

func TestRunnerProvidersTable(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range runnerProviders {
		if p.name != strings.ToLower(p.name) || seen[p.name] || !strings.HasPrefix(p.docs, "https://") {
			t.Errorf("provider %q: unique lower-case name and its documentation are required", p.name)
		}
		seen[p.name] = true
		if len(p.exact)+len(p.prefixes) == 0 {
			t.Errorf("provider %q has no label", p.name)
		}
		for _, s := range append(slices.Clone(p.exact), p.prefixes...) {
			if s != strings.ToLower(s) {
				t.Errorf("provider %q: %q must be in lower case", p.name, s)
			}
			// A provider must never swallow a label of GitHub, which is checked before it
			if _, ok := defaultRunnerOSCompats[s]; ok {
				t.Errorf("provider %q: %q is a label of GitHub", p.name, s)
			}
			for _, gh := range allGitHubHostedRunnerLabels {
				if strings.HasPrefix(gh, s) {
					t.Errorf("provider %q: the prefix %q covers the label %q of GitHub", p.name, s, gh)
				}
			}
		}
	}
}

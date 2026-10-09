//go:build !js

package jactionlint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A workflow with one finding of the correctness profile (an undefined property) and one of the default
// profile (an unpinned action).
const profileWorkflow = `name: CI
on: push
permissions: {}
concurrency:
  group: ci
  cancel-in-progress: true
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v4
      - run: echo ${{ github.nope }}
`

func profileCmd(t *testing.T, args ...string) (status int, ids []string, stderr string) {
	t.Helper()
	status, out, errOut := baselineCmd(t, args...)
	if strings.TrimSpace(out) != "" {
		ids = reportedIDs(t, out)
	}
	return status, ids, errOut
}

func TestProfileFlagOverridesConfig(t *testing.T) {
	baselineProject(t, profileWorkflow, "profile: correctness\n")

	_, ids, _ := profileCmd(t)
	if countID(ids, "unpinned-uses") != 0 || countID(ids, "undefined-property") != 1 {
		t.Errorf("the correctness profile of the config must not report the unpinned action: %v", ids)
	}
	_, ids, _ = profileCmd(t, "-profile", "default")
	if countID(ids, "unpinned-uses") != 1 || countID(ids, "undefined-property") != 1 {
		t.Errorf("-profile default must win over the config: %v", ids)
	}
	_, ids, _ = profileCmd(t, "-profile=pedantic")
	if countID(ids, "unpinned-uses") != 1 {
		t.Errorf("-profile pedantic: %v", ids)
	}
}

func TestProfileFlagWithoutConfig(t *testing.T) {
	baselineProject(t, profileWorkflow, "# no profile\n")
	// The profile of the run is the default one
	withDefaultProfile(t)
	_, ids, _ := profileCmd(t)
	if countID(ids, "unpinned-uses") != 1 {
		t.Errorf("the default profile must report the unpinned action: %v", ids)
	}
	_, ids, _ = profileCmd(t, "-profile", "correctness")
	if countID(ids, "unpinned-uses") != 0 || countID(ids, "undefined-property") != 1 {
		t.Errorf("-profile correctness: %v", ids)
	}
}

func TestProfileFlagInvalid(t *testing.T) {
	baselineProject(t, profileWorkflow, "")
	for _, name := range []string{"strict", "all", "paranoid"} {
		status, _, stderr := profileCmd(t, "-profile", name)
		if status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, `invalid value "`+name+`" for -profile`) || !strings.Contains(stderr, `"correctness", "default" and "pedantic"`) {
			t.Errorf("-profile %s: %d %q", name, status, stderr)
		}
	}
}

func TestRetiredProfileNamesInConfig(t *testing.T) {
	for _, name := range []string{"strict", "all"} {
		c, err := ParseConfig([]byte("profile: " + name + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		if c.Profile != ProfilePedantic {
			t.Errorf("profile: %s must mean pedantic but got %q", name, c.Profile)
		}
		if len(c.Deprecations) != 1 || !strings.Contains(c.Deprecations[0], `"profile: `+name+`" is deprecated`) || !strings.Contains(c.Deprecations[0], `"profile: pedantic"`) {
			t.Errorf("deprecation: %q", c.Deprecations)
		}
	}
	if _, err := ParseConfig([]byte("profile: paranoid\n")); err == nil || !strings.Contains(err.Error(), `"correctness", "default" and "pedantic"`) {
		t.Errorf("an unknown profile must list the available ones: %v", err)
	}
	// The warning is printed once per run
	baselineProject(t, profileWorkflow, "profile: strict\n")
	_, _, stderr := profileCmd(t)
	if strings.Count(stderr, `"profile: strict" is deprecated`) != 1 {
		t.Errorf("want one warning: %q", stderr)
	}
}

func TestMigrateConfigRetiredProfile(t *testing.T) {
	for _, name := range []string{"strict", "all"} {
		src := "# my config\nprofile: " + name + " # old name\nrules:\n  unpinned-uses: warn\n"
		got, migrated, err := MigrateConfig([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != strings.Replace(src, name, "pedantic", 1) || len(migrated) != 1 || migrated[0] != "profile" {
			t.Errorf("%s: %q %v", name, got, migrated)
		}
	}
	// It is migrated together with the deprecated keys
	got, migrated, err := MigrateConfig([]byte("profile: all\nrequire-shell: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseConfig(got)
	if err != nil || c.Profile != ProfilePedantic || len(c.Deprecations) != 0 || len(migrated) != 2 {
		t.Errorf("%q %v %v", got, migrated, err)
	}
	// Nothing to migrate
	src := "profile: default\n"
	if got, migrated, err := MigrateConfig([]byte(src)); err != nil || string(got) != src || len(migrated) != 0 {
		t.Errorf("%q %v %v", got, migrated, err)
	}
}

func TestActionlintConfigNotice(t *testing.T) {
	setup := func(t *testing.T, config string) {
		root := baselineProject(t, profileWorkflow, "")
		if err := os.Remove(filepath.Join(root, ".github", "jactionlint.yaml")); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(root, ".github", "actionlint.yaml"), config)
	}

	// A file for actionlint without a profile is read, with the default profile, and the notice says so once
	setup(t, "self-hosted-runner:\n  labels: [mine]\n")
	withDefaultProfile(t)
	_, ids, stderr := profileCmd(t)
	if countID(ids, "unpinned-uses") != 1 {
		t.Errorf("the default profile applies to a file without a profile: %v", ids)
	}
	if strings.Count(stderr, "note: config file") != 1 || !strings.Contains(stderr, ".github/actionlint.yaml") || !strings.Contains(stderr, "profile: correctness") {
		t.Errorf("want one notice naming the file and the profile for actionlint: %q", stderr)
	}

	// -profile makes the notice true to its word
	_, ids, _ = profileCmd(t, "-profile", "correctness")
	if countID(ids, "unpinned-uses") != 0 {
		t.Errorf("-profile correctness: %v", ids)
	}

	// A file that chooses a profile needs no notice
	setup(t, "profile: correctness\n")
	_, ids, stderr = profileCmd(t)
	if strings.Contains(stderr, "note:") || countID(ids, "unpinned-uses") != 0 {
		t.Errorf("no notice when the profile is set: %q %v", stderr, ids)
	}

	// A jactionlint file is not a legacy file
	baselineProject(t, profileWorkflow, "self-hosted-runner:\n  labels: [mine]\n")
	if _, _, stderr := profileCmd(t); strings.Contains(stderr, "note:") {
		t.Errorf("a jactionlint config needs no notice: %q", stderr)
	}
}

func TestSARIFRulesHaveTheirProfile(t *testing.T) {
	baselineProject(t, profileWorkflow, "")
	status, out, _ := baselineCmd(t, "-profile", "default", "-format", "sarif")
	if status != ExitStatusSuccessProblemFound {
		t.Fatalf("status %d", status)
	}
	var log struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID         string         `json:"id"`
						Properties map[string]any `json:"properties"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &log); err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	for _, r := range log.Runs[0].Tool.Driver.Rules {
		got[r.ID] = r.Properties["profile"]
	}
	if got["undefined-property"] != "correctness" || got["unpinned-uses"] != "default" {
		t.Errorf("profiles in the SARIF rules: %v", got)
	}
}

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

// A workflow with a finding of each pedantic tier that was a rule of its own before the audits were merged.
const retiredIDsWorkflow = `name: CI
on: pull_request_target
permissions: {}
concurrency:
  group: ci
  cancel-in-progress: true
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - run: echo ${{ github.event.pull_request.title }}
      - run: echo ${{ inputs.name }}
      - run: echo ${{ github.repository }}
        shell: zsh
`

func countTemplateInjection(t *testing.T, args ...string) (tiers map[string]int, stderr string) {
	t.Helper()
	status, out, errOut := baselineCmd(t, append([]string{"-format", "json"}, args...)...)
	if status == ExitStatusFailure {
		t.Fatalf("status %d: %s", status, errOut)
	}
	var errs []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	if strings.TrimSpace(out) != "" {
		if err := json.Unmarshal([]byte(out), &errs); err != nil {
			t.Fatalf("%v: %q", err, out)
		}
	}
	tiers = map[string]int{}
	for _, e := range errs {
		switch {
		case e.ID == "template-injection" && strings.Contains(e.Message, "is potentially untrusted"):
			tiers["direct"]++
		case e.ID == "template-injection" && strings.Contains(e.Message, "so a value with shell syntax"):
			tiers["expansion"]++
		case e.ID == "template-injection" && strings.Contains(e.Message, "is not controlled by an attacker"):
			tiers["trusted"]++
		case e.ID == "misfeature":
			tiers["shell"]++
		}
	}
	return tiers, errOut
}

func TestPedanticOptionOfTheMergedAudits(t *testing.T) {
	baselineProject(t, retiredIDsWorkflow, "profile: default\n")

	tiers, _ := countTemplateInjection(t)
	if tiers["direct"] != 1 || tiers["expansion"] != 0 || tiers["trusted"] != 0 || tiers["shell"] != 0 {
		t.Errorf("the default profile reports the untrusted input only: %v", tiers)
	}
	tiers, _ = countTemplateInjection(t, "-profile", "pedantic")
	if tiers["direct"] != 1 || tiers["expansion"] != 1 || tiers["trusted"] != 1 || tiers["shell"] != 1 {
		t.Errorf("the pedantic profile reports every tier: %v", tiers)
	}

	// The option turns the tiers on in one audit under the default profile, and off under the pedantic one
	baselineProject(t, retiredIDsWorkflow, "profile: default\nrules:\n  template-injection:\n    pedantic: true\n")
	tiers, _ = countTemplateInjection(t)
	if tiers["expansion"] != 1 || tiers["trusted"] != 1 || tiers["shell"] != 0 {
		t.Errorf("pedantic: true on template-injection: %v", tiers)
	}
	baselineProject(t, retiredIDsWorkflow, "profile: pedantic\nrules:\n  template-injection:\n    pedantic: false\n")
	tiers, _ = countTemplateInjection(t)
	if tiers["direct"] != 1 || tiers["expansion"] != 0 || tiers["trusted"] != 0 || tiers["shell"] != 1 {
		t.Errorf("pedantic: false on template-injection: %v", tiers)
	}
}

func TestRetiredRuleIDsInIgnores(t *testing.T) {
	baselineProject(t, retiredIDsWorkflow, "profile: pedantic\n")

	// -ignore with a retired ID drops only the findings it had, and warns once
	tiers, stderr := countTemplateInjection(t, "-ignore", "template-injection-expansion")
	if tiers["direct"] != 1 || tiers["expansion"] != 0 || tiers["trusted"] != 1 || tiers["shell"] != 1 {
		t.Errorf("-ignore=template-injection-expansion: %v", tiers)
	}
	if strings.Count(stderr, `the rule ID "template-injection-expansion" was merged into "template-injection"`) != 1 {
		t.Errorf("want one deprecation warning: %q", stderr)
	}
	tiers, _ = countTemplateInjection(t, "-ignore", "template-injection-trusted", "-ignore", "misfeature-custom-shell")
	if tiers["direct"] != 1 || tiers["expansion"] != 1 || tiers["trusted"] != 0 || tiers["shell"] != 0 {
		t.Errorf("-ignore of two retired IDs: %v", tiers)
	}
	// The new ID ignores the whole audit
	tiers, stderr = countTemplateInjection(t, "-ignore", "template-injection")
	if tiers["direct"] != 0 || tiers["expansion"] != 0 || tiers["trusted"] != 0 || strings.Contains(stderr, "warning") {
		t.Errorf("-ignore=template-injection: %v %q", tiers, stderr)
	}

	// "paths" ignores warn when the config is read
	baselineProject(t, retiredIDsWorkflow, "profile: pedantic\npaths:\n  \".github/workflows/*.yaml\":\n    ignore: [template-injection-trusted]\n")
	tiers, stderr = countTemplateInjection(t)
	if tiers["trusted"] != 0 || tiers["expansion"] != 1 || !strings.Contains(stderr, `"paths": the rule ID "template-injection-trusted" was merged`) {
		t.Errorf("paths ignore: %v %q", tiers, stderr)
	}

	// Inline comments too
	wf := strings.Replace(retiredIDsWorkflow, "      - run: echo ${{ github.repository }}\n", "      # jactionlint ignore=template-injection-trusted\n      - run: echo ${{ github.repository }}\n", 1)
	baselineProject(t, wf, "profile: pedantic\n")
	tiers, stderr = countTemplateInjection(t)
	if tiers["trusted"] != 0 || tiers["expansion"] != 1 || tiers["shell"] != 1 || !strings.Contains(stderr, `was merged into "template-injection"`) {
		t.Errorf("inline ignore: %v %q", tiers, stderr)
	}
}

func TestRetiredRuleIDsAreNotRules(t *testing.T) {
	for _, rr := range RenamedRules() {
		if _, ok := LookupRule(rr.Old); ok {
			t.Errorf("%q is a rule", rr.Old)
		}
		cfg := "rules:\n  " + rr.Old + ": warn\n"
		_, err := ParseConfig([]byte(cfg))
		if err == nil || !strings.Contains(err.Error(), `"`+rr.Old+`" was merged into "`+rr.ID+`"`) {
			t.Errorf("%q in rules must say where it went: %v", rr.Old, err)
		}
		if _, err := ParseConfig([]byte("ignores:\n  - rule: " + rr.Old + "\n    file: x.yaml\n")); err == nil || !strings.Contains(err.Error(), "was merged into") {
			t.Errorf("%q in ignores: %v", rr.Old, err)
		}
	}
}

func TestUnusedInlineIgnoreOfARetiredID(t *testing.T) {
	wf := strings.Replace(retiredIDsWorkflow, "      - run: echo ${{ github.repository }}\n", "      # jactionlint ignore=template-injection-trusted\n      - run: echo ${{ github.repository }}\n", 1)
	// Without the pedantic option the tier reports nothing, so the comment is not called unused
	baselineProject(t, wf, "profile: default\nrules:\n  unused-ignore: error\n")
	_, ids, _ := profileCmd(t)
	if countID(ids, "unused-ignore") != 0 {
		t.Errorf("an ignore of a tier that does not run is not unused: %v", ids)
	}
	// With it the comment is used
	baselineProject(t, wf, "profile: pedantic\n")
	_, ids, _ = profileCmd(t)
	if countID(ids, "unused-ignore") != 0 {
		t.Errorf("used: %v", ids)
	}
	// A comment for a tier with nothing to suppress is unused
	wf2 := strings.Replace(retiredIDsWorkflow, "      - run: echo ${{ inputs.name }}\n", "      # jactionlint ignore=template-injection-trusted\n      - run: echo ${{ inputs.name }}\n", 1)
	baselineProject(t, wf2, "profile: pedantic\n")
	_, ids, _ = profileCmd(t)
	if countID(ids, "unused-ignore") != 1 {
		t.Errorf("unused: %v", ids)
	}
}

func TestUnlockedInstallNeedsALockFileInTheRepository(t *testing.T) {
	wf := func(run, extra string) string {
		return "name: CI\non: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  build:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: " + run + "\n" + extra
	}
	tests := []struct {
		what  string
		run   string
		extra string
		files []string // files of the repository
		want  int
	}{
		{"npm install without a lock file", "npm install", "", nil, 0},
		{"npm install with package-lock.json", "npm install", "", []string{"package-lock.json"}, 1},
		{"npm install with npm-shrinkwrap.json", "npm install", "", []string{"npm-shrinkwrap.json"}, 1},
		{"npm ci", "npm ci", "", []string{"package-lock.json"}, 0},
		{"npm install of a package", "npm install -g eslint", "", []string{"package-lock.json"}, 0},
		{"the lock file of another tool", "npm install", "", []string{"yarn.lock"}, 0},
		{"yarn install with yarn.lock", "yarn install", "", []string{"yarn.lock"}, 1},
		{"yarn with yarn.lock", "yarn", "", []string{"yarn.lock"}, 1},
		{"yarn immutable", "yarn install --immutable", "", []string{"yarn.lock"}, 0},
		{"yarn without yarn.lock", "yarn install", "", nil, 0},
		{"pnpm not frozen", "pnpm install --no-frozen-lockfile", "", []string{"pnpm-lock.yaml"}, 1},
		{"pnpm not frozen without a lock file", "pnpm install --no-frozen-lockfile", "", nil, 0},
		{"pnpm install", "pnpm install", "", []string{"pnpm-lock.yaml"}, 0},
		{"working directory with the lock file", "npm install", "        working-directory: web\n", []string{"web/package-lock.json"}, 1},
		{"working directory without it", "npm install", "        working-directory: web\n", []string{"other/package-lock.json"}, 0},
		{"working directory outside the repository", "npm install", "        working-directory: ../web\n", []string{"package-lock.json"}, 1}, // the root one counts
		{"bun stays pedantic", "bun install", "", []string{"bun.lock"}, 0},
		{"pip stays pedantic", "pip install -r requirements.txt", "", []string{"requirements.txt"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			root := baselineProject(t, wf(tc.run, tc.extra), "profile: default\n")
			for _, f := range tc.files {
				writeTestFile(t, filepath.Join(root, f), "{}\n")
			}
			_, ids, _ := profileCmd(t)
			if got := countID(ids, "unlocked-install"); got != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, ids)
			}
		})
	}

	// The pedantic option reports them without a lock file too
	for _, run := range []string{"npm install", "yarn install", "bun install", "pip install -r requirements.txt"} {
		baselineProject(t, wf(run, ""), "profile: default\nrules:\n  unlocked-install:\n    pedantic: true\n")
		if _, ids, _ := profileCmd(t); countID(ids, "unlocked-install") != 1 {
			t.Errorf("%s with pedantic: %v", run, ids)
		}
	}
}

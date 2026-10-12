package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// cacheCount lints a workflow with the cache-poisoning rule only (the pedantic option as given) and returns how many
// findings the rule reports.
func cacheCount(t *testing.T, pedantic bool, src string) int {
	t.Helper()
	opt := ""
	if pedantic {
		opt = "    pedantic: true\n"
	}
	cfg := mustParseConfig(t, "profile: correctness\nrules:\n  cache-poisoning:\n    level: error\n"+opt)
	return len(policyErrorsOf(lintWithConfig(t, cfg, src), "cache-poisoning"))
}

func TestCachePoisoningJobThatFeedsAPublishingJob(t *testing.T) {
	const on = "on:\n  push:\n    tags: ['v*']\npermissions:\n  contents: read\n"
	const upload = "      - uses: actions/upload-artifact@v4\n        with:\n          name: dist\n          path: dist\n"
	const cache = "      - uses: Swatinem/rust-cache@v2\n"
	build := func(extra string) string {
		return "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n" + cache + extra
	}
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"a job that only checks is not a release job", on + build(""), 0},
		{"uploads and nothing publishes", on + build(upload), 0},
		{"a publishing job needs it", on + build(upload) +
			"  publish:\n    needs: build\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n", 1},
		{"the same without the upload: it only has to pass", on + build("") +
			"  publish:\n    needs: build\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n", 0},
		{"the id of the job in needs has another case", on + build(upload) +
			"  publish:\n    needs: [Build]\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n", 1},
		{"through another job", on + build(upload) +
			"  test:\n    needs: build\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n" +
			"  publish:\n    needs: test\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n", 1},
		{"a job with an environment", on + build(upload) +
			"  deploy:\n    needs: build\n    runs-on: ubuntu-latest\n    environment: production\n    steps:\n      - run: echo\n", 1},
		{"a job that runs a publishing action", on + build(upload) +
			"  release:\n    needs: build\n    runs-on: ubuntu-latest\n    steps:\n      - uses: softprops/action-gh-release@v2\n", 1},
		{"id-token is how a package is published", on + build(upload) +
			"  pypi:\n    needs: build\n    runs-on: ubuntu-latest\n    permissions:\n      id-token: write\n    steps:\n      - run: echo\n", 1},
		{"a job that comments on the pull request is no publisher", on + build(upload) +
			"  note:\n    needs: build\n    runs-on: ubuntu-latest\n    permissions:\n      pull-requests: write\n      issues: write\n    steps:\n      - run: echo\n", 0},
		{"a job that reads a secret is no publisher", on + build(upload) +
			"  notify:\n    needs: build\n    runs-on: ubuntu-latest\n    steps:\n      - run: ./notify\n        env:\n          HOOK: ${{ secrets.SLACK_WEBHOOK }}\n", 0},
		{"a publishing job that does not need it", on + build(upload) +
			"  publish:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n", 0},
		{"a job that needs another one", on + build(upload) +
			"  other:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n" +
			"  publish:\n    needs: other\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, false, tc.src); got != tc.want {
				t.Errorf("want %d findings but got %d\n%s", tc.want, got, tc.src)
			}
		})
	}
}

func TestCachePoisoningImagePush(t *testing.T) {
	const head = "on: workflow_dispatch\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n      - uses: docker/build-push-action@v6\n"
	tests := []struct {
		name string
		with string
		want int
	}{
		{"push true", "        with:\n          push: true\n", 1},
		{"push from an input", "        with:\n          push: ${{ inputs.push }}\n", 1},
		{"push on the events that are not a pull request", "        with:\n          push: ${{ github.event_name != 'pull_request' }}\n", 1},
		{"push false", "        with:\n          push: false\n", 0},
		{"push empty", "        with:\n          push: ''\n", 0},
		{"no push: the default is false", "        with:\n          tags: x\n", 0},
		{"no with at all", "", 0},
		{"the registry as an output", "        with:\n          outputs: type=registry\n", 1},
		{"push by digest", "        with:\n          outputs: type=image,\"name=ghcr.io/o/r\",push-by-digest=true,push=true\n", 1},
		{"push in an expression of the output", "        with:\n          outputs: type=image,name=x,push=${{ env.PUSH }}\n", 1},
		{"outputs from an input, push true", "        with:\n          push: true\n          outputs: ${{ inputs.outputs }}\n", 1},
		{"outputs from an input", "        with:\n          outputs: ${{ inputs.outputs }}\n", 1},
		{"an output to a directory", "        with:\n          outputs: type=local,dest=out\n", 0},
		{"an output that only names the registry", "        with:\n          outputs: type=image,name=registry.example.com/x\n", 0},
		{"outputs push next to push false", "        with:\n          push: false\n          outputs: type=registry\n", 1},
		{"a local output next to push true", "        with:\n          push: true\n          outputs: type=local,dest=out\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, false, head+tc.with); got != tc.want {
				t.Errorf("want %d findings but got %d", tc.want, got)
			}
		})
	}
}

func TestCachePoisoningMorePublishers(t *testing.T) {
	// the publishers of zizmor which were not in the list
	for _, a := range []string{
		"jreleaser/release-action@v2",
		"release-drafter/release-drafter@v6",
		"aws-actions/aws-cloudformation-github-deploy@v1",
		"azure/aci-deploy@v1",
		"azure/container-apps-deploy-action@v2",
		"azure/sql-action@v2",
		"google-github-actions/deploy-cloud-functions@v3",
	} {
		src := "on: workflow_dispatch\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + a + "\n      - uses: Swatinem/rust-cache@v2\n"
		if got := cacheCount(t, false, src); got != 1 {
			t.Errorf("%s: want 1 finding but got %d", a, got)
		}
	}
}

func TestCachePoisoningSccache(t *testing.T) {
	const on = "on:\n  push:\n    tags: ['v*']\n"
	job := func(env, steps string) string {
		return on + env + "jobs:\n  j:\n    runs-on: ubuntu-latest\n" + steps
	}
	const sccache = "    steps:\n      - uses: Mozilla-Actions/sccache-action@v0.0.9\n"
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"only installs sccache", job("", sccache), 0},
		{"the variable on the workflow", job("env:\n  SCCACHE_GHA_ENABLED: 'true'\n", sccache), 1},
		{"the variable on the job", job("", "    env:\n      SCCACHE_GHA_ENABLED: true\n"+sccache), 1},
		{"the variable on the step", job("", sccache+"        env:\n          SCCACHE_GHA_ENABLED: 'true'\n"), 1},
		{"the variable switched off", job("env:\n  SCCACHE_GHA_ENABLED: 'false'\n", sccache), 0},
		{"the variable is 0", job("env:\n  SCCACHE_GHA_ENABLED: '0'\n", sccache), 0},
		{"the step switches off what the workflow switches on", job("env:\n  SCCACHE_GHA_ENABLED: 'true'\n", sccache+"        env:\n          SCCACHE_GHA_ENABLED: 'false'\n"), 0},
		{"the job switches off what the workflow switches on", job("env:\n  SCCACHE_GHA_ENABLED: 'true'\n", "    env:\n      SCCACHE_GHA_ENABLED: '0'\n"+sccache), 0},
		{"the step switches on what the job switches off", job("", "    env:\n      SCCACHE_GHA_ENABLED: 'false'\n"+sccache+"        env:\n          SCCACHE_GHA_ENABLED: 'true'\n"), 1},
		{"maturin with an unknown cache and a condition on sccache", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          cache: ${{ vars.C }}\n          sccache: ${{ !startsWith(github.ref, 'refs/tags/') }}\n"), 1},
		{"maturin with cache none and a condition on sccache", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          cache: none\n          sccache: ${{ !startsWith(github.ref, 'refs/tags/') }}\n"), 0},
		{"maturin with another cache and a condition on sccache", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          cache: kache\n          sccache: ${{ !startsWith(github.ref, 'refs/tags/') }}\n"), 0},
		{"maturin with a cache that is false on tags", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          cache: ${{ startsWith(github.ref, 'refs/heads/') && 'sccache' }}\n          sccache: ${{ !startsWith(github.ref, 'refs/tags/') }}\n"), 0},
		{"maturin with cache sccache and a condition on sccache", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          cache: sccache\n          sccache: ${{ !startsWith(github.ref, 'refs/tags/') }}\n"), 1},
		{"an expression is not known to be false", job("env:\n  SCCACHE_GHA_ENABLED: ${{ vars.SCCACHE }}\n", sccache), 1},
		{"set by a script", job("", sccache+"      - run: echo \"SCCACHE_GHA_ENABLED=true\" >> \"$GITHUB_ENV\"\n"), 1},
		{"maturin with sccache", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          sccache: 'true'\n"), 1},
		{"maturin without sccache", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n"), 0},
		{"maturin with sccache false", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          sccache: false\n"), 0},
		{"maturin with the cache input", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          cache: sccache\n"), 1},
		{"maturin with another cache", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          cache: none\n"), 0},
		{"maturin with sccache for the tags only", job("", "    steps:\n      - uses: PyO3/maturin-action@v1\n        with:\n          sccache: ${{ !startsWith(github.ref, 'refs/tags/') }}\n"), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, false, tc.src); got != tc.want {
				t.Errorf("want %d findings but got %d\n%s", tc.want, got, tc.src)
			}
		})
	}
}

func TestCachePoisoningBuildxNeedsAVersion(t *testing.T) {
	const on = "on:\n  push:\n    tags: ['v*']\n"
	job := func(with string) string {
		return on + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/setup-buildx-action@v3\n" + with
	}
	tests := []struct {
		name     string
		src      string
		pedantic bool
		want     int
	}{
		{"no version: the runner has buildx", job(""), false, 0},
		{"no version, pedantic: a runner may not", job(""), true, 1},
		{"a version", job("        with:\n          version: v0.19.0\n"), false, 1},
		{"a version that is an expression", job("        with:\n          version: ${{ env.BUILDX }}\n"), false, 1},
		{"an empty version", job("        with:\n          version: ''\n"), false, 0},
		{"version and no binary cache", job("        with:\n          version: latest\n          cache-binary: false\n"), false, 0},
		{"pedantic and no binary cache", job("        with:\n          cache-binary: false\n"), true, 0},
		{"pedantic and v2", strings.Replace(job(""), "@v3", "@v2", 1), true, 0},
		{"pedantic and a commit with a v2 comment", strings.Replace(job(""), "@v3", "@8f4e6a1b2c3d4e5f60718293a4b5c6d7e8f90a1b # v2.10.0", 1), true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, tc.pedantic, tc.src); got != tc.want {
				t.Errorf("want %d findings but got %d", tc.want, got)
			}
		})
	}
}

func TestCachePoisoningConditionsThatAreNotAboutTheTrigger(t *testing.T) {
	const on = "on:\n  push:\n    tags: ['v*']\n"
	job := func(cond string) string {
		return on + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    if: " + cond + "\n    steps:\n      - uses: Swatinem/rust-cache@v2\n"
	}
	tests := []struct {
		name string
		cond string
		want int
	}{
		{"a repository and a tag name that does not match", "github.repository == 'o/r' && !startsWith(github.ref_name, 'go/fory')", 1},
		{"the event is known, another job decides the rest", "github.event_name == 'push' && !contains(needs.release.outputs.version, '-')", 1},
		{"another job decides, the event keeps the tag away", "github.event_name == 'pull_request' && needs.a.outputs.b == 'true'", 0},
		{"the event keeps the tag away and the rest is unknown", "needs.a.outputs.b == 'true' && github.event_name == 'workflow_dispatch'", 0},
		{"a branch only", "github.repository == 'o/r' && github.ref == 'refs/heads/main'", 0},
		{"the part about the trigger is not known", "github.ref == format('refs/tags/{0}', inputs.tag)", 0},
		{"the part about the trigger is not known, next to a known one", "github.event_name == 'push' && github.ref_name == inputs.name", 0},
		{"no trigger in it", "github.repository == 'o/r'", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, false, job(tc.cond)); got != tc.want {
				t.Errorf("want %d findings but got %d\n%s", tc.want, got, job(tc.cond))
			}
		})
	}
}

func TestCachePoisoningPedanticReleases(t *testing.T) {
	const job = "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n"
	tests := []struct {
		name string
		src  string
		def  int // findings without the option
		ped  int // findings with the option
	}{
		{"a check that a tag starts", "on:\n  push:\n    tags: ['v*']\npermissions: read-all\n" + job, 0, 1},
		{"a tag and write access", "on:\n  push:\n    tags: ['v*']\npermissions:\n  contents: write\n" + job, 1, 1},
		{"main and a release branch", "on:\n  push:\n    branches: [main, 'release/**']\n" + job, 0, 1},
		{"main and a release branch, read-only", "on:\n  push:\n    branches: [main, 'release/**']\npermissions: read-all\n" + job, 0, 1},
		{"release branches only", "on:\n  push:\n    branches: ['release/**']\n" + job, 1, 1},
		{"main only", "on:\n  push:\n    branches: [main]\n" + job, 0, 0},
		{"a negative pattern does not name a release branch", "on:\n  push:\n    branches: [main, '!release/**']\n" + job, 0, 0},
		{"a name that has release in it", "on:\n  push:\n    branches: [main, 'prerelease-*']\n" + job, 0, 1},
		{"no branch filter", "on: push\n" + job, 0, 0},
		{"a pull request", "on:\n  pull_request:\n    branches: [main, 'release/**']\n" + job, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, false, tc.src); got != tc.def {
				t.Errorf("without the option: want %d findings but got %d", tc.def, got)
			}
			if got := cacheCount(t, true, tc.src); got != tc.ped {
				t.Errorf("with the option: want %d findings but got %d", tc.ped, got)
			}
		})
	}
}

func TestCachePoisoningTagPatternDecidesTheConditionOfTheName(t *testing.T) {
	job := func(tags, cond string) string {
		return "on:\n  push:\n    tags: [" + tags + "]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: " + cond + "\n    steps:\n      - uses: Swatinem/rust-cache@v2\n"
	}
	tests := []struct {
		name string
		tags string
		cond string
		want int
	}{
		{"a prefix that is not v", "'preview-*'", "startsWith(github.ref, 'refs/tags/preview-')", 1},
		{"the name of the tag", "'preview-*'", "startsWith(github.ref_name, 'preview-')", 1},
		{"another prefix keeps it away", "'preview-*'", "startsWith(github.ref, 'refs/tags/v')", 0},
		{"v as before", "'v*'", "startsWith(github.ref, 'refs/tags/v')", 1},
		{"every tag", "'**'", "startsWith(github.ref, 'refs/tags/v')", 1},
		{"a pattern with a character class leaves v1.0.0", "'v[0-9]*'", "startsWith(github.ref, 'refs/tags/v')", 1},
		{"a negative pattern first", "'!v1', 'rel-*'", "startsWith(github.ref_name, 'rel-')", 1},
		{"an exact name", "release", "github.ref_name == 'release'", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, false, job(tc.tags, tc.cond)); got != tc.want {
				t.Errorf("want %d findings but got %d\n%s", tc.want, got, job(tc.tags, tc.cond))
			}
		})
	}
}

func TestCachePoisoningEveryTagPatternIsARun(t *testing.T) {
	src := "on:\n  push:\n    tags: ['sdk-v*', 'cli-v*']\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n        if: startsWith(github.ref_name, 'cli-v')\n"
	if got := cacheCount(t, false, src); got != 1 {
		t.Errorf("a condition that one of the patterns satisfies: want 1 finding but got %d", got)
	}
	src = strings.Replace(src, "'cli-v'", "'other-'", 1)
	if got := cacheCount(t, false, src); got != 0 {
		t.Errorf("a condition that none of the patterns satisfies: want 0 findings but got %d", got)
	}
}

func TestCachePoisoningHandoffFollowsTheConditions(t *testing.T) {
	const head = "on:\n  push:\n    tags: ['v*']\n  workflow_dispatch:\npermissions:\n  contents: read\n"
	build := func(uploadIf string) string {
		return "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n      - uses: actions/upload-artifact@v4\n" + uploadIf + "        with:\n          name: d\n          path: d\n"
	}
	publish := func(cond string) string {
		return "  publish:\n    needs: build\n" + cond + "    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n"
	}
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"both run on tags", head + build("") + publish(""), 1},
		{"the publisher only runs on a dispatch", head + build("") + publish("    if: github.event_name == 'workflow_dispatch'\n"), 0},
		{"the publisher runs on tags", head + build("") + publish("    if: startsWith(github.ref, 'refs/tags/')\n"), 1},
		{"a skipped test, then a release that runs always", head + build("") + "  test:\n    needs: build\n    if: github.event_name == 'workflow_dispatch'\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n  publish:\n    needs: test\n    if: always()\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n", 1},
		{"a skipped test, then a release without a status function", head + build("") + "  test:\n    needs: build\n    if: github.event_name == 'workflow_dispatch'\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n  publish:\n    needs: test\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - run: echo\n", 0},
		{"the upload is skipped on tags", head + build("        if: github.event_name == 'workflow_dispatch'\n") + publish(""), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, false, tc.src); got != tc.want {
				t.Errorf("want %d findings but got %d", tc.want, got)
			}
		})
	}
}

func TestCachePoisoningManyTagPatterns(t *testing.T) {
	src := "on:\n  push:\n    tags: ['a-*', 'b-*', 'c-*', 'd-*', 'e-*', 'f-*', 'g-*', 'h-*', 'i-*', 'v*']\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n        if: startsWith(github.ref_name, 'v')\n"
	if got := cacheCount(t, false, src); got != 1 {
		t.Errorf("want 1 finding but got %d", got)
	}
}

func TestCachePoisoningSccacheInACompositeAction(t *testing.T) {
	// the variable may come from the workflow that calls the action, which is not known: the finding stays, unless the
	// step of the action switches it off
	const step = "    - uses: Mozilla-Actions/sccache-action@v0.0.9\n"
	action := func(env string) string {
		return "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n" + step + env
	}
	const caller = "on:\n  push:\n    tags: ['v*']\nenv:\n  SCCACHE_GHA_ENABLED: 'true'\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/x\n"
	tests := []struct {
		name string
		env  string
		want int
	}{
		{"the caller sets it", "", 1},
		{"the step switches it off", "      env:\n        SCCACHE_GHA_ENABLED: 'false'\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeProject(t, map[string]string{
				".github/workflows/ci.yml":     caller,
				".github/actions/x/action.yml": action(tc.env),
			})
			l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  cache-poisoning: warn\n")
			errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "actions", "x", "action.yml")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := policyErrorsOf(errs, "cache-poisoning"); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

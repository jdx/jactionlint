package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// The pedantic finding of unlocked-install about the release age of the packages that an install resolves.
func TestUnlockedInstallReleaseAge(t *testing.T) {
	wf := func(head, step string) string {
		return "name: CI\non: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\n" + head +
			"jobs:\n  build:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n" + step
	}
	run := func(c string) string { return "      - run: " + c + "\n" }
	tests := []struct {
		what  string
		wf    string
		files map[string]string // files of the repository
		want  int
	}{
		// what is reported
		{"pip install", wf("", run("pip install requests")), nil, 1},
		{"pip install of a pinned package", wf("", run("pip install requests==2.32.3")), nil, 1},
		{"python -m pip", wf("", run("python -m pip install -r requirements.txt")), nil, 1},
		{"pip install of the project", wf("", run("pip install -e .")), nil, 1},
		{"uv pip install", wf("", run("uv pip install ruff")), nil, 1},
		{"uv tool install", wf("", run("uv tool install ruff")), nil, 1},
		{"uv add", wf("", run("uv add requests")), nil, 1},
		{"uvx", wf("", run("uvx ruff check")), nil, 1},
		{"pipx run", wf("", run("pipx run black .")), nil, 1},
		{"npm install of a package", wf("", run("npm install left-pad")), nil, 1},
		{"npm global install", wf("", run("npm install -g pnpm@9.0.0")), nil, 1},
		{"npm install without a lock file", wf("", run("npm install")), map[string]string{"package.json": "{}"}, 1},
		{"yarn add", wf("", run("yarn add left-pad")), nil, 1},
		{"yarn 1", wf("", run("yarn install")), map[string]string{"package.json": `{"packageManager": "yarn@1.22.22"}`}, 1},
		{"pnpm add", wf("", run("pnpm add left-pad")), nil, 1},
		{"pnpm install without a lock file", wf("", run("pnpm install")), nil, 1},
		{"bun add", wf("", run("bun add left-pad")), nil, 1},

		// installs that resolve nothing
		{"npm ci", wf("", run("npm ci")), nil, 0},
		{"npm install with the lock file", wf("", run("npm install")), map[string]string{"package-lock.json": "{}"}, 0},
		{"yarn with the lock file", wf("", run("yarn install")), map[string]string{"yarn.lock": ""}, 0},
		{"pnpm with the lock file", wf("", run("pnpm install")), map[string]string{"pnpm-lock.yaml": ""}, 0},
		{"frozen lock file", wf("", run("pnpm install --frozen-lockfile")), nil, 0},
		{"yarn immutable", wf("", run("yarn install --immutable")), nil, 0},
		{"uv sync", wf("", run("uv sync")), nil, 0},
		{"pip with hashes", wf("", run("pip install --require-hashes -r requirements.txt")), nil, 0},
		{"pip with constraints", wf("", run("pip install -r requirements.txt -c constraints.txt")), nil, 0},
		{"npx", wf("", run("npx prettier --check .")), nil, 0},
		{"cargo", wf("", run("cargo install cargo-edit")), nil, 0},
		{"npm run", wf("", run("npm run build")), nil, 0},

		// the command line
		{"uv --exclude-newer", wf("", run(`uv pip install --exclude-newer "7 days" ruff`)), nil, 0},
		{"uv --exclude-newer=", wf("", run("uv pip install --exclude-newer=2026-01-01 ruff")), nil, 0},
		{"uvx --exclude-newer", wf("", run("uvx --exclude-newer 2026-01-01 ruff")), nil, 0},
		{"pip --uploaded-prior-to", wf("", run("pip install --uploaded-prior-to P7D requests")), nil, 0},
		{"pipx --pip-args", wf("", run(`pipx install --pip-args="--uploaded-prior-to P7D" black`)), nil, 0},
		{"npm --min-release-age", wf("", run("npm install --min-release-age=7 left-pad")), nil, 0},
		{"npm --before", wf("", run("npm install --before=2026-01-01 left-pad")), nil, 0},

		// the workflow
		{"UV_EXCLUDE_NEWER", wf("env:\n  UV_EXCLUDE_NEWER: 7 days\n", run("uv pip install ruff")), nil, 0},
		{"PIP_UPLOADED_PRIOR_TO", wf("env:\n  PIP_UPLOADED_PRIOR_TO: P7D\n", run("pip install requests")), nil, 0},
		{"npm_config_min_release_age", wf("env:\n  npm_config_min_release_age: 7\n", run("npm install left-pad")), nil, 0},
		{"NPM_CONFIG_MIN_RELEASE_AGE", wf("env:\n  NPM_CONFIG_MIN_RELEASE_AGE: 7\n", run("npm install left-pad")), nil, 0},
		{"env of the step", wf("", "      - run: npm install left-pad\n        env:\n          npm_config_before: 2026-01-01\n"), nil, 0},
		{"npm config set before", wf("", run("npm config set min-release-age 7")+run("npm install left-pad")), nil, 0},
		{"a later npm config set", wf("", run("npm install left-pad")+run("npm config set min-release-age 7")), nil, 1},
		{"yarn env", wf("env:\n  YARN_NPM_MINIMAL_AGE_GATE: 3d\n", run("yarn add left-pad")), nil, 0},

		// the files of the repository
		{"uv.toml", wf("", run("uv pip install ruff")), map[string]string{"uv.toml": "exclude-newer = \"7 days\"\n"}, 0},
		{"uv.toml [pip]", wf("", run("uv pip install ruff")), map[string]string{"uv.toml": "[pip]\nexclude-newer = \"2026-01-01T00:00:00Z\"\n"}, 0},
		{"uv.toml false", wf("", run("uv pip install ruff")), map[string]string{"uv.toml": "exclude-newer = false\n"}, 1},
		{"pyproject [tool.uv]", wf("", run("uv add ruff")), map[string]string{"pyproject.toml": "[project]\nname = \"x\"\n\n[tool.uv]\nexclude-newer = \"P7D\" # a week\n"}, 0},
		{"pyproject [tool.uv.pip]", wf("", run("uv pip install ruff")), map[string]string{"pyproject.toml": "[tool.uv.pip]\nexclude-newer = \"P7D\"\n"}, 0},
		{"pyproject another table", wf("", run("uv add ruff")), map[string]string{"pyproject.toml": "[tool.other]\nexclude-newer = \"P7D\"\n"}, 1},
		{"pyproject does not help pip", wf("", run("pip install ruff")), map[string]string{"pyproject.toml": "[tool.uv]\nexclude-newer = \"P7D\"\n"}, 1},
		{"uv.toml of the working directory", wf("", "      - run: uv pip install ruff\n        working-directory: py\n"), map[string]string{"py/uv.toml": "exclude-newer = \"7 days\"\n"}, 0},
		{"uv.toml of another directory", wf("", "      - run: uv pip install ruff\n        working-directory: py\n"), map[string]string{"other/uv.toml": "exclude-newer = \"7 days\"\n"}, 1},
		{"uv --project", wf("", run("uv pip install --project py ruff")), map[string]string{"py/uv.toml": "exclude-newer = \"7 days\"\n"}, 0},
		{"after a cd", wf("", run("cd py && uv pip install ruff")), nil, 0},
		{".npmrc min-release-age", wf("", run("npm install left-pad")), map[string]string{".npmrc": "min-release-age=7\n"}, 0},
		{".npmrc min-release-age 0", wf("", run("npm install left-pad")), map[string]string{".npmrc": "min-release-age=0\n"}, 1},
		{".npmrc before", wf("", run("npm install left-pad")), map[string]string{".npmrc": "before=2026-01-01\n"}, 0},
		{".npmrc for bun", wf("", run("bun add left-pad")), map[string]string{".npmrc": "min-release-age=7\n"}, 0},
		{"pnpm-workspace.yaml", wf("", run("pnpm add left-pad")), map[string]string{"pnpm-workspace.yaml": "packages:\n  - a\nminimumReleaseAge: 1440\n"}, 0},
		{"pnpm-workspace.yaml 0", wf("", run("pnpm add left-pad")), map[string]string{"pnpm-workspace.yaml": "minimumReleaseAge: 0\n"}, 1},
		{"pnpm 11 waits a day by default", wf("", run("pnpm add left-pad")), map[string]string{"package.json": `{"packageManager": "pnpm@11.0.0"}`}, 0},
		{"pnpm 10", wf("", run("pnpm add left-pad")), map[string]string{"package.json": `{"packageManager": "pnpm@10.16.0"}`}, 1},
		{"pnpm 11 from the action", wf("", "      - uses: pnpm/action-setup@v4\n        with:\n          version: 11\n      - run: pnpm add left-pad\n"), nil, 0},
		{"pnpm 11 from a script", wf("", run("corepack prepare pnpm@11.0.0 --activate")+run("pnpm add left-pad")), nil, 0},
		{"bunfig", wf("", run("bun add left-pad")), map[string]string{"bunfig.toml": "[install]\nminimumReleaseAge = 259200\n"}, 0},
		{"yarnrc gate", wf("", run("yarn add left-pad")), map[string]string{".yarnrc.yml": "npmMinimalAgeGate: 3d\n"}, 0},
		{"yarnrc gate numeric", wf("", run("yarn add left-pad")), map[string]string{".yarnrc.yml": "npmMinimalAgeGate: 1440\n"}, 0},
		{"yarnrc gate 0", wf("", run("yarn add left-pad")), map[string]string{".yarnrc.yml": "npmMinimalAgeGate: 0\n", "package.json": `{"packageManager": "yarn@4.15.0"}`}, 1},
		{"yarnrc gate 0m", wf("", run("yarn add left-pad")), map[string]string{".yarnrc.yml": "npmMinimalAgeGate: \"0m\"\n"}, 1},
		{"yarn 4.15 waits a day by default", wf("", run("yarn add left-pad")), map[string]string{"package.json": `{"packageManager": "yarn@4.15.0"}`}, 0},
		{"yarn 4.9", wf("", run("yarn add left-pad")), map[string]string{"package.json": `{"packageManager": "yarn@4.9.0"}`, ".yarnrc.yml": "nodeLinker: node-modules\n"}, 1},
		{"yarn 2+ of an unknown version", wf("", run("yarn add left-pad")), map[string]string{".yarnrc.yml": "nodeLinker: node-modules\n"}, 0},
		{"the setting of another tool", wf("", run("npm install left-pad")), map[string]string{"pnpm-workspace.yaml": "minimumReleaseAge: 1440\n"}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			root := baselineProject(t, tc.wf, "profile: default\nrules:\n  unlocked-install:\n    pedantic: true\n")
			for name, content := range tc.files {
				writeTestFile(t, filepath.Join(root, filepath.FromSlash(name)), content)
			}
			_, out, _ := baselineCmd(t)
			if got := strings.Count(out, "however new"); got != tc.want {
				t.Errorf("want %d findings, got %d:\n%s", tc.want, got, out)
			}
		})
	}

	t.Run("not reported without the pedantic option", func(t *testing.T) {
		baselineProject(t, wf("", run("pip install requests")), "profile: default\n")
		if _, out, _ := baselineCmd(t); strings.Contains(out, "however new") {
			t.Errorf("reported under the default profile:\n%s", out)
		}
	})

	// A workflow linted on its own: no file tells about a cooldown, and a manifest install may have a lock file
	t.Run("without a project", func(t *testing.T) {
		src := "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: pip install requests\n      - run: npm install\n      - run: npm install left-pad\n"
		l, err := NewLinter(io.Discard, &LinterOptions{})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = &Config{Profile: ProfilePedantic}
		errs, err := l.Lint("test.yaml", []byte(src), nil)
		if err != nil {
			t.Fatal(err)
		}
		var lines []int
		for _, e := range errs {
			if e.ID == "unlocked-install" && strings.Contains(e.Message, "however new") {
				lines = append(lines, e.Line)
			}
		}
		if len(lines) != 2 || lines[0] != 6 || lines[1] != 8 {
			t.Errorf("want findings at lines 6 and 8, got %v", lines)
		}
	})
}

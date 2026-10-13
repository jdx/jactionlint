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
	block := func(lines ...string) string {
		return "      - run: |\n          " + strings.Join(lines, "\n          ") + "\n"
	}
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
		{"a later setting in the same script", wf("", block("npm install left-pad", "npm config set min-release-age 7")), nil, 1},
		{"an earlier setting in the same script", wf("", block("npm config set min-release-age 7", "npm install left-pad")), nil, 0},

		// the environment variable of another tool does nothing
		{"UV_EXCLUDE_NEWER for npm", wf("env:\n  UV_EXCLUDE_NEWER: 7 days\n", run("npm install left-pad")), nil, 1},
		{"PIP_UPLOADED_PRIOR_TO for yarn", wf("env:\n  PIP_UPLOADED_PRIOR_TO: P7D\n", run("yarn add left-pad")), nil, 1},
		{"npm_config_min_release_age for uv", wf("env:\n  npm_config_min_release_age: 7\n", run("uv pip install ruff")), nil, 1},
		{"YARN_NPM_MINIMAL_AGE_GATE for pip", wf("env:\n  YARN_NPM_MINIMAL_AGE_GATE: 3d\n", run("pip install requests")), nil, 1},
		{"UV_EXCLUDE_NEWER for uvx", wf("env:\n  UV_EXCLUDE_NEWER: 7 days\n", run("uvx ruff check")), nil, 0},
		{"PIP_UPLOADED_PRIOR_TO for pipx", wf("env:\n  PIP_UPLOADED_PRIOR_TO: P7D\n", run("pipx install black")), nil, 0},
		{"npm_config_before for pnpm", wf("env:\n  npm_config_before: 2026-01-01\n", run("pnpm add left-pad")), nil, 1},
		{"pnpm env", wf("env:\n  pnpm_config_minimum_release_age: 1440\n", run("pnpm add left-pad")), nil, 0},

		// a script that mentions an option of another tool or a command that is not an install
		{"git log --before", wf("", run("git log --before=2.weeks.ago")+run("pip install requests")), nil, 1},
		{"git log --before in the same script", wf("", block("git log --before=1.week.ago", "npm install left-pad")), nil, 1},
		{"a script of another tool", wf("", run("echo uv pip install --exclude-newer 7d")+run("npm install left-pad")), nil, 1},

		// a value that switches it off is no cooldown
		{"min age 0", wf("env:\n  npm_config_min_release_age: 0\n", run("npm install left-pad")), nil, 1},
		{"yarn gate 0", wf("env:\n  YARN_NPM_MINIMAL_AGE_GATE: 0\n", run("yarn add left-pad")), nil, 1},
		{"yarn gate 0m", wf("env:\n  YARN_NPM_MINIMAL_AGE_GATE: 0m\n", run("yarn add left-pad")), nil, 1},
		{"uv false", wf("env:\n  UV_EXCLUDE_NEWER: 'false'\n", run("uv pip install ruff")), nil, 1},
		{"empty", wf("env:\n  PIP_UPLOADED_PRIOR_TO: ''\n", run("pip install requests")), nil, 1},
		{"an expression", wf("env:\n  npm_config_min_release_age: ${{ vars.AGE }}\n", run("npm install left-pad")), nil, 0},
		{"the nearest scope decides: off", wf("env:\n  npm_config_min_release_age: 7\n", "      - run: npm install left-pad\n        env:\n          npm_config_min_release_age: 0\n"), nil, 1},
		{"the nearest scope decides: on", wf("env:\n  npm_config_min_release_age: 0\n", "      - run: npm install left-pad\n        env:\n          npm_config_min_release_age: 7\n"), nil, 0},

		// the last assignment wins, inside a script, across steps and among the options
		{"script: on then off", wf("", block("npm config set min-release-age 7", "npm config set min-release-age 0", "npm install left-pad")), nil, 1},
		{"script: off then on", wf("", block("npm config set min-release-age 0", "npm config set min-release-age 7", "npm install left-pad")), nil, 0},
		{"steps: on then off", wf("", run("npm config set min-release-age 7")+run("npm config set min-release-age 0")+run("npm install left-pad")), nil, 1},
		{"steps: off then on", wf("", run("npm config set min-release-age 0")+run("npm config set min-release-age 7")+run("npm install left-pad")), nil, 0},
		{"steps: on, then off in the step", wf("", run("npm config set min-release-age 7")+block("npm config set min-release-age 0", "npm install left-pad")), nil, 1},
		{"options: on then off", wf("", run("npm install --min-release-age=7 --min-release-age=0 left-pad")), nil, 1},
		{"options: off then on", wf("", run("npm install --min-release-age=0 --min-release-age=7 left-pad")), nil, 0},
		{"uv options: on then off", wf("", run("uv pip install --exclude-newer 7d --exclude-newer false ruff")), nil, 1},
		{"yarn script: on then off", wf("", run("yarn config set npmMinimalAgeGate 3d")+run("yarn config set npmMinimalAgeGate 0")+run("yarn add left-pad")), nil, 1},
		{"pip script: off then on", wf("", run("export PIP_UPLOADED_PRIOR_TO=0")+run("pip install --uploaded-prior-to P7D requests")), nil, 0},
		{".npmrc: the last line wins", wf("", run("npm install left-pad")), map[string]string{".npmrc": "min-release-age=7\nmin-release-age=0\n"}, 1},
		{"pnpm-workspace.yaml: the last line wins", wf("", run("pnpm add left-pad")), map[string]string{"pnpm-workspace.yaml": "minimumReleaseAge: 1440\nminimumReleaseAge: 0\n"}, 1},
		{"bunfig: the last line wins", wf("", run("bun add left-pad")), map[string]string{"bunfig.toml": "[install]\nminimumReleaseAge = 100\nminimumReleaseAge = 0\n"}, 1},
		{"pyproject: the last line wins", wf("", run("uv add ruff")), map[string]string{"pyproject.toml": "[tool.uv]\nexclude-newer = \"P7D\"\nexclude-newer = \"false\"\n"}, 1},
		// npm lets before win over min-release-age inside one place
		{"npm: before wins over a zero age", wf("", run("npm install --min-release-age=0 --before=2026-01-01 left-pad")), nil, 0},
		{"npm: both off", wf("", run("npm install --min-release-age=0 --before= left-pad")), nil, 1},
		{"npm: before cleared later", wf("", run("npm install --min-release-age=7 --before=2026-01-01 --before= left-pad")), nil, 0},
		{"npm: both cleared later", wf("", run("npm install --min-release-age=7 --before=2026-01-01 --min-release-age=0 --before= left-pad")), nil, 1},

		// the order of the places: file < workflow env < job env < step env < earlier script < options
		{"file on, workflow env off", wf("env:\n  npm_config_min_release_age: 0\n", run("npm install left-pad")), map[string]string{".npmrc": "min-release-age=7\n"}, 1},
		{"file off, workflow env on", wf("env:\n  npm_config_min_release_age: 7\n", run("npm install left-pad")), map[string]string{".npmrc": "min-release-age=0\n"}, 0},
		{"job env off over workflow env on", "name: CI\non: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\nenv:\n  npm_config_min_release_age: 7\njobs:\n  build:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    env:\n      npm_config_min_release_age: 0\n    steps:\n" + run("npm install left-pad"), nil, 1},
		{"step env on over job env off", "name: CI\non: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  build:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    env:\n      npm_config_min_release_age: 0\n    steps:\n      - run: npm install left-pad\n        env:\n          npm_config_min_release_age: 7\n", nil, 0},
		{"env on, script off", wf("env:\n  npm_config_min_release_age: 7\n", run("npm config set min-release-age 0")+run("npm install left-pad")), nil, 1},
		{"env off, script on", wf("env:\n  npm_config_min_release_age: 0\n", run("npm config set min-release-age 7")+run("npm install left-pad")), nil, 0},
		{"script off, option on", wf("", run("npm config set min-release-age 0")+run("npm install --min-release-age=7 left-pad")), nil, 0},
		{"script on, option off", wf("", run("npm config set min-release-age 7")+run("npm install --min-release-age=0 left-pad")), nil, 1},
		{"env on, option off", wf("env:\n  npm_config_min_release_age: 7\n", run("npm install --min-release-age=0 left-pad")), nil, 1},
		{"file on, option off", wf("", run("npm install --min-release-age=0 left-pad")), map[string]string{".npmrc": "min-release-age=7\n"}, 1},
		{"file on, script off, option nothing", wf("", run("npm config set min-release-age 0")+run("npm install left-pad")), map[string]string{".npmrc": "min-release-age=7\n"}, 1},
		{"pnpm 11 default, env off", wf("env:\n  pnpm_config_minimum_release_age: 0\n", run("pnpm add left-pad")), map[string]string{"package.json": `{"packageManager": "pnpm@11.0.0"}`}, 1},
		{"pnpm 11 default, file off", wf("", run("pnpm add left-pad")), map[string]string{"package.json": `{"packageManager": "pnpm@11.0.0"}`, "pnpm-workspace.yaml": "minimumReleaseAge: 0\n"}, 1},
		{"pnpm 11 default, option off", wf("", run("pnpm add --config.minimumReleaseAge=0 left-pad")), map[string]string{"package.json": `{"packageManager": "pnpm@11.0.0"}`}, 1},
		{"yarn 4.15 default, file off", wf("", run("yarn add left-pad")), map[string]string{"package.json": `{"packageManager": "yarn@4.15.0"}`, ".yarnrc.yml": "npmMinimalAgeGate: 0\n"}, 1},
		{"yarn of an unknown version, file off", wf("", run("yarn add left-pad")), map[string]string{".yarnrc.yml": "npmMinimalAgeGate: 0m\n"}, 1},
		{"uv.toml wins over pyproject", wf("", run("uv add ruff")), map[string]string{"pyproject.toml": "[tool.uv]\nexclude-newer = \"P7D\"\n", "uv.toml": "exclude-newer = false\n"}, 1},

		// zero durations in every spelling are no cooldown
		{"--exclude-newer 0 days", wf("", run(`uv pip install --exclude-newer "0 days" ruff`)), nil, 1},
		{"--exclude-newer P0D", wf("", run("uv pip install --exclude-newer P0D ruff")), nil, 1},
		{"--exclude-newer PT0S", wf("", run("uv pip install --exclude-newer=PT0S ruff")), nil, 1},
		{"--exclude-newer 0h", wf("", run("uv pip install --exclude-newer 0h ruff")), nil, 1},
		{"--exclude-newer 7 days", wf("", run(`uv pip install --exclude-newer "7 days" ruff`)), nil, 0},
		{"--exclude-newer P7D", wf("", run("uv pip install --exclude-newer P7D ruff")), nil, 0},
		{"--uploaded-prior-to P0D", wf("", run("pip install --uploaded-prior-to P0D requests")), nil, 1},
		{"--uploaded-prior-to 0 days", wf("", run(`pip install --uploaded-prior-to "0 days" requests`)), nil, 1},
		{"--uploaded-prior-to P3D", wf("", run("pip install --uploaded-prior-to P3D requests")), nil, 0},
		{"UV_EXCLUDE_NEWER P0D", wf("env:\n  UV_EXCLUDE_NEWER: P0D\n", run("uv pip install ruff")), nil, 1},
		{"uv.toml 0 days", wf("", run("uv pip install ruff")), map[string]string{"uv.toml": "exclude-newer = \"0 days\"\n"}, 1},
		{"pyproject P0D", wf("", run("uv add ruff")), map[string]string{"pyproject.toml": "[tool.uv]\nexclude-newer = \"P0D\"\n"}, 1},
		{"script PT0S", wf("", run("export UV_EXCLUDE_NEWER=PT0S")+run("uv pip install ruff")), nil, 1},

		// an install that is told not to freeze the lock file resolves
		{"pnpm --frozen-lockfile=false", wf("", run("pnpm install --frozen-lockfile=false")), map[string]string{"pnpm-lock.yaml": ""}, 1},
		{"pnpm --no-frozen-lockfile", wf("", run("pnpm install --no-frozen-lockfile")), map[string]string{"pnpm-lock.yaml": ""}, 1},
		{"pnpm --frozen-lockfile=true", wf("", run("pnpm install --frozen-lockfile=true")), map[string]string{"pnpm-lock.yaml": ""}, 0},
		{"yarn add --frozen-lockfile=false", wf("", run("yarn add left-pad --frozen-lockfile=false")), nil, 1},
		{"yarn --immutable=false", wf("", run("yarn install --immutable=false")), map[string]string{"yarn.lock": ""}, 1},

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
		{".npmrc does not give bun an age", wf("", run("bun add left-pad")), map[string]string{".npmrc": "min-release-age=7\n"}, 1},
		{"pnpm-workspace.yaml", wf("", run("pnpm add left-pad")), map[string]string{"pnpm-workspace.yaml": "packages:\n  - a\nminimumReleaseAge: 1440\n"}, 0},
		{"pnpm-workspace.yaml 0", wf("", run("pnpm add left-pad")), map[string]string{"pnpm-workspace.yaml": "minimumReleaseAge: 0\n"}, 1},
		{"pnpm 11 waits a day by default", wf("", run("pnpm add left-pad")), map[string]string{"package.json": `{"packageManager": "pnpm@11.0.0"}`}, 0},
		{"pnpm 10", wf("", run("pnpm add left-pad")), map[string]string{"package.json": `{"packageManager": "pnpm@10.16.0"}`}, 1},
		{"pnpm 11 from the action", wf("", "      - uses: pnpm/action-setup@v4\n        with:\n          version: 11\n      - run: pnpm add left-pad\n"), nil, 0},
		{"pnpm 11 earlier in the same script", wf("", block("corepack prepare pnpm@11.0.0 --activate", "pnpm add left-pad")), nil, 0},
		{"pnpm 11 later in the same script", wf("", block("pnpm add left-pad", "corepack prepare pnpm@11.0.0 --activate")), nil, 1},

		// the value of an option or of a setting in a script counts: zero, false and none switch the age off
		{"--min-release-age=0", wf("", run("npm install --min-release-age=0 left-pad")), nil, 1},
		{"--min-release-age 0", wf("", run("npm install --min-release-age 0 left-pad")), nil, 1},
		{"--exclude-newer false", wf("", run("uv pip install --exclude-newer false ruff")), nil, 1},
		{"--exclude-newer=false", wf("", run("uv pip install --exclude-newer=false ruff")), nil, 1},
		{"--exclude-newer-package only", wf("", run("uv pip install --exclude-newer-package foo=2026-01-01 ruff")), nil, 1},
		{"--exclude-newer a variable", wf("", run("uv pip install --exclude-newer \"$AGE\" ruff")), nil, 0},
		{"--uploaded-prior-to 0", wf("", run("pip install --uploaded-prior-to 0 requests")), nil, 1},
		{"--before empty", wf("", run("npm install --before= left-pad")), nil, 1},
		{"pnpm --config.minimumReleaseAge=0", wf("", run("pnpm add --config.minimumReleaseAge=0 left-pad")), nil, 1},
		{"pnpm --config.minimumReleaseAge=1440", wf("", run("pnpm add --config.minimumReleaseAge=1440 left-pad")), nil, 0},
		{"pnpm --config.minimumReleaseAgeExclude", wf("", run("pnpm add --config.minimumReleaseAgeExclude=left-pad left-pad")), nil, 1},
		{"bun --minimum-release-age", wf("", run("bun add --minimum-release-age 259200 left-pad")), nil, 0},
		{"bun --minimum-release-age 0", wf("", run("bun add --minimum-release-age 0 left-pad")), nil, 1},
		{"npm config set min-release-age 0", wf("", run("npm config set min-release-age 0")+run("npm install left-pad")), nil, 1},
		{"npm config set min-release-age=0", wf("", run("npm config set min-release-age=0")+run("npm install left-pad")), nil, 1},
		{"npmrc min-release-age 0 by echo", wf("", run(`echo "min-release-age=0" >> .npmrc`)+run("npm install left-pad")), nil, 1},
		{"npm config set before empty", wf("", run("npm config set before ''")+run("npm install left-pad")), nil, 1},
		{"yarn config set gate 0", wf("", run("yarn config set npmMinimalAgeGate 0")+run("yarn add left-pad")), nil, 1},
		{"yarn config set gate", wf("", run("yarn config set npmMinimalAgeGate 3d")+run("yarn add left-pad")), nil, 0},
		{"pipx --pip-args off", wf("", run(`pipx install --pip-args="--uploaded-prior-to 0" black`)), nil, 1},

		// the files of another tool: .npmrc keys of npm are not read by pnpm and bun
		{"pnpm and the .npmrc of npm", wf("", run("pnpm add left-pad")), map[string]string{".npmrc": "min-release-age=7\n"}, 1},
		{"pnpm and before in .npmrc", wf("", run("pnpm add left-pad")), map[string]string{".npmrc": "before=2026-01-01\n"}, 1},
		{"pnpm and minimum-release-age in .npmrc", wf("", run("pnpm add left-pad")), map[string]string{".npmrc": "minimum-release-age=1440\n"}, 0},
		{"npm and minimum-release-age in .npmrc", wf("", run("npm install left-pad")), map[string]string{".npmrc": "minimum-release-age=1440\n"}, 1},
		{"npm min-release-age 0 in .npmrc", wf("", run("npm install left-pad")), map[string]string{".npmrc": "min-release-age=0\n"}, 1},
		{"bun and before in .npmrc", wf("", run("bun add left-pad")), map[string]string{".npmrc": "before=2026-01-01\n"}, 1},

		// pnpm settings that are not the age
		{"pnpm minimumReleaseAgeExclude only", wf("", run("pnpm add left-pad")), map[string]string{"pnpm-workspace.yaml": "minimumReleaseAgeExclude:\n  - left-pad\n"}, 1},
		{"pnpm minimumReleaseAgeStrict only", wf("", run("pnpm add left-pad")), map[string]string{"pnpm-workspace.yaml": "minimumReleaseAgeStrict: true\n"}, 1},
		{"pnpm exclude in a script", wf("", run("pnpm config set minimumReleaseAgeExclude left-pad")+run("pnpm add left-pad")), nil, 1},
		{"pnpm exclude in the environment", wf("env:\n  pnpm_config_minimum_release_age_exclude: left-pad\n", run("pnpm add left-pad")), nil, 1},
		{"pnpm age in a script", wf("", run("pnpm config set minimumReleaseAge 1440")+run("pnpm add left-pad")), nil, 0},

		// uv --no-config reads no file
		{"uv --no-config with uv.toml", wf("", run("uv pip install --no-config ruff")), map[string]string{"uv.toml": "exclude-newer = \"7 days\"\n"}, 1},
		{"uv --no-config with the option", wf("", run("uv pip install --no-config --exclude-newer 7d ruff")), nil, 0},
		{"uv --config-file", wf("", run("uv pip install --config-file other.toml ruff")), nil, 0},
		{"uv --directory", wf("", run("uv add --directory py ruff")), nil, 0},

		// bunfig.toml: only [install]
		{"bunfig another table", wf("", run("bun add left-pad")), map[string]string{"bunfig.toml": "[test]\nminimumReleaseAge = 259200\n"}, 1},
		{"bunfig a subtable of install", wf("", run("bun add left-pad")), map[string]string{"bunfig.toml": "[install.cache]\nminimumReleaseAge = 259200\n"}, 1},
		{"bunfig zero", wf("", run("bun add left-pad")), map[string]string{"bunfig.toml": "[install]\nminimumReleaseAge = 0\n"}, 1},
		{"bunfig excludes only", wf("", run("bun add left-pad")), map[string]string{"bunfig.toml": "[install]\nminimumReleaseAgeExcludes = [\"typescript\"]\n"}, 1},
		{"bunfig after another table", wf("", run("bun add left-pad")), map[string]string{"bunfig.toml": "[test]\nx = 1\n\n[install]\nminimumReleaseAge = 259200\n"}, 0},
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

package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// The pedantic finding of unlocked-install about lifecycle scripts reads the repository to find out whether
// someone already chose.
func TestUnlockedInstallLifecycleScripts(t *testing.T) {
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
		{"npm ci", wf("", run("npm ci")), nil, 1},
		{"npm install", wf("", run("npm install")), nil, 1},
		{"npm install of a package", wf("", run("npm install left-pad@1.3.0")), nil, 1},
		{"npm i", wf("", run("npm i --no-fund")), nil, 1},
		{"yarn", wf("", run("yarn")), nil, 1},
		{"yarn install", wf("", run("yarn install --frozen-lockfile")), nil, 1},
		{"yarn add", wf("", run("yarn add left-pad")), nil, 1},
		{"bun install", wf("", run("bun install --frozen-lockfile")), nil, 1},
		{"bun ci", wf("", run("bun ci")), nil, 1},
		{"after &&", wf("", run("echo hi && npm ci")), nil, 1},

		// the command line decides
		{"--ignore-scripts", wf("", run("npm ci --ignore-scripts")), nil, 0},
		{"--ignore-scripts=true", wf("", run("npm ci --ignore-scripts=true")), nil, 0},
		{"--ignore-scripts=false", wf("", run("npm ci --ignore-scripts=false")), nil, 1},
		{"--no-ignore-scripts", wf("", run("npm ci --ignore-scripts --no-ignore-scripts")), nil, 1},
		{"a flag that is not known", wf("", run("npm ci --ignore-scripts=$IGNORE")), nil, 0},
		{"yarn --ignore-scripts", wf("", run("yarn install --ignore-scripts")), nil, 0},
		{"yarn --mode=skip-build", wf("", run("yarn install --immutable --mode=skip-build")), nil, 0},
		{"yarn --mode skip-build", wf("", run("yarn install --mode skip-build")), nil, 0},
		{"bun --ignore-scripts", wf("", run("bun install --ignore-scripts")), nil, 0},
		{"the lock file only", wf("", run("npm install --package-lock-only")), nil, 0},
		{"dry run", wf("", run("npm install --dry-run")), nil, 0},

		// commands that run no scripts of dependencies, or are not this kind of install
		{"global install", wf("", run("npm install -g typescript")), nil, 0},
		{"npx", wf("", run("npx prettier --check .")), nil, 0},
		{"pnpm", wf("", run("pnpm install --frozen-lockfile")), nil, 0},
		{"npm run", wf("", run("npm run build")), nil, 0},
		{"npm test", wf("", run("npm test")), nil, 0},
		{"pip", wf("", run("pip install requests==2.0.0")), nil, 0},

		// the workflow decides
		{"env of the step", wf("", run("npm ci\n        env:\n          npm_config_ignore_scripts: true")), nil, 0},
		{"env of the workflow, upper case", wf("env:\n  NPM_CONFIG_IGNORE_SCRIPTS: 'true'\n", run("npm ci")), nil, 0},
		{"env set to false", wf("env:\n  npm_config_ignore_scripts: false\n", run("npm ci")), nil, 1},
		{"yarn env", wf("env:\n  YARN_ENABLE_SCRIPTS: false\n", run("yarn install")), nil, 0},
		{"npm config set before", wf("", run("npm config set ignore-scripts true")+run("npm ci")), nil, 0},
		{"npm config set after", wf("", run("npm ci")+run("npm config set ignore-scripts true")), nil, 1},
		{"echo into npmrc", wf("", run(`echo "ignore-scripts=true" >> ~/.npmrc`)+run("npm ci")), nil, 0},
		{"npm config set with =", wf("", run("npm config set ignore-scripts=true")+run("npm ci")), nil, 0},
		{"npm config set underscore", wf("", run("npm config set ignore_scripts true")+run("npm ci")), nil, 0},
		{"overwrite npmrc", wf("", run(`echo ignore-scripts=true > .npmrc`)+run("npm ci")), nil, 0},
		{"tee into npmrc", wf("", run(`echo ignore-scripts=true | tee -a .npmrc`)+run("npm ci")), nil, 0},
		{"heredoc into npmrc", wf("", block("cat >> .npmrc <<EOF", "ignore-scripts=true", "EOF")+run("npm ci")), nil, 0},
		{"exported variable", wf("", block("export npm_config_ignore_scripts=true", "npm ci")), nil, 0},
		{"variable of the command", wf("", run("npm_config_ignore_scripts=true npm ci")), nil, 0},
		{"set to false in a script", wf("", run("npm config set ignore-scripts false")+run("npm ci")), nil, 1},
		// a text that only prints the setting does not set it
		{"echo of the setting", wf("", run(`echo "ignore-scripts=true"`)+run("npm ci")), nil, 1},
		{"echo of the setting into a file", wf("", run(`echo "ignore-scripts=true" > notes.txt`)+run("npm ci")), nil, 1},
		{"config get", wf("", run("npm config get ignore-scripts true")+run("npm ci")), nil, 1},
		// in the script of the install itself only what comes before it counts
		{"set before in the same script", wf("", block("npm config set ignore-scripts true", "npm ci")), nil, 0},
		{"set after in the same script", wf("", block("npm ci", "npm config set ignore-scripts true")), nil, 1},
		{"npmrc after in the same script", wf("", block("npm ci", "echo ignore-scripts=true >> .npmrc")), nil, 1},

		// the files of the repository decide
		{".npmrc ignore-scripts", wf("", run("npm ci")), map[string]string{".npmrc": "ignore-scripts=true\n"}, 0},
		{".npmrc with spaces and quotes", wf("", run("npm ci")), map[string]string{".npmrc": "; comment\nignore-scripts = \"true\"\n"}, 0},
		{".npmrc with underscores", wf("", run("npm ci")), map[string]string{".npmrc": "ignore_scripts=true\n"}, 0},
		{".npmrc without a value", wf("", run("npm ci")), map[string]string{".npmrc": "ignore-scripts\n"}, 0},
		{".npmrc ignore-scripts false", wf("", run("npm ci")), map[string]string{".npmrc": "ignore-scripts=false\n"}, 1},
		{".npmrc with the setting later turned off", wf("", run("npm ci")), map[string]string{".npmrc": "ignore-scripts=true\nignore-scripts=false\n"}, 1},
		{".npmrc commented out", wf("", run("npm ci")), map[string]string{".npmrc": "#ignore-scripts=true\n"}, 1},
		{".npmrc of the working directory", wf("", "      - run: npm ci\n        working-directory: web\n"), map[string]string{"web/.npmrc": "ignore-scripts=true\n"}, 0},
		{".npmrc of another directory", wf("", "      - run: npm ci\n        working-directory: web\n"), map[string]string{"other/.npmrc": "ignore-scripts=true\n"}, 1},
		{".npmrc of the job working directory", "name: CI\non: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  build:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    defaults:\n      run:\n        working-directory: web\n    steps:\n" + run("npm ci"), map[string]string{"web/.npmrc": "ignore-scripts=true\n"}, 0},
		{"--prefix names the directory", wf("", run("npm ci --prefix web")), map[string]string{"web/.npmrc": "ignore-scripts=true\n"}, 0},
		{"cd leaves the directory unknown", wf("", run("cd web && npm ci")), map[string]string{"web/.npmrc": "ignore-scripts=true\n"}, 0},
		{"cd without a config", wf("", run("cd web && npm ci")), map[string]string{"web/.npmrc": "ignore-scripts=false\n"}, 0},
		{"yarn reads .npmrc", wf("", run("yarn install")), map[string]string{".npmrc": "ignore-scripts=true\n"}, 0},
		{"bun reads .npmrc", wf("", run("bun install")), map[string]string{".npmrc": "ignore-scripts=true\n"}, 0},
		{"npm allow list", wf("", run("npm ci")), map[string]string{"package.json": `{"allowScripts": {"esbuild@0.24.2": true}}`}, 0},
		{"bun trustedDependencies", wf("", run("bun install")), map[string]string{"package.json": `{"trustedDependencies": ["esbuild"]}`}, 0},
		{"bun empty trustedDependencies", wf("", run("bun install")), map[string]string{"package.json": `{"trustedDependencies": []}`}, 0},
		{"bun without trustedDependencies", wf("", run("bun install")), map[string]string{"package.json": `{"name": "x"}`}, 1},
		{"bunfig", wf("", run("bun install")), map[string]string{"bunfig.toml": "[install]\nignoreScripts = true\n"}, 0},
		{"trustedDependencies is bun's", wf("", run("npm ci")), map[string]string{"package.json": `{"trustedDependencies": ["esbuild"]}`}, 1},
		{"yarn enableScripts false", wf("", run("yarn install")), map[string]string{".yarnrc.yml": "enableScripts: false\n"}, 0},
		{"yarn enableScripts true", wf("", run("yarn install")), map[string]string{".yarnrc.yml": "enableScripts: true\n"}, 1},
		{"yarn 4.2", wf("", run("yarn install")), map[string]string{".yarnrc.yml": "nodeLinker: node-modules\n", "package.json": `{"packageManager": "yarn@4.2.1+sha256.abc"}`}, 1},
		{"yarn 4.14 defaults to no scripts", wf("", run("yarn install")), map[string]string{".yarnrc.yml": "nodeLinker: node-modules\n", "package.json": `{"packageManager": "yarn@4.14.0"}`}, 0},
		{"yarn 6 defaults to no scripts", wf("", run("yarn install")), map[string]string{"package.json": `{"packageManager": "yarn@6.0.0"}`}, 0},
		{"yarn 1", wf("", run("yarn install")), map[string]string{"package.json": `{"packageManager": "yarn@1.22.22"}`}, 1},
		{"yarn 2+ of an unknown version", wf("", run("yarn install")), map[string]string{".yarnrc.yml": "nodeLinker: node-modules\n"}, 1},
		{"yarn 3", wf("", run("yarn install")), map[string]string{".yarnrc.yml": "nodeLinker: node-modules\n", "package.json": `{"packageManager": "yarn@3.8.7"}`}, 1},
		{"a package.json that is not valid", wf("", run("npm ci")), map[string]string{"package.json": `{`}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			root := baselineProject(t, tc.wf, "profile: default\nrules:\n  unlocked-install:\n    pedantic: true\n")
			for name, content := range tc.files {
				writeTestFile(t, filepath.Join(root, filepath.FromSlash(name)), content)
			}
			_, out, _ := baselineCmd(t)
			if got := strings.Count(out, "lifecycle scripts"); got != tc.want {
				t.Errorf("want %d findings, got %d:\n%s", tc.want, got, out)
			}
		})
	}

	t.Run("not reported without the pedantic option", func(t *testing.T) {
		baselineProject(t, wf("", run("npm ci")), "profile: default\n")
		if _, out, _ := baselineCmd(t); strings.Contains(out, "lifecycle scripts") {
			t.Errorf("reported under the default profile:\n%s", out)
		}
	})
}

// A workflow that is linted on its own has no files to look at: the install is reported unless the command line says
// otherwise.
func TestUnlockedInstallLifecycleScriptsWithoutAProject(t *testing.T) {
	src := "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm ci\n      - run: npm ci --ignore-scripts\n      - run: cd web && npm ci\n"
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
		if e.ID == "unlocked-install" && strings.Contains(e.Message, "lifecycle scripts") {
			lines = append(lines, e.Line)
		}
	}
	if len(lines) != 1 || lines[0] != 6 {
		t.Errorf("want one finding at line 6, got %v: %v", lines, errs)
	}
}

package jactionlint

import "strings"

// The actions that run what is in the workspace. After a checkout of the code of a pull request in a privileged workflow, a step
// of one of them is "a step that runs it", like `make` or `npm ci` in a `run:` step.
//
// Sources: poutine (`build_github_actions` of untrusted_checkout_exec.rego), CodeQL (`poisonableActionsDataModel` of
// poisonable_steps.yml) and the LOTP catalog that both cite (https://boostsecurityio.github.io/lotp/, whose entries for GitHub
// Actions are actions/setup-node, oxsecurity/megalinter and local actions; the rest are the command line tools that the actions
// below run on the workspace). An action is listed when it runs a tool on the files of the repository: a build, a linter with
// the plugins and the configuration of the repository, an installer that runs the scripts of a package. An action that only
// installs a tool is not, and neither is one that runs something only for some of its inputs unless the input is checked:
// the entries with a gate say which input.
//
// Not listed, although poutine or CodeQL has them: `docker/setup-buildx-action` (it installs buildx), `pre-commit-ci/lite-action`
// (it commits the changes of an earlier step), `browniebroke/pre-commit-autoupdate-action` (it updates the pins and runs no
// hook), `actions/jekyll-build-pages` (the plugins of a site are not run by the action in safe mode, which was not verified) and
// `gradle/actions/setup-gradle` (it prepares a later step).

// workspaceAction is an action that runs the workspace, and when.
type workspaceAction struct {
	// gate says whether this use of the action runs the workspace, nil for every use.
	gate func(e *ExecAction) bool
	// prefix is whether the entry also covers the actions below it (`oxsecurity/megalinter/flavors/python`).
	prefix bool
}

// workspaceActions are the actions by lower case name (owner/repo[/path]). A trailing option says how an entry is used.
var workspaceActions = func() map[string]workspaceAction {
	m := map[string]workspaceAction{}
	always := func(names ...string) {
		for _, n := range names {
			m[strings.ToLower(n)] = workspaceAction{}
		}
	}
	family := func(names ...string) {
		for _, n := range names {
			m[strings.ToLower(n)] = workspaceAction{prefix: true}
		}
	}
	gated := func(name string, gate func(e *ExecAction) bool) {
		m[strings.ToLower(name)] = workspaceAction{gate: gate}
	}

	// the actions of the first version of the rule
	always("github/codeql-action/autobuild", "github/codeql-action/analyze", "pre-commit/action", "golangci/golangci-lint-action")

	// linters and checkers with the configuration, the plugins and the custom checks of the repository (eslint configuration
	// and plugins are code, `.mega-linter.yml` has `PRE_COMMANDS`, checkov loads custom checks and `.checkov.yaml`,
	// `.trunk/trunk.yaml` defines linters, vale styles, rubocop, tflint, mypy and stylelint plugins, phpstan extensions)
	family("oxsecurity/megalinter", "super-linter/super-linter", "github/super-linter")
	always(
		"bridgecrewio/checkov-action",
		"trunk-io/trunk-action",
		"sonarsource/sonarcloud-github-action", "sonarsource/sonarqube-scan-action",
		"errata-ai/vale-action", "gaurav-nelson/github-action-vale-lint",
		"php-actions/phpstan", "actions-hub/stylelint",
		"reviewdog/action-eslint", "reviewdog/action-rubocop", "reviewdog/action-tflint", "reviewdog/action-flake8",
		"reviewdog/action-stylelint", "reviewdog/action-golangci-lint", "reviewdog/action-mypy",
		"stefanoeb/eslint-action", "tj-actions/eslint-changed-files", "sibiraj-s/action-eslint", "tinovyatkin/action-eslint",
		"bradennapier/eslint-plus-action", "catchen/eslint-suggestion-action", "icrawl/action-eslint", "ninosaurus/eslint-check",
		"andrewmcodes-archive/rubocop-linter-action", "gimenete/rubocop-action", "r7kamura/rubocop-todo-corrector",
		"devops-infra/action-tflint",
		"ricardochaves/python-lint", "jpetrucciani/mypy-check", "sunnysid3up/python-linter", "tsuyoshicho/action-mypy",
		// pre-commit runs the hooks of the configuration of the repository
		"dbt-checkpoint/dbt-checkpoint", "cloudposse/github-action-pre-commit",
	)

	// builds and installers: build scripts, build.rs, setup.py, the scripts of a package, the providers of a terraform
	// configuration (`external` data sources, local-exec)
	always(
		"dflook/terraform-plan", "dflook/terraform-apply", "dflook/tofu-plan", "dflook/tofu-apply", "op5dev/tf-via-pr",
		"actions-rs/cargo",
		"js-devtools/npm-publish", // npm publish runs prepublishOnly and prepack
		"hynek/build-and-inspect-python-package", "pypa/cibuildwheel", "pyo3/maturin-action",
		"qcastel/github-actions-maven", "qcastel/github-actions-maven-release", "samuelmeuli/action-maven-publish", "lucafeger/action-maven-cli",
		"mhausenblas/mkdocs-deploy-gh-pages", "athackst/mkdocs-simple-plugin",
		"mvs-telecom/publish-nuget",
		"brettcannon/pip-secure-install", "bsfishy/pip-action",
	)
	m["qcastel/github-actions-maven"] = workspaceAction{prefix: true} // actions/maven is a path below it

	// actions that run the workspace for some of their inputs
	gated("ruby/setup-ruby", func(e *ExecAction) bool { return inputIsTrue(e, "bundler-cache") }) // bundle install: the Gemfile is Ruby
	gated("actions/setup-node", func(e *ExecAction) bool {
		// cache: runs npm, yarn or pnpm in the workspace to find the cache, and `yarnPath` of .yarnrc.yml is a script (LOTP)
		v, ok := e.input("cache")
		return ok && v != "" && !strings.Contains(v, "${{")
	})
	gated("docker/build-push-action", func(e *ExecAction) bool {
		// the default context is the git context of the repository, not the workspace: only a path runs its Dockerfile
		v, ok := e.input("context")
		if !ok || v == "" || strings.Contains(v, "${{") || strings.Contains(v, "{{") {
			return false
		}
		return !strings.Contains(v, "://") && !strings.HasPrefix(v, "git@")
	})
	runsArgs := func(e *ExecAction) bool {
		v, ok := e.input("args")
		if !ok || v == "" || (strings.HasPrefix(v, "${{") && strings.HasSuffix(v, "}}") && strings.Count(v, "${{") == 1) {
			return false // nothing to run, or not known
		}
		return !inputIsTrue(e, "install-only")
	}
	gated("goreleaser/goreleaser-action", func(e *ExecAction) bool {
		// goreleaser release and build run the hooks of .goreleaser.yaml; check and --version do not
		if !runsArgs(e) {
			return false
		}
		v, _ := e.input("args")
		for _, w := range strings.Fields(strings.ToLower(v)) {
			if w == "release" || w == "build" || w == "r" || w == "b" {
				return true
			}
		}
		return false
	})
	gated("magefile/mage-action", runsArgs)
	gated("gradle/gradle-build-action", func(e *ExecAction) bool {
		// only with `arguments` does the action run gradle (the build script is code); without it, it prepares a later step
		v, ok := e.input("arguments")
		return ok && v != "" && !(strings.HasPrefix(v, "${{") && strings.HasSuffix(v, "}}") && strings.Count(v, "${{") == 1)
	})
	gated("azure/powershell", func(e *ExecAction) bool {
		// the script is in the workflow; it runs the workspace when it calls a script of it
		v, _ := e.input("inlineScript")
		return strings.Contains(v, ".ps1") || strings.Contains(v, "./") || strings.Contains(v, `.\`)
	})
	gated("cypress-io/github-action", func(e *ExecAction) bool {
		// it installs the dependencies (npm ci, with their scripts) and runs cypress, which executes cypress.config.js
		install, installSet := e.input("install")
		runTests, runSet := e.input("runtests")
		return !((installSet && strings.EqualFold(install, "false")) && (runSet && strings.EqualFold(runTests, "false")))
	})
	return m
}()

// inputIsTrue reports whether the input of the step is the literal true.
func inputIsTrue(e *ExecAction, name string) bool {
	v, ok := e.input(name)
	return ok && strings.EqualFold(v, "true")
}

// workspaceActionRuns reports whether the step runs an action that runs what is in the workspace, for its inputs.
func workspaceActionRuns(e *ExecAction, u *UsesRef) bool {
	name := strings.ToLower(u.CanonicalName())
	if a, ok := workspaceActions[name]; ok && (a.gate == nil || a.gate(e)) {
		return true
	}
	// an action below a family: oxsecurity/megalinter/flavors/python
	for i := len(name) - 1; i > 0; i-- {
		if name[i] != '/' {
			continue
		}
		if a, ok := workspaceActions[name[:i]]; ok && a.prefix && (a.gate == nil || a.gate(e)) {
			return true
		}
	}
	return false
}

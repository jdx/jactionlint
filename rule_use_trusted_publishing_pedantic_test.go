package jactionlint

import (
	"strings"
	"testing"
)

// The pedantic findings of use-trusted-publishing: what a job that publishes with the OIDC token also does.
func TestUseTrustedPublishingPedantic(t *testing.T) {
	const checkout = "      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2\n"
	const publish = "      - uses: pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc # v1.12.4\n"
	job := func(head, steps string) string {
		return "on: release\npermissions: {}\njobs:\n  publish:\n    runs-on: ubuntu-latest\n" + head + "    steps:\n" + steps
	}
	jobOn := func(runner, head, steps string) string {
		return strings.Replace(job(head, steps), "runs-on: ubuntu-latest", "runs-on: "+runner, 1)
	}
	const oidc = "    permissions:\n      id-token: write\n"
	const env = "    environment: pypi\n" + oidc
	run := func(c string) string { return "      - run: " + c + "\n" }
	tests := []struct {
		what                  string
		src                   string
		wantBuild, wantEnv, n int // n counts the findings about the attestations
	}{
		{"checkout and build", job(env, checkout+run("python -m build")+publish), 1, 0, 0},
		{"checkout and pip install", job(env, checkout+run("pip install -r requirements.txt")+publish), 1, 0, 0},
		{"checkout and npm ci", job(env, checkout+run("npm ci")+"      - run: npm publish\n"), 1, 0, 0},
		{"checkout and make", job(env, checkout+run("make dist")+publish), 1, 0, 0},
		{"checkout and uv build", job(env, checkout+run("uv build")+publish), 1, 0, 0},
		{"checkout and poetry build", job(env, checkout+run("poetry build")+publish), 1, 0, 0},
		{"checkout and cargo build before publish", job(env, checkout+run("cargo build")+run("cargo publish")), 0, 0, 0},
		{"build in the script of the publish", job(env, checkout+run("npm ci && npm run build && npm publish")), 1, 0, 0},
		{"build action", job(env, checkout+"      - uses: PyO3/maturin-action@v1\n"+publish), 1, 0, 0},
		{"setup-ruby with a cache", job(env, checkout+"      - uses: ruby/setup-ruby@v1\n        with:\n          bundler-cache: true\n      - run: gem build x.gemspec\n      - run: gem push x.gem\n"), 1, 0, 0},

		// not a build, or not in this job
		{"no checkout", job(env, run("python -m build")+publish), 0, 0, 0},
		{"checkout only", job(env, checkout+publish), 0, 0, 0},
		{"download and publish", job(env, "      - uses: actions/download-artifact@fa0a91b85d4f404e444e00e005971372dc801d16 # v4.1.8\n"+publish), 0, 0, 0},
		{"installing the publisher", job(env, checkout+run("pip install twine")+run("twine upload dist/*")), 0, 0, 0},
		{"upgrading npm", job(env, checkout+run("npm install -g npm@latest")+run("npm publish ./pkg.tgz")), 0, 0, 0},
		{"build after the publish", job(env, checkout+publish+run("npm ci")), 0, 0, 0},
		{"checking out after the build", job(env, run("python -m build")+checkout+publish), 0, 0, 0},
		{"npx", job(env, checkout+run("npx semantic-release")+publish), 0, 0, 0},
		{"rubygems release-gem builds itself", job(env, checkout+run("bundle install")+"      - uses: rubygems/release-gem@v1\n"), 0, 0, 0},
		{"cargo publish builds itself", job(env, checkout+run("cargo test")+run("cargo publish")), 0, 0, 0},
		{"no id-token", job("    environment: pypi\n", checkout+run("python -m build")+publish), 0, 0, 0},
		{"a password", job(env, checkout+run("python -m build")+publish+"        with:\n          password: ${{ secrets.PYPI_TOKEN }}\n"), 0, 0, 0},
		{"a token", job(env, checkout+run("npm ci")+run("npm publish")+"        env:\n          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}\n"), 0, 0, 0},
		{"a private registry", job(env, checkout+run("npm ci")+run("npm publish --registry https://npm.pkg.github.com")), 0, 0, 0},
		{"a dry run", job(env, checkout+run("npm ci")+run("npm publish --dry-run")), 0, 0, 0},

		// a parallel group is part of the job
		{"checkout, build and publish in a parallel group", job(env, "      - parallel:\n          - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2\n          - run: python -m build\n"+publish), 1, 0, 0},
		{"publish in a parallel group", job(oidc, "      - parallel:\n          - uses: pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc # v1.12.4\n"), 0, 1, 0},
		{"attestations off in a parallel group", job(env, "      - parallel:\n          - uses: pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc # v1.12.4\n            with:\n              attestations: false\n"), 0, 0, 1},

		// -p is the password of twine: that is the default finding, not trusted publishing
		{"twine -p", job(env, checkout+run("python -m build")+run("twine upload -p ${{ secrets.PYPI_TOKEN }} dist/*")), 0, 0, 0},
		{"twine --password", job(env, checkout+run("python -m build")+run("twine upload --password ${{ secrets.PYPI_TOKEN }} dist/*")), 0, 0, 0},
		{"cargo publish -p is a package", job(oidc, checkout+run("cargo publish -p mycrate")), 0, 1, 0},

		// PowerShell: the default shell of the Windows runners
		{"pwsh npm publish after a build", jobOn("windows-latest", env, checkout+run("npm ci")+run("npm publish")), 1, 0, 0},
		{"pwsh dotnet nuget push without environment", jobOn("windows-latest", oidc, run("dotnet nuget push pkg.nupkg")), 0, 1, 0},
		{"pwsh build and publish in one script", jobOn("windows-latest", env, checkout+"      - run: |\n          dotnet build -c Release\n          dotnet nuget push pkg.nupkg\n"), 1, 0, 0},
		{"pwsh a token", jobOn("windows-latest", env, checkout+run("npm ci")+run("npm publish --token $env:T")), 0, 0, 0},
		{"pwsh a dry run", jobOn("windows-latest", env, checkout+run("npm ci")+run("npm publish --dry-run")), 0, 0, 0},
		{"pwsh npm global install of npm is no build", jobOn("windows-latest", env, checkout+run("npm install -g npm@latest")+run("npm publish")), 0, 0, 0},
		{"pwsh pip install twine is no build", jobOn("windows-latest", env, checkout+run("pip install twine")+run("twine upload dist/*")), 0, 0, 0},
		{"pwsh pip install of the project is a build", jobOn("windows-latest", env, checkout+run("pip install -e .")+run("twine upload dist/*")), 1, 0, 0},
		{"pwsh a variable is no build", jobOn("windows-latest", env, checkout+run("$env:CI = 1")+run("npm publish")), 0, 0, 0},
		{"pwsh build and publish on one line, ;", jobOn("windows-latest", env, checkout+run("npm ci; npm publish")), 1, 0, 0},
		{"pwsh build and publish on one line, &&", jobOn("windows-latest", env, checkout+run("npm ci && npm publish")), 1, 0, 0},
		{"pwsh build and publish on one line, ||", jobOn("windows-latest", env, checkout+run("npm ci || npm publish")), 1, 0, 0},
		{"pwsh publish then build on one line", jobOn("windows-latest", env, checkout+run("npm publish; npm ci")), 0, 0, 0},
		{"pwsh install of the tools and publish on one line", jobOn("windows-latest", env, checkout+run("npm install -g npm@latest; npm publish")), 0, 0, 0},
		{"pwsh a token on a one-liner", jobOn("windows-latest", env, checkout+run("npm ci; npm publish --token $env:T")), 0, 0, 0},
		// a requirements file or the project itself is a build, even next to the tools of publishing
		{"pip install twine -r requirements.txt", job(env, checkout+run("pip install twine -r requirements.txt")+run("twine upload dist/*")), 1, 0, 0},
		{"pip install -r requirements.txt twine", job(env, checkout+run("pip install --requirement requirements.txt twine")+run("twine upload dist/*")), 1, 0, 0},
		{"pip install twine -e .", job(env, checkout+run("pip install twine -e .")+run("twine upload dist/*")), 1, 0, 0},
		{"pip install twine .", job(env, checkout+run("pip install twine .")+run("twine upload dist/*")), 1, 0, 0},
		{"pip install twine -c constraints.txt", job(env, checkout+run("pip install twine -c constraints.txt")+run("twine upload dist/*")), 0, 0, 0},
		{"pip install twine build", job(env, checkout+run("pip install twine build")+run("twine upload dist/*")), 1, 0, 0},
		{"pwsh pip install -r requirements.txt", jobOn("windows-latest", env, checkout+run("pip install twine -r requirements.txt")+run("twine upload dist/*")), 1, 0, 0},

		// Windows spellings: the case and the suffix of the command do not matter
		{"pwsh NuGet.exe restore then push", jobOn("windows-latest", env, checkout+run("NuGet.exe restore")+run("NuGet.exe push pkg.nupkg")), 1, 0, 0},
		{"pwsh MSBuild.exe then push", jobOn("windows-latest", env, checkout+run("MSBuild.exe app.sln /t:Build")+run("dotnet nuget push pkg.nupkg")), 1, 0, 0},
		{"pwsh Dotnet.exe build", jobOn("windows-latest", env, checkout+run("Dotnet.exe build")+run("dotnet nuget push pkg.nupkg")), 1, 0, 0},
		{"pwsh msbuild.cmd", jobOn("windows-latest", env, checkout+run("msbuild.cmd app.sln")+run("dotnet nuget push pkg.nupkg")), 1, 0, 0},
		{"pwsh npm.cmd ci", jobOn("windows-latest", env, checkout+run("npm.cmd ci")+run("npm publish")), 1, 0, 0},
		{"pwsh NuGet.exe push alone", jobOn("windows-latest", env, checkout+run("NuGet.exe push pkg.nupkg")), 0, 0, 0},
		{"pwsh nuget restore then push", jobOn("windows-latest", env, checkout+run("nuget restore")+run("nuget push pkg.nupkg")), 1, 0, 0},
		{"bash nuget restore then push", job(env, checkout+run("nuget restore")+run("nuget push pkg.nupkg")), 1, 0, 0},
		{"dotnet restore then nuget push", job(env, checkout+run("dotnet restore")+run("dotnet nuget push pkg.nupkg")), 1, 0, 0},
		{"nuget push only", job(env, checkout+run("nuget push pkg.nupkg")), 0, 0, 0},
		{"pwsh no checkout", jobOn("windows-latest", env, run("npm ci")+run("npm publish")), 0, 0, 0},
		{"pwsh publish after the publish only", jobOn("windows-latest", env, checkout+run("npm publish")+run("npm ci")), 0, 0, 0},
		{"shell pwsh on linux", job(env, checkout+"      - run: npm ci\n        shell: pwsh\n      - run: npm publish\n        shell: pwsh\n"), 1, 0, 0},

		// the environment
		{"no environment", job(oidc, publish), 0, 1, 0},
		{"an environment", job(env, publish), 0, 0, 0},
		{"an environment as an expression", job("    environment: ${{ inputs.env }}\n"+oidc, publish), 0, 0, 0},
		{"environment with a name and a url", job("    environment:\n      name: pypi\n      url: https://pypi.org/p/x\n"+oidc, publish), 0, 0, 0},
		{"no environment, twine", job(oidc, run("twine upload dist/*")), 0, 1, 0},
		{"no environment, the build and the environment are two findings", job(oidc, checkout+run("npm ci")+run("npm publish")), 1, 1, 0},
		{"id-token from the workflow", "on: release\npermissions:\n  id-token: write\njobs:\n  publish:\n    runs-on: ubuntu-latest\n    steps:\n" + publish, 0, 1, 0},
		{"a local reusable workflow call", "on: release\njobs:\n  publish:\n    uses: ./.github/workflows/publish.yaml\n", 0, 0, 0},

		// the attestations
		{"attestations off", job(env, publish+"        with:\n          attestations: false\n"), 0, 0, 1},
		{"attestations off, upper case", job(env, publish+"        with:\n          attestations: FALSE\n"), 0, 0, 1},
		{"attestations on", job(env, publish+"        with:\n          attestations: true\n"), 0, 0, 0},
		{"attestations as an expression", job(env, publish+"        with:\n          attestations: ${{ inputs.attest }}\n"), 0, 0, 0},
		{"attestations off of release-gem", job(env, "      - uses: rubygems/release-gem@v1\n        with:\n          attestations: false\n"), 0, 0, 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			baselineProject(t, tc.src, "profile: default\nrules:\n  use-trusted-publishing:\n    pedantic: true\n")
			_, out, _ := baselineCmd(t)
			count := func(s string) int { return strings.Count(out, s) }
			if got := count("also checks out the repository and runs"); got != tc.wantBuild {
				t.Errorf("want %d findings about the build, got %d:\n%s", tc.wantBuild, got, out)
			}
			if got := count(`has no "environment:"`); got != tc.wantEnv {
				t.Errorf("want %d findings about the environment, got %d:\n%s", tc.wantEnv, got, out)
			}
			if got := count("not to publish attestations"); got != tc.n {
				t.Errorf("want %d findings about the attestations, got %d:\n%s", tc.n, got, out)
			}
		})
	}

	t.Run("not reported without the pedantic option", func(t *testing.T) {
		baselineProject(t, job(oidc, checkout+run("npm ci")+run("npm publish")), "profile: default\n")
		_, out, _ := baselineCmd(t)
		for _, s := range []string{"also checks out", "no \"environment:\"", "attestations"} {
			if strings.Contains(out, s) {
				t.Errorf("%q reported under the default profile:\n%s", s, out)
			}
		}
	})
}

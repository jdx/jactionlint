package jactionlint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func lintPublishing(t *testing.T, root, src string) int {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(fixtureConfig("use-trusted-publishing"))
	var proj *Project
	if root != "" {
		proj = &Project{root: root}
	}
	errs, err := l.Lint("test.yaml", []byte(src), proj)
	if err != nil {
		t.Fatal(err)
	}
	return len(policyErrorsOf(errs, "use-trusted-publishing"))
}

// A registry that is set for one scope hides a publish only when the package is of that scope.
func TestUseTrustedPublishingScopedRegistry(t *testing.T) {
	const head = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	const tail = "        env:\n          NODE_AUTH_TOKEN: ${{ secrets.T }}\n"
	const set = "      - run: |\n          npm config set @acme:registry http://localhost:4873\n          npm publish%s\n" + tail
	with := func(args string) string { return head + fmt.Sprintf(set, args) }
	named := func(n string) map[string]string { return map[string]string{"package.json": `{"name": "` + n + `"}`} }
	tests := []struct {
		name string
		pkg  map[string]string // path -> content
		src  string
		want int
	}{
		{"unscoped package goes to the public registry", named("left-pad"), with(""), 1},
		{"package of another scope goes to the public registry", named("@other/x"), with(""), 1},
		{"package of the private scope", named("@acme/x"), with(""), 0},
		{"scope matches without case", named("@ACME/x"), with(""), 0},
		{"no package.json", nil, with(""), 0},
		{"unreadable package.json", map[string]string{"package.json": `{`}, with(""), 0},
		{"no name", map[string]string{"package.json": `{}`}, with(""), 0},
		{"folder argument", map[string]string{"pkg/package.json": `{"name": "left-pad"}`, "package.json": `{"name": "@acme/x"}`}, with(" pkg"), 1},
		{"tarball argument", named("left-pad"), with(" x.tgz"), 0},
		{"workspace selection", named("left-pad"), with(" -w a"), 0},
		{"working directory of the step", map[string]string{"sub/package.json": `{"name": "left-pad"}`, "package.json": `{"name": "@acme/x"}`},
			head + "      - working-directory: sub\n        run: |\n          npm config set @acme:registry http://localhost:4873\n          npm publish\n" + tail, 1},
		{"working directory is an expression", named("left-pad"),
			head + "      - working-directory: ${{ matrix.d }}\n        run: |\n          npm config set @acme:registry http://localhost:4873\n          npm publish\n" + tail, 0},
		{"cd before the publish", named("left-pad"),
			head + "      - run: |\n          npm config set @acme:registry http://localhost:4873\n          cd sub\n          npm publish\n" + tail, 0},
		{"manifest rewritten by an earlier step", named("left-pad"),
			head + "      - run: npm pkg set name=@acme/x\n" + fmt.Sprintf(set, ""), 0},
		{"unscoped registry stays private", named("left-pad"),
			head + "      - run: |\n          npm config set registry http://localhost:4873\n          npm config set @acme:registry http://localhost:4873\n          npm publish\n" + tail, 0},
		{"scope set to the public registry", named("@acme/x"),
			head + "      - run: |\n          npm config set @acme:registry https://registry.npmjs.org/\n          npm publish\n" + tail, 1},
		{"scope reset to the public registry", named("@acme/x"),
			head + "      - run: |\n          npm config set @acme:registry http://localhost:4873\n          npm config set @acme:registry https://registry.npmjs.org/\n          npm publish\n" + tail, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for p, c := range tc.pkg {
				full := filepath.Join(root, filepath.FromSlash(p))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := lintPublishing(t, root, tc.src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, tc.src)
			}
		})
	}
	t.Run("no project", func(t *testing.T) {
		if got := lintPublishing(t, "", with("")); got != 0 {
			t.Errorf("%d findings, want 0", got)
		}
	})
}

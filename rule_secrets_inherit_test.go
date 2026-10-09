package jactionlint

import "testing"

func TestSecretsInheritAnchor(t *testing.T) {
	src := "on: push\njobs:\n  a:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n    secrets: inherit\n  b:\n    uses: octo/repo/.github/workflows/w.yaml@v1\n    secrets:\n      x: ${{ secrets.X }}\n"
	errs := lintFileWithConfig(t, ruleConfig("secrets-inherit"), "ci.yaml", src)
	wantLines(t, errs, "secrets-inherit", 4)
	if errs[0].Fix != nil {
		t.Errorf("secrets-inherit has no fix: %+v", errs[0].Fix)
	}
}

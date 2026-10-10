package jactionlint

import "testing"

func TestCachePoisoningReleaseBranches(t *testing.T) {
	const tail = "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n"
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"release/**", "on:\n  push:\n    branches: ['release/**']\n" + tail, 1},
		{"release-*", "on:\n  push:\n    branches: ['release-*', '!release-old']\n" + tail, 1},
		{"main next to a release branch", "on:\n  push:\n    branches: [main, 'release/**']\n" + tail, 0},
		{"a name that only starts like it", "on:\n  push:\n    branches: [releaser]\n" + tail, 0},
		{"read-only token is a check", "on:\n  push:\n    branches: ['release/**']\npermissions: read-all\n" + tail, 0},
		{"gated on the tag", "on:\n  push:\n    branches: ['release/**']\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n        if: startsWith(github.ref, 'refs/tags/')\n", 0},
		{"gated on the ref type tag", "on:\n  push:\n    branches: ['release/**']\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n        if: github.ref_type == 'tag'\n", 0},
		{"gated on the ref type branch", "on:\n  push:\n    branches: ['release/**']\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n        if: github.ref_type == 'branch'\n", 1},
		{"no branch filter", "on: push\n" + tail, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lintCacheWorkflow(t, "", tc.src); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

func TestCachePoisoningMorePublishingActions(t *testing.T) {
	for _, a := range []string{"rust-lang/crates-io-auth-action@v1", "azure/webapps-deploy@v3", "taiki-e/upload-rust-binary-action@v1"} {
		src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + a + "\n      - uses: Swatinem/rust-cache@v2\n"
		if got := lintCacheWorkflow(t, "", src); len(got) != 1 {
			t.Errorf("%s: want 1 finding but got %v", a, got)
		}
	}
}

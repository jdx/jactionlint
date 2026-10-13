package jactionlint

import (
	"strings"
	"testing"
)

// The forks of dawidd6/action-download-artifact and the downloaders of octoscan's rule are downloads of artifacts of
// other runs too.
func TestRuleUntrustedArtifactMoreDownloaders(t *testing.T) {
	const head = "on:\n  workflow_run:\n    workflows: [Build]\n    types: [completed]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		what string
		src  string
	}{
		{"aochmann", head + `      - uses: aochmann/actions-download-artifact@v1 # want
        with:
          name: pr
          path: pr
      - run: bash pr/run.sh
`},
		{"aochmann with a name only", head + `      - uses: aochmann/actions-download-artifact@v1 # want
        with:
          name: pr
      - run: ./pr/run.sh
`},
		{"aochmann writes the content to the environment", head + `      - uses: aochmann/actions-download-artifact@v1 # want
        with:
          name: pr
          path: pr
      - run: echo "N=$(cat pr/number)" >> "$GITHUB_ENV"
`},
		{"blablacar", head + `      - uses: blablacar/action-download-last-artifact@v1 # want
        with:
          name: build
          path: build
      - run: bash build/run.sh
`},
		{"bettermarks with a name", head + `      - uses: bettermarks/action-artifact-download@v1 # want
        with:
          artifact_name: build
          repo: o/r
      - run: bash build/run.sh
`},
		{"bettermarks renames it", head + `      - uses: bettermarks/action-artifact-download@v1 # want
        with:
          artifact_name: build
          rename: out
      - run: ./out/tool
`},
		{"bettermarks and a command of something else", head + `      - uses: bettermarks/action-artifact-download@v1
        with:
          artifact_name: build
      - run: bash scripts/run.sh
`},
		{"aochmann and a command of something else", head + `      - uses: aochmann/actions-download-artifact@v1
        with:
          name: pr
          path: pr
      - run: bash scripts/run.sh
`},
		{"downloads and uses nothing", head + `      - uses: blablacar/action-download-last-artifact@v1
        with:
          name: build
      - run: echo done
`},
		{"not in a workflow_run", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + `      - uses: aochmann/actions-download-artifact@v1
        with:
          name: pr
          path: pr
      - run: bash pr/run.sh
`},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := onlyID(lintInProject(t, nil, tc.src, "rules:\n  untrusted-artifact: error\n"), "untrusted-artifact")
			checkLines(t, errs, markedWantLines(tc.src)...)
		})
	}
}

// With the option pedantic, a download of every artifact of the triggering run is reported (ghlint#239).
func TestRuleUntrustedArtifactBlanketDownload(t *testing.T) {
	const head = "on:\n  workflow_run:\n    workflows: [Build]\n    types: [completed]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	const run = "          run-id: ${{ github.event.workflow_run.id }}\n          github-token: ${{ github.token }}\n"
	tests := []struct {
		what string
		step string
		want int
	}{
		{"download-artifact without a name", "      - uses: actions/download-artifact@v4\n        with:\n" + run + "          path: art\n", 1},
		{"download-artifact without with", "      - uses: actions/download-artifact@v4\n", 0}, // the artifacts of this run
		{"download-artifact with a name", "      - uses: actions/download-artifact@v4\n        with:\n" + run + "          name: pr\n", 0},
		{"download-artifact with a pattern", "      - uses: actions/download-artifact@v4\n        with:\n" + run + "          pattern: pr-*\n", 0},
		{"download-artifact with ids", "      - uses: actions/download-artifact@v4\n        with:\n" + run + "          artifact-ids: 1,2\n", 0},
		{"download-artifact with an expression as the name", "      - uses: actions/download-artifact@v4\n        with:\n" + run + "          name: ${{ matrix.name }}\n", 0},
		{"download-artifact of the current run", "      - uses: actions/download-artifact@v4\n        with:\n          path: art\n", 0},
		{"dawidd6 without a name", "      - uses: dawidd6/action-download-artifact@v6\n        with:\n          run_id: ${{ github.event.workflow_run.id }}\n", 1},
		{"dawidd6 with a name", "      - uses: dawidd6/action-download-artifact@v6\n        with:\n          run_id: ${{ github.event.workflow_run.id }}\n          name: pr\n", 0},
		{"aochmann without a name", "      - uses: aochmann/actions-download-artifact@v1\n", 1},
		{"gh run download", "      - run: gh run download ${{ github.event.workflow_run.id }} -D art\n", 1},
		{"gh run download with a name", "      - run: gh run download ${{ github.event.workflow_run.id }} -n pr -D art\n", 0},
		{"gh run download with a pattern", "      - run: gh run download ${{ github.event.workflow_run.id }} --pattern 'pr-*'\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head + tc.step
			count := func(cfg string) int {
				n := 0
				for _, e := range lintInProject(t, nil, src, cfg) {
					if e.ID == "untrusted-artifact" && strings.Contains(e.Message, "every artifact") {
						n++
					}
				}
				return n
			}
			if got := count("rules:\n  untrusted-artifact:\n    level: error\n    pedantic: true\n"); got != tc.want {
				t.Errorf("pedantic: want %d findings, got %d", tc.want, got)
			}
			if got := count("rules:\n  untrusted-artifact: error\n"); got != 0 {
				t.Errorf("default: want none, got %d", got)
			}
		})
	}
}

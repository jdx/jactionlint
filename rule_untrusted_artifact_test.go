package jactionlint

import (
	"strings"
	"testing"
)

func TestRuleUntrustedArtifact(t *testing.T) {
	const head = "on:\n  workflow_run:\n    workflows: [Build]\n    types: [completed]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	const dl = `      - uses: actions/download-artifact@v4 # want
        with:
          run-id: ${{ github.event.workflow_run.id }}
          github-token: ${{ github.token }}
`
	const dlN = `      - uses: actions/download-artifact@v4
        with:
          run-id: ${{ github.event.workflow_run.id }}
          github-token: ${{ github.token }}
`
	tests := []struct {
		what   string
		others map[string]string
		src    string
	}{
		{"content of the artifact goes to the environment", nil, head + dl + `          name: pr_number
      - run: echo "PR_NUMBER=$(cat pr_number)" >> "$GITHUB_ENV"
`},
		{"content goes to the output", nil, head + dl + `          name: pr
          path: pr
      - id: pr
        run: echo "number=$(cat pr/number)" >> $GITHUB_OUTPUT
`},
		{"read and write", nil, head + dl + `          name: pr
          path: pr
      - run: |
          read -r N < pr/number
          echo "N=$N" >> "$GITHUB_ENV"
`},
		{"extraction", nil, head + dl + `          name: result
          path: result
      - run: unzip result/out.zip -d out
`},
		{"extraction with tar", nil, head + dl + `          path: result
      - run: tar -xzf result/out.tgz
`},
		{"script from the artifact", nil, head + dl + `          path: build
      - run: bash build/run.sh
`},
		{"program from the artifact", nil, head + dl + `          path: build
      - run: ./build/tool --version
`},
		{"build in the artifact directory", nil, head + dl + `          path: out
      - run: npm ci
        working-directory: out
`},
		{"cd into the artifact", nil, head + dl + `          path: out
      - run: |
          cd out
          make
`},
		{"another download action", nil, head + `      - uses: dawidd6/action-download-artifact@v6 # want
        with:
          run_id: ${{ github.event.workflow_run.id }}
          path: art
      - run: python art/report.py
`},
		{"gh run download of the current run", nil, head + `      - run: gh run download ${{ github.run_id }} -D art
      - run: |
          cd art && ./build.sh
`},
		{"gh run download without a run", nil, head + `      - run: gh run download -n art -D art
      - run: |
          cd art && ./build.sh
`},
		{"guarded step that uses the artifact", nil, head + dl + `          name: pr
          path: pr
      - run: cd pr && ./build.sh
        if: contains(github.event.workflow_run.pull_requests.*.labels.*.name, 'safe to test')
`},
		{"gh run download", nil, head + `      - run: gh run download ${{ github.event.workflow_run.id }} -D art # want
      - run: cd art && make
`},
		{"github-script download and unzip", nil, head + `      - uses: actions/github-script@v7 # want
        with:
          script: |
            const artifacts = await github.rest.actions.listWorkflowRunArtifacts({owner: context.repo.owner, repo: context.repo.repo, run_id: context.payload.workflow_run.id});
            const m = artifacts.data.artifacts.filter(a => a.name == "pr")[0];
            const download = await github.rest.actions.downloadArtifact({owner: context.repo.owner, repo: context.repo.repo, artifact_id: m.id, archive_format: 'zip'});
            require('fs').writeFileSync('pr.zip', Buffer.from(download.data));
      - run: unzip pr.zip
`},

		// Not reported
		{"artifact of the current run", nil, head + `      - uses: actions/download-artifact@v4
        with:
          name: pr
          path: pr
      - run: bash pr/run.sh
`},
		{"validated in the same step", nil, head + dlN + `          name: pr_number
      - run: |
          PR=$(cat pr_number)
          [[ "$PR" =~ ^[0-9]+$ ]] || exit 1
          echo "PR_NUMBER=$PR" >> "$GITHUB_ENV"
`},
		{"validated in an earlier step", nil, head + dlN + `          name: pr_number
      - run: grep -qE '^[0-9]+$' pr_number
      - run: echo "PR_NUMBER=$(cat pr_number)" >> "$GITHUB_ENV"
`},
		{"checksum", nil, head + dlN + `          path: build
      - run: sha256sum -c build/SHA256SUMS
      - run: bash build/run.sh
`},
		{"only read", nil, head + dlN + `          name: pr_number
      - run: cat pr_number
      - run: ls -la pr_number
`},
		{"unrelated commands", nil, head + dlN + `          path: art
      - uses: actions/checkout@v4
      - run: npm ci
      - run: bash scripts/ci.sh
`},
		{"root download without a name", nil, head + dlN + `      - run: echo "X=$(cat file)" >> "$GITHUB_ENV"
`},
		{"trusted upstream", map[string]string{"build.yml": "name: Build\non:\n  push:\n    branches: [main]\njobs: {}\n"}, head + `      - uses: actions/download-artifact@v4
        with:
          name: pr
          path: pr
          run-id: ${{ github.event.workflow_run.id }}
      - run: bash pr/run.sh
`},
		{"guarded by the event", nil, `on:
  workflow_run:
    workflows: [Build]
jobs:
  j:
    if: github.event.workflow_run.event == 'push'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          name: pr
          path: pr
          run-id: ${{ github.event.workflow_run.id }}
      - run: bash pr/run.sh
`},
		{"other trigger", nil, `on: push
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          name: pr
          path: pr
          run-id: ${{ github.event.workflow_run.id }}
      - run: bash pr/run.sh
`},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := onlyID(lintInProject(t, tc.others, tc.src, "rules:\n  untrusted-artifact: error\n"), "untrusted-artifact")
			checkLines(t, errs, markedWantLines(tc.src)...)
			crlf := strings.ReplaceAll(tc.src, "\n", "\r\n")
			checkLines(t, onlyID(lintInProject(t, tc.others, crlf, "rules:\n  untrusted-artifact: error\n"), "untrusted-artifact"), markedWantLines(tc.src)...)
		})
	}
}

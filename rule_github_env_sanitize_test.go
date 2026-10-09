package jactionlint

import (
	"strings"
	"testing"
)

func TestGitHubEnvSanitizedValues(t *testing.T) {
	const head = "on: pull_request_target\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - env:\n          TITLE: ${{ github.event.pull_request.title }}\n          REF: ${{ github.head_ref }}\n          REPO: ${{ github.repository }}\n        run: |\n"
	tests := []struct {
		what   string
		script string
		want   int
	}{
		// still reported
		{"plain value", `echo "B=$TITLE" >> "$GITHUB_ENV"`, 1},
		{"a deletion that is not the newline", `echo "B=$(echo "$TITLE" | tr -d ' ')" >> "$GITHUB_ENV"`, 1},
		{"sed that replaces the first character only", `echo "B=$(echo "$TITLE" | sed 's/[^a-z]/-/')" >> "$GITHUB_ENV"`, 1},
		{"newline removal still lets a directory into the path", `echo "$(echo "$TITLE" | tr -d '\n')" >> "$GITHUB_PATH"`, 1},
		{"a regex that is not anchored", `[[ "$TITLE" =~ [a-z]+ ]] || exit 1
          echo "B=$TITLE" >> "$GITHUB_ENV"`, 1},
		{"a regex that lets any character through", `[[ "$TITLE" =~ ^.*$ ]] || exit 1
          echo "B=$TITLE" >> "$GITHUB_ENV"`, 1},
		{"a check after the write", `echo "B=$TITLE" >> "$GITHUB_ENV"
          [[ "$TITLE" =~ ^[a-z]+$ ]] || exit 1`, 1},
		{"a sanitized variable next to a raw one", `echo "B=${TITLE//[^a-z]/} $REF" >> "$GITHUB_ENV"`, 1},
		{"sanitized once and then raw", `S=$(echo "$TITLE" | tr -d '\n')
          echo "B=$S $TITLE" >> "$GITHUB_ENV"`, 1},

		{"a read after the write does not make the value trusted", `echo "TITLE=$TITLE" >> "$GITHUB_ENV"
          IFS=/ read -r OWNER TITLE <<< "$REPO"`, 1},
		{"a read of an untrusted value before the write", `IFS=/ read -r OWNER NAME <<< "$TITLE"
          echo "NAME=$NAME" >> "$GITHUB_ENV"`, 1},
		{"tr that keeps the dot and slash for the path", `echo "$(echo "$REF" | tr -cd 'a-z./')" >> "$GITHUB_PATH"`, 1},
		{"tr that keeps the colon for the path", `echo "$(echo "$REF" | tr -cd 'a-z:')" >> "$GITHUB_PATH"`, 1},
		{"sed whitelist with a slash for the path", `echo "$(echo "$REF" | sed 's/[^a-z.]/-/g')" >> "$GITHUB_PATH"`, 1},
		{"expansion keeping the dot and slash for the path", `echo "${TITLE//[^a-z.]/}" >> "$GITHUB_PATH"`, 1},
		{"tr that keeps the dot for the environment is fine", `echo "K=$(echo "$TITLE" | tr -cd 'a-z.')" >> "$GITHUB_ENV"`, 0},

		// not reported
		{"newline deleted", `C=$(echo "$TITLE" | tr -d "\n\r")
          echo "C=$C" >> "$GITHUB_ENV"`, 0},
		{"newline deleted in place", `echo "C=$(echo "$TITLE" | tr -d '\n')" >> "$GITHUB_ENV"`, 0},
		{"newlines turned into spaces", `echo "C=$(printf %s "$TITLE" | tr '\n' ' ')" >> "$GITHUB_ENV"`, 0},
		{"first line only", `echo "C=$(printf '%s\n' "$TITLE" | head -n 1)" >> "$GITHUB_ENV"`, 0},
		{"expansion removing everything else", `echo "F=${TITLE//[^a-zA-Z0-9]/}" >> "$GITHUB_ENV"`, 0},
		{"expansion removing the newline", `echo "F=${TITLE//$'\n'/}" >> "$GITHUB_ENV"`, 0},
		{"sed whitelist and cut", `SAFE=$(echo "$REF" | sed -e 's/[^a-zA-Z0-9-]/-/g' | cut -c1-63)
          echo "SUB=$SAFE" >> "$GITHUB_ENV"`, 0},
		{"sed whitelist for a path", `echo "$(echo "$REF" | sed 's/[^a-zA-Z0-9-]/-/g')" >> "$GITHUB_PATH"`, 0},
		{"sed that joins lines", `echo "J=$(echo "$TITLE" | sed ':a;N;$!ba;s/\n/ /g')" >> "$GITHUB_ENV"`, 0},
		{"tr keeping a character set", `echo "K=$(echo "$TITLE" | tr -cd 'a-zA-Z0-9_-')" >> "$GITHUB_ENV"`, 0},
		{"validated by an anchored regex that exits", `if [[ ! "$TITLE" =~ ^[0-9]+\.[0-9]+$ ]]; then exit 1; fi
          echo "VER=$TITLE" >> "$GITHUB_ENV"`, 0},
		{"validated by a bare test", `[[ "$TITLE" =~ ^[a-z0-9-]+$ ]]
          echo "VER=$TITLE" >> "$GITHUB_ENV"`, 0},
		{"validated or exit", `[[ "$TITLE" =~ ^[a-z0-9-]+$ ]] || { echo bad; exit 1; }
          echo "VER=$TITLE" >> "$GITHUB_ENV"`, 0},
		{"owner and name split from the repository", `IFS=/ read -r OWNER NAME <<< "$REPO"
          echo "REPO_OWNER=$OWNER" >> "$GITHUB_ENV"
          echo "REPO_NAME=$NAME" >> "$GITHUB_ENV"`, 0},
		{"a date of the run", `echo "D=$(date -d "${{ github.event.workflow_run.run_started_at }}" '+%b %d')" >> "$GITHUB_ENV"`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head
			for _, l := range strings.Split(tc.script, "\n") {
				src += "          " + strings.TrimLeft(l, " ") + "\n"
			}
			if got := countGitHubEnv(t, src); got != tc.want {
				t.Errorf("%s: %d findings, want %d\n%s", tc.what, got, tc.want, src)
			}
		})
	}
}

func TestGitHubEnvMintedToken(t *testing.T) {
	const src = `on: workflow_run
  workflows: [CI]
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - id: app
        uses: actions/create-github-app-token@v2
      - run: echo "GH_TOKEN=${{ steps.app.outputs.token }}" >> "$GITHUB_ENV"
      - id: other
        run: echo "x=1" >> "$GITHUB_OUTPUT"
      - run: echo "X=${{ steps.other.outputs.x }}" >> "$GITHUB_ENV"
`
	if got := countGitHubEnv(t, "on:\n  workflow_run:\n    workflows: [CI]\n"+src[len("on: workflow_run\n  workflows: [CI]\n"):]); got != 1 {
		t.Errorf("%d findings, want 1 (only the output of the step that is not a token)", got)
	}
}

func countGitHubEnv(t *testing.T, src string) int {
	t.Helper()
	n := 0
	for _, e := range lintBatchD(t, "test.yaml", src, allBatchDRules()) {
		if e.ID == "github-env" {
			n++
		}
	}
	return n
}

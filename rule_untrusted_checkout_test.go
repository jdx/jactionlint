package jactionlint

import (
	"strings"
	"testing"
)

func TestRuleUntrustedCheckout(t *testing.T) {
	const prt = "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n"
	tests := []struct {
		what   string
		others map[string]string
		src    string
	}{
		{"local action below the checkout directory", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }} # want
          path: pr
      - uses: ./pr/.github/actions/build
`},
		{"local action of the base repository next to a checkout in a subdirectory", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
          path: pr
      - uses: ./.github/actions/label
`},
		{"workflow env", nil, "on: pull_request_target\nenv:\n  HEAD: ${{ github.event.pull_request.head.sha }}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: | # want\n          git fetch origin \"$HEAD\"\n          git checkout \"$HEAD\"\n      - run: npm ci\n"},
		{"guarded step that runs the code", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci
        if: contains(github.event.pull_request.labels.*.name, 'safe to test')
`},
		{"head sha then npm", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }} # want
      - run: npm ci
`},
		{"head ref then make", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.ref }} # want
      - run: make test
`},
		{"quoted expression and a different spelling of the action", nil, prt + `    steps:
      - uses: Actions/Checkout@v4
        with:
          ref: "${{ github.event.pull_request.head.sha }}" # want
      - run: ./build.sh
`},
		{"enable -f loads a shared object of the checkout", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }} # want
      - run: enable -f ./mod.so mod
`},
		{"enable -n runs nothing", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: enable -n test
`},
		{"head_ref", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.head_ref }} # want
      - run: cargo build
`},
		{"merge ref", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: refs/pull/${{ github.event.number }}/merge # want
      - run: pip install .
`},
		{"merge commit sha", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.merge_commit_sha }} # want
      - run: go test ./...
`},
		{"repository of the fork", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          repository: ${{ github.event.pull_request.head.repo.full_name }} # want
      - uses: ./.github/actions/setup
`},
		{"gh pr checkout", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: gh pr checkout ${{ github.event.number }} # want
      - run: npm test
`},
		{"fetch and build in one script", nil, prt + `    steps:
      - run: | # want
          git fetch origin ${{ github.event.pull_request.head.sha }}
          git checkout FETCH_HEAD
          make
`},
		{"head through an environment variable", nil, prt + `    steps:
      - run: git checkout "$HEAD" # want
        env:
          HEAD: ${{ github.event.pull_request.head.sha }}
      - run: npm ci
`},
		{"checkout in a directory and a build in it", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }} # want
          path: pr
      - run: npm ci
        working-directory: pr
`},
		{"checkout in a directory and cd into it", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }} # want
          path: ./pr/
      - run: cd pr && npm ci
`},
		{"workflow_run without the project", nil, `on:
  workflow_run:
    workflows: [Build]
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.workflow_run.head_sha }} # want
      - run: npm ci
`},
		{"workflow_run of a pull request workflow", map[string]string{"build.yml": "name: Build\non: pull_request\njobs: {}\n"}, `on:
  workflow_run:
    workflows: [Build]
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.workflow_run.head_sha }} # want
      - run: npm ci
`},
		{"workflow_run of a workflow that is not in the project", map[string]string{"build.yml": "name: Other\non: push\njobs: {}\n"}, `on:
  workflow_run:
    workflows: [Build]
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.workflow_run.head_sha }} # want
      - run: npm ci
`},
		{"checkout in a directory of a Windows runner", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }} # want
          path: .\pr\code
      - run: cd pr\code; npm ci
        shell: pwsh
`},
		{"workflow_run only for pull requests is no guard", nil, `on:
  workflow_run:
    workflows: [Build]
jobs:
  j:
    if: github.event.workflow_run.event == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.workflow_run.head_sha }} # want
      - run: npm ci
`},

		{"fetch of the head followed by a checkout of it", nil, prt + `    steps:
      - run: | # want
          git fetch origin "pull/${{ github.event.number }}/head" refs/pull/1/head
          git checkout FETCH_HEAD
      - run: make
`},
		{"head checkout in a job whose token has no scope", nil, prt + `    permissions: {}
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }} # want
      - run: make
`},
		{"interpreter that runs the checkout as its module", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }} # want
          path: pr
      - run: python3 -m pytest pr
`},

		// Not reported
		{"base checkout", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: npm ci
`},
		{"base sha", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.base.sha }}
      - run: npm ci
`},
		{"head is only read", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: git diff --stat origin/main
      - run: cat README.md | grep -c todo
      - run: gh pr comment ${{ github.event.number }} --body ok
        env:
          GH_TOKEN: ${{ github.token }}
`},
		{"head in a directory that nothing runs", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
          path: pr
      - run: npm ci
      - run: diff -r pr/docs docs
`},
		{"checkout and nothing else", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
`},
		{"pull_request is not privileged", nil, `on: pull_request
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci
`},
		{"same repository guard", nil, `on: pull_request_target
jobs:
  j:
    if: github.event.pull_request.head.repo.full_name == github.repository
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci
`},
		{"label guard on a step", nil, prt + `    steps:
      - uses: actions/checkout@v4
        if: contains(github.event.pull_request.labels.*.name, 'safe to test')
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci
`},
		{"environment with reviewers", nil, `on: pull_request_target
jobs:
  j:
    environment: untrusted
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci
`},
		{"workflow_run of a push workflow", map[string]string{"build.yml": "name: Build\non:\n  push:\n    branches: [main]\n  workflow_dispatch:\njobs: {}\n"}, `on:
  workflow_run:
    workflows: [Build]
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.workflow_run.head_sha }}
      - run: npm ci
`},
		{"workflow_run guarded by the event", nil, `on:
  workflow_run:
    workflows: [Build]
jobs:
  j:
    if: github.event.workflow_run.event == 'push'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.workflow_run.head_sha }}
      - run: npm ci
`},
		{"fetch of the head that is only diffed", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          git fetch origin "refs/pull/${{ github.event.number }}/head"
          git diff --stat HEAD...FETCH_HEAD
          for f in a b; do
            if [ "$f" = a ]; then continue; fi
            break
          done
          compgen -c > /dev/null
          xxd -l 4 file
      - run: npm ci
`},
		{"fetch in one step, checkout of FETCH_HEAD in the next", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: git fetch origin ${{ github.event.pull_request.head.sha }} # want
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"fetch into a branch in one step, checkout of it in the next", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: git fetch origin refs/pull/${{ github.event.number }}/head:pr # want
      - run: git checkout pr
      - run: make
`},
		{"fetch in one step, checkout of the base branch in the next", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: git fetch origin ${{ github.event.pull_request.head.sha }}
      - run: git checkout main
      - run: make
`},
		{"fetch in one step, FETCH_HEAD only diffed in the next", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: git fetch origin ${{ github.event.pull_request.head.sha }}
      - run: git diff HEAD FETCH_HEAD
      - run: make
`},
		{"head checkout in a directory that only a script of the base reads", nil, prt + `    steps:
      - uses: actions/checkout@v4
        with:
          path: base
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
          path: pr-head
      - run: python3 base/scripts/check.py --root pr-head
      - run: sudo apt-get install -y valgrind
`},
		{"fetch, then a script of the repository that checks out FETCH_HEAD", map[string]string{"../../scripts/co.sh": "#!/bin/bash\ngit checkout FETCH_HEAD\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: git fetch origin ${{ github.event.pull_request.head.sha }} # want
      - run: ./scripts/co.sh
`},
		{"fetch, then bash on a script that checks out the destination", map[string]string{"../../scripts/co.sh": "git checkout pr\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: git fetch origin refs/pull/${{ github.event.number }}/head:pr # want
      - run: bash scripts/co.sh
`},
		{"fetch, then a script that does not use the fetch", map[string]string{"../../scripts/co.sh": "git checkout main\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: git fetch origin ${{ github.event.pull_request.head.sha }}
      - run: ./scripts/co.sh
`},
		{"fetch, then a script that is not in the project", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: git fetch origin ${{ github.event.pull_request.head.sha }}
      - run: ./scripts/missing.sh
`},
		// the revision reaches the working tree through a shell variable
		{"variable from the step env, fetched then checked out", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          REF=$PR_REF
          git fetch origin "$REF"
          git checkout FETCH_HEAD
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"variable set from an expression, fetched then checked out", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          REF="${{ github.event.pull_request.head.sha }}"
          git fetch origin "$REF"
          git checkout FETCH_HEAD
      - run: make
`},
		{"variable passed on twice and used in the next step", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          A=$PR_REF
          B="$A"
          git fetch origin "$B"
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"variable written to GITHUB_ENV and fetched in a later step", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: echo "PR_REF=${{ github.event.pull_request.head.sha }}" >> "$GITHUB_ENV"
      - run: git fetch origin "$PR_REF" # want
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"variable from a for loop", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          for r in "$PR_REF"; do
            git fetch origin "$r"
          done
          git checkout FETCH_HEAD
          npm ci
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
`},
		{"variable reset to a constant before the fetch", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          REF=$PR_REF
          REF=main
          git fetch origin "$REF"
          git checkout FETCH_HEAD
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"variable of the base revision", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          REF=$BASE
          git fetch origin "$REF"
          git checkout FETCH_HEAD
        env:
          BASE: ${{ github.event.pull_request.base.sha }}
      - run: make
`},
		{"variable that is only compared", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          REF=$PR_REF
          git fetch origin main
          git checkout FETCH_HEAD
          test "$REF" = x
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		// a script file does the checkout itself
		{"script that checks out its first argument, given the head", map[string]string{"../../scripts/co.sh": "#!/bin/bash\ngit checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "${{ github.event.pull_request.head.sha }}" # want
      - run: make
`},
		{"script that checks out the variable of the step env", map[string]string{"../../scripts/co.sh": "git checkout \"$PR_REF\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: bash scripts/co.sh # want
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"script that fetches and checks out the argument", map[string]string{"../../scripts/co.sh": "REF=$1\ngit fetch origin \"$REF\"\ngit checkout FETCH_HEAD\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "$PR_REF" # want
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"script that runs the code it checked out", map[string]string{"../../scripts/co.sh": "git checkout \"$1\"\nnpm ci\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "${{ github.event.pull_request.head.sha }}" # want
`},
		{"script that uses gh pr checkout", map[string]string{"../../scripts/co.sh": "gh pr checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "${{ github.event.number }}" # want
      - run: make
`},
		{"script that checks out its first argument, given a constant", map[string]string{"../../scripts/co.sh": "git checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh main
      - run: make
`},
		{"script that checks out its first argument, given the base", map[string]string{"../../scripts/co.sh": "git checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "${{ github.event.pull_request.base.sha }}"
      - run: make
`},
		{"script that checks out its first argument, given nothing", map[string]string{"../../scripts/co.sh": "git checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh
      - run: make
`},
		{"script that checks out the head and nothing runs it afterwards", map[string]string{"../../scripts/co.sh": "git checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "${{ github.event.pull_request.head.sha }}"
`},
		{"script that starts a second script that checks out its argument", map[string]string{"../../scripts/co.sh": "./scripts/inner.sh \"$1\"\n", "../../scripts/inner.sh": "git checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "${{ github.event.pull_request.head.sha }}" # want
      - run: make
`},
		// an inherited untrusted variable that the script overwrites is no longer untrusted
		{"step env variable overwritten by a literal before the fetch", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          PR_REF=main
          git fetch origin "$PR_REF"
          git checkout FETCH_HEAD
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"step env variable overwritten conditionally before the fetch", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          if [ -n "$X" ]; then PR_REF=main; fi
          git fetch origin "$PR_REF"
          git checkout FETCH_HEAD
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"GITHUB_ENV variable overwritten with a clean value in a later step", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: echo "PR_REF=${{ github.event.pull_request.head.sha }}" >> "$GITHUB_ENV"
      - run: echo "PR_REF=main" >> "$GITHUB_ENV"
      - run: git fetch origin "$PR_REF"
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"GITHUB_ENV variable overwritten by the env of the step", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: echo "PR_REF=${{ github.event.pull_request.head.sha }}" >> "$GITHUB_ENV"
      - run: |
          git fetch origin "$PR_REF"
          git checkout FETCH_HEAD
        env:
          PR_REF: main
      - run: make
`},
		{"script given a variable that the step exports as a literal", map[string]string{"../../scripts/co.sh": "git checkout \"$PR_REF\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          export PR_REF=main
          ./scripts/co.sh
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"script given a variable that a prefix assignment sets to a literal", map[string]string{"../../scripts/co.sh": "git checkout \"$PR_REF\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: PR_REF=main ./scripts/co.sh
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"script given a variable that the step reassigned to a literal", map[string]string{"../../scripts/co.sh": "git checkout \"$PR_REF\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          PR_REF=main
          ./scripts/co.sh
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"child env: step env REF clean, reassigned from the head, then the script", map[string]string{"../../scripts/co.sh": "git checkout \"$REF\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          REF=$PR_REF
          ./scripts/co.sh
        env:
          REF: main
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"child env: the prefix assignment wins over an earlier export", map[string]string{"../../scripts/co.sh": "git checkout \"$REF\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          export REF=main
          REF=$PR_REF ./scripts/co.sh
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"child env: an unexported variable does not reach the script", map[string]string{"../../scripts/co.sh": "git checkout \"$REF\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          REF=$PR_REF
          ./scripts/co.sh
      - run: make
`},
		{"nested script given a literal does not inherit the parent's argument", map[string]string{"../../scripts/co.sh": "./scripts/inner.sh main\n", "../../scripts/inner.sh": "git checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "${{ github.event.pull_request.head.sha }}"
      - run: make
`},
		{"GITHUB_ENV: echo of several words writes one entry for the first", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: echo "REF=main" >> "$GITHUB_ENV"
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: echo "REF=${{ github.event.pull_request.head.sha }}" >> "$GITHUB_ENV"
      - run: echo "NOTE=building" "REF=main" >> "$GITHUB_ENV"
      - run: git fetch origin "$REF" # want
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"GITHUB_ENV: echo with a flag then a clean write of the variable", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: echo -n "REF=${{ github.event.pull_request.head.sha }}" >> "$GITHUB_ENV"
      - run: echo -e "REF=main" >> "$GITHUB_ENV"
      - run: git fetch origin "$REF"
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"GITHUB_ENV: printf with the entry in the format", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: printf 'REF=%s\n' "$PR_REF" >> "$GITHUB_ENV"
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: git fetch origin "$REF" # want
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"GITHUB_ENV: printf %s with the entry in the argument", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: printf '%s\n' "REF=${{ github.event.pull_request.head.sha }}" >> "$GITHUB_ENV"
      - run: git fetch origin "$REF" # want
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"GITHUB_ENV: printf with a literal replaces the variable", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: echo "REF=${{ github.event.pull_request.head.sha }}" >> "$GITHUB_ENV"
      - run: printf 'REF=%s\n' main >> "$GITHUB_ENV"
      - run: git fetch origin "$REF"
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"GITHUB_ENV: here document with the head", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          cat <<EOF >> "$GITHUB_ENV"
          REF=${{ github.event.pull_request.head.sha }}
          EOF
      - run: git fetch origin "$REF" # want
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"GITHUB_ENV: here document with a delimiter entry, replaced by a literal", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          {
            echo "REF<<EOF"
            echo "${{ github.event.pull_request.head.sha }}"
            echo "EOF"
          } >> "$GITHUB_ENV"
      - run: |
          cat <<EOF >> "$GITHUB_ENV"
          REF=main
          EOF
      - run: git fetch origin "$REF"
      - run: git checkout FETCH_HEAD
      - run: make
`},
		{"script: execution before the checkout is not the code of the pull request", map[string]string{"../../scripts/co.sh": "git fetch origin \"$1\"\nnpm ci\ngit checkout FETCH_HEAD\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "${{ github.event.pull_request.head.sha }}"
`},
		{"script: execution after the checkout is reported", map[string]string{"../../scripts/co.sh": "git fetch origin \"$1\"\ngit checkout FETCH_HEAD\nnpm ci\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/co.sh "${{ github.event.pull_request.head.sha }}" # want
`},
		{"same script: npm before the checkout, nothing after", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          git fetch origin "$PR_REF"
          npm ci
          git checkout FETCH_HEAD
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
`},
		{"same script: npm after the checkout", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          git fetch origin "$PR_REF"
          git checkout FETCH_HEAD
          npm ci
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
`},
		{"cd into the directory of the script", map[string]string{"../../scripts/co.sh": "git checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          cd scripts
          bash co.sh "$PR_REF"
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"cd into another directory than the one of the script", map[string]string{"../../co.sh": "git checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          cd scripts
          bash co.sh "$PR_REF"
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"cd that is conditional makes the directory unknown", map[string]string{"../../scripts/co.sh": "git checkout \"$1\"\n"}, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          if [ -n "$X" ]; then cd scripts; fi
          bash co.sh "$PR_REF"
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"a long chain of assignments", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: | # want
          A=$PR_REF; B=$A; C=$B; D=$C; E=$D; F=$E
          git fetch origin "$F"
          git checkout FETCH_HEAD
        env:
          PR_REF: ${{ github.event.pull_request.head.sha }}
      - run: make
`},
		{"a doubling chain of assignments ends", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: |
          A=x; B=$A$A; C=$B$B; D=$C$C; E=$D$D; F=$E$E; G=$F$F; H=$G$G; I=$H$H; J=$I$I; K=$J$J; L=$K$K; M=$L$L
          git fetch origin "$M"
          git checkout FETCH_HEAD
      - run: make
`},
		{"GITHUB_ENV: printf with a format that is not understood keeps the value", nil, prt + `    steps:
      - uses: actions/checkout@v4
      - run: echo "REF=${{ github.event.pull_request.head.sha }}" >> "$GITHUB_ENV"
      - run: printf 'REF=%5.1f\n' 1 >> "$GITHUB_ENV"
      - run: git fetch origin "$REF" # want
      - run: git checkout FETCH_HEAD
      - run: make
`},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := onlyID(lintInProject(t, tc.others, tc.src, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout")
			checkLines(t, errs, markedWantLines(tc.src)...)
			crlf := strings.ReplaceAll(tc.src, "\n", "\r\n")
			checkLines(t, onlyID(lintInProject(t, tc.others, crlf, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout"), markedWantLines(tc.src)...)
		})
	}
}

func TestRuleUntrustedCheckoutNoTokenMessage(t *testing.T) {
	const src = "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n      - run: make\n"
	errs := onlyID(lintInProject(t, nil, src, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout")
	if len(errs) != 1 {
		t.Fatalf("want one finding, got %v", errs)
	}
	if strings.Contains(errs[0].Message, "write token") || !strings.Contains(errs[0].Message, "GITHUB_TOKEN has no permissions") {
		t.Fatalf("message claims a token: %s", errs[0].Message)
	}
}

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

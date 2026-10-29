---
layout: page
sidebar: false
title: Lint GitHub Actions workflows for bugs and security holes
description: jactionlint checks GitHub Actions workflows, composite actions and dependabot.yml. It type-checks expressions, validates action inputs, runs shellcheck on scripts, finds script injection, risky triggers and excessive permissions, and fixes what it can.
---

<!--
The page is drawn by theme/HomePage.vue, and the code blocks below fill its slots. TestDocsDemoIsCurrent
(docs_demo_test.go) runs jactionlint on the demo:input block and compares its output with the demo:* blocks here and in
README.md, and home.data.ts reads the findings of demo:output to mark them in the workflow. The release workflow
updates the pin of jdx/jactionlint in the CI workflow.
-->

<HomePage>
<template v-slot:workflow>

<!-- demo:input -->
```yaml
on:
  push:
    branch: main
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo "${{ github.event.head_commit.message }}"
      - run: echo "${{ matrix.os }}"
```

</template>
<template v-slot:output>

<!-- demo:output -->
```jactionlint-output
test.yaml:1:1: workflow has no "concurrency:", so every run of it executes at the same time even when a newer run supersedes the older ones. add a top-level "concurrency:" with a "group:" and "cancel-in-progress: true" [concurrency-limits]
  |
1 | on:
  | ^~~
test.yaml:3:5: unexpected key "branch" for "push" section. expected one of "branches", "branches-ignore", "paths", "paths-ignore", "tags", "tags-ignore", "types", "workflows" [workflow-syntax]
  |
3 |     branch: main
  |     ^~~~~~~
test.yaml:5:3: "timeout-minutes" is not set at this job. Set it to avoid wasting runner minutes when the job hangs. Set "default-minutes" in the "missing-timeout" rule options of the configuration to turn on the --fix fix [missing-timeout]
  |
5 |   test:
  |   ^~~~~
test.yaml:5:3: neither the workflow nor this job sets "permissions:" so the GITHUB_TOKEN gets the default permissions of the repository. set "permissions:" at workflow-level or job-level (use "permissions: {}" for no permissions) because the "missing-permissions" rule is enabled [missing-permissions]
  |
5 |   test:
  |   ^~~~~
test.yaml:8:15: action "actions/checkout@v4" must be pinned to a full-length commit SHA like "{owner}/{repo}@{sha}" because the "unpinned-uses" rule is enabled [unpinned-uses]
  |
8 |       - uses: actions/checkout@v4
  |               ^~~~~~~~~~~~~~~~~~~
test.yaml:8:15: actions/checkout leaves the GITHUB_TOKEN in the git config of the workspace, where a later step such as an artifact upload can publish it. set "persist-credentials: false" under "with:" unless a later step needs to push [artipacked]
  |
8 |       - uses: actions/checkout@v4
  |               ^~~~~~~~~~~~~~~~~~~
test.yaml:9:24: "github.event.head_commit.message" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
  |
9 |       - run: echo "${{ github.event.head_commit.message }}"
  |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:10:24: property "os" is not defined in object type {} [undefined-property]
   |
10 |       - run: echo "${{ matrix.os }}"
   |                        ^~~~~~~~~
```

</template>
<template v-slot:diff>

<!-- demo:diff -->
```diff
--- a/test.yaml
+++ b/test.yaml
@@ -1,10 +1,17 @@
 on:
   push:
     branch: main
+
+permissions:
+  contents: read
 jobs:
   test:
     runs-on: ubuntu-latest
     steps:
       - uses: actions/checkout@v4
-      - run: echo "${{ github.event.head_commit.message }}"
+        with:
+          persist-credentials: false
+      - run: echo "${HEAD_COMMIT_MESSAGE}"
+        env:
+          HEAD_COMMIT_MESSAGE: ${{ github.event.head_commit.message }}
       - run: echo "${{ matrix.os }}"
```

</template>
<template v-slot:ci>

```yaml
name: jactionlint
on:
  push:
    branches: [main]
  pull_request:
permissions: {}
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          persist-credentials: false
      - uses: jdx/jactionlint@baa8d685e2b27ff5821558c9eeabb2051c1da61b # v2.4.0
```

</template>
</HomePage>

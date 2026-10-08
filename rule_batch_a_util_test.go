package jactionlint

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// richExprWorkflow has a "${{ secrets.Mnn }}" in every field of the AST which can hold an expression.
// Each marker is unique, so a field which workflowExprSites skips shows up by name.
const richExprWorkflow = `
name: ${{ secrets.M01 }}
run-name: ${{ secrets.M02 }}
on:
  workflow_call:
    outputs:
      out:
        value: ${{ secrets.M03 }}
env:
  A: ${{ secrets.M04 }}
defaults:
  run:
    shell: ${{ secrets.M05 }}
    working-directory: ${{ secrets.M06 }}
concurrency:
  group: ${{ secrets.M07 }}
  cancel-in-progress: ${{ secrets.M08 }}
jobs:
  build:
    name: ${{ secrets.M09 }}
    if: secrets.M10
    runs-on: ${{ secrets.M11 }}
    environment:
      name: ${{ secrets.M12 }}
      url: ${{ secrets.M13 }}
    concurrency:
      group: ${{ secrets.M14 }}
    outputs:
      o: ${{ secrets.M15 }}
    env:
      B: ${{ secrets.M16 }}
    defaults:
      run:
        shell: ${{ secrets.M17 }}
    timeout-minutes: ${{ secrets.M18 }}
    continue-on-error: ${{ secrets.M19 }}
    strategy:
      fail-fast: ${{ secrets.M20 }}
      max-parallel: ${{ secrets.M21 }}
      matrix:
        os: ["${{ secrets.M22 }}", plain]
        deep:
          - key: "${{ secrets.M23 }}"
          - [x, "${{ secrets.M24 }}"]
        include:
          - os: "${{ secrets.M25 }}"
            extra: "${{ secrets.M26 }}"
        exclude:
          - os: "${{ secrets.M27 }}"
    container:
      image: ${{ secrets.M28 }}
      credentials:
        username: ${{ secrets.M29 }}
        password: ${{ secrets.M30 }}
      env:
        C: ${{ secrets.M31 }}
      ports:
        - ${{ secrets.M32 }}
      volumes:
        - ${{ secrets.M33 }}
      options: ${{ secrets.M34 }}
    services:
      db:
        image: ${{ secrets.M35 }}
        command: ${{ secrets.M36 }}
        entrypoint: ${{ secrets.M37 }}
    steps:
      - name: ${{ secrets.M38 }}
        if: secrets.M39
        env:
          D: ${{ secrets.M40 }}
        continue-on-error: ${{ secrets.M41 }}
        timeout-minutes: ${{ secrets.M42 }}
        run: echo ${{ secrets.M43 }}
        shell: ${{ secrets.M44 }}
        working-directory: ${{ secrets.M45 }}
      - uses: ${{ secrets.M46 }}
        with:
          k: ${{ secrets.M47 }}
      - uses: docker://alpine
        with:
          entrypoint: ${{ secrets.M48 }}
          args: ${{ secrets.M49 }}
  matrix-expr:
    runs-on: ubuntu-latest
    strategy:
      matrix: ${{ secrets.M50 }}
    steps:
      - run: echo
  matrix-rows:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        a: ${{ secrets.M51 }}
        include: ${{ secrets.M52 }}
        exclude:
          - ${{ secrets.M53 }}
    steps:
      - run: echo
  snap:
    runs-on: ubuntu-latest
    snapshot:
      image-name: ${{ secrets.M54 }}
      version: ${{ secrets.M55 }}
      if: secrets.M56
    steps:
      - run: echo
  call:
    uses: ./.github/workflows/x.yaml
    with:
      i: ${{ secrets.M57 }}
    secrets:
      s: ${{ secrets.M58 }}
`

func TestWorkflowExprSitesVisitEveryExpression(t *testing.T) {
	w, errs := Parse([]byte(richExprWorkflow))
	if w == nil {
		t.Fatalf("could not parse the fixture: %v", errs)
	}
	marker := regexp.MustCompile(`M\d\d`)
	want := map[string]bool{}
	for _, m := range marker.FindAllString(richExprWorkflow, -1) {
		want[m] = true
	}
	got := map[string]bool{}
	workflowExprs(w, func(_ exprSite, o *exprOccurrence) {
		VisitExprNode(o.Root, func(n, _ ExprNode, entering bool) {
			if name, ok := secretNameOf(n); ok && entering {
				got[strings.ToUpper(name)] = true
			}
		})
	})
	var missing []string
	for m := range want {
		if !got[m] {
			missing = append(missing, m)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		// Name the lines to find the field fast
		var lines []string
		src := strings.Split(richExprWorkflow, "\n")
		for _, m := range missing {
			for i, l := range src {
				if strings.Contains(l, m) {
					lines = append(lines, fmt.Sprintf("%s (fixture line %d: %s)", m, i+1, strings.TrimSpace(l)))
				}
			}
		}
		t.Fatalf("workflowExprSites does not visit %d expression(s):\n%s", len(missing), strings.Join(lines, "\n"))
	}
}

// A finding in a matrix value points at the secret whether the value is plain, quoted, a block
// scalar or an item of a flow sequence.
func TestMatrixExprPositions(t *testing.T) {
	src := "on: push\n" + // 1
		"jobs:\n" + // 2
		"  a:\n" + // 3
		"    runs-on: ubuntu-latest\n" + // 4
		"    strategy:\n" + // 5
		"      matrix:\n" + // 6
		"        plain:\n" + // 7
		"          - ${{ secrets.PLAIN }}\n" + // 8
		"        double:\n" + // 9
		"          - \"${{ secrets.DOUBLE }}\"\n" + // 10
		"        single:\n" + // 11
		"          - '${{ secrets.SINGLE }}'\n" + // 12
		"        block:\n" + // 13
		"          - |\n" + // 14
		"            x ${{ secrets.BLOCK }}\n" + // 15
		"        flow: ['${{ secrets.FLOW }}', \"${{ secrets.FLOWQ }}\"]\n" + // 16
		"        include:\n" + // 17
		"          - k: ${{ secrets.INC }}\n" + // 18
		"            q: \"${{ secrets.INCQ }}\"\n" + // 19
		"        exclude:\n" + // 20
		"          - k: '${{ secrets.EXC }}'\n" + // 21
		"    steps:\n" + // 22
		"      - run: echo\n" // 23
	lines := strings.Split(src, "\n")
	cfg := mustParseConfig(t, "rules:\n  secrets-outside-env: error\n")
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "secrets-outside-env")
	got := map[string][2]int{}
	for _, e := range errs {
		m := regexp.MustCompile(`secret "([A-Z]+)"`).FindStringSubmatch(e.Message)
		if m == nil {
			t.Fatalf("unexpected message %q", e.Message)
		}
		got[m[1]] = [2]int{e.Line, e.Column}
	}
	for name, line := range map[string]int{"PLAIN": 8, "DOUBLE": 10, "SINGLE": 12, "BLOCK": 15, "FLOW": 16, "FLOWQ": 16, "INC": 18, "INCQ": 19, "EXC": 21} {
		want := [2]int{line, strings.Index(lines[line-1], "secrets."+name) + 1}
		if g, ok := got[name]; !ok || g != want {
			t.Errorf("%s: want line:col %v but got %v (reported: %v)", name, want, g, ok)
		}
	}
}

package runner

import (
	"strings"
	"testing"

	"github.com/jdx/jactionlint/v2"
)

func planOf(t *testing.T, src string, only ...string) *Plan {
	t.Helper()
	w, errs := jactionlint.Parse([]byte(src))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	p, err := BuildPlan(w, "test.yml", only, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func cellNames(pj *PlannedJob) []string {
	var ns []string
	for _, c := range pj.Cells {
		ns = append(ns, c.Name)
	}
	return ns
}

func TestMatrixExpansion(t *testing.T) {
	tests := []struct {
		name   string
		matrix string
		want   []string
	}{
		{
			"product keeps declaration order",
			`os: [ubuntu, macos]
        node: [18, 20]`,
			[]string{"j (ubuntu, 18)", "j (ubuntu, 20)", "j (macos, 18)", "j (macos, 20)"},
		},
		{
			"exclude removes matching combinations",
			`os: [ubuntu, macos]
        node: [18, 20]
        exclude:
          - os: macos
            node: 18`,
			[]string{"j (ubuntu, 18)", "j (ubuntu, 20)", "j (macos, 20)"},
		},
		{
			"exclude with a subset of keys",
			`os: [ubuntu, macos]
        node: [18, 20]
        exclude:
          - os: macos`,
			[]string{"j (ubuntu, 18)", "j (ubuntu, 20)"},
		},
		{
			"exclude compares strings without case",
			`os: [Ubuntu, macos]
        exclude:
          - os: ubuntu`,
			[]string{"j (macos)"},
		},
		{
			"include adds to every combination it does not overwrite",
			`os: [ubuntu, macos]
        include:
          - os: ubuntu
            experimental: true`,
			[]string{"j (ubuntu, true)", "j (macos)"},
		},
		{
			"include that overwrites an original value makes a new combination",
			`os: [ubuntu]
        node: [18]
        include:
          - os: ubuntu
            node: 22`,
			[]string{"j (ubuntu, 18)", "j (ubuntu, 22)"},
		},
		{
			"include without original keys joins every combination",
			`os: [ubuntu, macos]
        include:
          - color: green`,
			[]string{"j (ubuntu, green)", "j (macos, green)"},
		},
		{
			"include only: each entry is a job",
			`include:
          - flavor: scratch
          - flavor: debian`,
			[]string{"j (scratch)", "j (debian)"},
		},
		{
			"added values can be overwritten by a later include",
			`os: [ubuntu]
        include:
          - os: ubuntu
            color: green
          - os: ubuntu
            color: red`,
			[]string{"j (ubuntu, red)"},
		},
		{
			"exclude runs before include",
			`os: [ubuntu, macos]
        exclude:
          - os: macos
        include:
          - os: macos
            extra: yes-please`,
			[]string{"j (ubuntu)", "j (macos, yes-please)"},
		},
		{
			"numbers print like GitHub prints them",
			`python: [3.10, "3.10", 3.9, 1e3]`,
			[]string{"j (3.1)", "j (3.10)", "j (3.9)", "j (1000)"},
		},
		{
			"booleans and nulls",
			`flag: [true, false, null]`,
			[]string{"j (true)", "j (false)", "j ()"},
		},
		{
			"objects are left out of the name",
			`cfg:
          - {os: a}
          - {os: b}
        os: [x]`,
			[]string{"j (x)", "j (x)"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        " + tc.matrix + "\n    steps:\n      - run: echo\n"
			p := planOf(t, src)
			got := cellNames(p.Job("j"))
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("cells = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMatrixDynamic(t *testing.T) {
	for name, m := range map[string]string{
		"whole matrix": `matrix: ${{ fromJSON(needs.a.outputs.m) }}`,
		"row":          "matrix:\n        os: ${{ fromJSON(needs.a.outputs.os) }}",
		"include":      "matrix:\n        include: ${{ fromJSON(needs.a.outputs.inc) }}",
		"strategy":     `${{ fromJSON(needs.a.outputs.s) }}`,
	} {
		t.Run(name, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n  j:\n    needs: a\n    runs-on: ubuntu-latest\n    strategy: " + strings.TrimPrefix(m, "") + "\n    steps:\n      - run: echo\n"
			if name != "strategy" {
				src = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n  j:\n    needs: a\n    runs-on: ubuntu-latest\n    strategy:\n      " + m + "\n    steps:\n      - run: echo\n"
			}
			pj := planOf(t, src).Job("j")
			if pj.Dynamic == "" || len(pj.Cells) != 0 {
				t.Fatalf("Dynamic = %q, cells = %d", pj.Dynamic, len(pj.Cells))
			}
		})
	}
}

func TestPlanOrderAndSelection(t *testing.T) {
	src := `on: push
jobs:
  deploy:
    needs: [test, lint]
    runs-on: ubuntu-latest
    steps: [{run: echo}]
  test:
    needs: build
    runs-on: ubuntu-latest
    steps: [{run: echo}]
  build:
    runs-on: ubuntu-latest
    steps: [{run: echo}]
  lint:
    runs-on: ubuntu-latest
    steps: [{run: echo}]
`
	p := planOf(t, src)
	var order []string
	for _, pj := range p.Jobs {
		order = append(order, pj.ID)
	}
	if got, want := strings.Join(order, ","), "build,lint,test,deploy"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	if p.Job("DEPLOY").Stage != 3 || p.Job("build").Stage != 1 || p.Job("test").Stage != 2 {
		t.Fatalf("stages: %d %d %d", p.Job("DEPLOY").Stage, p.Job("build").Stage, p.Job("test").Stage)
	}

	p = planOf(t, src, "test")
	for id, want := range map[string]bool{"build": true, "test": true, "lint": false, "deploy": false} {
		if p.Job(id).Selected != want {
			t.Errorf("%s selected = %v, want %v", id, p.Job(id).Selected, want)
		}
	}

	w, _ := jactionlint.Parse([]byte(src))
	if _, err := BuildPlan(w, "x", []string{"nope"}, nil); err == nil || !strings.Contains(err.Error(), `no job "nope"`) {
		t.Fatalf("unknown job error = %v", err)
	}
}

func TestPlanCycleAndMissingNeed(t *testing.T) {
	cyc := "on: push\njobs:\n  a:\n    needs: b\n    runs-on: x\n    steps: [{run: e}]\n  b:\n    needs: a\n    runs-on: x\n    steps: [{run: e}]\n"
	w, _ := jactionlint.Parse([]byte(cyc))
	if _, err := BuildPlan(w, "x", nil, nil); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
	miss := "on: push\njobs:\n  a:\n    needs: zzz\n    runs-on: x\n    steps: [{run: e}]\n"
	w, _ = jactionlint.Parse([]byte(miss))
	if _, err := BuildPlan(w, "x", nil, nil); err == nil || !strings.Contains(err.Error(), `"zzz"`) {
		t.Fatalf("missing need error = %v", err)
	}
}

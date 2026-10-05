package actionlint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCacheMode(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "ok", "cache_mode.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow, errs := Parse(input)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if workflow.CacheMode == nil || workflow.CacheMode.Value != "read" {
		t.Fatalf("unexpected workflow cache mode: %+v", workflow.CacheMode)
	}
	for job, expected := range map[string]string{
		"restore":   "",
		"save":      "write",
		"save-only": "write-only",
		"no-cache":  "none",
		"reusable":  "read",
	} {
		mode := workflow.Jobs[job].CacheMode
		if expected == "" {
			if mode != nil {
				t.Errorf("unexpected cache mode for job %q: %+v", job, mode)
			}
		} else if mode == nil || mode.Value != expected {
			t.Errorf("unexpected cache mode for job %q: %+v", job, mode)
		}
	}
}

func BenchmarkParseWorkflow(b *testing.B) {
	type bench struct {
		name  string
		input []byte
	}

	loadBench := func(name string) bench {
		i, err := os.ReadFile(filepath.Join("testdata", "bench", name+".yaml"))
		if err != nil {
			b.Fatal(err)
		}
		return bench{name, i}
	}

	for _, bc := range []bench{
		loadBench("minimal"),
		loadBench("small"),
		loadBench("large"),
	} {
		b.Run(bc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, errs := Parse(bc.input); len(errs) > 0 {
					b.Fatal(errs)
				}
			}
		})
	}
}

func TestParseContainerVolumesAndPorts(t *testing.T) {
	src := `on: push
jobs:
  test:
    runs-on: ubuntu-latest
    container:
      image: node:18
      volumes:
        - /a:/b
      ports:
        - 80
    services:
      redis:
        image: redis
        ports:
          - 6379
        volumes:
          - /c:/d
          - /e:/f
    steps:
      - run: echo
`
	w, errs := Parse([]byte(src))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	j := w.Jobs["test"]
	check := func(what string, got []*String, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: wanted %v but got %d elements", what, want, len(got))
		}
		for i, w := range want {
			if got[i].Value != w {
				t.Errorf("%s[%d]: wanted %q but got %q", what, i, w, got[i].Value)
			}
		}
	}
	check("container.volumes", j.Container.Volumes, "/a:/b")
	check("container.ports", j.Container.Ports, "80")
	r := j.Services.Value["redis"].Container
	check("service.volumes", r.Volumes, "/c:/d", "/e:/f")
	check("service.ports", r.Ports, "6379")
}

func BenchmarkParseTestData(b *testing.B) {
	type bench struct {
		name   string
		inputs [][]byte
	}

	loadBench := func(name string) bench {
		inputs := [][]byte{}
		_, fs, err := testFindAllWorkflowsInDir(name)
		if err != nil {
			b.Fatal(err)
		}
		for _, f := range fs {
			bs, err := os.ReadFile(f)
			if err != nil {
				b.Fatal(err)
			}
			inputs = append(inputs, bs)
		}
		return bench{name, inputs}
	}

	for _, bc := range []bench{
		loadBench("examples"),
		loadBench("ok"),
		loadBench("err"),
	} {
		b.Run(bc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				for _, in := range bc.inputs {
					// Note: Some workflows may cause parse error
					Parse(in)
				}
			}
		})
	}
}

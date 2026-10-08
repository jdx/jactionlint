package main

import (
	"os"
	"reflect"
	"testing"
)

func TestParseSummary(t *testing.T) {
	var applied int
	got := parseSummary("noise\nFixed 12 problem(s) in 3 file(s)\n  missing-timeout: 8\n  artipacked: 4\nerror: x\n", &applied)
	if applied != 12 || !reflect.DeepEqual(got, map[string]int{"missing-timeout": 8, "artipacked": 4}) {
		t.Errorf("unexpected: %d %v", applied, got)
	}
}

func TestGoneUp(t *testing.T) {
	got := goneUp(map[string]int{"a": 2, "b": 1}, map[string]int{"a": 1, "b": 2, "c": 1})
	if !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("unexpected: %v", got)
	}
}

func TestValidYAML(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := dir + "/" + name
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := validYAML(write("ok.yml", "a: 1\n---\nb: 2\n")); err != nil {
		t.Error(err)
	}
	if err := validYAML(write("bad.yml", "a: [1\n")); err == nil {
		t.Error("invalid YAML must be reported")
	}
}

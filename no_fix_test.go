package jactionlint

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func lintForNoFix(t *testing.T, src string, cfg *Config) []*Error {
	t.Helper()
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	errs, err := l.Lint("w.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func findNoFix(errs []*Error, id string) *Error {
	for _, e := range errs {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func TestNoFixReasons(t *testing.T) {
	const src = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"
	errs := lintForNoFix(t, src, &Config{Rules: map[string]RuleConfig{"missing-timeout": {Level: SeverityError}, "unpinned-uses": {Level: SeverityError}}})

	if e := findNoFix(errs, "missing-timeout"); e == nil || e.Fix != nil || e.NoFix == nil || e.NoFix.Code != NoFixOptionRequired || e.NoFix.Option != "missing-timeout.default-minutes" {
		t.Errorf("missing-timeout: %+v", e)
	}
	if e := findNoFix(errs, "unpinned-uses"); e == nil || e.NoFix == nil || e.NoFix.Code != NoFixOnlineRequired {
		t.Errorf("unpinned-uses: %+v", e)
	}
	// An advice-only finding has no reason, which tells it from an unavailable fix
	for _, e := range errs {
		if e.ID == "" || e.Fix != nil {
			continue
		}
		if e.NoFix != nil && !strings.Contains("missing-timeout unpinned-uses", e.ID) {
			t.Errorf("%s has a NoFix: %+v", e.ID, e.NoFix)
		}
	}

	f := findNoFix(errs, "missing-timeout").GetTemplateFields([]byte(src))
	b, _ := json.Marshal(f)
	if !strings.Contains(string(b), `"no_fix":{"code":"option-required"`) {
		t.Errorf("JSON: %s", b)
	}
}

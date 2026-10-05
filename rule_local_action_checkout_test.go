package jactionlint

import (
	"io"
	"os"
	"testing"
)

func TestRuleLocalActionCheckoutDisabledByDefault(t *testing.T) {
	b, err := os.ReadFile("testdata/examples/local_action_before_checkout.yaml")
	if err != nil {
		t.Fatal(err)
	}
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{}
	errs, err := l.Lint("test.yaml", b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 0 {
		t.Fatalf("no error is expected when the option is not enabled but got: %v", errs)
	}
}

func TestRuleLocalActionCheckoutParseConfig(t *testing.T) {
	c, err := ParseConfig([]byte("require-checkout-before-local-action: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.RequireCheckoutBeforeLocalAction {
		t.Fatal("option was not parsed")
	}
}

func TestIsCheckoutActionSpec(t *testing.T) {
	tests := map[string]bool{
		"actions/checkout@v4":       true,
		"Actions/Checkout/sub@main": true,
		"foo/lfs-checkout@abc":      true,
		"actions/setup-node@v4":     false,
		"docker://checkout:latest":  false,
		"checkout":                  false,
		"./checkout":                false,
		"owner/repo/checkout@v1":    false,
	}
	for spec, want := range tests {
		if have := isCheckoutActionSpec(spec); have != want {
			t.Errorf("%q: want %v but have %v", spec, want, have)
		}
	}
}

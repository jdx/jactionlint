package jactionlint

import (
	"io"
	"os"
	"testing"
)

func TestRuleLocalActionCheckoutEnabledByDefaultAndCanBeTurnedOff(t *testing.T) {
	b, err := os.ReadFile("testdata/examples/local_action_before_checkout.yaml")
	if err != nil {
		t.Fatal(err)
	}
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(&Config{})
	errs, err := l.Lint("test.yaml", b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 {
		t.Fatal("the rule must be enabled by the default profile")
	}
	for _, e := range errs {
		if e.ID != "local-action-checkout" {
			t.Errorf("unexpected error %v", e)
		}
	}

	l.defaultConfig = ruleSwitch("local-action-checkout", false)
	errs, err = l.Lint("test.yaml", b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 0 {
		t.Fatalf("no error is expected when the rule is off but got: %v", errs)
	}
}

func TestRuleLocalActionCheckoutParseConfig(t *testing.T) {
	c := mustParseConfig(t, "rules:\n  local-action-checkout: error\n")
	if !c.RuleEnabled("local-action-checkout") {
		t.Fatal("option was not parsed")
	}
	// The deprecated key is translated to the rule
	c = mustParseConfig(t, "require-checkout-before-local-action: true\n")
	if !c.RuleEnabled("local-action-checkout") || len(c.Deprecations) != 1 {
		t.Fatalf("deprecated option was not translated: %+v", c)
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
		// A wrapper in the path of a remote repository may check out (pytorch/pytorch/.github/actions/checkout-pytorch)
		"owner/repo/checkout@v1":                                true,
		"pytorch/pytorch/.github/actions/checkout-pytorch@main": true,
		"owner/repo/.github/actions/setup@v1":                   false,
	}
	for spec, want := range tests {
		if have := isCheckoutActionSpec(spec); have != want {
			t.Errorf("%q: want %v but have %v", spec, want, have)
		}
	}
}

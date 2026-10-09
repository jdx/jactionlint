package jactionlint

import (
	"strings"
	"testing"
)

func TestTyposquatUses(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  typosquat-uses: {level: error, allow: [Acme/Setup-Node]}\n")
	tests := []struct {
		uses string
		want bool
	}{
		{"action/checkout@v4", true},
		{"actions/checkout@v4", false},
		{"actionss/checkout@v4", true},
		{"acitons/checkout@v4", true},
		{"dokcer/login-action@v3", true},
		{"docker/login-action/sub@v3", false},
		{"Docker/Login-Action@v3", false},
		{"dokcer/login-action/sub@v3", true},
		{"actions/chekout@v4", false},  // right owner
		{"someone/checkout@v4", false}, // far from any owner
		{"acme/setup-node@v4", false},  // allowed
		{"Acme/Setup-Node@v4", false},  // allowed
		{"actions/checkout/.github/workflows/x.yaml@v4", false},
		{"./actions/checkout", false},
		{"docker://action/checkout", false},
		{"ab/checkout@v4", false}, // very short owner
	}
	for _, tc := range tests {
		t.Run(tc.uses, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + tc.uses + "\n"
			got := len(errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "typosquat-uses")) > 0
			if got != tc.want {
				t.Errorf("want %v but got %v", tc.want, got)
			}
		})
	}

	t.Run("reusable workflow call", func(t *testing.T) {
		src := "on: push\njobs:\n  a:\n    uses: dokcer/login-action/.github/workflows/w.yaml@v3\n"
		if len(errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "typosquat-uses")) != 1 {
			t.Error("the reusable workflow should be reported")
		}
	})
}

func TestTyposquatPopularActions(t *testing.T) {
	slugs := popularActionSlugs()
	if len(slugs) < 50 {
		t.Fatalf("too few popular actions: %d", len(slugs))
	}
	rule := NewRuleTyposquatUses(nil)
	report := func(slug string) bool {
		rule.errs = nil
		rule.check(&String{Value: slug + "@v1", Pos: &Pos{Line: 1, Col: 1}})
		return len(rule.errs) > 0
	}
	for _, s := range slugs {
		// No popular action is a typo of another one
		if report(s) {
			t.Errorf("popular action %q is reported", s)
		}
		owner, repo, _ := strings.Cut(s, "/")
		// Removing a character of the owner is a typo as long as the result is not an owner of its own
		if len(owner) >= 5 {
			typo := owner[:len(owner)-1] + "/" + repo
			isPopular := false
			for _, o := range slugs {
				if o == typo {
					isPopular = true
				}
			}
			if !isPopular && !report(typo) {
				t.Errorf("%q should be reported as a typo of %q", typo, s)
			}
		}
	}
}

func TestWithinOneEdit(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"abc", "abc", false},
		{"abc", "abd", true},
		{"abc", "ab", true},
		{"ab", "abc", true},
		{"abc", "xbc", true},
		{"abc", "bac", true},
		{"abc", "acb", true},
		{"abc", "cba", false},
		{"abc", "a", false},
		{"", "a", true},
		{"", "", false},
		{"actions/checkout", "action/checkout", true},
		{"actions/checkout", "actoins/checkout", true},
		{"actions/checkout", "actoins/chekcout", false},
		{"日本語", "日本", true},
	}
	for _, tc := range tests {
		if got := withinOneEdit(tc.a, tc.b); got != tc.want {
			t.Errorf("withinOneEdit(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
		if got := withinOneEdit(tc.b, tc.a); got != tc.want {
			t.Errorf("withinOneEdit(%q, %q) = %v, want %v", tc.b, tc.a, got, tc.want)
		}
	}
}

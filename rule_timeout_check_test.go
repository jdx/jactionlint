package jactionlint

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lintTimeoutCheck(t *testing.T, src string, cfg *Config) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Only the rule of this test: the profile of the configuration decides which other rules run
	var ret []*Error
	for _, e := range errs {
		if e.Kind == "timeout-check" {
			ret = append(ret, e)
		}
	}
	return ret
}

func TestRuleTimeoutCheck(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "timeout_check", "required.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)

	tests := []struct {
		what string
		cfg  *Config
		want []string // substrings of messages, in order of lines
	}{
		{"nil config does not require the key under the correctness profile", nil, nil},
		{"the correctness profile does not require the key", &Config{Profile: ProfileCorrectness}, nil},
		{"default profile requires the key", &Config{Profile: ProfileDefault}, []string{`"timeout-minutes" is not set at this job`}},
		{"the rule can be turned off", mustParseConfig(t, "rules:\n  missing-timeout: off\n"), nil},
		{
			"required",
			mustParseConfig(t, "rules:\n  missing-timeout: error\n"),
			[]string{`"timeout-minutes" is not set at this job`},
		},
		{
			"required with max",
			mustParseConfig(t, "rules:\n  missing-timeout: error\n  timeout-too-long: {max: 30}\n"),
			[]string{`"timeout-minutes" is not set at this job`, `"timeout-minutes" is 45, which is greater than the maximum 30 minutes`},
		},
		{
			"max only does not require the key",
			mustParseConfig(t, "rules:\n  missing-timeout: off\n  timeout-too-long: {max: 30}\n"),
			[]string{`"timeout-minutes" is 45, which is greater than the maximum 30 minutes`},
		},
		{
			"max equal to value is allowed",
			mustParseConfig(t, "rules:\n  missing-timeout: off\n  timeout-too-long: {max: 45}\n"),
			nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := lintTimeoutCheck(t, src, tc.cfg)
			if len(errs) != len(tc.want) {
				t.Fatalf("wanted %d errors but got %d: %v", len(tc.want), len(errs), errs)
			}
			for i, e := range errs {
				if e.Kind != "timeout-check" {
					t.Errorf("unexpected kind %q: %v", e.Kind, e)
				}
				if !strings.Contains(e.Message, tc.want[i]) {
					t.Errorf("wanted %q in %q", tc.want[i], e.Message)
				}
			}
		})
	}
}

func TestRuleTimeoutCheckErrorPositions(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "timeout_check", "required.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	errs := lintTimeoutCheck(t, string(data), mustParseConfig(t, "rules:\n  missing-timeout: error\n  timeout-too-long: {max: 30}\n"))
	if len(errs) != 2 {
		t.Fatal(errs)
	}
	if errs[0].Line != 3 || errs[0].Column != 3 {
		t.Errorf("missing key should be reported at the job: %v", errs[0])
	}
	if errs[1].Line != 14 {
		t.Errorf("too large value should be reported at the value: %v", errs[1])
	}
}

func TestConfigTimeoutMinutes(t *testing.T) {
	c := mustParseConfig(t, "timeout-minutes:\n  required: true\n  max: 60\n")
	if m, ok := c.ruleOptionNumber("timeout-too-long", "max"); !c.RuleEnabled("missing-timeout") || !c.RuleEnabled("timeout-too-long") || !ok || m != 60 {
		t.Fatalf("not translated: %+v", c.Rules)
	}
	if len(c.Deprecations) != 1 {
		t.Fatalf("want a deprecation: %v", c.Deprecations)
	}
	for _, in := range []string{
		"timeout-minutes:\n  max: -1\n",
		"timeout-minutes:\n  max: .inf\n",
		"timeout-minutes:\n  max: .nan\n",
	} {
		if _, err := ParseConfig([]byte(in)); err == nil || !strings.Contains(err.Error(), `"max" in "timeout-minutes"`) {
			t.Errorf("wanted error for %q but got %v", in, err)
		}
	}
	if _, err := ParseConfig([]byte("timeout-minutes: true\n")); err == nil {
		t.Error("non-mapping value should be an error")
	}
}

// A job without timeout-minutes whose steps all set it is ghalint's exception (policy 012). It is
// accepted only with the accept-step-timeouts option, because the timeout of a step does not cover
// setting up the job or the post steps of actions.
func TestRuleTimeoutCheckStepTimeouts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "timeout_check", "step_timeouts.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	const hint = `Every step sets "timeout-minutes"`

	tests := []struct {
		what  string
		cfg   *Config
		lines []int // lines of the reported jobs
		hints []bool
	}{
		{
			"every step with a timeout is reported by default, with a hint about the option",
			mustParseConfig(t, "profile: default\n"),
			[]int{3, 10, 15, 21}, []bool{true, true, false, false},
		},
		{
			"every step with a timeout is reported in the pedantic profile too",
			mustParseConfig(t, "profile: pedantic\n"),
			[]int{3, 10, 15, 21}, []bool{true, true, false, false},
		},
		{
			"the option accepts a job whose steps all set timeout-minutes, an expression included",
			mustParseConfig(t, "profile: default\nrules:\n  missing-timeout:\n    accept-step-timeouts: true\n"),
			[]int{15, 21}, []bool{false, false},
		},
		{
			"the option can be false",
			mustParseConfig(t, "profile: default\nrules:\n  missing-timeout:\n    accept-step-timeouts: false\n"),
			[]int{3, 10, 15, 21}, []bool{true, true, false, false},
		},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := lintTimeoutCheck(t, src, tc.cfg)
			if len(errs) != len(tc.lines) {
				t.Fatalf("wanted %d errors but got %d: %v", len(tc.lines), len(errs), errs)
			}
			for i, e := range errs {
				if e.Line != tc.lines[i] {
					t.Errorf("error %d: wanted line %d but got %d: %v", i, tc.lines[i], e.Line, e)
				}
				if got := strings.Contains(e.Message, hint); got != tc.hints[i] {
					t.Errorf("error %d: hint present is %v, wanted %v: %q", i, got, tc.hints[i], e.Message)
				}
			}
		})
	}
}

func TestRuleTimeoutCheckStepTimeoutsFix(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "timeout_check", "step_timeouts.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	const rules = "profile: default\nrules:\n  missing-timeout:\n    default-minutes: 15\n"
	// Without the option the job is still reported, and the fix adds a job-level timeout
	errs := lintTimeoutCheck(t, string(data), mustParseConfig(t, rules))
	if len(errs) != 4 {
		t.Fatalf("wanted 4 errors: %v", errs)
	}
	for _, e := range errs {
		if e.Fix == nil {
			t.Errorf("wanted a fix for %v", e)
		}
	}
	// With the option the accepted jobs have neither a finding nor a fix
	errs = lintTimeoutCheck(t, string(data), mustParseConfig(t, rules+"    accept-step-timeouts: true\n"))
	if len(errs) != 2 {
		t.Fatalf("wanted 2 errors: %v", errs)
	}
	for _, e := range errs {
		if e.Line == 3 || e.Line == 10 {
			t.Errorf("a job whose steps all set timeout-minutes must be accepted: %v", e)
		}
	}
}

package actionlint

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
	return errs
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
		{"nil config", nil, nil},
		{"default config is opt-in", &Config{}, nil},
		{
			"required",
			&Config{TimeoutMinutes: TimeoutMinutesConfig{Required: true}},
			[]string{`"timeout-minutes" is not set at this job`},
		},
		{
			"required with max",
			&Config{TimeoutMinutes: TimeoutMinutesConfig{Required: true, Max: 30}},
			[]string{`"timeout-minutes" is not set at this job`, `"timeout-minutes" is 45, which is greater than the maximum 30 minutes`},
		},
		{
			"max only does not require the key",
			&Config{TimeoutMinutes: TimeoutMinutesConfig{Max: 30}},
			[]string{`"timeout-minutes" is 45, which is greater than the maximum 30 minutes`},
		},
		{
			"max equal to value is allowed",
			&Config{TimeoutMinutes: TimeoutMinutesConfig{Max: 45}},
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
	errs := lintTimeoutCheck(t, string(data), &Config{TimeoutMinutes: TimeoutMinutesConfig{Required: true, Max: 30}})
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
	c, err := ParseConfig([]byte("timeout-minutes:\n  required: true\n  max: 60\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.TimeoutMinutes.Required || c.TimeoutMinutes.Max != 60 {
		t.Fatalf("%+v", c.TimeoutMinutes)
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

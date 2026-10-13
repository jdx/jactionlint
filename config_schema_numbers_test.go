package jactionlint

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestConfigSchemaAgreesOnNumbers tries the boundaries of every numeric key: 0, 1, the largest value and one more, and
// negative values, in the parser and in the schema.
func TestConfigSchemaAgreesOnNumbers(t *testing.T) {
	v := loadJSONSchema(t, ConfigSchema())
	check := func(name, src string) {
		_, err := parseConfig([]byte(src))
		errs := validateConfigYAML(t, v, src)
		if (err == nil) != (len(errs) == 0) {
			t.Errorf("%s: the parser says %v and the schema %v for %q", name, err, errs, src)
		}
	}
	// online-options
	for _, key := range []struct {
		name string
		max  int
	}{{"retries", maxOnlineRetries}, {"concurrency", maxOnlineInFlight}} {
		for _, n := range []int{-1, 0, 1, key.max - 1, key.max, key.max + 1, 1000} {
			check("online-options."+key.name, fmt.Sprintf("online-options:\n  %s: %d\n", key.name, n))
		}
	}
	// the options of the rules
	for _, r := range ruleRegistry {
		for _, o := range r.Options {
			if o.Kind != RuleOptionInt && o.Kind != RuleOptionNumber {
				continue
			}
			for _, n := range []string{"-1", "0", "1", "2", "60", "100", "1000", "1000000", "0.5", "1.5"} {
				check("the option "+o.Name+" of "+r.ID, fmt.Sprintf("rules:\n  %s:\n    %s: %s\n", r.ID, o.Name, n))
			}
		}
	}
	// the deprecated numeric keys
	for _, n := range []string{"-1", "0", "1", "100", "0.5"} {
		check("max-run-lines", "max-run-lines: "+n+"\n")
		check("timeout-minutes.max", "timeout-minutes:\n  max: "+n+"\n")
	}
}

type failingWriter struct{ after int }

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.after <= 0 {
		return 0, errors.New("write failed")
	}
	w.after--
	return len(p), nil
}

// TestPrintSchemaReportsAFailedWrite: a schema which was not written completely is a failure, not a success.
func TestPrintSchemaReportsAFailedWrite(t *testing.T) {
	for _, flag := range []string{"--print-config-schema", "--print-output-schema"} {
		var errOut bytes.Buffer
		cmd := Command{Stdin: strings.NewReader(""), Stdout: &failingWriter{}, Stderr: &errOut}
		if status := cmd.Main([]string{"jactionlint", flag}); status != ExitStatusFailure || !strings.Contains(errOut.String(), "write failed") {
			t.Errorf("%s: status %d, stderr %q", flag, status, errOut.String())
		}
	}
}

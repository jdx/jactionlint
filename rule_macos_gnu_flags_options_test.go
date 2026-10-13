package jactionlint

import (
	"testing"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

func TestGNUOptionsAreReadLeftToRight(t *testing.T) {
	for w, want := range map[string]bool{"-c": true, "-c%s": true, "-Lc": true, "-Lc%s": true, "-cf": true, "-fc": false, "-tc": false, "-f": false, "-L": false, "--format": false, "-xyc": false, "c": false, "-": false} {
		if have := hasStatFormatOption(w); have != want {
			t.Errorf("hasStatFormatOption(%q) = %v, want %v", w, have, want)
		}
	}
	for w, want := range map[string]bool{"-d": true, "-d@1": true, "-ud": true, "-jd": true, "-rd": false, "-fd": false, "-vd": false, "-u": false, "--date": false, "-I": false, "-": false} {
		if have := hasDateOption(w); have != want {
			t.Errorf("hasDateOption(%q) = %v, want %v", w, have, want)
		}
	}
}

func TestGrepClusterThatTakesTheNextWord(t *testing.T) {
	take := func(w string) bool {
		return clusterTakesNextWord(w, grepNoValueLetters, grepValueLetters, grepValueFlags)
	}
	for w, want := range map[string]bool{
		"-e": true, "-ne": true, "-rnf": true, "-m": true, "-nA": true, "--regexp": true, "--file": true,
		"-A3": false, "-nA3": false, "-ePATTERN": false, "-n": false, "-P": false, "-nP": false, "--regexp=x": false,
		"--": false, "x": false, "-": false,
	} {
		if have := take(w); have != want {
			t.Errorf("%q: have %v, want %v", w, have, want)
		}
	}
	for w, want := range map[string]bool{"-P": true, "-nP": true, "-oP": true, "-rnP": true, "-nA3P": false, "-eP": false, "-A3": false, "-n": false, "-nePattern": false, "--perl-regexp": false} {
		if have := optionClusterHasAny(w, grepNoValueLetters, grepValueLetters, "P"); have != want {
			t.Errorf("P in %q: have %v, want %v", w, have, want)
		}
	}
}

// TestRealCommandTrustsRunscriptWhereItIsFurther checks the two ways round: a wrapper that runscript resolves and that
// commandWordOf does not know, and a command that commandWordOf finds further on than runscript does.
func TestRealCommandTrustsRunscriptWhereItIsFurther(t *testing.T) {
	for script, want := range map[string]string{
		"xvfb-run sed -i s/a/b/ f":       "sed", // runscript knows xvfb-run, commandWordOf does not
		"xvfb-run -a sed -i s/a/b/ f":    "sed",
		"sudo -Eu root cp --parents a b": "cp", // commandWordOf reads the cluster, runscript took root
		"xargs -n 1 grep -P x":           "grep",
		"env -S 'sed -i x f'":            "",
	} {
		s, err := runscript.Analyze(script, "bash")
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		for _, c := range realCommands(s) {
			if c.Name == "sed" || c.Name == "cp" || c.Name == "grep" {
				got = c.Name
			}
		}
		if got != want {
			t.Errorf("%q: the real command is %q, want %q", script, got, want)
		}
	}
}

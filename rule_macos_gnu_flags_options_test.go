package jactionlint

import "testing"

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

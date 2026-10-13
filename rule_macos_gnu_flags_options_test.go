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

package jactionlint

import "testing"

func TestParseBoolIsCaseInsensitive(t *testing.T) {
	// YAML 1.2 reads True and TRUE as booleans too
	for _, v := range []string{"true", "True", "TRUE"} {
		w, errs := Parse([]byte("on: push\nconcurrency:\n  group: ci\n  cancel-in-progress: " + v + "\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"))
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		if c := w.Concurrency.CancelInProgress; c == nil || !c.Value {
			t.Errorf("%s was not read as true: %+v", v, c)
		}
	}
}

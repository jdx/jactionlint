package runscript

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update the golden files")

// TestGolden analyzes every script of testdata/scripts and compares the result with the .golden file next to
// it. testdata/scripts/real-* are `run:` scripts of real workflows, adv-* are hand written adversarial ones.
func TestGolden(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "scripts", "*.sh"))
	if err != nil || len(files) < 80 {
		t.Fatalf("scripts not found (%d): %v", len(files), err)
	}
	for _, f := range files {
		t.Run(strings.TrimSuffix(filepath.Base(f), ".sh"), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var got string
			s, err := Analyze(string(src), "")
			var pe *ParseError
			switch {
			case errors.As(err, &pe):
				got = "parse error\n"
			case err != nil:
				t.Fatal(err)
			default:
				got = dump(s)
			}
			golden := strings.TrimSuffix(f, ".sh") + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if string(want) != got {
				t.Errorf("%s differs from the golden file (run with -update to accept):\n%s", f, got)
			}
		})
	}
}

package jactionlint

import (
	"fmt"
	"testing"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

func TestDbg(t *testing.T) {
	for _, src := range []string{"awk -F: -v x=1 -f tools/sum.awk data.txt", "sed -n -f scripts/fix.sed input.txt", "unzip -d out -: build.zip"} {
		s, err := runscript.Analyze(src, "")
		fmt.Println(err)
		for _, c := range s.Commands {
			fmt.Printf("%q %d args:", c.Name, len(c.Args))
			for _, a := range c.Args {
				fmt.Printf(" [%s dyn=%v]", a.Value, a.Dynamic())
			}
			fmt.Println(commandRunsCode(c))
		}
	}
}

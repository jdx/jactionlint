package main

import (
	"os"

	"github.com/jdx/jactionlint"

	_ "time/tzdata"
)

func main() {
	cmd := jactionlint.Command{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	os.Exit(cmd.Main(os.Args))
}

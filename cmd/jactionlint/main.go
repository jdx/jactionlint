package main

import (
	"os"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner"

	_ "time/tzdata"
)

func main() {
	// `run` lives here and not in the jactionlint package so that the playground (GOOS=js) never links the runner.
	if runner.IsRunCommand(os.Args) {
		os.Exit(runner.Main(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	cmd := jactionlint.Command{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	os.Exit(cmd.Main(os.Args))
}

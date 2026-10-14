// Command generate-config-schema writes the JSON Schemas of jactionlint, schema/jactionlint-config.json (the config
// file, from the rule registry) and schema/jactionlint-output.json (--format json) and schema/jactionlint-facts.json (--export-facts), from the jactionlint package.
// With -check it only verifies that the files are up to date.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdx/jactionlint/v2"
)

type schemaFile struct {
	path string
	data []byte
}

func run(args []string) error {
	check := false
	dir := "schema"
	for _, a := range args {
		switch {
		case a == "-check":
			check = true
		case len(a) > 0 && a[0] != '-':
			dir = a
		default:
			return fmt.Errorf("usage: generate-config-schema [-check] [dstdir]")
		}
	}
	files := []schemaFile{
		{filepath.Join(dir, "jactionlint-config.json"), jactionlint.ConfigSchema()},
		{filepath.Join(dir, "jactionlint-output.json"), jactionlint.OutputSchema()},
		{filepath.Join(dir, "jactionlint-facts.json"), jactionlint.FactsSchema()},
	}
	stale := false
	for _, f := range files {
		if check {
			have, err := os.ReadFile(f.path)
			if err != nil {
				return fmt.Errorf("could not read %s: %w", f.path, err)
			}
			// A Windows checkout can have CRLF line endings
			if !bytes.Equal(bytes.ReplaceAll(have, []byte("\r\n"), []byte("\n")), f.data) {
				fmt.Fprintf(os.Stderr, "%s is outdated. run `mise run schema` (go run ./scripts/generate-config-schema) to regenerate it\n", f.path)
				stale = true
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
			return err
		}
	}
	if stale {
		os.Exit(1)
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

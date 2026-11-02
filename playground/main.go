//go:build wasm

package main

import (
	"io"
	"syscall/js"
	"time"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/exampleconfig"
)

var (
	window = js.Global().Get("window")
)

func fail(err error, when string) {
	window.Call("showError", err.Error()+" on "+when)
}

func encodeErrorAsMap(err *jactionlint.Error) map[string]interface{} {
	obj := make(map[string]interface{}, 5)
	obj["message"] = err.Message
	obj["line"] = err.Line
	obj["column"] = err.Column
	obj["id"] = err.ID
	obj["severity"] = err.Severity.String()
	return obj
}

// lint lints the source as of the day ("2006-01-02"), or as of now when day is empty. The tests of the permalinks of
// docs/checks.md pass the day the examples were linted on, so that a date-dependent check (the retirement of a runner
// label) gives the documented output.
func lint(source, day string) interface{} {
	// The same configuration as the one the examples of docs/checks.md are linted with
	cfg, err := jactionlint.ParseConfig([]byte(exampleconfig.YAML))
	if err != nil {
		fail(err, "reading the configuration")
		return nil
	}
	opts := jactionlint.LinterOptions{Config: cfg}
	if day != "" {
		t, err := time.Parse(time.DateOnly, day)
		if err != nil {
			fail(err, "reading the day")
			return nil
		}
		opts.Now = func() time.Time { return t }
	}
	linter, err := jactionlint.NewLinter(io.Discard, &opts)
	if err != nil {
		fail(err, "creating linter instance")
		return nil
	}

	errs, err := linter.Lint("test.yaml", []byte(source), nil)
	if err != nil {
		fail(err, "applying lint rules")
		return nil
	}

	ret := make([]interface{}, 0, len(errs))
	for _, err := range errs {
		ret = append(ret, encodeErrorAsMap(err))
	}

	window.Call("onCheckCompleted", js.ValueOf(ret))

	return nil
}

func runActionlint(_this js.Value, args []js.Value) interface{} {
	source := args[0].String()
	day := ""
	if len(args) > 1 && args[1].Type() == js.TypeString {
		day = args[1].String()
	}
	return lint(source, day)
}

func main() {
	window.Set("runActionlint", js.FuncOf(runActionlint))
	window.Call("dismissLoading")
	lint(window.Call("getYamlSource").String(), "") // Show the first result
	select {}
}

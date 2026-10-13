package main

import (
	"errors"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestWriteStdoutOK(t *testing.T) {
	in := filepath.Join("testdata", "ok.html")
	stdout := &strings.Builder{}
	if err := run([]string{in, "-"}, stdout, io.Discard, ""); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join("testdata", "ok.go"))
	if err != nil {
		t.Fatal(err)
	}

	want := strings.ReplaceAll(string(b), "\r\n", "\n")
	have := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
	if diff := cmp.Diff(want, have); diff != "" {
		t.Fatal(diff)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "", have, parser.AllErrors); err != nil {
		t.Fatalf("Input is not valid as Go. Error: %s\nSource: %s", err, have)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("dummy write error")
}

func TestWriteError(t *testing.T) {
	in := filepath.Join("testdata", "ok.html")
	err := run([]string{in, "-"}, errWriter{}, io.Discard, "")
	if err == nil {
		t.Fatal("error did not occur")
	}
	if !strings.Contains(err.Error(), "could not write output") {
		t.Fatalf("unexpected error: %q", err)
	}
	if !strings.Contains(err.Error(), "dummy write error") {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestInvalidCommandArgs(t *testing.T) {
	err := run([]string{"a", "b", "c"}, io.Discard, io.Discard, "")
	if err == nil {
		t.Fatal("error did not occur")
	}
	if !strings.Contains(err.Error(), "usage: generate-webhook-events [[srcfile] dstfile]") {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestParseError(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"no_article_body.html", "article body was not found"},
		{"no_markdown_body.html", "markdown body was not found"},
		{"no_heading.html", `"About events that trigger workflows" heading was missing`},
		{"no_tables.html", "no webhook table was found"},
		{"mismatched_aria_labelledby.html", `could not parse webhook types table for "check_run": expected table[aria-labelledby] to be "check_run", got "check_suite"`},
		{"no_thead.html", `could not parse webhook types table for "check_run": missing thead element`},
		{"no_header_row.html", `could not parse webhook types table for "check_run": missing header row in thead`},
		{"too_few_headers.html", `could not parse webhook types table for "check_run": expected at least 2 header columns, got 1`},
		{"non_webhook_table.html", `could not parse webhook types table for "check_run": expected first header to be "Webhook event payload", got "Operator"`},
		{"bad_second_header.html", `could not parse webhook types table for "check_run": expected second header to be "Activity types", got "Description"`},
		{"no_tbody.html", `could not parse webhook types table for "check_run": missing tbody element`},
		{"no_table_row.html", `could not parse webhook types table for "check_run": missing first data row in tbody`},
		{"too_few_columns.html", `could not parse webhook types table for "check_run": expected at least 2 data columns, got 1`},
		{"bad_activity_types.html", `could not parse webhook types table for "check_run": unexpected activity types cell text "Surprise"; expected code elements, "Not applicable", or "Custom"`},
	}

	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			stdout := &strings.Builder{}
			err := run([]string{filepath.Join("testdata", tc.file), "-"}, stdout, io.Discard, "")
			if err == nil {
				t.Fatal("error did not occur")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q in error %q", tc.want, err)
			}
		})
	}
}

// The production table makes a types table generated from documentation which does not list the types accept them.
func TestProductionExtraTypes(t *testing.T) {
	parsed := map[string][]string{"merge_group": {"checks_requested"}, "pull_request": {"opened"}, "pull_request_target": {"opened"}, "image_version": {}}
	if err := applyExtraTypes(parsed); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"pull_request", "pull_request_target"} {
		if !slices.Contains(parsed[ev], "stacked") {
			t.Errorf("%s does not have the stacked type: %v", ev, parsed[ev])
		}
	}
	if diff := cmp.Diff([]string{"created", "ready", "deleted"}, parsed["image_version"]); diff != "" {
		t.Error(diff)
	}
}

func TestFetchURL(t *testing.T) {
	b, err := fetch("https://github.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("Fetched source is empty")
	}
}

func TestExtraTypes(t *testing.T) {
	// The mechanism is tested with a table of its own, not the production one
	prev := extraTypes
	extraTypes = map[string][]string{"merge_group": {"destroyed"}}
	t.Cleanup(func() { extraTypes = prev })

	t.Run("added once", func(t *testing.T) {
		p := map[string][]string{"merge_group": {"checks_requested"}}
		for i := 0; i < 2; i++ {
			if err := applyExtraTypes(p); err != nil {
				t.Fatal(err)
			}
		}
		want := []string{"checks_requested", "destroyed"}
		if diff := cmp.Diff(want, p["merge_group"]); diff != "" {
			t.Fatal(diff)
		}
	})
	t.Run("already documented", func(t *testing.T) {
		p := map[string][]string{"merge_group": {"destroyed", "checks_requested"}}
		if err := applyExtraTypes(p); err != nil {
			t.Fatal(err)
		}
		want := []string{"destroyed", "checks_requested"}
		if diff := cmp.Diff(want, p["merge_group"]); diff != "" {
			t.Fatal(diff)
		}
	})
	t.Run("event missing in the document", func(t *testing.T) {
		err := applyExtraTypes(map[string][]string{"fork": {}})
		if err == nil || !strings.Contains(err.Error(), `event "merge_group"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestExistingOutputSurvivesError(t *testing.T) {
	ok, err := os.ReadFile(filepath.Join("testdata", "ok.html"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// merge_group is missing in the document, which is an error of the extra activity types
	noMergeGroup := filepath.Join(dir, "no_merge_group.html")
	if err := os.WriteFile(noMergeGroup, []byte(strings.ReplaceAll(string(ok), "merge_group", "merge_groups")), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, in := range []string{noMergeGroup, filepath.Join("testdata", "no_tables.html")} {
		dst := filepath.Join(dir, "all_webhooks.go")
		const existing = "package jactionlint // existing output\n"
		if err := os.WriteFile(dst, []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := run([]string{in, dst}, io.Discard, io.Discard, ""); err == nil {
			t.Fatalf("%s: error did not occur", in)
		}
		b, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != existing {
			t.Fatalf("%s: existing output was modified: %q", in, b)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.Contains(e.Name(), ".tmp") {
				t.Fatalf("%s: temporary file was left: %s", in, e.Name())
			}
		}
	}
}

func TestWriteFileOK(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "all_webhooks.go")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{filepath.Join("testdata", "ok.html"), dst}, io.Discard, io.Discard, ""); err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "ok.go"))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(strings.ReplaceAll(string(want), "\r\n", "\n"), strings.ReplaceAll(string(have), "\r\n", "\n")); diff != "" {
		t.Fatal(diff)
	}
}

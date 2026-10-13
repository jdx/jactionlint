package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update testdata/event_payloads.go")

// Two events of the real table are enough: pull_request_target has the payload of pull_request, and the universe
// has the properties of the events which are not in the table, too.
func withEvents(t *testing.T) {
	t.Helper()
	old, oldExtra := events, extraProps
	events = map[string]string{"pull_request": "pull_request", "pull_request_target": "pull_request", "push": "push"}
	extraProps = map[string][]string{"push:head_commit": {"extra_field"}}
	t.Cleanup(func() { events, extraProps = old, oldExtra })
}

func testInputs() *inputs {
	return &inputs{
		octokit: octokitFiles{
			"pull_request/opened.schema.json": []byte(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["opened"]},
    "number": {"type": "integer"},
    "Pull_Request": {
      "allOf": [
        {"$ref": "common/pull-request.schema.json"},
        {"type": "object", "properties": {"state": {"type": "string", "enum": ["open"]}, "merged_by": {"type": "null"}}}
      ]
    },
    "repository": {"$ref": "common/repository.schema.json"},
    "sender": {"$ref": "common/user.schema.json"},
    "changes": {"type": "object", "properties": {"title": {"type": "object", "properties": {"from": {"type": "string"}}}}}
  },
  "additionalProperties": false
}`),
			"pull_request/closed.schema.json": []byte(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["closed"]},
    "pull_request": {"$ref": "common/pull-request.schema.json"},
    "label": {"oneOf": [{"type": "null"}, {"$ref": "common/label.schema.json"}]}
  }
}`),
			"push/event.schema.json": []byte(`{
  "type": "object",
  "properties": {
    "ref": {"type": "string"},
    "commits": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "string"}, "author": {"type": "object", "properties": {"name": {"type": "string"}, "deep": {"type": "object", "properties": {"x": {"type": "string"}}}}}}}},
    "head_commit": {"type": ["object", "null"], "properties": {"id": {"type": "string"}}},
    "repository": {"$ref": "common/repository.schema.json"},
    "pusher": {"type": "object", "properties": {"name": {"type": "string"}}},
    "labels": {"type": "array", "items": {"type": "string"}}
  }
}`),
			"ping/event.schema.json": []byte(`{"type": "object", "properties": {"zen": {"type": "string"}, "hook_id": {"type": "integer"}}}`),
			"common/pull-request.schema.json": []byte(`{
  "type": "object",
  "properties": {
    "id": {"type": "integer"},
    "title": {"type": "string"},
    "state": {"type": "string"},
    "user": {"$ref": "user.schema.json"},
    "head": {"type": "object", "properties": {"ref": {"type": "string"}, "repo": {"$ref": "repository.schema.json"}}},
    "labels": {"type": "array", "items": {"$ref": "label.schema.json"}},
    "payload": {"type": "object", "properties": {"a": {"type": "string"}}}
  },
  "additionalProperties": false
}`),
			"common/repository.schema.json": []byte(`{
  "type": "object",
  "properties": {
    "id": {"type": "integer"},
    "name": {"type": "string"},
    "full_name": {"type": "string"},
    "description": {"type": ["string", "null"]},
    "owner": {"$ref": "user.schema.json"},
    "custom": {"type": "object", "additionalProperties": {"type": "string"}}
  }
}`),
			"common/user.schema.json":  []byte(`{"type": "object", "properties": {"login": {"type": "string"}, "id": {"type": "integer"}}}`),
			"common/label.schema.json": []byte(`{"type": "object", "properties": {"name": {"type": "string"}, "color": {"type": "string"}, "default": {"type": "boolean"}}}`),
		},
		api: openAPI{
			"info": map[string]any{"version": "9.9.9"},
			"x-webhooks": map[string]any{
				"pull-request-opened": map[string]any{"post": map[string]any{
					"x-github": map[string]any{"subcategory": "pull_request"},
					"requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"$ref": "#/components/schemas/webhook-pull-request-opened"}}}},
				}},
				"push": map[string]any{"post": map[string]any{
					"x-github": map[string]any{"subcategory": "push"},
					"requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"type": "object", "properties": map[string]any{
							"enterprise": map[string]any{"type": "object", "properties": map[string]any{"slug": map[string]any{"type": "string"}}},
							"ref":        map[string]any{"type": "string"},
						}}}}},
				}},
				"installation-created": map[string]any{"post": map[string]any{
					"x-github": map[string]any{"subcategory": "installation"},
					"requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"type": "object", "properties": map[string]any{"installation": map[string]any{"type": "object"}}}}}},
				}},
			},
			"components": map[string]any{"schemas": map[string]any{
				"webhook-pull-request-opened": map[string]any{"type": "object", "properties": map[string]any{
					"action":       map[string]any{"type": "string"},
					"pull_request": map[string]any{"$ref": "#/components/schemas/pull-request-webhook"},
				}},
				"pull-request-webhook": map[string]any{"type": "object", "properties": map[string]any{
					"title":      map[string]any{"type": "string"},
					"auto_merge": map[string]any{"type": "object", "properties": map[string]any{"enabled_by": map[string]any{"type": "string"}}},
				}},
			}},
		},
		octokitCommit: "abc123",
		apiVersion:    "9.9.9",
	}
}

func TestGenerateGolden(t *testing.T) {
	withEvents(t)
	got, err := generate(testInputs())
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "event_payloads.go")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("the output differs from %s (run with -update to change it):\n%s", golden, got)
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	withEvents(t)
	first, err := generate(testInputs())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		again, err := generate(testInputs())
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(first) {
			t.Fatalf("the output changed in run %d:\n%s\n%s", i, first, again)
		}
	}
}

func TestGenerateShapes(t *testing.T) {
	withEvents(t)
	out, err := generate(testInputs())
	if err != nil {
		t.Fatal(err)
	}
	src := string(out)
	for _, want := range []string{
		// The properties of the actions of both sources are merged, and names are lower case
		"pull_request:", "action", "number", "label:", "auto_merge",
		// pull_request_target has the payload of pull_request
		`"pull_request_target": "=pull_request"`,
		// The extra properties are added where they are said to be
		"extra_field",
		// The universe has the events which are not in the table
		"zen", "hook_id", "installation", "client_payload", "schedule",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("%q is not in the output:\n%s", want, src)
		}
	}
	for _, unwanted := range []string{
		"Pull_Request",
		// The contents of accounts are not looked into
		"login",
		// Arbitrary objects and what is deeper than maxLevels are not looked into
		"x:", "a:",
	} {
		if strings.Contains(src, unwanted) {
			t.Errorf("%q is in the output:\n%s", unwanted, src)
		}
	}
	// The header says where the shapes come from
	for _, want := range []string{"Code generated", "abc123", "9.9.9", "MIT licence"} {
		if !strings.Contains(src, want) {
			t.Errorf("the header lacks %q", want)
		}
	}
}

func TestGenerateErrors(t *testing.T) {
	tests := map[string]func(in *inputs){
		"event without schemas": func(in *inputs) {
			delete(in.octokit, "push/event.schema.json")
			delete(in.api["x-webhooks"].(map[string]any), "push")
		},
		"missing reference": func(in *inputs) {
			in.octokit["common/user.schema.json"] = []byte(`{"type": "object", "properties": {"x": {"$ref": "nowhere.schema.json"}}}`)
			in.octokit["common/repository.schema.json"] = []byte(`{"type": "object", "properties": {"owner": {"$ref": "user.schema.json"}}}`)
		},
		"broken json": func(in *inputs) {
			in.octokit["common/label.schema.json"] = []byte(`{`)
		},
		"name which cannot be written": func(in *inputs) {
			in.octokit["push/event.schema.json"] = []byte(`{"type": "object", "properties": {"a,b": {"type": "string"}}}`)
		},
		"webhook without event": func(in *inputs) {
			in.api["x-webhooks"].(map[string]any)["push"] = map[string]any{"post": map[string]any{}}
		},
	}
	for name, mod := range tests {
		t.Run(name, func(t *testing.T) {
			withEvents(t)
			in := testInputs()
			mod(in)
			if out, err := generate(in); err == nil {
				t.Errorf("want an error but got the output:\n%s", out)
			}
		})
	}
}

func TestRecursiveReferenceIsAnError(t *testing.T) {
	withEvents(t)
	in := testInputs()
	in.octokit["common/label.schema.json"] = []byte(`{"$ref": "label.schema.json"}`)
	if _, err := generate(in); err == nil {
		t.Error("want an error")
	}
}

func TestMerge(t *testing.T) {
	obj := func(names ...string) *node {
		n := &node{kind: kObject, props: map[string]*node{}}
		for _, k := range names {
			n.props[k] = anyNode
		}
		return n
	}
	tests := []struct {
		a, b *node
		want kind
		n    int
	}{
		{obj("a"), obj("b"), kObject, 2},
		{obj("a"), &node{kind: kNull}, kObject, 1},
		{&node{kind: kNull}, obj("a"), kObject, 1},
		{obj("a"), anyNode, kAny, 0},
		{&node{kind: kArray, elem: obj("a")}, &node{kind: kArray, elem: obj("b")}, kArray, 0},
		{&node{kind: kArray, elem: obj("a")}, obj("b"), kAny, 0},
		{nil, obj("a"), kObject, 1},
	}
	for i, tc := range tests {
		got := merge(tc.a, tc.b)
		if got.kind != tc.want || len(got.props) != tc.n {
			t.Errorf("case %d: got kind %d with %d properties", i, got.kind, len(got.props))
		}
		if tc.want == kArray && len(got.elem.props) != 2 {
			t.Errorf("case %d: the elements are merged: %v", i, got.elem.props)
		}
	}
	// merge does not change its arguments
	a, b := obj("a"), obj("b")
	merge(a, b)
	if len(a.props) != 1 || len(b.props) != 1 {
		t.Error("merge changed an argument")
	}
}

func TestWrap(t *testing.T) {
	long := strings.Repeat("abcdefghij,", 30)
	got := wrap(long, "\t")
	if strings.ReplaceAll(strings.ReplaceAll(got, "\n", ""), "\t", "") != long {
		t.Error("wrap changed the text")
	}
	for _, l := range strings.Split(got, "\n") {
		if len(l) > lineWidth+12 {
			t.Errorf("line is too long: %d", len(l))
		}
	}
}

func TestReadTarball(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": "feedbeef0123"}}); err != nil {
		t.Fatal(err)
	}
	add := func(name, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("webhooks-0123abc/payload-schemas/api.github.com/push/event.schema.json", `{}`)
	add("webhooks-0123abc/payload-schemas/api.github.com/common/user.schema.json", `{}`)
	add("webhooks-0123abc/payload-schemas/index.mts", `x`)
	add("webhooks-0123abc/README.md", `x`)
	tw.Close()
	zw.Close()

	files, commit, err := readTarball(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if commit != "feedbeef0123" {
		t.Errorf("commit is %q", commit)
	}
	if len(files) != 2 || files["push/event.schema.json"] == nil || files["common/user.schema.json"] == nil {
		t.Errorf("files are %v", files)
	}
	if _, _, err := readTarball([]byte("not a tarball")); err == nil {
		t.Error("want an error")
	}

	// Without the comment, the commit is in the name of the directory
	buf.Reset()
	zw = gzip.NewWriter(&buf)
	tw = tar.NewWriter(zw)
	add("webhooks-0123abc/payload-schemas/api.github.com/push/event.schema.json", `{}`)
	tw.Close()
	zw.Close()
	if _, commit, err := readTarball(buf.Bytes()); err != nil || commit != "0123abc" {
		t.Errorf("commit is %q: %v", commit, err)
	}
}

func TestRunWritesOnlyWhenTheInputIsGood(t *testing.T) {
	withEvents(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.go")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A failed run leaves the output as it is
	octokitDir := filepath.Join(dir, "schemas")
	if err := os.MkdirAll(octokitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(octokitDir, "x.schema.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	api := filepath.Join(dir, "api.json")
	if err := os.WriteFile(api, []byte(`{"info": {"version": "1"}, "x-webhooks": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, dbg bytes.Buffer
	if err := run([]string{octokitDir, api, dst}, &out, &dbg, "", ""); err == nil {
		t.Fatal("want an error: the events have no schema")
	}
	if b, _ := os.ReadFile(dst); string(b) != "old" {
		t.Errorf("a failed run changed the output: %q", b)
	}
	if err := run([]string{"a", "b"}, &out, &dbg, "", ""); err == nil {
		t.Error("want a usage error")
	}
}

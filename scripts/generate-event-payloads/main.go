package main

// This is a script to generate event_payloads.go, which holds the shape of the payload of each webhook event
// (the first levels of github.event). Run the following command from the root of this repository to apply
// manually. This script is usually run via `go generate`.
// ```
// go run ./scripts/generate-event-payloads ./event_payloads.go
// ```

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The sources are the JSON schemas of the webhook payloads of octokit/webhooks and the webhooks of the OpenAPI
// description of the REST API of GitHub. Both have the MIT licence. A property is known when any of them has it, so
// that a property which one source lacks does not make a false report.
const (
	octokitURL  = "https://codeload.github.com/octokit/webhooks/tar.gz/refs/heads/main"
	openAPIURL  = "https://raw.githubusercontent.com/github/rest-api-description/main/descriptions/api.github.com/api.github.com.json"
	schemaDir   = "payload-schemas/api.github.com"
	universeKey = "*"
)

// events maps the events of the "on:" section of a workflow to the names of the events of the sources. An event which
// is not here is untyped. workflow_call, workflow_dispatch, repository_dispatch and schedule are not here on
// purpose: their payload is the one of the caller, or it is arbitrary, or it has no schema.
var events = map[string]string{
	"branch_protection_rule":      "branch_protection_rule",
	"check_run":                   "check_run",
	"check_suite":                 "check_suite",
	"create":                      "create",
	"delete":                      "delete",
	"deployment":                  "deployment",
	"deployment_status":           "deployment_status",
	"discussion":                  "discussion",
	"discussion_comment":          "discussion_comment",
	"fork":                        "fork",
	"gollum":                      "gollum",
	"issue_comment":               "issue_comment",
	"issues":                      "issues",
	"label":                       "label",
	"merge_group":                 "merge_group",
	"milestone":                   "milestone",
	"page_build":                  "page_build",
	"public":                      "public",
	"pull_request":                "pull_request",
	"pull_request_review":         "pull_request_review",
	"pull_request_review_comment": "pull_request_review_comment",
	"pull_request_target":         "pull_request",
	"push":                        "push",
	"registry_package":            "registry_package",
	"release":                     "release",
	"status":                      "status",
	"watch":                       "watch",
	"workflow_run":                "workflow_run",
}

// universeExtra are properties of the payload of events which have no webhook: the properties which
// the universe, the payload of any event, has besides the ones of the webhooks.
var universeExtra = []string{"inputs", "client_payload", "schedule"}

// extraProps are properties of payloads which GitHub sends but the sources lack. The key is the event ("*" for all
// of them) and the path of the object from the payload, separated by dots, and an empty path is the payload itself.
var extraProps = map[string][]string{
	// The issue of the REST API has these, and the issue of a webhook may have them, too (the documentation of
	// GitHub does not say).
	"*:issue": {"closed_by", "parent_issue_url"},
	// The repository of the REST API has it when the token may see it
	"*:repository": {"security_and_analysis"},
}

var dbg = log.New(io.Discard, "", log.LstdFlags)

func parseJSON(b []byte) (map[string]any, error) {
	var s map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return s, nil
}

// inputs are the sources of the shapes.
type inputs struct {
	octokit octokitFiles
	api     openAPI
	// where the sources come from, for the header of the output
	octokitCommit string
	apiVersion    string // the version and the start of the SHA-256 of the description, which has no commit
}

// payloads returns the shape of the payload of every event of events and of the universe, which is the payload of any
// event: the union of the ones of all the webhooks.
func (in *inputs) payloads() (map[string]*node, error) {
	oc := newConverter(in.octokit)
	ac := newConverter(in.api)
	hooks, err := in.api.webhooks()
	if err != nil {
		return nil, err
	}
	octokitEvents := map[string]bool{}
	for _, d := range in.octokit.events() {
		octokitEvents[d] = true
	}

	// source is the union of the payloads of an event in both of the sources
	source := func(event string) (*node, error) {
		var ret *node
		if octokitEvents[event] {
			n, err := in.octokit.payload(oc, event)
			if err != nil {
				return nil, err
			}
			ret = merge(ret, n)
		}
		for i, s := range hooks[event] {
			n, err := ac.convert(s, fmt.Sprintf("#/x-webhooks/%s/%d", event, i), 1)
			if err != nil {
				return nil, fmt.Errorf("webhook %s: %w", event, err)
			}
			ret = merge(ret, n)
		}
		if ret == nil {
			return nil, fmt.Errorf("no source has the event %s", event)
		}
		return ret, nil
	}

	ret := map[string]*node{}
	for name, src := range events {
		n, err := source(src)
		if err != nil {
			return nil, err
		}
		ret[name] = n
	}

	// The universe has the properties of the payload of every event of the sources, also the ones which have no entry
	// in events
	all := map[string]bool{}
	for e := range octokitEvents {
		all[e] = true
	}
	for e := range hooks {
		all[e] = true
	}
	names := make([]string, 0, len(all))
	for e := range all {
		names = append(names, e)
	}
	sort.Strings(names)
	var universe *node
	for _, e := range names {
		n, err := source(e)
		if err != nil {
			return nil, err
		}
		universe = merge(universe, n)
	}
	if universe == nil || universe.kind != kObject {
		return nil, errors.New("the universe of payloads is empty")
	}
	for _, p := range universeExtra {
		if _, ok := universe.props[p]; !ok {
			universe.props[p] = anyNode
		}
	}
	ret[universeKey] = universe
	return ret, nil
}

// generate returns the Go source for the sources.
func generate(in *inputs) ([]byte, error) {
	payloads, err := in.payloads()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(payloads))
	for e := range payloads {
		names = append(names, e)
	}
	sort.Strings(names)

	interned := newInterner()
	roots := map[string]*shape{}
	for _, e := range names {
		n := prune(payloads[e])
		addExtra(n, e, "")
		roots[e] = interned.intern(n, e)
	}

	// Events with the same payload (pull_request and pull_request_target) are written once.
	alias := map[string]string{}
	first := map[*shape]string{}
	for _, e := range names {
		if f, ok := first[roots[e]]; ok {
			alias[e] = f
			continue
		}
		first[roots[e]] = e
		countRefs(roots[e], map[*shape]bool{})
	}
	shared := nameShared(interned)

	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by scripts/generate-event-payloads; DO NOT EDIT.\n\n")
	fmt.Fprintf(&out, "package jactionlint\n\n")
	fmt.Fprintf(&out, "// The shapes of the payloads of the webhook events, which are the shapes of github.event. They are derived from\n")
	fmt.Fprintf(&out, "// the JSON schemas of the webhooks of octokit/webhooks (MIT licence) at the commit %s and of\n", in.octokitCommit)
	fmt.Fprintf(&out, "// the OpenAPI description of github/rest-api-description (MIT licence), version %s,\n", in.apiVersion)
	fmt.Fprintf(&out, "// reduced to the first %d levels of objects: an object of a deeper level is not written, and nothing is known of it.\n", maxLevels)
	fmt.Fprintf(&out, "// A property of an object is known when the payload of any action of the event has it, in any of the sources.\n")
	fmt.Fprintf(&out, "//\n")
	fmt.Fprintf(&out, "// The text of a shape is an object: {name, name:shape, ...}. A name without a shape is a value which is not\n")
	fmt.Fprintf(&out, "// looked into. A shape is an object {...}, an array [shape] (or [] of anything), or @name for an object of\n")
	fmt.Fprintf(&out, "// eventPayloadObjects. White space is ignored. The text =name is the payload of the event of that name.\n\n")

	fmt.Fprintf(&out, "// eventPayloadObjects are the objects which are used more than once.\n")
	fmt.Fprintf(&out, "var eventPayloadObjects = map[string]string{\n")
	for _, s := range shared {
		var b strings.Builder
		if err := emit(&b, s, true); err != nil {
			return nil, err
		}
		fmt.Fprintf(&out, "\t%q: `%s`,\n", s.name, wrap(b.String(), "\t\t"))
	}
	fmt.Fprintf(&out, "}\n\n")

	fmt.Fprintf(&out, "// eventPayloads are the payloads of the events by the name of the event in the \"on:\" section. The payload\n")
	fmt.Fprintf(&out, "// of %q is the union of the payloads of all the events, which also have the properties of the events\n", universeKey)
	fmt.Fprintf(&out, "// which cannot be a trigger of a workflow.\n")
	fmt.Fprintf(&out, "var eventPayloads = map[string]string{\n")
	for _, e := range names {
		if f, ok := alias[e]; ok {
			fmt.Fprintf(&out, "\t%q: %q,\n", e, "="+f)
			continue
		}
		var b strings.Builder
		if err := emit(&b, roots[e], true); err != nil {
			return nil, err
		}
		fmt.Fprintf(&out, "\t%q: `%s`,\n", e, wrap(b.String(), "\t\t"))
	}
	fmt.Fprintf(&out, "}\n")

	src, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("could not format Go source: %w\n%s", err, out.String())
	}
	return src, nil
}

func fetch(url string) ([]byte, error) {
	var c http.Client
	dbg.Println("Fetching", url)
	res, err := c.Get(url)
	if err != nil {
		return nil, fmt.Errorf("could not fetch %s: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || 300 <= res.StatusCode {
		return nil, fmt.Errorf("request was not successful for %s: %s", url, res.Status)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("could not fetch body for %s: %w", url, err)
	}
	dbg.Printf("Fetched %d bytes from %s", len(body), url)
	return body, nil
}

// readTarball reads the schemas of a tarball of octokit/webhooks. It returns them and the commit of the tarball.
func readTarball(tgz []byte) (octokitFiles, string, error) {
	zr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, "", err
	}
	tr := tar.NewReader(zr)
	fs := octokitFiles{}
	commit := ""
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", err
		}
		// The tarball of GitHub has the commit in the comment of its global header
		if h.Typeflag == tar.TypeXGlobalHeader {
			if c := h.PAXRecords["comment"]; c != "" {
				commit = c
			}
			continue
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		top, rest, ok := strings.Cut(h.Name, "/")
		if !ok {
			continue
		}
		if commit == "" {
			commit = strings.TrimPrefix(top, "webhooks-")
		}
		rel, ok := strings.CutPrefix(rest, schemaDir+"/")
		if !ok || !strings.HasSuffix(rel, ".json") {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, "", err
		}
		fs[rel] = b
	}
	if len(fs) == 0 {
		return nil, "", errors.New("the tarball has no payload schemas")
	}
	return fs, commit, nil
}

// readDir reads the schemas of a local directory, which is payload-schemas/api.github.com of octokit/webhooks.
func readDir(dir string) (octokitFiles, error) {
	fs := octokitFiles{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".json") {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		fs[filepath.ToSlash(rel)] = b
		return err
	})
	if err == nil && len(fs) == 0 {
		err = fmt.Errorf("%s has no JSON schemas", dir)
	}
	return fs, err
}

func parseOpenAPI(b []byte) (openAPI, string, error) {
	var d openAPI
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, "", fmt.Errorf("could not parse the OpenAPI description: %w", err)
	}
	info, _ := d["info"].(map[string]any)
	version, _ := info["version"].(string)
	sum := sha256.Sum256(b)
	return d, fmt.Sprintf("%s (SHA-256 %x)", version, sum[:6]), nil
}

func run(args []string, stdout, dbgout io.Writer, octokitSrc, openAPISrc string) error {
	dbg.SetOutput(dbgout)
	if len(args) != 0 && len(args) != 1 && len(args) != 3 {
		return errors.New("usage: generate-event-payloads [[octokit-schema-dir openapi.json] dstfile]")
	}

	in := &inputs{}
	var apiText []byte
	var err error
	if len(args) == 3 {
		if in.octokit, err = readDir(args[0]); err != nil {
			return err
		}
		if apiText, err = os.ReadFile(args[1]); err != nil {
			return err
		}
		in.octokitCommit = "(local)"
	} else {
		tgz, err := fetch(octokitSrc)
		if err != nil {
			return err
		}
		if in.octokit, in.octokitCommit, err = readTarball(tgz); err != nil {
			return err
		}
		if apiText, err = fetch(openAPISrc); err != nil {
			return err
		}
	}
	if in.api, in.apiVersion, err = parseOpenAPI(apiText); err != nil {
		return err
	}

	// Everything which can fail because of the input happens before the destination is touched, so that a
	// failed refresh never empties the existing output.
	src, err := generate(in)
	if err != nil {
		return err
	}

	if len(args) == 0 || args[len(args)-1] == "-" {
		_, err := stdout.Write(src)
		return err
	}
	return writeFile(src, args[len(args)-1])
}

// writeFile writes the output to a temporary file next to dst and renames it over dst, so that dst is either
// the complete new output or untouched.
func writeFile(src []byte, dst string) error {
	f, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after a successful rename
	if _, err := f.Write(src); err != nil {
		f.Close()
		return fmt.Errorf("could not write output: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, octokitURL, openAPIURL); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

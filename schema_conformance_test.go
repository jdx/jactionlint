package jactionlint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestSchemaConformance compares the tables of the parser and of the rules with the schemas which the runner of
// GitHub Actions reads workflows and actions with: src/Sdk/WorkflowParser/workflow-v1.0.json and
// src/Runner.Worker/action_yaml.json of actions/runner. The schemas are vendored in testdata/schema/runner at the commit
// in testdata/schema/runner/COMMIT, so the test runs offline. A scheduled workflow (.github/workflows/schema-pin.yml,
// scripts/update-schema-pin.bash) bumps the pin and opens a pull request, which is how a change of GitHub's syntax is
// noticed: the test fails with the difference until the parser follows it or the difference is explained.
//
// It compares:
//   - the keys which each mapping accepts (what the parser lists in its "unexpected key" error, and which filters of
//     an event the rules accept);
//   - the names of the events and their activity types;
//   - the scopes and the levels of permissions;
//   - the contexts and the special functions that are available at each key (availability.go), and where a whole
//     value may be an expression.
//
// Every difference has to be in schemaConformanceAllowed with the reason, or the test fails. An entry which is not
// needed any more fails the test too, so the list only shrinks when a difference is fixed.
func TestSchemaConformance(t *testing.T) {
	wf := loadSchema(t, "workflow-v1.0.json")
	act := loadSchema(t, "action_yaml.json")
	if len(wf.defs) < 100 || len(act.defs) < 10 {
		t.Fatalf("the schemas are too small: %d and %d definitions", len(wf.defs), len(act.defs))
	}

	var diffs []string
	add := func(area, format string, args ...any) {
		diffs = append(diffs, area+": "+fmt.Sprintf(format, args...))
	}

	checkMappingKeys(t, wf, act, add)
	checkEvents(t, wf, add)
	checkPermissions(t, wf, add)
	checkAvailability(t, wf, act, add)
	checkWholeValueExpressions(t, wf, add)

	sort.Strings(diffs)
	diffs = slices.Compact(diffs)
	seen := map[string]bool{}
	for _, d := range diffs {
		seen[d] = true
		if _, ok := schemaConformanceAllowed[d]; !ok {
			t.Errorf("the parser and the schema of the runner differ:\n\t%s\nFix the parser, or explain the difference in schemaConformanceAllowed (schema_conformance_test.go)", d)
		}
	}
	for d, reason := range schemaConformanceAllowed {
		if reason == "" {
			t.Errorf("the allowed difference needs a reason: %s", d)
		}
		if !seen[d] && !strings.HasPrefix(reason, pendingPrefix) {
			t.Errorf("the allowed difference does not exist any more, remove it from schemaConformanceAllowed:\n\t%s", d)
		}
	}
}

// The schema of the runner is also the one of the checkout in testdata/schema/runner
func TestSchemaPinIsConsistent(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "schema", "runner", "COMMIT"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}\n$`).Match(b) {
		t.Fatalf("testdata/schema/runner/COMMIT must hold one full commit hash: %q", b)
	}
}

// schema is a schema file of the runner: {"definitions": {name: definition}}.
type schema struct {
	defs map[string]map[string]any
}

func loadSchema(t *testing.T, name string) *schema {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "schema", "runner", name))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Definitions map[string]map[string]any `json:"definitions"`
	}
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &f); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return &schema{f.Definitions}
}

func (s *schema) def(name string) map[string]any { return s.defs[name] }

// oneOf returns the names of the alternatives of a definition.
func oneOf(d map[string]any) []string {
	var ret []string
	if l, ok := d["one-of"].([]any); ok {
		for _, e := range l {
			if n, ok := e.(string); ok {
				ret = append(ret, n)
			}
		}
	}
	return ret
}

// typeName returns the name of the definition which a property or an item refers to.
func typeName(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case map[string]any:
		if n, ok := v["type"].(string); ok {
			return n
		}
	}
	return ""
}

// keys returns the keys of the mappings of a definition, following the alternatives of one-of. loose tells that
// one of the mappings accepts any other key. found is false when the definition has no mapping.
func (s *schema) keys(name string) (keys map[string]bool, loose, found bool) {
	keys = map[string]bool{}
	var walk func(string, map[string]bool)
	walk = func(n string, visited map[string]bool) {
		if visited[n] {
			return
		}
		visited[n] = true
		d := s.def(n)
		if d == nil {
			return
		}
		if m, ok := d["mapping"].(map[string]any); ok {
			found = true
			if props, ok := m["properties"].(map[string]any); ok {
				for k := range props {
					keys[k] = true
				}
			}
			if _, ok := m["loose-key-type"]; ok {
				loose = true
			}
		}
		for _, c := range oneOf(d) {
			walk(c, visited)
		}
	}
	walk(name, map[string]bool{})
	return
}

// allowedValues returns the values of the definitions with allowed-values, following one-of.
func (s *schema) allowedValues(name string) []string {
	var ret []string
	visited := map[string]bool{}
	var walk func(string)
	walk = func(n string) {
		if visited[n] {
			return
		}
		visited[n] = true
		d := s.def(n)
		if d == nil {
			return
		}
		if l, ok := d["allowed-values"].([]any); ok {
			for _, v := range l {
				if str, ok := v.(string); ok {
					ret = append(ret, str)
				}
			}
		}
		if str, ok := d["string"].(map[string]any); ok {
			if c, ok := str["constant"].(string); ok {
				ret = append(ret, c)
			}
		}
		if q, ok := d["sequence"].(map[string]any); ok {
			if item, ok := q["item-type"].(string); ok {
				walk(item)
			}
		}
		for _, c := range oneOf(d) {
			walk(c)
		}
	}
	walk(name)
	sort.Strings(ret)
	ret = slices.Compact(ret)
	return ret
}

// property returns the type of the property of the mapping of a definition (or of its alternatives). "*" is a key
// which the mapping accepts through loose-value-type and "[]" an item of a sequence.
func (s *schema) property(name, key string) (string, bool) {
	visited := map[string]bool{}
	var walk func(string) (string, bool)
	walk = func(n string) (string, bool) {
		if visited[n] {
			return "", false
		}
		visited[n] = true
		d := s.def(n)
		if d == nil {
			return "", false
		}
		if m, ok := d["mapping"].(map[string]any); ok && key != "[]" {
			if key == "*" {
				if t, ok := m["loose-value-type"].(string); ok {
					return t, true
				}
			} else if props, ok := m["properties"].(map[string]any); ok {
				if v, ok := props[key]; ok {
					return typeName(v), true
				}
			}
		}
		if q, ok := d["sequence"].(map[string]any); ok && key == "[]" {
			if t, ok := q["item-type"].(string); ok {
				return t, true
			}
		}
		for _, c := range oneOf(d) {
			if t, ok := walk(c); ok {
				return t, true
			}
		}
		return "", false
	}
	return walk(name)
}

// contexts returns the contexts and the special functions that are available at the path: those of the definitions
// on the way from the root of the workflow (the contexts of a definition are available in everything below it).
// The names are lower case; a function is "name()" without its arity. found is false when the path does not exist.
func (s *schema) contexts(root string, path []string) (ctxs, funcs []string, found bool) {
	set := map[string]bool{}
	addDef := func(n string) {
		d := s.def(n)
		l, _ := d["context"].([]any)
		for _, v := range l {
			if str, ok := v.(string); ok {
				set[strings.ToLower(str)] = true
			}
		}
	}
	cur := root
	addDef(cur)
	for _, seg := range path {
		next, ok := s.property(cur, seg)
		if !ok {
			return nil, nil, false
		}
		cur = next
		addDef(cur)
	}
	for c := range set {
		if i := strings.IndexByte(c, '('); i >= 0 {
			funcs = append(funcs, c[:i])
		} else {
			ctxs = append(ctxs, c)
		}
	}
	sort.Strings(ctxs)
	sort.Strings(funcs)
	return ctxs, funcs, true
}

// ---------------------------------------------------------------------------------------------------------------
// Keys of the mappings

var reQuoted = regexp.MustCompile(`"([^"]*)"`)

// probeKeys parses the source, which has the key zz_probe in the mapping to look at, and returns the keys that the
// parser lists as expected for it. ok is false when the parser does not report the key.
func probeKeys(parse func(string) []*Error, src string) (keys map[string]bool, ok bool) {
	for _, e := range parse(src) {
		if !strings.Contains(e.Message, `unexpected key "zz_probe"`) {
			continue
		}
		_, list, found := strings.Cut(e.Message, "expected one of ")
		if !found {
			return nil, false
		}
		keys = map[string]bool{}
		for _, m := range reQuoted.FindAllStringSubmatch(list, -1) {
			keys[m[1]] = true
		}
		return keys, true
	}
	return nil, false
}

func parseWorkflowErrors(src string) []*Error {
	_, errs := Parse([]byte(src))
	return errs
}

func parseActionErrors(src string) []*Error {
	_, errs := ParseAction([]byte(src))
	return errs
}

const probeJobs = "jobs:\n  a:\n    runs-on: x\n    steps:\n      - run: x\n"

// keyProbe is a mapping of the schema and the source with which the keys of the parser are read.
type keyProbe struct {
	name   string
	defs   []string // the definitions of the schema, whose keys are united
	src    string
	action bool // the source is an action.yml and the definitions are in action_yaml.json
}

var keyProbes = []keyProbe{
	{name: "workflow", defs: []string{"workflow-root-strict"}, src: "zz_probe: 1\non: push\n" + probeJobs},
	{name: "job", defs: []string{"job-factory", "workflow-job"}, src: "on: push\njobs:\n  a:\n    zz_probe: 1\n    runs-on: x\n    steps:\n      - run: x\n"},
	{name: "run step", defs: []string{"run-step"}, src: "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: x\n        zz_probe: 1\n"},
	{name: "uses step", defs: []string{"regular-step"}, src: "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - uses: a/b@v1\n        zz_probe: 1\n"},
	{name: "snapshot", defs: []string{"snapshot-mapping"}, src: "on: push\njobs:\n  a:\n    runs-on: x\n    snapshot:\n      image-name: i\n      zz_probe: 1\n    steps:\n      - run: x\n"},
	{name: "container", defs: []string{"container-mapping"}, src: "on: push\njobs:\n  a:\n    runs-on: x\n    container:\n      image: i\n      zz_probe: 1\n    steps:\n      - run: x\n"},
	{name: "service container", defs: []string{"service-container-mapping"}, src: "on: push\njobs:\n  a:\n    runs-on: x\n    services:\n      s:\n        image: i\n        zz_probe: 1\n    steps:\n      - run: x\n"},
	{name: "strategy", defs: []string{"strategy"}, src: "on: push\njobs:\n  a:\n    runs-on: x\n    strategy:\n      zz_probe: 1\n      matrix:\n        a: [1]\n    steps:\n      - run: x\n"},
	{name: "environment", defs: []string{"job-environment-mapping"}, src: "on: push\njobs:\n  a:\n    runs-on: x\n    environment:\n      name: n\n      zz_probe: 1\n    steps:\n      - run: x\n"},
	{name: "concurrency", defs: []string{"concurrency-mapping"}, src: "on: push\nconcurrency:\n  group: g\n  zz_probe: 1\n" + probeJobs},
	{name: "defaults.run", defs: []string{"workflow-defaults-run"}, src: "on: push\ndefaults:\n  run:\n    shell: bash\n    zz_probe: 1\n" + probeJobs},
	{name: "runs-on", defs: []string{"runs-on-mapping"}, src: "on: push\njobs:\n  a:\n    runs-on:\n      group: g\n      zz_probe: 1\n    steps:\n      - run: x\n"},
	{name: "workflow_dispatch input", defs: []string{"workflow-dispatch-input"}, src: "on:\n  workflow_dispatch:\n    inputs:\n      i:\n        type: string\n        zz_probe: 1\n" + probeJobs},
	{name: "workflow_call input", defs: []string{"workflow-call-input-definition"}, src: "on:\n  workflow_call:\n    inputs:\n      i:\n        type: string\n        zz_probe: 1\n" + probeJobs},
	{name: "workflow_call secret", defs: []string{"workflow-call-secret-mapping-definition"}, src: "on:\n  workflow_call:\n    secrets:\n      s:\n        zz_probe: 1\n" + probeJobs},
	{name: "workflow_call output", defs: []string{"workflow-call-output-definition"}, src: "on:\n  workflow_call:\n    outputs:\n      o:\n        value: v\n        zz_probe: 1\n" + probeJobs},
	{name: "schedule entry", defs: []string{"cron-mapping"}, src: "on:\n  schedule:\n    - cron: '0 0 * * *'\n      zz_probe: 1\n" + probeJobs},
	{name: "action", action: true, defs: []string{"action-root"}, src: "zz_probe: 1\nname: n\ndescription: d\nruns:\n  using: node24\n  main: m\n"},
	// The parser has one list of keys for the three kinds of actions, and reports the keys of another kind with
	// another finding
	{name: "action runs", action: true, defs: []string{"node-runs", "container-runs", "composite-runs", "plugin-runs"}, src: "name: n\ndescription: d\nruns:\n  using: node24\n  main: m\n  zz_probe: 1\n"},
	{name: "action input", action: true, defs: []string{"input"}, src: "name: n\ndescription: d\ninputs:\n  i:\n    zz_probe: 1\nruns:\n  using: node24\n  main: m\n"},
	{name: "action output", action: true, defs: []string{"output-definition"}, src: "name: n\ndescription: d\noutputs:\n  o:\n    zz_probe: 1\nruns:\n  using: node24\n  main: m\n"},
	{name: "action composite run step", action: true, defs: []string{"run-step"}, src: "name: n\ndescription: d\nruns:\n  using: composite\n  steps:\n    - run: x\n      shell: bash\n      zz_probe: 1\n"},
	{name: "action composite uses step", action: true, defs: []string{"uses-step"}, src: "name: n\ndescription: d\nruns:\n  using: composite\n  steps:\n    - uses: a/b@v1\n      zz_probe: 1\n"},
}

func checkMappingKeys(t *testing.T, wf, act *schema, add func(area, format string, args ...any)) {
	t.Helper()
	for _, p := range keyProbes {
		s, parse := wf, parseWorkflowErrors
		if p.action {
			s, parse = act, parseActionErrors
		}
		want := map[string]bool{}
		loose := false
		for _, d := range p.defs {
			ks, l, found := s.keys(d)
			if !found {
				t.Fatalf("keys %q: the definition %q has no mapping in the schema", p.name, d)
			}
			loose = loose || l
			for k := range ks {
				want[k] = true
			}
		}
		have, ok := probeKeys(parse, p.src)
		if !ok {
			add("keys of "+p.name, "the parser does not report an unknown key")
			continue
		}
		for _, k := range sortedKeys(want) {
			if !have[k] {
				add("keys of "+p.name, "%q is only in the schema", k)
			}
		}
		// A mapping which accepts any key is not wrong for knowing more
		if !loose {
			for _, k := range sortedKeys(have) {
				if !want[k] {
					add("keys of "+p.name, "%q is only in the parser", k)
				}
			}
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	ret := make([]string, 0, len(m))
	for k := range m {
		ret = append(ret, k)
	}
	sort.Strings(ret)
	return ret
}

// ---------------------------------------------------------------------------------------------------------------
// Events and their activity types

// webhookKeys are the keys which the mapping of a webhook event of the schema can have and which the rules decide on.
var webhookKeys = []string{"types", "branches", "branches-ignore", "tags", "tags-ignore", "paths", "paths-ignore", "workflows"}

func checkEvents(t *testing.T, wf *schema, add func(area, format string, args ...any)) {
	t.Helper()
	on, ok := wf.property("workflow-root-strict", "on")
	if !ok {
		t.Fatal("the schema has no on")
	}
	events, _, found := wf.keys(on)
	if !found || len(events) < 20 {
		t.Fatalf("the schema has no events: %v", events)
	}

	ours := map[string]bool{"schedule": true, "workflow_dispatch": true, "workflow_call": true}
	for e := range AllWebhookTypes {
		ours[e] = true
	}
	for _, e := range sortedKeys(events) {
		if !ours[e] {
			add("events", "%q is only in the schema", e)
		}
	}
	for _, e := range sortedKeys(ours) {
		if !events[e] {
			add("events", "%q is only in jactionlint", e)
		}
	}

	for _, e := range sortedKeys(events) {
		typ, _ := wf.property("on-mapping-strict", e)
		if _, isWebhook := AllWebhookTypes[e]; !isWebhook {
			continue
		}
		// These have a mapping of their own, which the keys probes above look at
		switch e {
		case "schedule", "workflow_call", "workflow_dispatch", "repository_dispatch":
			continue
		}
		keys, _, _ := wf.keys(typ)

		// The activity types
		var schemaTypes []string
		if tt, ok := wf.property(firstMapping(wf, typ), "types"); ok {
			schemaTypes = wf.allowedValues(tt)
		}
		ourTypes := slices.Clone(AllWebhookTypes[e])
		sort.Strings(ourTypes)
		if ourTypes == nil && e != "repository_dispatch" {
			ourTypes = []string{}
		}
		if e == "repository_dispatch" {
			continue // the types are chosen by the sender
		}
		for _, ty := range schemaTypes {
			if !slices.Contains(ourTypes, ty) {
				add("activity types of "+e, "%q is only in the schema", ty)
			}
		}
		for _, ty := range ourTypes {
			if !slices.Contains(schemaTypes, ty) {
				add("activity types of "+e, "%q is only in jactionlint", ty)
			}
		}

		// The keys that the rules accept for the event
		if e == "image_version" {
			continue // names, versions and types are its own keys
		}
		for _, k := range webhookKeys {
			if k == "types" {
				continue
			}
			rejected := webhookKeyRejected(t, e, k)
			if keys[k] && rejected {
				add("keys of the event "+e, "%q is only in the schema", k)
			}
			if !keys[k] && !rejected {
				add("keys of the event "+e, "%q is only in the parser", k)
			}
		}
		if hasTypes := keys["types"]; hasTypes != (len(ourTypes) > 0) {
			if hasTypes {
				add("keys of the event "+e, `"types" is only in the schema`)
			} else {
				add("keys of the event "+e, `"types" is only in the parser`)
			}
		}
	}
}

// firstMapping returns the name of the first definition below name (or name itself) which has a mapping.
func firstMapping(s *schema, name string) string {
	visited := map[string]bool{}
	var walk func(string) string
	walk = func(n string) string {
		if visited[n] {
			return ""
		}
		visited[n] = true
		d := s.def(n)
		if _, ok := d["mapping"]; ok {
			return n
		}
		for _, c := range oneOf(d) {
			if r := walk(c); r != "" {
				return r
			}
		}
		return ""
	}
	return walk(name)
}

// webhookKeyRejected tells whether the rules report the key as not available for the event.
func webhookKeyRejected(t *testing.T, event, key string) bool {
	t.Helper()
	val := "[x]"
	if key == "workflows" {
		val = "[w]"
	}
	src := "on:\n  " + event + ":\n    " + key + ": " + val + "\n" + probeJobs
	for _, e := range lintFileWithConfig(t, fixtureConfig(), "test.yaml", src) {
		switch e.ID {
		case "invalid-event-filter":
			if strings.Contains(e.Message, `"`+key+`" filter is not available`) {
				return true
			}
		case "invalid-event-config":
			if strings.Contains(e.Message, `"`+key+`" cannot be`) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------------------------------------------
// Permissions

func checkPermissions(t *testing.T, wf *schema, add func(area, format string, args ...any)) {
	t.Helper()
	m, ok := wf.property("workflow-root-strict", "permissions")
	if !ok {
		t.Fatal("the schema has no permissions")
	}
	mapping := ""
	for _, c := range append([]string{m}, oneOf(wf.def(m))...) {
		if strings.Contains(c, "mapping") {
			mapping = c
		}
	}
	if mapping == "" {
		t.Fatalf("the schema has no mapping of permissions in %q", m)
	}
	scopes, _, _ := wf.keys(mapping)
	for _, s := range sortedKeys(scopes) {
		typ, _ := wf.property(mapping, s)
		levels := wf.allowedValues(typ)
		ours, ok := allPermissionScopes[s]
		if !ok {
			add("permissions", "the scope %q is only in the schema", s)
			continue
		}
		have := slices.Clone(ours)
		sort.Strings(have)
		if !slices.Equal(have, levels) {
			add("permissions", "the levels of %q are %v in the schema and %v in jactionlint", s, levels, have)
		}
	}
	for _, s := range sortedKeys(allPermissionScopes) {
		if !scopes[s] {
			add("permissions", "the scope %q is only in jactionlint", s)
		}
	}
}

// ---------------------------------------------------------------------------------------------------------------
// Contexts and functions

// availabilityPaths are the paths in the schema of the keys of the table of availability.go.
var availabilityPaths = map[string]string{
	"concurrency":                                      "concurrency",
	"env":                                              "env",
	"run-name":                                         "run-name",
	"jobs.<job_id>.concurrency":                        "jobs.*.concurrency",
	"jobs.<job_id>.container":                          "jobs.*.container",
	"jobs.<job_id>.container.credentials":              "jobs.*.container.credentials",
	"jobs.<job_id>.container.env.<env_id>":             "jobs.*.container.env.*",
	"jobs.<job_id>.container.image":                    "jobs.*.container.image",
	"jobs.<job_id>.continue-on-error":                  "jobs.*.continue-on-error",
	"jobs.<job_id>.defaults.run":                       "jobs.*.defaults.run",
	"jobs.<job_id>.env":                                "jobs.*.env",
	"jobs.<job_id>.environment":                        "jobs.*.environment",
	"jobs.<job_id>.environment.url":                    "jobs.*.environment.url",
	"jobs.<job_id>.if":                                 "jobs.*.if",
	"jobs.<job_id>.name":                               "jobs.*.name",
	"jobs.<job_id>.outputs.<output_id>":                "jobs.*.outputs.*",
	"jobs.<job_id>.runs-on":                            "jobs.*.runs-on",
	"jobs.<job_id>.secrets.<secrets_id>":               "jobs.*.secrets.*",
	"jobs.<job_id>.services":                           "jobs.*.services",
	"jobs.<job_id>.services.<service_id>.credentials":  "jobs.*.services.*.credentials",
	"jobs.<job_id>.services.<service_id>.env.<env_id>": "jobs.*.services.*.env.*",
	"jobs.<job_id>.snapshot.if":                        "jobs.*.snapshot.if",
	"jobs.<job_id>.steps.continue-on-error":            "jobs.*.steps.[].continue-on-error",
	"jobs.<job_id>.steps.env":                          "jobs.*.steps.[].env",
	"jobs.<job_id>.steps.if":                           "jobs.*.steps.[].if",
	"jobs.<job_id>.steps.name":                         "jobs.*.steps.[].name",
	"jobs.<job_id>.steps.run":                          "jobs.*.steps.[].run",
	"jobs.<job_id>.steps.timeout-minutes":              "jobs.*.steps.[].timeout-minutes",
	"jobs.<job_id>.steps.with":                         "jobs.*.steps.[].with",
	"jobs.<job_id>.steps.working-directory":            "jobs.*.steps.[].working-directory",
	"jobs.<job_id>.strategy":                           "jobs.*.strategy",
	"jobs.<job_id>.timeout-minutes":                    "jobs.*.timeout-minutes",
	"jobs.<job_id>.with.<with_id>":                     "jobs.*.with.*",
	"on.workflow_call.inputs.<inputs_id>.default":      "on.workflow_call.inputs.*.default",
	"on.workflow_call.outputs.<output_id>.value":       "on.workflow_call.outputs.*.value",
}

// actionAvailability are the places of an action.yml where an expression is evaluated: the path in
// action_yaml.json, and the source of an action with the placeholder @@ in the expression there.
var actionAvailability = []struct{ path, src string }{
	{"inputs.*.default", "name: n\ndescription: d\ninputs:\n  i:\n    default: ${{ @@ }}\nruns:\n  using: node24\n  main: m\n"},
	{"outputs.*.value", "name: n\ndescription: d\noutputs:\n  o:\n    value: ${{ @@ }}\nruns:\n  using: composite\n  steps:\n    - run: x\n      shell: bash\n"},
	{"runs.steps.[].run", "name: n\ndescription: d\nruns:\n  using: composite\n  steps:\n    - run: echo ${{ @@ }}\n      shell: bash\n"},
	{"runs.steps.[].if", "name: n\ndescription: d\nruns:\n  using: composite\n  steps:\n    - run: x\n      shell: bash\n      if: ${{ @@ }}\n"},
	{"runs.steps.[].env", "name: n\ndescription: d\nruns:\n  using: composite\n  steps:\n    - run: x\n      shell: bash\n      env:\n        V: ${{ @@ }}\n"},
	{"runs.steps.[].with", "name: n\ndescription: d\nruns:\n  using: composite\n  steps:\n    - uses: a/b@v1\n      with:\n        v: ${{ @@ }}\n"},
}

// actionProbeContexts and actionProbeFunctions are what is tried in an action.yml, where the tables of the
// rules are not read directly but through the findings.
var (
	actionProbeContexts  = []string{"github", "env", "vars", "secrets", "needs", "inputs", "steps", "job", "runner", "strategy", "matrix"}
	actionProbeFunctions = map[string]string{"always": "always()", "cancelled": "cancelled()", "failure": "failure()", "success": "success()", "hashfiles": "hashFiles('a')"}
)

func checkAvailability(t *testing.T, wf, act *schema, add func(area, format string, args ...any)) {
	t.Helper()
	for _, key := range sortedKeys(availabilityPaths) {
		ctx, fn, found := wf.contexts("workflow-root-strict", strings.Split(availabilityPaths[key], "."))
		if !found {
			add("contexts at "+key, "the path %q does not exist in the schema", availabilityPaths[key])
			continue
		}
		haveCtx, haveFn := WorkflowKeyAvailability(key)
		compareAvailability(add, key, ctx, fn, haveCtx, haveFn)
	}
	for _, k := range allWorkflowKeys {
		if _, ok := availabilityPaths[k]; !ok {
			t.Errorf("the key %q of availability.go has no path in availabilityPaths", k)
		}
	}
	for _, a := range actionAvailability {
		ctx, fn, found := act.contexts("action-root", strings.Split(a.path, "."))
		if !found {
			add("contexts at "+a.path+" of an action", "the path does not exist in the schema")
			continue
		}
		var haveCtx, haveFn []string
		for _, c := range actionProbeContexts {
			if !expressionRejected(t, a.src, c+".x", `context "`+c+`" is not allowed`) {
				haveCtx = append(haveCtx, c)
			}
		}
		for _, f := range sortedKeys(actionProbeFunctions) {
			if !expressionRejected(t, a.src, actionProbeFunctions[f], `calling function "`+f) {
				haveFn = append(haveFn, f)
			}
		}
		compareAvailability(add, a.path+" of an action", ctx, fn, haveCtx, haveFn)
	}
}

// expressionRejected tells whether the action with the expression in place of @@ has a finding with the text.
func expressionRejected(t *testing.T, src, expr, text string) bool {
	t.Helper()
	for _, e := range lintActionSource(t, strings.ReplaceAll(src, "@@", expr)) {
		if strings.Contains(strings.ToLower(e.Message), strings.ToLower(text)) {
			return true
		}
	}
	return false
}

func compareAvailability(add func(area, format string, args ...any), key string, ctx, fn, haveCtx, haveFn []string) {
	area := "contexts at " + key
	haveCtx, haveFn = slices.Clone(haveCtx), slices.Clone(haveFn)
	sort.Strings(haveCtx)
	sort.Strings(haveFn)
	for _, c := range ctx {
		if !slices.Contains(haveCtx, c) {
			add(area, "the context %q is only in the schema", c)
		}
	}
	for _, c := range haveCtx {
		if !slices.Contains(ctx, c) {
			add(area, "the context %q is only in jactionlint", c)
		}
	}
	for _, f := range fn {
		if !slices.Contains(haveFn, f) {
			add(area, "the function %q is only in the schema", f)
		}
	}
	for _, f := range haveFn {
		if !slices.Contains(fn, f) {
			add(area, "the function %q is only in jactionlint", f)
		}
	}
}

// ---------------------------------------------------------------------------------------------------------------
// Whole values which are expressions

// wholeValue is a place where the value is a mapping or a sequence, the path of the definition in the schema and the
// source which has the placeholder @@ at the place.
type wholeValue struct {
	path string
	src  string
}

var wholeValues = []wholeValue{
	{"jobs.*.strategy", "on: push\njobs:\n  a:\n    runs-on: x\n    strategy: @@\n    steps:\n      - run: x\n"},
	{"jobs.*.container", "on: push\njobs:\n  a:\n    runs-on: x\n    container: @@\n    steps:\n      - run: x\n"},
	{"jobs.*.services", "on: push\njobs:\n  a:\n    runs-on: x\n    services: @@\n    steps:\n      - run: x\n"},
	{"jobs.*.environment", "on: push\njobs:\n  a:\n    runs-on: x\n    environment: @@\n    steps:\n      - run: x\n"},
	{"jobs.*.concurrency", "on: push\njobs:\n  a:\n    runs-on: x\n    concurrency: @@\n    steps:\n      - run: x\n"},
	{"concurrency", "on: push\nconcurrency: @@\n" + probeJobs},
	{"jobs.*.env", "on: push\njobs:\n  a:\n    runs-on: x\n    env: @@\n    steps:\n      - run: x\n"},
	{"env", "on: push\nenv: @@\n" + probeJobs},
	{"jobs.*.defaults.run", "on: push\njobs:\n  a:\n    runs-on: x\n    defaults:\n      run: @@\n    steps:\n      - run: x\n"},
	{"jobs.*.snapshot", "on: push\njobs:\n  a:\n    runs-on: x\n    snapshot: @@\n    steps:\n      - run: x\n"},
	{"jobs.*.runs-on", "on: push\njobs:\n  a:\n    runs-on: @@\n    steps:\n      - run: x\n"},
	{"jobs.*.steps.[].with", "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - uses: a/b@v1\n        with: @@\n"},
	{"jobs.*.steps.[].env", "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: x\n        env: @@\n"},
}

func checkWholeValueExpressions(t *testing.T, wf *schema, add func(area, format string, args ...any)) {
	t.Helper()
	for _, w := range wholeValues {
		ctx, _, found := wf.contexts("workflow-root-strict", strings.Split(w.path, "."))
		if !found {
			add("whole value expression at "+w.path, "the path does not exist in the schema")
			continue
		}
		schemaAccepts := len(ctx) > 0
		src := strings.ReplaceAll(w.src, "@@", "${{ fromJSON('{}') }}")
		// Only the errors of the parser say that the value is not accepted
		_, errs := Parse([]byte(src))
		haveAccepts := len(errs) == 0
		switch {
		case schemaAccepts && !haveAccepts:
			add("whole value expression at "+w.path, "the schema accepts an expression and the parser does not: %s", errs[0].Message)
		case !schemaAccepts && haveAccepts:
			add("whole value expression at "+w.path, "the parser accepts an expression and the schema does not")
		}
	}
}

// pendingPrefix marks the entries of the differences which a pull request that is not merged yet adds or removes. They are not
// required to exist, so the pull requests can be merged in any order. Remove the prefix, or the entry, when they are in.
const pendingPrefix = "pending: "

const newerThanPin = "the schema of the runner at the pinned commit is older than the documentation and the schema of the language service (actions/languageservices), which have it"

// schemaConformanceAllowed are the differences between the parser and the schema which are on purpose or known, each
// with the reason. The key is the text of the difference which the test prints.
var schemaConformanceAllowed = map[string]string{
	"activity types of image_version: \"created\" is only in the schema":                                        pendingPrefix + "#170 adds the types of the image_version event (the docs say \"Not applicable\")",
	"activity types of image_version: \"deleted\" is only in the schema":                                        pendingPrefix + "#170 adds the types of the image_version event (the docs say \"Not applicable\")",
	"activity types of image_version: \"ready\" is only in the schema":                                          pendingPrefix + "#170 adds the types of the image_version event (the docs say \"Not applicable\")",
	"activity types of issues: \"field_added\" is only in jactionlint":                                          newerThanPin,
	"activity types of issues: \"field_removed\" is only in jactionlint":                                        newerThanPin,
	"activity types of issues: \"typed\" is only in jactionlint":                                                newerThanPin,
	"activity types of issues: \"untyped\" is only in jactionlint":                                              newerThanPin,
	"activity types of merge_group: \"destroyed\" is only in jactionlint":                                       pendingPrefix + "#186 removes it. The docs of the workflow trigger and both schemas list only checks_requested; 23f9662c added destroyed from the docs of the webhook",
	"activity types of pull_request: \"demilestoned\" is only in jactionlint":                                   newerThanPin,
	"activity types of pull_request: \"dequeued\" is only in jactionlint":                                       newerThanPin,
	"activity types of pull_request: \"enqueued\" is only in jactionlint":                                       newerThanPin,
	"activity types of pull_request: \"milestoned\" is only in jactionlint":                                     newerThanPin,
	"activity types of pull_request: \"stacked\" is only in jactionlint":                                        pendingPrefix + "#170 adds it (actions/languageservices#384); it is not in the runner schema yet",
	"activity types of pull_request_target: \"demilestoned\" is only in jactionlint":                            newerThanPin,
	"activity types of pull_request_target: \"dequeued\" is only in jactionlint":                                newerThanPin,
	"activity types of pull_request_target: \"enqueued\" is only in jactionlint":                                newerThanPin,
	"activity types of pull_request_target: \"milestoned\" is only in jactionlint":                              newerThanPin,
	"activity types of pull_request_target: \"stacked\" is only in jactionlint":                                 pendingPrefix + "#170 adds it (actions/languageservices#384); it is not in the runner schema yet",
	"contexts at inputs.*.default of an action: the context \"env\" is only in jactionlint":                     pendingPrefix + "#186 restricts the default of an input to the contexts of input-default-context",
	"contexts at inputs.*.default of an action: the context \"inputs\" is only in jactionlint":                  pendingPrefix + "#186 restricts the default of an input to the contexts of input-default-context",
	"contexts at inputs.*.default of an action: the context \"needs\" is only in jactionlint":                   pendingPrefix + "#186 restricts the default of an input to the contexts of input-default-context",
	"contexts at inputs.*.default of an action: the context \"secrets\" is only in jactionlint":                 pendingPrefix + "#186 restricts the default of an input to the contexts of input-default-context",
	"contexts at inputs.*.default of an action: the context \"steps\" is only in jactionlint":                   pendingPrefix + "#186 restricts the default of an input to the contexts of input-default-context",
	"contexts at inputs.*.default of an action: the context \"vars\" is only in jactionlint":                    pendingPrefix + "#186 restricts the default of an input to the contexts of input-default-context",
	"contexts at jobs.<job_id>.container.env.<env_id>: the context \"steps\" is only in the schema":             "The table is generated from the docs, which do not list steps there. The runner schema allows it, but nothing says it is available at run time, so it is not added",
	"contexts at jobs.<job_id>.services.<service_id>.env.<env_id>: the context \"steps\" is only in the schema": "The table is generated from the docs, which do not list steps there. The runner schema allows it, but nothing says it is available at run time, so it is not added",
	"contexts at jobs.<job_id>.snapshot.if: the context \"env\" is only in the schema":                          pendingPrefix + "#170 allows them (actions/runner#4443)",
	"contexts at jobs.<job_id>.snapshot.if: the context \"job\" is only in the schema":                          pendingPrefix + "#170 allows them (actions/runner#4443)",
	"contexts at jobs.<job_id>.snapshot.if: the context \"runner\" is only in the schema":                       pendingPrefix + "#170 allows them (actions/runner#4443)",
	"contexts at jobs.<job_id>.snapshot.if: the context \"steps\" is only in the schema":                        pendingPrefix + "#170 allows them (actions/runner#4443)",
	"contexts at jobs.<job_id>.snapshot.if: the function \"always\" is only in the schema":                      pendingPrefix + "#170 allows them (actions/runner#4443)",
	"contexts at jobs.<job_id>.snapshot.if: the function \"cancelled\" is only in the schema":                   pendingPrefix + "#170 allows them (actions/runner#4443)",
	"contexts at jobs.<job_id>.snapshot.if: the function \"failure\" is only in the schema":                     pendingPrefix + "#170 allows them (actions/runner#4443)",
	"contexts at jobs.<job_id>.snapshot.if: the function \"hashfiles\" is only in the schema":                   pendingPrefix + "#170 allows them (actions/runner#4443)",
	"contexts at jobs.<job_id>.snapshot.if: the function \"success\" is only in the schema":                     pendingPrefix + "#170 allows them (actions/runner#4443)",
	"contexts at outputs.*.value of an action: the context \"needs\" is only in jactionlint":                    "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at outputs.*.value of an action: the context \"vars\" is only in jactionlint":                     "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at outputs.*.value of an action: the function \"hashfiles\" is only in jactionlint":               "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at runs.steps.[].env of an action: the context \"needs\" is only in jactionlint":                  "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at runs.steps.[].env of an action: the context \"vars\" is only in jactionlint":                   "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at runs.steps.[].if of an action: the context \"needs\" is only in jactionlint":                   "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at runs.steps.[].if of an action: the context \"vars\" is only in jactionlint":                    "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at runs.steps.[].run of an action: the context \"needs\" is only in jactionlint":                  "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at runs.steps.[].run of an action: the context \"vars\" is only in jactionlint":                   "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at runs.steps.[].with of an action: the context \"needs\" is only in jactionlint":                 "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"contexts at runs.steps.[].with of an action: the context \"vars\" is only in jactionlint":                  "A composite action is checked with the table of the steps of a workflow, which has more contexts than the schema of the runner. It is not documented what the runner passes on, so the findings would be guesses (compositeContexts)",
	"events: \"project\" is only in the schema":                                                                 "Classic Projects is sunset and pull_request_comment is an alias which the docs replace with issue_comment. They are reported as unknown events on purpose",
	"events: \"project_card\" is only in the schema":                                                            "Classic Projects is sunset and pull_request_comment is an alias which the docs replace with issue_comment. They are reported as unknown events on purpose",
	"events: \"project_column\" is only in the schema":                                                          "Classic Projects is sunset and pull_request_comment is an alias which the docs replace with issue_comment. They are reported as unknown events on purpose",
	"events: \"pull_request_comment\" is only in the schema":                                                    "Classic Projects is sunset and pull_request_comment is an alias which the docs replace with issue_comment. They are reported as unknown events on purpose",
	"keys of action composite run step: \"background\" is only in the parser":                                   "Parallel steps (changelog 2026-06-25) are newer than the pinned schema. It is not known whether a composite action accepts them",
	"keys of action composite run step: \"timeout-minutes\" is only in the parser":                              "The steps of workflows and of composite actions are read with one list of keys. timeout-minutes in a composite step is reported by its own finding (#186)",
	"keys of action composite uses step: \"background\" is only in the parser":                                  "Parallel steps (changelog 2026-06-25) are newer than the pinned schema. It is not known whether a composite action accepts them",
	"keys of action composite uses step: \"timeout-minutes\" is only in the parser":                             "The steps of workflows and of composite actions are read with one list of keys. timeout-minutes in a composite step is reported by its own finding (#186)",
	"keys of action runs: \"plugin\" is only in the schema":                                                     "plugin-runs is for the internal actions of GitHub",
	"keys of concurrency: \"queue\" is only in the parser":                                                      newerThanPin,
	"keys of environment: \"deployment\" is only in the parser":                                                 newerThanPin,
	"keys of job: \"cache-mode\" is only in the parser":                                                         newerThanPin,
	"keys of job: \"cancel-timeout-minutes\" is only in the schema":                                             pendingPrefix + "#170 adds it",
	"keys of run step: \"background\" is only in the parser":                                                    "Parallel steps (changelog 2026-06-25) are newer than the pinned schema",
	"keys of schedule entry: \"timezone\" is only in the parser":                                                newerThanPin,
	"keys of the event merge_group: \"branches\" is only in the parser":                                         "The docs of the workflow syntax have on.merge_group.branches and branches-ignore. The runner schema has only types",
	"keys of the event merge_group: \"branches-ignore\" is only in the parser":                                  "The docs of the workflow syntax have on.merge_group.branches and branches-ignore. The runner schema has only types",
	"keys of uses step: \"background\" is only in the parser":                                                   "Parallel steps (changelog 2026-06-25) are newer than the pinned schema",
	"keys of workflow: \"cache-mode\" is only in the parser":                                                    newerThanPin,
	"keys of workflow: \"description\" is only in the schema":                                                   pendingPrefix + "#170 adds it",
	"permissions: the scope \"code-quality\" is only in jactionlint":                                            newerThanPin,
	"permissions: the scope \"copilot-requests\" is only in jactionlint":                                        newerThanPin,
	"permissions: the scope \"drives\" is only in jactionlint":                                                  pendingPrefix + "#170 adds it (actions/languageservices#380)",
}

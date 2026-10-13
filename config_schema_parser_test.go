package jactionlint

import (
	"testing"
)

// TestConfigSchemaAgreesWithTheParser runs the same snippets through the parser and through the schema: an editor
// must accept what the parser accepts and reject what it rejects. A snippet is the YAML and whether the parser takes
// it (the expectation is written down, so a change of the parser shows here as well).
func TestConfigSchemaAgreesWithTheParser(t *testing.T) {
	v := loadJSONSchema(t, ConfigSchema())
	var plain string
	for _, r := range ruleRegistry {
		if len(r.Options) == 0 && !r.Online {
			plain = r.ID
			break
		}
	}
	const withOptions = "max-run-lines"
	if _, ok := ruleIndex[withOptions]; !ok || plain == "" {
		t.Fatal("the registry has no rule to test with")
	}
	type snippet struct {
		src    string
		parses bool
	}
	var corpus []snippet
	add := func(parses bool, src ...string) {
		for _, s := range src {
			corpus = append(corpus, snippet{s, parses})
		}
	}
	rule := func(id, value string) string { return "rules:\n  " + id + ": " + value + "\n" }

	// A rule without options: a level, a boolean, or a mapping with the level only
	add(true, rule(plain, "warn"), rule(plain, "error"), rule(plain, "info"), rule(plain, "off"), rule(plain, "true"), rule(plain, "false"),
		rule(plain, "{}"), rule(plain, "{level: warn}"), rule(plain, "{level: off}"), rule(plain, "{level: error}"))
	add(false, rule(plain, "fatal"), rule(plain, "5"), rule(plain, "[warn]"), rule(plain, "{level: true}"), rule(plain, "{level: false}"),
		rule(plain, "{level: fatal}"), rule(plain, "{level: [warn]}"), rule(plain, "{max: 3}"), rule(plain, "{level: warn, max: 3}"))
	// A rule with options
	add(true, rule(withOptions, "warn"), rule(withOptions, "true"), rule(withOptions, "{}"), rule(withOptions, "{max: 30}"),
		rule(withOptions, "{level: warn, max: 30}"), rule(withOptions, "{level: error}"))
	add(false, rule(withOptions, "{level: true}"), rule(withOptions, "{level: false}"), rule(withOptions, "{max: -1}"),
		rule(withOptions, "{max: lots}"), rule(withOptions, "{max: 1.5}"), rule(withOptions, "{maxx: 3}"), rule(withOptions, "[]"))
	add(false, "rules:\n  no-such-rule: warn\n", "rules: warn\n", "rules: [a]\n")
	// ignores
	add(true,
		"ignores:\n  - rule: "+plain+"\n    file: a.yaml\n",
		"ignores:\n  - rule: ["+plain+", "+withOptions+"]\n    job: build\n",
		"ignores:\n  - rule: "+plain+"\n    uses: actions/checkout\n    reason: because\n    expires: '2027-06-30'\n",
		"ignores:\n  - rule: "+plain+"\n    step: Build\n    expires: 2027-06-30\n")
	add(false,
		"ignores:\n  - rule: "+plain+"\n",
		"ignores:\n  - file: a.yaml\n",
		"ignores:\n  - rule: nope\n    file: a.yaml\n",
		"ignores:\n  - rule: []\n    file: a.yaml\n",
		"ignores:\n  - rule: "+plain+"\n    file: a.yaml\n    expires: tomorrow\n",
		"ignores:\n  - rule: "+plain+"\n    file: a.yaml\n    when: now\n",
		"ignores: a\n")
	// the other keys
	add(true, "profile: correctness\n", "profile: default\n", "profile: pedantic\n", "profile: strict\n", "profile:\n", "online: true\n", "online:\n",
		"baseline: auto\n", "baseline: true\n", "baseline: false\n", "baseline: some/file.json\n", "extends: [a.yaml]\n",
		"config-variables: null\n", "config-variables: []\n", "config-variables: [A]\n", "config-secrets: [A]\n",
		"self-hosted-runner:\n  labels: [a]\n  strict-labels: true\n", "paths:\n  '**/*.yaml':\n    ignore: [a, 'b.*']\n",
		"required-actions:\n  - action: actions/checkout\n    version: v4\n", "assume-default-permissions: restricted\n", "generated-files: skip\n",
		"fix:\n  rules: ["+plain+"]\n", "online-options:\n  mode: cache,strict\n  retries: 3\n  cache-ttl: 1h\n  deny: [a/b]\n",
		"rules:\n", "")
	add(false, "profile: bogus\n", "profile: [default]\n", "online: maybe\n", "baseline: [a]\n", "extends: a.yaml\n",
		"config-variables: A\n", "config-secrets: 1\n", "self-hosted-runner:\n  labels: a\n", "self-hosted-runner:\n  label: [a]\n",
		"paths:\n  a:\n    ignored: [a]\n", "required-actions:\n  - version: v4\n", "required-actions:\n  - action: a/b\n    extra: 1\n",
		"assume-default-permissions: always\n", "generated-files: sometimes\n", "fix:\n  rules: [nope]\n", "online-options:\n  mode: sometimes\n",
		"online-options:\n  retries: -1\n", "online-options:\n  nope: 1\n", "unknown-key: 1\n")

	for _, c := range corpus {
		// "extends" is an error of ParseConfig by itself (it needs the path of a file): the YAML is valid for ReadConfigFile
		_, err := parseConfig([]byte(c.src))
		if (err == nil) != c.parses {
			t.Errorf("the parser answers %v for %q (want %v): %v", err == nil, c.src, c.parses, err)
			continue
		}
		errs := validateConfigYAML(t, v, c.src)
		if (len(errs) == 0) != c.parses {
			verb, other := "accepts", "rejects"
			if !c.parses {
				verb, other = other, verb
			}
			t.Errorf("the parser %s %q but the schema %s it: %v", verb, c.src, other, errs)
		}
	}
}

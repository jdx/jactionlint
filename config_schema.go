package jactionlint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// This file generates the two JSON Schemas of jactionlint: the one of the config file
// (.github/jactionlint.yaml), from the rule registry and the keys the config parser knows, and the one of
// `--format json` and `--format jsonl`, the stable contract for the programs which read the findings. The
// generated documents are committed as schema/jactionlint-config.json and schema/jactionlint-output.json;
// TestSchemasAreUpToDate keeps them in sync, and `mise run schema` writes them.

const (
	// ConfigSchemaURL is where the JSON Schema of the config file is published. The comment which --init-config
	// writes at the top of the file points editors to it.
	ConfigSchemaURL = "https://raw.githubusercontent.com/jdx/jactionlint/main/schema/jactionlint-config.json"
	// OutputSchemaURL is where the JSON Schema of --format json is published.
	OutputSchemaURL = "https://raw.githubusercontent.com/jdx/jactionlint/main/schema/jactionlint-output.json"

	schemaDraft = "http://json-schema.org/draft-07/schema#"
)

type jsonObject = map[string]any

func sType(desc, typ string) jsonObject {
	return jsonObject{"type": typ, "description": desc}
}

func sStr(desc string) jsonObject  { return sType(desc, "string") }
func sBool(desc string) jsonObject { return sType(desc, "boolean") }
func sInt(desc string) jsonObject {
	o := sType(desc, "integer")
	o["minimum"] = 0
	return o
}

// sIntMax is a non-negative integer with an upper limit.
func sIntMax(desc string, max int) jsonObject {
	o := sInt(desc)
	o["maximum"] = max
	return o
}

// sNumber is a non-negative number.
func sNumber(desc string) jsonObject {
	o := sType(desc, "number")
	o["minimum"] = 0
	return o
}

func sEnum(desc string, vals ...string) jsonObject {
	o := sType(desc, "string")
	o["enum"] = vals
	return o
}

func sArray(desc string, items jsonObject) jsonObject {
	return jsonObject{"type": "array", "description": desc, "items": items}
}

func sStrings(desc string) jsonObject { return sArray(desc, jsonObject{"type": "string"}) }

// sObject is a mapping with the listed properties and no others.
func sObject(desc string, props jsonObject, required ...string) jsonObject {
	o := jsonObject{"type": "object", "description": desc, "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func sRef(name string) jsonObject { return jsonObject{"$ref": "#/definitions/" + name} }

// sDescribedRef is a reference with a description. In draft-07 every keyword next to a "$ref" is ignored, so the
// reference goes in an allOf and the description stays beside it.
func sDescribedRef(desc, name string) jsonObject {
	return jsonObject{"description": desc, "allOf": []jsonObject{sRef(name)}}
}

func sDeprecated(o jsonObject) jsonObject {
	o["deprecated"] = true
	return o
}

func marshalSchema(v jsonObject) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		panic(err) // the schema is made of strings, numbers and booleans
	}
	return b.Bytes()
}

// levelValues are the severities a rule can be set to in the config file.
var levelValues = []string{"off", "info", "warn", "warning", "error"}

// ConfigSchema returns the JSON Schema (draft-07) of the config file as indented JSON. Rule IDs are an enum and
// each rule lists its options, so editors complete and check them.
func ConfigSchema() []byte {
	ruleProps := jsonObject{}
	for _, r := range ruleRegistry {
		ruleProps[r.ID] = ruleSchema(r)
	}

	profiles := []string{string(ProfileCorrectness), string(ProfileDefault), string(ProfilePedantic)}
	ids := ruleIDs()

	props := jsonObject{
		"profile": jsonObject{
			"description": "The set of rules which run: correctness (what actionlint checks and the bug detectors of jactionlint), default (adds the security posture and policy rules worth failing a build on) or pedantic (adds the noisy and opinionated rules). Each profile includes the one before it. The --profile flag overrides it. The retired names strict and all mean pedantic.",
			"type":        "string",
			"enum":        append(append([]string{}, profiles...), deprecatedProfileStrict, deprecatedProfileAll),
			"default":     string(ProfileDefault),
		},
		"extends": sStrings("Config files to inherit from. Relative paths are resolved from the directory of this file. Later files win over earlier ones and this file wins over all of them."),
		"rules": jsonObject{
			"type":                 "object",
			"description":          "The level and the options of each rule by its rule ID. A rule which is not listed follows the profile. See https://jactionlint.jdx.dev/rules",
			"properties":           ruleProps,
			"additionalProperties": false,
		},
		"online":         sBool("Turns on the checks which query the GitHub API for the files this configuration applies to, like the --online flag. The default is false: nothing uses the network."),
		"online-options": onlineOptionsSchema(),
		"baseline": jsonObject{
			"description": "The baseline of accepted findings. auto (or true) applies .github/jactionlint-baseline.json when the file exists, false or no value applies none, and any other value is the path of the baseline file relative to the repository root, which must exist. The --baseline flag overrides it.",
			"type":        []string{"string", "boolean"},
		},
		"self-hosted-runner": sObject("Configuration for your self-hosted runners.", jsonObject{
			"labels":        sStrings("Label names of your self-hosted runners. Glob patterns are supported."),
			"strict-labels": sBool("When true, runs-on accepts only the labels listed in labels. The built-in labels are reported unless they are listed too. The default is false."),
			"providers":     nullableArray("The hosted-runner providers whose labels runs-on accepts without them being listed in labels: "+strings.Join(runnerProviderNames(), ", ")+". Names are not case sensitive. Unset (or null) accepts all of them, an empty list none. It has no effect with strict-labels.", jsonObject{"type": "string", "pattern": caseInsensitiveAlternatives(runnerProviderNames())}),
		}),
		"config-variables": nullableStrings("Names of the configuration variables of your repository or organization. When it is a list, the vars context is checked against it (an empty list allows none). null disables the check."),
		"config-secrets":   nullableStrings("Names of the secrets of your repository or organization. When it is a list, the secrets context is checked against it (an empty list allows none; GITHUB_TOKEN is always allowed). null disables the check."),
		"paths": jsonObject{
			"type":        "object",
			"description": "Configuration for the files matching a glob pattern, relative to the repository root. The path separator is always /.",
			"additionalProperties": sObject("The configuration of the files matching the pattern.", jsonObject{
				"ignore": sStrings("Rule IDs and regular expressions. A rule ID ignores all the findings of the rule, and a regular expression ignores the findings whose message matches it."),
			}),
		},
		"ignores": sArray("Findings to accept, matched by the rule and by where they are (file, job, step, uses) instead of by line, so they survive tools which rewrite the workflow. At least one of file, uses, job and step is required.", sIgnore(ids)),
		"required-actions": sArray("Actions or reusable workflows which must be used at least once in every checked workflow. The check is off when the list is empty.", sObject("A required action.", jsonObject{
			"action":  sStr("The name of the action, like actions/checkout, without @version. A sub-path is part of the name. It is case-insensitive."),
			"version": sStr("The exact ref (tag, branch or commit SHA) which one use of the action must have. Any version is accepted when it is omitted."),
		}, "action")),
		"assume-default-permissions": sEnum("What to assume for a caller workflow which declares no permissions: restricted is GitHub's restricted default token, permissive is write on every scope except id-token. Unset assumes nothing.", AssumeDefaultPermissionsRestricted, AssumeDefaultPermissionsPermissive),
		"platform":                   sEnum("The system which runs the workflows: github (the default), or forgejo or gitea, which also accept `uses: https://host/owner/repo@ref` for an action. That form stays an error on github.", PlatformGitHub, PlatformForgejo, PlatformGitea),
		"generated-files": jsonObject{
			"description": "What to do with a file whose first comment says that a tool wrote it (DO NOT EDIT, automatically generated by ...): skip-policy (the default) does not report the rules of the policy and style groups there, report treats it like any other file, skip reports nothing.",
			"type":        "string",
			"enum":        []string{GeneratedFilesReport, GeneratedFilesSkipPolicy, GeneratedFilesSkip},
			"default":     GeneratedFilesSkipPolicy,
		},
		"shellcheck": sObject("How shellcheck is run for the scripts of run steps. By default it runs with --norc.", jsonObject{
			"rcfile": sStr("A shellcheckrc file to read instead of --norc. A relative path is resolved from the repository root. The file must exist. A config file of the repository can name only a file inside the repository."),
			"args":   sStrings("More arguments for shellcheck, one option per item with its value attached, such as --exclude=SC2129. The options jactionlint sets itself (--format, --color, --shell, --norc, --rcfile, --version, --wiki-link-count) and file names are errors."),
			"enable": sStrings("Optional checks of shellcheck to turn on (--enable=NAME,...), such as avoid-nullary-conditions, or all."),
		}),
		"fix": sObject("Configuration of --fix.", jsonObject{
			"rules": sArray("Apply only the fixes of these rules, like --fix-rules (which overrides it). All fixes are applied when the list is empty.", jsonObject{"type": "string", "enum": ids}),
		}),

		// Deprecated keys. They still work and are translated into rules: jactionlint --migrate-config rewrites them.
		"timeout-minutes": sDeprecated(sObject("Deprecated: use the missing-timeout and timeout-too-long rules.", jsonObject{
			"required": sBool("Deprecated: enables missing-timeout."),
			"max":      sNumber("Deprecated: the max option of timeout-too-long."),
		})),
		"require-commit-hash":                  sDeprecated(sBool("Deprecated: turn on the unpinned-uses rule.")),
		"require-permissions":                  sDeprecated(sBool("Deprecated: turn on the missing-permissions rule.")),
		"require-checkout-before-local-action": sDeprecated(sBool("Deprecated: turn on the local-action-checkout rule.")),
		"require-expression-wrapping":          sDeprecated(sBool("Deprecated: turn on the require-expression-wrapping rule.")),
		"check-falsy-ternary":                  sDeprecated(sBool("Deprecated: turn on the unsound-ternary rule.")),
		"check-workflow-run-names":             sDeprecated(sBool("Deprecated: turn on the workflow-run-names rule.")),
		"require-shell":                        sDeprecated(sBool("Deprecated: turn on the require-shell rule.")),
		"max-run-lines":                        sDeprecated(sInt("Deprecated: the max option of the max-run-lines rule.")),
	}

	// A key with no value (`rules:` followed only by comments, which --init-config writes) is null, and the parser
	// reads it as not set.
	for _, p := range props {
		nullable(p.(jsonObject))
	}

	return marshalSchema(jsonObject{
		"$schema":              schemaDraft,
		"$id":                  ConfigSchemaURL,
		"title":                "jactionlint configuration",
		"description":          "The configuration file of jactionlint, .github/jactionlint.yaml. Generated from the rule registry by `jactionlint --print-config-schema`. See https://jactionlint.jdx.dev/config",
		"type":                 []string{"object", "null"}, // an empty file, or one with only comments, is null: no settings
		"additionalProperties": false,
		"properties":           props,
		"definitions": jsonObject{
			"severity": jsonObject{
				"description": "The level of a rule: error (fails the run), warn, info or off.",
				"type":        "string",
				"enum":        levelValues,
			},
			"level": jsonObject{
				"description": "The value of a rule: a level (error, warn, info or off), true for the default level of the rule, or false for off.",
				"oneOf": []jsonObject{
					sRef("severity"),
					{"type": "boolean"},
				},
			},
		},
	})
}

// caseInsensitiveAlternatives returns a pattern which matches exactly one of the names whatever the case of its letters
// (the pattern language of JSON Schema has no flag for it).
func caseInsensitiveAlternatives(names []string) string {
	alts := make([]string, 0, len(names))
	for _, n := range names {
		var b strings.Builder
		for _, r := range n {
			if lo, up := strings.ToLower(string(r)), strings.ToUpper(string(r)); lo != up {
				fmt.Fprintf(&b, "[%s%s]", lo, up)
			} else {
				b.WriteString(regexp.QuoteMeta(string(r)))
			}
		}
		alts = append(alts, b.String())
	}
	return "^(" + strings.Join(alts, "|") + ")$"
}

// nullableArray is an array which can also be null (a key with no value is not set).
func nullableArray(desc string, items jsonObject) jsonObject {
	o := sArray(desc, items)
	nullable(o)
	return o
}

// nullable lets a schema accept null too.
func nullable(o jsonObject) {
	switch t := o["type"].(type) {
	case string:
		o["type"] = []string{t, "null"}
	case []string:
		if !slices.Contains(t, "null") {
			o["type"] = append(slices.Clone(t), "null")
		}
	}
	if e, ok := o["enum"].([]string); ok {
		vals := make([]any, 0, len(e)+1)
		for _, v := range e {
			vals = append(vals, v)
		}
		o["enum"] = append(vals, nil) // the empty key is the same as the default
	}
}

func ruleIDs() []string {
	ids := make([]string, 0, len(ruleRegistry))
	for _, r := range ruleRegistry {
		ids = append(ids, r.ID)
	}
	return ids
}

func nullableStrings(desc string) jsonObject {
	return jsonObject{"type": []string{"array", "null"}, "description": desc, "items": jsonObject{"type": "string"}}
}

func sIgnore(ids []string) jsonObject {
	ruleID := jsonObject{"type": "string", "enum": ids}
	return jsonObject{
		"type":        "object",
		"description": "One accepted finding.",
		"properties": jsonObject{
			"rule": jsonObject{
				"description": "The rule ID, or a list of rule IDs, whose findings are accepted.",
				"oneOf":       []jsonObject{ruleID, {"type": "array", "items": ruleID, "minItems": 1}},
			},
			"file":         sStr("A glob matched against the path of the file, relative to the repository root (** crosses directories)."),
			"uses":         sStr("The uses value of the step or of the called workflow: a pattern like actions/checkout or actions/*, a glob for other values (docker://alpine*), or a regular expression between slashes (/^actions\\/(checkout|cache)@/)."),
			"job":          sStr("The ID of the job."),
			"step":         sStr("The id or the name of the step."),
			"reason":       sStr("Why the finding is accepted. It is for the readers of the file."),
			"allow-unused": sBool("Do not report the entry as unused-ignore when it suppresses nothing, for a finding which comes and goes."),
			"expires":      jsonObject{"type": "string", "description": "The last day (YYYY-MM-DD, UTC) the entry accepts findings. After it the findings come back and the expired-ignore rule reports the entry. Quote it (\"2027-06-30\") if your editor reads an unquoted date as a timestamp, which has no string type.", "pattern": `^\d{4}-\d{2}-\d{2}$`},
		},
		"required": []string{"rule"},
		// The parser rejects an entry which says nothing about where: it would turn the rule off everywhere, which is
		// what "rules" is for
		"anyOf":                []jsonObject{{"required": []string{"file"}}, {"required": []string{"uses"}}, {"required": []string{"job"}}, {"required": []string{"step"}}},
		"additionalProperties": false,
	}
}

func onlineOptionsSchema() jsonObject {
	return sObject("Tunes the online checks. Every key is optional. The command line flags win over them.", jsonObject{
		"mode":                  sEnum("cache never uses the network and answers from the disk cache, strict makes the exit status 3 when a lookup is skipped, cache,strict is both. A mode other than the default also turns the online checks on, unless online is false.", "cache", "strict", "cache,strict"),
		"api-url":               sStr("The REST API URL of a GitHub Enterprise Server, like https://ghe.example.com/api/v3. A token is not sent to a host named here by the config file of a repository."),
		"token-env":             sStr("The name of the environment variable which holds the token. It is read before GITHUB_TOKEN and GH_TOKEN."),
		"token-file":            sStr("A file which holds the token."),
		"allow":                 sStrings("Look up only the repositories matching one of these owner/repo patterns (* is a wildcard within a part). Empty allows all."),
		"deny":                  sStrings("Never look up the repositories matching one of these owner/repo patterns. It wins over allow."),
		"cache-ttl":             sStr("How long a cached answer is used without asking GitHub again, as a duration like 1h. 0s always revalidates. The default is 1h."),
		"max-rate-limit-wait":   sStr("The longest to wait for a rate limit to reset, as a duration like 30s. 0s never waits. The default is 30s."),
		"retries":               sIntMax(fmt.Sprintf("How often a request is repeated after a server error or a secondary rate limit (at most %d). The default is %d.", maxOnlineRetries, defaultOnlineRetries), maxOnlineRetries),
		"concurrency":           sIntMax(fmt.Sprintf("How many requests are in flight at most (at most %d; 0 is the default, %d).", maxOnlineInFlight, defaultOnlineInFlight), maxOnlineInFlight),
		"gh-cli":                sBool("Whether gh auth token is asked for a token when no variable or file has one. The default is true."),
		"github-com-fallback":   sBool("On a GitHub Enterprise Server, also ask api.github.com about the repositories which the Server answers 404 for (actions synced with GitHub Connect). The names of those repositories are sent to github.com: list internal ones in deny. The default is false."),
		"github-com-token-env":  sStr("With github-com-fallback: the name of the environment variable which holds the token for api.github.com. It is read before GITHUB_COM_TOKEN. The token of the Server is never sent to github.com."),
		"github-com-token-file": sStr("With github-com-fallback: a file which holds the token for api.github.com."),
	})
}

// ruleSchema returns the schema of the value of one rule in the "rules" mapping: a level, or a mapping with the
// level and the options of the rule.
func ruleSchema(r RuleInfo) jsonObject {
	desc := fmt.Sprintf("%s Group: %s. ", strings.TrimSpace(r.Summary), r.Group)
	switch {
	case r.Online:
		desc += "Runs only with --online. "
	case r.Profile == "":
		desc += "Runs only when configured. "
	default:
		desc += fmt.Sprintf("Profile: %s. ", r.Profile)
	}
	desc += fmt.Sprintf("Default level: %s. %s", r.DefaultLevel, r.DocURL())
	// Inside a mapping the level is a severity: true and false are the whole value of a rule only
	opts := jsonObject{"level": sDescribedRef("The level of the rule: error, warn, info or off.", "severity")}
	for _, o := range r.Options {
		opts[o.Name] = optionSchema(o)
	}
	return jsonObject{
		"description": desc,
		"oneOf": []jsonObject{
			sRef("level"),
			{"type": "object", "properties": opts, "additionalProperties": false},
		},
	}
}

func optionSchema(o RuleOption) jsonObject {
	var s jsonObject
	switch o.Kind {
	case RuleOptionInt:
		s = sInt(o.Summary)
		if o.Min > 0 {
			s["minimum"] = o.Min
		}
	case RuleOptionNumber:
		s = sType(o.Summary, "number")
		s["minimum"] = 0
	case RuleOptionBool:
		s = sBool(o.Summary)
	case RuleOptionStrings:
		s = sStrings(o.Summary)
	case RuleOptionStringMap:
		s = jsonObject{"type": "object", "description": o.Summary, "additionalProperties": jsonObject{"type": "string"}}
	default:
		s = jsonObject{"description": o.Summary}
	}
	if o.Default != nil {
		s["default"] = o.Default
	}
	return s
}

// OutputSchema returns the JSON Schema (draft-07) of the findings of --format json (an array of findings) and
// --format jsonl (one finding per line, which is validated by #/definitions/finding). It is the contract for
// the programs which read them: a field is never removed or renamed within a major version, and fields can be
// added, so a reader must ignore the fields it does not know.
func OutputSchema() []byte {
	str := func(d string) jsonObject { return sStr(d) }
	posInt := func(d string) jsonObject {
		o := sType(d, "integer")
		o["minimum"] = 1
		return o
	}
	finding := jsonObject{
		"type":        "object",
		"description": "One finding.",
		"properties": jsonObject{
			"message":    str("The message of the finding."),
			"filepath":   str("The path of the file, relative to the working directory when possible. It is <stdin> for the input of stdin without a name, or the value of --stdin-filename."),
			"line":       posInt("The line where the finding starts (1-based)."),
			"column":     posInt("The column where the finding starts (1-based, counted in Unicode code points)."),
			"end_line":   posInt("The line where the region of the finding ends. It is equal to line unless the region spans several lines."),
			"end_column": posInt("The column of the last character of the indicator in snippet, counted in Unicode code points like column, not in display width. It equals column when no indicator can be shown. It is not the exclusive end of the region."),
			"kind":       str("The legacy name of the checker which reported the finding. Several rules share a kind, and it is not always a rule ID. Legacy: use id."),
			"id":         str("The stable ID of the rule, like unpinned-uses. See https://jactionlint.jdx.dev/rules. A custom rule has its name as ID."),
			"severity":   sEnum("The level of the finding.", "error", "warn", "info"),
			"snippet":    str("The lines of the source with the indicator (^~~~) under the region. It is omitted when empty."),
			"doc_url":    jsonObject{"type": "string", "format": "uri", "description": "The documentation of the rule. It is omitted for custom rules."},
			"fix": jsonObject{
				"type":        "object",
				"description": "The automatic fix. It is omitted when the finding has none.",
				"properties": jsonObject{
					"description": str("What applying the fix does."),
					"unsafe":      sBool("The fix may change the behavior of the workflow, so --fix applies it only with --fix=unsafe. It is omitted when false."),
					"edits": sArray("The replacements, which do not overlap.", jsonObject{
						"type": "object",
						"properties": jsonObject{
							"start":    sInt("The byte offset where the replaced range starts."),
							"end":      sInt("The byte offset just after the replaced range."),
							"new_text": str("The text put in place of the range."),
						},
						"required": []string{"start", "end", "new_text"},
					}),
				},
				"required": []string{"description", "edits"},
			},
			"events":      sStrings("The sorted names of the events which trigger the workflow. Only with --finding-context, and for a workflow."),
			"permissions": jsonObject{"type": "object", "description": "The permissions of the token of the job the finding is in, scope to read, write, none or unknown (an expression decides it); the scope * is read-all, write-all, an empty mapping (none) or an expression (unknown). Only with --finding-context, and when the file configures them.", "additionalProperties": jsonObject{"type": "string"}},
			"no_fix": jsonObject{
				"type":        "object",
				"description": "Why a finding which the rule can fix has no fix. It is omitted when the finding has a fix or is advice only.",
				"properties": jsonObject{
					"code":   jsonObject{"type": "string", "description": "The machine-readable reason.", "enum": noFixCodes()},
					"reason": str("The explanation for a person."),
					"option": str("The rule option to configure, as rule-id.option, for option-required and option-invalid."),
				},
				"required": []string{"code", "reason"},
			},
		},
		"required": []string{"message", "line", "column", "end_line", "end_column", "kind", "id", "severity"},
	}
	return marshalSchema(jsonObject{
		"$schema":     schemaDraft,
		"$id":         OutputSchemaURL,
		"title":       "jactionlint findings",
		"description": "The output of `jactionlint --format json`: an array of findings. `--format jsonl` prints one finding per line, each valid against #/definitions/finding. Fields are never removed or renamed within a major version; new fields can appear, so readers must ignore the fields they do not know. Generated by `jactionlint --print-output-schema`.",
		"type":        "array",
		"items":       sRef("finding"),
		"definitions": jsonObject{"finding": finding},
	})
}

func noFixCodes() []string {
	return []string{
		string(NoFixOptionRequired), string(NoFixOptionInvalid), string(NoFixOnlineRequired), string(NoFixLookupFailed),
		string(NoFixUnsupportedShape), string(NoFixNeedsJudgment), string(NoFixUnsafeEdit), string(NoFixEditConflict), string(NoFixInvalidEdit),
	}
}

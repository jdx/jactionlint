package jactionlint

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

// RuleLevel returns the severity at which the rule with the given ID runs with this configuration:
// the level written in "rules", or else the default level of the rule when the profile includes the
// rule, or else SeverityOff. It can be called on a nil Config, which behaves like an empty one.
// Unknown IDs (IDs of custom rules) are always enabled as errors.
func (c *Config) RuleLevel(id string) Severity {
	info, ok := ruleIndex[id]
	if !ok {
		return SeverityError
	}
	if c != nil {
		if rc, ok := c.Rules[id]; ok {
			return rc.Level
		}
	}
	if c != nil && id == "required-actions" && len(c.RequiredActions) > 0 {
		return info.DefaultLevel
	}
	if info.Online {
		// Online rules do not follow the profile. They exist only when the online checks are on
		// (-online or "online: true"), and then run at their own level.
		return info.DefaultLevel
	}
	if info.Profile != "" && c.profile().Includes(info.Profile) {
		return info.DefaultLevel
	}
	return SeverityOff
}

// RuleEnabled reports whether the rule runs with this configuration. It can be called on a nil Config.
func (c *Config) RuleEnabled(id string) bool {
	return c.RuleLevel(id) != SeverityOff
}

// RuleRuns reports whether the rule runs in a run where the online checks are on or off: an online rule
// needs online mode as well as a level which is not off. Use it where a consumer asks whether a rule could
// have reported something (unused-ignore); RuleEnabled only looks at the level.
func (c *Config) RuleRuns(id string, online bool) bool {
	if info, ok := ruleIndex[id]; ok && info.Online && !online {
		return false
	}
	return c.RuleEnabled(id)
}

// implicitProfile is the profile of a configuration which sets none. It is ProfileDefault. It is a variable
// only so that the tests of the individual rules can run the way they were written, one rule at a time,
// without every sample workflow also being judged on pinning and permissions: the tests set it to
// ProfileCorrectness in TestMain, and the test of the default restores it.
var implicitProfile = ProfileDefault

func (c *Config) profile() Profile {
	if c == nil || c.Profile == "" {
		return implicitProfile
	}
	return c.Profile
}

// RuleOption returns the value of the option of the rule: the one written in "rules", or else the
// default of the option. The boolean is false when the option has neither. The value is an int or a
// float64 according to RuleOption.Kind. It can be called on a nil Config.
func (c *Config) RuleOption(id, name string) (any, bool) {
	if c != nil {
		if rc, ok := c.Rules[id]; ok {
			if v, ok := rc.Options[name]; ok {
				return v, true
			}
		}
	}
	if info, ok := ruleIndex[id]; ok {
		if o, ok := info.option(name); ok && o.Default != nil {
			return o.Default, true
		}
	}
	return nil, false
}

// RuleOptionStrings returns the value of a list option of the rule: the one written in "rules", or else
// the default of the option. The boolean is false when the option has neither. It can be called on
// a nil Config.
func (c *Config) RuleOptionStrings(id, name string) ([]string, bool) {
	v, ok := c.RuleOption(id, name)
	if !ok {
		return nil, false
	}
	l, ok := v.([]string)
	return l, ok
}

// ruleOptionNumber returns the value of a numeric option as float64.
func (c *Config) ruleOptionNumber(id, name string) (float64, bool) {
	v, ok := c.RuleOption(id, name)
	if !ok {
		return 0, false
	}
	switch v := v.(type) {
	case int:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

// ruleOptionStrings returns the value of an option which is a list of strings.
func (c *Config) ruleOptionStrings(id, name string) []string {
	v, ok := c.RuleOption(id, name)
	if !ok {
		return nil
	}
	ss, _ := v.([]string)
	return ss
}

// normalizeRules validates the "rules" mapping against the registry and fills in the default level of
// the rules which are configured with options only.
func (c *Config) normalizeRules() error {
	for id, rc := range c.Rules {
		info, ok := ruleIndex[id]
		if !ok {
			return fmt.Errorf("unknown rule ID %q in \"rules\"%s", id, suggestRuleID(id))
		}
		if !rc.levelSet {
			rc.Level = info.DefaultLevel
			rc.levelSet = true
		}
		for name, v := range rc.Options {
			opt, ok := info.option(name)
			if !ok {
				return fmt.Errorf("unknown option %q for rule %q in \"rules\"%s", name, id, suggestOption(name, info))
			}
			nv, err := normalizeOption(opt, v)
			if err == nil && opt.Validate != nil {
				err = opt.Validate(nv)
			}
			if err != nil {
				return fmt.Errorf("invalid value %v for option %q of rule %q in \"rules\": %w", v, name, id, err)
			}
			rc.Options[name] = nv
		}
		c.Rules[id] = rc
	}
	return nil
}

func normalizeOption(opt RuleOption, v any) (any, error) {
	switch opt.Kind {
	case RuleOptionInt:
		switch v := v.(type) {
		case int:
			if v >= 0 {
				return v, nil
			}
		case int64:
			if v >= 0 {
				return int(v), nil
			}
		case uint64:
			return int(v), nil
		}
		return nil, fmt.Errorf("it must be a non-negative integer")
	case RuleOptionNumber:
		var f float64
		switch v := v.(type) {
		case int:
			f = float64(v)
		case int64:
			f = float64(v)
		case uint64:
			f = float64(v)
		case float64:
			f = v
		default:
			return nil, fmt.Errorf("it must be a non-negative number")
		}
		if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			return nil, fmt.Errorf("it must be a non-negative number")
		}
		return f, nil
	case RuleOptionBool:
		if b, ok := v.(bool); ok {
			return b, nil
		}
		return nil, fmt.Errorf("it must be true or false")
	case RuleOptionStringMap:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("it must be a mapping")
		}
		ret := make(map[string]string, len(m))
		for k, e := range m {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("the value of %q must be a string", k)
			}
			ret[k] = s
		}
		return ret, nil
	case RuleOptionStrings:
		items, ok := v.([]any)
		if !ok {
			if ss, ok := v.([]string); ok {
				return slices.Clone(ss), nil
			}
			return nil, fmt.Errorf("it must be a list of strings")
		}
		ss := make([]string, 0, len(items))
		for _, it := range items {
			s, ok := it.(string)
			if !ok {
				return nil, fmt.Errorf("it must be a list of strings")
			}
			ss = append(ss, s)
		}
		return ss, nil
	}
	return nil, fmt.Errorf("unsupported option kind %q", opt.Kind)
}

// legacyConfig is the keys of the configuration file which were replaced by the "rules" mapping.
// They still work and are translated into rules.
type legacyConfig struct {
	TimeoutMinutes *struct {
		Required *bool   `yaml:"required"`
		Max      float64 `yaml:"max"`
	} `yaml:"timeout-minutes"`
	RequireCommitHash                *bool `yaml:"require-commit-hash"`
	RequirePermissions               *bool `yaml:"require-permissions"`
	RequireCheckoutBeforeLocalAction *bool `yaml:"require-checkout-before-local-action"`
	RequireExpressionWrapping        *bool `yaml:"require-expression-wrapping"`
	CheckFalsyTernary                *bool `yaml:"check-falsy-ternary"`
	CheckWorkflowRunNames            *bool `yaml:"check-workflow-run-names"`
	RequireShell                     *bool `yaml:"require-shell"`
	MaxRunLines                      *int  `yaml:"max-run-lines"`
}

// legacyBoolKeys maps the deprecated boolean keys to the rule they enable.
var legacyBoolKeys = []struct {
	key  string
	rule string
	get  func(*legacyConfig) *bool
}{
	{"require-commit-hash", "unpinned-uses", func(l *legacyConfig) *bool { return l.RequireCommitHash }},
	{"require-permissions", "missing-permissions", func(l *legacyConfig) *bool { return l.RequirePermissions }},
	{"require-checkout-before-local-action", "local-action-checkout", func(l *legacyConfig) *bool { return l.RequireCheckoutBeforeLocalAction }},
	{"require-expression-wrapping", "require-expression-wrapping", func(l *legacyConfig) *bool { return l.RequireExpressionWrapping }},
	{"check-falsy-ternary", "unsound-ternary", func(l *legacyConfig) *bool { return l.CheckFalsyTernary }},
	{"check-workflow-run-names", "workflow-run-names", func(l *legacyConfig) *bool { return l.CheckWorkflowRunNames }},
	{"require-shell", "require-shell", func(l *legacyConfig) *bool { return l.RequireShell }},
}

// legacyEntry is one rule which a deprecated key of the config file stands for.
type legacyEntry struct {
	// key is the deprecated key.
	key string
	// rule is the ID of the rule which replaces the key.
	rule string
	rc   RuleConfig
	// instead describes the replacement in the deprecation message. It is the same for all entries of a key.
	instead string
}

// entries translates the deprecated keys into the rules replacing them, in a fixed order.
func (l *legacyConfig) entries() ([]legacyEntry, error) {
	var ret []legacyEntry
	for _, k := range legacyBoolKeys {
		v := k.get(l)
		if v == nil {
			continue
		}
		level, name := SeverityOff, "off"
		if *v {
			level, name = SeverityError, "error"
		}
		ret = append(ret, legacyEntry{k.key, k.rule, RuleConfig{Level: level, levelSet: true}, fmt.Sprintf("\"rules: {%s: %s}\"", k.rule, name)})
	}

	if l.MaxRunLines != nil {
		n := *l.MaxRunLines
		switch {
		case n < 0:
			return nil, fmt.Errorf("\"max-run-lines\" must not be negative but got %d", n)
		case n > 0:
			ret = append(ret, legacyEntry{"max-run-lines", "max-run-lines", RuleConfig{Level: SeverityError, levelSet: true, Options: map[string]any{"max": n}}, fmt.Sprintf("\"rules: {max-run-lines: {level: error, max: %d}}\"", n)})
		default:
			ret = append(ret, legacyEntry{"max-run-lines", "max-run-lines", RuleConfig{Level: SeverityOff, levelSet: true}, "\"rules: {max-run-lines: off}\""})
		}
	}

	if t := l.TimeoutMinutes; t != nil {
		if math.IsNaN(t.Max) || math.IsInf(t.Max, 0) || t.Max < 0 {
			return nil, fmt.Errorf("\"max\" in \"timeout-minutes\" must be a non-negative number, but got %v", t.Max)
		}
		// "required" decides missing-timeout only when it is written. Leaving it out says nothing about the
		// rule, so the profile decides; "required: false" is the only way to turn it off.
		instead := "\"rules: {timeout-too-long: {level: error, max: ...}}\""
		if t.Required != nil {
			instead = "\"rules: {missing-timeout: error, timeout-too-long: {level: error, max: ...}}\""
			level := SeverityOff
			if *t.Required {
				level = SeverityError
			}
			ret = append(ret, legacyEntry{"timeout-minutes", "missing-timeout", RuleConfig{Level: level, levelSet: true}, instead})
		}
		if t.Max > 0 {
			ret = append(ret, legacyEntry{"timeout-minutes", "timeout-too-long", RuleConfig{Level: SeverityError, levelSet: true, Options: map[string]any{"max": t.Max}}, instead})
		}
	}
	return ret, nil
}

// applyLegacy translates the deprecated keys into entries of "rules" and records a deprecation
// message for each. A rule listed explicitly in "rules" wins over a deprecated key.
func (c *Config) applyLegacy(l *legacyConfig) error {
	es, err := l.entries()
	if err != nil {
		return err
	}
	prev := ""
	for _, e := range es {
		if _, ok := c.Rules[e.rule]; !ok {
			if c.Rules == nil {
				c.Rules = map[string]RuleConfig{}
			}
			c.Rules[e.rule] = e.rc
		}
		if e.key != prev {
			c.Deprecations = append(c.Deprecations, fmt.Sprintf("%q is deprecated and will be removed in a future version. use %s instead. run \"jactionlint -migrate-config\" to rewrite the file", e.key, e.instead))
			prev = e.key
		}
	}
	return nil
}

// --- strict parsing of the keys -------------------------------------------------------------

var (
	configTopKeys = []string{
		"profile", "extends", "rules", "online", "online-options", "baseline",
		"self-hosted-runner", "config-variables", "config-secrets", "paths", "ignores", "required-actions", "assume-default-permissions", "fix",
		// Deprecated keys which are translated into rules
		"timeout-minutes", "require-commit-hash", "require-permissions", "require-checkout-before-local-action",
		"require-expression-wrapping", "check-falsy-ternary", "check-workflow-run-names", "require-shell", "max-run-lines",
	}
	selfHostedRunnerKeys   = []string{"labels", "strict-labels"}
	fixConfigKeys          = []string{"rules"}
	onlineOptionsKeys      = []string{"mode", "api-url", "token-env", "token-file", "allow", "deny", "cache-ttl", "max-rate-limit-wait", "retries", "concurrency", "gh-cli"}
	pathConfigKeys         = []string{"ignore"}
	requiredActionKeys     = []string{"action", "version"}
	legacyTimeoutMinutesKy = []string{"required", "max"}
)

// mappingPairs returns the key and value nodes of a mapping node. Other nodes have no pairs.
func mappingPairs(n *yaml.Node) (keys, vals []*yaml.Node) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		keys = append(keys, n.Content[i])
		vals = append(vals, n.Content[i+1])
	}
	return keys, vals
}

func unknownKeyError(k *yaml.Node, where string, allowed []string) error {
	msg := fmt.Sprintf("unknown key %q in %s at line:%d,col:%d", k.Value, where, k.Line, k.Column)
	if s := didYouMean(k.Value, allowed); s != "" {
		return fmt.Errorf("%s. did you mean %q?", msg, s)
	}
	return fmt.Errorf("%s. available keys are %s", msg, quotes(sortedCopy(allowed)))
}

func sortedCopy(ss []string) []string {
	ret := slices.Clone(ss)
	sort.Strings(ret)
	return ret
}

func checkKeys(n *yaml.Node, where string, allowed []string) error {
	keys, _ := mappingPairs(n)
	for _, k := range keys {
		if !slices.Contains(allowed, k.Value) {
			return unknownKeyError(k, where, allowed)
		}
	}
	return nil
}

// validateConfigKeys reports unknown keys of the config file. Values of the wrong type are left to
// the decoder, which reports them.
func validateConfigKeys(root *yaml.Node) error {
	n := root
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	keys, vals := mappingPairs(n)
	for i, k := range keys {
		v := vals[i]
		if !slices.Contains(configTopKeys, k.Value) {
			return unknownKeyError(k, "the configuration", configTopKeys)
		}
		var err error
		switch k.Value {
		case "self-hosted-runner":
			err = checkKeys(v, "\"self-hosted-runner\"", selfHostedRunnerKeys)
		case "fix":
			if err = checkKeys(v, "\"fix\"", fixConfigKeys); err != nil {
				break
			}
			_, fvals := mappingPairs(v)
			for _, fv := range fvals {
				for _, id := range fv.Content {
					if _, ok := ruleIndex[id.Value]; !ok {
						return fmt.Errorf("unknown rule ID %q in \"fix.rules\" at line:%d,col:%d%s", id.Value, id.Line, id.Column, suggestRuleID(id.Value))
					}
				}
			}
		case "online-options":
			err = checkKeys(v, "\"online-options\"", onlineOptionsKeys)
		case "timeout-minutes":
			err = checkKeys(v, "\"timeout-minutes\"", legacyTimeoutMinutesKy)
		case "paths":
			_, pcs := mappingPairs(v)
			for _, pc := range pcs {
				if err = checkKeys(pc, "\"paths\"", pathConfigKeys); err != nil {
					break
				}
			}
		case "ignores":
			if v.Kind == yaml.SequenceNode {
				for _, item := range v.Content {
					if err = checkKeys(item, "\"ignores\"", configIgnoreKeys); err != nil {
						break
					}
				}
			}
		case "required-actions":
			if v.Kind == yaml.SequenceNode {
				for _, item := range v.Content {
					if err = checkKeys(item, "\"required-actions\"", requiredActionKeys); err != nil {
						break
					}
				}
			}
		case "rules":
			rkeys, rvals := mappingPairs(v)
			for j, rk := range rkeys {
				info, ok := ruleIndex[rk.Value]
				if !ok {
					return fmt.Errorf("unknown rule ID %q in \"rules\" at line:%d,col:%d%s", rk.Value, rk.Line, rk.Column, suggestRuleID(rk.Value))
				}
				allowed := []string{"level"}
				for _, o := range info.Options {
					allowed = append(allowed, o.Name)
				}
				if err = checkKeys(rvals[j], fmt.Sprintf("the options of rule %q", rk.Value), allowed); err != nil {
					break
				}
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// presentKeys returns the set of the keys written in the config file. Nested keys are joined with
// dots for the ones merging needs to know.
func presentKeys(root *yaml.Node) map[string]bool {
	ret := map[string]bool{}
	n := root
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	keys, vals := mappingPairs(n)
	for i, k := range keys {
		ret[k.Value] = true
		if k.Value == "fix" {
			nk, _ := mappingPairs(vals[i])
			for _, c := range nk {
				ret["fix."+c.Value] = true
			}
		}
		if k.Value == "self-hosted-runner" {
			nk, _ := mappingPairs(vals[i])
			for _, c := range nk {
				ret["self-hosted-runner."+c.Value] = true
			}
		}
	}
	return ret
}

func suggestRuleID(id string) string {
	if rr, ok := lookupRenamed(id); ok {
		if rr.Option != "" {
			return fmt.Sprintf(". the rule ID %q was merged into %q before 2.0: its findings are the ones of %q with the option %q, which the pedantic profile turns on. write \"rules: {%s: {%s: true}}\" to turn them on", id, rr.ID, rr.ID, rr.Option, rr.ID, rr.Option)
		}
		return fmt.Sprintf(". the rule ID %q was merged into %q before 2.0. use %q", id, rr.ID, rr.ID)
	}
	ids := make([]string, 0, len(ruleRegistry))
	for _, r := range ruleRegistry {
		ids = append(ids, r.ID)
	}
	if s := didYouMean(id, ids); s != "" {
		return fmt.Sprintf(". did you mean %q? see https://jactionlint.jdx.dev/rules for all rule IDs", s)
	}
	return ". see https://jactionlint.jdx.dev/rules for all rule IDs"
}

func suggestOption(name string, info *RuleInfo) string {
	var names []string
	for _, o := range info.Options {
		names = append(names, o.Name)
	}
	if len(names) == 0 {
		return ". the rule has no option"
	}
	if s := didYouMean(name, names); s != "" {
		return fmt.Sprintf(". did you mean %q?", s)
	}
	return ". available options are " + quotes(names)
}

// didYouMean returns the candidate which is the most similar to the word. It returns an empty
// string when no candidate is similar enough to be a plausible typo.
func didYouMean(word string, candidates []string) string {
	word = strings.ToLower(word)
	best, bestDist := "", 0
	for _, c := range candidates {
		d := editDistance(word, strings.ToLower(c))
		if len(word) >= 3 && strings.HasPrefix(strings.ToLower(c), word) {
			d = 1 // An abbreviation of the key
		}
		limit := 2
		if l := max(len(word), len(c)); l >= 10 {
			limit = l / 4 // allow more typos in long names
		}
		if d > limit {
			continue
		}
		if best == "" || d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// editDistance is the Damerau-Levenshtein distance (optimal string alignment) of two strings.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}

// ruleOptionBool returns the value of a boolean option.
func (c *Config) ruleOptionBool(id, name string) (bool, bool) {
	v, ok := c.RuleOption(id, name)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// ruleOptionStringMap returns the value of a string mapping option. The result must not be modified.
func (c *Config) ruleOptionStringMap(id, name string) (map[string]string, bool) {
	v, ok := c.RuleOption(id, name)
	if !ok {
		return nil, false
	}
	m, ok := v.(map[string]string)
	return m, ok
}

// ruleConfigured reports whether the configuration has an entry for the rule in "rules".
func (c *Config) ruleConfigured(id string) bool {
	if c == nil {
		return false
	}
	_, ok := c.Rules[id]
	return ok
}

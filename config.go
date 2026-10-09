package jactionlint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v4"
)

// IgnorePattern is one pattern of an "ignore" list. It is either a rule ID such as
// "unpinned-uses", which ignores every error with the ID, or a regular expression matched against
// the error messages.
type IgnorePattern struct {
	// ID is the rule ID to ignore. It is empty when the pattern is a regular expression.
	ID string
	// Regexp is the regular expression to match error messages. It is nil when the pattern is a rule ID.
	Regexp *regexp.Regexp
}

// ParseIgnorePattern parses a pattern of -ignore, "paths.*.ignore" and inline ignore comments. When the
// pattern is exactly the ID of a rule listed in Rules, it ignores the errors of that rule. Otherwise it
// is compiled as a regular expression which is matched against the error messages.
func ParseIgnorePattern(s string) (IgnorePattern, error) {
	if _, ok := LookupRule(s); ok {
		return IgnorePattern{ID: s}, nil
	}
	r, err := regexp.Compile(s)
	if err != nil {
		return IgnorePattern{}, err
	}
	return IgnorePattern{Regexp: r}, nil
}

// String returns the source of the pattern.
func (p IgnorePattern) String() string {
	if p.Regexp != nil {
		return p.Regexp.String()
	}
	return p.ID
}

// Match returns whether the pattern matches the error.
func (p IgnorePattern) Match(err *Error) bool {
	if p.Regexp != nil {
		return p.Regexp.MatchString(err.Message)
	}
	return p.ID != "" && p.ID == err.ID
}

// IgnorePatterns is a list of patterns. These patterns are used for filtering errors by matching the
// rule IDs or the error messages.
type IgnorePatterns []IgnorePattern

// Match returns whether the given error should be ignored due to the "ignore" configuration.
func (pats IgnorePatterns) Match(err *Error) bool {
	for _, p := range pats {
		if p.Match(err) {
			return true
		}
	}
	return false
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (pats *IgnorePatterns) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("yaml: \"ignore\" must be a sequence node at line:%d,col:%d", n.Line, n.Column)
	}
	rs := make([]IgnorePattern, 0, len(n.Content))
	for _, p := range n.Content {
		if p.Kind != yaml.ScalarNode || (p.Tag != "" && p.Tag != "!!str") {
			return fmt.Errorf("yaml: \"ignore\" items must be strings at line:%d,col:%d", p.Line, p.Column)
		}
		r, err := ParseIgnorePattern(p.Value)
		if err != nil {
			return fmt.Errorf("invalid regular expression %q in \"ignore\" at line%d,col:%d: %w", p.Value, n.Line, n.Column, err)
		}
		rs = append(rs, r)
	}
	*pats = rs
	return nil
}

// PathConfig is a configuration for specific file path pattern. This is for values of the "paths" mapping
// in the configuration file.
type PathConfig struct {
	// Ignore is a list of patterns. They are used for ignoring errors by matching to the error messages.
	// It is similar to the "-ignore" command line option.
	Ignore IgnorePatterns `yaml:"ignore"`
}

// RuleConfig is the configuration of one rule in the "rules" mapping of the configuration file. It
// is either a level (`unpinned-uses: warn`) or a mapping with the level and the options of the rule
// (`max-run-lines: {level: warn, max: 80}`). A mapping without "level" enables the rule at its
// default level.
type RuleConfig struct {
	// Level is the severity of the findings of the rule. SeverityOff disables the rule.
	Level Severity
	// Options are the options of the rule. The values are int or float64 depending on RuleOption.Kind.
	Options map[string]any

	levelSet bool
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (rc *RuleConfig) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		switch n.Value {
		case "true":
			*rc = RuleConfig{} // The default level is filled in later
			return nil
		case "false":
			*rc = RuleConfig{Level: SeverityOff, levelSet: true}
			return nil
		}
		lv, err := ParseSeverity(n.Value)
		if err != nil {
			return fmt.Errorf("%w at line:%d,col:%d", err, n.Line, n.Column)
		}
		*rc = RuleConfig{Level: lv, levelSet: true}
		return nil
	case yaml.MappingNode:
		out := RuleConfig{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Value == "level" {
				lv, err := ParseSeverity(v.Value)
				if v.Kind != yaml.ScalarNode || err != nil {
					return fmt.Errorf("\"level\" must be one of \"off\", \"info\", \"warn\" and \"error\" at line:%d,col:%d", v.Line, v.Column)
				}
				out.Level, out.levelSet = lv, true
				continue
			}
			var val any
			if err := v.Decode(&val); err != nil {
				return err
			}
			if out.Options == nil {
				out.Options = map[string]any{}
			}
			out.Options[k.Value] = val
		}
		*rc = out
		return nil
	}
	return fmt.Errorf("a rule must be configured with a level or a mapping at line:%d,col:%d", n.Line, n.Column)
}

// Config is configuration of jactionlint. This struct instance is parsed from "jactionlint.yaml"
// file usually put in ".github" directory.
type Config struct {
	// Profile selects the set of rules which are enabled by default: "default", "strict" or "all". The
	// empty value means ProfileDefault. See Rules for which rules each profile enables.
	Profile Profile `yaml:"profile"`
	// Extends is a list of config files to inherit from. Relative paths are resolved from the directory
	// of the config file listing them. Later files win over earlier ones, and the config file itself
	// wins over all of them. ReadConfigFile loads them and merges them into the returned Config.
	Extends []string `yaml:"extends"`
	// Online turns on the online checks (see LinterOptions.Online) for the files this configuration
	// applies to, like the -online flag does for the whole run. They query the GitHub API.
	Online bool `yaml:"online"`
	// Rules sets the level and the options of each rule by rule ID. A rule not listed here follows the
	// profile.
	Rules map[string]RuleConfig `yaml:"rules"`
	// SelfHostedRunner is configuration for self-hosted runner.
	SelfHostedRunner struct {
		// Labels is label names for self-hosted runner.
		Labels []string `yaml:"labels"`
		// StrictLabels makes the runner-label rule accept only the labels listed in Labels. When true, the
		// built-in labels (GitHub-hosted runner labels and the preset self-hosted labels such as "self-hosted"
		// and "linux") are reported as unknown unless they are explicitly listed in Labels.
		StrictLabels bool `yaml:"strict-labels"`
	} `yaml:"self-hosted-runner"`
	// ConfigVariables is names of configuration variables used in the checked workflows. When this value is nil,
	// property names of `vars` context will not be checked. Otherwise jactionlint will report a name which is not
	// listed here as undefined config variables.
	// https://docs.github.com/en/actions/learn-github-actions/variables
	ConfigVariables []string `yaml:"config-variables"`
	// ConfigSecrets is names of secrets used in the checked workflows. When this value is nil,
	// property names of `secrets` context will not be checked. Otherwise jactionlint will report a name which is not
	// listed here as undefined secrets.
	// https://docs.github.com/en/actions/security-guides/using-secrets-in-github-actions
	ConfigSecrets []string `yaml:"config-secrets"`
	// Paths is a "paths" mapping in the configuration file. The keys are glob patterns to match file paths.
	// And the values are corresponding configurations applied to the file paths.
	Paths map[string]PathConfig `yaml:"paths"`

	// RequiredActions is a "required-actions" list in the configuration file. Each item is an action (or
	// reusable workflow) which must be used at least once in every checked workflow. The check is disabled
	// when the list is empty.
	RequiredActions []RequiredActionRule `yaml:"required-actions"`
	// AssumeDefaultPermissions controls how the workflow-call permission check treats a caller that
	// has no `permissions:` block at the workflow level and none on the calling job. "restricted"
	// (default) assumes GitHub's restricted default token (contents/packages: read, everything else:
	// none). "permissive" assumes write on every scope except `id-token`, which always requires an
	// explicit opt-in regardless of the repo-level Workflow permissions setting. Only affects callers
	// with no permissions block anywhere; once any permissions block is declared, the check always
	// runs against it.
	AssumeDefaultPermissions *string `yaml:"assume-default-permissions"`

	// Path is the path of the file which the config was read from. It is empty when the config was
	// parsed from bytes.
	Path string `yaml:"-"`
	// Deprecations are messages about deprecated keys the config file uses, for example the old
	// "require-shell: true" which is replaced by the "rules" mapping. The config still works.
	// "jactionlint -migrate-config" rewrites the file.
	Deprecations []string `yaml:"-"`

	// present records which keys were written explicitly so that merging with the files listed in
	// "extends" can tell a missing key from a zero value.
	present map[string]bool
}

// AssumeDefaultPermissionsRestricted is the config value enabling the restricted-default assumption.
const AssumeDefaultPermissionsRestricted = "restricted"

// AssumeDefaultPermissionsPermissive is the config value enabling the permissive-default assumption.
const AssumeDefaultPermissionsPermissive = "permissive"

// PathConfigs returns a list of all PathConfig values matching to the given file path. The path must
// be relative to the root of the project.
func (c *Config) PathConfigs(path string) []PathConfig {
	path = filepath.ToSlash(path)

	var ret []PathConfig
	if c != nil {
		for p, pc := range c.Paths {
			// Glob patterns were validated in `ParseConfig()`
			if doublestar.MatchUnvalidated(p, path) {
				ret = append(ret, pc)
			}
		}
	}
	return ret
}

// ParseConfig parses the given bytes as an jactionlint config file. When deserializing the YAML file
// or the config validation fails, this function returns an error. Unknown keys are errors too, with
// a suggestion when the key looks like a typo of a known one. The "extends" key needs the path of the
// file to find the files to inherit from so it is an error here; use ReadConfigFile.
func ParseConfig(b []byte) (*Config, error) {
	c, err := parseConfig(b)
	if err != nil {
		return nil, err
	}
	if len(c.Extends) > 0 {
		return nil, errors.New("\"extends\" can be used only in a config file. use ReadConfigFile to read it")
	}
	return c, nil
}

// parseConfig parses one config file without resolving "extends".
func parseConfig(b []byte) (*Config, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		msg := strings.ReplaceAll(err.Error(), "\n", " ")
		return nil, errors.New(msg)
	}
	if err := validateConfigKeys(&root); err != nil {
		return nil, err
	}

	var c Config
	var legacy legacyConfig
	if root.Kind != 0 {
		if err := root.Decode(&c); err != nil {
			msg := strings.ReplaceAll(err.Error(), "\n", " ")
			return nil, errors.New(msg)
		}
		if err := root.Decode(&legacy); err != nil {
			msg := strings.ReplaceAll(err.Error(), "\n", " ")
			return nil, errors.New(msg)
		}
	}
	c.present = presentKeys(&root)

	if c.Profile != "" {
		if _, err := ParseProfile(string(c.Profile)); err != nil {
			return nil, fmt.Errorf("%w in \"profile\"", err)
		}
	}
	for pat := range c.Paths {
		if !doublestar.ValidatePattern(pat) {
			return nil, fmt.Errorf("invalid glob pattern %q in \"paths\"", pat)
		}
	}
	for i, r := range c.RequiredActions {
		if r.Action == "" {
			return nil, fmt.Errorf("\"action\" is required in \"required-actions\" item at index %d", i)
		}
		if strings.Contains(r.Action, "@") || strings.HasPrefix(r.Action, "./") || strings.HasPrefix(r.Action, selfRepositoryUsesPrefix) || strings.HasPrefix(r.Action, "docker://") || !strings.Contains(r.Action, "/") {
			return nil, fmt.Errorf("invalid action %q in \"required-actions\": it must be like \"owner/repo\" without \"@version\"; put the version in \"version\"", r.Action)
		}
	}
	if c.AssumeDefaultPermissions != nil {
		switch *c.AssumeDefaultPermissions {
		case AssumeDefaultPermissionsRestricted, AssumeDefaultPermissionsPermissive:
		default:
			return nil, fmt.Errorf("invalid value %q for \"assume-default-permissions\". available values are %q and %q", *c.AssumeDefaultPermissions, AssumeDefaultPermissionsRestricted, AssumeDefaultPermissionsPermissive)
		}
	}
	if err := c.normalizeRules(); err != nil {
		return nil, err
	}
	if err := c.applyLegacy(&legacy); err != nil {
		return nil, err
	}
	return &c, nil
}

// ReadConfigFile reads jactionlint config file (jactionlint.yaml) from the given file path. The
// files listed in "extends" are read too and merged into the returned config.
func ReadConfigFile(path string) (*Config, error) {
	return readConfigFile(path, nil)
}

// maxExtendsDepth limits how deep "extends" chains can be.
const maxExtendsDepth = 10

func readConfigFile(path string, stack []string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config file %q: %w", path, err)
	}
	c, err := parseConfig(b)
	if err != nil {
		return nil, fmt.Errorf("could not parse config file %q: %w", path, err)
	}
	c.Path = path
	for i, d := range c.Deprecations {
		c.Deprecations[i] = fmt.Sprintf("config file %q: %s", path, d)
	}

	if len(c.Extends) == 0 {
		return c, nil
	}

	abs := absPath(path)
	if slices.Contains(stack, abs) {
		return nil, fmt.Errorf("could not parse config file %q: \"extends\" makes a cycle: %s", path, strings.Join(append(slices.Clone(stack), abs), " -> "))
	}
	if len(stack) >= maxExtendsDepth {
		return nil, fmt.Errorf("could not parse config file %q: \"extends\" is nested more than %d levels", path, maxExtendsDepth)
	}
	stack = append(slices.Clone(stack), abs)

	merged := &Config{}
	for _, e := range c.Extends {
		p := e
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(path), p)
		}
		base, err := readConfigFile(p, stack)
		if err != nil {
			return nil, fmt.Errorf("could not load %q listed in \"extends\" of config file %q: %w", e, path, err)
		}
		merged.merge(base)
	}
	merged.merge(c)
	merged.Path = path
	merged.Extends = c.Extends
	return merged, nil
}

// merge applies the explicitly written settings of over on top of c. Mappings (rules, paths) are
// merged by key and everything else is replaced.
func (c *Config) merge(over *Config) {
	if c.present == nil {
		c.present = map[string]bool{}
	}
	if over.present["profile"] {
		c.Profile = over.Profile
	}
	if len(over.Rules) > 0 && c.Rules == nil {
		c.Rules = map[string]RuleConfig{}
	}
	for id, rc := range over.Rules {
		c.Rules[id] = rc
	}
	if over.present["online"] {
		c.Online = over.Online
	}
	if over.present["self-hosted-runner.labels"] {
		c.SelfHostedRunner.Labels = over.SelfHostedRunner.Labels
	}
	if over.present["self-hosted-runner.strict-labels"] {
		c.SelfHostedRunner.StrictLabels = over.SelfHostedRunner.StrictLabels
	}
	if over.present["config-variables"] {
		c.ConfigVariables = over.ConfigVariables
	}
	if over.present["config-secrets"] {
		c.ConfigSecrets = over.ConfigSecrets
	}
	if len(over.Paths) > 0 && c.Paths == nil {
		c.Paths = map[string]PathConfig{}
	}
	for p, pc := range over.Paths {
		c.Paths[p] = pc
	}
	if over.present["required-actions"] {
		c.RequiredActions = over.RequiredActions
	}
	if over.present["assume-default-permissions"] {
		c.AssumeDefaultPermissions = over.AssumeDefaultPermissions
	}
	c.Deprecations = append(c.Deprecations, over.Deprecations...)
	for k := range over.present {
		c.present[k] = true
	}
}

// configFileNames are the names of config files in order of precedence. The "actionlint" names
// are the ones used by the original actionlint and are still accepted.
var configFileNames = []string{"jactionlint.yaml", "jactionlint.yml", "actionlint.yaml", "actionlint.yml"}

// globalConfigDirs are the directory names under $XDG_CONFIG_HOME in order of precedence together with
// the config file names looked up in each of them.
var globalConfigDirs = []struct {
	dir   string
	files []string
}{
	{"jactionlint", []string{"jactionlint.yaml", "jactionlint.yml"}},
	{"actionlint", []string{"actionlint.yaml", "actionlint.yml"}},
}

// loadRepoConfig reads config file from the repository's .github/jactionlint.yaml or
// .github/jactionlint.yml. The names used by actionlint (.github/actionlint.yaml and
// .github/actionlint.yml) are also accepted.
func loadRepoConfig(root string) (*Config, error) {
	for _, f := range configFileNames {
		p := filepath.Join(root, ".github", f)
		c, err := ReadConfigFile(p)
		switch {
		case errors.Is(err, os.ErrNotExist):
			continue
		case err != nil:
			return nil, fmt.Errorf("could not parse config file %q: %w", p, err)
		default:
			return c, nil
		}
	}
	return nil, nil
}

// loadGlobalConfig reads the user-global config file from
// $XDG_CONFIG_HOME/jactionlint/jactionlint.yaml (or jactionlint.yml), falling back
// to $HOME/.config/jactionlint/ when $XDG_CONFIG_HOME is unset. The location used by actionlint
// ($XDG_CONFIG_HOME/actionlint/actionlint.yaml) is also accepted. It returns the loaded config and
// its file path, or (nil, "", nil) when no config file exists.
func loadGlobalConfig() (*Config, string, error) {
	// The XDG Base Directory spec says relative paths in $XDG_CONFIG_HOME are invalid and must be
	// ignored. Otherwise a config file in an arbitrary working directory could be picked up.
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" || !filepath.IsAbs(dir) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", nil
		}
		dir = filepath.Join(home, ".config")
	}
	for _, d := range globalConfigDirs {
		for _, f := range d.files {
			p := filepath.Join(dir, d.dir, f)
			c, err := ReadConfigFile(p)
			switch {
			case errors.Is(err, os.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
				continue
			case err != nil:
				return nil, "", fmt.Errorf("could not parse global config file %q: %w", p, err)
			default:
				return c, p, nil
			}
		}
	}
	return nil, "", nil
}

func writeDefaultConfigFile(path string) error {
	b := []byte(`# Rules are enabled by profile. "default" has the correctness checks and the
# checks with (almost) no false positives. "strict" adds the security posture and
# policy checks such as pinning actions to a commit SHA. "all" adds the style
# checks. See https://jactionlint.jdx.dev/rules for all rule IDs.
#profile: default

# Turn on the checks which query the GitHub API (impostor commits, known
# vulnerable actions, archived repositories, ...). Same as the -online flag.
# See https://jactionlint.jdx.dev/usage#online-checks
#online: false

# Set the level of a rule by its ID: "error" (fails the run), "warn", "info" or
# "off". A rule with options takes a mapping.
rules:
#  unpinned-uses: error
#  require-shell: off
#  max-run-lines:
#    level: warn
#    max: 80

# Config files to inherit from. Relative paths are resolved from this file. Later
# files win and this file wins over all of them.
#extends:
#  - ../shared/jactionlint.yaml

self-hosted-runner:
  # Labels of self-hosted runner in array of strings.
  labels: []

# Configuration variables in array of strings defined in your repository or
# organization. ` + "`null`" + ` means disabling configuration variables check.
# Empty array means no configuration variable is allowed.
config-variables: null

# Secrets in array of strings defined in your repository or organization.
# ` + "`null`" + ` means disabling secrets check. Empty array means no secret is allowed.
config-secrets: null

# Configuration for file paths. The keys are glob patterns to match to file
# paths relative to the repository root. The values are the configurations for
# the file paths. Note that the path separator is always '/'.
# The following configurations are available.
#
# "ignore" is an array of rule IDs or regular expression patterns. Errors of the
# rules and errors with matched messages are ignored. This is similar to the
# "-ignore" command line option.
paths:
#  .github/workflows/**/*.yml:
#    ignore: [unpinned-uses, 'some message']

# Controls what permissions are assumed for a caller workflow that declares no
# "permissions:" block at all when checking reusable workflow calls. Set to
# "restricted" (the default) to assume GitHub's restricted default token. Set to
# "permissive" to assume write on every scope except "id-token" (which always
# requires an explicit opt-in).
#assume-default-permissions: restricted
`)
	if err := os.WriteFile(path, b, 0644); err != nil {
		return fmt.Errorf("could not write default configuration file at %q: %w", path, err)
	}
	return nil
}

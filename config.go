package actionlint

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v4"
)

// IgnorePatterns is a list of regular expressions. These patterns are used for filtering errors by
// matching the error messages.
type IgnorePatterns []*regexp.Regexp

// Match returns whether the given error should be ignored due to the "ignore" configuration.
func (pats IgnorePatterns) Match(err *Error) bool {
	for _, r := range pats {
		if r.MatchString(err.Message) {
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
	rs := make([]*regexp.Regexp, 0, len(n.Content))
	for _, p := range n.Content {
		if p.Kind != yaml.ScalarNode || (p.Tag != "" && p.Tag != "!!str") {
			return fmt.Errorf("yaml: \"ignore\" items must be strings at line:%d,col:%d", p.Line, p.Column)
		}
		r, err := regexp.Compile(p.Value)
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

// TimeoutMinutesConfig is a configuration for the "timeout-check" rule. The rule is opt-in; it does nothing
// unless "required" is true or "max" is set. This is for the "timeout-minutes" mapping in the configuration file.
type TimeoutMinutesConfig struct {
	// Required is whether every job (except for jobs calling a reusable workflow, which do not support
	// "timeout-minutes") must set "timeout-minutes".
	Required bool `yaml:"required"`
	// Max is the maximum allowed value of "timeout-minutes" of a job. Zero means no upper limit.
	Max float64 `yaml:"max"`
}

// Config is configuration of actionlint. This struct instance is parsed from "actionlint.yaml"
// file usually put in ".github" directory.
type Config struct {
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
	// property names of `vars` context will not be checked. Otherwise actionlint will report a name which is not
	// listed here as undefined config variables.
	// https://docs.github.com/en/actions/learn-github-actions/variables
	ConfigVariables []string `yaml:"config-variables"`
	// ConfigSecrets is names of secrets used in the checked workflows. When this value is nil,
	// property names of `secrets` context will not be checked. Otherwise actionlint will report a name which is not
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
	// TimeoutMinutes is a configuration for the "timeout-check" rule, which checks "timeout-minutes" of jobs.
	// The rule is disabled by default.
	TimeoutMinutes TimeoutMinutesConfig `yaml:"timeout-minutes"`
	// Requires action and docker versions to use a commit hash instead of version/branch.
	RequireCommitHash bool `yaml:"require-commit-hash"`
	// RequirePermissions reports jobs which are not covered by an explicit "permissions:" at workflow-level
	// or job-level. This is opt-in and disabled by default.
	RequirePermissions bool `yaml:"require-permissions"`
	// RequireCheckoutBeforeLocalAction reports a local action (`uses: ./path`) which is used in a job before any
	// step that checks out the repository.
	RequireCheckoutBeforeLocalAction bool `yaml:"require-checkout-before-local-action"`
	// RequireExpressionWrapping requires `if:` conditions to be wrapped in `${{ }}` explicitly.
	RequireExpressionWrapping bool `yaml:"require-expression-wrapping"`
	// CheckFalsyTernary reports `cond && falsy-literal || other` where the value after `&&` is a
	// literal which is always falsy so the whole expression always evaluates to the value after `||`.
	CheckFalsyTernary bool `yaml:"check-falsy-ternary"`
	// CheckWorkflowRunNames enables the opt-in "workflow-run" rule, which reports workflow names at
	// 'on.workflow_run.workflows' not found in the repository.
	CheckWorkflowRunNames bool `yaml:"check-workflow-run-names"`
	// RequireShell requires every "run:" step to have an explicit shell, set by "shell:" of the step or by
	// "defaults.run.shell" of the job or the workflow.
	RequireShell bool `yaml:"require-shell"`
	// MaxRunLines is the maximum number of non-blank lines allowed in a "run:" script. Zero (the default)
	// disables the check.
	MaxRunLines int `yaml:"max-run-lines"`
}

// AssumeDefaultPermissionsRestricted is the config value enabling the restricted-default assumption.
const AssumeDefaultPermissionsRestricted = "restricted"

// AssumeDefaultPermissionsPermissive is the config value enabling the permissive-default assumption.
const AssumeDefaultPermissionsPermissive = "permissive"

// PathConfigs returns a list of all PathConfig values matching to the given file path. The path must
// be relative to the root of the project.
func (cfg *Config) PathConfigs(path string) []PathConfig {
	path = filepath.ToSlash(path)

	var ret []PathConfig
	if cfg != nil {
		for p, c := range cfg.Paths {
			// Glob patterns were validated in `ParseConfig()`
			if doublestar.MatchUnvalidated(p, path) {
				ret = append(ret, c)
			}
		}
	}
	return ret
}

// ParseConfig parses the given bytes as an actionlint config file. When deserializing the YAML file
// or the config validation fails, this function returns an error.
func ParseConfig(b []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		msg := strings.ReplaceAll(err.Error(), "\n", " ")
		return nil, errors.New(msg)
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
	if c.MaxRunLines < 0 {
		return nil, fmt.Errorf("\"max-run-lines\" must not be negative but got %d", c.MaxRunLines)
	}
	if m := c.TimeoutMinutes.Max; math.IsNaN(m) || math.IsInf(m, 0) || m < 0 {
		return nil, fmt.Errorf("\"max\" in \"timeout-minutes\" must be a non-negative number, but got %v", m)
	}
	return &c, nil
}

// ReadConfigFile reads actionlint config file (actionlint.yaml) from the given file path.
func ReadConfigFile(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config file %q: %w", path, err)
	}
	c, err := ParseConfig(b)
	if err != nil {
		return nil, fmt.Errorf("could not parse config file %q: %w", path, err)
	}
	return c, nil
}

// loadRepoConfig reads config file from the repository's .github/actionlint.yml or
// .github/actionlint.yaml.
func loadRepoConfig(root string) (*Config, error) {
	for _, f := range []string{"actionlint.yaml", "actionlint.yml"} {
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
// $XDG_CONFIG_HOME/actionlint/actionlint.yaml (or actionlint.yml), falling back
// to $HOME/.config/actionlint/ when $XDG_CONFIG_HOME is unset. It returns the
// loaded config and its file path, or (nil, "", nil) when no config file exists.
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
	for _, f := range []string{"actionlint.yaml", "actionlint.yml"} {
		p := filepath.Join(dir, "actionlint", f)
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
	return nil, "", nil
}

func writeDefaultConfigFile(path string) error {
	b := []byte(`self-hosted-runner:
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
# "ignore" is an array of regular expression patterns. Matched error messages
# are ignored. This is similar to the "-ignore" command line option.
paths:
#  .github/workflows/**/*.yml:
#    ignore: []

# Controls what permissions are assumed for a caller workflow that declares no
# "permissions:" block at all when checking reusable workflow calls. Set to
# "restricted" (the default) to assume GitHub's restricted default token. Set to
# "permissive" to assume write on every scope except "id-token" (which always
# requires an explicit opt-in).
#assume-default-permissions: restricted
# Configuration for the "timeout-check" rule, which is disabled by default.
# "required" set to true requires every job to set "timeout-minutes".
# "max" is the maximum allowed value of "timeout-minutes" in minutes (0 means no limit).
#timeout-minutes:
#  required: false
#  max: 60
`)
	if err := os.WriteFile(path, b, 0644); err != nil {
		return fmt.Errorf("could not write default configuration file at %q: %w", path, err)
	}
	return nil
}

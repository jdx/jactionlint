package jactionlint

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

// UpdateConfig is the "update" mapping of the config file. It sets the defaults of "jactionlint update"; the
// options of the command override it.
type UpdateConfig struct {
	// CooldownDays is how old a release must be to be chosen. Nil and 0 turn the restriction off.
	CooldownDays *int `yaml:"cooldown-days"`
	// Freeze pins the references to the commit SHA of the chosen release, with the version as a comment.
	Freeze *bool `yaml:"freeze"`
	// IncludeTags are the glob patterns a tag must match (any of them) to be chosen. Empty means every tag.
	IncludeTags TagPatterns `yaml:"include-tags"`
	// ExcludeTags are the glob patterns of the tags which are never chosen. They win over IncludeTags.
	ExcludeTags TagPatterns `yaml:"exclude-tags"`
	// ExcludePrereleases leaves out the releases which GitHub marks as prereleases.
	ExcludePrereleases *bool `yaml:"exclude-prereleases"`
	// RequireImmutableReleases accepts only published releases which GitHub marks immutable.
	RequireImmutableReleases *bool `yaml:"require-immutable-releases"`
}

// TagPatterns is a list of glob patterns, written in the config file as one string or a list of strings.
type TagPatterns []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (p *TagPatterns) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*p = TagPatterns{n.Value}
		return nil
	case yaml.SequenceNode:
		out := make(TagPatterns, 0, len(n.Content))
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("yaml: the tag patterns must be strings at line %d, column %d", c.Line, c.Column)
			}
			out = append(out, c.Value)
		}
		*p = out
		return nil
	}
	return fmt.Errorf("yaml: the tag patterns must be a string or a list of strings at line %d, column %d", n.Line, n.Column)
}

// merge copies the key of over into c. It is how a config file overrides the files it extends.
func (c *UpdateConfig) merge(over *UpdateConfig, key string) {
	switch key {
	case "cooldown-days":
		c.CooldownDays = over.CooldownDays
	case "freeze":
		c.Freeze = over.Freeze
	case "include-tags":
		c.IncludeTags = over.IncludeTags
	case "exclude-tags":
		c.ExcludeTags = over.ExcludeTags
	case "exclude-prereleases":
		c.ExcludePrereleases = over.ExcludePrereleases
	case "require-immutable-releases":
		c.RequireImmutableReleases = over.RequireImmutableReleases
	}
}

func (c UpdateConfig) validate() error {
	if c.CooldownDays != nil && *c.CooldownDays < 0 {
		return fmt.Errorf("\"update.cooldown-days\" must not be negative, got %d", *c.CooldownDays)
	}
	for _, list := range []struct {
		key string
		p   TagPatterns
	}{{"include-tags", c.IncludeTags}, {"exclude-tags", c.ExcludeTags}} {
		for _, p := range list.p {
			if _, err := compileTagPattern(p); err != nil {
				return fmt.Errorf("invalid pattern %q in \"update.%s\": %w", p, list.key, err)
			}
		}
	}
	return nil
}

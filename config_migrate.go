package jactionlint

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

// legacyConfigKeys are the deprecated keys of the config file which MigrateConfig rewrites.
var legacyConfigKeys = []string{
	"timeout-minutes", "require-commit-hash", "require-permissions", "require-checkout-before-local-action",
	"require-expression-wrapping", "check-falsy-ternary", "check-workflow-run-names", "require-shell", "max-run-lines",
}

// MigrateConfig rewrites the deprecated keys of the content of a config file ("require-shell: true",
// "timeout-minutes: {...}" and so on) into the "rules" mapping. Rules which are already listed in
// "rules" are not changed. Comments and the other keys are kept. It returns the rewritten content and
// the deprecated keys which were migrated. When the config has no deprecated key, the returned
// content is the input and the list is empty.
func MigrateConfig(src []byte) ([]byte, []string, error) {
	if _, err := parseConfig(src); err != nil {
		return nil, nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return nil, nil, errors.New(strings.ReplaceAll(err.Error(), "\n", " "))
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return src, nil, nil
	}
	top := root.Content[0]

	var legacy legacyConfig
	if err := root.Decode(&legacy); err != nil {
		return nil, nil, errors.New(strings.ReplaceAll(err.Error(), "\n", " "))
	}
	entries, err := legacy.entries()
	if err != nil {
		return nil, nil, err
	}
	if len(entries) == 0 {
		return src, nil, nil
	}

	// Remove the deprecated keys and remember where the first one was and what the comments said
	var migrated []string
	insertAt := -1
	var comment string
	content := make([]*yaml.Node, 0, len(top.Content))
	for i := 0; i+1 < len(top.Content); i += 2 {
		k, v := top.Content[i], top.Content[i+1]
		if slices.Contains(legacyConfigKeys, k.Value) {
			if insertAt < 0 {
				insertAt = len(content)
			}
			if k.HeadComment != "" {
				if comment != "" {
					comment += "\n"
				}
				comment += k.HeadComment
			}
			migrated = append(migrated, k.Value)
			continue
		}
		content = append(content, k, v)
	}

	// Find or create the "rules" mapping
	var rules *yaml.Node
	for i := 0; i+1 < len(content); i += 2 {
		if content[i].Value == "rules" {
			rules = content[i+1]
			if rules.Kind != yaml.MappingNode {
				rules.Kind, rules.Tag, rules.Value, rules.Content, rules.Style = yaml.MappingNode, "!!map", "", nil, 0
			}
		}
	}
	created := rules == nil
	if created {
		rules = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	existing := map[string]bool{}
	for i := 0; i+1 < len(rules.Content); i += 2 {
		existing[rules.Content[i].Value] = true
	}
	for _, e := range entries {
		if existing[e.rule] {
			continue
		}
		existing[e.rule] = true
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: e.rule}
		rules.Content = append(rules.Content, key, ruleConfigNode(e.rc))
	}
	if created {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "rules", HeadComment: comment}
		at := min(max(insertAt, 0), len(content))
		content = slices.Insert(content, at, keyNode, rules)
	} else if comment != "" && len(rules.Content) > 0 {
		rules.Content[0].HeadComment = comment
	}
	top.Content = content

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, nil, fmt.Errorf("could not encode the migrated config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, nil, fmt.Errorf("could not encode the migrated config: %w", err)
	}

	// The result must still be valid and mean the same
	if _, err := parseConfig(buf.Bytes()); err != nil {
		return nil, nil, fmt.Errorf("the migrated config is invalid: %w", err)
	}
	return buf.Bytes(), migrated, nil
}

// ruleConfigNode builds the YAML for a rule: a level, or a mapping when the rule has options.
func ruleConfigNode(rc RuleConfig) *yaml.Node {
	scalar := func(v string) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
	}
	if len(rc.Options) == 0 {
		return scalar(rc.Level.String())
	}
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
	m.Content = append(m.Content, scalar("level"), scalar(rc.Level.String()))
	names := make([]string, 0, len(rc.Options))
	for n := range rc.Options {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		var val, tag string
		switch v := rc.Options[n].(type) {
		case int:
			val, tag = strconv.Itoa(v), "!!int"
		case float64:
			val, tag = strconv.FormatFloat(v, 'f', -1, 64), "!!float"
			if v == float64(int64(v)) {
				tag = "!!int"
			}
		default:
			val, tag = fmt.Sprint(v), "!!str"
		}
		m.Content = append(m.Content, scalar(n), &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: val})
	}
	return m
}

// MigrateConfigFile rewrites the deprecated keys of the config file at the path in place. See
// MigrateConfig. The file is not touched when it has no deprecated key.
func MigrateConfigFile(path string) ([]string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config file %q: %w", path, err)
	}
	out, migrated, err := MigrateConfig(src)
	if err != nil {
		return nil, fmt.Errorf("could not migrate config file %q: %w", path, err)
	}
	if len(migrated) == 0 {
		return nil, nil
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return nil, fmt.Errorf("could not write config file %q: %w", path, err)
	}
	return migrated, nil
}

// MigrateConfig rewrites the deprecated keys of the config file into the "rules" mapping and prints
// what changed. The file is ConfigFile of the options when given, otherwise the config file of the
// project which the directory belongs to. When the directory is empty, the current directory is used.
func (l *Linter) MigrateConfig(dir string) error {
	path := l.configFile
	if path == "" {
		if dir == "" {
			dir = l.cwd
		}
		proj, err := l.projects.At(dir)
		if err != nil {
			return err
		}
		if proj == nil {
			return errors.New("project is not found. check current project is initialized as Git repository and \".github/workflows\" directory exists")
		}
		for _, f := range configFileNames {
			p := filepath.Join(proj.RootDir(), ".github", f)
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
		if path == "" {
			return fmt.Errorf("no config file was found in %q", filepath.Join(proj.RootDir(), ".github"))
		}
	}

	l.log("Migrating config file:", path)
	migrated, err := MigrateConfigFile(path)
	if err != nil {
		return err
	}
	if len(migrated) == 0 {
		fmt.Fprintf(l.out, "Config file %q has no deprecated key. Nothing was changed\n", path)
		return nil
	}
	fmt.Fprintf(l.out, "Config file %q was migrated. Replaced the deprecated keys with \"rules\": %s\n", path, strings.Join(migrated, ", "))
	return nil
}

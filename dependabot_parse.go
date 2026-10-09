package jactionlint

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

// dependabotSyntaxID is the ID of the syntax errors of the Dependabot configuration.
const dependabotSyntaxID = "dependabot-syntax"

// Values which Dependabot accepts. Dependabot adds ecosystems and registry types over time. Keep
// these lists in sync with the options reference:
// https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference
var (
	dependabotEcosystems = []string{
		"bazel", "bun", "bundler", "cargo", "composer", "conda", "deno", "devcontainers", "docker", "docker-compose",
		"dotnet-sdk", "elm", "github-actions", "gitsubmodule", "gomod", "gradle", "helm", "julia", "maven",
		"mix", "nix", "npm", "nuget", "opentofu", "pip", "pre-commit", "pub", "rust-toolchain", "sbt", "swift",
		"terraform", "uv", "vcpkg",
	}
	dependabotIntervals     = []string{"daily", "weekly", "monthly", "quarterly", "semiannually", "yearly", "cron"}
	dependabotDays          = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	dependabotRegistryTypes = []string{
		"cargo-registry", "composer-repository", "docker-registry", "git", "goproxy-server", "hex-organization",
		"hex-repository", "maven-repository", "npm-registry", "nuget-feed", "pub-repository", "python-index",
		"rubygems-server", "terraform-registry",
	}
	dependabotVersioningStrategies = []string{"auto", "increase", "increase-if-necessary", "lockfile-only", "widen"}
	dependabotAllowDependencyTypes = []string{"direct", "indirect", "all", "production", "development"}
	dependabotIgnoreUpdateTypes    = []string{"version-update:semver-major", "version-update:semver-minor", "version-update:semver-patch"}
	dependabotGroupUpdateTypes     = []string{"major", "minor", "patch"}
)

var dependabotTimeRegexp = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func (p *parser) unexpectedDependabotKey(k *String, sec string, expected ...string) {
	if p.unexpectedAt == nil {
		p.unexpectedAt = map[[2]int]bool{}
	}
	p.unexpectedAt[[2]int{k.Pos.Line, k.Pos.Col}] = true
	if len(expected) == 1 {
		// The message of the workflow parser ("expected X key but got Y") reads as two mistakes here
		p.errorAt(k.Pos, fmt.Sprintf("unexpected key %q for %q section. the only key it takes is %q", k.Value, sec, expected[0]))
		return
	}
	p.unexpectedKey(k, sec, slices.Clone(expected))
}

// missingDependabotKey reports a key which the mapping n lacks. It does not when the mapping has a key that was reported
// as unexpected: that is most likely the same mistake (a misspelled key), and one mistake is one finding.
func (p *parser) missingDependabotKey(n *yaml.Node, key, where string) {
	for i := 0; i < len(n.Content); i += 2 {
		if p.unexpectedAt[[2]int{n.Content[i].Line, n.Content[i].Column}] {
			return
		}
	}
	p.errorf(n, "%q key is missing in %s", key, where)
}

// notMapping reports whether the node cannot be parsed as a mapping with required keys. In that
// case the error was already reported so the caller should not report the missing keys as well.
func (p *parser) notMapping(where string, n *yaml.Node) bool {
	if n.Kind == yaml.MappingNode && len(n.Content) > 0 {
		return false
	}
	for range p.parseMappingAt(where, n, false, true) { // Reports the error
	}
	return true
}

// parseDependabotEnum parses a string which must be one of the given values.
func (p *parser) parseDependabotEnum(n *yaml.Node, what string, allowed []string) *String {
	s := p.parseString(n, false)
	if n.Kind == yaml.ScalarNode && s.Value != "" && !slices.Contains(allowed, s.Value) {
		p.errorf(n, "%s %q is invalid. expected one of %s", what, s.Value, quotes(allowed))
	}
	return s
}

func (p *parser) parseDependabotEnums(sec string, n *yaml.Node, what string, allowed []string) []*String {
	ss := p.parseStringSequence(sec, n, true, false)
	for i, s := range ss {
		if s.Value != "" && !slices.Contains(allowed, s.Value) && i < len(n.Content) {
			p.errorf(n.Content[i], "%s %q is invalid. expected one of %s", what, s.Value, quotes(allowed))
		}
	}
	return ss
}

// parseDependabotInt parses an integer in the range [min, max]. Dependabot does not have
// expressions so a string such as "5" is an error.
func (p *parser) parseDependabotInt(n *yaml.Node, what string, min, max int) *Int {
	if n.Kind != yaml.ScalarNode || n.Tag != "!!int" {
		p.errorf(n, "%s must be an integer but found %s node with %q tag", what, nodeKindName(n.Kind), n.Tag)
		return nil
	}
	i := p.parseInt(n)
	if i != nil && (i.Value < min || i.Value > max) {
		if max == math.MaxInt {
			p.errorf(n, "%s must be %d or larger but got %d", what, min, i.Value)
		} else {
			p.errorf(n, "%s must be between %d and %d but got %d", what, min, max, i.Value)
		}
	}
	return i
}

func (p *parser) parseDependabotBool(n *yaml.Node, what string) *Bool {
	if n.Kind != yaml.ScalarNode || n.Tag != "!!bool" {
		p.errorf(n, "%s must be a boolean but found %s node with %q tag", what, nodeKindName(n.Kind), n.Tag)
		return nil
	}
	return p.parseBool(n)
}

func (p *parser) parseDependabotSchedule(pos *Pos, n *yaml.Node) *DependabotSchedule {
	s := &DependabotSchedule{Pos: pos}
	if p.notMapping(`"schedule" section`, n) {
		return s
	}
	for e := range p.parseSectionMapping("schedule", n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "interval":
			s.Interval = p.parseDependabotEnum(v, "schedule interval", dependabotIntervals)
		case "day":
			s.Day = p.parseDependabotEnum(v, "schedule day", dependabotDays)
		case "time":
			s.Time = p.parseString(v, false)
			if v.Kind == yaml.ScalarNode && s.Time.Value != "" && !dependabotTimeRegexp.MatchString(s.Time.Value) {
				p.errorf(v, "schedule time %q is invalid. it must be a time of the day in the format \"HH:MM\"", s.Time.Value)
			}
		case "timezone":
			s.Timezone = p.parseString(v, false)
			if v.Kind == yaml.ScalarNode && s.Timezone.Value != "" {
				// `time.LoadLocation` accepts special values "" and "Local" but they are not IANA timezone names.
				if _, err := time.LoadLocation(s.Timezone.Value); err != nil || s.Timezone.Value == "Local" {
					p.errorf(v, "schedule timezone %q is not a valid IANA timezone name", s.Timezone.Value)
				}
			}
		case "cronjob":
			s.Cronjob = p.parseString(v, false)
		default:
			p.unexpectedDependabotKey(k, "schedule", "interval", "day", "time", "timezone", "cronjob")
		}
	}
	if s.Interval == nil {
		p.missingDependabotKey(n, "interval", `"schedule" section`)
	} else if s.Interval.Value == "cron" && s.Cronjob == nil {
		p.missingDependabotKey(n, "cronjob", `"schedule" section whose interval is "cron"`)
	}
	return s
}

func (p *parser) parseDependabotCommitMessage(pos *Pos, n *yaml.Node) *DependabotCommitMessage {
	c := &DependabotCommitMessage{Pos: pos}
	for e := range p.parseSectionMapping("commit-message", n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "prefix":
			c.Prefix = p.parseString(v, false)
		case "prefix-development":
			c.PrefixDevelopment = p.parseString(v, false)
		case "include":
			c.Include = p.parseDependabotEnum(v, "commit message include", []string{"scope"})
		default:
			p.unexpectedDependabotKey(k, "commit-message", "prefix", "prefix-development", "include")
		}
	}
	return c
}

func (p *parser) parseDependabotBranchName(pos *Pos, n *yaml.Node) *DependabotBranchName {
	b := &DependabotBranchName{Pos: pos}
	if p.notMapping(`"pull-request-branch-name" section`, n) {
		return b
	}
	for e := range p.parseSectionMapping("pull-request-branch-name", n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "separator":
			b.Separator = p.parseDependabotEnum(v, "branch name separator", []string{"-", "_", "/"})
		default:
			p.unexpectedDependabotKey(k, "pull-request-branch-name", "separator")
		}
	}
	if b.Separator == nil {
		p.missingDependabotKey(n, "separator", `"pull-request-branch-name" section`)
	}
	return b
}

func (p *parser) parseDependabotCooldown(pos *Pos, n *yaml.Node) *DependabotCooldown {
	c := &DependabotCooldown{Pos: pos}
	for e := range p.parseSectionMapping("cooldown", n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "default-days":
			c.HasDefaultDays = true
			c.DefaultDays = p.parseDependabotInt(v, "default-days", 0, 90)
		case "semver-major-days":
			c.SemverMajorDays = p.parseDependabotInt(v, "semver-major-days", 0, 90)
		case "semver-minor-days":
			c.SemverMinorDays = p.parseDependabotInt(v, "semver-minor-days", 0, 90)
		case "semver-patch-days":
			c.SemverPatchDays = p.parseDependabotInt(v, "semver-patch-days", 0, 90)
		case "include":
			c.Include = p.parseStringSequence("include", v, true, false)
		case "exclude":
			c.Exclude = p.parseStringSequence("exclude", v, true, false)
		default:
			p.unexpectedDependabotKey(k, "cooldown", "default-days", "semver-major-days", "semver-minor-days", "semver-patch-days", "include", "exclude")
		}
	}
	return c
}

func (p *parser) parseDependabotAllow(n *yaml.Node) *DependabotAllow {
	a := &DependabotAllow{Pos: posAt(n)}
	if p.notMapping(`"allow" item`, n) {
		return a
	}
	for e := range p.parseSectionMapping("allow", n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "dependency-name":
			a.DependencyName = p.parseString(v, false)
		case "dependency-type":
			a.DependencyType = p.parseDependabotEnum(v, "dependency type", dependabotAllowDependencyTypes)
		default:
			p.unexpectedDependabotKey(k, "allow", "dependency-name", "dependency-type")
		}
	}
	if a.DependencyName == nil && a.DependencyType == nil {
		p.errorf(n, `"dependency-name" or "dependency-type" key is missing in "allow" section`)
	}
	return a
}

func (p *parser) parseDependabotIgnore(n *yaml.Node) *DependabotIgnore {
	i := &DependabotIgnore{Pos: posAt(n)}
	if p.notMapping(`"ignore" item`, n) {
		return i
	}
	for e := range p.parseSectionMapping("ignore", n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "dependency-name":
			i.DependencyName = p.parseString(v, false)
		case "versions":
			i.Versions = p.parseStringSequence("versions", v, true, false)
		case "update-types":
			i.UpdateTypes = p.parseDependabotEnums("update-types", v, "update type", dependabotIgnoreUpdateTypes)
		default:
			p.unexpectedDependabotKey(k, "ignore", "dependency-name", "versions", "update-types")
		}
	}
	if i.DependencyName == nil {
		p.missingDependabotKey(n, "dependency-name", `"ignore" section`)
	}
	return i
}

func (p *parser) parseDependabotGroup(name *String, n *yaml.Node) *DependabotGroup {
	g := &DependabotGroup{Pos: name.Pos, Name: name}
	for e := range p.parseMappingAt(fmt.Sprintf("group %q", name.Value), n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "applies-to":
			g.AppliesTo = p.parseDependabotEnum(v, "group applies-to", []string{"version-updates", "security-updates"})
		case "dependency-type":
			g.DependencyType = p.parseDependabotEnum(v, "group dependency type", []string{"development", "production"})
		case "group-by":
			g.GroupBy = p.parseDependabotEnum(v, "group-by", []string{"dependency-name"})
		case "patterns":
			g.Patterns = p.parseStringSequence("patterns", v, true, false)
		case "exclude-patterns":
			g.ExcludePatterns = p.parseStringSequence("exclude-patterns", v, true, false)
		case "update-types":
			g.UpdateTypes = p.parseDependabotEnums("update-types", v, "update type", dependabotGroupUpdateTypes)
		default:
			p.unexpectedDependabotKey(k, "group", "applies-to", "dependency-type", "group-by", "patterns", "exclude-patterns", "update-types")
		}
	}
	return g
}

func (p *parser) parseDependabotGroups(n *yaml.Node) []*DependabotGroup {
	var gs []*DependabotGroup
	for e := range p.parseSectionMapping("groups", n, false, true) {
		gs = append(gs, p.parseDependabotGroup(e.key, e.val))
	}
	return gs
}

// parseDependabotList parses a sequence of mappings.
func parseDependabotList[T any](p *parser, sec string, n *yaml.Node, parse func(*yaml.Node) *T) []*T {
	if !p.checkSequence(sec, n, true) {
		return nil
	}
	ret := make([]*T, 0, len(n.Content))
	for _, c := range n.Content {
		ret = append(ret, parse(c))
	}
	return ret
}

func (p *parser) parseDependabotUpdate(n *yaml.Node) *DependabotUpdate {
	u := &DependabotUpdate{Pos: posAt(n)}
	if p.notMapping(`"updates" item`, n) {
		return u
	}
	// Keys are tracked by name, not by the parsed value. A key with a value of a wrong type was reported
	// when it was parsed and must not be reported again as missing.
	seen := map[string]*String{}
	for e := range p.parseSectionMapping("updates", n, false, true) {
		k, v := e.key, e.val
		seen[e.id] = k
		switch e.id {
		case "package-ecosystem":
			u.PackageEcosystem = p.parseDependabotEnum(v, "package ecosystem", dependabotEcosystems)
		case "directory":
			u.Directory = p.parseString(v, false)
		case "directories":
			u.Directories = p.parseStringSequence("directories", v, false, false)
		case "schedule":
			u.Schedule = p.parseDependabotSchedule(k.Pos, v)
		case "allow":
			u.Allow = parseDependabotList(p, "allow", v, p.parseDependabotAllow)
		case "assignees":
			u.Assignees = p.parseStringSequence("assignees", v, true, false)
		case "commit-message":
			u.CommitMessage = p.parseDependabotCommitMessage(k.Pos, v)
		case "cooldown":
			u.Cooldown = p.parseDependabotCooldown(k.Pos, v)
		case "exclude-paths":
			u.ExcludePaths = p.parseStringSequence("exclude-paths", v, true, false)
		case "groups":
			u.Groups = p.parseDependabotGroups(v)
		case "ignore":
			u.Ignore = parseDependabotList(p, "ignore", v, p.parseDependabotIgnore)
		case "insecure-external-code-execution":
			u.InsecureExternalCodeExecution = p.parseDependabotEnum(v, "insecure-external-code-execution", []string{"allow", "deny"})
		case "labels":
			u.Labels = p.parseStringSequence("labels", v, true, false)
		case "milestone":
			u.Milestone = p.parseDependabotInt(v, "milestone", 0, math.MaxInt)
		case "multi-ecosystem-group":
			u.MultiEcosystemGroup = p.parseString(v, false)
		case "open-pull-requests-limit":
			u.OpenPullRequestsLimit = p.parseDependabotInt(v, "open-pull-requests-limit", 0, math.MaxInt)
		case "patterns":
			u.Patterns = p.parseStringSequence("patterns", v, true, false)
		case "pull-request-branch-name":
			u.PullRequestBranchName = p.parseDependabotBranchName(k.Pos, v)
		case "rebase-strategy":
			u.RebaseStrategy = p.parseDependabotEnum(v, "rebase-strategy", []string{"auto", "disabled"})
		case "registries":
			u.Registries = p.parseStringOrStringSequence("registries", v, true, false)
		case "reviewers":
			u.Reviewers = p.parseStringSequence("reviewers", v, true, false)
		case "target-branch":
			u.TargetBranch = p.parseString(v, false)
		case "vendor":
			u.Vendor = p.parseDependabotBool(v, "vendor")
		case "versioning-strategy":
			u.VersioningStrategy = p.parseDependabotEnum(v, "versioning-strategy", dependabotVersioningStrategies)
		default:
			p.unexpectedDependabotKey(k, "updates",
				"package-ecosystem", "directory", "directories", "schedule", "allow", "assignees", "commit-message",
				"cooldown", "exclude-paths", "groups", "ignore", "insecure-external-code-execution", "labels",
				"milestone", "multi-ecosystem-group", "open-pull-requests-limit", "patterns", "pull-request-branch-name",
				"rebase-strategy", "registries", "reviewers", "target-branch", "vendor", "versioning-strategy")
		}
	}

	if seen["package-ecosystem"] == nil {
		p.missingDependabotKey(n, "package-ecosystem", `"updates" item`)
	}
	switch dir, dirs := seen["directory"], seen["directories"]; {
	case dir == nil && dirs == nil:
		p.errorf(n, `"directory" or "directories" key is missing in "updates" item`)
	case dir != nil && dirs != nil:
		p.errorAt(dirs.Pos, `"directory" and "directories" cannot be set at the same time in "updates" item. use only one of them`)
	}
	if seen["schedule"] == nil && seen["multi-ecosystem-group"] == nil {
		p.missingDependabotKey(n, "schedule", `"updates" item`)
	}
	if seen["multi-ecosystem-group"] != nil && seen["patterns"] == nil {
		p.errorf(n, `"patterns" key is missing in "updates" item which sets "multi-ecosystem-group". Dependabot requires "patterns" to join a multi-ecosystem group`)
	}
	return u
}

func (p *parser) parseDependabotRegistry(name *String, n *yaml.Node) *DependabotRegistry {
	r := &DependabotRegistry{Pos: name.Pos, Name: name}
	if p.notMapping(fmt.Sprintf("registry %q", name.Value), n) {
		return r
	}
	for e := range p.parseMappingAt(fmt.Sprintf("registry %q", name.Value), n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "type":
			r.Type = p.parseDependabotEnum(v, "registry type", dependabotRegistryTypes)
		case "url":
			r.URL = p.parseString(v, false)
		case "username":
			r.Username = p.parseString(v, false)
		case "password":
			r.Password = p.parseString(v, false)
		case "key":
			r.Key = p.parseString(v, false)
		case "token":
			r.Token = p.parseString(v, false)
		case "replaces-base":
			r.ReplacesBase = p.parseDependabotBool(v, "replaces-base")
		default:
			// The keys depend on the type of the registry (OIDC settings, etc.). Only require scalars.
			r.Settings = append(r.Settings, &DependabotSetting{Key: k, Value: p.parseString(v, true)})
		}
	}
	if r.Type == nil {
		p.missingDependabotKey(n, "type", fmt.Sprintf("registry %q", name.Value))
	}
	return r
}

func (p *parser) parseDependabotRegistries(n *yaml.Node) []*DependabotRegistry {
	rs := []*DependabotRegistry{}
	for e := range p.parseSectionMapping("registries", n, false, true) {
		rs = append(rs, p.parseDependabotRegistry(e.key, e.val))
	}
	return rs
}

func (p *parser) parseDependabotMultiEcosystemGroup(name *String, n *yaml.Node) *DependabotMultiEcosystemGroup {
	g := &DependabotMultiEcosystemGroup{Pos: name.Pos, Name: name}
	if p.notMapping(fmt.Sprintf("multi-ecosystem group %q", name.Value), n) {
		return g
	}
	for e := range p.parseMappingAt(fmt.Sprintf("multi-ecosystem group %q", name.Value), n, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "schedule":
			g.Schedule = p.parseDependabotSchedule(k.Pos, v)
		case "assignees":
			g.Assignees = p.parseStringSequence("assignees", v, true, false)
		case "reviewers":
			g.Reviewers = p.parseStringSequence("reviewers", v, true, false)
		case "labels":
			g.Labels = p.parseStringSequence("labels", v, true, false)
		case "milestone":
			g.Milestone = p.parseDependabotInt(v, "milestone", 0, math.MaxInt)
		case "target-branch":
			g.TargetBranch = p.parseString(v, false)
		case "commit-message":
			g.CommitMessage = p.parseDependabotCommitMessage(k.Pos, v)
		case "pull-request-branch-name":
			g.PullRequestBranchName = p.parseDependabotBranchName(k.Pos, v)
		case "open-pull-requests-limit":
			g.OpenPullRequestsLimit = p.parseDependabotInt(v, "open-pull-requests-limit", 0, math.MaxInt)
		default:
			p.unexpectedDependabotKey(k, "multi-ecosystem-groups", "schedule", "assignees", "reviewers", "labels", "milestone",
				"target-branch", "commit-message", "pull-request-branch-name", "open-pull-requests-limit")
		}
	}
	if g.Schedule == nil {
		p.missingDependabotKey(n, "schedule", fmt.Sprintf("multi-ecosystem group %q", name.Value))
	}
	return g
}

func (p *parser) parseDependabotMultiEcosystemGroups(n *yaml.Node) []*DependabotMultiEcosystemGroup {
	gs := []*DependabotMultiEcosystemGroup{}
	for e := range p.parseSectionMapping("multi-ecosystem-groups", n, false, true) {
		gs = append(gs, p.parseDependabotMultiEcosystemGroup(e.key, e.val))
	}
	return gs
}

// checkDependabotReferences reports names which are used by updates but not defined.
func (p *parser) checkDependabotReferences(d *Dependabot) {
	registries := make(map[string]struct{}, len(d.Registries))
	for _, r := range d.Registries {
		registries[r.Name.Value] = struct{}{}
	}
	groups := make(map[string]struct{}, len(d.MultiEcosystemGroups))
	for _, g := range d.MultiEcosystemGroups {
		groups[g.Name.Value] = struct{}{}
	}

	type updateKey struct{ ecosystem, directory, branch string }
	seen := map[updateKey]*Pos{}

	for _, u := range d.Updates {
		for _, r := range u.Registries {
			if r.Value == "" || r.Value == "*" {
				continue
			}
			if _, ok := registries[r.Value]; !ok {
				p.errorfAt(r.Pos, "registry %q is not defined in \"registries\" section", r.Value)
			}
		}
		if g := u.MultiEcosystemGroup; g != nil && g.Value != "" {
			if _, ok := groups[g.Value]; !ok {
				p.errorfAt(g.Pos, "multi-ecosystem group %q is not defined in \"multi-ecosystem-groups\" section", g.Value)
			}
		}

		if u.PackageEcosystem == nil {
			continue
		}
		dirs := u.Directories
		if u.Directory != nil {
			dirs = append(slices.Clone(dirs), u.Directory)
		}
		branch := ""
		if u.TargetBranch != nil {
			branch = u.TargetBranch.Value
		}
		for _, dir := range dirs {
			key := updateKey{u.PackageEcosystem.Value, dir.Value, branch}
			if prev, ok := seen[key]; ok {
				p.errorfAt(dir.Pos, "update for package ecosystem %q and directory %q is duplicated. previously defined at %s", key.ecosystem, key.directory, prev.String())
				continue
			}
			seen[key] = dir.Pos
		}
	}
}

// parseDependabot parses the root node of the Dependabot configuration.
// https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference
func (p *parser) parseDependabot(n *yaml.Node) *Dependabot {
	p.resolveAliases(n)

	d := &Dependabot{}

	if n.Line == 0 {
		n.Line = 1
	}
	if n.Column == 0 {
		n.Column = 1
	}
	d.Pos = posAt(n)

	if len(n.Content) == 0 {
		p.errorID("dependabot-syntax", n, "Dependabot configuration is empty")
		return d
	}
	root := n.Content[0]
	d.Pos = posAt(root)

	for e := range p.parseSectionMapping("dependabot", root, false, true) {
		k, v := e.key, e.val
		switch e.id {
		case "version":
			d.Version = p.parseDependabotInt(v, "version", 0, math.MaxInt)
			if d.Version != nil && d.Version.Value != 2 {
				p.errorf(v, "version %d is not supported. Dependabot supports only version 2", d.Version.Value)
			}
		case "updates":
			if p.checkSequence("updates", v, false) {
				d.Updates = make([]*DependabotUpdate, 0, len(v.Content))
				for _, c := range v.Content {
					d.Updates = append(d.Updates, p.parseDependabotUpdate(c))
				}
			}
		case "registries":
			d.Registries = p.parseDependabotRegistries(v)
		case "multi-ecosystem-groups":
			d.MultiEcosystemGroups = p.parseDependabotMultiEcosystemGroups(v)
		case "enable-beta-ecosystems":
			d.EnableBetaEcosystems = p.parseDependabotBool(v, "enable-beta-ecosystems")
		default:
			p.unexpectedDependabotKey(k, "dependabot", "version", "updates", "registries", "multi-ecosystem-groups", "enable-beta-ecosystems")
		}
	}

	if d.Version == nil && !mappingHasKey(root, "version") {
		p.missingDependabotKey(root, "version", `Dependabot configuration`)
	}
	if d.Updates == nil && !mappingHasKey(root, "updates") {
		p.missingDependabotKey(root, "updates", `Dependabot configuration`)
	}

	p.checkDependabotReferences(d)
	return d
}

// mappingHasKey reports whether the mapping has the key. A key which is present but invalid
// already caused an error so reporting that it is missing would only be noise.
func mappingHasKey(m *yaml.Node, key string) bool {
	if m.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return true
		}
	}
	return false
}

// ParseDependabot parses given source as the Dependabot configuration (.github/dependabot.yml). It
// returns all errors detected while parsing the input. Like Parse, detecting one error does not
// stop parsing.
func ParseDependabot(b []byte) (*Dependabot, []*Error) {
	var n yaml.Node

	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, handleYAMLUnmarshalError(err)
	}

	p := &parser{lines: strings.Split(string(b), "\n"), syntaxID: dependabotSyntaxID}
	d := p.parseDependabot(&n)

	return d, p.errors
}

package jactionlint

// Dependabot is the root of the syntax tree of a Dependabot configuration file
// (.github/dependabot.yml). Every node has the position where it starts in the source.
//
// https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference
type Dependabot struct {
	// Version is the "version" key. Dependabot supports only version 2.
	Version *Int
	// Updates is the "updates" key. Each item configures the updates of one package ecosystem.
	Updates []*DependabotUpdate
	// Registries is the "registries" key in the order of the source.
	Registries []*DependabotRegistry
	// MultiEcosystemGroups is the "multi-ecosystem-groups" key in the order of the source.
	MultiEcosystemGroups []*DependabotMultiEcosystemGroup
	// EnableBetaEcosystems is the "enable-beta-ecosystems" key.
	EnableBetaEcosystems *Bool
	// Pos is the position of the top-level mapping.
	Pos *Pos
}

// DependabotUpdate is an item of "updates" of the Dependabot configuration.
type DependabotUpdate struct {
	// Pos is the position of the item.
	Pos *Pos
	// PackageEcosystem is the "package-ecosystem" key such as "npm" or "github-actions".
	PackageEcosystem *String
	// Directory is the "directory" key.
	Directory *String
	// Directories is the "directories" key.
	Directories []*String
	// Schedule is the "schedule" key.
	Schedule *DependabotSchedule
	// Allow is the "allow" key.
	Allow []*DependabotAllow
	// Assignees is the "assignees" key.
	Assignees []*String
	// CommitMessage is the "commit-message" key.
	CommitMessage *DependabotCommitMessage
	// Cooldown is the "cooldown" key.
	Cooldown *DependabotCooldown
	// ExcludePaths is the "exclude-paths" key.
	ExcludePaths []*String
	// Groups is the "groups" key in the order of the source.
	Groups []*DependabotGroup
	// Ignore is the "ignore" key.
	Ignore []*DependabotIgnore
	// InsecureExternalCodeExecution is the "insecure-external-code-execution" key: "allow" or "deny".
	InsecureExternalCodeExecution *String
	// Labels is the "labels" key.
	Labels []*String
	// Milestone is the "milestone" key.
	Milestone *Int
	// MultiEcosystemGroup is the "multi-ecosystem-group" key. It is the name of an item of
	// Dependabot.MultiEcosystemGroups.
	MultiEcosystemGroup *String
	// OpenPullRequestsLimit is the "open-pull-requests-limit" key.
	OpenPullRequestsLimit *Int
	// Patterns is the "patterns" key which is used with MultiEcosystemGroup.
	Patterns []*String
	// PullRequestBranchName is the "pull-request-branch-name" key.
	PullRequestBranchName *DependabotBranchName
	// RebaseStrategy is the "rebase-strategy" key: "auto" or "disabled".
	RebaseStrategy *String
	// Registries is the "registries" key. Each item is a name of Dependabot.Registries or "*".
	Registries []*String
	// Reviewers is the "reviewers" key.
	Reviewers []*String
	// TargetBranch is the "target-branch" key.
	TargetBranch *String
	// Vendor is the "vendor" key.
	Vendor *Bool
	// VersioningStrategy is the "versioning-strategy" key.
	VersioningStrategy *String
}

// DependabotSchedule is the "schedule" key of an update or a multi-ecosystem group.
type DependabotSchedule struct {
	// Pos is the position of the "schedule" key.
	Pos *Pos
	// Interval is the "interval" key such as "daily" or "weekly".
	Interval *String
	// Day is the "day" key.
	Day *String
	// Time is the "time" key in the format HH:MM.
	Time *String
	// Timezone is the "timezone" key, an IANA timezone name.
	Timezone *String
	// Cronjob is the "cronjob" key which is used with the interval "cron".
	Cronjob *String
}

// DependabotAllow is an item of "allow" of an update.
type DependabotAllow struct {
	// Pos is the position of the item.
	Pos *Pos
	// DependencyName is the "dependency-name" key.
	DependencyName *String
	// DependencyType is the "dependency-type" key.
	DependencyType *String
}

// DependabotIgnore is an item of "ignore" of an update.
type DependabotIgnore struct {
	// Pos is the position of the item.
	Pos *Pos
	// DependencyName is the "dependency-name" key.
	DependencyName *String
	// Versions is the "versions" key.
	Versions []*String
	// UpdateTypes is the "update-types" key.
	UpdateTypes []*String
}

// DependabotCommitMessage is the "commit-message" key of an update.
type DependabotCommitMessage struct {
	// Pos is the position of the "commit-message" key.
	Pos *Pos
	// Prefix is the "prefix" key.
	Prefix *String
	// PrefixDevelopment is the "prefix-development" key.
	PrefixDevelopment *String
	// Include is the "include" key: "scope".
	Include *String
}

// DependabotCooldown is the "cooldown" key of an update. It delays updating to a version until it
// is old enough.
type DependabotCooldown struct {
	// Pos is the position of the "cooldown" key.
	Pos *Pos
	// DefaultDays is the "default-days" key. It is nil when the key is missing or its value is not a valid integer.
	DefaultDays *Int
	// HasDefaultDays is whether the "default-days" key is written, valid or not.
	HasDefaultDays bool
	// SemverMajorDays is the "semver-major-days" key.
	SemverMajorDays *Int
	// SemverMinorDays is the "semver-minor-days" key.
	SemverMinorDays *Int
	// SemverPatchDays is the "semver-patch-days" key.
	SemverPatchDays *Int
	// Include is the "include" key.
	Include []*String
	// Exclude is the "exclude" key.
	Exclude []*String
}

// DependabotGroup is an item of "groups" of an update. The key of the mapping is the Name.
type DependabotGroup struct {
	// Pos is the position of the name of the group.
	Pos *Pos
	// Name is the name of the group.
	Name *String
	// AppliesTo is the "applies-to" key: "version-updates" or "security-updates".
	AppliesTo *String
	// DependencyType is the "dependency-type" key: "development" or "production".
	DependencyType *String
	// GroupBy is the "group-by" key.
	GroupBy *String
	// Patterns is the "patterns" key.
	Patterns []*String
	// ExcludePatterns is the "exclude-patterns" key.
	ExcludePatterns []*String
	// UpdateTypes is the "update-types" key.
	UpdateTypes []*String
}

// DependabotBranchName is the "pull-request-branch-name" key of an update.
type DependabotBranchName struct {
	// Pos is the position of the key.
	Pos *Pos
	// Separator is the "separator" key.
	Separator *String
}

// DependabotRegistry is an item of "registries" of the Dependabot configuration. The key of the
// mapping is the Name.
type DependabotRegistry struct {
	// Pos is the position of the name of the registry.
	Pos *Pos
	// Name is the name which updates refer to.
	Name *String
	// Type is the "type" key such as "npm-registry".
	Type *String
	// URL is the "url" key.
	URL *String
	// Username is the "username" key.
	Username *String
	// Password is the "password" key.
	Password *String
	// Key is the "key" key.
	Key *String
	// Token is the "token" key.
	Token *String
	// ReplacesBase is the "replaces-base" key.
	ReplacesBase *Bool
	// Settings are the other keys. They depend on the type of the registry and new ones are added
	// over time (e.g. for OIDC), so they are not validated.
	Settings []*DependabotSetting
}

// DependabotSetting is a key-value pair whose key jactionlint does not know.
type DependabotSetting struct {
	// Key is the key of the setting.
	Key *String
	// Value is the value of the setting.
	Value *String
}

// DependabotMultiEcosystemGroup is an item of "multi-ecosystem-groups" of the Dependabot
// configuration. It makes updates of several package ecosystems share one pull request. The key of
// the mapping is the Name.
type DependabotMultiEcosystemGroup struct {
	// Pos is the position of the name of the group.
	Pos *Pos
	// Name is the name which updates refer to with "multi-ecosystem-group".
	Name *String
	// Schedule is the "schedule" key.
	Schedule *DependabotSchedule
	// Assignees is the "assignees" key.
	Assignees []*String
	// Reviewers is the "reviewers" key.
	Reviewers []*String
	// Labels is the "labels" key.
	Labels []*String
	// Milestone is the "milestone" key.
	Milestone *Int
	// TargetBranch is the "target-branch" key.
	TargetBranch *String
	// CommitMessage is the "commit-message" key.
	CommitMessage *DependabotCommitMessage
	// PullRequestBranchName is the "pull-request-branch-name" key.
	PullRequestBranchName *DependabotBranchName
	// OpenPullRequestsLimit is the "open-pull-requests-limit" key.
	OpenPullRequestsLimit *Int
}

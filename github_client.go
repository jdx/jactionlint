package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// GitHubClient is the part of the GitHub API which the online rules (see LinterOptions.Online) use. It
// is an interface so that tests, and programs embedding jactionlint, can serve the data from fixtures
// (see NewFixtureGitHubClient) instead of the network. The client built into the jactionlint command
// talks REST to api.github.com, and caches the answers on disk.
//
// A client must be safe for concurrent use. Methods return ErrGitHubNotFound (possibly wrapped) when
// the repository or object does not exist or is not visible, and an error satisfying
// errors.Is(err, ErrGitHubRateLimited) when the API refused the request because of the rate limit.
// The repository owner and name are used as given; they are not case-normalized.
type GitHubClient interface {
	// Repository returns the metadata of a repository.
	Repository(ctx context.Context, owner, repo string) (*GitHubRepo, error)
	// Tags returns the tags of a repository with the commit each one points to. Annotated tags are
	// dereferenced to the commit. The client reads a bounded number of tags: when there are more,
	// the list is marked as truncated.
	Tags(ctx context.Context, owner, repo string) (*GitHubTagList, error)
	// ResolveRef resolves the branch (GitHubRefHeads) or tag (GitHubRefTags) of the name to the SHA of
	// the commit it points to. Annotated tags are dereferenced. The boolean is false when the ref
	// does not exist.
	ResolveRef(ctx context.Context, owner, repo string, ns GitHubRefNamespace, name string) (sha string, found bool, err error)
	// Branches returns up to limit branches with their head commit. The list is marked as truncated when
	// the repository has more branches.
	Branches(ctx context.Context, owner, repo string, limit int) (*GitHubBranchList, error)
	// Compare tells how the head commit relates to the base commit (see GitHubCompareStatus). Both
	// are commit SHAs. It returns ErrGitHubNotFound when GitHub knows no such commit.
	Compare(ctx context.Context, owner, repo, base, head string) (GitHubCompareStatus, error)
	// Advisories returns the published security advisories which affect the GitHub Actions
	// ecosystem package "owner/repo".
	Advisories(ctx context.Context, owner, repo string) ([]GitHubAdvisory, error)
}

// GitHubRepo is the metadata of a repository.
type GitHubRepo struct {
	// Archived is true when the repository is archived, which makes it read-only and unmaintained.
	Archived bool `json:"archived"`
	// DefaultBranch is the name of the default branch.
	DefaultBranch string `json:"default_branch"`
}

// GitHubTag is a tag and the commit it points to.
type GitHubTag struct {
	Name string `json:"name"`
	// SHA is the full SHA of the commit. For an annotated tag it is the commit the tag object points to.
	SHA string `json:"sha"`
}

// GitHubTagList is the tags of a repository.
type GitHubTagList struct {
	Tags []GitHubTag `json:"tags"`
	// Truncated is true when the repository has more tags than Tags lists. A tag or commit missing
	// from the list may still exist.
	Truncated bool `json:"truncated,omitempty"`
}

// GitHubBranch is a branch and its head commit.
type GitHubBranch struct {
	Name string `json:"name"`
	SHA  string `json:"sha"`
}

// GitHubBranchList is the branches of a repository.
type GitHubBranchList struct {
	Branches []GitHubBranch `json:"branches"`
	// Truncated is true when the repository has more branches than Branches lists.
	Truncated bool `json:"truncated,omitempty"`
}

// GitHubRefNamespace tells whether a ref name is a branch or a tag.
type GitHubRefNamespace string

const (
	// GitHubRefHeads is the namespace of branches (refs/heads/).
	GitHubRefHeads GitHubRefNamespace = "heads"
	// GitHubRefTags is the namespace of tags (refs/tags/).
	GitHubRefTags GitHubRefNamespace = "tags"
)

// GitHubCompareStatus is how the head commit of a comparison relates to the base commit.
type GitHubCompareStatus string

const (
	// GitHubCompareIdentical means that both commits are the same.
	GitHubCompareIdentical GitHubCompareStatus = "identical"
	// GitHubCompareAhead means that the head is a descendant of the base.
	GitHubCompareAhead GitHubCompareStatus = "ahead"
	// GitHubCompareBehind means that the head is an ancestor of the base.
	GitHubCompareBehind GitHubCompareStatus = "behind"
	// GitHubCompareDiverged means that each commit has changes which the other lacks.
	GitHubCompareDiverged GitHubCompareStatus = "diverged"
)

// GitHubAdvisory is a published security advisory (GHSA).
type GitHubAdvisory struct {
	// ID is the GHSA identifier such as "GHSA-mrrh-fwg8-r2c3".
	ID string `json:"id"`
	// CVE is the CVE identifier. It can be empty.
	CVE string `json:"cve,omitempty"`
	// Summary is the one-line description.
	Summary string `json:"summary"`
	// Severity is "low", "medium", "high" or "critical".
	Severity string `json:"severity,omitempty"`
	// URL is the page of the advisory.
	URL string `json:"url,omitempty"`
	// Vulnerabilities are the affected packages.
	Vulnerabilities []GitHubVulnerability `json:"vulnerabilities"`
}

// GitHubVulnerability is one affected package of an advisory.
type GitHubVulnerability struct {
	// Package is the name of the affected action: "owner/repo", optionally with a subdirectory.
	Package string `json:"package"`
	// VulnerableRange is the affected versions, such as ">= 1.0.0, < 1.2.3".
	VulnerableRange string `json:"range"`
	// FirstPatched is the first version which fixes the vulnerability. It is empty when none does.
	FirstPatched string `json:"first_patched,omitempty"`
}

// ErrGitHubNotFound is returned (wrapped) by a GitHubClient when something does not exist or is not
// visible to the client.
var ErrGitHubNotFound = errors.New("not found on GitHub")

// ErrGitHubRateLimited is matched by errors.Is for an error returned by a GitHubClient because the
// API rate limit was reached. The client does not retry: the online rules stop for the run.
var ErrGitHubRateLimited = errors.New("GitHub API rate limit exceeded")

// GitHubRateLimitError is the error of a GitHubClient when the API refused a request because of its
// rate limit. errors.Is(err, ErrGitHubRateLimited) is true for it.
type GitHubRateLimitError struct {
	// Reset is when the limit resets. It is the zero time when unknown.
	Reset time.Time
	// Authenticated is whether the request carried a token. Unauthenticated requests have a much
	// lower limit.
	Authenticated bool

	now func() time.Time // for tests
}

func (e *GitHubRateLimitError) Error() string {
	s := ErrGitHubRateLimited.Error()
	if !e.Reset.IsZero() {
		now := time.Now
		if e.now != nil {
			now = e.now
		}
		s += fmt.Sprintf(" (resets at %s", e.Reset.Local().Format("15:04:05"))
		if d := e.Reset.Sub(now()); d > time.Second {
			s += ", in " + d.Round(time.Second).String()
		}
		s += ")"
	}
	if !e.Authenticated {
		s += ". set GITHUB_TOKEN (or GH_TOKEN) to raise the limit"
	}
	return s
}

// Is makes errors.Is(err, ErrGitHubRateLimited) true.
func (e *GitHubRateLimitError) Is(target error) bool { return target == ErrGitHubRateLimited }

// ErrGitHubNotCached is returned (wrapped) by the built-in client in the offline mode (--online=cache)
// when the disk cache has no answer for a request. Nothing is sent to the network in that mode.
var ErrGitHubNotCached = errors.New("not in the cache of GitHub answers")

// errOnlineUnsupported is why the online rules cannot run in builds without network access.
var errOnlineUnsupported = errors.New("online checks need network access to the GitHub API, which is not available in the WebAssembly build (the playground)")

// GitHubPackageAdvisoryClient is an optional interface of a GitHubClient. GitHub publishes the
// advisories of an action in a subdirectory (gradle/actions/setup-gradle) under the full package
// name, which the "owner/repo" lookup of GitHubClient.Advisories does not find. A client which
// implements it returns the advisories which affect any of the packages, each written
// "owner/repo" or "owner/repo/subpath", in one lookup and without duplicates.
type GitHubPackageAdvisoryClient interface {
	AdvisoriesForPackages(ctx context.Context, packages []string) ([]GitHubAdvisory, error)
}

// GitHubBranchScan is the result of GitHubBranchScanner.CommitOnAnyBranch.
type GitHubBranchScan struct {
	// Found is true when the commit is the head of a branch or an ancestor of one.
	Found bool
	// Complete is true when every branch of the repository was looked at, so that !Found means the
	// commit is on no branch. It is false when the repository has more branches than the limit.
	Complete bool
}

// GitHubBranchScanner is an optional interface of a GitHubClient. A client which implements it answers
// "is the commit on any branch?" in fewer requests than comparing with every branch (the built-in
// client uses one GraphQL request per 100 branches when it has a token). The online rules use it when
// it is there, and compare the branches one by one otherwise.
type GitHubBranchScanner interface {
	// CommitOnAnyBranch looks at up to limit branches. It returns ErrGitHubBranchScanUnavailable
	// when the client cannot do it now, for example without a token.
	CommitOnAnyBranch(ctx context.Context, owner, repo, sha string, limit int) (GitHubBranchScan, error)
}

// ErrGitHubBranchScanUnavailable is returned by GitHubBranchScanner when the scan cannot be done and the
// caller should compare the branches one by one.
var ErrGitHubBranchScanUnavailable = errors.New("branch scan unavailable")

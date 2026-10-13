package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// maxConsecutiveGitHubFailures is how many failed requests in a row (timeouts, unreachable API, server
// errors) make the online session skip the remaining lookups of the run instead of trying each one:
// the API is down. A failure for one repository (404, 403) never counts.
const maxConsecutiveGitHubFailures = 3

// errOnlineExcluded is why a lookup was not made: the repository is outside the allow list or on the deny
// list of the online options. It is not a failure.
var errOnlineExcluded = errors.New("excluded by the online allow or deny list")

// GitHubStatusError is the error of a GitHubClient when the API answered with a failure status which
// is neither "not found" nor "rate limited".
type GitHubStatusError struct {
	// Status is the HTTP status code.
	Status int
	// Message is what the API said, if anything.
	Message string
}

func (e *GitHubStatusError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("GitHub API returned status %d", e.Status)
	}
	return fmt.Sprintf("GitHub API returned status %d: %s", e.Status, e.Message)
}

// memo caches the result of a function per key and runs it at most once for each key, also when many
// goroutines ask for the same key at the same time (files are linted in parallel). Errors are
// cached too so that a failing lookup is never repeated.
type memo[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]*memoEntry[V]
}

type memoEntry[V any] struct {
	once sync.Once
	v    V
	err  error
}

func (m *memo[K, V]) get(k K, fn func() (V, error)) (V, error) {
	m.mu.Lock()
	if m.m == nil {
		m.m = map[K]*memoEntry[V]{}
	}
	e, ok := m.m[k]
	if !ok {
		e = &memoEntry[V]{}
		m.m[k] = e
	}
	m.mu.Unlock()
	e.once.Do(func() { e.v, e.err = fn() })
	return e.v, e.err
}

type repoKey struct{ owner, repo string }

func newRepoKey(owner, repo string) repoKey {
	return repoKey{strings.ToLower(owner), strings.ToLower(repo)}
}

type refKey struct {
	repoKey
	ns   GitHubRefNamespace
	name string
}

type compareKey struct {
	repoKey
	base, head string
}

type refResult struct {
	sha   string
	found bool
}

// tagIndex is the tags of a repository indexed both ways.
type tagIndex struct {
	byName    map[string]string
	bySHA     map[string][]string // sorted by name
	truncated bool
}

func newTagIndex(l *GitHubTagList) *tagIndex {
	idx := &tagIndex{byName: make(map[string]string, len(l.Tags)), bySHA: map[string][]string{}, truncated: l.Truncated}
	for _, t := range l.Tags {
		sha := strings.ToLower(t.SHA)
		idx.byName[t.Name] = sha
		idx.bySHA[sha] = append(idx.bySHA[sha], t.Name)
	}
	for _, names := range idx.bySHA {
		slices.Sort(names)
	}
	return idx
}

// onlineSession is what the online rules use to reach GitHub during one run of the linter. It wraps a
// GitHubClient and
//
//   - remembers every answer, so the same repository, tag or commit is asked for once however many
//     steps and files mention it,
//   - skips a lookup which failed (404, 403, a server error, a timeout, no DNS) and remembers the
//     failure for the run, so the other lookups and their findings are not affected. It warns once per
//     kind of failure and counts the skipped lookups (see skippedLookups, which --online=strict turns
//     into a failing exit status),
//   - stops asking when asking again cannot work: the rate limit is reached (until it resets), the
//     token was rejected, the API failed several lookups in a row, or the run was interrupted,
//   - does not look up the repositories outside its allow list or on its deny list.
//
// It is safe for concurrent use.
type onlineSession struct {
	client GitHubClient
	ctx    context.Context
	warn   func(msg string)
	// detail receives one line for every skipped lookup (--verbose). It can be nil.
	detail func(format string, args ...any)
	allow  []string
	deny   []string

	mu         sync.Mutex
	stopErr    error // the run was interrupted
	blocked    error // asking again cannot work, see block
	blockUntil time.Time
	transient  int // lookups in a row which failed in a way that suggests an outage
	skipped    int
	// advisoryListNoted is set once the failure of the advisory list was said (--verbose).
	advisoryListNoted bool
	skippedBy         map[failureKind]int // skipped by the kind of failure
	excluded          map[string]bool     // repositories left out by the allow and deny lists
	seen              map[failureKind]bool
	// fileFailures is the failure of the first file lookup of a repository which failed, see file.
	fileFailures map[repoKey]error
	fileLocks    map[repoKey]*sync.Mutex

	repos      memo[repoKey, *GitHubRepo]
	tags       memo[repoKey, *tagIndex]
	refs       memo[refKey, refResult]
	branches   memo[repoKey, *GitHubBranchList]
	compares   memo[compareKey, GitHubCompareStatus]
	advisories memo[string, []GitHubAdvisory]
	// advisoryList is the list of all advisories of the actions ecosystem, asked for once (see actionAdvisories).
	advisoryList memo[struct{}, []GitHubAdvisory]
	origins      memo[originKey, originResult]

	// images answers the digest of container images. It is nil when the client itself can (ImageDigestClient).
	images           ImageDigestClient
	offline          bool // the cache mode: the registries are not asked
	imageDigests     memo[imageKey, string]
	registryDown     map[string]error // registries which are not asked again in this run
	registryFailures map[string]int   // failures in a row, by registry
	files            memo[fileKey, remoteFile]
	actions          memo[remoteActionKey, *ActionMetadata]
	workflows        memo[remoteWorkflowKey, *ReusableWorkflowMetadata]
	pins             memo[pinProblemsKey, []pinProblem]
	owners           memo[string, *GitHubOwner]
	releases         memo[releaseKey, *GitHubRelease]
	releaseDates     memo[releaseDateKey, *GitHubReleaseDate]

	environments memo[environmentKey, environmentResult]
}

type originKey struct {
	repoKey
	sha   string
	limit int
}

type originResult struct {
	origin commitOrigin
}

func newOnlineSession(ctx context.Context, client GitHubClient, warn func(string)) *onlineSession {
	if ctx == nil {
		ctx = context.Background()
	}
	return &onlineSession{client: client, ctx: ctx, warn: warn, seen: map[failureKind]bool{}}
}

// skippedLookups is how many lookups were skipped because they failed. Lookups outside the allow
// list are not counted.
func (s *onlineSession) skippedLookups() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.skipped
}

func (s *onlineSession) countKindLocked(kind failureKind) {
	if s.skippedBy == nil {
		s.skippedBy = map[failureKind]int{}
	}
	s.skippedBy[kind]++
}

// skippedByKind is skippedLookups by the kind of failure.
func (s *onlineSession) skippedByKind() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.skippedBy))
	for k, n := range s.skippedBy {
		out[string(k)] = n
	}
	return out
}

// allows reports whether the repository may be looked up.
func (s *onlineSession) allows(owner, repo string) bool {
	slug := owner + "/" + repo
	if s.matchesLists(slug) {
		return true
	}
	s.mu.Lock()
	if s.excluded == nil {
		s.excluded = map[string]bool{}
	}
	s.excluded[strings.ToLower(slug)] = true
	s.mu.Unlock()
	return false
}

func (s *onlineSession) matchesLists(slug string) bool {
	for _, p := range s.deny {
		if matchRepoPattern(p, slug) {
			return false
		}
	}
	if len(s.allow) == 0 {
		return true
	}
	for _, p := range s.allow {
		if matchRepoPattern(p, slug) {
			return true
		}
	}
	return false
}

func (s *onlineSession) excludedRepos() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.excluded)
}

// stopped returns the reason why the online rules stopped for the rest of the run (an interruption), or nil.
func (s *onlineSession) stopped() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopErr == nil {
		if err := s.ctx.Err(); err != nil {
			s.stopLocked(fmt.Errorf("interrupted: %w", err))
		}
	}
	return s.stopErr
}

func (s *onlineSession) stopLocked(err error) {
	if s.stopErr != nil {
		return
	}
	s.stopErr = err
	if s.warn != nil {
		s.warn(fmt.Sprintf("the online checks were stopped and the findings which need GitHub are missing: %s", err))
	}
}

// failureKind groups the ways a lookup fails; the session warns once for each kind.
type failureKind string

const (
	failRateLimit    failureKind = "rate limit"
	failUnauthorized failureKind = "unauthorized"
	failForbidden    failureKind = "forbidden"
	failNotFound     failureKind = "not found"
	failServer       failureKind = "server error"
	failTimeout      failureKind = "timeout"
	failNetwork      failureKind = "network"
	failNotCached    failureKind = "not cached"
	failOther        failureKind = "other"
)

// classifyFailure tells what kind of failure the error of a GitHubClient is.
func classifyFailure(err error) failureKind {
	var statusErr *GitHubStatusError
	var netErr net.Error
	switch {
	case errors.Is(err, ErrGitHubRateLimited):
		return failRateLimit
	case errors.Is(err, ErrGitHubNotCached):
		return failNotCached
	case errors.Is(err, ErrGitHubNotFound):
		return failNotFound
	case errors.As(err, &statusErr):
		switch {
		case statusErr.Status == http.StatusUnauthorized:
			return failUnauthorized
		case statusErr.Status == http.StatusForbidden:
			return failForbidden
		case statusErr.Status == http.StatusNotFound:
			return failNotFound
		case statusErr.Status >= 500:
			return failServer
		}
		return failOther
	case errors.Is(err, context.DeadlineExceeded):
		return failTimeout
	case errors.As(err, &netErr):
		if netErr.Timeout() {
			return failTimeout
		}
		// Only a host that does not resolve or a refused connection will not work for the next lookup
		// either. A reset or a dropped connection is transient like a server error.
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) || errors.Is(err, syscall.ECONNREFUSED) {
			return failNetwork
		}
		return failOther
	}
	return failOther
}

// failureMessage is the one warning for the first failure of a kind.
func failureMessage(kind failureKind, what string, err error) string {
	where := ""
	lookup := err.Error()
	if what != "" {
		where = " for " + what
		lookup = fmt.Sprintf("could not look up %s: %s", what, err)
	}
	const tail = " --verbose lists every skipped lookup."
	switch kind {
	case failRateLimit:
		if host := hostOf(err); host != "" {
			return fmt.Sprintf("online: %s. the lookups on %s are skipped until then and the others go on, so some findings may be missing.", err, host) + tail
		}
		return fmt.Sprintf("online: %s. the lookups which need GitHub are skipped until then, so some findings may be missing.", err) + tail
	case failUnauthorized:
		if host := hostOf(err); host != "" {
			return fmt.Sprintf("online: %s rejected the token (%s). the remaining lookups on it are skipped and the others go on; fix the token or run without one for public repositories.", host, err) + tail
		}
		return fmt.Sprintf("online: GitHub rejected the token (%s). the remaining lookups are skipped; fix the token or run without one for public repositories.", err) + tail
	case failForbidden:
		return fmt.Sprintf("online: %s. the lookup was skipped. a token with access to the repository, or listing it under online-options.deny, avoids this.", lookup) + tail
	case failNotFound:
		return fmt.Sprintf("online: %s was not found on GitHub or is not visible to the token, so the checks which need it were skipped. list private actions under online-options.deny to silence this.", orSomething(what)) + tail
	case failServer:
		return fmt.Sprintf("online: %s. the lookup was skipped.", lookup) + tail
	case failTimeout:
		return fmt.Sprintf("online: the GitHub API did not answer in time%s (%s). the lookup was skipped.", where, err) + tail
	case failNetwork:
		return fmt.Sprintf("online: %s. the lookup was skipped; --online=cache works from the cache without the network.", err) + tail
	case failNotCached:
		return fmt.Sprintf("online: %s has no cached answer and --online=cache does not use the network. run once with --online to fill the cache.", orSomething(what)) + tail
	}
	return fmt.Sprintf("online: %s. the lookup was skipped.", lookup) + tail
}

// hostOf returns the host whose failure the error is (see hostScopedError), or "" for a failure of the whole session.
func hostOf(err error) string {
	var hs *hostScopedError
	if errors.As(err, &hs) {
		return hs.host
	}
	return ""
}

func orSomething(what string) string {
	if what == "" {
		return "a repository"
	}
	return what
}

// noteMissing records that a repository is not on GitHub (or not visible). It is not a failure of the
// API, but the checks for the repository cannot run, so it is counted and warned about once.
func (s *onlineSession) noteMissing(what string, err error) {
	if !errors.Is(err, ErrGitHubNotFound) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedLocked(failNotFound, what, err)
}

// failedLocked counts a skipped lookup, logs it, and warns the first time its kind is seen.
func (s *onlineSession) failedLocked(kind failureKind, what string, err error) {
	s.skipped++
	s.countKindLocked(kind)
	if s.detail != nil {
		s.detail("online: skipped %s: %v", orSomething(what), err)
	}
	if !s.seen[kind] {
		s.seen[kind] = true
		if s.warn != nil {
			s.warn(failureMessage(kind, what, err))
		}
	}
}

// blockLocked makes the next lookups fail at once with err, until the time (zero: for the rest of the run).
func (s *onlineSession) blockLocked(err error, until time.Time) {
	s.blocked, s.blockUntil = err, until
}

// blockedErr returns the error to fail with when asking cannot work, or nil.
func (s *onlineSession) blockedErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked == nil {
		return nil
	}
	if !s.blockUntil.IsZero() && time.Now().After(s.blockUntil) {
		s.blocked, s.blockUntil, s.transient = nil, time.Time{}, 0
		return nil
	}
	return s.blocked
}

// record looks at the result of a lookup of what (a repository, for messages) and decides how to go on.
func (s *onlineSession) record(err error, what string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case err == nil, errors.Is(err, ErrGitHubNotFound):
		s.transient = 0
		return
	case errors.Is(err, ErrGitHubBranchScanUnavailable), errors.Is(err, errOnlineExcluded), errors.Is(err, errNoOwnerLookup), errors.Is(err, errOnlineNoFiles):
		return // Not a failure: the caller does it another way, or was asked not to
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded) && s.ctx.Err() != nil:
		s.stopLocked(err)
		return
	}
	kind := classifyFailure(err)
	s.failedLocked(kind, what, err)
	s.streakLocked(err, kind)
}

// recordList accounts for the request of the list of advisories like record does, except that a failure which leaves the
// run able to go on is not a skipped lookup and is not warned about: the lookups by action which follow it are, if they fail as well.
// A failure which blocks the run is counted and warned about like any other (failedLocked), as nothing follows to do it. A list which worked ends
// the streak of failures in a row, a list which is too long is an answer too, and a failure counts in the streak like any other.
func (s *onlineSession) recordList(err error) {
	if errors.Is(err, errAdvisoryListUnavailable) {
		return // nothing was asked
	}
	if errors.Is(err, ErrGitHubListTruncated) {
		err = nil // the API answered
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case err == nil, errors.Is(err, ErrGitHubNotFound):
		s.transient = 0
		return
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded) && s.ctx.Err() != nil:
		s.stopLocked(err)
		return
	}
	kind := classifyFailure(err)
	wasBlocked := s.blocked != nil
	s.streakLocked(err, kind)
	if s.blocked != nil && !wasBlocked {
		// The list stopped the lookups of the run (a rate limit, a rejected token, an unreachable host, a streak of
		// failures): the lookups which follow are skipped without a request, so this is the one that is warned about
		s.failedLocked(kind, "the advisories", err)
	}
}

// streakLocked counts the failure in the streak of failures in a row and blocks the run when asking again cannot work.
func (s *onlineSession) streakLocked(err error, kind failureKind) {
	var scoped *hostScopedError
	if errors.As(err, &scoped) {
		return // the failure of one host does not stop the lookups of the other one
	}
	switch kind {
	case failRateLimit:
		var rl *GitHubRateLimitError
		var until time.Time
		if errors.As(err, &rl) {
			until = rl.Reset
		}
		s.blockLocked(err, until)
	case failUnauthorized:
		s.blockLocked(err, time.Time{})
	case failForbidden:
		// A repository the token cannot see: the server answers, so it is no outage
		s.transient = 0
	case failNetwork:
		// The host does not resolve or refuses connections: it will not work for the next lookup either
		s.blockLocked(err, time.Time{})
	case failTimeout, failServer, failOther:
		s.transient++
		if s.transient >= maxConsecutiveGitHubFailures {
			s.blockLocked(fmt.Errorf("skipped after %d failed lookups in a row: %w", s.transient, err), time.Time{})
		}
	}
}

// call runs one request unless the session is stopped or blocked, and records its outcome. what names
// the repository for messages and may be empty.
func call[T any](s *onlineSession, fn func(ctx context.Context) (T, error)) (T, error) {
	return callFor(s, "", fn)
}

func callFor[T any](s *onlineSession, what string, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	if err := s.stopped(); err != nil {
		return zero, err
	}
	if err := s.blockedErr(); err != nil {
		s.mu.Lock()
		s.skipped++
		s.countKindLocked(classifyFailure(err))
		if s.detail != nil {
			s.detail("online: skipped %s: %v", orSomething(what), err)
		}
		s.mu.Unlock()
		return zero, err
	}
	v, err := fn(s.ctx)
	s.record(err, what)
	return v, err
}

// Repository returns the metadata of the repository.
func (s *onlineSession) Repository(owner, repo string) (*GitHubRepo, error) {
	if !s.allows(owner, repo) {
		return nil, errOnlineExcluded
	}
	what := owner + "/" + repo
	return s.repos.get(newRepoKey(owner, repo), func() (*GitHubRepo, error) {
		r, err := callFor(s, what, func(ctx context.Context) (*GitHubRepo, error) { return s.client.Repository(ctx, owner, repo) })
		s.noteMissing(what, err)
		return r, err
	})
}

// Tags returns the tags of the repository.
func (s *onlineSession) Tags(owner, repo string) (*tagIndex, error) {
	if !s.allows(owner, repo) {
		return nil, errOnlineExcluded
	}
	what := owner + "/" + repo
	return s.tags.get(newRepoKey(owner, repo), func() (*tagIndex, error) {
		l, err := callFor(s, what, func(ctx context.Context) (*GitHubTagList, error) { return s.client.Tags(ctx, owner, repo) })
		if err != nil {
			s.noteMissing(what, err)
			return nil, err
		}
		return newTagIndex(l), nil
	})
}

func (s *onlineSession) resolveRef(owner, repo string, ns GitHubRefNamespace, name string) (refResult, error) {
	if !s.allows(owner, repo) {
		return refResult{}, errOnlineExcluded
	}
	return s.refs.get(refKey{newRepoKey(owner, repo), ns, name}, func() (refResult, error) {
		return callFor(s, owner+"/"+repo, func(ctx context.Context) (refResult, error) {
			sha, found, err := s.client.ResolveRef(ctx, owner, repo, ns, name)
			return refResult{strings.ToLower(sha), found}, err
		})
	})
}

// TagCommit returns the commit the tag of the name points to. The boolean is false when the
// repository has no such tag.
func (s *onlineSession) TagCommit(owner, repo, name string) (string, bool, error) {
	idx, err := s.Tags(owner, repo)
	if err != nil {
		return "", false, err
	}
	if sha, ok := idx.byName[name]; ok {
		return sha, true, nil
	}
	if !idx.truncated {
		return "", false, nil
	}
	// The list is incomplete: ask for the tag itself
	r, err := s.resolveRef(owner, repo, GitHubRefTags, name)
	return r.sha, r.found, err
}

// BranchCommit returns the commit the branch of the name points to. The boolean is false when the
// repository has no such branch.
func (s *onlineSession) BranchCommit(owner, repo, name string) (string, bool, error) {
	r, err := s.resolveRef(owner, repo, GitHubRefHeads, name)
	return r.sha, r.found, err
}

// Branches returns at most limit branches of the repository. The first call for a repository decides
// how many are read.
func (s *onlineSession) Branches(owner, repo string, limit int) (*GitHubBranchList, error) {
	if !s.allows(owner, repo) {
		return nil, errOnlineExcluded
	}
	return s.branches.get(newRepoKey(owner, repo), func() (*GitHubBranchList, error) {
		return callFor(s, owner+"/"+repo, func(ctx context.Context) (*GitHubBranchList, error) {
			return s.client.Branches(ctx, owner, repo, limit)
		})
	})
}

// Compare compares two commits of the repository.
func (s *onlineSession) Compare(owner, repo, base, head string) (GitHubCompareStatus, error) {
	if !s.allows(owner, repo) {
		return "", errOnlineExcluded
	}
	return s.compares.get(compareKey{newRepoKey(owner, repo), base, head}, func() (GitHubCompareStatus, error) {
		return callFor(s, owner+"/"+repo, func(ctx context.Context) (GitHubCompareStatus, error) {
			return s.client.Compare(ctx, owner, repo, base, head)
		})
	})
}

// Advisories returns the security advisories of the action: those of the repository and, for an action in
// a subdirectory, those published under the full package name ("owner/repo/subpath"). The subpath may be
// empty. A client which cannot look up packages is asked for the repository only.
func (s *onlineSession) Advisories(owner, repo, subpath string) ([]GitHubAdvisory, error) {
	if !s.allows(owner, repo) {
		return nil, errOnlineExcluded
	}
	packages := []string{strings.ToLower(owner + "/" + repo)}
	if sub := strings.Trim(strings.ToLower(subpath), "/"); sub != "" {
		packages = append(packages, packages[0]+"/"+sub)
	}
	return s.advisories.get(strings.Join(packages, ","), func() ([]GitHubAdvisory, error) {
		if list, ok := s.actionAdvisories(); ok {
			return advisoriesOfPackages(list, packages), nil
		}
		return callFor(s, owner+"/"+repo, func(ctx context.Context) ([]GitHubAdvisory, error) {
			if pc, ok := s.client.(GitHubPackageAdvisoryClient); ok {
				return pc.AdvisoriesForPackages(ctx, packages)
			}
			return s.client.Advisories(ctx, owner, repo)
		})
	})
}

// actionAdvisories returns every advisory of the actions ecosystem, from one list request for the whole run instead
// of one request for each action. It reports false, and the caller asks about its action, when the client cannot list them,
// the run was stopped or is blocked, or the list failed or is longer than the client reads. A failed
// list is not counted as a skipped lookup: the lookups by action that follow are, if they fail too, and say why.
func (s *onlineSession) actionAdvisories() ([]GitHubAdvisory, bool) {
	lc, ok := s.client.(GitHubAdvisoryListClient)
	if !ok {
		return nil, false
	}
	list, err := s.advisoryList.get(struct{}{}, func() ([]GitHubAdvisory, error) {
		if err := s.stopped(); err != nil {
			return nil, err
		}
		if err := s.blockedErr(); err != nil {
			return nil, err
		}
		l, err := lc.ActionAdvisories(s.ctx)
		s.recordList(err)
		return l, err
	})
	if err != nil {
		s.advisoryListFailed(err)
		return nil, false
	}
	return list, true
}

// advisoryListFailed says once, with --verbose, why the advisories are asked for one action at a time.
func (s *onlineSession) advisoryListFailed(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.advisoryListNoted || s.detail == nil {
		return
	}
	s.advisoryListNoted = true
	s.detail("online: the list of advisories is not available (%v); asking about each action instead", err)
}

// advisoriesOfPackages returns the advisories of the list which have a vulnerability in one of the packages
// ("owner/repo" or "owner/repo/subpath", lower case), as the lookup by package does.
func advisoriesOfPackages(list []GitHubAdvisory, packages []string) []GitHubAdvisory {
	var ret []GitHubAdvisory
	for _, a := range list {
		for _, v := range a.Vulnerabilities {
			if slices.Contains(packages, strings.ToLower(v.Package)) {
				ret = append(ret, a)
				break
			}
		}
	}
	return ret
}

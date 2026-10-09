package jactionlint

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
)

// DefaultImpostorMaxBranches is how many branches of an action repository the impostor-commit rule
// compares a pinned commit with before it gives up without a verdict.
const DefaultImpostorMaxBranches = 1000

// maxImpostorRESTBranches is how many branches are compared one request at a time when the repository
// cannot be scanned with a batch (no token). Each branch costs a request.
const maxImpostorRESTBranches = 100

// RuleImpostorCommit reports an action pinned to a commit which is not part of the history of the
// repository itself. GitHub shares the commits of a repository and all its forks, so a commit
// which exists only in a fork can be addressed with the slug of the parent repository, and looks
// like any other hash-pinned action.
type RuleImpostorCommit struct {
	onlineUsesRule
}

// NewRuleImpostorCommit creates a new RuleImpostorCommit instance.
func NewRuleImpostorCommit(sess *onlineSession) *RuleImpostorCommit {
	return &RuleImpostorCommit{newOnlineUsesRule("impostor-commit", "Checks that actions pinned to a commit SHA use a commit of the action repository itself", sess)}
}

// commitOrigin tells whether the commit is part of the repository's own branches or tags.
type commitOrigin int

const (
	originUnknown commitOrigin = iota // No verdict: the commit is unknown, or too much would be needed to know
	originOwn
	originImpostor
)

// VisitWorkflowPost implements Pass.
func (r *RuleImpostorCommit) VisitWorkflowPost(*Workflow) error {
	limit := DefaultImpostorMaxBranches
	if v, ok := r.Config().ruleOptionNumber("impostor-commit", "max-branches"); ok {
		limit = int(v)
	}
	for _, s := range r.sites {
		if s.ref.RefKind != RefFullSHA {
			continue
		}
		sha := strings.ToLower(s.ref.Ref)
		origin, err := r.sess.commitOrigin(s.ref.Owner, s.ref.Repo, sha, limit)
		if err != nil {
			r.skipped(s, "the commit", err)
			continue
		}
		if origin == originImpostor {
			r.ReportIDf("impostor-commit", s.pos,
				"%s %q is pinned to commit %s, which is on no branch or tag of %s. it can come from a fork of the repository, where anyone can create a commit that looks like part of it (an impostor commit). pin a commit from the history of %s instead",
				s.what(), s.ref.Raw, shortSHA(sha), s.repoSlug(), s.repoSlug())
		}
	}
	return nil
}

// commitOrigin finds out if the commit is reachable from a tag or a branch of the repository itself.
//
// A tag points at it, or it is an ancestor of the head of the default branch, or of the head of one of
// the other branches (at most limit of them: when the repository has more, there is no verdict
// unless a branch matched). A commit which is none of these exists in the fork network only, or in
// refs which are not branches or tags such as pull request heads.
func (s *onlineSession) commitOrigin(owner, repo, sha string, limit int) (commitOrigin, error) {
	r, err := s.origins.get(originKey{newRepoKey(owner, repo), sha, limit}, func() (originResult, error) {
		o, err := s.findCommitOrigin(owner, repo, sha, limit)
		return originResult{o}, err
	})
	return r.origin, err
}

func (s *onlineSession) findCommitOrigin(owner, repo, sha string, limit int) (commitOrigin, error) {
	idx, err := s.Tags(owner, repo)
	if err != nil {
		return originUnknown, err
	}
	if len(idx.bySHA[sha]) > 0 {
		return originOwn, nil
	}
	// With more tags than were read, the commit may be a tagged release whose branch is deleted. That is
	// not an impostor, so only a branch can prove the commit is the repository's own then.
	verdict := originImpostor
	if idx.truncated {
		verdict = originUnknown
	}

	info, err := s.Repository(owner, repo)
	if err != nil {
		return originUnknown, err
	}
	// A 404 of a comparison means the commit shares no history with the branch, or that GitHub does not know
	// the commit at all. Only a comparison which gave an answer shows that the commit exists, and without one
	// there is no verdict: an unknown commit is not an impostor.
	var exists atomic.Bool
	reachable := func(branchHead string) (bool, error) {
		st, err := s.Compare(owner, repo, branchHead, sha)
		if err != nil {
			return false, err
		}
		exists.Store(true)
		return st == GitHubCompareBehind || st == GitHubCompareIdentical, nil
	}

	seen := map[string]bool{}
	if info.DefaultBranch != "" {
		head, found, err := s.BranchCommit(owner, repo, info.DefaultBranch)
		if err != nil {
			return originUnknown, err
		}
		if found {
			seen[head] = true
			ok, err := reachable(head)
			switch {
			case errors.Is(err, ErrGitHubNotFound):
				// No common history with the default branch: the other branches can still have the commit
			case err != nil:
				return originUnknown, err
			case ok:
				return originOwn, nil
			}
		}
	}

	// One batched request per 100 branches when the client can do it
	if scanner, ok := s.client.(GitHubBranchScanner); ok {
		scan, err := call(s, func(ctx context.Context) (GitHubBranchScan, error) {
			return scanner.CommitOnAnyBranch(ctx, owner, repo, sha, limit)
		})
		switch {
		case err == nil && scan.Found:
			return originOwn, nil
		case err == nil && scan.Complete && exists.Load():
			return verdict, nil
		case err == nil && scan.Complete:
			// The commit is on no branch, but nothing shows that it exists: compare the branches
		case err == nil && !scan.Complete:
			return originUnknown, nil
		}
		// Otherwise compare the branches one by one
	}

	limit = min(limit, maxImpostorRESTBranches)
	branches, err := s.Branches(owner, repo, limit)
	if err != nil {
		return originUnknown, err
	}
	var heads []string
	for _, b := range branches.Branches {
		head := strings.ToLower(b.SHA)
		if !seen[head] {
			seen[head] = true
			heads = append(heads, head)
		}
	}
	// Compare in small parallel batches and stop at the first branch that has the commit. The
	// session shares the HTTP concurrency limit, so this is polite.
	const batch = 6
	for len(heads) > 0 {
		n := min(batch, len(heads))
		results := make([]bool, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i, h := range heads[:n] {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i], errs[i] = reachable(h)
			}()
		}
		wg.Wait()
		for i := range results {
			switch {
			case errs[i] == nil && results[i]:
				return originOwn, nil
			case errs[i] != nil && !errors.Is(errs[i], ErrGitHubNotFound): // Not found: no common history, e.g. gh-pages
				return originUnknown, errs[i]
			}
		}
		heads = heads[n:]
	}
	if branches.Truncated || !exists.Load() {
		return originUnknown, nil
	}
	return verdict, nil
}

func init() {
	registerRules(
		RuleInfo{
			ID: "impostor-commit", Group: RuleGroupSecurity, Summary: "A hash-pinned action uses a commit which is not part of the repository's own history (it exists only in a fork).",
			DefaultLevel: SeverityError, Online: true, DocsAnchor: "check-impostor-commit",
			Options: []RuleOption{{Name: "max-branches", Kind: RuleOptionInt, Default: DefaultImpostorMaxBranches, Summary: "How many branches of the action repository a commit is compared with before giving up without a verdict. Without a token each branch costs a request, so at most 100 are compared."}},
		},
	)
	registerRuleFactory("impostor-commit", func(env *RuleEnv) []Rule {
		if env.online == nil || !env.config.RuleEnabled("impostor-commit") {
			return nil
		}
		return []Rule{NewRuleImpostorCommit(env.online)}
	})
}

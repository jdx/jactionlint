package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// This file has the lookup of the runs of a workflow and the jobs of a run, which the report of the durations of
// jobs (--suggest-timeouts) uses. It is the only use of the API of Actions runs.

// GitHubWorkflowRun is a run of a workflow.
type GitHubWorkflowRun struct {
	// ID is the ID of the run.
	ID int64 `json:"id"`
	// CreatedAt is when the run was created.
	CreatedAt time.Time `json:"created_at"`
	// Event is the event that triggered the run.
	Event string `json:"event,omitempty"`
}

// GitHubRunJob is a job of a run.
type GitHubRunJob struct {
	// Name is the name that GitHub shows: the ID of the job, or its name with the values of the matrix.
	Name string `json:"name"`
	// Conclusion is "success", "failure", "cancelled", "skipped" and the like. It is empty while the job runs.
	Conclusion string `json:"conclusion,omitempty"`
	// StartedAt and CompletedAt are when the job started on a runner and when it ended. The wait for a runner is
	// not in between.
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// GitHubRunJobs are the jobs of the latest attempt of a run.
type GitHubRunJobs struct {
	Jobs []GitHubRunJob `json:"jobs"`
	// Truncated is true when the run has more jobs than Jobs lists (a large matrix).
	Truncated bool `json:"truncated,omitempty"`
}

// GitHubRunsClient is an optional interface of a GitHubClient. A client which implements it can list the runs of a
// workflow and the jobs of a run, which --suggest-timeouts reads. Without it the report is not available.
type GitHubRunsClient interface {
	// WorkflowRuns returns up to limit of the most recent runs of the workflow file ("ci.yaml", the name of the
	// file in .github/workflows) that ended with success, newest first. ErrGitHubNotFound means the workflow is not
	// known to GitHub (a file that was never pushed, or a repository the token cannot read).
	WorkflowRuns(ctx context.Context, owner, repo, workflowFile string, limit int) ([]GitHubWorkflowRun, error)
	// RunJobs returns the jobs of the latest attempt of the run.
	RunJobs(ctx context.Context, owner, repo string, runID int64) (*GitHubRunJobs, error)
}

var errNoRunsLookup = errors.New("the GitHub client cannot look up workflow runs")

// maxWorkflowRuns bounds how many runs of a workflow are read: the API lists 100 on a page and the report reads
// the jobs of each run with a request of its own.
const maxWorkflowRuns = 100

type runsKey struct {
	repoKey
	workflow string
	limit    int
}

type runJobsKey struct {
	repoKey
	run int64
}

// WorkflowRuns returns the recent successful runs of the workflow file. It is one request for each workflow.
func (s *onlineSession) WorkflowRuns(owner, repo, workflowFile string, limit int) ([]GitHubWorkflowRun, error) {
	if !s.allows(owner, repo) {
		return nil, errOnlineExcluded
	}
	rc, ok := s.client.(GitHubRunsClient)
	if !ok {
		return nil, errNoRunsLookup
	}
	what := fmt.Sprintf("the runs of %s in %s/%s", workflowFile, owner, repo)
	return s.runs.get(runsKey{newRepoKey(owner, repo), workflowFile, limit}, func() ([]GitHubWorkflowRun, error) {
		return callFor(s, what, func(ctx context.Context) ([]GitHubWorkflowRun, error) {
			return rc.WorkflowRuns(ctx, owner, repo, workflowFile, limit)
		})
	})
}

// RunJobs returns the jobs of a run. It is one request for each run.
func (s *onlineSession) RunJobs(owner, repo string, runID int64) (*GitHubRunJobs, error) {
	if !s.allows(owner, repo) {
		return nil, errOnlineExcluded
	}
	rc, ok := s.client.(GitHubRunsClient)
	if !ok {
		return nil, errNoRunsLookup
	}
	what := fmt.Sprintf("the jobs of run %d in %s/%s", runID, owner, repo)
	return s.runJobs.get(runJobsKey{newRepoKey(owner, repo), runID}, func() (*GitHubRunJobs, error) {
		return callFor(s, what, func(ctx context.Context) (*GitHubRunJobs, error) {
			return rc.RunJobs(ctx, owner, repo, runID)
		})
	})
}

// validWorkflowFile reports whether the name is the name of a file of .github/workflows that can be put in the path of
// a request as it is.
func validWorkflowFile(s string) bool {
	if s == "" || len(s) > 255 || !(strings.HasSuffix(s, ".yml") || strings.HasSuffix(s, ".yaml")) {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return !strings.HasPrefix(s, ".")
}

// --- fixtures -------------------------------------------------------------------------------

// fixtureRuns are the recorded runs of one workflow file and the jobs of each run, by the ID of the run.
type fixtureRuns struct {
	Runs []GitHubWorkflowRun       `json:"runs"`
	Jobs map[string]*GitHubRunJobs `json:"jobs,omitempty"`
}

// WorkflowRuns implements GitHubRunsClient.
func (c *FixtureGitHubClient) WorkflowRuns(_ context.Context, owner, repo, workflowFile string, limit int) ([]GitHubWorkflowRun, error) {
	r, err := c.repo(owner, repo, "workflow runs")
	if err != nil {
		return nil, err
	}
	fr, ok := r.WorkflowRuns[workflowFile]
	if !ok {
		return nil, fmt.Errorf("workflow %q of %s/%s: %w", workflowFile, owner, repo, ErrGitHubNotFound)
	}
	runs := append([]GitHubWorkflowRun(nil), fr.Runs...)
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

// RunJobs implements GitHubRunsClient.
func (c *FixtureGitHubClient) RunJobs(_ context.Context, owner, repo string, runID int64) (*GitHubRunJobs, error) {
	r, err := c.repo(owner, repo, "jobs of a run")
	if err != nil {
		return nil, err
	}
	for _, fr := range r.WorkflowRuns {
		if j, ok := fr.Jobs[strconv.FormatInt(runID, 10)]; ok {
			cp := *j
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("run %d of %s/%s: %w", runID, owner, repo, ErrGitHubNotFound)
}

// WorkflowRuns implements GitHubRunsClient. The runs are the ones of the repository of the workflow, which is on the
// Server whatever the actions are: only the Server is asked.
func (c *fallbackGitHubClient) WorkflowRuns(ctx context.Context, owner, repo, workflowFile string, limit int) ([]GitHubWorkflowRun, error) {
	rc, ok := c.primary.(GitHubRunsClient)
	if !ok {
		return nil, errNoRunsLookup
	}
	return rc.WorkflowRuns(ctx, owner, repo, workflowFile, limit)
}

// RunJobs implements GitHubRunsClient on the Server, like WorkflowRuns.
func (c *fallbackGitHubClient) RunJobs(ctx context.Context, owner, repo string, runID int64) (*GitHubRunJobs, error) {
	rc, ok := c.primary.(GitHubRunsClient)
	if !ok {
		return nil, errNoRunsLookup
	}
	return rc.RunJobs(ctx, owner, repo, runID)
}

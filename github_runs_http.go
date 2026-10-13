//go:build !js

package jactionlint

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// WorkflowRuns implements GitHubRunsClient. The endpoint needs the permission "Actions: read" of a token and shows the
// runs of a public repository to everybody. The answers go through the cache like the others, so a run that is listed
// again within the TTL costs no request.
func (c *httpGitHubClient) WorkflowRuns(ctx context.Context, owner, repo, workflowFile string, limit int) ([]GitHubWorkflowRun, error) {
	if !validWorkflowFile(workflowFile) {
		return nil, fmt.Errorf("%w: workflow %q", errInvalidGitHubName, workflowFile)
	}
	limit = min(max(limit, 1), maxWorkflowRuns)
	path, err := repoURL(owner, repo, "actions", "workflows", workflowFile, "runs")
	if err != nil {
		return nil, err
	}
	q := url.Values{"status": {"success"}, "per_page": {strconv.Itoa(limit)}}
	var raw struct {
		Runs []struct {
			ID        int64     `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			Event     string    `json:"event"`
		} `json:"workflow_runs"`
	}
	if _, err := c.getJSON(ctx, path+"?"+q.Encode(), &raw); err != nil {
		return nil, err
	}
	runs := make([]GitHubWorkflowRun, 0, len(raw.Runs))
	for _, r := range raw.Runs {
		runs = append(runs, GitHubWorkflowRun{ID: r.ID, CreatedAt: r.CreatedAt, Event: r.Event})
	}
	return runs, nil
}

// RunJobs implements GitHubRunsClient. It reads the first 100 jobs of the latest attempt, and says when the run has more.
func (c *httpGitHubClient) RunJobs(ctx context.Context, owner, repo string, runID int64) (*GitHubRunJobs, error) {
	if runID <= 0 {
		return nil, fmt.Errorf("%w: run %d", errInvalidGitHubName, runID)
	}
	path, err := repoURL(owner, repo, "actions", "runs", strconv.FormatInt(runID, 10), "jobs")
	if err != nil {
		return nil, err
	}
	q := url.Values{"filter": {"latest"}, "per_page": {"100"}}
	var raw struct {
		Total int `json:"total_count"`
		Jobs  []struct {
			Name        string    `json:"name"`
			Conclusion  string    `json:"conclusion"`
			StartedAt   time.Time `json:"started_at"`
			CompletedAt time.Time `json:"completed_at"`
		} `json:"jobs"`
	}
	if _, err := c.getJSON(ctx, path+"?"+q.Encode(), &raw); err != nil {
		return nil, err
	}
	out := &GitHubRunJobs{Truncated: raw.Total > len(raw.Jobs)}
	for _, j := range raw.Jobs {
		out.Jobs = append(out.Jobs, GitHubRunJob{Name: j.Name, Conclusion: j.Conclusion, StartedAt: j.StartedAt, CompletedAt: j.CompletedAt})
	}
	return out, nil
}

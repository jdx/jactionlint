package runner

import (
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runner/eval"
)

// githubEnv returns the GITHUB_* and RUNNER_* variables of a step.
func githubEnv(in *Inputs, jr *jobRun, files fileSet) map[string]string {
	gh := jr.env.Contexts["github"].(map[string]any)
	s := func(k string) string { return eval.ToString(gh[k]) }
	r := jr.env.Contexts["runner"].(map[string]any)
	env := map[string]string{
		"GITHUB_ACTION":           "",
		"GITHUB_ACTIONS":          "true",
		"GITHUB_ACTOR":            s("actor"),
		"GITHUB_ACTOR_ID":         s("actor_id"),
		"GITHUB_API_URL":          s("api_url"),
		"GITHUB_BASE_REF":         s("base_ref"),
		"GITHUB_ENV":              files.env,
		"GITHUB_EVENT_NAME":       s("event_name"),
		"GITHUB_EVENT_PATH":       files.event,
		"GITHUB_GRAPHQL_URL":      s("graphql_url"),
		"GITHUB_HEAD_REF":         s("head_ref"),
		"GITHUB_JOB":              jr.pj.ID,
		"GITHUB_OUTPUT":           files.output,
		"GITHUB_PATH":             files.path,
		"GITHUB_REF":              s("ref"),
		"GITHUB_REF_NAME":         s("ref_name"),
		"GITHUB_REF_PROTECTED":    "false",
		"GITHUB_REF_TYPE":         s("ref_type"),
		"GITHUB_REPOSITORY":       s("repository"),
		"GITHUB_REPOSITORY_ID":    s("repository_id"),
		"GITHUB_REPOSITORY_OWNER": s("repository_owner"),
		"GITHUB_RETENTION_DAYS":   s("retention_days"),
		"GITHUB_RUN_ATTEMPT":      s("run_attempt"),
		"GITHUB_RUN_ID":           s("run_id"),
		"GITHUB_RUN_NUMBER":       s("run_number"),
		"GITHUB_SERVER_URL":       s("server_url"),
		"GITHUB_SHA":              s("sha"),
		"GITHUB_STATE":            files.state,
		"GITHUB_STEP_SUMMARY":     files.summary,
		"GITHUB_TRIGGERING_ACTOR": s("triggering_actor"),
		"GITHUB_WORKFLOW":         s("workflow"),
		"GITHUB_WORKFLOW_REF":     s("workflow_ref"),
		"GITHUB_WORKFLOW_SHA":     s("workflow_sha"),
		"GITHUB_WORKSPACE":        jr.work,
		"RUNNER_ARCH":             eval.ToString(r["arch"]),
		"RUNNER_ENVIRONMENT":      eval.ToString(r["environment"]),
		"RUNNER_NAME":             eval.ToString(r["name"]),
		"RUNNER_OS":               eval.ToString(r["os"]),
		"RUNNER_TEMP":             jr.rd.Temp,
		"RUNNER_TOOL_CACHE":       jr.rd.ToolCache,
		"GITHUB_SECRET_SOURCE":    "None",
	}
	for k, v := range env {
		if strings.HasPrefix(k, "GITHUB_") && v == "" && k != "GITHUB_ACTION" {
			delete(env, k)
		}
	}
	return env
}

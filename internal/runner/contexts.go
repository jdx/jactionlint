package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runner/eval"
)

// Inputs is everything the contexts are built from, apart from the progress of the run.
type Inputs struct {
	Repo         repoInfo
	Event        string
	EventData    map[string]any // github.event; synthesized when nil
	Workflow     string         // name of the workflow
	WorkflowPath string         // path relative to the root
	Inputs       map[string]any // workflow_dispatch / workflow_call inputs
	Vars         map[string]any
	Secrets      map[string]any
	Env          map[string]any // workflow-level env, filled by the caller
	Token        string
	Workspace    string
	Temp         string
	ToolCache    string
}

// githubContext builds the github context.
func githubContext(in *Inputs) map[string]any {
	ri := in.Repo
	ref, refName, refType := "", ri.Branch, "branch"
	switch {
	case ri.Branch != "":
		ref = "refs/heads/" + ri.Branch
	case ri.Tag != "":
		ref, refName, refType = "refs/tags/"+ri.Tag, ri.Tag, "tag"
	default:
		ref, refName = "refs/heads/"+ri.DefaultBranch, ri.DefaultBranch
	}
	headRef, baseRef := "", ""
	if strings.HasPrefix(in.Event, "pull_request") {
		headRef, baseRef = refName, ri.DefaultBranch
		ref, refName = "refs/pull/1/merge", "1/merge"
	}
	event := in.EventData
	if event == nil {
		event = syntheticEvent(in.Event, ri, ref, refName)
	}
	return map[string]any{
		"action":           "",
		"action_path":      "",
		"actor":            ri.Actor,
		"actor_id":         "0",
		"api_url":          apiURL(ri.Server),
		"base_ref":         baseRef,
		"env":              "",
		"event":            event,
		"event_name":       in.Event,
		"event_path":       "",
		"graphql_url":      apiURL(ri.Server) + "/graphql",
		"head_ref":         headRef,
		"job":              "",
		"output":           "",
		"path":             "",
		"ref":              ref,
		"ref_name":         refName,
		"ref_protected":    false,
		"ref_type":         refType,
		"repository":       ri.Owner + "/" + ri.Name,
		"repository_id":    "0",
		"repository_owner": ri.Owner,
		"repositoryurl":    ri.Server + "/" + ri.Owner + "/" + ri.Name + ".git",
		"retention_days":   "90",
		"run_attempt":      "1",
		"run_id":           "1",
		"run_number":       "1",
		"secret_source":    "None",
		"server_url":       ri.Server,
		"sha":              ri.SHA,
		"state":            "",
		"step_summary":     "",
		"token":            in.Token,
		"triggering_actor": ri.Actor,
		"workflow":         in.Workflow,
		"workflow_ref":     ri.Owner + "/" + ri.Name + "/" + in.WorkflowPath + "@" + ref,
		"workflow_sha":     ri.SHA,
		"workspace":        in.Workspace,
	}
}

func apiURL(server string) string {
	if server == "https://github.com" {
		return "https://api.github.com"
	}
	return server + "/api/v3"
}

func runnerContext(in *Inputs) map[string]any {
	os_ := map[string]string{"linux": "Linux", "darwin": "macOS", "windows": "Windows"}[runtime.GOOS]
	if os_ == "" {
		os_ = runtime.GOOS
	}
	arch := map[string]string{"amd64": "X64", "arm64": "ARM64", "386": "X86", "arm": "ARM"}[runtime.GOARCH]
	if arch == "" {
		arch = strings.ToUpper(runtime.GOARCH)
	}
	return map[string]any{
		"os":          os_,
		"arch":        arch,
		"name":        "jactionlint",
		"environment": "self-hosted",
		"temp":        in.Temp,
		"tool_cache":  in.ToolCache,
		"debug":       "",
	}
}

// BaseEnv returns the environment of the contexts which exist before any job starts.
func BaseEnv(in *Inputs) *eval.Env {
	return &eval.Env{HashFiles: func(p []string) (string, error) { return hashFiles(in.Workspace, p) }, Contexts: map[string]any{
		"github":  githubContext(in),
		"runner":  runnerContext(in),
		"vars":    orEmpty(in.Vars),
		"secrets": orEmpty(in.Secrets),
		"env":     orEmpty(in.Env),
		"inputs":  orEmpty(in.Inputs),
	}}
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// loadEvent reads --event-file.
func loadEvent(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return eval.Normalize(v).(map[string]any), nil
}

// syntheticEvent makes a small but well-formed payload for the event, so that expressions such as
// github.event.pull_request.head.ref or github.event.repository.default_branch have a value. --event-file replaces it.
func syntheticEvent(event string, ri repoInfo, ref, refName string) map[string]any {
	owner := map[string]any{"login": ri.Owner, "type": "User"}
	repo := map[string]any{
		"name": ri.Name, "full_name": ri.Owner + "/" + ri.Name, "owner": owner, "private": false,
		"default_branch": ri.DefaultBranch, "html_url": ri.Server + "/" + ri.Owner + "/" + ri.Name,
	}
	sender := map[string]any{"login": ri.Actor, "type": "User"}
	ev := map[string]any{"repository": repo, "sender": sender}
	switch {
	case event == "push":
		ev["ref"] = ref
		ev["before"] = strings.Repeat("0", 40)
		ev["after"] = ri.SHA
		ev["created"] = false
		ev["deleted"] = false
		ev["forced"] = false
		ev["head_commit"] = map[string]any{"id": ri.SHA, "message": "", "author": map[string]any{"name": ri.Actor}}
		ev["commits"] = []any{}
		ev["pusher"] = map[string]any{"name": ri.Actor}
	case strings.HasPrefix(event, "pull_request"):
		ev["action"] = "synchronize"
		ev["number"] = 1.0
		ev["pull_request"] = map[string]any{
			"number": 1.0, "title": "", "state": "open", "draft": false, "merged": false,
			"user": sender, "labels": []any{},
			"head": map[string]any{"ref": ri.Branch, "sha": ri.SHA, "repo": repo},
			"base": map[string]any{"ref": ri.DefaultBranch, "sha": ri.SHA, "repo": repo},
		}
	case event == "workflow_dispatch":
		ev["ref"] = ref
		ev["inputs"] = map[string]any{}
		ev["workflow"] = ""
	case event == "schedule":
		ev = map[string]any{"schedule": "0 0 * * *", "repository": repo}
	case event == "release":
		ev["action"] = "published"
		ev["release"] = map[string]any{"tag_name": ri.Tag, "name": ri.Tag, "prerelease": false, "draft": false}
	case event == "issues" || event == "issue_comment":
		ev["action"] = "opened"
		ev["issue"] = map[string]any{"number": 1.0, "title": "", "body": "", "labels": []any{}, "user": sender}
	}
	return ev
}

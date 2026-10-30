package runner

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"time"
)

// repoInfo is what the github context needs to know about the checkout.
type repoInfo struct {
	Root          string
	Owner, Name   string // from the remote "origin"; "local"/<dir name> when there is none
	SHA           string
	Branch        string // "" on a detached head
	Tag           string
	DefaultBranch string
	Actor         string
	Server        string // https://github.com or the host of origin
}

var reRemote = regexp.MustCompile(`(?i)^(?:https?://|ssh://)?(?:[^@/]+@)?([^/:]+)[:/]+([^/\s]+)/([^/\s]+?)(?:\.git)?/?$`)

func git(root string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readRepoInfo(root string) repoInfo {
	ri := repoInfo{Root: root, Owner: "local", Name: baseName(root), SHA: strings.Repeat("0", 40), Server: "https://github.com", DefaultBranch: "main"}
	if sha := git(root, "rev-parse", "HEAD"); sha != "" {
		ri.SHA = sha
	}
	ri.Branch = git(root, "symbolic-ref", "-q", "--short", "HEAD")
	if ri.Branch == "" {
		ri.Tag = git(root, "describe", "--tags", "--exact-match")
	}
	if url := git(root, "config", "--get", "remote.origin.url"); url != "" {
		if m := reRemote.FindStringSubmatch(url); m != nil {
			ri.Owner, ri.Name = m[2], m[3]
			if h := strings.ToLower(m[1]); h != "github.com" {
				ri.Server = "https://" + h
			}
		}
	}
	if def := git(root, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"); strings.HasPrefix(def, "origin/") {
		ri.DefaultBranch = strings.TrimPrefix(def, "origin/")
	}
	ri.Actor = git(root, "config", "user.name")
	if ri.Actor == "" {
		if u, err := user.Current(); err == nil {
			ri.Actor = u.Username
		}
	}
	if ri.Actor == "" {
		ri.Actor = "local"
	}
	return ri
}

func baseName(p string) string {
	p = strings.TrimRight(p, "/\\")
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}

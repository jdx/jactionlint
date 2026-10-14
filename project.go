package jactionlint

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Project represents one GitHub project. One Git repository corresponds to one project.
type Project struct {
	root   string
	config *Config
}

func absPath(path string) string {
	if p, err := filepath.Abs(path); err == nil {
		path = p
	}
	return path
}

// findProject creates new Project instance by finding a project which the given path belongs to.
// A project must be a repository (a ".git" directory or file, or a ".jj" directory of a Jujutsu workspace) and have a ".github/workflows" directory, an "action.yml" at its root
// or a ".github/actions" directory.
func findProject(path string) (*Project, error) {
	d := absPath(path)
	for {
		if hasProjectContent(d) {
			for _, marker := range repositoryMarkers {
				if _, err := os.Stat(filepath.Join(d, marker)); err == nil { // Note: .git may be a file
					return NewProject(d)
				}
			}
		}

		p := filepath.Dir(d)
		if p == d {
			return nil, nil
		}
		d = p
	}
}

// repositoryMarkers are the names which mark the root of a repository: ".git", and ".jj" for the workspaces of
// Jujutsu, a secondary one of which has no ".git" (actionlint#752).
var repositoryMarkers = []string{".git", ".jj"}

// hasProjectContent reports whether the directory has workflows or actions to check.
func hasProjectContent(d string) bool {
	for _, dir := range []string{filepath.Join(".github", "workflows"), filepath.Join(".github", "actions")} {
		if s, err := os.Stat(filepath.Join(d, dir)); err == nil && s.IsDir() {
			return true
		}
	}
	for _, name := range actionFileNames {
		if s, err := os.Stat(filepath.Join(d, name)); err == nil && !s.IsDir() {
			return true
		}
	}
	return false
}

// NewProject creates a new instance with a file path to the root directory of the repository.
// This function returns an error when failing to parse an jactionlint config file in the repository.
func NewProject(root string) (*Project, error) {
	c, err := loadRepoConfig(root)
	if err != nil {
		return nil, err
	}
	return &Project{root, c}, nil
}

// RootDir returns a root directory path of the GitHub project repository.
func (p *Project) RootDir() string {
	return p.root
}

// WorkflowsDir returns a ".github/workflows" directory path of the GitHub project repository.
// This method does not check if the directory exists.
func (p *Project) WorkflowsDir() string {
	return filepath.Join(p.root, ".github", "workflows")
}

// Knows returns true when the project knows the given file. When a file is included in the
// project's directory, the project knows the file.
func (p *Project) Knows(path string) bool {
	return isPathInDir(p.root, absPath(path))
}

// isPathInDir returns true when the path is the directory itself or is located under the directory.
// Both paths must be absolute. Unlike a string prefix check, a sibling directory sharing the same
// name prefix (e.g. "/work/repo-other" for "/work/repo") is not regarded as a descendant. It uses
// OS-specific path separators so it works on Windows too.
func isPathInDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false // Different volumes on Windows, etc.
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

// Config returns config object of the GitHub project repository. The config file was read from
// ".github/jactionlint.yaml" or ".github/jactionlint.yml" when this Project instance was created.
// When no config was found, this method returns nil.
func (p *Project) Config() *Config {
	// Note: Calling this method must be thread safe (#333)
	return p.config
}

// Projects represents set of projects. It caches Project instances which was created previously
// and reuses them.
type Projects struct {
	known []*Project
}

// NewProjects creates new Projects instance.
func NewProjects() *Projects {
	return &Projects{}
}

// At returns the Project instance which the path belongs to. It returns nil if no project is found
// from the path.
func (ps *Projects) At(path string) (*Project, error) {
	for _, p := range ps.known {
		if p.Knows(path) {
			return p, nil
		}
	}

	p, err := findProject(path)
	if err != nil {
		return nil, err
	}
	if p != nil {
		ps.known = append(ps.known, p)
	}

	return p, nil
}

// ActionFiles returns the paths of the metadata files (action.yml or action.yaml) of the local actions
// of the project: the one in the root, the ones under ".github/actions" and the ones which a local
// `uses: ./path` of a workflow or of another action refers to. The paths are absolute and sorted.
// It reads the workflows and the actions of the project on every call; the linter keeps what it read.
func (p *Project) ActionFiles() []string {
	return newCallGraph(p.root).actionPaths()
}

var reGitHubRemote = regexp.MustCompile(`(?i)github\.com[:/]+([^/\s]+)/([^/\s]+?)(?:\.git)?/?\s*$`)

// githubRepositoryFromGit is githubRepositoryOf for a clone whose .git is a file (a linked work tree, a submodule),
// read with the command git itself.
func githubRepositoryFromGit(root string) string {
	if s, err := os.Stat(filepath.Join(root, ".git")); err != nil || s.IsDir() {
		return "" // no clone, or a directory without a configuration
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := gitReadCommand(ctx, root, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return ""
	}
	if m := reGitHubRemote.FindStringSubmatch(strings.TrimSpace(string(out))); m != nil {
		return strings.ToLower(m[1] + "/" + m[2])
	}
	return ""
}

// githubRepositoryOf returns the "owner/repo" in lower case of the GitHub repository whose clone is at root, read
// from the url of the remote "origin" in .git/config, and "" when there is none.
func githubRepositoryOf(root string) string {
	b, err := os.ReadFile(filepath.Join(root, ".git", "config"))
	if err != nil {
		// A linked work tree and a submodule have a .git file, and the configuration is elsewhere: ask git
		return githubRepositoryFromGit(root)
	}
	inOrigin := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inOrigin = strings.HasPrefix(line, `[remote "origin"`)
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if inOrigin && ok && strings.TrimSpace(k) == "url" {
			if m := reGitHubRemote.FindStringSubmatch(strings.TrimSpace(v)); m != nil {
				return strings.ToLower(m[1] + "/" + m[2])
			}
		}
	}
	return ""
}

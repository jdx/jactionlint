package main

import (
	"path"
	"strings"
)

// Finding is one diagnostic from either tool, reduced to what the comparison needs.
type Finding struct {
	Repo string `json:"repo"`
	// File is slash-separated and relative to the repository root.
	File string `json:"file"`
	Line int    `json:"line"`
	// Rule is the zizmor audit name, or the jactionlint rule ID (the legacy kind for v1).
	Rule    string `json:"rule"`
	Message string `json:"message,omitempty"`
}

// normalizePath turns a path reported by a tool into a slash-separated path relative to the repo root.
func normalizePath(repoDir, p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	root := strings.TrimSuffix(strings.ReplaceAll(repoDir, "\\", "/"), "/")
	if root != "" {
		p = strings.TrimPrefix(p, root+"/")
	}
	p = path.Clean(p)
	return strings.TrimPrefix(p, "./")
}

// isWorkflowFile reports whether jactionlint checks the file by default. zizmor also audits action.yml,
// dependabot.yml and others, and those are reported as out of scope instead of as misses.
func isWorkflowFile(file string) bool {
	dir, base := path.Split(file)
	if dir != ".github/workflows/" {
		return false
	}
	ext := path.Ext(base)
	return ext == ".yml" || ext == ".yaml"
}

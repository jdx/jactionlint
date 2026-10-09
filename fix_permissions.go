package jactionlint

import (
	"regexp"
	"strings"
)

// readOnlyActions are the actions which are known to work with the "contents: read" permission only.
// They use the GITHUB_TOKEN to read the repository or do not use it at all (the artifact and cache
// services have their own credentials). An action which is not listed here may need more.
var readOnlyActions = map[string]bool{
	"actions/checkout":                       true,
	"actions/cache":                          true,
	"actions/upload-artifact":                true,
	"actions/download-artifact":              true,
	"actions/setup-node":                     true,
	"actions/setup-python":                   true,
	"actions/setup-go":                       true,
	"actions/setup-java":                     true,
	"actions/setup-dotnet":                   true,
	"actions/setup-ruby":                     true,
	"ruby/setup-ruby":                        true,
	"jdx/mise-action":                        true,
	"dtolnay/rust-toolchain":                 true,
	"swatinem/rust-cache":                    true,
	"docker/setup-buildx-action":             true,
	"docker/setup-qemu-action":               true,
	"astral-sh/setup-uv":                     true,
	"pnpm/action-setup":                      true,
	"oven-sh/setup-bun":                      true,
	"denoland/setup-deno":                    true,
	"hashicorp/setup-terraform":              true,
	"actions/upload-pages-artifact":          true,
	"extractions/setup-just":                 true,
	"taiki-e/install-action":                 true,
	"actions-rust-lang/setup-rust-toolchain": true,
}

// tokenUseRe matches the text of a workflow which uses the GITHUB_TOKEN or the GitHub API or CLI
// from a script, which usually needs more than "contents: read".
var tokenUseRe = regexp.MustCompile(`(?i)github_token|github\.token|\bgh_token\b|\bgh\s+[a-z]|\bgit\s+push\b|api\.github\.com|\bhub\s+[a-z]|secrets:\s*inherit`)

// fixMissingPermissions makes the fix which adds "permissions: contents: read" to the workflow. It
// returns nil when the workflow is not written in a shape the fix understands. The fix is unsafe
// unless the jobs without "permissions:" are known to need nothing but reading the repository:
// "contents: read" removes every other permission from the GITHUB_TOKEN, which breaks a job
// commenting on a pull request, publishing a package and so on.
func fixMissingPermissions(w *Workflow) *Fix {
	if _, ok := w.FindWorkflowCallEvent(); ok {
		// A reusable workflow gets at most the permissions its caller grants. Setting "contents: read"
		// in the callee makes GitHub reject every caller which grants less, and the callers are in
		// other files, so there is no value that is right to write. The finding stays without a fix.
		return nil
	}
	d := newSrcDoc(w.Source)
	if d == nil {
		return nil
	}
	if _, _, _, ok := d.topLevelKey("permissions"); ok {
		return nil // the parser did not see it, so the shape is not understood
	}
	line, indent, inline, ok := d.topLevelKey("on")
	if !ok {
		line, indent, inline, ok = d.topLevelKey(`"on"`)
	}
	if !ok {
		return nil
	}
	end, ok := d.entryEnd(line, indent, inline)
	if !ok {
		return nil
	}
	line = end
	// The new key goes at the indentation of the other keys of the root mapping
	pad := strings.Repeat(" ", indent)
	unit := strings.Repeat(" ", d.indentUnit())
	return &Fix{
		Description: "Add permissions: contents: read",
		Unsafe:      !onlyReadsRepository(w),
		Edits:       []TextEdit{d.insertAfterLine(line, pad+"permissions:", pad+unit+"contents: read")},
	}
}

// onlyReadsRepository reports whether the jobs which have no "permissions:" look like they work with
// the "contents: read" permission. It is a heuristic and errs on the side of saying no.
//
// What it looks at: the text of the file for the token and the GitHub API (tokenUseRe); a job that calls a
// reusable workflow; a job with container: or services:, whatever the image and the credentials, because
// GitHub pulls an image of ghcr.io with the GITHUB_TOKEN without the file naming it and "contents: read"
// removes "packages: read"; and every action, which has to be one of readOnlyActions. Not looked at:
// "docker login" in a script uses the token by name (tokenUseRe sees it), and an action which reads
// another repository takes a token input (named in the file).
func onlyReadsRepository(w *Workflow) bool {
	if tokenUseRe.Match(w.Source) {
		return false
	}
	for _, j := range w.Jobs {
		if j.Permissions != nil {
			continue
		}
		if j.WorkflowCall != nil {
			return false
		}
		if j.Container != nil || j.Services != nil {
			// GitHub pulls the images with the GITHUB_TOKEN when they are in ghcr.io, without the file saying so
			// and "contents: read" removes "packages: read"
			return false
		}
		for _, s := range j.Steps {
			a, ok := s.Exec.(*ExecAction)
			if !ok || a.Uses == nil {
				continue
			}
			u := ParseUses(a.Uses.Value)
			if u.Kind != UsesAction || !readOnlyActions[strings.ToLower(u.Owner+"/"+u.Repo)] {
				return false
			}
		}
	}
	return true
}

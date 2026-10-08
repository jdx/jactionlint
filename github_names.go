package jactionlint

import (
	"strings"
	"unicode"
)

// The names below end up in the path of a GitHub API request. A workflow from an untrusted pull request
// decides them, so they are validated strictly before any request is built: a value which does not pass
// is not looked up at all.

const (
	maxGitHubOwnerLen = 39
	maxGitHubRepoLen  = 100
	maxGitHubRefLen   = 255
)

// validGitHubOwner reports whether s can be a GitHub user or organization name: letters, digits and
// hyphens, at most 39 characters, no leading or trailing hyphen.
func validGitHubOwner(s string) bool {
	if s == "" || len(s) > maxGitHubOwnerLen || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// validGitHubRepo reports whether s can be a GitHub repository name: letters, digits, '.', '_' and '-',
// at most 100 characters. "." and ".." and every name starting with ".." are refused, because they are
// path segments with a meaning of their own.
func validGitHubRepo(s string) bool {
	if s == "" || len(s) > maxGitHubRepoLen || s == "." || strings.HasPrefix(s, "..") {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// validGitRefName reports whether s is acceptable as a branch or tag name (without "refs/heads/"): the
// rules of `git check-ref-format`, plus '%' (which a request would have to escape and which proxies may
// decode twice) and every control character, which git allows in no position.
func validGitRefName(s string) bool {
	if s == "" || len(s) > maxGitHubRefLen || s == "@" ||
		strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") || strings.HasSuffix(s, ".") ||
		strings.Contains(s, "..") || strings.Contains(s, "//") || strings.Contains(s, "@{") {
		return false
	}
	for _, r := range s {
		if r == ' ' || r == 0x7f || unicode.IsControl(r) || strings.ContainsRune("~^:?*[\\%", r) {
			return false
		}
	}
	for _, comp := range strings.Split(s, "/") {
		if strings.HasPrefix(comp, ".") || strings.HasSuffix(comp, ".lock") {
			return false
		}
	}
	return true
}

// validGitSHA reports whether s is a full commit SHA (SHA-1 or SHA-256), in either case.
func validGitSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

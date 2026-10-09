//go:build js

package jactionlint

import "time"

// newDefaultGitHubClient reports that the GitHub API is not reachable in the WebAssembly build.
func newDefaultGitHubClient(ttl time.Duration, notify func(string), debug func(string, ...any)) (GitHubClient, error) {
	return nil, errOnlineUnsupported
}

// onlineSupported is false where the GitHub API cannot be used.
const onlineSupported = false

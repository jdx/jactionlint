//go:build js

package jactionlint

// newDefaultGitHubClient reports that the GitHub API is not reachable in the WebAssembly build.
func newDefaultGitHubClient(defaultClientOptions) (GitHubClient, error) {
	return nil, errOnlineUnsupported
}

// onlineSupported is false where the GitHub API cannot be used.
const onlineSupported = false

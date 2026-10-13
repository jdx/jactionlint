//go:build js

package jactionlint

// defaultImageClient: the WebAssembly build has no network.
func defaultImageClient(GitHubClient, bool) ImageDigestClient { return nil }

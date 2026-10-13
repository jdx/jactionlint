//go:build !js

package jactionlint

// defaultImageClient is the client which asks the registries when the API client is the one built into jactionlint.
// The token goes to ghcr.io only when it is the token for the API of the public service.
func defaultImageClient(c GitHubClient, offline bool) ImageDigestClient {
	var token func() string
	if h, ok := c.(*httpGitHubClient); ok && h.base.Host == "api.github.com" {
		token = h.currentToken
	}
	return newRegistryClient(defaultRegistryEndpoints, token, offline)
}

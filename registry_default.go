//go:build !js

package jactionlint

// defaultImageClient is the client which asks the registries when the API client is the one built into jactionlint.
// The token goes to ghcr.io only when it is the token for the API of the public service.
func defaultImageClient(c GitHubClient, offline bool) ImageDigestClient {
	var token func() string
	if s, ok := c.(registryTokenSource); ok {
		token = s.registryToken()
	}
	return newRegistryClient(defaultRegistryEndpoints, token, offline)
}

// registryToken implements registryTokenSource: the token of the client is the one for ghcr.io when the API is the one of
// github.com, whose registry it is. The token of an Enterprise Server is not.
func (c *httpGitHubClient) registryToken() func() string {
	if c.base.Host == "api.github.com" {
		return c.currentToken
	}
	return nil
}

//go:build !js

package jactionlint

import (
	"context"
	"fmt"
	"net/url"
)

// Owner implements GitHubOwnerClient. GET /users/{login} answers for organizations as well, and needs no token.
func (c *httpGitHubClient) Owner(ctx context.Context, login string) (*GitHubOwner, error) {
	if !validGitHubOwner(login) {
		return nil, fmt.Errorf("%w: %q", errInvalidGitHubName, login)
	}
	var o GitHubOwner
	if _, err := c.getJSON(ctx, "/users/"+url.PathEscape(login), &o); err != nil {
		return nil, err
	}
	return &o, nil
}

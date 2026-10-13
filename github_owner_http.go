//go:build !js

package jactionlint

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// ownerLookupAPIHost is the API host whose answers about owners mean something. An Enterprise Server runs the actions
// of github.com through GitHub Connect, so its API has no such owner and the answer "not found" says nothing: the tests
// point it at their server.
var ownerLookupAPIHost = "api.github.com"

// Owner implements GitHubOwnerClient. GET /users/{login} answers for organizations as well, and needs no token. It is
// answered for api.github.com only.
func (c *httpGitHubClient) Owner(ctx context.Context, login string) (*GitHubOwner, error) {
	if !strings.EqualFold(c.base.Host, ownerLookupAPIHost) {
		return nil, errNoOwnerLookup
	}
	if !validGitHubOwner(login) {
		return nil, fmt.Errorf("%w: %q", errInvalidGitHubName, login)
	}
	var o GitHubOwner
	if _, err := c.getJSON(ctx, "/users/"+url.PathEscape(login), &o); err != nil {
		return nil, err
	}
	return &o, nil
}

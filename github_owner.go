package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// This file has the lookup of the owner of a repository, a user or an organization, which the repojacking rule
// needs to tell a name that is taken from one that anyone can register.

// GitHubOwner is a user or an organization.
type GitHubOwner struct {
	// Login is the name of the account as GitHub spells it.
	Login string `json:"login"`
	// Type is "User" or "Organization".
	Type string `json:"type,omitempty"`
}

// GitHubOwnerClient is an optional interface of a GitHubClient. A client which implements it can tell whether a
// user or an organization exists, which jactionlint asks for when the repository of an action does not exist, or
// has moved to another owner. Without it the repojacking check does nothing.
type GitHubOwnerClient interface {
	// Owner returns the user or organization of the login. It returns ErrGitHubNotFound when GitHub has no account
	// of that name: anyone can register it.
	Owner(ctx context.Context, login string) (*GitHubOwner, error)
}

var (
	errNoOwnerLookup    = errors.New("the GitHub client cannot look up owners")
	errInvalidOwnerName = errors.New("not a valid GitHub user or organization name")
)

// Owner returns the user or organization of the login, or ErrGitHubNotFound (wrapped) when there is none. The error
// is errNoOwnerLookup when the client cannot look owners up, which is no failure.
func (s *onlineSession) Owner(login string) (*GitHubOwner, error) {
	oc, ok := s.client.(GitHubOwnerClient)
	if !ok {
		return nil, errNoOwnerLookup
	}
	if !validGitHubOwner(login) {
		return nil, fmt.Errorf("owner %q: %w", login, errInvalidOwnerName)
	}
	return s.owners.get(strings.ToLower(login), func() (*GitHubOwner, error) {
		return callFor(s, "the owner "+login, func(ctx context.Context) (*GitHubOwner, error) { return oc.Owner(ctx, login) })
	})
}

// Owner implements GitHubOwnerClient.
func (c *FixtureGitHubClient) Owner(_ context.Context, login string) (*GitHubOwner, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	exists, ok := c.data.Owners[strings.ToLower(login)]
	if !ok {
		return nil, fmt.Errorf("the GitHub fixtures have no owner %q", login)
	}
	if !exists {
		return nil, fmt.Errorf("owner %q: %w", login, ErrGitHubNotFound)
	}
	return &GitHubOwner{Login: login}, nil
}

// Owner implements GitHubOwnerClient when the wrapped client does.
func (c *RecordingGitHubClient) Owner(ctx context.Context, login string) (*GitHubOwner, error) {
	oc, ok := c.inner.(GitHubOwnerClient)
	if !ok {
		return nil, errNoOwnerLookup
	}
	o, err := oc.Owner(ctx, login)
	record := func(exists bool) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.data.Owners == nil {
			c.data.Owners = map[string]bool{}
		}
		c.data.Owners[strings.ToLower(login)] = exists
	}
	switch {
	case err == nil:
		record(true)
	case isNotFound(err):
		record(false)
	}
	return o, err
}

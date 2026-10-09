package jactionlint

import (
	"os"
	"path/filepath"
	"testing"
)

// onlineFixtureClient returns a GitHubClient which serves testdata/online/github.json, so that the tests
// of the online rules need no network.
func onlineFixtureClient(t testing.TB) *FixtureGitHubClient {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "online", "github.json"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewFixtureGitHubClient(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

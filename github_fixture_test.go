//go:build !js

package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestRecordAndReplayRoundTrip(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	f.handle("/repos/actions/checkout/git/ref/heads/v4", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	f.json("/repos/actions/checkout/branches", fmt.Sprintf(`[{"name":"main","commit":{"sha":%q}}]`, testSHAv422))
	f.json("/repos/actions/checkout/compare/"+testSHAv422+"..."+testSHAv4, `{"status":"ahead"}`)
	f.json("/repos/gone/repo", `{}`)
	f.handle("/repos/gone/repo", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })

	real := f.client(httpGitHubOptions{})
	rec := NewRecordingGitHubClient(real)
	ctx := context.Background()

	repo, _ := rec.Repository(ctx, "actions", "checkout")
	tags, _ := rec.Tags(ctx, "actions", "checkout")
	sha, found, _ := rec.ResolveRef(ctx, "actions", "checkout", GitHubRefTags, "v4")
	_, notFound, _ := rec.ResolveRef(ctx, "actions", "checkout", GitHubRefHeads, "v4")
	branches, _ := rec.Branches(ctx, "actions", "checkout", 10)
	cmp, _ := rec.Compare(ctx, "actions", "checkout", testSHAv422, testSHAv4)
	_, cmpErr := rec.Compare(ctx, "actions", "checkout", testSHAv422, strings.Repeat("0", 40))
	advs, _ := rec.Advisories(ctx, "actions", "checkout")
	_, goneErr := rec.Repository(ctx, "gone", "repo")
	if !errors.Is(goneErr, ErrGitHubNotFound) || !errors.Is(cmpErr, ErrGitHubNotFound) {
		t.Fatalf("the real client should report missing things: %v, %v", goneErr, cmpErr)
	}

	data, err := rec.Fixtures()
	if err != nil {
		t.Fatal(err)
	}
	data2, _ := rec.Fixtures()
	if string(data) != string(data2) {
		t.Error("Fixtures() is not deterministic")
	}

	fx, err := NewFixtureGitHubClient(data)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := fx.Repository(ctx, "ACTIONS", "Checkout"); err != nil || !reflect.DeepEqual(got, repo) {
		t.Errorf("Repository: %+v, %v", got, err)
	}
	if got, err := fx.Tags(ctx, "actions", "checkout"); err != nil || len(got.Tags) != len(tags.Tags) {
		t.Errorf("Tags: %+v, %v", got, err)
	}
	if s, ok, err := fx.ResolveRef(ctx, "actions", "checkout", GitHubRefTags, "v4"); err != nil || s != sha || ok != found {
		t.Errorf("ResolveRef: %q, %v, %v", s, ok, err)
	}
	if _, ok, err := fx.ResolveRef(ctx, "actions", "checkout", GitHubRefHeads, "v4"); err != nil || ok != notFound {
		t.Errorf("ResolveRef of a missing ref: %v, %v", ok, err)
	}
	if got, err := fx.Branches(ctx, "actions", "checkout", 10); err != nil || !reflect.DeepEqual(got, branches) {
		t.Errorf("Branches: %+v, %v", got, err)
	}
	if got, err := fx.Compare(ctx, "actions", "checkout", testSHAv422, testSHAv4); err != nil || got != cmp {
		t.Errorf("Compare: %q, %v", got, err)
	}
	if _, err := fx.Compare(ctx, "actions", "checkout", testSHAv422, strings.Repeat("0", 40)); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("Compare of an unknown commit: %v", err)
	}
	if got, err := fx.Advisories(ctx, "actions", "checkout"); err != nil || len(got) != len(advs) {
		t.Errorf("Advisories: %+v, %v", got, err)
	}
	if _, err := fx.Repository(ctx, "gone", "repo"); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("a missing repository: %v", err)
	}
	if fx.Calls() == 0 {
		t.Error("Calls() counts nothing")
	}
}

func TestFixtureClientFailsOnUnrecordedData(t *testing.T) {
	fx, err := NewFixtureGitHubClient([]byte(`{"repos":{"o/r":{"repo":{"default_branch":"main"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	check := func(what string, err error) {
		t.Helper()
		if err == nil || errors.Is(err, ErrGitHubNotFound) || !strings.Contains(err.Error(), "fixtures have no") {
			t.Errorf("%s: want an error about missing fixtures but got %v", what, err)
		}
	}
	_, err = fx.Tags(ctx, "o", "r")
	check("tags", err)
	_, _, err = fx.ResolveRef(ctx, "o", "r", GitHubRefTags, "v1")
	check("ref", err)
	_, err = fx.Branches(ctx, "o", "r", 1)
	check("branches", err)
	_, err = fx.Compare(ctx, "o", "r", "a", "b")
	check("compare", err)
	_, err = fx.Advisories(ctx, "o", "r")
	check("advisories", err)
	_, err = fx.Repository(ctx, "x", "y")
	check("repository", err)

	if _, err := NewFixtureGitHubClient([]byte("{")); err == nil {
		t.Error("invalid JSON should be refused")
	}
}

func TestFixtureBranchesAreLimited(t *testing.T) {
	fx, _ := NewFixtureGitHubClient([]byte(`{"repos":{"o/r":{"branches":{"branches":[{"name":"a","sha":"1"},{"name":"b","sha":"2"}]}}}}`))
	l, err := fx.Branches(context.Background(), "o", "r", 1)
	if err != nil || len(l.Branches) != 1 || !l.Truncated {
		t.Errorf("Branches() = %+v, %v", l, err)
	}
}

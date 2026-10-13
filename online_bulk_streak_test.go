package jactionlint

import (
	"context"
	"testing"
)

// listClient fails every repository lookup with a server error and answers the list of advisories as told.
type listClient struct {
	GitHubClient
	listErr error
}

func (c listClient) Repository(context.Context, string, string) (*GitHubRepo, error) {
	return nil, &GitHubStatusError{Status: 500}
}

func (c listClient) ActionAdvisories(context.Context) ([]GitHubAdvisory, error) {
	return nil, c.listErr
}

func (c listClient) Advisories(context.Context, string, string) ([]GitHubAdvisory, error) {
	return nil, &GitHubStatusError{Status: 500}
}

// A list request which worked proves that the API is up, so it ends the streak of failures in a row that blocks the run (and one
// which failed with a server error is a failure like the others).
func TestBulkAdvisoriesCountInTheStreakOfFailures(t *testing.T) {
	fail := func(s *onlineSession, n int) {
		for i := 0; i < n; i++ {
			_, _ = s.Repository("o", string(rune('a'+i)))
		}
	}
	t.Run("a list that worked ends the streak", func(t *testing.T) {
		s := newOnlineSession(context.Background(), listClient{}, nil)
		fail(s, 2)
		if _, err := s.Advisories("o", "x", ""); err != nil {
			t.Fatal(err)
		}
		_, _ = s.Repository("o", "late")
		if err := s.blockedErr(); err != nil {
			t.Errorf("two failures, a list which worked and one more failure must not block the run: %v", err)
		}
	})
	t.Run("without the list the third failure blocks", func(t *testing.T) {
		s := newOnlineSession(context.Background(), listClient{}, nil)
		fail(s, 3)
		if s.blockedErr() == nil {
			t.Error("three failures in a row must block the run")
		}
	})
	t.Run("a list which failed counts as a failure", func(t *testing.T) {
		s := newOnlineSession(context.Background(), listClient{listErr: &GitHubStatusError{Status: 502}}, nil)
		fail(s, 2)
		_, _ = s.Advisories("o", "x", "") // the list fails, then the lookup of the action does
		if s.blockedErr() == nil {
			t.Error("the failed list is the third failure in a row")
		}
	})
	t.Run("a truncated list is an answer", func(t *testing.T) {
		s := newOnlineSession(context.Background(), listClient{listErr: ErrGitHubListTruncated}, nil)
		fail(s, 2)
		_, _ = s.Advisories("o", "x", "") // the list is too long: the API answered, so the lookup of the action starts a new streak
		if err := s.blockedErr(); err != nil {
			t.Errorf("a truncated list is no failure: %v", err)
		}
	})
}

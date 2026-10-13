package jactionlint

import (
	"context"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

// listClient fails every repository lookup with a server error and answers the list of advisories as told.
type listClient struct {
	GitHubClient
	listErr   error
	actionErr error // what the lookup of one action answers
	calls     *int  // counts the repository lookups which reached the client
}

func (c listClient) Repository(context.Context, string, string) (*GitHubRepo, error) {
	if c.calls != nil {
		*c.calls++
	}
	return nil, &GitHubStatusError{Status: 500}
}

func (c listClient) ActionAdvisories(context.Context) ([]GitHubAdvisory, error) {
	return nil, c.listErr
}

func (c listClient) Advisories(context.Context, string, string) ([]GitHubAdvisory, error) {
	return nil, c.actionErr
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
		s := newOnlineSession(context.Background(), listClient{listErr: &GitHubStatusError{Status: 502}, actionErr: &GitHubStatusError{Status: 500}}, nil)
		fail(s, 2)
		_, _ = s.Advisories("o", "x", "") // the list fails, then the lookup of the action does
		if s.blockedErr() == nil {
			t.Error("the failed list is the third failure in a row")
		}
	})
	t.Run("a truncated list is an answer", func(t *testing.T) {
		s := newOnlineSession(context.Background(), listClient{listErr: ErrGitHubListTruncated, actionErr: &GitHubStatusError{Status: 500}}, nil)
		fail(s, 2)
		_, _ = s.Advisories("o", "x", "") // the list is too long: the API answered, so the lookup of the action starts a new streak
		if err := s.blockedErr(); err != nil {
			t.Errorf("a truncated list is no failure: %v", err)
		}
	})
}

// A list request which fails in a way that blocks the run (a rate limit, a rejected token, an unreachable host) is warned about
// once like any other lookup which does that, and the next lookups are skipped. A failure which only means that each action is
// asked for, such as a 404 or a server error, is quiet and blocks nothing.
func TestBulkAdvisoriesListFailureWarnsWhenItBlocks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		block bool
		want  string
	}{
		{"rate limit", &GitHubRateLimitError{Reset: time.Now().Add(time.Hour)}, true, "rate limit"},
		{"rejected token", &GitHubStatusError{Status: 401}, true, "rejected the token"},
		{"connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, true, "refused"},
		{"not found", &GitHubStatusError{Status: 404}, false, ""},
		{"server error", &GitHubStatusError{Status: 500}, false, ""},
		{"forbidden", &GitHubStatusError{Status: 403}, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var warnings []string
			var calls int
			s := newOnlineSession(context.Background(), listClient{listErr: tc.err, calls: &calls}, func(m string) { warnings = append(warnings, m) })
			var details []string
			s.detail = func(format string, args ...any) { details = append(details, fmt.Sprintf(format, args...)) }
			advs, err := s.Advisories("o", "x", "")
			if tc.block {
				if len(warnings) != 1 || !strings.Contains(warnings[0], tc.want) {
					t.Errorf("want one warning about %q but got %q", tc.want, warnings)
				}
				if s.blockedErr() == nil {
					t.Error("the run should be blocked")
				}
				if _, err := s.Repository("o", "later"); err == nil || calls != 0 {
					t.Errorf("later lookups are skipped without a request: %v after %d requests", err, calls)
				}
				if err == nil {
					t.Error("the lookup of the action is skipped")
				}
				s.mu.Lock()
				n := s.skipped
				s.mu.Unlock()
				if n < 2 {
					t.Errorf("the list and the later lookups are counted as skipped, got %d", n)
				}
				if !strings.Contains(strings.Join(details, "\n"), "skipped the advisories") {
					t.Errorf("--verbose should name the list:\n%s", strings.Join(details, "\n"))
				}
				return
			}
			if len(warnings) != 0 || s.blockedErr() != nil || err != nil || len(advs) != 0 {
				t.Errorf("a list which only means asking per action is quiet: warnings=%q blocked=%v err=%v", warnings, s.blockedErr(), err)
			}
		})
	}
}

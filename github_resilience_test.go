//go:build !js

package jactionlint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// These tests cover how the HTTP client of the online checks behaves when GitHub or the network do
// not cooperate. They use httptest servers and fake clocks: no test waits for real or touches the
// network.

func okRepo(w http.ResponseWriter) {
	w.Header().Set("ETag", `"1"`)
	fmt.Fprint(w, `{"default_branch":"main"}`)
}

func TestRateLimitWithinBoundIsWaitedFor(t *testing.T) {
	f := newFakeGitHub(t)
	var n atomic.Int32
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(5*time.Second).Unix()))
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
			return
		}
		okRepo(w)
	})
	c := f.client(httpGitHubOptions{Token: "tok"})
	if _, err := c.Repository(context.Background(), "o", "r"); err != nil {
		t.Fatalf("a limit which resets in 5 seconds must be waited for: %v", err)
	}
	sl := f.sleptFor()
	if len(sl) != 1 || sl[0] < time.Second || sl[0] > 8*time.Second {
		t.Errorf("want one wait of about the time to the reset but got %v", sl)
	}
	if n.Load() != 2 {
		t.Errorf("want 2 requests but got %d", n.Load())
	}
}

func TestRateLimitBeyondBoundIsSkippedWithTheResetTime(t *testing.T) {
	f := newFakeGitHub(t)
	reset := time.Now().Add(10 * time.Minute)
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(reset.Unix()))
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	})
	c := f.client(httpGitHubOptions{Token: "tok"})
	_, err := c.Repository(context.Background(), "o", "r")
	if !errors.Is(err, ErrGitHubRateLimited) {
		t.Fatal(err)
	}
	want := reset.Local().Format("15:04:05")
	if !strings.Contains(err.Error(), "resets at "+want) || !(strings.Contains(err.Error(), "in 9m") || strings.Contains(err.Error(), "in 10m")) {
		t.Errorf("the message must name the reset time %s and the wait: %v", want, err)
	}
	if len(f.sleptFor()) != 0 {
		t.Errorf("a limit far away must not be waited for: %v", f.sleptFor())
	}
	if _, err := c.Repository(context.Background(), "o", "other"); !errors.Is(err, ErrGitHubRateLimited) || f.total() != 1 {
		t.Errorf("later lookups fail without a request: %v, %d requests", err, f.total())
	}
}

func TestMaxWaitZeroNeverWaits(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(3*time.Second).Unix()))
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	})
	zero := time.Duration(0)
	c := f.client(httpGitHubOptions{MaxWait: &zero})
	if _, err := c.Repository(context.Background(), "o", "r"); !errors.Is(err, ErrGitHubRateLimited) {
		t.Fatal(err)
	}
	if len(f.sleptFor()) != 0 {
		t.Errorf("waited: %v", f.sleptFor())
	}
}

func TestSecondaryRateLimitHonorsRetryAfter(t *testing.T) {
	f := newFakeGitHub(t)
	var n atomic.Int32
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"You have exceeded a secondary rate limit."}`)
			return
		}
		okRepo(w)
	})
	c := f.client(httpGitHubOptions{Token: "tok"})
	if _, err := c.Repository(context.Background(), "o", "r"); err != nil {
		t.Fatal(err)
	}
	if sl := f.sleptFor(); len(sl) != 1 || sl[0] < 2*time.Second || sl[0] > 4*time.Second {
		t.Errorf("want one wait of the Retry-After of 2 seconds but got %v", sl)
	}
}

func TestSecondaryRateLimitWithoutHintBacksOffThenGivesUp(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"message":"slow down"}`)
	})
	c := f.client(httpGitHubOptions{Token: "tok"})
	_, err := c.Repository(context.Background(), "o", "r")
	var rl *GitHubRateLimitError
	if !errors.As(err, &rl) || rl.Reset.IsZero() {
		t.Fatalf("want a rate limit error with a reset time but got %v", err)
	}
	if f.total() != 3 {
		t.Errorf("one request and two retries expected but got %d", f.total())
	}
	if sl := f.sleptFor(); len(sl) != 2 || sl[0] != 500*time.Millisecond || sl[1] != time.Second {
		t.Errorf("want the exponential backoff 500ms, 1s but got %v", sl)
	}
}

func TestServerErrorHonorsRetryAfterAndRecovers(t *testing.T) {
	f := newFakeGitHub(t)
	var n atomic.Int32
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		okRepo(w)
	})
	c := f.client(httpGitHubOptions{})
	if _, err := c.Repository(context.Background(), "o", "r"); err != nil {
		t.Fatal(err)
	}
	if sl := f.sleptFor(); len(sl) != 1 || sl[0] != time.Second {
		t.Errorf("want to wait the Retry-After of one second but got %v", sl)
	}
}

func TestStatusesWhichAreNotRetried(t *testing.T) {
	for _, st := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusNotImplemented, http.StatusBadRequest} {
		f := newFakeGitHub(t)
		f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(st)
			fmt.Fprint(w, `{"message":"no"}`)
		})
		c := f.client(httpGitHubOptions{})
		c.Repository(context.Background(), "o", "r")
		if f.total() != 1 || len(f.sleptFor()) != 0 {
			t.Errorf("%d: want one request and no wait but got %d requests, waits %v", st, f.total(), f.sleptFor())
		}
	}
}

func TestBackoffIsBoundedAndJittered(t *testing.T) {
	c, err := newHTTPGitHubClient(httpGitHubOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for attempt, ceiling := range []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second} {
		seen := map[time.Duration]bool{}
		for i := 0; i < 200; i++ {
			d := c.backoff(attempt)
			if d < ceiling/2 || d > ceiling {
				t.Fatalf("attempt %d: %s is outside [%s, %s]", attempt, d, ceiling/2, ceiling)
			}
			seen[d] = true
		}
		if len(seen) < 10 {
			t.Errorf("attempt %d: the backoff has no jitter: %d distinct values", attempt, len(seen))
		}
	}
}

func TestTimeoutIsRetriedOnceThenClassified(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		okRepo(w)
	})
	c := f.client(httpGitHubOptions{HTTPClient: &http.Client{Timeout: 30 * time.Millisecond}})
	_, err := c.Repository(context.Background(), "o", "r")
	if err == nil || classifyFailure(err) != failTimeout {
		t.Fatalf("want a timeout but got %v (%s)", err, classifyFailure(err))
	}
	if f.total() != 2 {
		t.Errorf("a timeout is repeated once: %d requests", f.total())
	}
}

// roundTripFunc replaces the transport so that a test can make any transport error happen.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransportErrorsAreClassifiedAndRetriedAsAppropriate(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		calls int32
		kind  failureKind
	}{
		{"dns", &net.DNSError{Err: "no such host", Name: "api.github.com", IsNotFound: true}, 1, failNetwork},
		{"refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, 1, failNetwork},
		// A dropped connection is transient like a server error: it must not skip the rest of the run
		{"reset", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, 2, failOther},
		{"eof", io.ErrUnexpectedEOF, 2, failOther},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var n atomic.Int32
			hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				n.Add(1)
				return nil, tc.err
			})}
			c, err := newHTTPGitHubClient(httpGitHubOptions{HTTPClient: hc})
			if err != nil {
				t.Fatal(err)
			}
			c.sleep = func(context.Context, time.Duration) error { return nil }
			_, err = c.Repository(context.Background(), "o", "r")
			if err == nil || n.Load() != tc.calls {
				t.Errorf("want an error after %d calls but got %v after %d", tc.calls, err, n.Load())
			}
			if k := classifyFailure(err); k != tc.kind {
				t.Errorf("kind %s. want %s (%v)", k, tc.kind, err)
			}
			if !strings.Contains(err.Error(), "could not reach the GitHub API") {
				t.Errorf("the error should say what failed: %v", err)
			}
		})
	}
}

func TestCanceledContextIsNotRetried(t *testing.T) {
	var n atomic.Int32
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n.Add(1)
		return nil, r.Context().Err()
	})}
	c, _ := newHTTPGitHubClient(httpGitHubOptions{HTTPClient: hc})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Repository(ctx, "o", "r"); !errors.Is(err, context.Canceled) || n.Load() > 1 {
		t.Errorf("%v after %d calls", err, n.Load())
	}
}

// --- tokens ---------------------------------------------------------------------------------

const secretToken = "ghp_SuperSecretTokenValue0123456789"

func TestTokenIsNeverPrinted(t *testing.T) {
	f := newFakeGitHub(t)
	// Servers can echo what they got. Nothing of it may reach the output.
	f.handle("/repos/o/echo", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"message":"bad request %s"}`, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	})
	f.handle("/repos/o/unauth", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"message":"Bad credentials %s"}`, secretToken)
			return
		}
		okRepo(w)
	})
	f.handle("/repos/o/limit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "9999")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"message":"secondary rate limit %s"}`, secretToken)
	})
	var out bytes.Buffer
	var mu sync.Mutex
	say := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(&out, format+"\n", args...)
	}
	c := f.client(httpGitHubOptions{
		Token: secretToken, TokenSource: "$GITHUB_TOKEN",
		Notify: func(m string) { say("notice: %s", m) },
		Debug:  say,
	})
	for _, repo := range []string{"echo", "unauth", "limit", "missing"} {
		_, err := c.Repository(context.Background(), "o", repo)
		say("error: %v", err)
	}
	// A transport error which carries the token (a proxy that logs the URL, or a custom transport)
	hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("connection to proxy failed (credentials %s)", secretToken)
	})}
	c2, _ := newHTTPGitHubClient(httpGitHubOptions{Token: secretToken, HTTPClient: hc, Debug: say, Retries: new(int)})
	_, err := c2.Repository(context.Background(), "o", "r")
	say("error: %v", err)
	if !strings.Contains(err.Error(), redactedText) {
		t.Errorf("the token should be replaced: %v", err)
	}

	if strings.Contains(out.String(), secretToken) {
		t.Errorf("the token was printed:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "GET") {
		t.Errorf("the test printed nothing:\n%s", out.String())
	}
}

func TestAPIURLWithCredentialsIsRefusedWithoutEchoingThem(t *testing.T) {
	_, err := newHTTPGitHubClient(httpGitHubOptions{BaseURL: "https://user:" + secretToken + "@ghe.example.com/api/v3"})
	if err == nil || strings.Contains(err.Error(), secretToken) {
		t.Errorf("want an error without the secret: %v", err)
	}
	_, err = newHTTPGitHubClient(httpGitHubOptions{BaseURL: "ftp://" + secretToken})
	if err == nil || strings.Contains(err.Error(), secretToken) {
		t.Errorf("want an error without the secret: %v", err)
	}
}

func TestTokenGoesOnlyToTheAPIHost(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the other host received a request with Authorization %q", r.Header.Get("Authorization"))
	}))
	defer other.Close()
	f := newFakeGitHub(t)
	f.handle("/repos/o/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/repos/o/moved", http.StatusMovedPermanently)
	})
	f.handle("/repos/o/same", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, f.srv.URL+"/repos/o/target", http.StatusMovedPermanently)
	})
	f.handle("/repos/o/target", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("a redirect inside the API host keeps the token")
		}
		okRepo(w)
	})
	c := f.client(httpGitHubOptions{Token: secretToken})
	_, err := c.Repository(context.Background(), "o", "moved")
	if err == nil || !strings.Contains(err.Error(), "another host") {
		t.Errorf("a redirect to another host must be refused: %v", err)
	}
	if _, err := c.Repository(context.Background(), "o", "same"); err != nil {
		t.Errorf("a redirect inside the host is fine: %v", err)
	}
	// An URL of another host is not requested at all, not even as a pagination link
	if _, err := c.get(context.Background(), other.URL+"/x"); err == nil {
		t.Error("an absolute URL of another host must be refused")
	}
	if _, err := c.get(context.Background(), "https://api.github.com.evil.example/x"); err == nil {
		t.Error("a look-alike host must be refused")
	}
}

func TestTokenIsNotSentOverPlainHTTPToARemoteHost(t *testing.T) {
	var notices []string
	c, err := newHTTPGitHubClient(httpGitHubOptions{BaseURL: "http://ghe.example.com/api/v3", Token: secretToken, Notify: func(m string) { notices = append(notices, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if c.currentToken() != "" || len(notices) != 1 || strings.Contains(notices[0], secretToken) {
		t.Errorf("token %q, notices %q", c.currentToken(), notices)
	}
	for _, base := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "http://[::1]:8080", "https://ghe.example.com/api/v3"} {
		c, err := newHTTPGitHubClient(httpGitHubOptions{BaseURL: base, Token: secretToken})
		if err != nil || c.currentToken() == "" {
			t.Errorf("%s: the token is fine here: %v", base, err)
		}
	}
}

func TestTokenDiscovery(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	files := map[string]string{"/tok": "  filetoken\n", "/bad": "two words", "/empty": "\n"}
	read := func(name string) ([]byte, error) {
		if v, ok := files[name]; ok {
			return []byte(v), nil
		}
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrNotExist}
	}
	ghCalls := 0
	gh := func(out string, err error) func(context.Context, string) (string, error) {
		return func(_ context.Context, host string) (string, error) {
			ghCalls++
			if host != "github.com" && host != "ghe.example.com" {
				t.Errorf("gh asked for %q", host)
			}
			return out, err
		}
	}
	tests := []struct {
		name       string
		d          tokenDiscovery
		want       string
		source     string
		notice     string
		wantGHCall int
	}{
		{"github token first", tokenDiscovery{dotCom: true, trusted: true, getenv: env("GITHUB_TOKEN", "a", "GH_TOKEN", "b")}, "a", "$GITHUB_TOKEN", "", 0},
		{"gh token", tokenDiscovery{dotCom: true, trusted: true, getenv: env("GH_TOKEN", "b")}, "b", "$GH_TOKEN", "", 0},
		{"named variable wins", tokenDiscovery{dotCom: true, trusted: true, tokenEnv: "MY_PAT", getenv: env("MY_PAT", "mine", "GITHUB_TOKEN", "a")}, "mine", "$MY_PAT", "", 0},
		{"named variable empty falls through", tokenDiscovery{dotCom: true, trusted: true, tokenEnv: "MY_PAT", getenv: env("GITHUB_TOKEN", "a")}, "a", "$GITHUB_TOKEN", "MY_PAT", 0},
		{"file wins over GITHUB_TOKEN", tokenDiscovery{dotCom: true, trusted: true, tokenFile: "/tok", getenv: env("GITHUB_TOKEN", "a")}, "filetoken", "the file /tok", "", 0},
		{"file with two words", tokenDiscovery{dotCom: true, trusted: true, tokenFile: "/bad", getenv: env("GITHUB_TOKEN", "a")}, "a", "$GITHUB_TOKEN", "exactly one token", 0},
		{"empty file", tokenDiscovery{dotCom: true, trusted: true, tokenFile: "/empty", getenv: env()}, "", "", "exactly one token", 0},
		{"missing file", tokenDiscovery{dotCom: true, trusted: true, tokenFile: "/nope", getenv: env("GITHUB_TOKEN", "a")}, "a", "$GITHUB_TOKEN", "could not read the token file /nope", 0},
		{"enterprise variables for other hosts", tokenDiscovery{host: "ghe.example.com", trusted: true, getenv: env("GITHUB_ENTERPRISE_TOKEN", "e", "GITHUB_TOKEN", "a")}, "e", "$GITHUB_ENTERPRISE_TOKEN", "", 0},
		{"enterprise variables are for other hosts only", tokenDiscovery{dotCom: true, trusted: true, getenv: env("GITHUB_ENTERPRISE_TOKEN", "e")}, "", "", "", 0},
		{"control characters", tokenDiscovery{dotCom: true, trusted: true, getenv: env("GITHUB_TOKEN", "a\r\nX-Evil: 1")}, "", "", "GITHUB_TOKEN does not hold a token", 0},
		{"gh when nothing else", tokenDiscovery{host: "github.com", dotCom: true, trusted: true, useGH: true, getenv: env(), runGH: gh("ghtoken\n", nil)}, "ghtoken", "gh auth token", "", 1},
		{"gh not used with an env token", tokenDiscovery{host: "github.com", dotCom: true, trusted: true, useGH: true, getenv: env("GITHUB_TOKEN", "a"), runGH: gh("ghtoken", nil)}, "a", "$GITHUB_TOKEN", "", 0},
		{"gh not installed", tokenDiscovery{host: "github.com", dotCom: true, trusted: true, useGH: true, getenv: env(), runGH: gh("", errors.New("not found"))}, "", "", "", 1},
		{"gh output that is no token", tokenDiscovery{host: "github.com", dotCom: true, trusted: true, useGH: true, getenv: env(), runGH: gh("You are not logged in", nil)}, "", "", "", 1},
		{"gh disabled", tokenDiscovery{host: "github.com", dotCom: true, trusted: true, useGH: false, getenv: env(), runGH: gh("ghtoken", nil)}, "", "", "", 0},
		{"gh for an enterprise host", tokenDiscovery{host: "ghe.example.com", trusted: true, useGH: true, getenv: env(), runGH: gh("ghe", nil)}, "ghe", "gh auth token", "", 1},
		{"a host chosen by the repository gets nothing", tokenDiscovery{dotCom: false, trusted: false, useGH: true, getenv: env("GITHUB_TOKEN", "a"), runGH: gh("ghtoken", nil)}, "", "", "config file of the repository", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ghCalls = 0
			tc.d.readFile = read
			got, notices := tc.d.discover(context.Background())
			if got.token != tc.want || got.source != tc.source {
				t.Errorf("got %q from %q. want %q from %q", got.token, got.source, tc.want, tc.source)
			}
			if tc.notice == "" && len(notices) != 0 || tc.notice != "" && !strings.Contains(strings.Join(notices, "\n"), tc.notice) {
				t.Errorf("notices %q. want one with %q", notices, tc.notice)
			}
			if ghCalls != tc.wantGHCall {
				t.Errorf("gh was run %d times. want %d", ghCalls, tc.wantGHCall)
			}
			for _, n := range notices {
				if len(got.token) > 3 && strings.Contains(n, got.token) {
					t.Errorf("notice with the token: %q", n)
				}
			}
		})
	}
}

func TestRunGHAuthTokenWithAFakeGh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1 $2 $3\" = \"auth token --hostname\" ] && [ \"$4\" = \"ghe.example.com\" ]; then echo ghp_fromgh; echo 'secret on stderr' >&2; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	out, err := runGHAuthToken(context.Background(), "ghe.example.com")
	if err != nil || strings.TrimSpace(out) != "ghp_fromgh" {
		t.Errorf("%q, %v", out, err)
	}
	if _, err := runGHAuthToken(context.Background(), "other.example.com"); err == nil {
		t.Error("a failing gh is an error")
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := runGHAuthToken(context.Background(), "ghe.example.com"); err == nil {
		t.Error("a missing gh is an error")
	}
}

func TestReadTokenFileLimited(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "t")
	os.WriteFile(p, []byte(strings.Repeat("a", 3*maxTokenFileBytes)), 0o600)
	b, err := readTokenFileLimited(p)
	if err != nil || len(b) != maxTokenFileBytes+1 {
		t.Errorf("a big file is read only up to the limit: %d, %v", len(b), err)
	}
	if _, err := readTokenFileLimited(dir); err == nil {
		t.Error("a directory is not a token file")
	}
}

func TestOnlineAPIDefaults(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	for _, tc := range []struct {
		name string
		env  func(string) string
		want string
	}{
		{"nothing", env(), ""},
		{"api url wins", env("GITHUB_API_URL", "https://ghe.example.com/api/v3", "GITHUB_SERVER_URL", "https://other.example.com", "GH_HOST", "x.example.com"), "https://ghe.example.com/api/v3"},
		{"server url", env("GITHUB_SERVER_URL", "https://ghe.example.com"), "https://ghe.example.com/api/v3"},
		{"server url with a port", env("GITHUB_SERVER_URL", "https://ghe.example.com:8443"), "https://ghe.example.com:8443/api/v3"},
		{"server url of github.com", env("GITHUB_SERVER_URL", "https://github.com"), ""},
		{"data residency", env("GITHUB_SERVER_URL", "https://acme.ghe.com"), "https://api.acme.ghe.com"},
		{"gh host", env("GH_HOST", "ghe.example.com"), "https://ghe.example.com/api/v3"},
		{"gh host github.com", env("GH_HOST", "github.com"), ""},
	} {
		if got := onlineAPIDefaults(tc.env); got != tc.want {
			t.Errorf("%s: %q. want %q", tc.name, got, tc.want)
		}
	}
}

// --- offline mode and the cache --------------------------------------------------------------

func TestOfflineModeUsesOnlyTheCache(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/cached", `{"default_branch":"main"}`)
	dir := t.TempDir()
	warm := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
	if _, err := warm.Repository(context.Background(), "o", "cached"); err != nil {
		t.Fatal(err)
	}
	f.json("/repos/o/missing", `{}`) // exists on the server but was never cached
	before := f.total()

	// A zero TTL would revalidate; the offline mode does not care about age
	c := f.client(httpGitHubOptions{CacheDir: dir, TTL: 0, Offline: true})
	for i := 0; i < 3; i++ {
		r, err := c.Repository(context.Background(), "o", "cached")
		if err != nil || r.DefaultBranch != "main" {
			t.Fatalf("a cached answer is served offline: %+v, %v", r, err)
		}
	}
	_, err := c.Repository(context.Background(), "o", "missing")
	if !errors.Is(err, ErrGitHubNotCached) || classifyFailure(err) != failNotCached {
		t.Errorf("want ErrGitHubNotCached but got %v", err)
	}
	if f.total() != before {
		t.Errorf("the offline mode made %d requests", f.total()-before)
	}
	if got := c.noticedAnon; got {
		t.Error("no request is made, so there is nothing to say about the token")
	}
	// Tags and the GraphQL scan do not reach the network either
	if _, err := c.Tags(context.Background(), "o", "cached"); !errors.Is(err, ErrGitHubNotCached) {
		t.Errorf("tags: %v", err)
	}
	if _, err := c.CommitOnAnyBranch(context.Background(), "o", "cached", strings.Repeat("a", 40), 10); !errors.Is(err, ErrGitHubBranchScanUnavailable) {
		t.Errorf("scan: %v", err)
	}
	if f.total() != before {
		t.Errorf("the offline mode made %d requests", f.total()-before)
	}
}

func TestOfflineModeServesCachedNotFound(t *testing.T) {
	f := newFakeGitHub(t) // Everything is a 404
	dir := t.TempDir()
	warm := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
	warm.Repository(context.Background(), "o", "gone")
	c := f.client(httpGitHubOptions{CacheDir: dir, Offline: true})
	if _, err := c.Repository(context.Background(), "o", "gone"); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("a cached 404 is a 404, not an uncached lookup: %v", err)
	}
}

func TestCacheIsKeyedByHostAndToken(t *testing.T) {
	f1 := newFakeGitHub(t)
	f1.json("/repos/o/r", `{"default_branch":"one"}`)
	f2 := newFakeGitHub(t)
	f2.json("/repos/o/r", `{"default_branch":"two"}`)
	dir := t.TempDir()
	r1, _ := f1.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour}).Repository(context.Background(), "o", "r")
	r2, err := f2.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour}).Repository(context.Background(), "o", "r")
	if err != nil || r1.DefaultBranch != "one" || r2.DefaultBranch != "two" {
		t.Errorf("two hosts must not share answers: %+v %+v %v", r1, r2, err)
	}

	// A private answer fetched with one token is not served to another token
	f3 := newFakeGitHub(t)
	f3.handle("/repos/o/private", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer tokA" {
			okRepo(w)
			return
		}
		http.NotFound(w, r)
	})
	dir = t.TempDir()
	if _, err := f3.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "tokA"}).Repository(context.Background(), "o", "private"); err != nil {
		t.Fatal(err)
	}
	if _, err := f3.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "tokB"}).Repository(context.Background(), "o", "private"); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("another token must not see the private answer: %v", err)
	}
	if _, err := f3.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Offline: true}).Repository(context.Background(), "o", "private"); !errors.Is(err, ErrGitHubNotCached) {
		t.Errorf("an anonymous client must not see the private answer: %v", err)
	}
}

func TestPublicAnswerOfAnonymousRunServesAClientWithAToken(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/pub", `{"default_branch":"main"}`)
	dir := t.TempDir()
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour}).Repository(context.Background(), "o", "pub"); err != nil {
		t.Fatal(err)
	}
	before := f.total()
	// GITHUB_TOKEN of a CI job changes on every run; the public answers should survive that
	c := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "fresh-token-of-this-job"})
	if r, err := c.Repository(context.Background(), "o", "pub"); err != nil || r.DefaultBranch != "main" {
		t.Fatal(r, err)
	}
	off := f.client(httpGitHubOptions{CacheDir: dir, Offline: true, Token: "another-token"})
	if r, err := off.Repository(context.Background(), "o", "pub"); err != nil || r.DefaultBranch != "main" {
		t.Fatal(r, err)
	}
	if f.total() != before {
		t.Errorf("the public answer should come from the cache: %d requests", f.total()-before)
	}

	// A "not found" of an anonymous run says nothing about what a token can see
	f.handle("/repos/o/hidden", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			okRepo(w)
			return
		}
		http.NotFound(w, r)
	})
	f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour}).Repository(context.Background(), "o", "hidden")
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "t"}).Repository(context.Background(), "o", "hidden"); err != nil {
		t.Errorf("a client with a token must ask again: %v", err)
	}
}

func TestConcurrencyOption(t *testing.T) {
	f := newFakeGitHub(t)
	var cur, peak atomic.Int32
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		n := cur.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(20 * time.Millisecond)
		cur.Add(-1)
		okRepo(w)
	})
	c := f.client(httpGitHubOptions{Concurrency: 2, CacheDir: t.TempDir()})
	c.cache = nil
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Repository(context.Background(), "o", "r")
		}()
	}
	wg.Wait()
	if peak.Load() != 2 {
		t.Errorf("at most 2 requests at once. peak %d", peak.Load())
	}
}

func TestNewDefaultClientDoesNotSendTheTokenToAHostTheRepositoryChose(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r", `{"default_branch":"main"}`)
	t.Setenv("GITHUB_TOKEN", secretToken)
	t.Setenv("GITHUB_API_URL", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // No gh
	var notices []string
	mk := func(trusted bool) GitHubClient {
		c, err := newDefaultGitHubClient(defaultClientOptions{
			Options: OnlineOptions{APIURL: f.srv.URL}, OptionsTrusted: trusted, Notify: func(m string) { notices = append(notices, m) },
		})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if _, err := mk(false).Repository(context.Background(), "o", "r"); err != nil {
		t.Fatal(err)
	}
	for _, a := range f.auth {
		if a != "" {
			t.Errorf("the token went to a host the repository chose: %q", a)
		}
	}
	if !strings.Contains(strings.Join(notices, "\n"), "config file of the repository") {
		t.Errorf("the user should be told why: %q", notices)
	}
	f.mu.Lock()
	f.auth = nil
	f.mu.Unlock()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if _, err := mk(true).Repository(context.Background(), "o", "r"); err != nil {
		t.Fatal(err)
	}
	if len(f.auth) == 0 || f.auth[0] != "Bearer "+secretToken {
		t.Errorf("a trusted host gets the token: %q", f.auth)
	}
}

func TestForbiddenForTheTokenIsRetriedWithoutIt(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/pub", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Resource not accessible by personal access token"}`)
			return
		}
		okRepo(w)
	})
	f.handle("/repos/o/blocked", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Repository access blocked"}`)
	})
	c := f.client(httpGitHubOptions{Token: secretToken})
	if r, err := c.Repository(context.Background(), "o", "pub"); err != nil || r.DefaultBranch != "main" {
		t.Fatalf("a public repository is readable without the token: %v", err)
	}
	if c.currentToken() == "" {
		t.Error("the token is kept for other requests")
	}
	var se *GitHubStatusError
	if _, err := c.Repository(context.Background(), "o", "blocked"); !errors.As(err, &se) || se.Status != 403 {
		t.Errorf("a repository which is blocked for everyone stays a 403: %v", err)
	}
}

func TestGHHostnameOfTheAPIHost(t *testing.T) {
	for api, want := range map[string]string{
		"api.github.com":   "github.com",
		"api.acme.ghe.com": "acme.ghe.com",
		"ghe.example.com":  "ghe.example.com",
		"api.example.com":  "api.example.com",
		"127.0.0.1:8080":   "127.0.0.1:8080",
	} {
		if got := ghHostname(api); got != want {
			t.Errorf("ghHostname(%q) = %q, want %q", api, got, want)
		}
	}
}

// A 403 is a per-repository miss: the server answered, so it ends a run of timeouts and server errors.
func TestForbiddenEndsTheStreakOfFailures(t *testing.T) {
	s := newOnlineSession(context.Background(), scanErrClient{errors.New("x")}, nil)
	timeout := &net.OpError{Op: "read", Err: os.ErrDeadlineExceeded}
	s.record(timeout, "a/a")
	s.record(timeout, "b/b")
	s.record(&GitHubStatusError{Status: http.StatusForbidden}, "c/c")
	s.record(timeout, "d/d")
	s.record(timeout, "e/e")
	if err := s.blockedErr(); err != nil {
		t.Errorf("four timeouts that are not in a row must not skip the rest: %v", err)
	}
	s.record(timeout, "f/f")
	if s.blockedErr() == nil {
		t.Error("three in a row do")
	}
}

// GraphQL requests wait for a rate limit and retry a server error like the REST requests do.
func TestGraphQLRetriesAndWaitsLikeREST(t *testing.T) {
	sha := strings.Repeat("a", 40)
	body := `{"data":{"repository":{"refs":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"name":"b","compare":{"status":"DIVERGED"}}]}}}}`
	for name, first := range map[string]func(w http.ResponseWriter){
		"server error": func(w http.ResponseWriter) { w.WriteHeader(http.StatusBadGateway) },
		"rate limit": func(w http.ResponseWriter) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(5*time.Second).Unix()))
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeGitHub(t)
			var n atomic.Int32
			f.handle("/graphql", func(w http.ResponseWriter, r *http.Request) {
				if n.Add(1) == 1 {
					first(w)
					return
				}
				fmt.Fprint(w, body)
			})
			scan, err := f.client(httpGitHubOptions{Token: "tok"}).CommitOnAnyBranch(context.Background(), "o", "r", sha, 10)
			if err != nil || n.Load() != 2 || !scan.Complete {
				t.Errorf("scan %+v, %v after %d requests", scan, err, n.Load())
			}
			if len(f.sleptFor()) != 1 {
				t.Errorf("want one wait: %v", f.sleptFor())
			}
		})
	}
}

// The anonymous request that asks again after a refused token has a rate limit of its own: it must not
// make the client think that the token is limited.
func TestAnonymousProbeDoesNotPoisonTheRateLimit(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/private", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Resource not accessible by personal access token"}`)
	})
	f.handle("/repos/o/public", func(w http.ResponseWriter, r *http.Request) { okRepo(w) })
	c := f.client(httpGitHubOptions{Token: secretToken})
	if _, err := c.Repository(context.Background(), "o", "private"); err == nil {
		t.Fatal("the private repository is refused")
	}
	if _, err := c.Repository(context.Background(), "o", "public"); err != nil {
		t.Errorf("the token still has quota: %v", err)
	}
}

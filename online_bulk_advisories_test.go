//go:build !js

package jactionlint

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// advisoryJSON is one advisory of the actions ecosystem as the API writes it.
func advisoryJSON(id, pkg, rng, patched string) string {
	return fmt.Sprintf(`{"ghsa_id":%q,"summary":"leaks a token","severity":"high","html_url":"https://github.com/advisories/%s","vulnerabilities":[`+
		`{"package":{"ecosystem":"actions","name":%q},"vulnerable_version_range":%q,"first_patched_version":%q}]}`, id, id, pkg, rng, patched)
}

// bulkServer serves what the API serves for the advisories of three actions, by package and as the whole list.
type bulkServer struct {
	f *fakeGitHub
	// mu guards the requests recorded.
	mu       sync.Mutex
	list     int // requests without "affects"
	byAction int // requests with "affects"
	// bulk answers the request without "affects"; nil serves the list in one page.
	bulk func(w http.ResponseWriter, r *http.Request)
}

var bulkAdvisories = []string{
	advisoryJSON("GHSA-aaaa-0000-0001", "o/vuln", "< 2", "2"),
	advisoryJSON("GHSA-aaaa-0000-0002", "O/Vuln/Sub", "< 5", "5"), // the case of the package is not the one of the workflow
	advisoryJSON("GHSA-aaaa-0000-0003", "other/elsewhere", "< 9", "9"),
}

const bulkConfig = "rules:\n  impostor-commit: off\n  ref-confusion: off\n  ref-version-mismatch: off\n  stale-action-refs: off\n  unpinned-uses: off\n  repojacking: off\n  archived-uses: off\n"

func newBulkServer(t *testing.T, bulk func(w http.ResponseWriter, r *http.Request)) *bulkServer {
	t.Helper()
	b := &bulkServer{f: newFakeGitHub(t), bulk: bulk}
	for _, repo := range []string{"o/vuln", "o/fine", "p/other"} {
		b.f.json("/repos/"+repo, `{"archived":false,"default_branch":"main"}`)
		b.f.json("/repos/"+repo+"/contents/action.yml", `{"type":"file","size":50,"encoding":"base64","content":"`+base64.StdEncoding.EncodeToString([]byte("name: x\nruns:\n  using: composite\n  steps: []\n"))+`"}`)
		b.f.json("/repos/"+repo+"/contents/sub/action.yml", `{"type":"file","size":50,"encoding":"base64","content":"`+base64.StdEncoding.EncodeToString([]byte("name: x\nruns:\n  using: composite\n  steps: []\n"))+`"}`)
		b.f.json("/repos/"+repo+"/tags", `[{"name":"v1","commit":{"sha":"`+strings.Repeat("1", 40)+`"}}]`)
	}
	b.f.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("ecosystem") != "actions" {
			t.Errorf("unexpected query %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		affects := q.Get("affects")
		b.mu.Lock()
		if affects == "" {
			b.list++
		} else {
			b.byAction++
		}
		b.mu.Unlock()
		if affects == "" {
			if b.bulk != nil {
				b.bulk(w, r)
				return
			}
			fmt.Fprint(w, "["+strings.Join(bulkAdvisories, ",")+"]")
			return
		}
		// What the API does for "affects": the advisories with one of the packages
		var out []string
		for _, a := range bulkAdvisories {
			for _, p := range strings.Split(affects, ",") {
				if strings.Contains(strings.ToLower(a), `"name":"`+p+`"`) {
					out = append(out, a)
					break
				}
			}
		}
		fmt.Fprint(w, "["+strings.Join(out, ",")+"]")
	})
	return b
}

func (b *bulkServer) counts() (list, byAction int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.list, b.byAction
}

func bulkRun(t *testing.T, b *bulkServer, extra ...string) (stdout, stderr string) {
	t.Helper()
	setOnlineEnv(t, b.f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: o/vuln@v1", "uses: o/vuln/sub@v1", "uses: o/fine@v1", "uses: p/other@v1"), bulkConfig)
	_, stdout, stderr = runOnlineCommand(t, append([]string{"--online", "--online-max-wait=0", "--rule-ids"}, append(extra, wf)...)...)
	return stdout, stderr
}

// wantVulnerable checks the findings, which must be the same whichever way the advisories were asked for.
func wantVulnerable(t *testing.T, stdout string) {
	t.Helper()
	if n := strings.Count(stdout, "[known-vulnerable-actions]"); n != 3 ||
		!strings.Contains(stdout, `"o/vuln@v1" (version 1) is affected by GHSA-aaaa-0000-0001`) ||
		!strings.Contains(stdout, `"o/vuln/sub@v1" (version 1) is affected by GHSA-aaaa-0000-0002`) {
		t.Errorf("want the findings of o/vuln (one) and o/vuln/sub (the advisory of the repository and its own), and none for the others:\n%s", stdout)
	}
}

func TestBulkAdvisoriesOneRequestForTheRun(t *testing.T) {
	b := newBulkServer(t, nil)
	stdout, stderr := bulkRun(t, b)
	wantVulnerable(t, stdout)
	if list, by := b.counts(); list != 1 || by != 0 {
		t.Errorf("want the one list request and none by action but got %d and %d", list, by)
	}
	if strings.Contains(stderr, "warning") {
		t.Errorf("unexpected warning:\n%s", stderr)
	}
}

func TestBulkAdvisoriesPaginated(t *testing.T) {
	var base string
	b := newBulkServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/advisories?ecosystem=actions&per_page=100&page=2>; rel="next"`, base))
			fmt.Fprint(w, "["+bulkAdvisories[0]+"]")
		case "2":
			w.Header().Set("Link", fmt.Sprintf(`<%s/advisories?ecosystem=actions&per_page=100&page=3>; rel="next"`, base))
			fmt.Fprint(w, "["+bulkAdvisories[1]+"]")
		default:
			fmt.Fprint(w, "["+bulkAdvisories[2]+"]")
		}
	})
	base = b.f.srv.URL
	stdout, _ := bulkRun(t, b)
	wantVulnerable(t, stdout)
	if list, by := b.counts(); list != 3 || by != 0 {
		t.Errorf("want three pages of the list and no request by action but got %d and %d", list, by)
	}
}

// A list that is longer than the cap is not used (a part of it could miss the advisory of an action): each action is asked for.
func TestBulkAdvisoriesTruncatedFallsBack(t *testing.T) {
	var base string
	b := newBulkServer(t, func(w http.ResponseWriter, r *http.Request) {
		next := 2
		fmt.Sscan(r.URL.Query().Get("page"), &next)
		if r.URL.Query().Get("page") != "" {
			next++
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/advisories?ecosystem=actions&per_page=100&page=%d>; rel="next"`, base, next))
		fmt.Fprint(w, "[]") // the advisories are on pages that are never read
	})
	base = b.f.srv.URL
	stdout, stderr := bulkRun(t, b, "--verbose")
	wantVulnerable(t, stdout)
	list, by := b.counts()
	if list != maxBulkAdvisoryPages {
		t.Errorf("the list should be read up to the cap of %d pages but %d were", maxBulkAdvisoryPages, list)
	}
	if by != 4 {
		t.Errorf("want one request for each of the four distinct sets of packages but got %d", by)
	}
	if !strings.Contains(stderr, "the list of advisories is not available") {
		t.Errorf("--verbose should say why:\n%s", stderr)
	}
}

func TestBulkAdvisoriesFailureFallsBack(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError, http.StatusBadRequest} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			b := newBulkServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"nope"}`)
			})
			stdout, stderr := bulkRun(t, b, "--verbose")
			wantVulnerable(t, stdout)
			if _, by := b.counts(); by != 4 {
				t.Errorf("want one request for each of the four distinct sets of packages but got %d", by)
			}
			if strings.Contains(stderr, "warning") {
				t.Errorf("the failure of the list is not a skipped lookup, as the others worked:\n%s", stderr)
			}
			if !strings.Contains(stderr, "the list of advisories is not available") {
				t.Errorf("--verbose should say why:\n%s", stderr)
			}
		})
	}
}

// When nothing works the run warns once, not once for the list and once for every action.
func TestBulkAdvisoriesEverythingFailingWarnsOnce(t *testing.T) {
	b := newBulkServer(t, nil)
	b.f.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Resource not accessible"}`)
	})
	_, stderr := bulkRun(t, b)
	if n := strings.Count(stderr, "warning:"); n != 1 {
		t.Errorf("want one warning but got %d:\n%s", n, stderr)
	}
}

// The list is cached like every answer: a second run asks for nothing within the TTL, and --online=cache can use it.
func TestBulkAdvisoriesAreCached(t *testing.T) {
	b := newBulkServer(t, nil)
	setOnlineEnv(t, b.f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: o/vuln@v1", "uses: o/fine@v1"), bulkConfig)
	_, first, _ := runOnlineCommand(t, "--online", "--rule-ids", wf)
	n := b.f.total()
	_, second, _ := runOnlineCommand(t, "--online", "--rule-ids", wf)
	if b.f.total() != n || first != second {
		t.Errorf("the second run should use the cache only (%d then %d requests)\n%s\n%s", n, b.f.total(), first, second)
	}
	_, third, stderr := runOnlineCommand(t, "--online=cache", "--rule-ids", wf)
	if third != first || strings.Contains(stderr, "warning") {
		t.Errorf("--online=cache should give the same findings from the cache:\n%s\n%s", third, stderr)
	}
	if !strings.Contains(first, "[known-vulnerable-actions]") {
		t.Errorf("missing finding:\n%s", first)
	}
}

// On a GitHub Enterprise Server with the fallback the list comes from github.com, as every advisory does, and the Server is not asked.
func TestBulkAdvisoriesGitHubComFallback(t *testing.T) {
	h := newTwoHosts(t, false)
	h.dotcom.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("affects") != "" {
			t.Errorf("github.com should be asked for the list only: %v", r.URL.Query())
		}
		fmt.Fprint(w, "["+advisoryJSON("GHSA-ghes-0000-0001", "actions/checkout", "< 5", "5")+"]")
	})
	h.ghes.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the Server should not be asked for advisories: %v", r.URL)
	})
	h.dotcom.json("/repos/actions/checkout/tags", `[{"name":"v4","commit":{"sha":"`+strings.Repeat("4", 40)+`"}}]`)
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), fallbackConfig)
	_, stdout, stderr := runOnlineCommand(t, "--online", "--online-github-com-fallback", "--rule-ids", wf)
	if !strings.Contains(stdout, "GHSA-ghes-0000-0001") || strings.Contains(stderr, "warning") {
		t.Errorf("want the finding without a warning:\n%s%s", stdout, stderr)
	}
	if h.dotcom.count("/advisories") != 1 || h.ghes.count("/advisories") != 0 {
		t.Errorf("requests: github.com %d, Server %d", h.dotcom.count("/advisories"), h.ghes.count("/advisories"))
	}
}

// A host which cannot list the advisories is asked for each action as before.
func TestBulkAdvisoriesFallbackHostWithoutTheListFallsBack(t *testing.T) {
	h := newTwoHosts(t, false)
	h.dotcom.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("affects") == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "["+advisoryJSON("GHSA-ghes-0000-0002", "actions/checkout", "< 5", "5")+"]")
	})
	h.dotcom.json("/repos/actions/checkout/tags", `[{"name":"v4","commit":{"sha":"`+strings.Repeat("4", 40)+`"}}]`)
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), fallbackConfig)
	_, stdout, stderr := runOnlineCommand(t, "--online", "--online-github-com-fallback", "--rule-ids", wf)
	if !strings.Contains(stdout, "GHSA-ghes-0000-0002") || strings.Contains(stderr, "warning") {
		t.Errorf("want the finding without a warning:\n%s%s", stdout, stderr)
	}
}

func TestHTTPClientActionAdvisories(t *testing.T) {
	f := newFakeGitHub(t)
	var requests int
	f.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if q.Get("ecosystem") != "actions" || q.Get("per_page") != "100" || q.Get("affects") != "" {
			t.Errorf("unexpected query %v", q)
		}
		fmt.Fprint(w, `[
		 {"ghsa_id":"GHSA-1","summary":"s","severity":"high","html_url":"u","vulnerabilities":[
		   {"package":{"ecosystem":"actions","name":"o/r"},"vulnerable_version_range":"< 2","first_patched_version":"2"},
		   {"package":{"ecosystem":"npm","name":"o/r"},"vulnerable_version_range":"< 9"}]},
		 {"ghsa_id":"GHSA-2","withdrawn_at":"2024-01-01T00:00:00Z","vulnerabilities":[]},
		 {"ghsa_id":"GHSA-1","vulnerabilities":[]},
		 {"ghsa_id":"GHSA-3","vulnerabilities":[{"package":{"ecosystem":"actions","name":"o/r/sub"},"vulnerable_version_range":"<= 1"}]}]`)
	})
	dir := t.TempDir()
	c := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
	advs, err := c.ActionAdvisories(context.Background())
	if err != nil || len(advs) != 2 || advs[0].ID != "GHSA-1" || advs[1].ID != "GHSA-3" {
		t.Fatalf("ActionAdvisories() = %+v, %v", advs, err)
	}
	if len(advs[0].Vulnerabilities) != 1 || advs[0].Vulnerabilities[0].FirstPatched != "2" {
		t.Errorf("only the vulnerabilities of the actions ecosystem are kept: %+v", advs[0])
	}
	// The cache of the disk serves the next client
	c2 := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
	if _, err := c2.ActionAdvisories(context.Background()); err != nil || requests != 1 {
		t.Errorf("the second client should use the cache: %v after %d requests", err, requests)
	}
}

func TestHTTPClientActionAdvisoriesTruncated(t *testing.T) {
	f := newFakeGitHub(t)
	var pages int
	f.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		pages++
		w.Header().Set("Link", fmt.Sprintf(`<%s/advisories?ecosystem=actions&per_page=100&page=%d>; rel="next"`, f.srv.URL, pages+1))
		fmt.Fprintf(w, `[{"ghsa_id":"GHSA-%d","vulnerabilities":[]}]`, pages)
	})
	c := f.client(httpGitHubOptions{})
	advs, err := c.ActionAdvisories(context.Background())
	if !errors.Is(err, ErrGitHubListTruncated) || advs != nil {
		t.Errorf("a list of more than %d pages is an error and gives nothing: %+v, %v", maxBulkAdvisoryPages, advs, err)
	}
	if pages != maxBulkAdvisoryPages {
		t.Errorf("read %d pages, want %d", pages, maxBulkAdvisoryPages)
	}
}

func TestHTTPClientActionAdvisoriesNeverPrintTheToken(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"message":"bad request %s"}`, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	})
	var out bytes.Buffer
	var mu sync.Mutex
	say := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(&out, format+"\n", args...)
	}
	c := f.client(httpGitHubOptions{Token: secretToken, Debug: say, Retries: new(int)})
	_, err := c.ActionAdvisories(context.Background())
	if err == nil {
		t.Fatal("want an error")
	}
	say("error: %v", err)
	if strings.Contains(out.String(), secretToken) {
		t.Errorf("the token was printed:\n%s", out.String())
	}
}

func TestAdvisoriesOfPackages(t *testing.T) {
	list := []GitHubAdvisory{
		{ID: "1", Vulnerabilities: []GitHubVulnerability{{Package: "O/R"}}},
		{ID: "2", Vulnerabilities: []GitHubVulnerability{{Package: "o/r/sub"}, {Package: "o/r"}}},
		{ID: "3", Vulnerabilities: []GitHubVulnerability{{Package: "o/r/other"}}},
		{ID: "4", Vulnerabilities: []GitHubVulnerability{{Package: "o/rr"}}},
		{ID: "5"},
	}
	ids := func(as []GitHubAdvisory) string {
		var s []string
		for _, a := range as {
			s = append(s, a.ID)
		}
		return strings.Join(s, ",")
	}
	for _, tc := range []struct {
		packages []string
		want     string
	}{
		{[]string{"o/r"}, "1,2"},
		{[]string{"o/r", "o/r/sub"}, "1,2"},
		{[]string{"o/r", "o/r/other"}, "1,2,3"},
		{[]string{"x/y"}, ""},
	} {
		if got := ids(advisoriesOfPackages(list, tc.packages)); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.packages, got, tc.want)
		}
	}
}

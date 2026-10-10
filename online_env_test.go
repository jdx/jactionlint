package jactionlint

import (
	"strings"
	"testing"
)

func TestParseOnlineEnv(t *testing.T) {
	tests := []struct {
		in   string
		set  bool
		off  bool
		mode OnlineMode
		err  bool
	}{
		{in: ""},
		{in: "  "},
		{in: "1", set: true},
		{in: "true", set: true},
		{in: "TRUE", set: true},
		{in: "on", set: true},
		{in: "cache", set: true, mode: OnlineModeCache},
		{in: "strict", set: true, mode: OnlineModeStrict},
		{in: "cache,strict", set: true, mode: OnlineModeCacheStrict},
		{in: "0", off: true},
		{in: "false", off: true},
		{in: "Off", off: true},
		{in: "no", off: true},
		{in: "maybe", err: true},
		{in: "cache,fast", err: true},
		{in: ",", err: true},
		{in: "+", err: true},
		{in: ", ,", err: true},
		{in: "true,", set: true},
	}
	for _, tc := range tests {
		set, off, mode, err := parseOnlineEnv(tc.in)
		if (err != nil) != tc.err {
			t.Errorf("%q: error %v, want error %v", tc.in, err, tc.err)
			continue
		}
		if err != nil {
			if !strings.Contains(err.Error(), "JACTIONLINT_ONLINE") {
				t.Errorf("%q: the error does not name the variable: %v", tc.in, err)
			}
			continue
		}
		if set != tc.set || off != tc.off || mode != tc.mode {
			t.Errorf("%q: got set=%v off=%v mode=%q, want set=%v off=%v mode=%q", tc.in, set, off, mode, tc.set, tc.off, tc.mode)
		}
	}
}

// requestsFor lints a workflow with the fake GitHub and returns how many requests were made.
func requestsWithEnv(t *testing.T, env string, cfg string, args ...string) (int, int, string) {
	t.Helper()
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	t.Setenv("JACTIONLINT_ONLINE", env)
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), cfg)
	status, _, stderr := runOnlineCommand(t, append(args, wf)...)
	return f.total(), status, stderr
}

func TestOnlineEnvTurnsTheOnlineChecksOn(t *testing.T) {
	if n, _, stderr := requestsWithEnv(t, "", ""); n != 0 {
		t.Errorf("an empty JACTIONLINT_ONLINE made %d requests\n%s", n, stderr)
	}
	if n, _, stderr := requestsWithEnv(t, "1", ""); n == 0 {
		t.Errorf("JACTIONLINT_ONLINE=1 made no request\n%s", stderr)
	}
	if n, _, stderr := requestsWithEnv(t, "true", ""); n == 0 {
		t.Errorf("JACTIONLINT_ONLINE=true made no request\n%s", stderr)
	}
}

func TestOnlineEnvModes(t *testing.T) {
	// cache never uses the network, with nothing cached it only warns
	n, status, stderr := requestsWithEnv(t, "cache", "")
	if n != 0 || status != ExitStatusSuccessNoProblem || !strings.Contains(stderr, "no cached answer") {
		t.Errorf("JACTIONLINT_ONLINE=cache: %d requests, status %d\n%s", n, status, stderr)
	}
}

func TestOnlineEnvOffBeatsTheConfigFile(t *testing.T) {
	if n, _, stderr := requestsWithEnv(t, "0", "online: true\n"); n != 0 {
		t.Errorf("JACTIONLINT_ONLINE=0 made %d requests although the config file says online: true\n%s", n, stderr)
	}
	// Without the variable the configuration file decides
	if n, _, stderr := requestsWithEnv(t, "", "online: true\n"); n == 0 {
		t.Errorf("online: true in the config file made no request\n%s", stderr)
	}
}

func TestOnlineEnvLosesToTheCommandLine(t *testing.T) {
	if n, _, stderr := requestsWithEnv(t, "1", "", "--no-online"); n != 0 {
		t.Errorf("--no-online made %d requests with JACTIONLINT_ONLINE=1\n%s", n, stderr)
	}
	if n, _, stderr := requestsWithEnv(t, "0", "", "--online"); n == 0 {
		t.Errorf("--online made no request with JACTIONLINT_ONLINE=0\n%s", stderr)
	}
	// An invalid value of the variable is not looked at when the flags decide
	if n, status, stderr := requestsWithEnv(t, "bogus", "", "--online"); n == 0 || status == ExitStatusInvalidCommandOption {
		t.Errorf("--online with JACTIONLINT_ONLINE=bogus: %d requests, status %d\n%s", n, status, stderr)
	}
}

func TestOnlineEnvInvalidValue(t *testing.T) {
	n, status, stderr := requestsWithEnv(t, "bogus", "")
	if status != ExitStatusInvalidCommandOption || n != 0 || !strings.Contains(stderr, "JACTIONLINT_ONLINE") {
		t.Errorf("status %d, %d requests, stderr:\n%s", status, n, stderr)
	}
}

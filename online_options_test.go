package jactionlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseOnlineOptions(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
online: true
online-options:
  mode: cache,strict
  api-url: https://ghe.example.com/api/v3/
  token-env: MY_TOKEN
  token-file: /run/secrets/gh
  allow: ["actions/*", "docker/login-action"]
  deny: ["corp/*"]
  cache-ttl: 30m
  max-rate-limit-wait: 0s
  retries: 4
  concurrency: 3
  gh-cli: false
`))
	if err != nil {
		t.Fatal(err)
	}
	o := cfg.OnlineOptions
	if o.Mode != OnlineModeCacheStrict || !o.Mode.Offline() || !o.Mode.Strict() {
		t.Errorf("mode %q", o.Mode)
	}
	if o.APIURL != "https://ghe.example.com/api/v3/" || o.TokenEnv != "MY_TOKEN" || o.TokenFile != "/run/secrets/gh" {
		t.Errorf("%+v", o)
	}
	if len(o.Allow) != 2 || len(o.Deny) != 1 {
		t.Errorf("%+v", o)
	}
	if o.CacheTTL == nil || *o.CacheTTL != 30*time.Minute || o.MaxRateLimitWait == nil || *o.MaxRateLimitWait != 0 {
		t.Errorf("durations: %v %v", o.CacheTTL, o.MaxRateLimitWait)
	}
	if o.Retries == nil || *o.Retries != 4 || o.Concurrency != 3 || o.GitHubCLI == nil || *o.GitHubCLI {
		t.Errorf("%+v", o)
	}
}

func TestParseOnlineOptionsErrors(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"unknown key":    {"online-options:\n  tokn-env: X\n", "did you mean \"token-env\""},
		"bad mode":       {"online-options:\n  mode: sometimes\n", "invalid online mode"},
		"bad url":        {"online-options:\n  api-url: ftp://x\n", "invalid GitHub API URL"},
		"url secret":     {"online-options:\n  api-url: https://u:p@x.example.com\n", "must not contain credentials"},
		"bad pattern":    {"online-options:\n  deny: [\"just-owner\"]\n", "owner/repo"},
		"bad wildcard":   {"online-options:\n  allow: [\"a/b[c]\"]\n", "only \"*\""},
		"negative ttl":   {"online-options:\n  cache-ttl: -1s\n", "must not be negative"},
		"many retries":   {"online-options:\n  retries: 99\n", "between 0 and"},
		"concurrency":    {"online-options:\n  concurrency: 500\n", "between 1 and"},
		"duration shape": {"online-options:\n  cache-ttl: soon\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tc.src))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want an error with %q but got %v", tc.want, err)
			}
			if err != nil && strings.Contains(err.Error(), "u:p") {
				t.Errorf("the credentials are repeated: %v", err)
			}
		})
	}
}

func TestOnlineOptionsMergeThroughExtends(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("base.yaml", "online-options:\n  deny: [\"corp/*\"]\n  retries: 1\n  token-env: BASE\n")
	p := write("jactionlint.yaml", "extends: [base.yaml]\nonline-options:\n  token-env: OWN\n")
	cfg, err := ReadConfigFile(p)
	if err != nil {
		t.Fatal(err)
	}
	o := cfg.OnlineOptions
	if o.TokenEnv != "OWN" || len(o.Deny) != 1 || o.Retries == nil || *o.Retries != 1 {
		t.Errorf("the file wins field by field: %+v", o)
	}
}

func TestOnlineModeOnTheConfigTurnsTheChecksOn(t *testing.T) {
	cfg, err := ParseConfig([]byte("online-options:\n  mode: cache\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !onlineEnabledBy(cfg) || onlineEnabledBy(&Config{}) || onlineEnabledBy(nil) {
		t.Error("only a mode or online: true turns the checks on")
	}
}

func TestParseOnlineMode(t *testing.T) {
	for in, want := range map[string]OnlineMode{
		"": OnlineModeDefault, "true": OnlineModeDefault, "ON": OnlineModeDefault,
		"cache": OnlineModeCache, "Strict": OnlineModeStrict,
		"cache,strict": OnlineModeCacheStrict, "strict+cache": OnlineModeCacheStrict, "strict, cache": OnlineModeCacheStrict,
	} {
		if got, err := ParseOnlineMode(in); err != nil || got != want {
			t.Errorf("%q: %q, %v. want %q", in, got, err, want)
		}
	}
	if _, err := ParseOnlineMode("cache,fast"); err == nil {
		t.Error("an unknown word is an error")
	}
}

func TestMatchRepoPattern(t *testing.T) {
	for _, tc := range []struct {
		pat, slug string
		want      bool
	}{
		{"actions/checkout", "actions/checkout", true},
		{"Actions/Checkout", "actions/CHECKOUT", true},
		{"actions/*", "actions/setup-node", true},
		{"actions/*", "actionsx/setup-node", false},
		{"*/setup-*", "actions/setup-node", true},
		{"actions/set", "actions/setup-node", false},
		{"*", "actions/x", false}, // not a valid pattern: one * does not cross the slash
		{"*/*", "any/thing", true},
	} {
		if got := matchRepoPattern(tc.pat, tc.slug); got != tc.want {
			t.Errorf("%s vs %s: %v", tc.pat, tc.slug, got)
		}
	}
}

func TestUserOwnedConfigs(t *testing.T) {
	l, err := NewLinter(os.Stderr, &LinterOptions{Config: &Config{}})
	if err != nil {
		t.Fatal(err)
	}
	if !l.defaultConfig.userOwned {
		t.Error("a config given to the library is the caller's")
	}
	p := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(p, []byte("online: false\n"), 0o644)
	l, err = NewLinter(os.Stderr, &LinterOptions{ConfigFile: p})
	if err != nil {
		t.Fatal(err)
	}
	if !l.defaultConfig.userOwned {
		t.Error("-config-file is the user's")
	}
	repo, err := ParseConfig([]byte("online: true\n"))
	if err != nil || repo.userOwned {
		t.Error("a parsed repository config is not user owned")
	}
}

// A bare -online on the command line replaces the mode of the configuration file.
func TestOnlineModeSetByTheCommandLineWins(t *testing.T) {
	cfg := OnlineOptions{Mode: OnlineModeCache}
	if got := cfg.overlay(OnlineOptions{}).Mode; got != OnlineModeCache {
		t.Errorf("nothing given on the command line keeps the mode of the file: %q", got)
	}
	if got := cfg.overlay(OnlineOptions{ModeSet: true}).Mode; got != OnlineModeDefault {
		t.Errorf("-online must force the default mode: %q", got)
	}
	if got := cfg.overlay(OnlineOptions{Mode: OnlineModeStrict, ModeSet: true}).Mode; got != OnlineModeStrict {
		t.Errorf("-online=strict must win: %q", got)
	}
}

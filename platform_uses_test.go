package jactionlint

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const sha40 = "08c6903cd8c0fde910a37f88322edcfb5dd907a8"

func platformFindings(t *testing.T, config string, uses ...string) []string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{".git", filepath.Join(".github", "workflows")} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if config != "" {
		if err := os.WriteFile(filepath.Join(root, ".github", "jactionlint.yaml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var b strings.Builder
	b.WriteString("on: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n")
	for _, u := range uses {
		b.WriteString("      - uses: " + u + "\n")
	}
	f := filepath.Join(root, ".github", "workflows", "ci.yaml")
	if err := os.WriteFile(f, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	cmd.Main([]string{"jactionlint", "--profile", "default", "--shellcheck=", "--pyflakes=", "--format", "json", f})
	var items []struct {
		ID      string `json:"id"`
		Line    int    `json:"line"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(out.Bytes(), &items); err != nil {
		t.Fatalf("not JSON: %v: %s %s", err, out.String(), errOut.String())
	}
	var ret []string
	for _, it := range items {
		ret = append(ret, it.ID+": "+it.Message)
	}
	sort.Strings(ret)
	return ret
}

func hasFinding(fs []string, substr string) bool {
	for _, f := range fs {
		if strings.Contains(f, substr) {
			return true
		}
	}
	return false
}

func TestURLUsesOnForgejo(t *testing.T) {
	for _, platform := range []string{"forgejo", "gitea"} {
		t.Run(platform, func(t *testing.T) {
			cfg := "platform: " + platform + "\n"
			fs := platformFindings(t, cfg,
				"https://code.forgejo.org/actions/checkout@"+sha40,
				"https://code.forgejo.org/actions/checkout/sub/dir@"+sha40,
				"https://example.com:3000/owner/repo@v1")
			if hasFinding(fs, "invalid-uses") {
				t.Errorf("valid URL forms are reported: %v", fs)
			}
			if len(fs) != 1 || !strings.HasPrefix(fs[0], "unpinned-uses: action \"https://example.com:3000/owner/repo@v1\" must be pinned") {
				t.Errorf("only the unpinned one is reported: %v", fs)
			}
		})
	}

	t.Run("malformed", func(t *testing.T) {
		fs := platformFindings(t, "platform: forgejo\n",
			"https://code.forgejo.org/actions/checkout",
			"https://code.forgejo.org",
			"https:///owner/repo@v1",
			"https://code.forgejo.org/owner@v1",
			"https://code.forgejo.org/owner/repo@")
		for _, want := range []string{
			`"https://code.forgejo.org/actions/checkout" in invalid format because ref is missing`,
			`"https://code.forgejo.org" in invalid format because owner is missing`,
			`"https:///owner/repo@v1" in invalid format because host is missing`,
			`"https://code.forgejo.org/owner@v1" in invalid format because owner is missing`,
			`"https://code.forgejo.org/owner/repo@" in invalid format because owner and repo and ref should not be empty`,
		} {
			if !hasFinding(fs, want) {
				t.Errorf("no finding with %q in %v", want, fs)
			}
		}
		if !hasFinding(fs, `with the platform "forgejo", "https://{host}/{owner}/{repo}[/{path}]@{ref}"`) {
			t.Errorf("the message does not name the form: %v", fs)
		}
	})

	t.Run("a ref policy is applied to owner/repo", func(t *testing.T) {
		cfg := "platform: forgejo\nrules:\n  unpinned-uses:\n    policies:\n      '*': hash-pin\n      myorg/*: ref-pin\n"
		fs := platformFindings(t, cfg, "https://git.example.com/myorg/tool@v2", "https://git.example.com/other/tool@v2")
		if len(fs) != 1 || !strings.Contains(fs[0], `"https://git.example.com/other/tool@v2"`) {
			t.Errorf("%v", fs)
		}
	})
}

func TestURLUsesStaysAnErrorOnGitHub(t *testing.T) {
	for _, cfg := range []string{"", "platform: github\n"} {
		fs := platformFindings(t, cfg, "https://code.forgejo.org/actions/checkout@"+sha40)
		if !hasFinding(fs, "invalid-uses: specifying action \"https://code.forgejo.org/actions/checkout@"+sha40+"\" in invalid format because owner and repo and ref should not be empty. available formats are \"{owner}/{repo}@{ref}\" or \"{owner}/{repo}/{path}@{ref}\"") {
			t.Errorf("config %q: %v", cfg, fs)
		}
	}
}

func TestPlatformConfig(t *testing.T) {
	if _, err := ParseConfig([]byte("platform: gitlab\n")); err == nil || !strings.Contains(err.Error(), `invalid value "gitlab" for "platform"`) {
		t.Errorf("an unknown platform: %v", err)
	}
	c, err := ParseConfig([]byte("platform: gitea\n"))
	if err != nil || !c.allowsURLUses() {
		t.Errorf("gitea: %v", err)
	}
	if c, _ := ParseConfig([]byte("profile: default\n")); c.allowsURLUses() {
		t.Error("the default platform is GitHub")
	}
	var nilConfig *Config
	if nilConfig.allowsURLUses() {
		t.Error("no config is GitHub")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), []byte("platform: forgejo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir, "child.yaml")
	if err := os.WriteFile(child, []byte("extends: [base.yaml]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, err := ReadConfigFile(child); err != nil || !c.allowsURLUses() {
		t.Errorf("extends: %v", err)
	}
}

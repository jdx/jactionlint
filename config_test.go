package actionlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v4"
)

func TestConfigParseSelfHostedRunnerOK(t *testing.T) {
	testCases := []struct {
		what   string
		input  string
		labels []string
	}{
		{
			what:   "empty config",
			input:  "",
			labels: nil,
		},
		{
			what:   "empty self-hosted-runner",
			input:  "self-hosted-runner:\n",
			labels: nil,
		},
		{
			what:   "null self-hosted-runner labels",
			input:  "self-hosted-runner:\n  labels:",
			labels: nil,
		},
		{
			what:   "empty self-hosted-runner labels",
			input:  "self-hosted-runner:\n  labels: []",
			labels: []string{},
		},
		{
			what:   "self-hosted-runner labels",
			input:  "self-hosted-runner:\n  labels: [foo, bar]",
			labels: []string{"foo", "bar"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.what, func(t *testing.T) {
			c, err := ParseConfig([]byte(tc.input))
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(c.SelfHostedRunner.Labels, tc.labels); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestConfigParseError(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{
			in:   `self-hosted-runner: 42`,
			want: `cannot construct`,
		},
		{
			in: `
paths:
  foo:
    ignore: foo+
`,
			want: `"ignore" must be a sequence node`,
		},
		{
			in: `
paths:
  foo:
    ignore: ['(foo']
`,
			want: `invalid regular expression "(foo" in "ignore"`,
		},
		{
			in: `
paths:
  foo:
    ignore: [{}]
`,
			want: `"ignore" items must be strings`,
		},
		{
			in: `
paths:
  foo:
    ignore: [[foo]]
`,
			want: `"ignore" items must be strings`,
		},
		{
			in: `
paths:
  foo:
    ignore: [42]
`,
			want: `"ignore" items must be strings`,
		},
		{
			in: `
paths:
  foo.{txt,xml:
`,
			want: `invalid glob pattern`,
		},
		{
			in:   "assume-default-permissions: foo\n",
			want: `invalid value "foo" for "assume-default-permissions"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			_, err := ParseConfig([]byte(tc.in))
			if err == nil {
				t.Fatal("no error occurred")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted error message %q to contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestConfigParseAssumeDefaultPermissions(t *testing.T) {
	tests := []struct {
		in   string
		want *string
	}{
		{in: "", want: nil},
		{in: "assume-default-permissions: restricted\n", want: strPtr("restricted")},
		{in: "assume-default-permissions: permissive\n", want: strPtr("permissive")},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			c, err := ParseConfig([]byte(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == nil {
				if c.AssumeDefaultPermissions != nil {
					t.Fatalf("want nil, got %q", *c.AssumeDefaultPermissions)
				}
				return
			}
			if c.AssumeDefaultPermissions == nil {
				t.Fatalf("want %q, got nil", *tc.want)
			}
			if *c.AssumeDefaultPermissions != *tc.want {
				t.Fatalf("want %q, got %q", *tc.want, *c.AssumeDefaultPermissions)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestConfigPathConfigIgnores(t *testing.T) {
	tests := []struct {
		input string
		msg   string
		want  bool
	}{
		{
			input: ``,
			msg:   "this is test",
			want:  false,
		},
		{
			input: `ignore: []`,
			msg:   "this is test",
			want:  false,
		},
		{
			input: `ignore: ['(is )+']`,
			msg:   "this is test",
			want:  true,
		},
		{
			input: `ignore: ['does not match', '(is )+']`,
			msg:   "this is test",
			want:  true,
		},
		{
			input: `ignore: ['does not match', 'does not match 2']`,
			msg:   "this is test",
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.input+"_"+tc.msg, func(t *testing.T) {
			var c PathConfig
			if err := yaml.Unmarshal([]byte(tc.input), &c); err != nil {
				t.Fatal(err)
			}
			have := c.Ignore.Match(&Error{Message: tc.msg})
			if tc.want != have {
				t.Fatalf("wanted %v but got %v for message %q and input %q", tc.want, have, tc.msg, tc.input)
			}
		})
	}
}

func TestConfigIgnoreErrors(t *testing.T) {
	src := `
paths:
  .github/workflows/**/*.yaml:
    ignore: [xxx]
  .github/workflows/*.yaml:
    ignore: [yyy]
  .github/workflows/a/*.yaml:
    ignore: [zzz]
  .github/workflows/*/b.yaml:
    ignore: [uuu]
  .github/workflows/a/b.yaml:
    ignore: [vvv]
  .github/workflows/**/x.yaml:
    ignore: [www]
  .github/workflows/**/*.{yml,yaml}:
    ignore: [ttt]
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		msg  string
		want bool
	}{
		{"foo.yaml", "xxx", false},
		{".github/workflows/a.yaml", "xxx", true},
		{".github/workflows/a/b.yaml", "xxx", true},
		{".github/workflows/a/b/c/d/e/f/g/h.yaml", "xxx", true},
		{".github/workflows/a.yaml", "yyy", true},
		{".github/workflows/a/b.yaml", "yyy", false},
		{".github/workflows/a/b.yaml", "zzz", true},
		{".github/workflows/a/a.yaml", "zzz", true},
		{".github/workflows/b/b.yaml", "zzz", false},
		{".github/workflows/a/b.yaml", "uuu", true},
		{".github/workflows/b/b.yaml", "uuu", true},
		{".github/workflows/a/a.yaml", "uuu", false},
		{".github/workflows/a/b.yaml", "vvv", true},
		{".github/workflows/b/b.yaml", "vvv", false},
		{".github/workflows/a/a.yaml", "vvv", false},
		{".github/workflows/x.yaml", "www", true},
		{".github/workflows/a/x.yaml", "www", true},
		{".github/workflows/a/b/x.yaml", "www", true},
		{".github/workflows/a/b/c/x.yaml", "www", true},
		{".github/workflows/a/b.yaml", "this is not ignored", false},
		{".github/workflows/a.yml", "xxx", false},
		{".github/workflows/a.yml", "ttt", true},
	}

	for _, tc := range tests {
		var ignored bool
		for _, c := range cfg.PathConfigs(tc.path) {
			if c.Ignore.Match(&Error{Message: tc.msg}) {
				ignored = true
				break
			}
		}
		if ignored != tc.want {
			want, have := "not be ignored", "was ignored"
			if tc.want {
				want, have = "be ignored", "was not ignored"
			}
			t.Fatalf("error message %q with path %q should %s but actually %s", tc.msg, tc.path, want, have)
		}
	}
}

func TestConfigReadFileOK(t *testing.T) {
	p := filepath.Join("testdata", "config", "ok.yml")
	c, err := ReadConfigFile(p)
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{"foo", "bar"}
	if diff := cmp.Diff(c.SelfHostedRunner.Labels, labels); diff != "" {
		t.Fatal(diff)
	}
}

func TestConfigReadFileReadError(t *testing.T) {
	p := filepath.Join("testdata", "config", "does-not-exist.yml")
	_, err := ReadConfigFile(p)
	if err == nil {
		t.Fatal("error did not occur")
	}
	msg := err.Error()
	if !strings.Contains(msg, "could not read config file") {
		t.Fatalf("unexpected error message: %q", msg)
	}
}

func TestConfigReadFileParseError(t *testing.T) {
	p := filepath.Join("testdata", "config", "broken.yml")
	_, err := ReadConfigFile(p)
	if err == nil {
		t.Fatal("error did not occur")
	}
	msg := err.Error()
	if !strings.Contains(msg, "could not parse config file") {
		t.Fatalf("unexpected error message: %q", msg)
	}
}

func TestConfigGenerateDefaultConfigFileOK(t *testing.T) {
	f := filepath.Join(t.TempDir(), "default-config-for-test.yml")
	if err := writeDefaultConfigFile(f); err != nil {
		t.Fatal(err)
	}
	c, err := ReadConfigFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.SelfHostedRunner.Labels) != 0 {
		t.Fatal(c.SelfHostedRunner.Labels)
	}
	if c.ConfigVariables != nil {
		t.Fatal(c.SelfHostedRunner.Labels)
	}
	if len(c.Paths) != 0 {
		t.Fatal(c.Paths)
	}
}

func TestConfigGenerateDefaultConfigFileError(t *testing.T) {
	p := filepath.Join("testdata", "config", "dir-does-not-exist", "test.yml")
	err := writeDefaultConfigFile(p)
	if err == nil {
		t.Fatal("error did not occur")
	}
	msg := err.Error()
	if !strings.Contains(msg, "could not write default configuration file") {
		t.Fatalf("unexpected error message: %q", msg)
	}
}

func TestConfigParseStrictLabels(t *testing.T) {
	c, err := ParseConfig([]byte("self-hosted-runner:\n  strict-labels: true\n  labels: [foo]"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.SelfHostedRunner.StrictLabels {
		t.Fatal("strict-labels was not parsed")
	}
	c, err = ParseConfig([]byte("self-hosted-runner:\n  labels: [foo]"))
	if err != nil {
		t.Fatal(err)
	}
	if c.SelfHostedRunner.StrictLabels {
		t.Fatal("strict-labels must default to false")
	}
}

func TestConfigLoadGlobalConfigOK(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	confDir := filepath.Join(dir, "actionlint")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "self-hosted-runner:\n  labels:\n    - foo\n    - bar\n"
	want := filepath.Join(confDir, "actionlint.yaml")
	if err := os.WriteFile(want, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	c, p, err := loadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("global config was not loaded")
	}
	if p != want {
		t.Fatalf("wanted config path %q but have %q", want, p)
	}
	if diff := cmp.Diff(c.SelfHostedRunner.Labels, []string{"foo", "bar"}); diff != "" {
		t.Fatal(diff)
	}
}

func TestConfigLoadGlobalConfigPrefersYamlOverYml(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	confDir := filepath.Join(dir, "actionlint")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "actionlint.yaml"), []byte("self-hosted-runner:\n  labels:\n    - yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "actionlint.yml"), []byte("self-hosted-runner:\n  labels:\n    - yml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, p, err := loadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("global config was not loaded")
	}
	if want := filepath.Join(confDir, "actionlint.yaml"); p != want {
		t.Fatalf("wanted config path %q but have %q", want, p)
	}
	if diff := cmp.Diff(c.SelfHostedRunner.Labels, []string{"yaml"}); diff != "" {
		t.Fatal(diff)
	}
}

func TestConfigLoadGlobalConfigYmlExtension(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	confDir := filepath.Join(dir, "actionlint")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(confDir, "actionlint.yml")
	if err := os.WriteFile(want, []byte("self-hosted-runner:\n  labels:\n    - only-yml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, p, err := loadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("global config was not loaded")
	}
	if p != want {
		t.Fatalf("wanted config path %q but have %q", want, p)
	}
}

func TestConfigLoadGlobalConfigNotFound(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, p, err := loadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c != nil || p != "" {
		t.Fatalf("wanted no global config but have config=%v and path=%q", c, p)
	}
}

func TestConfigLoadGlobalConfigParseError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	confDir := filepath.Join(dir, "actionlint")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, "actionlint.yaml"), []byte("this: [is not: valid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := loadGlobalConfig()
	if err == nil {
		t.Fatal("error did not occur")
	}
	if msg := err.Error(); !strings.Contains(msg, "global config file") {
		t.Fatalf("unexpected error message: %q", msg)
	}
}

func TestConfigLoadGlobalConfigHomeDirFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	// os.UserHomeDir reads %USERPROFILE% instead of $HOME on Windows.
	t.Setenv("USERPROFILE", home)
	confDir := filepath.Join(home, ".config", "actionlint")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(confDir, "actionlint.yaml")
	if err := os.WriteFile(want, []byte("self-hosted-runner:\n  labels:\n    - home\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, p, err := loadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("global config was not loaded from $HOME/.config")
	}
	if p != want {
		t.Fatalf("wanted config path %q but have %q", want, p)
	}
}

func TestConfigLoadGlobalConfigIgnoresRelativeXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	rel := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if r, err := filepath.Rel(wd, rel); err == nil {
		t.Setenv("XDG_CONFIG_HOME", r)
	} else {
		t.Skip("cannot make relative path")
	}
	// A config in the relative directory must not be loaded
	if err := os.MkdirAll(filepath.Join(rel, "actionlint"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rel, "actionlint", "actionlint.yaml"), []byte("self-hosted-runner:\n  labels: [rel]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, p, err := loadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c != nil || p != "" {
		t.Fatalf("relative XDG_CONFIG_HOME must be ignored but loaded %q", p)
	}
}

func TestConfigLoadGlobalConfigNotADirectory(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", f)
	c, _, err := loadGlobalConfig()
	if err != nil || c != nil {
		t.Fatalf("wanted no config and no error: %v %v", c, err)
	}
}

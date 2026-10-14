package jactionlint

import (
	"strconv"
	"strings"
	"testing"
)

const registryCredentialsTail = "updates:\n  - package-ecosystem: npm\n    directory: /\n    schedule: {interval: weekly}\n    cooldown: {default-days: 7}\n"

func TestHardcodedRegistryCredentials(t *testing.T) {
	tests := []struct {
		name string
		reg  string // the body of the "registries" section
		want []string
	}{
		{"password", "  a:\n    type: docker-registry\n    username: u\n    password: hunter2\n", []string{"4:15:password"}},
		{"token, quoted", "  a:\n    type: npm-registry\n    token: 'hunter2'\n", []string{"3:12:token"}},
		{"key", "  a:\n    type: hex-organization\n    organization: o\n    key: hunter2\n", []string{"4:10:key"}},
		{"auth-key", "  a:\n    type: hex-repository\n    repo: r\n    auth-key: hunter2\n", []string{"4:15:auth-key"}},
		{"a literal that looks like a variable", "  a:\n    type: npm-registry\n    token: $NPM_TOKEN\n", []string{"3:12:token"}},
		{"a literal in a flow mapping", "  a: {type: npm-registry, token: hunter2}\n", []string{"1:34:token"}},
		{"password and token of two registries", "  a:\n    type: npm-registry\n    token: x\n  b:\n    type: nuget-feed\n    username: u\n    password: y\n", []string{"3:12:token", "7:15:password"}},
		{"password in the URL", "  a:\n    type: maven-repository\n    url: https://u:hunter2@example.com/r\n", []string{"3:10:url"}},
		{"password in the URL without a path", "  a:\n    type: maven-repository\n    url: https://u:hunter2@example.com\n", []string{"3:10:url"}},
		{"password in the URL with an @ in the query", "  a:\n    type: maven-repository\n    url: https://u:hunter2@example.com/r?a=b@c\n", []string{"3:10:url"}},
		// not reported
		{"secret", "  a:\n    type: npm-registry\n    token: ${{secrets.NPM_TOKEN}}\n", nil},
		{"secret with spaces", "  a:\n    type: npm-registry\n    token: ${{ secrets.NPM_TOKEN }}\n", nil},
		{"secret with spaces, quoted", "  a:\n    type: docker-registry\n    username: u\n    password: \"${{ secrets.PW }}\"\n", nil},
		{"secret in the URL", "  a:\n    type: maven-repository\n    url: https://u:${{secrets.PW}}@example.com/r\n", nil},
		{"username only", "  a:\n    type: docker-registry\n    username: octocat\n    password: ${{secrets.PW}}\n", nil},
		{"literal username is not a credential", "  a:\n    type: git\n    url: https://example.com\n    username: x-access-token\n    password: ${{secrets.PW}}\n", nil},
		{"user in the URL without a password", "  a:\n    type: maven-repository\n    url: https://u@example.com/r\n", nil},
		{"empty password in the URL", "  a:\n    type: maven-repository\n    url: https://u:@example.com/r\n", nil},
		{"an @ in the path is not user information", "  a:\n    type: npm-registry\n    url: https://example.com/@scope:name\n", nil},
		{"an @ in the query is not user information", "  a:\n    type: npm-registry\n    url: https://example.com/r?a=b:c@d\n", nil},
		{"URL without a scheme", "  a:\n    type: docker-registry\n    url: u:p@registry.example.com\n", nil},
		{"OIDC", "  a:\n    type: docker-registry\n    url: https://example.azurecr.io\n    tenant-id: t\n    client-id: c\n", nil},
		{"AWS and other settings without a secret", "  a:\n    type: maven-repository\n    url: https://example.com\n    aws-region: us-east-1\n    account-id: \"1\"\n    role-name: r\n    domain: d\n", nil},
		{"public key fingerprint", "  a:\n    type: hex-repository\n    repo: r\n    auth-key: ${{secrets.K}}\n    public-key-fingerprint: abcdef\n", nil},
		{"replaces-base", "  a:\n    type: npm-registry\n    url: https://example.com\n    replaces-base: true\n    token: ${{secrets.T}}\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "version: 2\nregistries:\n" + tc.reg + registryCredentialsTail
			// The line of the section is 3
			errs := lintDependabot(t, src, defaultProfileConfig(), nil)
			var got []string
			for _, e := range errs {
				if e.ID != "hardcoded-registry-credentials" {
					t.Errorf("unexpected finding: %v", e)
					continue
				}
				if e.Severity != SeverityError {
					t.Errorf("severity: %v", e.Severity)
				}
				if e.Fix != nil {
					t.Errorf("a fix would invent a secret name: %+v", e.Fix)
				}
				if strings.Contains(e.Message, "hunter2") {
					t.Errorf("the message repeats the credential: %s", e.Message)
				}
				word := strings.Fields(strings.ReplaceAll(e.Message, `"`, " "))[0]
				got = append(got, strconv.Itoa(e.Line-2)+":"+strconv.Itoa(e.Column)+":"+word)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v, want %v\n%v", got, tc.want, errs)
			}
		})
	}
}

func TestHardcodedRegistryCredentialsProfileAndConfig(t *testing.T) {
	src := "version: 2\nregistries:\n  a:\n    type: npm-registry\n    token: hunter2\n" + registryCredentialsTail
	// The correctness profile does not have the rule
	if errs := lintDependabot(t, src, &Config{Profile: ProfileCorrectness}, nil); len(errs) != 0 {
		t.Fatal(errs)
	}
	// The default profile has it, at error level
	if errs := lintDependabot(t, src, defaultProfileConfig(), nil); len(errs) != 1 || errs[0].Severity != SeverityError {
		t.Fatal(errs)
	}
	// And it can be turned off
	cfg := &Config{Profile: ProfileDefault, Rules: map[string]RuleConfig{"hardcoded-registry-credentials": {Level: SeverityOff}}}
	if errs := lintDependabot(t, src, cfg, nil); len(errs) != 0 {
		t.Fatal(errs)
	}
	// A registry which no update uses still holds the credential in the repository
	unused := "version: 2\nregistries:\n  a:\n    type: npm-registry\n    token: hunter2\nupdates:\n  - package-ecosystem: pip\n    directory: /\n    schedule: {interval: weekly}\n    cooldown: {default-days: 7}\n"
	if errs := lintDependabot(t, unused, defaultProfileConfig(), nil); len(errs) != 1 {
		t.Fatal(errs)
	}
}

func TestDependabotIsLiteral(t *testing.T) {
	for v, want := range map[string]bool{
		"":                      false,
		"  ":                    false,
		"x":                     true,
		" x ":                   true,
		"${{secrets.X}}":        false,
		"${{ secrets.X }}":      false,
		"prefix-${{secrets.X}}": false,
		"$X":                    true,
		"{{secrets.X}}":         true,
	} {
		if got := dependabotIsLiteral(v); got != want {
			t.Errorf("dependabotIsLiteral(%q) = %v, want %v", v, got, want)
		}
	}
}

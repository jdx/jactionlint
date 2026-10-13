package jactionlint

import (
	"fmt"
	"slices"
	"strings"
)

// credentialDirs are directories which hold credentials by their purpose: a cache of one of them saves the files.
var credentialDirs = []string{
	".ssh", ".aws", ".docker", ".kube", ".gnupg", ".azure", ".config/gh", ".config/gcloud", ".config/doctl", ".config/hub",
}

// credentialFiles are files that hold credentials, or the token of a registry, when they exist.
var credentialFiles = []string{
	".npmrc", ".yarnrc.yml", ".pypirc", ".netrc", ".git-credentials", ".docker/config.json", ".kube/config", ".m2/settings.xml",
	".cargo/credentials", ".cargo/credentials.toml", ".gem/credentials", ".terraform.d/credentials.tfrc.json", ".env",
}

// privateKeyNames are the default names of the private keys of ssh.
var privateKeyNames = []string{"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519"}

// credentialPath tells which file of a "path" entry of a cache holds credentials: it returns the name of the file or
// directory, and false for any other path. The home directory can be written as "~", "$HOME" or "${HOME}", and the entry
// can be a path of the workspace (".env", "./.npmrc", "**/.env"). A glob that only ends in the name ("**/.npmrc") is
// the name, and a path below a credential directory is not, except for a private key: caching "~/.ssh/known_hosts" or
// "~/.docker/cli-plugins" is not a leak. ".git" is the directory of the repository: its config holds the token which
// actions/checkout persists, and a cache of the workspace root is not reported since it is not one of the named files.
func credentialPath(entry string) (string, bool) {
	p := strings.TrimSpace(entry)
	p = strings.Trim(p, `"'`)
	if p == "" || strings.HasPrefix(p, "!") {
		return "", false // an exclusion
	}
	home := false
	for _, h := range []string{"${HOME}", "$HOME", "~"} {
		if rest, ok := strings.CutPrefix(p, h); ok && (rest == "" || strings.HasPrefix(rest, "/")) {
			p, home = strings.TrimPrefix(rest, "/"), true
			break
		}
	}
	p = strings.TrimPrefix(p, "./")
	for strings.HasPrefix(p, "**/") {
		p = strings.TrimPrefix(p, "**/")
	}
	p = strings.TrimRight(p, "/")
	p = strings.TrimSuffix(p, "/**")
	p = strings.TrimSuffix(p, "/*")
	if strings.ContainsAny(p, "*?[") || strings.Contains(p, "${{") {
		return "", false
	}
	// Files of the workspace which hold a token: the repository's own .git and .npmrc and the environment file. The other
	// names are the ones of the home directory: a directory ".docker" of the workspace is a cache which the project
	// names itself (apache/arrow), not the login of docker.
	switch {
	case p == ".git", p == ".npmrc", p == ".yarnrc.yml", p == ".env":
		return p, true
	case strings.HasPrefix(p, ".env.") && !strings.Contains(p, "/"):
		return ".env", true // .env.local, .env.production
	case !home:
		return "", false
	}
	if slices.Contains(credentialDirs, p) || slices.Contains(credentialFiles, p) {
		return p, true
	}
	if dir, name, ok := strings.Cut(p, "/"); ok && dir == ".ssh" {
		for _, k := range privateKeyNames {
			if name == k {
				return ".ssh/" + k, true
			}
		}
	}
	return "", false
}

// checkCredentialCaches reports the caches whose "path" holds a file with credentials, with the option pedantic of the
// rule (zizmor#723). A cache is not private: a workflow of the repository that can read the key restores it, so what a
// step puts there, a token of a registry or a private key, can be read by whatever runs with that key, for example the
// workflows of pull requests of the same repository. It is not about the trigger of the workflow, so it is reported for
// every workflow.
func (rule *RuleCachePoisoning) checkCredentialCaches(j *Job) {
	// "cache-mode: none" gives no access to the cache and "read" forbids saving to it: nothing is written then
	mode := j.CacheMode
	if mode == nil && rule.wf != nil {
		mode = rule.wf.CacheMode
	}
	if mode != nil && (mode.Value == "none" || mode.Value == "read") {
		return
	}
	for _, s := range flattenSteps(j.Steps) {
		a, ref := stepAction(s)
		if a == nil || ref.Kind != UsesAction {
			continue
		}
		name := ref.CanonicalName()
		if name != "actions/cache" && name != "actions/cache/save" {
			continue
		}
		path, ok := a.input("path")
		if !ok {
			continue
		}
		if v, _ := a.input("lookup-only"); isTrueLiteral(v) {
			continue // it only checks that the entry exists: nothing is restored or saved
		}
		pos := a.Uses.Pos
		if in := a.Inputs["path"]; in != nil && in.Value != nil && in.Value.Pos != nil {
			pos = in.Value.Pos
		}
		for _, entry := range strings.FieldsFunc(path, func(r rune) bool { return r == '\n' || r == ',' }) {
			if what, ok := credentialPath(entry); ok {
				if what == ".git" && !checkoutPersists(j) {
					continue // without a checkout that leaves its token in .git/config, the directory has none
				}
				rule.ReportID("cache-poisoning", pos, fmt.Sprintf("the cache saves %q, which can hold credentials (%s). a cache can be restored by other workflows of the repository, so what is in it is not secret. remove the path from the cache, or cache what the step downloads and not the configuration with the token", strings.TrimSpace(entry), credentialKind(what)))
			}
		}
	}
}

func credentialKind(what string) string {
	switch {
	case what == ".git":
		return "the git config of the checkout keeps the token of actions/checkout unless persist-credentials is false"
	case strings.HasPrefix(what, ".ssh"):
		return "private keys of ssh"
	case what == ".env":
		return "the secrets of an application"
	case what == ".docker" || what == ".docker/config.json":
		return "the login of docker registries"
	case what == ".npmrc" || what == ".yarnrc.yml":
		return "the token of an npm registry"
	}
	return "the login of a service"
}

// checkoutPersists reports whether a checkout of the job leaves the token in the git config: actions/checkout without
// "persist-credentials: false". Without a checkout in the job there is no token to leave (the cache is restored before
// a checkout in the jobs which cache the git repository for a faster fetch).
func checkoutPersists(j *Job) bool {
	for _, s := range flattenSteps(j.Steps) {
		a, ref := stepAction(s)
		if a == nil || !ref.isRepoAction("actions/checkout") {
			continue
		}
		if v, set := a.input("persist-credentials"); !set || !isFalseLiteral(v) {
			return true
		}
	}
	return false
}

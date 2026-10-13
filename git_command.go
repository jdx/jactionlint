package jactionlint

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// gitRedirectingEnv are the variables of the environment which point git to a repository, an index or a configuration
// other than the one of the directory it runs in. A hook of git sets some of them (GIT_DIR, GIT_INDEX_FILE), and `git -C`
// does not override an absolute GIT_DIR.
var gitRedirectingEnv = map[string]bool{
	"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_OBJECT_DIRECTORY": true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_COMMON_DIR": true, "GIT_NAMESPACE": true, "GIT_PREFIX": true,
	"GIT_CONFIG": true, "GIT_CONFIG_GLOBAL": true, "GIT_CONFIG_SYSTEM": true, "GIT_CONFIG_NOSYSTEM": true,
	"GIT_CONFIG_PARAMETERS": true, "GIT_CONFIG_COUNT": true,
}

// cleanGitEnv returns the environment without the variables that redirect git (see gitRedirectingEnv, and
// GIT_CONFIG_KEY_n and GIT_CONFIG_VALUE_n). The other variables are kept: GIT_CEILING_DIRECTORIES, GIT_TERMINAL_PROMPT.
func cleanGitEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		name, _, _ := strings.Cut(e, "=")
		if gitRedirectingEnv[name] || strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// gitReadCommand returns the command `git <args>` run in the directory dir, for reading a repository: it does not take the
// repository from the environment, does not write the index or take its lock (--no-optional-locks), and does not quote
// the names of the files (core.quotepath).
func gitReadCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	full := append([]string{"--no-optional-locks", "-c", "core.quotepath=false", "-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = cleanGitEnv(os.Environ())
	return cmd
}

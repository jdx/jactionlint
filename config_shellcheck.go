package jactionlint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ShellcheckConfig is the "shellcheck" mapping of the config file. It tunes how jactionlint runs shellcheck for
// the scripts of "run:" steps. Without it jactionlint runs shellcheck with --norc, so that the .shellcheckrc of the
// directory that happens to be the working directory cannot change the results, and with the arguments it needs.
//
// The environment variable SHELLCHECK_OPTS is still read by shellcheck itself. These keys are for the places where
// it cannot be set, such as pre-commit.ci, and for keeping the settings in the repository.
type ShellcheckConfig struct {
	// RCFile is the shellcheckrc file to use instead of --norc. A relative path is resolved from the root of the
	// repository (the directory which has ".github"); the --shellcheck-rcfile flag, which is resolved from the
	// working directory, overrides it. The file must exist. A config file which is not the user's own (the one of the
	// repository) can name only a file inside the repository.
	RCFile string `yaml:"rcfile"`
	// Args are more arguments for shellcheck, such as "--exclude=SC2129" or "--severity=warning".
	// jactionlint sets the output format, the shell, the external sources and the script on its own, so
	// the options which would change them are errors.
	Args []string `yaml:"args"`
	// Enable lists the optional checks of shellcheck to turn on (shellcheck --enable=NAME,...), for example
	// "avoid-nullary-conditions" or "all".
	Enable []string `yaml:"enable"`
}

var shellcheckConfigKeys = []string{"rcfile", "args", "enable"}

var reShellcheckEnable = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// shellcheckManagedOptions are the options of shellcheck which jactionlint sets itself, with the reason.
var shellcheckManagedOptions = map[string]string{
	"-f": "jactionlint reads the JSON output", "--format": "jactionlint reads the JSON output",
	"-C": "jactionlint reads the JSON output", "--color": "jactionlint reads the JSON output",
	"-s": "jactionlint passes the shell of each script", "--shell": "jactionlint passes the shell of each script",
	"--norc": "use \"rcfile\" to read a shellcheckrc", "--rcfile": "use \"rcfile\"",
	"-V": "the output would not be findings", "--version": "the output would not be findings",
	"-W": "the output would not be findings", "--wiki-link-count": "the output would not be findings",
}

// validate checks the values which can be checked without the file system.
func (s *ShellcheckConfig) validate() error {
	for _, a := range s.Args {
		if a == "-" || !strings.HasPrefix(a, "-") {
			return fmt.Errorf("invalid argument %q in \"shellcheck.args\": write each option as one item with its value attached, \"--exclude=SC2129\" or \"-eSC2129\", and no file names: jactionlint passes the script itself", a)
		}
		name := a
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		if strings.HasPrefix(name, "-") && !strings.HasPrefix(name, "--") && len(name) > 2 {
			name = name[:2] // -fjson: the option letter with its value attached
		}
		if why, ok := shellcheckManagedOptions[name]; ok {
			return fmt.Errorf("invalid argument %q in \"shellcheck.args\": %s", a, why)
		}
	}
	for _, e := range s.Enable {
		if !reShellcheckEnable.MatchString(e) {
			return fmt.Errorf("invalid check name %q in \"shellcheck.enable\": write the name of an optional check of shellcheck such as \"avoid-nullary-conditions\" or \"all\"", e)
		}
	}
	return nil
}

// shellcheckRCFile resolves the shellcheckrc to use for the files of a project. flag is the value of
// --shellcheck-rcfile (relative to the working directory), and the config value is relative to the root of the
// project. It returns "" when no rcfile is configured, and an error when the file does not exist or a config of a
// repository points outside of it.
func shellcheckRCFile(flag, cwd string, cfg *Config, root string) (string, error) {
	var p, base, from string
	switch {
	case flag != "":
		p, base, from = flag, cwd, "--shellcheck-rcfile"
	case cfg != nil && cfg.Shellcheck.RCFile != "":
		p, base, from = cfg.Shellcheck.RCFile, root, "\"shellcheck.rcfile\""
		if base == "" {
			base = cwd
		}
	default:
		return "", nil
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	if from != "--shellcheck-rcfile" && !cfg.userOwned && root != "" && !isPathInDir(absPath(root), absPath(p)) {
		return "", fmt.Errorf("%s: %q is outside of the repository. a config file of the repository can use only files inside it", from, cfg.Shellcheck.RCFile)
	}
	if s, err := os.Stat(p); err != nil || s.IsDir() {
		return "", fmt.Errorf("%s: the shellcheckrc %q does not exist or is not a file", from, p)
	}
	return absPath(p), nil
}

// extraArgs returns the arguments the config adds to the call of shellcheck.
func (s *ShellcheckConfig) extraArgs() []string {
	var ret []string
	ret = append(ret, s.Args...)
	if len(s.Enable) > 0 {
		ret = append(ret, "--enable="+strings.Join(s.Enable, ","))
	}
	return ret
}

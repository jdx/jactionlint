package jactionlint

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// releaseTools are release tools that publish or push tags when run with one of the verbs. The recognition is by what
// the command does, not by the name of one tool, so a new release tool is a line here. An empty verb list means that
// every run of the tool releases.
var releaseTools = map[string][]string{
	"semantic-release": nil,
	"release-it":       nil,
	"release-plz":      {"release"},
	"changeset":        {"publish"},
	"changesets":       {"publish"},
	"lerna":            {"publish"},
	"mvn":              {"deploy"},
	"mvnw":             {"deploy"},
	"gradle":           {"publish", "publishtomavencentral"},
	"gradlew":          {"publish", "publishtomavencentral"},
	"vsce":             {"publish"},
	"ovsx":             {"publish"},
	"dotnet":           {"push"},
	"nuget":            {"push"},
}

// releaseCommand reports whether the command runs a release tool that publishes or releases and describes it. A run
// that only simulates (`--dry-run`) is not one.
func releaseCommand(c *runscript.Command) (string, bool) {
	words := make([]string, 0, len(c.Words))
	for _, w := range c.Words {
		words = append(words, strings.ToLower(w.Value))
	}
	i := 0
	for i < len(words) && launchers[words[i]] {
		i++
	}
	if i >= len(words) {
		return "", false
	}
	// ./gradlew and ./mvnw are run with their path, and on Windows with an extension
	tool := strings.SplitN(path.Base(words[i]), "@", 2)[0]
	tool = strings.TrimSuffix(strings.TrimSuffix(tool, ".cmd"), ".bat")
	args := words[i+1:]
	if slices.Contains(args, "--dry-run") || slices.Contains(args, "--noop") {
		return "", false
	}
	if tool == "cargo" {
		// cargo-release only simulates until it is given --execute
		// the subcommand follows the toolchain (+nightly) and the options of cargo (--locked)
		sub := slices.IndexFunc(args, func(a string) bool { return !strings.HasPrefix(a, "+") && !strings.HasPrefix(a, "-") })
		if sub >= 0 && args[sub] == "release" && (slices.Contains(args, "--execute") || slices.Contains(args, "-x")) {
			return "cargo release", true
		}
		return "", false
	}
	verbs, ok := releaseTools[tool]
	if !ok {
		return "", false
	}
	if tool == "dotnet" && !slices.Contains(args, "nuget") {
		return "", false
	}
	if len(verbs) == 0 {
		return tool, true
	}
	for _, a := range args {
		if slices.Contains(verbs, a) {
			return tool + " " + a, true
		}
	}
	return "", false
}

var versionLikeRe = regexp.MustCompile(`^v?\d+(\.\d+)*`)

// gitCommand returns the subcommand of a git command and its arguments, skipping the global options
// (`git -C dir push`). The subcommand is "" when the command is not git.
func gitCommand(c *runscript.Command) (string, []string) {
	if len(c.Words) == 0 || c.Words[0].Value != "git" {
		return "", nil
	}
	rest := c.Words[1:]
	for len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
		skip := 1
		if rest[0].Value == "-C" || rest[0].Value == "-c" {
			skip = 2
		}
		if skip > len(rest) {
			return "", nil
		}
		rest = rest[skip:]
	}
	if len(rest) == 0 {
		return "", nil
	}
	args := make([]string, 0, len(rest)-1)
	for _, w := range rest[1:] {
		args = append(args, w.Value)
	}
	return rest[0].Value, args
}

// pushesTags reports whether the arguments of `git push` push tags: --tags, --follow-tags, --mirror, a refspec of
// refs/tags, or a ref that is named like a tag or a version (`git push origin v1.2.3`, `git push origin "$TAG"`).
func pushesTags(args []string) bool {
	positional := 0
	for _, a := range args {
		l := strings.ToLower(a)
		switch {
		case l == "--tags" || l == "--follow-tags" || l == "--mirror" || strings.HasPrefix(l, "--follow-tags="):
			return true
		case strings.HasPrefix(l, "-"):
			continue
		}
		positional++
		if positional == 1 {
			continue // the remote
		}
		if strings.Contains(l, "refs/tags/") || strings.Contains(l, "tag") || versionLikeRe.MatchString(strings.TrimPrefix(l, "+")) {
			return true
		}
	}
	return false
}

// createsTag reports whether the arguments of `git tag` create a tag (and do not list or delete).
func createsTag(args []string) bool {
	for _, a := range args {
		switch a {
		case "-l", "--list", "-d", "--delete", "-v", "--verify":
			return false
		}
	}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return true
		}
	}
	return false
}

// tagState tracks, over the steps of one job, whether it creates a tag and whether it pushes. A job that does both
// releases, however the tag is named.
type tagState struct{ created, pushed bool }

// observe looks at a command. It returns a description when the command pushes tags by itself.
func (t *tagState) observe(c *runscript.Command) (string, bool) {
	sub, args := gitCommand(c)
	switch sub {
	case "push":
		if pushesTags(args) {
			return "git push of tags", true
		}
		t.pushed = true
	case "tag":
		if createsTag(args) {
			t.created = true
		}
	}
	return "", false
}

func (t *tagState) pushesCreatedTag() bool { return t.created && t.pushed }

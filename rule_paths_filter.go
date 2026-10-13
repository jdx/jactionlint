package jactionlint

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// RuleUnmatchedPathsFilter reports an entry of `paths` or `paths-ignore` which matches no file of the repository.
//
// GitHub starts a workflow for a push or a pull request only when a changed file matches an entry of `paths`. An entry
// whose path was renamed, moved or misspelled matches nothing, so the workflow does not start for the files that it
// was written for, and nothing says so. In `paths-ignore` such an entry ignores nothing.
//
// The entries are matched against the files that git lists in the working tree (tracked, and untracked ones which are
// not ignored), or, when git cannot say, a walk of the project root (`.git`, node_modules and virtual environments
// excluded).
// The rule is conservative in what it calls a mismatch, because a false report would hide a real one:
//
//   - it runs only when the file belongs to a project (not for stdin or a file outside a repository), and not for a
//     generated file (unless the configuration reports them), whose entries come from another file;
//   - an entry matches when it matches a file or a directory of the repository, in either case: GitHub does not say
//     whether `docs` alone matches the files in it, so it is not called dead;
//   - an entry starting with `!` is skipped (a negation that matches nothing changes nothing), and so is an entry that is
//     invalid, contains `${{`, starts with `/` or `./`, or uses a form this rule cannot evaluate;
//   - an entry in a git submodule that is not checked out is skipped, as is any entry that could match inside it;
//   - a repository with more than pathsFilterMaxEntries files is not looked at.
type RuleUnmatchedPathsFilter struct {
	RuleBase
	root string
}

// pathsFilterMaxEntries bounds the number of files and directories that the rule reads.
var pathsFilterMaxEntries = 300000

// NewRuleUnmatchedPathsFilter creates a new RuleUnmatchedPathsFilter instance for the project at root.
func NewRuleUnmatchedPathsFilter(root string) *RuleUnmatchedPathsFilter {
	return &RuleUnmatchedPathsFilter{
		RuleBase: RuleBase{
			name: "unmatched-paths-filter",
			desc: "Checks that the entries of \"paths:\" and \"paths-ignore:\" match a file of the repository",
		},
		root: root,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnmatchedPathsFilter) VisitWorkflowPre(n *Workflow) error {
	var files *repoPaths
	for _, e := range n.On {
		hook, ok := e.(*WebhookEvent)
		if !ok {
			continue
		}
		for _, f := range []struct {
			filter *WebhookEventFilter
			where  string
			effect string
		}{
			{hook.Paths, "paths", "it never triggers the workflow"},
			{hook.PathsIgnore, "paths-ignore", "it ignores nothing"},
		} {
			if f.filter == nil {
				continue
			}
			for _, v := range f.filter.Values {
				m := compilePathFilter(v.Value)
				if m == nil {
					continue
				}
				if files == nil {
					files = readRepoPaths(rule.root)
				}
				if files == nil {
					return nil // the repository cannot be read, or it has too many files
				}
				if files.cannotDecide(m) {
					continue // this entry could match inside a submodule that is not checked out; the others can still be decided
				}
				if !files.matches(m) {
					rule.ReportIDf("unmatched-paths-filter", v.Pos, "the entry %q of %q matches no file or directory of the repository, so %s. a path may have been renamed or moved, or have a typo", v.Value, f.where, f.effect)
				}
			}
		}
	}
	return nil
}

// pathFilter is an entry of `paths` compiled to match the lower case paths of the repository.
type pathFilter struct {
	re *regexp.Regexp
	// prefix is the text that every path which matches starts with, in lower case.
	prefix string
	// deep is whether the filter can match a path in a directory below the one of its prefix: it has `**`, or a `/`
	// after the prefix. A filter that is not deep (`*.md`) cannot reach into a submodule.
	deep bool
}

// compilePathFilter compiles an entry of `paths`. It returns nil for an entry that is not looked at.
func compilePathFilter(pat string) *pathFilter {
	if pat == "" || strings.HasPrefix(pat, "!") || strings.HasPrefix(pat, "/") || strings.HasPrefix(pat, "./") ||
		strings.Contains(pat, "${{") || len(ValidatePathGlob(pat)) > 0 {
		return nil
	}
	// GitHub does not say whether `dir/` matches the files in the directory, so it is read as `dir`
	pat = strings.ToLower(strings.TrimSuffix(pat, "/"))
	if pat == "" {
		return nil
	}
	var b strings.Builder
	b.WriteString("^")
	rs := []rune(pat)
	atom := false // the last thing written can be repeated by `?` and `+`
	prefix, inPrefix := "", true
	deep := false
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch c {
		case '*':
			j := i
			for j+1 < len(rs) && rs[j+1] == '*' {
				j++
			}
			if j > i {
				deep = true
				// `**/` also matches nothing at all in front of the next name: `**/README.md` is README.md
				if j+1 < len(rs) && rs[j+1] == '/' {
					b.WriteString("(?:.*/)?")
					j++
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
			i = j
			atom = false
			inPrefix = false
		case '?', '+':
			if !atom {
				return nil
			}
			b.WriteRune(c)
			atom = false
			if inPrefix && prefix != "" {
				_, size := utf8.DecodeLastRuneInString(prefix)
				prefix = prefix[:len(prefix)-size] // the character in front of it is optional or repeated
			}
			inPrefix = false
		case '[':
			end := -1
			for k := i + 1; k < len(rs); k++ {
				if rs[k] == ']' {
					end = k
					break
				}
			}
			if end < 0 || end == i+1 || rs[i+1] == '!' || rs[i+1] == '^' {
				return nil
			}
			class := string(rs[i+1 : end])
			if strings.ContainsAny(class, `\[`) {
				return nil
			}
			b.WriteString("[" + class + "]")
			i = end
			atom = true
			inPrefix = false
		case '\\':
			// A backslash escapes only [ ? * + \ and !, anywhere else it is a character of the path (glob.go)
			lit := '\\'
			if i+1 < len(rs) && strings.ContainsRune(`[?*+\!`, rs[i+1]) {
				i++
				lit = rs[i]
			}
			b.WriteString(regexp.QuoteMeta(string(lit)))
			atom = true
			if inPrefix {
				prefix += string(lit)
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
			atom = true
			if inPrefix {
				prefix += string(c)
			} else if c == '/' {
				deep = true
			}
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	return &pathFilter{re: re, prefix: prefix, deep: deep}
}

// repoPaths are the files and directories of a repository, in lower case and sorted.
type repoPaths struct {
	paths []string
	// opaque are the directories of the submodules that are not checked out, in lower case.
	opaque []string
}

// matches reports whether a path of the repository matches the filter.
func (r *repoPaths) matches(m *pathFilter) bool {
	i := sort.SearchStrings(r.paths, m.prefix)
	for ; i < len(r.paths) && strings.HasPrefix(r.paths[i], m.prefix); i++ {
		if m.re.MatchString(r.paths[i]) {
			return true
		}
	}
	return false
}

// cannotDecide reports whether the filter could match something in a directory that is not available.
func (r *repoPaths) cannotDecide(m *pathFilter) bool {
	for _, o := range r.opaque {
		// The filter names a path inside the submodule, or its wildcard can reach into it from a shorter prefix
		// (`**/*.js`, `vendor/**`): then a file that it matches may be there. A filter like `*.md` cannot reach
		// below the top directory and is decided by the files of the repository itself.
		if m.prefix == o || strings.HasPrefix(m.prefix, o+"/") || (m.deep && strings.HasPrefix(o+"/", m.prefix)) {
			return true
		}
	}
	return false
}

type repoPathsEntry struct {
	at    time.Time
	paths *repoPaths
}

var (
	repoPathsMu    sync.Mutex
	repoPathsCache = map[string]repoPathsEntry{}
)

// repoPathsTTL is how long the files of a repository are kept: the workflows of a run are linted one after another.
const repoPathsTTL = 20 * time.Second

// readRepoPaths lists the files and directories under root, or returns nil when it cannot be done or the repository
// has too many.
func readRepoPaths(root string) *repoPaths {
	repoPathsMu.Lock()
	defer repoPathsMu.Unlock()
	if e, ok := repoPathsCache[root]; ok && time.Since(e.at) < repoPathsTTL {
		return e.paths
	}
	p := walkRepoPaths(root)
	repoPathsCache[root] = repoPathsEntry{time.Now(), p}
	return p
}

// gitListing says what the files of git are worth.
type gitListing int

const (
	// gitUnavailable: git cannot say (it is not installed, the root is not a work tree, it failed), so the tree can be
	// walked instead.
	gitUnavailable gitListing = iota
	// gitListed: the files are the ones of the repository.
	gitListed
	// gitUnusable: git answered, but there is nothing to match against (no file at all) or the repository has more files
	// than the bound. The walk would not do better, so the rule says nothing.
	gitUnusable
)

// gitListedPaths returns the files that git knows in the working tree of root: the tracked ones, and the untracked
// ones that are not ignored (a file which was just created and is not added yet is a file of the next commit). A
// submodule is one path, its gitlink. GitHub matches the changed files of a commit, which are tracked, so an untracked
// tree such as node_modules is not what an entry is about, and not reading it also keeps the bound for the files that
// count.
func gitListedPaths(root string) (map[string]struct{}, gitListing) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return nil, gitUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, gitUnavailable
	}
	seen := map[string]struct{}{}
	for _, f := range strings.Split(string(out), "\x00") {
		if f == "" || isRepositoryInternal(f) {
			continue
		}
		// The directories of the file are paths of the repository too
		for i := 0; i < len(f); i++ {
			if f[i] == '/' {
				seen[strings.ToLower(f[:i])] = struct{}{}
			}
		}
		if len(seen) > pathsFilterMaxEntries {
			return nil, gitUnusable
		}
		seen[strings.ToLower(f)] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, gitUnusable
	}
	return seen, gitListed
}

// isRepositoryInternal reports whether the path is inside the directory of the version control system (.git, .jj), which
// is not a file of the repository.
func isRepositoryInternal(p string) bool {
	for _, m := range repositoryMarkers {
		if p == m || strings.HasPrefix(p, m+"/") {
			return true
		}
	}
	return false
}

// skippedInstallDirs are the trees of installed packages, which are not part of the repository. The walk, used when
// git cannot list the files, does not read them.
var skippedInstallDirs = map[string]bool{"node_modules": true, ".venv": true, "venv": true}

func walkRepoPaths(root string) *repoPaths {
	switch seen, st := gitListedPaths(root); st {
	case gitListed:
		return newRepoPaths(root, seen)
	case gitUnusable:
		return nil
	}
	seen := map[string]struct{}{}
	n := 0
	tooMany := false
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir // a directory which cannot be read has no files to see
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return nil
		}
		if slices.Contains(repositoryMarkers, d.Name()) || (d.IsDir() && skippedInstallDirs[d.Name()]) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		if n++; n > pathsFilterMaxEntries {
			tooMany = true
			return filepath.SkipAll
		}
		seen[strings.ToLower(rel)] = struct{}{}
		return nil
	})
	if err != nil || tooMany {
		return nil
	}
	return newRepoPaths(root, seen)
}

func newRepoPaths(root string, seen map[string]struct{}) *repoPaths {
	ps := make([]string, 0, len(seen))
	for s := range seen {
		ps = append(ps, s)
	}
	sort.Strings(ps)
	return &repoPaths{paths: ps, opaque: opaqueSubmodules(root, seen)}
}

// opaqueSubmodules returns the paths of the submodules in .gitmodules which have no file in the working tree.
func opaqueSubmodules(root string, seen map[string]struct{}) []string {
	b, err := os.ReadFile(filepath.Join(root, ".gitmodules"))
	if err != nil {
		return nil
	}
	var ret []string
	for _, l := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok || strings.TrimSpace(k) != "path" {
			continue
		}
		p := strings.ToLower(path.Clean(strings.Trim(strings.TrimSpace(v), `"`)))
		empty := true
		for s := range seen {
			if strings.HasPrefix(s, p+"/") {
				empty = false
				break
			}
		}
		if empty {
			ret = append(ret, p)
		}
	}
	return ret
}

func init() {
	registerRules(
		RuleInfo{ID: "unmatched-paths-filter", Group: RuleGroupCorrectness, Summary: "An entry of paths or paths-ignore matches no file of the repository.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-unmatched-paths-filter"},
	)
	registerRuleFactory("unmatched-paths-filter", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("unmatched-paths-filter") {
			return nil // the pedantic rule is off: do not walk the repository for nothing
		}
		if env.project == nil {
			env.Skip("the workflow does not belong to a project, so there are no files to match")
			return nil
		}
		generated := isGeneratedSource(env.src)
		if generated && (env.config == nil || env.config.GeneratedFiles == nil || *env.config.GeneratedFiles != GeneratedFilesReport) {
			env.Skip("the file is generated, its filters come from another file")
			return nil
		}
		return []Rule{NewRuleUnmatchedPathsFilter(env.project.RootDir())}
	})
}

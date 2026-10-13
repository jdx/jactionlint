package jactionlint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// repoFiles reads the small configuration files of the repository that decide whether an install command is safe
// (`.npmrc`, `package.json`, `bunfig.toml`, `.yarnrc.yml`). A rule that looks at many steps reads a file once. A file
// that does not exist, cannot be read or is larger than 1 MiB is "not there": a rule that cannot tell stays quiet.
type repoFiles struct {
	cache map[string]repoFile
}

type repoFile struct {
	text string
	ok   bool
}

const repoFileLimit = 1 << 20

// read returns the content of the file.
func (r *repoFiles) read(path string) (string, bool) {
	if f, ok := r.cache[path]; ok {
		return f.text, f.ok
	}
	if r.cache == nil {
		r.cache = map[string]repoFile{}
	}
	f := repoFile{}
	if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() && st.Size() <= repoFileLimit {
		if b, err := os.ReadFile(path); err == nil {
			f = repoFile{string(b), true}
		}
	}
	r.cache[path] = f
	return f.text, f.ok
}

// npmrc returns the settings of the `.npmrc` of the directory. The last assignment of a key wins, a key without a
// value is true, and keys are written with dashes (`ignore_scripts` is the same as `ignore-scripts`).
func (r *repoFiles) npmrc(dir string) map[string]string {
	text, ok := r.read(filepath.Join(dir, ".npmrc"))
	if !ok {
		return nil
	}
	return parseNpmrc(text)
}

func parseNpmrc(text string) map[string]string {
	cfg := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == ';' || line[0] == '#' || line[0] == '[' {
			continue
		}
		key, val, found := strings.Cut(line, "=")
		key = strings.ReplaceAll(strings.TrimSpace(key), "_", "-")
		if !found {
			val = "true"
		}
		val = strings.TrimSpace(val)
		if i := strings.IndexAny(val, ";#"); i > 0 && (val[i-1] == ' ' || val[i-1] == '\t') {
			val = strings.TrimSpace(val[:i])
		}
		cfg[key] = strings.Trim(val, `"'`)
	}
	return cfg
}

// packageJSON is the part of package.json that the rules look at.
type packageJSON struct {
	PackageManager      string          `json:"packageManager"`
	TrustedDependencies json.RawMessage `json:"trustedDependencies"`
	AllowScripts        json.RawMessage `json:"allowScripts"`
}

func (r *repoFiles) packageJSON(dir string) (packageJSON, bool) {
	text, ok := r.read(filepath.Join(dir, "package.json"))
	if !ok {
		return packageJSON{}, false
	}
	var p packageJSON
	if json.Unmarshal([]byte(text), &p) != nil {
		// a field of another type (a "packageManager" that is not a string) is not worth a decision
		var raw map[string]json.RawMessage
		if json.Unmarshal([]byte(text), &raw) != nil {
			return packageJSON{}, false
		}
		p.TrustedDependencies, p.AllowScripts = raw["trustedDependencies"], raw["allowScripts"]
		_ = json.Unmarshal(raw["packageManager"], &p.PackageManager)
	}
	return p, true
}

// managerVersion returns the version of the package manager that package.json asks for with "packageManager": "yarn@4.14.0+sha224.abc".
// The major, minor and the ok are zero and false when it names no version.
func (p packageJSON) managerVersion(manager string) (major, minor int, ok bool) {
	name, ver, found := strings.Cut(strings.TrimPrefix(p.PackageManager, "^"), "@")
	if !found || name != manager {
		return 0, 0, false
	}
	m := yarnVersionRe.FindStringSubmatch(ver)
	if m == nil {
		return 0, 0, false
	}
	major, minor = atoiSafe(m[1]), atoiSafe(m[2])
	return major, minor, true
}

var yarnVersionRe = regexp.MustCompile(`^(\d+)\.(\d+)`)

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' || n > 1<<20 {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// commandDirs returns the directories whose configuration files apply to the command: the root of the repository
// and the working directory of the step (its own, else that of the job, else that of the workflow), and the
// directory chosen with `--prefix` or `--cwd` when it is a literal path inside the repository. It returns false
// when the directory is not known: a working directory or prefix that is an expression, or a `cd` before the
// command.
func (c *runContext) commandDirs(root string, step *Step, s *runscript.Script, cmd *runscript.Command) ([]string, bool) {
	for _, o := range s.Commands {
		if o.Offset < cmd.Offset && (o.Name == "cd" || o.Name == "pushd") {
			return nil, false
		}
	}
	if root == "" {
		// a workflow linted on its own: there are no files to look at, and what they would say is not known
		return nil, true
	}
	dirs := []string{root}
	var wd *String
	if c.wf != nil && c.wf.Defaults != nil && c.wf.Defaults.Run != nil {
		wd = c.wf.Defaults.Run.WorkingDirectory
	}
	if c.job != nil && c.job.Defaults != nil && c.job.Defaults.Run != nil && c.job.Defaults.Run.WorkingDirectory != nil {
		wd = c.job.Defaults.Run.WorkingDirectory
	}
	if run, ok := step.Exec.(*ExecRun); ok && run.WorkingDirectory != nil {
		wd = run.WorkingDirectory
	}
	add := func(v string, dyn bool) bool {
		d := filepath.FromSlash(strings.TrimSpace(v))
		if dyn || filepath.IsAbs(d) || !filepath.IsLocal(d) {
			return false
		}
		if d != "." && d != "" {
			dirs = append(dirs, filepath.Join(root, d))
		}
		return true
	}
	if wd != nil && !add(wd.Value, wd.ContainsExpression()) {
		return nil, false
	}
	for _, w := range cmd.FlagValues("--prefix", "--cwd", "-C", "--dir") {
		if !add(w.Value, w.Dynamic()) {
			return nil, false
		}
	}
	return dirs, true
}

// Command fix-corpus checks that -fix is safe on real workflows.
//
// It copies the .github directory of each repository into a scratch directory, applies the fixes of
// jactionlint there and reports, for every repository:
//
//  1. whether all YAML files are still valid YAML,
//  2. whether a second run of -fix changes nothing (and -diff prints nothing),
//  3. whether the number of findings went up for any rule (it must only go down),
//  4. whether the number of findings of zizmor (--persona pedantic --offline) went up for any audit.
//
// It exits with status 1 when any of the four fails, or when jactionlint refused a fix, so it can run in
// CI. The repositories themselves are never modified.
//
// Usage:
//
//	go build -o jactionlint ./cmd/jactionlint
//	go run ./scripts/fix-corpus -jactionlint ./jactionlint [-unsafe] [-no-zizmor] [dir ...]
//
// Without directories it uses ~/src/*-jactionlint and ~/src/mise. See scripts/fix-corpus/README.md.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

type repoReport struct {
	Label, Dir string
	Files      int
	Changed    int
	Applied    int
	ByRule     map[string]int
	JLBefore   map[string]int
	JLAfter    map[string]int
	ZBefore    map[string]int
	ZAfter     map[string]int
	Problems   []string
	Skipped    string
}

func main() {
	jal := flag.String("jactionlint", "jactionlint", "the jactionlint executable")
	cfg := flag.String("config", "", "config file for jactionlint (default: config.yaml next to this program's source)")
	zizmor := flag.String("zizmor", "mise x zizmor@1.30.1 -- zizmor", "command that runs zizmor")
	noZizmor := flag.Bool("no-zizmor", false, "do not compare with zizmor")
	unsafe := flag.Bool("unsafe", false, "apply the unsafe fixes too (-fix=unsafe)")
	keep := flag.String("keep", "", "keep the fixed copies in this directory instead of a temporary one")
	flag.Parse()

	if *cfg == "" {
		*cfg = "scripts/fix-corpus/config.yaml"
	}
	cfgPath, err := filepath.Abs(*cfg)
	if err != nil {
		fatal(err)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		fatal(fmt.Errorf("config: %w", err))
	}
	exe, err := exec.LookPath(*jal)
	if err != nil {
		fatal(err)
	}
	if exe, err = filepath.Abs(exe); err != nil {
		fatal(err)
	}

	dirs := flag.Args()
	if len(dirs) == 0 {
		home, _ := os.UserHomeDir()
		dirs, _ = filepath.Glob(filepath.Join(home, "src", "*-jactionlint"))
		dirs = append(dirs, filepath.Join(home, "src", "mise"))
	}
	scratch := *keep
	if scratch == "" {
		if scratch, err = os.MkdirTemp("", "fix-corpus-"); err != nil {
			fatal(err)
		}
		defer os.RemoveAll(scratch)
	} else if err := os.MkdirAll(scratch, 0o755); err != nil {
		fatal(err)
	}

	mode := "-fix"
	if *unsafe {
		mode = "-fix=unsafe"
	}
	var reports []*repoReport
	failed := false
	for i, dir := range dirs {
		r := &repoReport{Label: strings.TrimSuffix(filepath.Base(dir), "-jactionlint"), Dir: dir}
		work := filepath.Join(scratch, fmt.Sprintf("%02d-%s", i, r.Label))
		run(r, exe, cfgPath, *zizmor, !*noZizmor, mode, dir, work)
		if len(r.Problems) > 0 {
			failed = true
		}
		reports = append(reports, r)
	}
	printReport(os.Stdout, reports, mode, !*noZizmor)
	if failed {
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "fix-corpus:", err)
	os.Exit(2)
}

func run(r *repoReport, exe, cfg, zizmor string, withZizmor bool, mode, src, work string) {
	gh := filepath.Join(src, ".github")
	if st, err := os.Stat(gh); err != nil || !st.IsDir() {
		r.Skipped = "no .github directory"
		return
	}
	if err := copyDir(gh, filepath.Join(work, ".github")); err != nil {
		r.Problems = append(r.Problems, "copy: "+err.Error())
		return
	}
	// The repository's own config would hide findings: the test runs with its own
	_ = os.Remove(filepath.Join(work, ".github", "jactionlint.yaml"))
	_ = os.Remove(filepath.Join(work, ".github", "jactionlint.yml"))
	if err := os.MkdirAll(filepath.Join(work, ".git"), 0o755); err != nil {
		r.Problems = append(r.Problems, err.Error())
		return
	}
	files := yamlFiles(work)
	r.Files = len(files)
	if r.Files == 0 {
		r.Skipped = "no YAML files"
		return
	}
	origHashes := hashAll(files)

	var err error
	if r.JLBefore, err = lintCounts(exe, cfg, work); err != nil {
		r.Problems = append(r.Problems, "lint before: "+err.Error())
		return
	}
	if withZizmor {
		if r.ZBefore, err = zizmorCounts(zizmor, work); err != nil {
			r.Problems = append(r.Problems, "zizmor before: "+err.Error())
			withZizmor = false
		}
	}

	// Fix
	out, code, err := runJL(exe, cfg, work, mode)
	if err != nil {
		r.Problems = append(r.Problems, "fix: "+err.Error())
		return
	}
	if code >= 3 {
		r.Problems = append(r.Problems, fmt.Sprintf("fix exited with status %d: %s", code, firstLines(out.stderr, 6)))
	}
	for _, l := range strings.Split(out.stderr, "\n") {
		if strings.HasPrefix(l, "error:") {
			r.Problems = append(r.Problems, "a fix was refused: "+strings.TrimPrefix(l, "error: "))
		}
	}
	r.ByRule = parseSummary(out.stderr, &r.Applied)

	// 1. valid YAML
	for _, f := range files {
		if err := validYAML(f); err != nil {
			r.Problems = append(r.Problems, fmt.Sprintf("%s is not valid YAML after fixing: %v", rel(work, f), err))
		}
	}
	after := hashAll(files)
	for f, h := range after {
		if origHashes[f] != h {
			r.Changed++
		}
	}

	// 2. the second run changes nothing
	out2, code2, err := runJL(exe, cfg, work, mode)
	if err != nil || code2 >= 3 {
		r.Problems = append(r.Problems, fmt.Sprintf("second fix failed: %v %s", err, firstLines(out2.stderr, 4)))
	}
	if again := hashAll(files); !equalMaps(again, after) {
		r.Problems = append(r.Problems, "a second -fix changed files: "+strings.Join(diffKeys(work, after, again), ", "))
	}
	if out2.applied(&r.Applied) != 0 {
		r.Problems = append(r.Problems, "a second -fix reported fixes")
	}
	diffOut, _, err := runJL(exe, cfg, work, "-diff")
	if err == nil && strings.Contains("\n"+diffOut.stdout, "\n--- ") {
		r.Problems = append(r.Problems, "-diff still prints changes after fixing")
	}

	// 3. findings of jactionlint never go up
	if r.JLAfter, err = lintCounts(exe, cfg, work); err != nil {
		r.Problems = append(r.Problems, "lint after: "+err.Error())
	} else {
		for _, id := range goneUp(r.JLBefore, r.JLAfter) {
			r.Problems = append(r.Problems, fmt.Sprintf("findings of %s went up: %d -> %d", id, r.JLBefore[id], r.JLAfter[id]))
		}
	}

	// 4. findings of zizmor never go up
	if withZizmor {
		if r.ZAfter, err = zizmorCounts(zizmor, work); err != nil {
			r.Problems = append(r.Problems, "zizmor after: "+err.Error())
		} else {
			for _, id := range goneUp(r.ZBefore, r.ZAfter) {
				r.Problems = append(r.Problems, fmt.Sprintf("zizmor findings of %s went up: %d -> %d", id, r.ZBefore[id], r.ZAfter[id]))
			}
		}
	}
}

type jlOutput struct{ stdout, stderr string }

var summaryRe = regexp.MustCompile(`^(?:Fixed|Would fix) (\d+) problem`)

func (o jlOutput) applied(_ *int) int {
	for _, l := range strings.Split(o.stderr, "\n") {
		if m := summaryRe.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			return n
		}
	}
	return 0
}

func parseSummary(stderr string, applied *int) map[string]int {
	by := map[string]int{}
	in := false
	for _, l := range strings.Split(stderr, "\n") {
		if m := summaryRe.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			*applied += n
			in = true
			continue
		}
		if in && strings.HasPrefix(l, "  ") {
			if id, n, ok := strings.Cut(strings.TrimSpace(l), ": "); ok {
				c, _ := strconv.Atoi(n)
				by[id] += c
			}
			continue
		}
		in = false
	}
	return by
}

func runJL(exe, cfg, dir string, args ...string) (jlOutput, int, error) {
	cmd := exec.Command(exe, append([]string{"-no-color", "-config-file", cfg, "-format", "jsonl"}, args...)...)
	cmd.Dir = dir
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return jlOutput{so.String(), se.String()}, ee.ExitCode(), nil
	}
	return jlOutput{so.String(), se.String()}, 0, err
}

func lintCounts(exe, cfg, dir string) (map[string]int, error) {
	out, code, err := runJL(exe, cfg, dir)
	if err != nil {
		return nil, err
	}
	if code >= 2 {
		return nil, fmt.Errorf("exit status %d: %s", code, firstLines(out.stderr, 4))
	}
	counts := map[string]int{}
	for _, l := range strings.Split(out.stdout, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var e struct {
			ID, Kind string
		}
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			return nil, fmt.Errorf("bad jsonl: %w", err)
		}
		id := e.ID
		if id == "" {
			id = e.Kind
		}
		counts[id]++
	}
	return counts, nil
}

func zizmorCounts(zizmor, dir string) (map[string]int, error) {
	parts := strings.Fields(zizmor)
	args := append(parts[1:], "--offline", "--persona", "pedantic", "--format", "json", "--no-exit-codes", "--no-progress", dir)
	cmd := exec.Command(parts[0], args...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%v: %s", err, firstLines(se.String(), 4))
	}
	var findings []struct {
		Ident string `json:"ident"`
	}
	if so.Len() > 0 {
		if err := json.Unmarshal(so.Bytes(), &findings); err != nil {
			return nil, err
		}
	}
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Ident]++
	}
	return counts, nil
}

func goneUp(before, after map[string]int) []string {
	var ids []string
	for id, n := range after {
		if n > before[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func validYAML(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func yamlFiles(dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml")) {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func hashAll(files []string) map[string]string {
	m := map[string]string{}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		h := sha256.Sum256(b)
		m[f] = hex.EncodeToString(h[:])
	}
	return m
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func diffKeys(root string, a, b map[string]string) []string {
	var ks []string
	for k, v := range a {
		if b[k] != v {
			ks = append(ks, rel(root, k))
		}
	}
	sort.Strings(ks)
	return ks
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " | ")
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		r, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, r)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			// Follow a symbolic link to a file, skip the rest
			st, err := os.Stat(p)
			if err != nil || !st.Mode().IsRegular() {
				return nil
			}
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

func sum(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func printReport(w io.Writer, reports []*repoReport, mode string, withZizmor bool) {
	fmt.Fprintf(w, "# fix corpus (%s)\n\n", mode)
	fmt.Fprintln(w, "| repository | YAML files | files changed | fixes applied | jactionlint findings | zizmor findings | result |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|")
	totalJB, totalJA, totalZB, totalZA, totalApplied, totalChanged, totalFiles := 0, 0, 0, 0, 0, 0, 0
	byRule := map[string]int{}
	jb, ja := map[string]int{}, map[string]int{}
	zb, za := map[string]int{}, map[string]int{}
	ok := 0
	for _, r := range reports {
		if r.Skipped != "" {
			fmt.Fprintf(w, "| %s | | | | | | skipped: %s |\n", r.Label, r.Skipped)
			continue
		}
		res := "ok"
		if len(r.Problems) > 0 {
			res = fmt.Sprintf("%d problem(s)", len(r.Problems))
		} else {
			ok++
		}
		zcell := "-"
		if r.ZBefore != nil && r.ZAfter != nil {
			zcell = fmt.Sprintf("%d -> %d", sum(r.ZBefore), sum(r.ZAfter))
		}
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d -> %d | %s | %s |\n", r.Label, r.Files, r.Changed, r.Applied, sum(r.JLBefore), sum(r.JLAfter), zcell, res)
		totalFiles += r.Files
		totalChanged += r.Changed
		totalApplied += r.Applied
		totalJB += sum(r.JLBefore)
		totalJA += sum(r.JLAfter)
		if r.ZBefore != nil && r.ZAfter != nil {
			totalZB += sum(r.ZBefore)
			totalZA += sum(r.ZAfter)
			for k, v := range r.ZBefore {
				zb[k] += v
			}
			for k, v := range r.ZAfter {
				za[k] += v
			}
		}
		for k, v := range r.ByRule {
			byRule[k] += v
		}
		for k, v := range r.JLBefore {
			jb[k] += v
		}
		for k, v := range r.JLAfter {
			ja[k] += v
		}
	}
	fmt.Fprintf(w, "| **total** | %d | %d | %d | %d -> %d | %d -> %d | %d of %d ok |\n\n", totalFiles, totalChanged, totalApplied, totalJB, totalJA, totalZB, totalZA, ok, len(reports))

	fmt.Fprint(w, "## Fixes by rule\n\n")
	fmt.Fprintln(w, "| rule | fixes applied | findings before | findings after |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, id := range sortedKeys(jb, byRule) {
		fmt.Fprintf(w, "| %s | %d | %d | %d |\n", id, byRule[id], jb[id], ja[id])
	}
	if withZizmor && totalZB+totalZA > 0 {
		fmt.Fprint(w, "\n## zizmor findings (pedantic)\n\n")
		fmt.Fprintln(w, "| audit | before | after |")
		fmt.Fprintln(w, "|---|---|---|")
		for _, id := range sortedKeys(zb, za) {
			fmt.Fprintf(w, "| %s | %d | %d |\n", id, zb[id], za[id])
		}
	}
	for _, r := range reports {
		if len(r.Problems) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n## Problems in %s (%s)\n\n", r.Label, r.Dir)
		for _, p := range r.Problems {
			fmt.Fprintf(w, "- %s\n", p)
		}
	}
}

func sortedKeys(ms ...map[string]int) []string {
	seen := map[string]bool{}
	var ks []string
	for _, m := range ms {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				ks = append(ks, k)
			}
		}
	}
	sort.Strings(ks)
	return ks
}

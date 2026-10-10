package jactionlint

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// runHintMinFindings is the number of findings from which a run prints the hint about adopting the checks
// gradually with --baseline-write. A handful of findings is something to fix, not to adopt; the default profile
// reports a median of 37 findings per repository on the corpus, so 20 is below what most repositories see on the
// first run and above what a repository that already follows the checks sees. docs/usage.md names the number.
const runHintMinFindings = 20

// noteRunProfile remembers whether a file was linted with more than the checks of actionlint, which the
// hint at the end of the run needs.
func (l *Linter) noteRunProfile(cfg *Config) {
	if cfg.profile() != ProfileCorrectness {
		l.hintBeyondCorrectness.Store(true)
	}
}

// reportRunHint prints one line to the log output (stderr) after a run of the text format which found many
// findings, saying how to see them counted, how to adopt them gradually and how to get the checks of
// actionlint only. The default profile fails nearly every repository the first time, and the line is the
// answer to "what now?". It is printed only when the caller asked for it (LinterOptions.RunHints) and the
// output is read by a person: a terminal or a CI log. It is never part of a structured format.
func (l *Linter) reportRunHint(results []fileResult) {
	if !l.runHints || !l.hintBeyondCorrectness.Load() {
		return
	}
	if _, ok := l.printer.(textPrinter); !ok {
		return
	}
	if !isTerminalWriter(l.logOut) && !inCI() {
		return
	}
	findings, files, hidden := 0, 0, 0
	for _, r := range results {
		hidden += len(r.baselined)
		if r.baselineFile {
			continue
		}
		n := 0
		for _, e := range r.errs {
			if e.ID != unusedBaselineEntryID {
				n++
			}
		}
		findings += n
		if n > 0 {
			files++
		}
	}
	if findings < runHintMinFindings {
		return
	}
	head := fmt.Sprintf("note: %s in %s", countNoun(findings, "finding"), countNoun(files, "file"))
	var lead string
	if hidden == 0 && !l.hintBaseline.Load() {
		// No baseline yet: this is most likely the first run of the default profile, so say the way in first
		lead = head + ", which is a lot for a first run of the default profile. to adopt the checks without fixing everything first, " +
			"record the findings with --baseline-write: later runs then fail only on new findings"
	} else {
		lead = head
	}
	tips := []string{"see --format summary for the counts per rule", "the name at the end of a finding is its rule ID, which --ignore, the config and ignore comments accept", "for the checks of actionlint only use --profile correctness"}
	fmt.Fprintf(l.logOut, "%s. %s. silence this note with --no-hints or JACTIONLINT_NO_HINTS=1\n", lead, strings.Join(tips, "; "))
}

// isTerminalWriter reports whether w is a terminal.
func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// inCI reports whether the process runs in a CI service, whose log a person reads later.
func inCI() bool {
	if v := os.Getenv("GITHUB_ACTIONS"); v != "" && v != "false" {
		return true
	}
	v := os.Getenv("CI")
	return v != "" && v != "false" && v != "0"
}

// HintsDisabledByEnv reports whether JACTIONLINT_NO_HINTS asks for no hints about the run.
func HintsDisabledByEnv() bool {
	v := os.Getenv("JACTIONLINT_NO_HINTS")
	return v != "" && v != "0" && v != "false"
}

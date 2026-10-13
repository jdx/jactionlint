package jactionlint

import (
	"fmt"
	"strings"
	"time"
)

// runnerLabelRetirement is what is known about the end of life of a label of a GitHub-hosted runner.
type runnerLabelRetirement struct {
	// label is the default label of the runner image, in lower case.
	label string
	// deprecated is the day GitHub started the deprecation (longer queue times). Informational.
	deprecated string
	// retired is the first day on which a job on the label gets no runner (YYYY-MM-DD). From this day on the label is
	// unknown: the rule unknown-runner-label reports it. Before it the label works, and only the pedantic rule
	// deprecated-runner-label warns.
	retired string
	// use lists the labels to move to, best first.
	use []string
	// note adds what else is known before the date: the brownouts, scheduled days on which GitHub fails the jobs on purpose.
	note string
	// source is the announcement.
	source string
}

// runnerLabelRetirements is the one place that records when a label goes away. To record a new deprecation add a row.
// Nothing else has to change on the day of the retirement: the label is reported by unknown-runner-label from then on,
// whatever the version of jactionlint. Keep the rows of retired labels, they give the message of unknown-runner-label.
//
// Last verified against the announcements on 2026-10-13.
var runnerLabelRetirements = []runnerLabelRetirement{
	// Announced, not yet retired
	{label: "macos-14", deprecated: "2026-07-06", retired: "2026-11-02", use: []string{"macos-15", "macos-26"}, source: "https://github.com/actions/runner-images/issues/13518",
		note: "GitHub fails jobs on it on scheduled brownout days in October 2026"},
	{label: "macos-14-large", deprecated: "2026-07-06", retired: "2026-11-02", use: []string{"macos-15-large", "macos-26-large"}, source: "https://github.com/actions/runner-images/issues/13518",
		note: "GitHub fails jobs on it on scheduled brownout days in October 2026"},
	{label: "macos-14-xlarge", deprecated: "2026-07-06", retired: "2026-11-02", use: []string{"macos-15-xlarge", "macos-26-xlarge"}, source: "https://github.com/actions/runner-images/issues/13518",
		note: "GitHub fails jobs on it on scheduled brownout days in October 2026"},
	{label: "ubuntu-22.04", deprecated: "2026-09-17", retired: "2027-04-17", use: []string{"ubuntu-24.04", "ubuntu-26.04"}, source: "https://github.com/actions/runner-images/issues/14254",
		note: "GitHub fails jobs on it on scheduled brownout days before that"},
	{label: "ubuntu-22.04-arm", deprecated: "2026-09-17", retired: "2027-04-17", use: []string{"ubuntu-24.04-arm", "ubuntu-26.04-arm"}, source: "https://github.com/actions/runner-images/issues/14254",
		note: "GitHub fails jobs on it on scheduled brownout days before that"},

	// Retired
	{label: "macos-13", retired: "2025-12-04", use: []string{"macos-15", "macos-26"}, source: "https://github.com/actions/runner-images/issues/13046"},
	{label: "macos-13-large", retired: "2025-12-04", use: []string{"macos-15-large", "macos-15-intel"}, source: "https://github.com/actions/runner-images/issues/13046"},
	{label: "macos-13-xlarge", retired: "2025-12-04", use: []string{"macos-15-xlarge", "macos-26-xlarge"}, source: "https://github.com/actions/runner-images/issues/13046"},
	{label: "macos-12", retired: "2024-12-03", use: []string{"macos-15", "macos-26"}, source: "https://github.com/actions/runner-images/issues/10721"},
	{label: "macos-10.15", retired: "2022-12-01", use: []string{"macos-15", "macos-26"}, source: "https://github.com/actions/runner-images/issues/5583"},
	{label: "ubuntu-20.04", retired: "2025-04-15", use: []string{"ubuntu-24.04", "ubuntu-26.04"}, source: "https://github.com/actions/runner-images/issues/11101"},
	{label: "ubuntu-18.04", retired: "2023-04-03", use: []string{"ubuntu-24.04", "ubuntu-26.04"}, source: "https://github.com/actions/runner-images/issues/6002"},
	{label: "windows-2019", retired: "2025-06-30", use: []string{"windows-2022", "windows-2025"}, source: "https://github.com/actions/runner-images/issues/12045"},
	{label: "windows-2016", retired: "2022-04-01", use: []string{"windows-2022", "windows-2025"}, source: "https://github.com/actions/runner-images/issues/5238"},
}

// retirementDay parses a day of the table. It panics on a malformed one, which the tests catch.
func retirementDay(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(fmt.Sprintf("invalid date %q in runnerLabelRetirements: %v", s, err))
	}
	return t
}

// retiredOn returns the first day on which the label is gone.
func (r *runnerLabelRetirement) retiredOn() time.Time {
	return retirementDay(r.retired)
}

// isRetired reports whether the label is gone at the time. The day of the retirement counts: GitHub removes the image
// on that day.
func (r *runnerLabelRetirement) isRetired(now time.Time) bool {
	y, m, d := now.UTC().Date()
	return !time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Before(r.retiredOn())
}

// runnerLabelRetirementOf returns the row for the label, matching case-insensitively, or nil.
func runnerLabelRetirementOf(label string) *runnerLabelRetirement {
	l := strings.ToLower(label)
	for i := range runnerLabelRetirements {
		if runnerLabelRetirements[i].label == l {
			return &runnerLabelRetirements[i]
		}
	}
	return nil
}

// useText returns `"a" or "b"`.
func (r *runnerLabelRetirement) useText() string {
	q := make([]string, len(r.use))
	for i, u := range r.use {
		q[i] = fmt.Sprintf("%q", u)
	}
	return strings.Join(q, " or ")
}

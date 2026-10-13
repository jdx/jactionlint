package jactionlint

import "strings"

// specificVersionTag picks, from the names of the tags which point to one commit, the version tag
// to write in the comment of a pinned action: the one with the most components ("v4.2.4" over
// "v4.2" and "v4"), the way zizmor, pinact and Renovate name a pin.
//
// given is the tag that was written before the pin ("v4"), or "" when there was none. The result
// must refine it: "v4" can become "v4.2.4" but never "v5.0.0", even when a mislabeled commit carries
// both, so the comment never contradicts what the author asked for. A given tag that is not a
// version ("release", "v1.0.0-rc.1", a tag with build metadata) is kept as it is, and so is the
// choice when the commit has two different versions of the same specificity: nothing then says
// which one the author means. The boolean is false in all of those cases.
func specificVersionTag(names []string, given string) (string, bool) {
	return pickVersionTag(names, given, given == "" || hasVPrefix(given))
}

// pickVersionTag is specificVersionTag with the spelling of the result chosen by the caller: wantV
// prefers the tag written with the "v" when the commit has the same version both ways.
func pickVersionTag(names []string, given string, wantV bool) (string, bool) {
	var gv advisoryVersion
	if given != "" {
		var ok bool
		gv, ok = parseAdvisoryVersion(given)
		if !ok || !semverTagRegex.MatchString(given) || gv.pre != "" || strings.Contains(given, "+") {
			return "", false
		}
	}
	type cand struct {
		name string
		v    advisoryVersion
	}
	var cands []cand
	release := false
	for _, n := range names {
		if !semverTagRegex.MatchString(n) || strings.Contains(n, "+") {
			continue
		}
		v, ok := parseAdvisoryVersion(n)
		if !ok || len(v.nums) < len(gv.nums) || !sameLeadingNumbers(v, gv) {
			continue
		}
		cands = append(cands, cand{n, v})
		release = release || v.pre == ""
	}
	var best []cand
	for _, c := range cands {
		if release && c.v.pre != "" {
			continue // a prerelease does not name the version of a commit which is also a release
		}
		switch {
		case len(best) == 0 || len(c.v.nums) > len(best[0].v.nums):
			best = []cand{c}
		case len(c.v.nums) == len(best[0].v.nums):
			best = append(best, c)
		}
	}
	if len(best) == 0 {
		return "", false
	}
	// The same version may be spelled with and without the "v": follow the way the author writes
	pick := best[0]
	for _, c := range best[1:] {
		if c.v.compare(pick.v) != 0 {
			return "", false // two versions of the same specificity
		}
		if hasVPrefix(c.name) == wantV && hasVPrefix(pick.name) != wantV {
			pick = c
		}
	}
	return pick.name, true
}

func hasVPrefix(name string) bool {
	return strings.HasPrefix(name, "v") || strings.HasPrefix(name, "V")
}

// sameLeadingNumbers reports whether v starts with the numbers of prefix.
func sameLeadingNumbers(v, prefix advisoryVersion) bool {
	for i, n := range prefix.nums {
		if i >= len(v.nums) || v.nums[i] != n {
			return false
		}
	}
	return true
}

// shortPinCommentOption is the option of unpinned-uses which keeps the pin comment of --online --fix short.
var shortPinCommentOption = RuleOption{
	Name: "short-pin-comment", Kind: RuleOptionBool, Default: false,
	Summary: "Make `--online --fix` name the tag as written in the comment of a pin (`# v4`), not the most specific version tag of the commit (`# v4.2.4`).",
}

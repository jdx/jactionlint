package jactionlint

import (
	"fmt"
	"strconv"
	"strings"
)

// advisoryVersion is a version as written in tags and in the ranges of security advisories:
// "1.2.3", "v1.2", "v4", "2.0.0-rc.1". It is deliberately not general: a string which does not look
// like this is not a version, and the callers then say nothing instead of guessing an order.
type advisoryVersion struct {
	nums []int // at least one number
	pre  string
}

// parseAdvisoryVersion parses a dotted version with an optional leading "v" and an optional
// "-prerelease" suffix. Build metadata ("+...") is dropped.
func parseAdvisoryVersion(s string) (advisoryVersion, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v advisoryVersion
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s, v.pre = s[:i], s[i+1:]
		if v.pre == "" {
			return v, false
		}
	}
	if s == "" {
		return v, false
	}
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" || p[0] == '+' {
			return v, false
		}
		v.nums = append(v.nums, n)
	}
	return v, true
}

func (v advisoryVersion) String() string {
	parts := make([]string, len(v.nums))
	for i, n := range v.nums {
		parts[i] = strconv.Itoa(n)
	}
	s := strings.Join(parts, ".")
	if v.pre != "" {
		s += "-" + v.pre
	}
	return s
}

// compare returns -1, 0 or 1. Missing components count as zero ("1.2" equals "1.2.0"), and a
// prerelease is older than the release.
func (v advisoryVersion) compare(o advisoryVersion) int {
	for i := 0; i < max(len(v.nums), len(o.nums)); i++ {
		var a, b int
		if i < len(v.nums) {
			a = v.nums[i]
		}
		if i < len(o.nums) {
			b = o.nums[i]
		}
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
	}
	switch {
	case v.pre == o.pre:
		return 0
	case v.pre == "":
		return 1
	case o.pre == "":
		return -1
	}
	return comparePrerelease(v.pre, o.pre)
}

// comparePrerelease compares dot-separated prerelease identifiers like semver does: numbers
// numerically and before words, words lexically, and a shorter list before a longer one.
func comparePrerelease(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < min(len(as), len(bs)); i++ {
		x, xerr := strconv.Atoi(as[i])
		y, yerr := strconv.Atoi(bs[i])
		switch {
		case xerr == nil && yerr == nil:
			if x != y {
				if x < y {
					return -1
				}
				return 1
			}
		case xerr == nil:
			return -1
		case yerr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

// versionRange is the "vulnerable_version_range" of an advisory: comparators joined with commas, all of
// which must hold. For example ">= 1.0.0, < 1.2.3", "< 4.1.2" or "= 2.0.0".
type versionRange []versionComparator

type versionComparator struct {
	op string
	v  advisoryVersion
}

func parseVersionRange(s string) (versionRange, error) {
	var r versionRange
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			return nil, fmt.Errorf("empty comparator in %q", s)
		}
		op := ""
		for _, o := range []string{"<=", ">=", "==", "<", ">", "="} {
			if strings.HasPrefix(c, o) {
				op = o
				break
			}
		}
		if op == "" {
			return nil, fmt.Errorf("unknown comparator %q in %q", c, s)
		}
		v, ok := parseAdvisoryVersion(strings.TrimSpace(c[len(op):]))
		if !ok {
			return nil, fmt.Errorf("invalid version in comparator %q of %q", c, s)
		}
		if op == "==" {
			op = "="
		}
		r = append(r, versionComparator{op, v})
	}
	return r, nil
}

// contains reports whether the version satisfies every comparator of the range.
func (r versionRange) contains(v advisoryVersion) bool {
	for _, c := range r {
		d := v.compare(c.v)
		var ok bool
		switch c.op {
		case "<":
			ok = d < 0
		case "<=":
			ok = d <= 0
		case ">":
			ok = d > 0
		case ">=":
			ok = d >= 0
		case "=":
			ok = d == 0
		}
		if !ok {
			return false
		}
	}
	return len(r) > 0
}

package jactionlint

import (
	"regexp"
	"slices"
	"strings"
)

// The sources of a ref. Whether the code that a checkout gets can change after a gate is decided from the list of the values
// that make up the ref, not from the text that describes it: each value is one source with a kind, and the ref is mutable
// when any of them is not a commit.

// refKind says what a source is.
type refKind uint8

const (
	// kindCommit is a commit (`head.sha`, `merge_commit_sha`, an output that ends in sha): it cannot change.
	kindCommit refKind = iota
	// kindBranch is a branch of the pull request (`head.ref`, `github.head_ref`, `head_branch`, `refs/heads/...`).
	kindBranch
	// kindPullRef is the ref or the number of a pull request (`refs/pull/N/head`, `gh pr checkout`, `pull/N/head`).
	kindPullRef
	// kindOther is a value that names the pull request and that is not known to be a commit (an input of a dispatch).
	kindOther
)

// refSource is one value that makes up a ref.
type refSource struct {
	kind refKind
	text string
}

// sourcesMutable reports whether the ref can change after a gate: any source that is not a commit. A ref without a known
// source is not.
func sourcesMutable(sources []refSource) bool {
	return slices.ContainsFunc(sources, func(s refSource) bool { return s.kind != kindCommit })
}

var (
	reGhPrCheckout = regexp.MustCompile(`(?i)\bgh\s+pr\s+checkout\b`)
	reContextToken = regexp.MustCompile(`[A-Za-z_][\w-]*(?:\.[\w*-]+)+`)
)

// sourcesOfText returns the sources that a ref or a command makes: the literal refs of a pull request and of a branch
// (also inside an expression: `format('refs/pull/{0}/head', ...)`), `gh pr checkout`, and each context that names an untrusted
// value (a context of the head of a pull request, or the output of a step), by the kind of its last name.
func sourcesOfText(text string) []refSource {
	var out []refSource
	if namesPullRef(text) || reGhPrCheckout.MatchString(text) {
		out = append(out, refSource{kindPullRef, text})
	}
	if strings.Contains(text, "refs/heads/") {
		out = append(out, refSource{kindBranch, text})
	}
	refs, _ := stringExprRefs(text)
	if len(refs) == 0 && !strings.Contains(text, "${{") {
		// a description of a command: the contexts in it are plain words
		for _, tok := range reContextToken.FindAllString(text, -1) {
			refs = append(refs, exprRef{chain: strings.Split(strings.ToLower(tok), ".")})
		}
	}
	for _, r := range refs {
		if k, ok := sourceKindOfChain(r); ok {
			out = append(out, refSource{k, strings.Join(r.chain, ".")})
		}
	}
	return out
}

// sourceKindOfChain classifies a context that names an untrusted value: a commit when it ends in sha (or is the id of the head
// commit), a branch when its name has ref or branch in it. A context that is not untrusted, and a name that says nothing, is not a
// source.
func sourceKindOfChain(r exprRef) (refKind, bool) {
	if len(r.chain) == 0 {
		return 0, false
	}
	if _, ok := untrustedRefs([]exprRef{r}); !ok && !slices.Contains(r.chain, "outputs") {
		return 0, false
	}
	last := r.chain[len(r.chain)-1]
	switch {
	case strings.HasSuffix(last, "sha"), last == "id" && slices.ContainsFunc(r.chain, func(s string) bool { return strings.Contains(s, "commit") }):
		return kindCommit, true
	case strings.Contains(last, "ref"), strings.Contains(last, "branch"):
		return kindBranch, true
	}
	return 0, false
}

var reInputToken = regexp.MustCompile(`(?i)\binputs\.([\w-]+)`)

// sourcesOfDescription returns the sources of a checkout by a command, whose description names what it checks out: the sources of
// the text, and for each input of the callee that the description reads, the sources of what every caller passes (the
// description itself names the first caller only).
func (rule *RuleUntrustedCheckout) sourcesOfDescription(what string) []refSource {
	out := sourcesOfText(what)
	for _, m := range reInputToken.FindAllStringSubmatch(what, -1) {
		name := strings.ToLower(m[1])
		if in, ok := rule.inputTaint[name]; ok {
			for _, src := range in.sources {
				out = append(out, sourcesOfText(src)...)
			}
		} else if slices.Contains(rule.dispatch, name) {
			out = append(out, refSource{kindOther, "inputs." + name})
		}
	}
	return out
}

// refSourcesOfValue returns the sources of the value of a `ref:`: the ones of the text of the value (with the inputs of a
// composite action replaced by the values of the call), and for each input of the callee that it reads, the sources of the
// value that each caller passes. An input of a dispatch that names the pull request is a source that is not known to be a commit.
func (rule *RuleUntrustedCheckout) refSourcesOfValue(value string, subst func(string) string) []refSource {
	v := value
	if subst != nil {
		v = subst(value)
	}
	out := sourcesOfText(v)
	for _, name := range inputsRead(v) {
		if in, ok := rule.inputTaint[name]; ok {
			for _, src := range in.sources {
				out = append(out, sourcesOfText(src)...)
			}
			continue
		}
		if slices.Contains(rule.dispatch, name) {
			out = append(out, refSource{kindOther, "inputs." + name})
		}
	}
	return out
}

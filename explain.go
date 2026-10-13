package jactionlint

import (
	_ "embed" // the checks document is part of the binary, for --explain
	"fmt"
	"io"
	"regexp"
	"strings"
)

// checksDoc is docs/checks.md, which describes every check with examples. --explain prints the section of a
// rule from it, so offline use and coding agents get the documentation of the installed version.
//
//go:embed docs/checks.md
var checksDoc string

var reAnchorLine = regexp.MustCompile(`^<a id="([^"]+)"></a>\s*$`)

// checksSection returns the section of docs/checks.md which starts at the anchor, as Markdown: from its heading
// to the next heading of the same level. It is empty when there is no such anchor.
func checksSection(doc, anchor string) string {
	lines := strings.Split(doc, "\n")
	start := -1
	for i, l := range lines {
		if m := reAnchorLine.FindStringSubmatch(l); m != nil && m[1] == anchor {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	// A section can have several anchors in a row
	for start < len(lines) && reAnchorLine.MatchString(lines[start]) {
		start++
	}
	end := len(lines)
	seenHeading := false
	inFence := false
	for i := start; i < len(lines); i++ {
		l := lines[i]
		if strings.HasPrefix(l, "```") {
			inFence = !inFence
		}
		if inFence {
			continue
		}
		if reAnchorLine.MatchString(l) {
			end = i
			break
		}
		if strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "# ") {
			if seenHeading {
				end = i
				break
			}
			seenHeading = true
		}
	}
	// The comments for the tools which check the document are noise in a terminal
	var kept []string
	inFence = false
	for _, l := range lines[start:end] {
		if strings.HasPrefix(l, "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(l, "<!--") && strings.HasSuffix(strings.TrimSpace(l), "-->") {
			continue
		}
		kept = append(kept, l)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// docsBase is where the documents of the version are published.
const docsBase = "https://jactionlint.jdx.dev/"

// explainRule writes the documentation of the rule: what it says in the registry (summary, group, profile, level,
// options, fixes), where the documentation is and the section of the checks document.
func explainRule(w io.Writer, id string) error {
	info, ok := ruleIndex[id]
	if !ok {
		if rr, ok := lookupRenamed(id); ok {
			return &usageError{fmt.Errorf("the rule ID %q was merged into %q before 2.0. use --explain %s", id, rr.ID, rr.ID)}
		}
		return &usageError{fmt.Errorf("unknown rule ID %q in --explain%s", id, suggestRuleID(id))}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n\n", info.ID, strings.Repeat("=", len(info.ID)))
	fmt.Fprintf(&b, "%s\n\n", info.Summary)
	fmt.Fprintf(&b, "Group:          %s\n", info.Group)
	fmt.Fprintf(&b, "Profile:        %s\n", explainProfile(info))
	fmt.Fprintf(&b, "Default level:  %s\n", info.DefaultLevel)
	if info.Fixable {
		b.WriteString("Fix:            yes. --fix applies the safe fixes; a fix which may change the behavior of the workflow needs --fix=unsafe\n")
	} else {
		b.WriteString("Fix:            no\n")
	}
	fmt.Fprintf(&b, "Rules page:     %s\n", info.DocURL())
	if info.DocsAnchor != "" {
		fmt.Fprintf(&b, "Details:        %schecks#%s\n", docsBase, info.DocsAnchor)
	}
	b.WriteString("\nConfiguration (.github/jactionlint.yaml):\n")
	fmt.Fprintf(&b, "  rules:\n    %s: %s    # error, warn, info or off\n", info.ID, info.DefaultLevel)
	fmt.Fprintf(&b, "\nIgnore a finding with a comment above the line:\n  # jactionlint ignore=%s\n", info.ID)
	if len(info.Options) > 0 {
		b.WriteString("\nOptions (in \"rules\"):\n")
		for _, o := range info.Options {
			def := "no default"
			if o.Default != nil {
				def = fmt.Sprintf("default %v", o.Default)
			}
			fmt.Fprintf(&b, "  %s (%s, %s)\n      %s\n", o.Name, o.Kind, def, o.Summary)
		}
	}
	if info.DocsAnchor != "" {
		if sec := checksSection(checksDoc, info.DocsAnchor); sec != "" {
			b.WriteString("\n---\n\n")
			b.WriteString(sec)
			b.WriteString("\n")
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func explainProfile(r *RuleInfo) string {
	switch {
	case r.Online:
		return "only with --online (independent of the profile)"
	case r.Profile == "":
		return "none: it runs only when the configuration turns it on"
	case r.Profile == ProfileCorrectness:
		return "correctness (so also default and pedantic)"
	case r.Profile == ProfileDefault:
		return "default (so also pedantic)"
	}
	return string(r.Profile)
}

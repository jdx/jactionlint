package jactionlint

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// RuleUnpinnedImages is a rule checker which reports container images of `container:` and
// `services:` that are not pinned by a digest. An image without a tag, or with the tag "latest", is
// the worst case: the registry decides what runs. A tag other than "latest" is mutable too, so it is
// reported unless the "require-digest" option is turned off.
//
// An image given as `${{ matrix.name }}` is checked at the values the matrix writes for that name.
//
// Docker images in `uses: docker://...` are checked by the unpinned-uses rule.
type RuleUnpinnedImages struct {
	RuleBase
}

// NewRuleUnpinnedImages creates a new RuleUnpinnedImages instance.
func NewRuleUnpinnedImages() *RuleUnpinnedImages {
	return &RuleUnpinnedImages{
		RuleBase: RuleBase{
			name: "unpinned-images",
			desc: "Checks that the container images of \"container:\" and \"services:\" are pinned by a digest",
		},
	}
}

// matrixImageRegex matches an image which is the value of one matrix variable.
var matrixImageRegex = regexp.MustCompile(`^\$\{\{\s*matrix\.([A-Za-z0-9_-]+)\s*\}\}$`)

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUnpinnedImages) VisitJobPre(n *Job) error {
	if !rule.Config().RuleEnabled("unpinned-images") {
		return nil
	}
	rule.checkContainer(n, n.Container, func(img string) string { return fmt.Sprintf("container image %q", img) })
	if n.Services == nil || n.Services.Expression != nil {
		return nil
	}
	ids := make([]string, 0, len(n.Services.Value))
	for id := range n.Services.Value {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if s := n.Services.Value[id]; s != nil {
			rule.checkContainer(n, s.Container, func(img string) string { return fmt.Sprintf("image %q of service %q", img, id) })
		}
	}
	return nil
}

func (rule *RuleUnpinnedImages) checkContainer(job *Job, c *Container, subject func(image string) string) {
	if c == nil || c.Image == nil || c.Image.Value == "" {
		return
	}
	if !c.Image.ContainsExpression() {
		rule.checkImage(c.Image.Value, c.Image.Pos, subject)
		return
	}
	// An image taken from the matrix is checked where the matrix writes it
	m := matrixImageRegex.FindStringSubmatch(strings.TrimSpace(c.Image.Value))
	if m == nil || job.Strategy == nil || job.Strategy.Matrix == nil {
		return
	}
	name := strings.ToLower(m[1])
	matrix := job.Strategy.Matrix
	var values []RawYAMLValue
	if row, ok := matrix.Rows[name]; ok && row != nil {
		values = append(values, row.Values...)
	}
	if matrix.Include != nil {
		for _, comb := range matrix.Include.Combinations {
			if a, ok := comb.Assigns[name]; ok && a != nil {
				values = append(values, a.Value)
			}
		}
	}
	for _, v := range values {
		if s, ok := v.(*RawYAMLString); ok && s.Value != "" && !ContainsExpression(s.Value) {
			rule.checkImage(s.Value, s.Pos(), subject)
		}
	}
}

func (rule *RuleUnpinnedImages) checkImage(img string, pos *Pos, subject func(image string) string) {
	requireDigest, ok := rule.Config().ruleOptionBool("unpinned-images", "require-digest")
	if !ok {
		requireDigest = true
	}

	what := subject(img)
	ref := ParseUses("docker://" + img)
	if ref.Kind != UsesDocker {
		return
	}
	if hasImageDigest(ref.Digest) {
		return
	}

	switch {
	case ref.Digest != "":
		rule.ReportIDf("unpinned-images", pos, "%s has the digest %q, which is not a valid sha256 digest like \"sha256:{64 hex digits}\"", what, ref.Digest)
	case !ref.HasTag:
		rule.ReportIDf("unpinned-images", pos, "%s has no tag, so the registry decides which image is pulled (\"latest\"). pin it to a digest like \"%s@sha256:{digest}\"", what, ref.Image)
	case ref.Tag == "latest":
		rule.ReportIDf("unpinned-images", pos, "%s uses the tag \"latest\", which changes whenever the registry is updated. pin it to a digest like \"%s@sha256:{digest}\"", what, ref.Image)
	case requireDigest:
		rule.ReportIDf("unpinned-images", pos, "%s is pinned by a tag, which can be moved to another image. pin it to a digest like \"%s:%s@sha256:{digest}\"", what, ref.Image, ref.Tag)
	}
}

// hasImageDigest reports whether the digest of an image reference is a content digest.
func hasImageDigest(d string) bool {
	algo, hex, ok := strings.Cut(strings.ToLower(d), ":")
	if !ok || !isHex(hex) {
		return false
	}
	switch algo {
	case "sha256":
		return len(hex) == 64
	case "sha512":
		return len(hex) == 128
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{
			ID: "unpinned-images", Group: RuleGroupSecurity, Summary: "A container or service image is not pinned by a digest.",
			DefaultLevel: SeverityWarning, Profile: ProfileStrict, DocsAnchor: "check-unpinned-images",
			Options: []RuleOption{{Name: "require-digest", Kind: RuleOptionBool, Default: true, Summary: "Report images pinned by a tag other than latest too. Turn it off to report only images without a tag or with the latest tag."}},
		},
	)
	registerRuleFactory("unpinned-images", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleUnpinnedImages()}
	})
}

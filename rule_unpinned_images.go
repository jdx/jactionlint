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
	runContext
	// local are the images that the steps of the job build or tag, and localUnknown is true when they also load images
	// some other way (see localImagesOf). It is worked out when the first image of a script of the job is found.
	local        []string
	localDone    bool
	localUnknown bool
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
var matrixImageRegex = regexp.MustCompile(`^\$\{\{\s*matrix((?:\.[A-Za-z0-9_-]+)+)\s*\}\}$`)

// imageExprRegex finds the expressions of an image and captures what is inside of them.
var imageExprRegex = regexp.MustCompile(`\$\{\{(.*?)\}\}`)

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children. It checks the
// "image" of a Docker action (action.yml), which is a registry image when it starts with "docker://"
// and a Dockerfile otherwise.
func (rule *RuleUnpinnedImages) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	if n.Action == nil || n.Action.Runs == nil || n.Action.Runs.Image == nil || !rule.Config().RuleEnabled("unpinned-images") {
		return nil
	}
	img := n.Action.Runs.Image
	if ref, ok := strings.CutPrefix(img.Value, "docker://"); ok {
		rule.checkImage(ref, img.Pos, func(image string) string { return fmt.Sprintf("image %q of this Docker action", image) })
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUnpinnedImages) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	rule.local, rule.localDone, rule.localUnknown = nil, false, false
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
	img := strings.TrimSpace(c.Image.Value)
	// An image taken from the matrix is checked where the matrix writes it
	if m := matrixImageRegex.FindStringSubmatch(img); m != nil {
		values, complete := matrixStrings(job, m[1])
		for _, v := range values {
			rule.checkImage(v.Value, v.Pos(), subject)
		}
		if !complete && rule.pedantic("unpinned-images") {
			// some value is only known when the workflow runs
			rule.ReportIDf("unpinned-images", c.Image.Pos, "%s comes from an expression, so whether it is pinned by a digest is only known when the workflow runs. make the value end with a digest like \"image:tag@sha256:{digest}\"", subject(img))
		}
		return
	}
	// An image which is made of text and matrix values (`postgres:${{ matrix.postgres }}`) is checked for each
	// combination of the values, at the image
	if images, ok := matrixImages(job, img); ok {
		reported := map[string]bool{}
		for _, v := range images {
			if !reported[v] {
				reported[v] = true
				rule.checkImage(v, c.Image.Pos, subject)
			}
		}
		return
	}
	// Any other expression is a value which is only known when the workflow runs. A digest is the last part of an
	// image, so when the text after the last expression is written out (`${{ needs.build.outputs.image }}-bookworm`,
	// `${{ vars.REGISTRY }}/tool:1.2`) and holds no digest, no value of the expression can make the image pinned.
	// When the image ends with an expression, the expression may be the digest, which is not known.
	end := strings.LastIndex(img, "}}") + 2
	tail := img[end:]
	switch {
	case strings.Contains(strings.ToLower(img), "@sha256:"):
		// pinned, or at least written as a digest
	case tail != "":
		rule.ReportIDf("unpinned-images", c.Image.Pos, "%s is not pinned by a digest: it ends with %q after the expression, so no value of the expression can add the digest. pin it to a digest like \"image:tag@sha256:{digest}\"", subject(img), tail)
	case rule.pedantic("unpinned-images"):
		rule.ReportIDf("unpinned-images", c.Image.Pos, "%s comes from an expression, so whether it is pinned by a digest is only known when the workflow runs. make the value end with a digest like \"image:tag@sha256:{digest}\"", subject(img))
	}
}

// matrixImageRefRegex matches the matrix references an image can be built from: `matrix.name` and
// `matrix.name.key`.
var matrixImageRefRegex = regexp.MustCompile(`^matrix((?:\.[A-Za-z0-9_-]+)+)$`)

// matrixStrings returns the string values that the matrix of the job has for the reference (`name` or `name.key`),
// each with its position. A value that is not a plain string (an expression, a list) is left out, and complete is false
// then: the values are not all known. It is false too when the matrix is an expression or has no value for the name.
func matrixStrings(job *Job, ref string) (out []*RawYAMLString, complete bool) {
	if job.Strategy == nil || job.Strategy.Matrix == nil || job.Strategy.Matrix.Expression != nil {
		return nil, false
	}
	path := strings.Split(strings.ToLower(strings.TrimPrefix(ref, ".")), ".")
	matrix := job.Strategy.Matrix
	var values []RawYAMLValue
	if row, ok := matrix.Rows[path[0]]; ok && row != nil {
		if row.Expression != nil {
			return nil, false
		}
		values = append(values, row.Values...)
	}
	if matrix.Include != nil {
		for _, comb := range matrix.Include.Combinations {
			if a, ok := comb.Assigns[path[0]]; ok && a != nil {
				values = append(values, a.Value)
			}
		}
	}
	complete = len(values) > 0
	for _, v := range values {
		for _, key := range path[1:] {
			o, ok := v.(*RawYAMLObject)
			if !ok {
				v = nil
				break
			}
			v = o.Props[key]
		}
		if o, ok := v.(*RawYAMLObject); ok {
			// `container: ${{ matrix.container }}` where the matrix writes the whole container: its image is the one to check
			v = o.Props["image"]
		}
		if s, ok := v.(*RawYAMLString); ok && s.Value != "" && !ContainsExpression(s.Value) {
			out = append(out, s)
		} else {
			complete = false
		}
	}
	return out, complete
}

// maxMatrixImages bounds the images built from a matrix: more combinations than this are not expanded.
const maxMatrixImages = 64

// matrixImages builds the images that an image written as text and matrix references can be, one for each combination
// of values. It returns false when some expression is not a plain matrix reference or has a value that is not known
// (an expression, a matrix that fromJSON() writes), because the image is then not known either.
func matrixImages(job *Job, img string) ([]string, bool) {
	locs := imageExprRegex.FindAllStringSubmatchIndex(img, -1)
	if len(locs) == 0 {
		return nil, false
	}
	images := []string{""}
	last := 0
	for _, loc := range locs {
		m := matrixImageRefRegex.FindStringSubmatch(strings.TrimSpace(img[loc[2]:loc[3]]))
		if m == nil {
			return nil, false
		}
		values, complete := matrixStrings(job, m[1])
		if !complete {
			return nil, false
		}
		var next []string
		for _, prefix := range images {
			for _, v := range values {
				next = append(next, prefix+img[last:loc[0]]+v.Value)
			}
		}
		if len(next) > maxMatrixImages {
			return nil, false
		}
		images = next
		last = loc[1]
	}
	for i := range images {
		images[i] += img[last:]
	}
	return images, true
}

func (rule *RuleUnpinnedImages) checkImage(img string, pos *Pos, subject func(image string) string) {
	requireDigest, ok := rule.Config().ruleOptionBool("unpinned-images", "require-digest")
	if !ok {
		requireDigest = true
	}
	// The policy of the first pattern of "policies" which matches the image wins over require-digest
	fallback := imagePolicyHashPin
	if !requireDigest {
		fallback = imagePolicyTagPin
	}
	policies, _ := rule.Config().ruleOptionStringMap("unpinned-images", "policies")
	policy := imagePolicyFor(policies, img, fallback)
	if policy == imagePolicyAny {
		return
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
	case policy == imagePolicyHashPin:
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
			DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-unpinned-images",
			Options: []RuleOption{
				{Name: "require-digest", Kind: RuleOptionBool, Default: true, Summary: "Report images pinned by a tag other than latest too. Turn it off to report only images without a tag or with the latest tag."},
				unpinnedImagesPolicies,
				pedanticOption,
			},
		},
	)
	registerRuleFactory("unpinned-images", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleUnpinnedImages()}
	})
}

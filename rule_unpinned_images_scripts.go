package jactionlint

import (
	"fmt"
	"strings"
)

// unpinnedImagesPolicies is the "policies" option of the unpinned-images rule.
var unpinnedImagesPolicies = RuleOption{
	Name: "policies", Kind: RuleOptionStringMap, Default: map[string]string{},
	Summary:  "How strongly to pin the images matching a pattern: hash-pin (a digest, the default of require-digest), tag-pin (a tag other than latest, or a digest) or any. The most specific pattern that matches wins over require-digest: more literal characters before the first *, in the pattern with the registry written out, then fewer *, then a tag, then the longer pattern. Patterns are globs of the image name, where * matches any characters: \"*\", \"ghcr.io/my-org/*\", \"registry.example.com/team/tool\" or \"node:20\". The registry of Docker Hub can be left out (\"node\" is \"docker.io/library/node\").",
	Validate: validateImagePolicies,
}

// buildPushActions are docker/build-push-action and its drop-in replacements, which take its "tags", "load", "push" and
// "outputs" inputs: the images they build are named by "tags".
var buildPushActions = map[string]bool{
	"docker/build-push-action":        true,
	"depot/build-push-action":         true,
	"useblacksmith/build-push-action": true,
	"warpbuilds/build-push-action":    true,
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleUnpinnedImages) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node. It checks the images that `docker run`, `docker create` and
// `docker pull` (also of podman) use in a script.
func (rule *RuleUnpinnedImages) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || !rule.Config().RuleEnabled("unpinned-images") {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	for _, c := range s.Commands {
		img, ok := dockerImagesOf(c)
		if !ok {
			continue
		}
		if !rule.localDone {
			rule.collectLocalImages()
		}
		if rule.localUnknown || rule.isLocalImage(img.Ref) {
			continue
		}
		rule.checkImage(img.Ref, scriptPos(s, origin, img.Word.Offset), func(image string) string {
			return fmt.Sprintf("image %q of %q in this script", image, img.Command)
		})
	}
	return nil
}

// collectLocalImages reads the steps of the job for the images that they build, tag or load: an image built there is
// not one of a registry, and the registry cannot pin it. Each job runs on a runner of its own, so an image which
// another job builds or loads is not there: it is pulled from the registry like any other.
func (rule *RuleUnpinnedImages) collectLocalImages() {
	rule.localDone = true
	if rule.job == nil {
		return
	}
	walkSteps(rule.job.Steps, func(st *Step) {
		switch e := st.Exec.(type) {
		case *ExecRun:
			if s, _ := rule.script(e); s != nil {
				names, unknown := localImagesOf(s)
				rule.local = append(rule.local, names...)
				rule.localUnknown = rule.localUnknown || unknown
			}
		case *ExecAction:
			if e.Uses == nil {
				return
			}
			name := strings.ToLower(ParseUses(e.Uses.Value).CanonicalName())
			switch {
			case buildPushActions[name]:
				// the tags it builds ("tags:", multi-line or separated by commas) are local images
				rule.addImageList(e, "tags")
			case name == "docker/metadata-action":
				// its "tags" are rules for the tags of the images, not image names; "images" names the images it is for
				rule.addImageList(e, "images")
			case name == "docker/bake-action":
				rule.localUnknown = true
			case name == "docker/load-action":
				rule.localUnknown = true
			}
		}
	})
}

// isLocalImage reports whether the image is one of the images that the job builds: the same literal name, whatever
// the tag. A name which is written with an expression or a shell variable is not one that can be compared, so it
// suppresses nothing: the suppression is only for an image that is provably built here.
func (rule *RuleUnpinnedImages) isLocalImage(ref string) bool {
	name, _, _ := imageRefParts(ref)
	name = normalizedImageName(name)
	for _, l := range rule.local {
		n, _, _ := imageRefParts(l)
		if !isLiteralImageName(n) {
			continue
		}
		if normalizedImageName(n) == name {
			return true
		}
	}
	return false
}

// isLiteralImageName reports whether the name of an image is plain text: no expression, variable, substitution or glob.
func isLiteralImageName(n string) bool {
	return n != "" && !strings.ContainsAny(n, "${}*?`()\\\"' \t")
}

// addImageList adds the names of a list input (lines or commas) of an action to the local images.
func (rule *RuleUnpinnedImages) addImageList(e *ExecAction, input string) {
	if in := e.Inputs[input]; in != nil && in.Value != nil {
		rule.local = append(rule.local, strings.FieldsFunc(in.Value.Value, func(r rune) bool { return r == '\n' || r == ',' || r == ' ' })...)
	}
}

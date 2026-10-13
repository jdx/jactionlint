package jactionlint

import (
	"fmt"
	"strings"
)

// unpinnedImagesPolicies is the "policies" option of the unpinned-images rule.
var unpinnedImagesPolicies = RuleOption{
	Name: "policies", Kind: RuleOptionStringMap, Default: map[string]string{},
	Summary:  "How strongly to pin the images matching a pattern: hash-pin (a digest, the default of require-digest), tag-pin (a tag other than latest, or a digest) or any. The most specific pattern that matches (one with a tag, then the most literal characters) wins over require-digest. Patterns are globs of the image name, where * matches any characters: \"*\", \"ghcr.io/my-org/*\", \"registry.example.com/team/tool\" or \"node:20\". The registry of Docker Hub can be left out (\"node\" is \"docker.io/library/node\").",
	Validate: validateImagePolicies,
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

// collectLocalImages reads the scripts of the workflow for the images that they build, tag or load: an image built
// there is not one of a registry, and the registry cannot pin it.
func (rule *RuleUnpinnedImages) collectLocalImages() {
	rule.localDone = true
	w := rule.wf
	if w == nil {
		return
	}
	visit := func(steps []*Step) {
		walkSteps(steps, func(st *Step) {
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
				switch name {
				case "docker/build-push-action", "docker/bake-action", "docker/metadata-action":
					// the tags it builds ("tags:", multi-line or separated by commas) are local images
					if in := e.Inputs["tags"]; in != nil && in.Value != nil {
						rule.local = append(rule.local, strings.FieldsFunc(in.Value.Value, func(r rune) bool { return r == '\n' || r == ',' || r == ' ' })...)
					}
					if name == "docker/bake-action" {
						rule.localUnknown = true
					}
				case "docker/load-action":
					rule.localUnknown = true
				}
			}
		})
	}
	for _, j := range w.Jobs {
		if j != nil {
			visit(j.Steps)
		}
	}
}

// isLocalImage reports whether the image is one of the images that the workflow builds: the same name, whatever the tag.
func (rule *RuleUnpinnedImages) isLocalImage(ref string) bool {
	name, _, _ := imageRefParts(ref)
	name = normalizedImageName(name)
	for _, l := range rule.local {
		n, _, _ := imageRefParts(l)
		if strings.Contains(n, "$") {
			// a name which is built from an expression or a variable can be any image
			return true
		}
		if normalizedImageName(n) == name {
			return true
		}
	}
	return false
}

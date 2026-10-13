package jactionlint

import (
	"errors"
	"fmt"
	"strings"
)

// attachImageFixes gives the "unpinned-images" errors, and the "unpinned-uses" errors of `docker://` images, the fix which
// pins the image to the digest that its tag has in the registry:
//
//	image: postgres:16
//	image: postgres:16@sha256:...
//
// It is the format Renovate writes and reads. The fix needs the registry, so it exists only with the online checks, and
// it is unsafe: the image behind a tag moves, and the digest is the one of today. It is also what runs today
// (`docker pull` of the tag resolves to the same digest), so the fix pins what runs now; it never
// updates a pin to a newer image. An image without a tag is the tag "latest", as for Docker, and gets
// `image:latest@sha256:...`. The images of `container:`, of `services:`, of the matrix which they come from, of
// `uses: docker://` and of `runs.image: docker://` of an action are handled. Docker Hub and ghcr.io
// are asked, other registries are not (the finding says so).
func (l *Linter) attachImageFixes(sess *onlineSession, src []byte, errs []*Error) {
	var starts []int // Byte offset of each line
	for _, e := range errs {
		if (e.ID != "unpinned-images" && e.ID != "unpinned-uses") || e.Fix != nil || e.Line <= 0 || e.Column <= 0 {
			continue
		}
		if starts == nil {
			starts = lineStarts(src)
		}
		if e.Line > len(starts) {
			continue
		}
		fix, err := imageFix(sess, src, starts[e.Line-1], e.Column, e.ID == "unpinned-uses")
		if err != nil {
			if e.ID == "unpinned-uses" && errors.Is(err, errNotAnImage) {
				continue // A tag of an action: the pin fix is the one to ask
			}
			l.debug("No fix for the image at line %d: %v", e.Line, err)
			e.NoFix = noFixOf(err)
			continue
		}
		e.Fix, e.NoFix = fix, nil
	}
}

// errNotAnImage is why a `uses:` value is not for this fix: it is an action, whose pin fix is in online_pin.go.
var errNotAnImage = errors.New("not a registry image")

// imageFix builds the fix for the image whose value starts at the column (1-based, in characters) of the line which
// starts at lineStart. With uses the value is a `uses:` and must start with "docker://".
func imageFix(sess *onlineSession, src []byte, lineStart, col int, uses bool) (*Fix, error) {
	v, err := scanValue(src, lineStart, col)
	if err != nil {
		return nil, err
	}
	if uses && !strings.HasPrefix(v.text, "docker://") {
		return nil, errNotAnImage
	}
	img := strings.TrimPrefix(v.text, "docker://")
	if strings.Contains(img, "${{") {
		return nil, shapeErr("the image %q comes from an expression, which is only known when the workflow runs", img)
	}
	if rest := strings.TrimSpace(string(v.line[v.tokenEnd:])); rest != "" && !strings.HasPrefix(rest, "#") {
		return nil, shapeErr("the line has other content after the image")
	}
	ref := ParseUses("docker://" + img)
	if ref.Kind != UsesDocker {
		return nil, shapeErr("%q is not an image", img)
	}
	if ref.Digest != "" {
		return nil, judgmentErr("the image has the digest %q, which is not a valid sha256 digest: fix it by hand", ref.Digest)
	}
	tag := ref.Tag
	switch {
	case !ref.HasTag:
		tag = "latest" // What Docker pulls for an image without a tag
	case tag == "":
		return nil, shapeErr("the tag of the image is empty")
	}
	reg, reason := parseRegistryImage(ref.Image)
	if reason != "" {
		return nil, judgmentErr("%s. pin the image with a digest from your registry, like \"image:tag@sha256:{digest}\"", reason)
	}
	if sess == nil {
		return nil, &pinFixError{code: NoFixOnlineRequired, msg: "pinning an image needs its registry to resolve the tag, run with --online to enable the fix"}
	}
	digest, err := sess.ImageDigest(reg, tag)
	if err != nil {
		if errors.Is(err, ErrImageNotFound) {
			return nil, judgmentErr("the registry %s has no image %s:%s, or it is private: pin it by hand", reg.registry, ref.Image, tag)
		}
		return nil, &pinFixError{code: NoFixLookupFailed, err: err,
			msg: fmt.Sprintf("pinning the image needs %s to resolve the tag and the lookup failed: %v", reg.registry, err)}
	}
	add := "@" + digest
	if !ref.HasTag {
		add = ":latest" + add
	}
	at := lineStart + v.valueEnd
	return &Fix{
		Description: fmt.Sprintf("Pin %s:%s to %s, the digest the tag has in %s today. Unsafe: the image behind a tag moves, and the digest is the image of this moment", ref.Image, tag, digest[:min(len(digest), 19)], reg.registry),
		Unsafe:      true,
		Edits:       []TextEdit{{Start: at, End: at, NewText: add}},
	}, nil
}

// onlineSessionOrNil returns the online session, or nil when the online checks are off or cannot run.
func (l *Linter) onlineSessionOrNil(cfg *Config) *onlineSession {
	sess, _ := l.onlineSession(cfg)
	return sess
}

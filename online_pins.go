package jactionlint

import (
	"fmt"
	"strings"
)

// maxTransitivePinDepth is how far pinned actions are followed: the steps of the action a workflow uses
// (depth 1) and the steps of the commit-pinned actions those steps use (depth 2). A deeper chain is not read.
const maxTransitivePinDepth = 2

type pinProblemKind int

const (
	// pinStepAction is a composite step which uses an action by a ref which is not a commit SHA.
	pinStepAction pinProblemKind = iota
	// pinStepImage is a composite step which uses a Docker image without a digest.
	pinStepImage
	// pinImage is the Docker image without a digest which a Docker action runs.
	pinImage
)

// pinProblem is a dependency which a pinned action pulls in by a name that can move.
type pinProblem struct {
	kind pinProblemKind
	// target is the `uses:` value of the step, or the image of the action.
	target string
	// holder is the commit-pinned action which has the dependency, when it is not the pinned action itself but
	// one that it uses.
	holder string
}

func (p pinProblem) String() string {
	const noSHA = "a ref which can move and not a commit SHA"
	const noDigest = "a tag and not a digest"
	if p.holder == "" {
		switch p.kind {
		case pinStepAction:
			return fmt.Sprintf("a composite step uses %q, %s", p.target, noSHA)
		case pinStepImage:
			return fmt.Sprintf("a composite step uses the Docker image %q, %s", p.target, noDigest)
		}
		return fmt.Sprintf("it runs the Docker image %q, %s", p.target, noDigest)
	}
	switch p.kind {
	case pinStepAction:
		return fmt.Sprintf("%q, which it uses, has a composite step which uses %q, %s", p.holder, p.target, noSHA)
	case pinStepImage:
		return fmt.Sprintf("%q, which it uses, has a composite step which uses the Docker image %q, %s", p.holder, p.target, noDigest)
	}
	return fmt.Sprintf("%q, which it uses, runs the Docker image %q, %s", p.holder, p.target, noDigest)
}

type pinProblemsKey struct {
	repoKey
	subpath, sha string
}

// PinProblems reads the action owner/repo[/subpath] at the commit and tells what it pulls in by a name that can
// move: a composite step which uses an action by a tag or a branch, or a Docker image by a tag, and the image of
// a Docker action. The commit-pinned actions which its steps use are read too, to the depth of
// maxTransitivePinDepth, once each (an action which uses itself, directly or not, ends the walk). The result is
// empty when the action pins everything it uses, or when its metadata is not known; the error says why
// the metadata of the action could not be read. The metadata of a nested action which cannot be read is
// skipped, and counted as a skipped lookup by the session.
//
// The Dockerfile of a Docker action which builds its image is not read.
func (s *onlineSession) PinProblems(owner, repo, subpath, sha string) ([]pinProblem, error) {
	subpath = strings.Trim(subpath, "/")
	sha = normalizeFileRef(sha)
	return s.pins.get(pinProblemsKey{newRepoKey(owner, repo), subpath, sha}, func() ([]pinProblem, error) {
		w := pinWalk{s: s, max: maxTransitivePinDepth, seen: map[string]bool{}}
		ps, err := w.walk(owner, repo, subpath, sha, 1, "")
		return uniquePinProblems(ps), err
	})
}

// uniquePinProblems drops the repeated problems: an action which several steps pin to the same commit is walked for
// each of them, and a repeat must not use up the places of the dependencies a finding lists.
func uniquePinProblems(ps []pinProblem) []pinProblem {
	seen := map[pinProblem]bool{}
	var out []pinProblem
	for _, p := range ps {
		k := pinProblem{kind: p.kind, target: p.target, holder: strings.ToLower(p.holder)}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	return out
}

type pinWalk struct {
	s    *onlineSession
	max  int             // how many levels are read, see maxTransitivePinDepth
	seen map[string]bool // the actions on the path from the pinned action, to end a cycle
}

func actionSpec(owner, repo, subpath, ref string) string {
	spec := owner + "/" + repo
	if subpath != "" {
		spec += "/" + subpath
	}
	return spec + "@" + ref
}

func (w *pinWalk) walk(owner, repo, subpath, sha string, depth int, holder string) ([]pinProblem, error) {
	key := strings.ToLower(actionSpec(owner, repo, subpath, sha))
	if w.seen[key] {
		return nil, nil
	}
	w.seen[key] = true
	defer delete(w.seen, key)

	meta, err := w.s.RemoteAction(owner, repo, subpath, sha)
	if err != nil || meta == nil {
		return nil, err
	}
	var out []pinProblem
	add := func(kind pinProblemKind, target string) {
		out = append(out, pinProblem{kind: kind, target: target, holder: holder})
	}

	switch meta.Runs.Using {
	case "docker":
		if img := meta.Runs.Image; img != "" {
			if ref, ok := dockerImageRef(img); ok && ref.Digest == "" {
				add(pinImage, img)
			}
		}
	case "composite":
		// stepsUses reads the steps of the parallel groups too
		for _, spec := range stepsUses(meta.Runs.Steps) {
			if spec == "" {
				continue
			}
			u := ParseUses(spec)
			switch {
			case u.Dynamic:
				// Not known
			case u.Kind == UsesDocker:
				if u.Digest == "" {
					add(pinStepImage, spec)
				}
			case u.Kind == UsesAction && u.RefKind == RefFullSHA:
				if depth >= w.max || !remoteActionLookup(u) {
					continue
				}
				nested, err := w.walk(u.Owner, u.Repo, u.Subpath, u.Ref, depth+1, u.Raw)
				if err == nil {
					out = append(out, nested...)
				}
			case u.Kind == UsesAction:
				add(pinStepAction, spec)
			}
		}
	}
	return out, nil
}

// dockerImageRef parses the value of runs.image of a Docker action which names an image of a registry:
// "docker://img:tag", or a name which starts with a registry host the runner accepts without the scheme. A
// path (a Dockerfile) is not one.
func dockerImageRef(image string) (*UsesRef, bool) {
	switch {
	case strings.HasPrefix(image, "docker://"):
		return ParseUses(image), true
	case isImageOnDockerRegistry(image):
		return ParseUses("docker://" + image), true
	}
	return nil, false
}

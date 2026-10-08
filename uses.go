package jactionlint

import (
	"regexp"
	"strings"
)

// selfRepositoryUsesPrefix marks the "self-repository" form of `uses:`. It resolves against the
// repository running the workflow at the exact commit running it, so it carries no `@ref` and is
// accepted everywhere the workspace-relative `./` form is: workflow steps, composite action steps,
// nested composition, and reusable workflow calls.
// https://github.blog/changelog/2026-07-30-reference-same-repository-actions-with-self-repository-syntax/
const selfRepositoryUsesPrefix = "$/"

// canonLocalUsesSpec converts a `uses:` value naming an action or a reusable workflow in the
// workflow's own repository into its canonical `./{path}` form, and reports whether the value was
// such a reference at all.
//
// Callers normalise rather than test for either prefix because the metadata caches are keyed by
// spec. LocalReusableWorkflowCache.WriteWorkflowCallEvent writes its keys in the `./` form, so a
// `$/` caller looking the unnormalised spec up finds nothing that was written ahead of it.
func canonLocalUsesSpec(spec string) (string, bool) {
	if strings.HasPrefix(spec, "./") {
		return spec, true
	}
	if p, ok := strings.CutPrefix(spec, selfRepositoryUsesPrefix); ok {
		return "./" + p, true
	}
	return "", false
}

// UsesKind classifies the value of a `uses:` key.
type UsesKind int

const (
	// UsesInvalid is a value that follows none of the accepted formats. UsesRef.Problem says why.
	UsesInvalid UsesKind = iota
	// UsesAction is a repository action: {owner}/{repo}[/{subpath}]@{ref}.
	UsesAction
	// UsesReusableWorkflow is a repository workflow: {owner}/{repo}/{path}.yml@{ref}.
	UsesReusableWorkflow
	// UsesDocker is a container image: docker://{image}[:{tag}][@{digest}].
	UsesDocker
	// UsesLocal is a path in the workflow's own repository: ./{path} or $/{path}.
	UsesLocal
)

// String returns the name of the kind as used in messages.
func (k UsesKind) String() string {
	switch k {
	case UsesAction:
		return "action"
	case UsesReusableWorkflow:
		return "reusable-workflow"
	case UsesDocker:
		return "docker"
	case UsesLocal:
		return "local"
	default:
		return "invalid"
	}
}

// RefKind classifies the `@{ref}` part of a repository `uses:` value.
type RefKind int

const (
	// RefNone means the value has no ref at all (local paths, a missing '@', or a Docker image
	// without a digest).
	RefNone RefKind = iota
	// RefFullSHA is a full-length (40 hex digits) commit SHA.
	RefFullSHA
	// RefShortSHA is an abbreviated commit SHA (7 to 39 hex digits). It is a heuristic: GitHub
	// cannot tell it from a branch or tag that happens to be spelled in hex.
	RefShortSHA
	// RefSemverTag looks like a version: v1, v1.2, 1.2.3, v1.2.3-rc.1.
	RefSemverTag
	// RefOther is any other tag or branch name, or a ref containing an expression.
	RefOther
	// RefDigest is a Docker content digest: sha256:{64 hex digits}.
	RefDigest
)

// String returns the name of the ref kind as used in messages.
func (k RefKind) String() string {
	switch k {
	case RefFullSHA:
		return "full-sha"
	case RefShortSHA:
		return "short-sha"
	case RefSemverTag:
		return "semver-tag"
	case RefOther:
		return "other-tag-or-branch"
	case RefDigest:
		return "digest"
	default:
		return "none"
	}
}

// Reasons UsesRef.Problem can carry. The first two are also what the "invalid-uses" rule reports.
const (
	usesProblemRefMissing   = "ref is missing"
	usesProblemOwnerMissing = "owner is missing"
	usesProblemEmptyPart    = "owner and repo and ref should not be empty"
)

// UsesRef is the parsed form of a `uses:` value, for both step-level actions and job-level reusable
// workflow calls. Obtain it with ParseUses. A UsesRef is never nil, and fields that do not apply
// to the Kind are empty.
type UsesRef struct {
	// Raw is the value exactly as given.
	Raw string
	// Kind is the kind of reference.
	Kind UsesKind
	// Problem says why Kind is UsesInvalid. Empty otherwise.
	Problem string
	// Dynamic is true when the value contains a `${{ }}` expression. The value is still split on a
	// best-effort basis but cannot be judged: IsPinned is false.
	Dynamic bool

	// Owner, Repo and Subpath are set for UsesAction and UsesReusableWorkflow (and best effort for
	// invalid values). Owner and Repo keep the case they were written in; compare them with
	// SameRepo. Subpath is everything after "{owner}/{repo}/" and keeps trailing slashes.
	Owner, Repo, Subpath string
	// Ref is what follows the first '@'. HasRef tells an absent ref from an empty one.
	Ref     string
	HasRef  bool
	RefKind RefKind

	// Path is the canonical "./{path}" form of a UsesLocal value.
	Path string

	// Image, Tag and Digest are set for UsesDocker. Image excludes the "docker://" scheme, the tag
	// and the digest, and keeps a registry host and port ("localhost:5000/img"). HasTag tells an
	// absent tag from an empty one ("docker://img:"). Digest is the part after '@', for instance
	// "sha256:...".
	Image, Tag, Digest string
	HasTag             bool
}

// ParseUses parses the value of a `uses:` key. It never fails: a value that is not understood
// comes back as UsesInvalid with Problem set, and as much of it as could be split is filled in.
//
// A value starting with "./" or "$/" is UsesLocal and one starting with "docker://" is UsesDocker.
// Everything else is read as {owner}/{repo}[/{subpath}]@{ref}; it is a UsesReusableWorkflow when
// the subpath names a .yml or .yaml file. Whether the value is legal in a given position (a step
// cannot call a workflow, a job cannot run an action) is for the caller to decide.
func ParseUses(spec string) *UsesRef {
	u := &UsesRef{Raw: spec, Dynamic: strings.Contains(spec, "${{")}

	if p, ok := canonLocalUsesSpec(spec); ok {
		u.Kind = UsesLocal
		u.Path = p
		return u
	}

	if rest, ok := strings.CutPrefix(spec, "docker://"); ok {
		u.parseDocker(rest)
		return u
	}

	s := spec
	if i := strings.IndexByte(s, '@'); i >= 0 {
		u.Ref, u.HasRef = s[i+1:], true
		s = s[:i]
	}
	u.RefKind = classifyRef(u.Ref, u.HasRef, u.Dynamic)

	i := strings.IndexByte(s, '/')
	if i >= 0 {
		u.Owner = s[:i]
		s = s[i+1:]
		u.Repo = s
		if j := strings.IndexByte(s, '/'); j >= 0 {
			u.Repo, u.Subpath = s[:j], s[j+1:]
		}
	}

	switch {
	case !u.HasRef:
		u.Problem = usesProblemRefMissing
	case i < 0:
		u.Problem = usesProblemOwnerMissing
	case u.Owner == "" || u.Repo == "" || u.Ref == "":
		u.Problem = usesProblemEmptyPart
	case isWorkflowFilePath(u.Subpath):
		u.Kind = UsesReusableWorkflow
	default:
		u.Kind = UsesAction
	}
	return u
}

func isWorkflowFilePath(p string) bool {
	p = strings.ToLower(p)
	return strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml")
}

func (u *UsesRef) parseDocker(rest string) {
	u.Kind = UsesDocker
	name := rest
	if i := strings.IndexByte(name, '@'); i >= 0 {
		u.Digest = name[i+1:]
		name = name[:i]
	}
	// A ':' is a tag separator only after the last '/', otherwise it is a registry port.
	if i := strings.LastIndexByte(name, ':'); i > strings.LastIndexByte(name, '/') {
		u.Tag, u.HasTag = name[i+1:], true
		name = name[:i]
	}
	u.Image = name
	if isSHA256Digest(u.Digest) {
		u.RefKind = RefDigest
	}
	if u.Image == "" {
		u.Kind = UsesInvalid
		u.Problem = "image is missing"
	}
}

func isSHA256Digest(d string) bool {
	h, ok := strings.CutPrefix(strings.ToLower(d), "sha256:")
	return ok && len(h) == 64 && isHex(h)
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}

var semverTagRegex = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){0,2}([-+][0-9A-Za-z.+-]+)?$`)

func classifyRef(ref string, has, dynamic bool) RefKind {
	switch {
	case !has:
		return RefNone
	case dynamic:
		return RefOther
	case len(ref) == 40 && isHex(ref):
		return RefFullSHA
	case len(ref) >= 7 && len(ref) < 40 && isHex(ref):
		return RefShortSHA
	case semverTagRegex.MatchString(ref):
		return RefSemverTag
	default:
		return RefOther
	}
}

// IsPinned reports whether the reference is immutable: a full commit SHA for repository actions
// and reusable workflows, a sha256 digest for Docker images. Local references are always pinned
// since they are resolved at the running commit. Invalid and dynamic values are not.
func (u *UsesRef) IsPinned() bool {
	if u.Dynamic {
		return false
	}
	switch u.Kind {
	case UsesLocal:
		return true
	case UsesAction, UsesReusableWorkflow:
		return u.RefKind == RefFullSHA
	case UsesDocker:
		return u.RefKind == RefDigest
	default:
		return false
	}
}

// IsRepo reports whether the reference points into a GitHub repository (an action or a reusable
// workflow, as opposed to a container image or a local path).
func (u *UsesRef) IsRepo() bool {
	return u.Kind == UsesAction || u.Kind == UsesReusableWorkflow
}

// SameRepo reports whether both references point into the same GitHub repository. Owner and repo
// names are case-insensitive on GitHub, so they are compared that way.
func (u *UsesRef) SameRepo(other *UsesRef) bool {
	return other != nil && u.IsRepo() && other.IsRepo() &&
		strings.EqualFold(u.Owner, other.Owner) && strings.EqualFold(u.Repo, other.Repo)
}

// CanonicalName returns the reference without its ref, normalised for comparison and lookup:
// "owner/repo[/subpath]" with owner and repo lowercased and trailing slashes dropped, the image
// name for Docker images, the "./{path}" form for local paths. Invalid values give "".
func (u *UsesRef) CanonicalName() string {
	switch u.Kind {
	case UsesAction, UsesReusableWorkflow:
		n := strings.ToLower(u.Owner) + "/" + strings.ToLower(u.Repo)
		if p := strings.TrimRight(u.Subpath, "/"); p != "" {
			n += "/" + p
		}
		return n
	case UsesDocker:
		return u.Image
	case UsesLocal:
		return u.Path
	default:
		return ""
	}
}

// isLocalWorkflowCall reports whether the value is a well-formed local reusable workflow call:
// a local path that carries no ref.
func (u *UsesRef) isLocalWorkflowCall() bool {
	if u.Kind != UsesLocal {
		return false
	}
	p := strings.TrimPrefix(u.Path, "./")
	return strings.IndexByte(p, '@') <= 0 && len(p) > 0
}

// isRepoWorkflowCall reports whether the value is {owner}/{repo}/{path}@{ref}. Unlike Kind it does
// not insist that the path is a YAML file; the workflow call rule has always accepted any path.
func (u *UsesRef) isRepoWorkflowCall() bool {
	return u.IsRepo() && u.Subpath != "" && !strings.HasPrefix(u.Owner, ".")
}

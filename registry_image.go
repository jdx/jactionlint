package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// This file has what the digest pinning of container images (`--online --fix` of unpinned-images and of the
// docker:// images of unpinned-uses) needs from the registries, apart from the HTTP client of the
// registries (registry_http.go).

// Registries which jactionlint asks for the digest of an image. Another registry is not asked: a workflow chooses
// the host, and a host that a pull request names must not make jactionlint send requests there.
const (
	registryDockerHub = "docker.io"
	registryGHCR      = "ghcr.io"
)

// ImageDigestClient is an optional interface of a GitHubClient. A client which implements it answers the digest of an
// image, which `--online --fix` writes after the tag of an image (`image:tag@sha256:...`). The client built
// into jactionlint asks the registry (Docker Hub and ghcr.io); a custom one can serve recorded answers.
type ImageDigestClient interface {
	// ImageDigest returns the digest of the manifest which the tag has in the registry: the digest of the image index
	// of a multi-platform image, the one that `docker pull image:tag` resolves and Renovate writes. registry is
	// "docker.io" or "ghcr.io", repository is the path in it ("library/ubuntu"). It returns ErrImageNotFound
	// (possibly wrapped) when there is no such image or it is not visible.
	ImageDigest(ctx context.Context, registry, repository, tag string) (string, error)
}

// ErrImageNotFound is returned (wrapped) by an ImageDigestClient when the registry has no such image, or the image is not
// visible without credentials.
var ErrImageNotFound = errors.New("image not found in the registry")

// errImagesOffline is why no digest is looked up in the cache mode (--online=cache): it never uses the network, and the digest
// of a tag is not kept in the disk cache because a tag moves, so there is nothing to find. It is no failure and no skipped lookup.
var errImagesOffline = errors.New("an image is not looked up in the cache mode, which never uses the network")

// errImagesUnsupported is why no digest is known: the client cannot ask a registry.
var errImagesUnsupported = errors.New("the online client cannot look up the digest of an image")

// imageRepoRe is the name of a repository of a registry: lower case path components. It is checked before a name is
// put in a request.
var imageRepoRe = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*)*$`)

// imageTagRe is a valid tag of an image.
var imageTagRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// registryImage is an image on a registry which can be asked.
type registryImage struct {
	registry   string // registryDockerHub or registryGHCR
	repository string // "library/ubuntu", "owner/image"
}

// slug names the image for the allow and deny lists of the online options: "owner/image", the first two parts of the path,
// whichever registry it is on. "library/ubuntu" is the official Docker Hub image.
func (i registryImage) slug() string {
	parts := strings.SplitN(i.repository, "/", 3)
	if len(parts) < 2 {
		return i.repository
	}
	return parts[0] + "/" + parts[1]
}

// parseRegistryImage finds the registry and the repository of the image of a `uses: docker://` or `image:` value without the tag and the
// digest. The reason is not empty when the image cannot be asked.
func parseRegistryImage(image string) (registryImage, string) {
	host, rest, hasHost := strings.Cut(image, "/")
	// The first part is a registry when it has a dot or a port, or is localhost. Otherwise the image is on Docker Hub.
	if !hasHost || !(strings.ContainsAny(host, ".:") || host == "localhost") {
		host, rest = registryDockerHub, image
	}
	var reg string
	switch strings.ToLower(host) {
	case "docker.io", "index.docker.io", "registry-1.docker.io", "registry.hub.docker.com":
		reg = registryDockerHub
		if !strings.Contains(rest, "/") {
			rest = "library/" + rest // An official image
		}
	case "ghcr.io":
		reg = registryGHCR
	default:
		return registryImage{}, fmt.Sprintf("the registry %s is not supported: only Docker Hub and ghcr.io are asked for a digest", host)
	}
	if !imageRepoRe.MatchString(rest) || len(rest) > 255 {
		return registryImage{}, fmt.Sprintf("%q is not a name of an image which can be asked for a digest", image)
	}
	return registryImage{registry: reg, repository: rest}, ""
}

type imageKey struct {
	registry, repository, tag string
}

// ImageDigest returns the digest of the image the tag points to in the registry. It is asked once for every image and tag, and the
// image is left out when its owner is not allowed by the online options (the same patterns as for repositories,
// matched against "owner/image").
func (s *onlineSession) ImageDigest(img registryImage, tag string) (string, error) {
	slug := img.slug()
	owner, repo, _ := strings.Cut(slug, "/")
	if !s.allows(owner, repo) {
		return "", errOnlineExcluded
	}
	if s.offline {
		return "", errImagesOffline
	}
	var ic ImageDigestClient
	switch {
	case s.images != nil:
		ic = s.images
	default:
		c, ok := s.client.(ImageDigestClient)
		if !ok {
			return "", errImagesUnsupported
		}
		ic = c
	}
	what := img.registry + "/" + img.repository + ":" + tag
	return s.imageDigests.get(imageKey{img.registry, img.repository, tag}, func() (string, error) {
		if err := s.stopped(); err != nil {
			return "", err
		}
		s.mu.Lock()
		down := s.registryDown[img.registry]
		s.mu.Unlock()
		if down != nil {
			s.mu.Lock()
			s.skipped++
			s.countKindLocked(classifyRegistryFailure(down))
			s.mu.Unlock()
			return "", down
		}
		d, err := ic.ImageDigest(s.ctx, img.registry, img.repository, tag)
		if err != nil {
			s.recordRegistry(img.registry, what, err)
			return "", err
		}
		s.mu.Lock()
		delete(s.registryFailures, img.registry)
		s.mu.Unlock()
		return d, nil
	})
}

// maxConsecutiveRegistryFailures is how many failures of one registry in a row make the run skip the rest of its images.
const maxConsecutiveRegistryFailures = 3

// recordRegistry counts the failure of a lookup in a registry. A registry which fails several times in a row, or says that
// its limit is reached, is not asked again in this run; the GitHub lookups are not affected.
func (s *onlineSession) recordRegistry(registry, what string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case errors.Is(err, errOnlineExcluded), errors.Is(err, errImagesUnsupported), errors.Is(err, errImagesOffline):
		return
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded) && s.ctx.Err() != nil:
		s.stopLocked(err)
		return
	case errors.Is(err, ErrImageNotFound):
		delete(s.registryFailures, registry) // The registry answered
		s.registryFailedLocked(failNotFound, "image-not-found", what, err,
			fmt.Sprintf("online: the image %s was not found in its registry or is private, so it is not pinned. --verbose lists every skipped lookup.", what))
		return
	}
	s.registryFailedLocked(classifyRegistryFailure(err), "registry:"+registry, what, err,
		fmt.Sprintf("online: could not look up %s: %s. this image is not pinned; the other images are still tried. --verbose lists every skipped lookup.", what, err))
	if s.registryFailures == nil {
		s.registryFailures = map[string]int{}
	}
	s.registryFailures[registry]++
	if errors.Is(err, errRegistryRateLimited) || s.registryFailures[registry] >= maxConsecutiveRegistryFailures {
		if s.registryDown == nil {
			s.registryDown = map[string]error{}
		}
		s.registryDown[registry] = fmt.Errorf("skipped after a failure of %s: %w", registry, err)
		s.warnRegistryOnceLocked("registry-down:"+registry,
			fmt.Sprintf("online: %s is not asked again in this run (%s), so its other images are not pinned; the lookups of GitHub are not affected.", registry, err))
	}
}

// errRegistryRateLimited is the error of a registry which refused a request because of its limit.
var errRegistryRateLimited = errors.New("rate limit of the registry reached")

// registryFailedLocked counts a skipped lookup of an image, logs it, and warns the first time of its kind (the message
// says what the user can do, which is not what failureMessage says about GitHub).
func (s *onlineSession) registryFailedLocked(kind failureKind, key, what string, err error, message string) {
	s.skipped++
	s.countKindLocked(kind)
	if s.detail != nil {
		s.detail("online: skipped %s: %v", what, err)
	}
	s.warnRegistryOnceLocked(key, message)
}

// warnRegistryOnceLocked warns the first time of its key.
func (s *onlineSession) warnRegistryOnceLocked(key, message string) {
	if k := failureKind(key); !s.seen[k] {
		s.seen[k] = true
		if s.warn != nil {
			s.warn(message)
		}
	}
}

// registryStatusError is a registry answering with a status that is not a success.
type registryStatusError struct {
	registry string
	status   int
	message  string
}

func (e *registryStatusError) Error() string {
	if e.message == "" {
		return fmt.Sprintf("the registry %s answered with status %d", e.registry, e.status)
	}
	return fmt.Sprintf("the registry %s answered with status %d: %s", e.registry, e.status, e.message)
}

// classifyRegistryFailure tells the kind of a failure of a registry: classifyFailure knows the errors of GitHub, and not
// the status of a registry.
func classifyRegistryFailure(err error) failureKind {
	var se *registryStatusError
	switch {
	case errors.Is(err, errRegistryRateLimited):
		return failRateLimit
	case errors.As(err, &se):
		switch {
		case se.status == http.StatusUnauthorized:
			return failUnauthorized
		case se.status == http.StatusForbidden:
			return failForbidden
		case se.status == http.StatusNotFound:
			return failNotFound
		case se.status >= 500:
			return failServer
		}
		return failOther
	}
	return classifyFailure(err)
}

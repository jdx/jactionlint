package jactionlint

import (
	"context"
	"errors"
	"fmt"
)

// fixtureNoImage is the value of an image in the fixtures which the registry does not have.
const fixtureNoImage = "not-found"

func fixtureImageKey(registry, repository, tag string) string {
	return registry + "/" + repository + ":" + tag
}

// ImageDigest implements ImageDigestClient. An image which was not recorded fails, so that a test notices when a rule
// starts to ask for something new.
func (c *FixtureGitHubClient) ImageDigest(_ context.Context, registry, repository, tag string) (string, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	key := fixtureImageKey(registry, repository, tag)
	d, ok := c.data.Images[key]
	if !ok {
		return "", fmt.Errorf("the GitHub fixtures have no image %s", key)
	}
	if d == fixtureNoImage {
		return "", fmt.Errorf("%s: %w", key, ErrImageNotFound)
	}
	return d, nil
}

// ImageDigest implements ImageDigestClient for a wrapped client which can ask a registry.
func (c *RecordingGitHubClient) ImageDigest(ctx context.Context, registry, repository, tag string) (string, error) {
	ic, ok := c.inner.(ImageDigestClient)
	if !ok {
		return "", errImagesUnsupported
	}
	d, err := ic.ImageDigest(ctx, registry, repository, tag)
	key := fixtureImageKey(registry, repository, tag)
	record := func(v string) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.data.Images == nil {
			c.data.Images = map[string]string{}
		}
		c.data.Images[key] = v
	}
	switch {
	case err == nil:
		record(d)
	case isImageNotFound(err):
		record(fixtureNoImage)
	}
	return d, err
}

func isImageNotFound(err error) bool { return errors.Is(err, ErrImageNotFound) }

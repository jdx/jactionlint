package jactionlint

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type failingImageClient struct {
	GitHubClient
}

func (failingImageClient) ImageDigest(context.Context, string, string, string) (string, error) {
	return "", errors.New("boom")
}

func TestImageFixReasonOfAFailedLookupIsAboutTheRegistry(t *testing.T) {
	src := imageTestHead + "    container: alpine:3.19\n    steps:\n      - run: echo\n"
	for name, client := range map[string]GitHubClient{"a failing registry": failingImageClient{onlineFixtureClient(t)}} {
		errs := lintImages(t, client, imagesConfig(t, ""), src)
		var found bool
		for _, e := range errs {
			if e.ID != "unpinned-images" {
				continue
			}
			found = true
			if e.NoFix == nil || e.NoFix.Code != NoFixLookupFailed || !strings.Contains(e.NoFix.Reason, "docker.io") || strings.Contains(e.NoFix.Reason, "GitHub") || strings.Contains(e.NoFix.Reason, "ref") {
				t.Errorf("%s: %+v", name, e.NoFix)
			}
		}
		if !found {
			t.Errorf("%s: no finding", name)
		}
	}

	// A denied image says so
	cfg := mustParseConfig(t, "online-options:\n  deny: [\"library/*\"]\nrules:\n  unpinned-images: error\n")
	errs, _ := lintOnline(t, onlineFixtureClient(t), cfg, src)
	for _, e := range errs {
		if e.ID == "unpinned-images" && (e.NoFix == nil || !strings.Contains(e.NoFix.Reason, "deny")) {
			t.Errorf("a denied image: %+v", e.NoFix)
		}
	}
}

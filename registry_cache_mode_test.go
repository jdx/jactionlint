package jactionlint

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// --online=cache never uses the network, and the digest of an image is not stored in the cache (a tag moves), so an image is not
// looked up at all: that is no failure, it does not count for --online=strict, and it does not stop the registry.
func TestSessionDoesNotAskRegistriesInCacheMode(t *testing.T) {
	c := &scriptedImages{}
	s, warnings := newImageSession(t, c)
	s.offline = true
	for i := 0; i < 2*maxConsecutiveRegistryFailures; i++ {
		_, err := s.ImageDigest(imageOf(t, fmt.Sprintf("img%d", i)), "1")
		if !errors.Is(err, errImagesOffline) {
			t.Fatalf("got %v", err)
		}
	}
	if len(c.calls) != 0 {
		t.Errorf("the registry was asked: %v", c.calls)
	}
	if s.skippedLookups() != 0 || len(*warnings) != 0 || len(s.registryDown) != 0 {
		t.Errorf("no failure: skipped %d, warnings %v, down %v", s.skippedLookups(), *warnings, s.registryDown)
	}
}

func TestImageFixInCacheModeSaysThatItNeedsTheNetwork(t *testing.T) {
	cfg := mustParseConfig(t, "online-options:\n  mode: cache,strict\nrules:\n  unpinned-images: error\n")
	src := imageTestHead + "    container: alpine:3.19\n    services:\n      a:\n        image: postgres:16\n      b:\n        image: ubuntu:22.04\n      c:\n        image: node:latest\n    steps:\n      - run: echo\n"
	errs, l := lintOnline(t, onlineFixtureClient(t), cfg, src)
	n := 0
	for _, e := range errs {
		if e.ID != "unpinned-images" {
			continue
		}
		n++
		if e.Fix != nil || e.NoFix == nil || e.NoFix.Code != NoFixOnlineRequired ||
			!strings.Contains(e.NoFix.Reason, "--online") || strings.Contains(e.NoFix.Reason, "cache of GitHub") || strings.Contains(e.NoFix.Reason, "not in the cache") {
			t.Errorf("the reason says that the lookup needs the network: %+v", e.NoFix)
		}
	}
	if n != 4 {
		t.Errorf("%d findings", n)
	}
	if l.OnlineSkipped() != 0 || l.OnlineFailed() {
		t.Errorf("a lookup which is not made is not a skipped one: %d skipped, failed=%v", l.OnlineSkipped(), l.OnlineFailed())
	}
}

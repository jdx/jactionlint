package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// This file reads the metadata of an action which is not in the data set built into jactionlint
// (PopularActions) from GitHub, at the ref the workflow pins, so that the checks of the inputs and of the
// outputs of an action run on any action the way they run on a local one.

// errOnlineNoFiles is why a file lookup was not made: the GitHubClient cannot read files (it does not
// implement GitHubContentClient). It is not a failure of the API.
var errOnlineNoFiles = errors.New("the GitHub client cannot read files")

// actionMetadataFiles are the names of the file of an action, in the order the runner looks for them.
var actionMetadataFiles = []string{"action.yml", "action.yaml"}

type fileKey struct {
	repoKey
	ref, path string
}

type repoFile struct {
	content []byte
	// found is false when GitHub has no such file at the ref (the repository, the ref or the file is missing, or
	// not visible to the token).
	found bool
}

type remoteActionKey struct {
	repoKey
	subpath, ref string
}

// normalizeFileRef makes the spellings of one commit one key: a full SHA is case-insensitive, any other ref is not.
func normalizeFileRef(ref string) string {
	if validGitSHA(ref) {
		return strings.ToLower(ref)
	}
	return ref
}

// file returns the file of the repository at the ref. A file which does not exist is not an error: found
// is false.
func (s *onlineSession) file(owner, repo, ref, path string) (repoFile, error) {
	if !s.allows(owner, repo) {
		return repoFile{}, errOnlineExcluded
	}
	cc, ok := s.client.(GitHubContentClient)
	if !ok {
		return repoFile{}, errOnlineNoFiles
	}
	ref = normalizeFileRef(ref)
	rk := newRepoKey(owner, repo)
	return s.files.get(fileKey{rk, ref, path}, func() (repoFile, error) {
		// The files of one repository are read one after the other, so that a failure is known to the lookups
		// which follow it (see below)
		lock := s.fileLock(rk)
		lock.Lock()
		defer lock.Unlock()

		// A repository whose files could not be read is not asked again: the answer will be the same, every
		// try can take several retries, and the failures of one repository must not add up to what the session takes
		// for an outage of the API (see maxConsecutiveGitHubFailures). "Not cached" is no such failure: the
		// cache mode may know the next ref.
		s.mu.Lock()
		prev := s.fileFailures[rk]
		if prev != nil {
			s.skipped++
			s.countKindLocked(classifyFailure(prev))
			if s.detail != nil {
				s.detail("online: skipped %s: %v", owner+"/"+repo, prev)
			}
		}
		s.mu.Unlock()
		if prev != nil {
			return repoFile{}, prev
		}
		f, err := callFor(s, owner+"/"+repo, func(ctx context.Context) (repoFile, error) {
			b, err := cc.FileContent(ctx, owner, repo, ref, path)
			if errors.Is(err, ErrGitHubNotFound) {
				return repoFile{}, nil
			}
			return repoFile{content: b, found: err == nil}, err
		})
		if err != nil {
			switch classifyFailure(err) {
			case failForbidden, failServer, failTimeout, failOther:
				if !errors.Is(err, context.Canceled) {
					s.mu.Lock()
					if s.fileFailures == nil {
						s.fileFailures = map[repoKey]error{}
					}
					s.fileFailures[rk] = err
					s.mu.Unlock()
				}
			}
		}
		return f, err
	})
}

// fileLock returns the lock which orders the file lookups of a repository.
func (s *onlineSession) fileLock(rk repoKey) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fileLocks == nil {
		s.fileLocks = map[repoKey]*sync.Mutex{}
	}
	l := s.fileLocks[rk]
	if l == nil {
		l = &sync.Mutex{}
		s.fileLocks[rk] = l
	}
	return l
}

// RemoteAction returns the metadata of the action owner/repo[/subpath] at the ref, read from its action.yml (or
// action.yaml) on GitHub. The metadata is nil, with a nil error, when the action has no metadata file there
// (a Docker action which is a bare Dockerfile, a path which is no action): there is nothing to check
// against. The error says why the metadata could not be read: the lookup failed, or the file is not valid
// metadata. The lookup is made once for the run, and the answer is shared by every rule which asks.
func (s *onlineSession) RemoteAction(owner, repo, subpath, ref string) (*ActionMetadata, error) {
	if !s.allows(owner, repo) {
		return nil, errOnlineExcluded
	}
	if _, ok := s.client.(GitHubContentClient); !ok {
		return nil, errOnlineNoFiles
	}
	subpath = strings.Trim(subpath, "/")
	ref = normalizeFileRef(ref)
	return s.actions.get(remoteActionKey{newRepoKey(owner, repo), subpath, ref}, func() (*ActionMetadata, error) {
		spec := owner + "/" + repo
		if subpath != "" {
			spec += "/" + subpath
		}
		spec += "@" + ref
		for _, name := range actionMetadataFiles {
			path := name
			if subpath != "" {
				path = subpath + "/" + name
			}
			f, err := s.file(owner, repo, ref, path)
			if err != nil {
				return nil, err
			}
			if !f.found {
				continue
			}
			meta, err := decodeActionMetadata(f.content, true)
			if err != nil {
				// The file is there but jactionlint cannot use it: the checks which need it did not run
				err = fmt.Errorf("the %s of %s is not valid action metadata: %w", name, spec, err)
				s.mu.Lock()
				s.failedLocked(failOther, "the "+name+" of "+spec, err)
				s.mu.Unlock()
				return nil, err
			}
			return meta, nil
		}
		missing := fmt.Errorf("%s has no action.yml or action.yaml: %w", spec, ErrGitHubNotFound)
		s.noteMissing("the action.yml of "+spec, missing)
		return nil, nil
	})
}

package jactionlint

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// onlineSettings is how a Linter reaches GitHub. The session is created when the first file which
// needs it is checked and is shared by all files, so that the answers about a repository are
// asked for once per run.
type onlineSettings struct {
	enabled bool
	client  GitHubClient
	ttl     time.Duration
	ctx     context.Context

	once sync.Once
	sess *onlineSession
	err  error
}

// onlineSession returns the session of the online rules, or nil when the online checks are off for
// the configuration. The error says the checks were asked for in a build that cannot make them.
func (l *Linter) onlineSession(cfg *Config) (*onlineSession, error) {
	o := &l.online
	if !o.enabled && (cfg == nil || !cfg.Online) {
		return nil, nil
	}
	o.once.Do(func() {
		client := o.client
		if client == nil {
			if !onlineSupported {
				o.err = fmt.Errorf("\"online: true\" in the configuration cannot be used: %w", errOnlineUnsupported)
				return
			}
			ttl := o.ttl
			switch {
			case ttl == 0:
				ttl = time.Hour
			case ttl < 0:
				ttl = 0
			}
			c, err := newDefaultGitHubClient(ttl, l.warnOnline, l.debug)
			if err != nil {
				o.err = err
				return
			}
			client = c
		}
		o.sess = newOnlineSession(o.ctx, client, l.warnOnline)
	})
	return o.sess, o.err
}

// warnOnline tells the user something about the online checks once per run. Like the warnings about
// the deprecated keys of the configuration it goes to the log output, or into the output document
// of a structured format.
func (l *Linter) warnOnline(msg string) {
	l.notesMu.Lock()
	for _, n := range l.notes {
		if n == msg {
			l.notesMu.Unlock()
			return
		}
	}
	l.notes = append(l.notes, msg)
	l.notesMu.Unlock()
	if structured(l.printer) {
		return
	}
	fmt.Fprintln(l.logOut, "warning:", msg)
}

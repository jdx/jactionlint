package jactionlint

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// onlineSettings is how a Linter reaches GitHub. The session is created when the first file which
// needs it is checked and is shared by all files, so that the answers about a repository are
// asked for once per run. The online options of that first file's configuration, over those of
// LinterOptions, apply to the whole run.
type onlineSettings struct {
	enabled bool
	// off is LinterOptions.OnlineOff: the configuration cannot turn the online checks on.
	off    bool
	client GitHubClient
	ttl    time.Duration
	ctx    context.Context
	// opts are LinterOptions.OnlineOptions: the command line, which wins over the configuration.
	opts OnlineOptions

	once sync.Once
	sess *onlineSession
	err  error
	mode OnlineMode
}

// enabledBy reports whether the configuration turns the online checks on.
func (o *onlineSettings) enabledBy(cfg *Config) bool {
	return !o.off && cfg != nil && (cfg.Online || cfg.OnlineOptions.Mode != OnlineModeDefault)
}

// onlineSession returns the session of the online rules, or nil when the online checks are off for
// the configuration. The error says the checks were asked for in a build that cannot make them.
func (l *Linter) onlineSession(cfg *Config) (*onlineSession, error) {
	o := &l.online
	if !o.enabled && !o.enabledBy(cfg) {
		return nil, nil
	}
	o.once.Do(func() {
		var cfgOpts OnlineOptions
		if cfg != nil {
			cfgOpts = cfg.OnlineOptions
		}
		eff := cfgOpts.overlay(o.opts)
		o.mode = eff.Mode
		client := o.client
		if client == nil {
			if !onlineSupported {
				o.err = fmt.Errorf("\"online: true\" in the configuration cannot be used: %w", errOnlineUnsupported)
				return
			}
			ttl := defaultOnlineCacheTTL
			switch {
			case o.opts.CacheTTL != nil:
				ttl = *o.opts.CacheTTL
			case o.ttl > 0:
				ttl = o.ttl
			case o.ttl < 0:
				ttl = 0
			case cfgOpts.CacheTTL != nil:
				ttl = *cfgOpts.CacheTTL
			}
			// A host named on the command line, in the environment or by a config of the user is the
			// user's choice. The config file of a repository is not: it gets no token.
			trusted := o.opts.APIURL != "" || eff.APIURL == "" || (cfg != nil && cfg.userOwned)
			c, err := newDefaultGitHubClient(defaultClientOptions{
				Options: eff, OptionsTrusted: trusted, TTL: ttl,
				Notify: l.warnOnline, Debug: l.debug, Verbose: l.logf, Context: o.ctx,
			})
			if err != nil {
				o.err = err
				return
			}
			client = c
		}
		o.sess = newOnlineSession(o.ctx, client, l.warnOnline)
		o.sess.allow, o.sess.deny = eff.Allow, eff.Deny
		o.sess.detail = l.logf
	})
	return o.sess, o.err
}

// logf writes a line to the log output with --verbose.
func (l *Linter) logf(format string, args ...any) {
	l.log(fmt.Sprintf(format, args...))
}

// OnlineSkipped returns how many GitHub lookups of the online checks were skipped because they failed
// (rate limit, 404, 403, server error, timeout, no network, nothing cached in the offline mode). The
// checks which needed them report nothing for those actions. Lookups left out by the allow and deny
// lists are not counted. It is zero when the online checks did not run.
func (l *Linter) OnlineSkipped() int {
	if l.online.sess == nil {
		return 0
	}
	return l.online.sess.skippedLookups()
}

// OnlineFailed reports whether the run was asked to fail (the "strict" online mode) and a lookup was
// skipped, so that a check which could not run does not pass silently.
func (l *Linter) OnlineFailed() bool {
	return l.online.mode.Strict() && l.OnlineSkipped() > 0
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

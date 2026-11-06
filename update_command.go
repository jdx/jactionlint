package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
)

// isUpdateCommand reports whether the command line is `jactionlint update ...`.
func isUpdateCommand(args []string) bool { return len(args) > 1 && args[1] == "update" }

func printUpdateHelp(w io.Writer) {
	fmt.Fprint(w, `Usage: jactionlint update [OPTIONS] [FILES...]

Checks the external actions and reusable workflows that the workflows and composite actions use, finds the latest
release the update policy allows, and reports, previews or writes the new references. It is separate from --fix: it
changes dependencies, not lint findings. Without FILES it reads the workflows and the action.yml and action.yaml files of
the repository. It reads releases from GitHub, so it needs the network (or --online=cache) and uses the GitHub options
and token of the other commands.

Modes (at most one; without one the updates are listed and nothing is changed):
      --check                 exit 1 when a reference has an update, violates the policy or cannot be verified
      --diff                  print the changes as a unified diff and write nothing
      --apply                 write the changes

Release selection (each overrides the "update" mapping of .github/jactionlint.yaml):
      --min-age=DAYS          use only releases at least this old (0 turns the cooldown off), update.cooldown-days
      --freeze                pin to the commit SHA of the release with a version comment, update.freeze
      --pin-only              pin the release that is referenced and do not upgrade it
      --branch-to-tag=REGEX   replace a branch whose name matches by the latest eligible release. Repeatable
      --include-tag=GLOB      use only tags that match, update.include-tags. Repeatable
      --exclude-tag=GLOB      never use tags that match, update.exclude-tags. Braces expand: '*-{alpha,beta,rc}*'. Repeatable

Which references:
      --include-action=REGEX  update only actions whose name (owner/repo[/path]) matches. Repeatable
      --exclude-action=REGEX  leave out actions whose name matches. Repeatable
      --action-rule=RULE      REGEX=ignore or REGEX=cooldown:DAYS, matched against 'owner/repo@ref # comment'. The first
                              rule that matches applies. Repeatable

Shared with the linter: --format (sarif and json included), --changed-since and --changed-scope, --config-file, --color,
--online=cache|strict, --no-online, --online-api-url, --online-token-env, --online-token-file, --online-github-com-fallback,
--online-allow, --online-deny, --online-cache-ttl, --online-max-wait.

Exit status: 0 no problem, 1 --check found a problem, 2 invalid command line, 3 failure.
`)
}

// updateMain runs `jactionlint update`. args are the arguments after "update".
func (cmd *Command) updateMain(args []string) int {
	f := newCommandFlags()
	fs := f.fs
	var (
		check, apply, freeze, pinOnly bool
		minAge                        string
		branchToTag, incTag, excTag   []string
		incAction, excAction, rules   []string
	)
	fs.BoolVar(&check, "check", false, "")
	fs.BoolVar(&apply, "apply", false, "")
	fs.StringVar(&minAge, "min-age", "", "")
	fs.BoolVar(&freeze, "freeze", false, "")
	fs.BoolVar(&pinOnly, "pin-only", false, "")
	fs.StringArrayVar(&branchToTag, "branch-to-tag", nil, "")
	fs.StringArrayVar(&incTag, "include-tag", nil, "")
	fs.StringArrayVar(&excTag, "exclude-tag", nil, "")
	fs.StringArrayVar(&incAction, "include-action", nil, "")
	fs.StringArrayVar(&excAction, "exclude-action", nil, "")
	fs.StringArrayVar(&rules, "action-rule", nil, "")
	if err := f.parse(args); err != nil {
		fmt.Fprintf(cmd.Stderr, "jactionlint update: %s (try update --help)\n", err)
		return ExitStatusInvalidCommandOption
	}
	if f.help {
		printUpdateHelp(cmd.Stdout)
		return ExitStatusSuccessNoProblem
	}
	fail := func(format string, a ...any) int {
		fmt.Fprintf(cmd.Stderr, "jactionlint update: "+format+"\n", a...)
		return ExitStatusInvalidCommandOption
	}
	if f.fix.mode != 0 || f.baselineWrite.set || f.initConfig || f.migrateConfig || f.migrateIgnores || f.exportFacts || f.suggestTimeouts {
		return fail("--fix, --baseline-write, --init-config, --migrate-config, --migrate-ignores, --export-facts and --suggest-timeouts belong to the linter, not to update")
	}
	if countTrue(check, f.diff, apply) > 1 {
		return fail("--check, --diff and --apply cannot be combined")
	}
	if f.noOnline {
		return fail("update reads the releases from GitHub, so it cannot be combined with --no-online")
	}
	if f.onlineReport != "" {
		return fail("--online-report is not supported by update")
	}
	if pinOnly && (fs.Changed("min-age") || len(branchToTag) > 0) {
		return fail("--pin-only keeps the version, so --min-age and --branch-to-tag have nothing to do")
	}

	uo := UpdateOptions{
		PinOnly: pinOnly, BranchToTag: branchToTag, IncludeTags: incTag, ExcludeTags: excTag,
		IncludeActions: incAction, ExcludeActions: excAction, ActionRules: rules,
	}
	switch {
	case check:
		uo.Mode = UpdateCheck
	case f.diff:
		uo.Mode = UpdateDiff
	case apply:
		uo.Mode = UpdateApply
	}
	if fs.Changed("min-age") {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(minAge), "d"))
		if err != nil || n < 0 {
			return fail("invalid value %q for --min-age: write a number of days, 0 or more", minAge)
		}
		uo.MinAge = &n
	}
	if fs.Changed("freeze") {
		uo.Freeze = &freeze
	}

	opts := f.opts
	sev, err := ParseSeverity(f.minSeverity)
	if err != nil || sev == SeverityOff {
		return fail("invalid value %q for --min-severity. available values are \"info\", \"warn\" and \"error\"", f.minSeverity)
	}
	opts.MinSeverity = sev
	if f.profileName != "" {
		p, err := ParseProfile(f.profileName)
		if err != nil {
			return fail("invalid value %q for --profile", f.profileName)
		}
		opts.Profile = p
	}
	if fs.Changed("changed-scope") && f.changedSince == "" {
		return fail("--changed-scope can be used only with --changed-since")
	}
	if fs.Changed("changed-since") {
		if f.changedSince == "" {
			return fail("--changed-since needs a revision, such as --changed-since origin/main")
		}
		scope, err := ParseChangedScope(f.changedScope)
		if err != nil {
			return fail("%s", err)
		}
		opts.ChangedSince, opts.ChangedScope = f.changedSince, scope
	}
	opts.IgnorePatterns = f.ignorePats
	opts.LogWriter = cmd.Stderr
	opts.RunHints = false

	online, onlineAllow, onlineDeny := f.online, f.onlineAllow, f.onlineDeny
	if !online.set {
		set, off, mode, err := parseOnlineEnv(os.Getenv(onlineEnvName))
		if err != nil {
			return fail("%s", err)
		}
		if off {
			return fail("update reads the releases from GitHub, so it cannot be combined with %s=0", onlineEnvName)
		}
		if set {
			online = onlineFlag{set: true, mode: mode}
		}
	}
	// update always uses GitHub: the mode of --online only says how
	opts.Online = true
	if online.set {
		opts.OnlineOptions.Mode = online.mode
		opts.OnlineOptions.ModeSet = true
	}
	if fs.Changed("online-cache-ttl") {
		opts.OnlineOptions.CacheTTL = &f.onlineTTL
	}
	if fs.Changed("online-github-com-fallback") {
		v := f.onlineGitHubComFallback
		opts.OnlineOptions.GitHubComFallback = &v
	}
	if fs.Changed("online-max-wait") {
		opts.OnlineOptions.MaxRateLimitWait = &f.onlineMaxWait
	}
	if len(onlineAllow) > 0 {
		opts.OnlineOptions.Allow = onlineAllow
	}
	if len(onlineDeny) > 0 {
		opts.OnlineOptions.Deny = onlineDeny
	}
	if err := opts.OnlineOptions.validate(); err != nil {
		return fail("invalid online option: %s", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	opts.Context = ctx
	if f.color.set {
		opts.Color = f.color.kind
	}
	if f.noColor {
		opts.Color = ColorOptionKindNever
	}

	l, err := NewLinter(cmd.Stdout, &opts)
	if err != nil {
		fmt.Fprintln(cmd.Stderr, err.Error())
		var ue *usageError
		if errors.As(err, &ue) {
			return ExitStatusInvalidCommandOption
		}
		return ExitStatusFailure
	}
	res, err := l.Update(fs.Args(), uo)
	if err != nil {
		fmt.Fprintln(cmd.Stderr, err.Error())
		var ue *usageError
		if errors.As(err, &ue) {
			return ExitStatusInvalidCommandOption
		}
		return ExitStatusFailure
	}
	if l.OnlineFailed() {
		fmt.Fprintf(cmd.Stderr, "online=strict: %d GitHub lookups were skipped, so the update is incomplete\n", l.OnlineSkipped())
		return ExitStatusFailure
	}
	if uo.Mode == UpdateCheck && (res.Updates > 0 || res.Violations > 0) {
		return ExitStatusSuccessProblemFound
	}
	return ExitStatusSuccessNoProblem
}

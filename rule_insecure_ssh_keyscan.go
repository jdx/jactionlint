package jactionlint

import (
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleInsecureSSHKeyscan is a rule checker which reports `ssh-keyscan` output that a `run:` script
// writes to a known_hosts file. The scan trusts whatever key the server presents when the job runs
// (trust on first use), so anyone who can intercept the connection of the runner becomes the host in
// every later ssh, scp or rsync of the job. The host key has to come from somewhere that does not depend
// on the connection being scanned: a repository variable or secret with the verified key, or for GitHub
// the keys published in its documentation.
//
// A script that prints the fingerprints of the scanned keys with `ssh-keygen -l` is assumed to compare
// them, and is not reported.
type RuleInsecureSSHKeyscan struct {
	RuleBase
	shell shellScope
}

// NewRuleInsecureSSHKeyscan creates a new RuleInsecureSSHKeyscan instance.
func NewRuleInsecureSSHKeyscan() *RuleInsecureSSHKeyscan {
	return &RuleInsecureSSHKeyscan{
		RuleBase: RuleBase{
			name: "insecure-ssh-keyscan",
			desc: "Checks that SSH host keys are not collected with ssh-keyscan without being verified",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleInsecureSSHKeyscan) VisitWorkflowPre(n *Workflow) error {
	rule.shell.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleInsecureSSHKeyscan) VisitJobPre(n *Job) error {
	rule.shell.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleInsecureSSHKeyscan) VisitJobPost(n *Job) error {
	rule.shell.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleInsecureSSHKeyscan) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok {
		return nil
	}
	sc, origin := rule.shell.analyze(run)
	if sc == nil {
		return nil
	}
	for _, c := range sc.Commands {
		if c.Name == "ssh-keygen" && c.HasFlag("-l") {
			return nil // the fingerprints are shown to be compared
		}
	}
	for _, k := range sc.Commands {
		if k.Name != "ssh-keyscan" {
			continue
		}
		target := knownHostsWriteOf(sc, k)
		if target == "" {
			continue
		}
		rule.ReportIDf("insecure-ssh-keyscan", scriptPos(sc, origin, k.Offset), "\"ssh-keyscan\" writes the host key it receives to %q, so the key of whoever answers the connection is trusted (trust on first use), and an attacker on the path to the server can take it over for every later ssh, scp or rsync. store the verified host key in a secret or variable and write that to the known_hosts file instead", target)
	}
	return nil
}

// knownHostsWriteOf returns the known_hosts file the output of the ssh-keyscan command is written to, or
// "" if it is not written to one.
func knownHostsWriteOf(sc *runscript.Script, k *runscript.Command) string {
	for _, r := range sc.Redirects {
		if !r.Write || r.Target == nil || !strings.Contains(strings.ToLower(r.Target.Value), "known_hosts") {
			continue
		}
		switch {
		case r.Cmd == k:
		case r.Cmd != nil && k.Pipeline != nil && r.Cmd.Pipeline == k.Pipeline:
		case r.Cmd != nil && wordHasCommand(r.Cmd, k):
		case r.Group && containsCommand(r.Inner, k):
		default:
			continue
		}
		if r.Fd == "2" {
			continue
		}
		return r.Target.Value
	}
	return ""
}

func wordHasCommand(c, k *runscript.Command) bool {
	for _, w := range c.Words {
		if containsCommand(w.Subs, k) {
			return true
		}
	}
	return false
}

func containsCommand(cs []*runscript.Command, k *runscript.Command) bool {
	for _, c := range cs {
		if c == k {
			return true
		}
	}
	return false
}

func init() {
	registerRules(RuleInfo{
		ID: "insecure-ssh-keyscan", Group: RuleGroupSecurity, Summary: "ssh-keyscan output is trusted as the host key without verification.",
		DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-insecure-ssh-keyscan",
	})
	registerRuleFactory("insecure-ssh-keyscan", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("insecure-ssh-keyscan") {
			return nil
		}
		return []Rule{NewRuleInsecureSSHKeyscan()}
	})
}

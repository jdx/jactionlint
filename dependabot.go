package jactionlint

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// DependabotPass is an interface to traverse the syntax tree of a Dependabot configuration. It is
// the counterpart of Pass for .github/dependabot.yml.
type DependabotPass interface {
	// VisitDependabotPre is callback when visiting the root node before visiting its children. It
	// returns internal error when it cannot continue the process.
	VisitDependabotPre(node *Dependabot) error
	// VisitDependabotUpdate is callback when visiting an item of "updates". It returns internal error
	// when it cannot continue the process.
	VisitDependabotUpdate(node *DependabotUpdate) error
	// VisitDependabotPost is callback when visiting the root node after visiting its children. It
	// returns internal error when it cannot continue the process.
	VisitDependabotPost(node *Dependabot) error
}

// DependabotRule is an interface which all rules for Dependabot configurations must meet. It is
// the counterpart of Rule. Embed DependabotRuleBase to implement the methods other than the
// visiting callbacks you need.
type DependabotRule interface {
	DependabotPass
	Errs() []*Error
	Name() string
	Description() string
	EnableDebug(out io.Writer)
	SetConfig(cfg *Config)
	Config() *Config
}

// DependabotRuleBase is a struct to be a base of rule structs for Dependabot configurations. It
// has the same reporting methods as RuleBase (Error, ReportID, ...) and visiting callbacks which
// do nothing.
type DependabotRuleBase struct {
	RuleBase
}

// NewDependabotRuleBase creates a new DependabotRuleBase instance. It should be embedded to your
// own rule instance.
func NewDependabotRuleBase(name string, desc string) DependabotRuleBase {
	return DependabotRuleBase{NewRuleBase(name, desc)}
}

// VisitDependabotPre is callback when visiting the root node before visiting its children.
func (r *DependabotRuleBase) VisitDependabotPre(node *Dependabot) error { return nil }

// VisitDependabotUpdate is callback when visiting an item of "updates".
func (r *DependabotRuleBase) VisitDependabotUpdate(node *DependabotUpdate) error { return nil }

// VisitDependabotPost is callback when visiting the root node after visiting its children.
func (r *DependabotRuleBase) VisitDependabotPost(node *Dependabot) error { return nil }

// DependabotVisitor visits the syntax tree of a Dependabot configuration from the root in
// depth-first order.
type DependabotVisitor struct {
	passes []DependabotPass
	dbg    io.Writer
}

// NewDependabotVisitor creates DependabotVisitor instance.
func NewDependabotVisitor() *DependabotVisitor {
	return &DependabotVisitor{}
}

// AddPass adds new pass which is called on traversing a syntax tree.
func (v *DependabotVisitor) AddPass(p DependabotPass) {
	v.passes = append(v.passes, p)
}

// EnableDebug enables debug output when non-nil io.Writer value is given. All debug outputs from
// visitor will be written to the writer.
func (v *DependabotVisitor) EnableDebug(w io.Writer) {
	v.dbg = w
}

func (v *DependabotVisitor) reportElapsedTime(what string, start time.Time) {
	fmt.Fprintf(v.dbg, "[DependabotVisitor] %s took %vms\n", what, time.Since(start).Milliseconds())
}

// Visit visits given syntax tree in depth-first order.
func (v *DependabotVisitor) Visit(n *Dependabot) error {
	var t time.Time
	if v.dbg != nil {
		t = time.Now()
	}

	for _, p := range v.passes {
		if err := p.VisitDependabotPre(n); err != nil {
			return err
		}
	}
	for _, u := range n.Updates {
		for _, p := range v.passes {
			if err := p.VisitDependabotUpdate(u); err != nil {
				return err
			}
		}
	}
	for _, p := range v.passes {
		if err := p.VisitDependabotPost(n); err != nil {
			return err
		}
	}

	if v.dbg != nil {
		v.reportElapsedTime(fmt.Sprintf("Visiting %d updates", len(n.Updates)), t)
	}
	return nil
}

// dependabotRuleContext is what the constructors of the built-in Dependabot rules need.
type dependabotRuleContext struct {
	path    string
	project *Project
	config  *Config
}

// dependabotRuleFactory creates one rule implementation for Dependabot configurations. A factory
// returns nil when the rule is not needed for the file.
type dependabotRuleFactory struct {
	// kind is the name of the rule. Errors of the rule have it as Error.Kind.
	kind string
	new  func(ctx *dependabotRuleContext) (DependabotRule, error)
}

// dependabotRuleFactories is the list of all built-in rules for Dependabot configurations in the
// order they are applied. The IDs which each rule can report are in ruleRegistry. The syntax errors
// ("dependabot-syntax") are reported by the parser, not by a rule.
var dependabotRuleFactories = []dependabotRuleFactory{}

func newDependabotRules(ctx *dependabotRuleContext, log func(args ...interface{})) []DependabotRule {
	rules := make([]DependabotRule, 0, len(dependabotRuleFactories))
	for _, f := range dependabotRuleFactories {
		r, err := f.new(ctx)
		if err != nil {
			log("Rule \"" + f.kind + "\" was disabled: " + err.Error())
			continue
		}
		if r != nil {
			rules = append(rules, r)
		}
	}
	return rules
}

// dependabotFileNames are the names GitHub reads the Dependabot configuration from.
var dependabotFileNames = []string{"dependabot.yml", "dependabot.yaml"}

// isDependabotPath reports whether the path is a Dependabot configuration file, which is checked
// with different rules from workflows. The decision is made only from the path: the file is named
// dependabot.yml or dependabot.yaml, and it is in a ".github" directory or has no directory (e.g. the
// name given to -stdin-filename). ".github/workflows/dependabot.yml" is a workflow.
func isDependabotPath(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	base := path.Base(p)
	if base != "dependabot.yml" && base != "dependabot.yaml" {
		return false
	}
	dir := path.Dir(p)
	return dir == "." || dir == "/" || path.Base(dir) == ".github"
}

// isDependabotFile is isDependabotPath for a path given to the linter. A relative path is resolved
// against the working directory first, so that "dependabot.yml" linted from inside ".github/workflows"
// is still a workflow. The name for STDIN is used as it is: it has no directory on purpose.
func (l *Linter) isDependabotFile(p string) bool {
	if p != l.stdin && !filepath.IsAbs(p) {
		p = filepath.Join(l.cwd, p)
	}
	return isDependabotPath(p)
}

// DependabotFiles returns the paths of the Dependabot configuration files of the project:
// ".github/dependabot.yml" and ".github/dependabot.yaml" when they exist.
func (p *Project) DependabotFiles() []string {
	files := []string{}
	for _, n := range dependabotFileNames {
		f := filepath.Join(p.root, ".github", n)
		if s, err := os.Stat(f); err == nil && !s.IsDir() {
			files = append(files, f)
		}
	}
	return files
}

// checkDependabot checks the content of a Dependabot configuration file.
func (l *Linter) checkDependabot(path string, content []byte, project *Project, cfg *Config, start time.Time) ([]*Error, error) {
	d, all := ParseDependabot(content)

	if l.logLevel >= LogLevelVerbose {
		l.log("Found", len(all), "parse errors in", time.Since(start).Milliseconds(), "ms for", path)
	}

	if d != nil {
		dbg := l.debugWriter()

		rules := newDependabotRules(&dependabotRuleContext{path: path, project: project, config: cfg}, l.log)
		if l.onDependabot != nil {
			rules = l.onDependabot(rules)
		}

		v := NewDependabotVisitor()
		for _, rule := range rules {
			v.AddPass(rule)
		}
		if dbg != nil {
			v.EnableDebug(dbg)
			for _, r := range rules {
				r.EnableDebug(dbg)
			}
		}
		if cfg != nil {
			for _, r := range rules {
				r.SetConfig(cfg)
			}
		}

		if err := v.Visit(d); err != nil {
			l.debug("Error occurred while visiting Dependabot syntax tree: %v", err)
			return nil, err
		}

		for _, rule := range rules {
			errs := rule.Errs()
			l.debug("%s found %d errors", rule.Name(), len(errs))
			all = append(all, errs...)
		}
	}

	return l.finishCheck(path, content, all, cfg, start), nil
}

func init() {
	registerRules(
		RuleInfo{ID: dependabotSyntaxID, Group: RuleGroupCorrectness, Summary: "The Dependabot configuration does not follow the syntax of dependabot.yml.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-dependabot-syntax"},
	)
}

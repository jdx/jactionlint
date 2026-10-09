package jactionlint

import (
	"go.yaml.in/yaml/v4"
)

// A zizmor finding has several locations (the step, the job or the trigger it is about, and the key it
// points at), and a `# zizmor: ignore[...]` comment on any line of any of them ignores the finding. So
// a comment anywhere in a step ignores the template-injection finding of the step, and a comment
// anywhere in the `on:` value ignores a dangerous-triggers finding of any trigger. The ranges below
// are the regions which the comments of a source can cover, found with the YAML structure.

// zizmorStepRules are the audits whose findings span the whole step: a comment on any line of the step
// applies (zizmor adds the step as a location of the finding).
var zizmorStepRules = map[string]bool{
	"template-injection":   true,
	"artipacked":           true,
	"unpinned-uses":        true,
	"ref-version-mismatch": true,
	"superfluous-actions":  true,
	"adhoc-packages":       true,
}

// zizmorJobRules are the audits whose findings span the whole job, with the steps in it.
var zizmorJobRules = map[string]bool{"secrets-outside-env": true}

// zizmorCallJobRules are the audits whose finding about a job which calls a reusable workflow span the
// whole job.
var zizmorCallJobRules = map[string]bool{"unpinned-uses": true, "ref-version-mismatch": true}

// zizmorRegion is a range of lines, 1-based and inclusive.
type zizmorRegion struct{ start, end int }

func (r zizmorRegion) has(line int) bool { return r.start <= line && line <= r.end }

// zizmorScope is a range of lines in which the findings of the rules are ignored by a comment.
type zizmorScope struct {
	rules      map[string]bool
	start, end int
}

type zizmorJob struct {
	zizmorRegion
	steps []zizmorRegion
	// uses and secrets are the `uses:` and `secrets:` pairs of a job which calls a reusable workflow.
	uses, secrets zizmorRegion
}

// zizmorStructure is where the steps, jobs and triggers of a source are.
type zizmorStructure struct {
	on    zizmorRegion
	jobs  []zizmorJob
	steps []zizmorRegion // the steps of a composite action
}

// regionEnd returns the last line of the region which starts at the line start. A region ends where the
// next sibling starts (nextStart, 0 for none), which makes the comments between two siblings belong to the
// first one like in zizmor. The last sibling ends with the comments which follow it.
func regionEnd(lines []string, start, nextStart int) int {
	if nextStart > 0 {
		e := nextStart - 1
		for e > start {
			if _, blank := isCommentOrBlank(lines[e-1]); !blank {
				break
			}
			e--
		}
		return e
	}
	e := ignoreTargetEnd(lines, start-1)
	for e < len(lines) {
		if comment, _ := isCommentOrBlank(lines[e]); !comment {
			break
		}
		e++
	}
	return e
}

// childPairs returns the key and value nodes of a mapping.
func childPairs(n *yaml.Node) (keys, vals []*yaml.Node) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		keys = append(keys, n.Content[i])
		vals = append(vals, n.Content[i+1])
	}
	return keys, vals
}

// pairRegions returns the region of each pair of a mapping.
func pairRegions(lines []string, keys []*yaml.Node, parentEnd int) []zizmorRegion {
	ret := make([]zizmorRegion, len(keys))
	for i, k := range keys {
		next := 0
		if i+1 < len(keys) {
			next = keys[i+1].Line
		}
		ret[i] = zizmorRegion{k.Line, regionEnd(lines, k.Line, next)}
		if next == 0 && parentEnd > 0 {
			ret[i].end = max(ret[i].end, parentEnd)
		}
	}
	return ret
}

// itemRegions returns the region of each item of a block sequence.
func itemRegions(lines []string, seq *yaml.Node) []zizmorRegion {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	ret := make([]zizmorRegion, len(seq.Content))
	for i, it := range seq.Content {
		next := 0
		if i+1 < len(seq.Content) {
			next = seq.Content[i+1].Line
		}
		if it.Line < 1 || it.Line > len(lines) {
			continue
		}
		ret[i] = zizmorRegion{it.Line, regionEnd(lines, it.Line, next)}
	}
	return ret
}

// scanZizmorStructure finds the regions of the steps, jobs and triggers of the source.
func scanZizmorStructure(src []byte, lines []string) *zizmorStructure {
	st := &zizmorStructure{}
	var doc yaml.Node
	if yaml.Unmarshal(src, &doc) != nil || len(doc.Content) == 0 {
		return st
	}
	keys, vals := childPairs(doc.Content[0])
	regions := pairRegions(lines, keys, 0)
	for i, k := range keys {
		switch k.Value {
		case "on", "true":
			st.on = regions[i]
		case "runs":
			ks, vs := childPairs(vals[i])
			for j, kk := range ks {
				if kk.Value == "steps" {
					st.steps = itemRegions(lines, vs[j])
				}
			}
		case "jobs":
			jks, jvs := childPairs(vals[i])
			jrs := pairRegions(lines, jks, regions[i].end)
			for j := range jks {
				job := zizmorJob{zizmorRegion: jrs[j]}
				ks, vs := childPairs(jvs[j])
				prs := pairRegions(lines, ks, jrs[j].end)
				for p, kk := range ks {
					switch kk.Value {
					case "steps":
						job.steps = itemRegions(lines, vs[p])
					case "uses":
						job.uses = prs[p]
					case "secrets":
						job.secrets = prs[p]
					}
				}
				st.jobs = append(st.jobs, job)
			}
		}
	}
	return st
}

// zizmorScopesOf returns the regions which a zizmor comment on the line covers besides its own line.
func zizmorScopesOf(st *zizmorStructure, line int, entries []*inlineIgnoreEntry) []zizmorScope {
	if st == nil {
		return nil
	}
	var ret []zizmorScope
	add := func(set map[string]bool, r zizmorRegion) {
		rules := map[string]bool{}
		for _, e := range entries {
			for _, t := range e.targets {
				if set[t.ID] {
					rules[t.ID] = true
				}
			}
		}
		if len(rules) > 0 {
			ret = append(ret, zizmorScope{rules, r.start, r.end})
		}
	}
	for _, s := range st.steps {
		if s.has(line) {
			add(zizmorStepRules, s)
		}
	}
	for _, j := range st.jobs {
		if !j.has(line) {
			continue
		}
		for _, s := range j.steps {
			if s.has(line) {
				add(zizmorStepRules, s)
			}
		}
		add(zizmorJobRules, j.zizmorRegion)
		if len(j.steps) == 0 {
			add(zizmorCallJobRules, j.zizmorRegion)
		}
		if j.uses.has(line) || j.secrets.has(line) {
			add(map[string]bool{"secrets-inherit": true}, j.zizmorRegion)
		}
	}
	if st.on.has(line) {
		add(map[string]bool{"dangerous-triggers": true}, st.on)
	}
	return ret
}

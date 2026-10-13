package jactionlint

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// parseSelect validates the rule IDs of --select and returns them as a set. An unknown ID is an error which
// suggests the closest one, like --fix-rules does.
func parseSelect(ids []string) (map[string]bool, error) {
	sel := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := ruleIndex[id]; !ok {
			return nil, fmt.Errorf("unknown rule ID %q in --select%s", id, suggestRuleID(id))
		}
		sel[id] = true
	}
	if len(sel) == 0 {
		return nil, fmt.Errorf("--select lists no rule ID")
	}
	return sel, nil
}

// selectRules returns the configuration which runs only the selected rules: every other rule is off, and a
// selected rule which the configuration turns off is on at its default level. The options of the rules and
// everything else of the configuration are kept. A selected rule which the configuration sets to a level
// keeps it. The given configuration is not modified.
func selectRules(cfg *Config, sel map[string]bool) *Config {
	var c Config
	if cfg != nil {
		c = *cfg
	}
	c.Rules = make(map[string]RuleConfig, len(ruleRegistry))
	for _, info := range ruleRegistry {
		rc := RuleConfig{}
		if cfg != nil {
			rc = cfg.Rules[info.ID]
		}
		switch {
		case !sel[info.ID]:
			rc = RuleConfig{Level: SeverityOff, levelSet: true}
		case cfg.RuleLevel(info.ID) == SeverityOff:
			rc.Level, rc.levelSet = info.DefaultLevel, true
		default:
			rc.Level, rc.levelSet = cfg.RuleLevel(info.ID), true
		}
		c.Rules[info.ID] = rc
	}
	return &c
}

// ruleListEntry is one rule of --list-rules --format json.
type ruleListEntry struct {
	ID           string `json:"id"`
	Group        string `json:"group"`
	Profile      string `json:"profile"`
	DefaultLevel string `json:"default_level"`
	Online       bool   `json:"online"`
	Fixable      bool   `json:"fixable"`
	Summary      string `json:"summary"`
	DocURL       string `json:"doc_url"`
}

func ruleList() []ruleListEntry {
	rules := Rules()
	ret := make([]ruleListEntry, 0, len(rules))
	for _, r := range rules {
		p := string(r.Profile)
		switch {
		case r.Online:
			p = "online"
		case p == "":
			p = "opt-in"
		}
		ret = append(ret, ruleListEntry{r.ID, string(r.Group), p, r.DefaultLevel.String(), r.Online, r.Fixable, r.Summary, r.DocURL()})
	}
	return ret
}

// writeRuleList prints every rule ID, as an aligned table (the text format) or as a JSON array (--format json).
// PROFILE is the first profile which runs the rule, "online" for a rule which needs --online and "opt-in" for a
// rule which runs only when the configuration turns it on.
func writeRuleList(w io.Writer, format string) error {
	list := ruleList()
	switch format {
	case "", FormatText, FormatOneline:
	case FormatJSON:
		return encodeJSON(w, list)
	case FormatJSONL:
		for _, e := range list {
			if err := encodeJSON(w, e); err != nil {
				return err
			}
		}
		return nil
	default:
		return &usageError{fmt.Errorf("--list-rules supports --format text, json and jsonl, not %q", format)}
	}
	idW, grpW, proW := len("RULE"), len("GROUP"), len("PROFILE")
	for _, e := range list {
		idW = max(idW, utf8.RuneCountInString(e.ID))
		grpW = max(grpW, len(e.Group))
		proW = max(proW, len(e.Profile))
	}
	fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-5s  %s\n", idW, "RULE", grpW, "GROUP", proW, "PROFILE", "LEVEL", "SUMMARY")
	for _, e := range list {
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-5s  %s\n", idW, e.ID, grpW, e.Group, proW, e.Profile, e.DefaultLevel, e.Summary)
	}
	return nil
}

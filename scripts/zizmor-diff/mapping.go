package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

//go:embed mapping.json
var defaultMapping []byte

// Match selects jactionlint findings. Rule is the rule ID (the kind in v1). MessageContains narrows a
// coarse v1 kind down to one check and can be dropped once rule IDs are stable.
type Match struct {
	Rule            string `json:"rule"`
	MessageContains string `json:"message_contains,omitempty"`
}

// AuditMapping says which jactionlint findings correspond to a zizmor audit.
type AuditMapping struct {
	Coverage    string  `json:"coverage"` // full or partial
	Notes       string  `json:"notes,omitempty"`
	Jactionlint []Match `json:"jactionlint"`
}

// Mapping is the content of mapping.json.
type Mapping struct {
	Zizmor string                  `json:"zizmor"`
	Audits map[string]AuditMapping `json:"audits"`
}

func parseMapping(b []byte) (*Mapping, error) {
	var m Mapping
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse mapping: %w", err)
	}
	for name, a := range m.Audits {
		if a.Coverage != "full" && a.Coverage != "partial" {
			return nil, fmt.Errorf("mapping: audit %q has coverage %q, want full or partial", name, a.Coverage)
		}
		if len(a.Jactionlint) == 0 {
			return nil, fmt.Errorf("mapping: audit %q lists no jactionlint rules", name)
		}
		for _, j := range a.Jactionlint {
			if j.Rule == "" {
				return nil, fmt.Errorf("mapping: audit %q has an entry without a rule", name)
			}
		}
	}
	return &m, nil
}

func loadMapping(path string) (*Mapping, error) {
	if path == "" {
		return parseMapping(defaultMapping)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseMapping(b)
}

func (m Match) matches(f Finding) bool {
	return f.Rule == m.Rule && strings.Contains(f.Message, m.MessageContains)
}

// covers reports whether the jactionlint finding is one the audit maps to.
func (a AuditMapping) covers(f Finding) bool {
	for _, m := range a.Jactionlint {
		if m.matches(f) {
			return true
		}
	}
	return false
}

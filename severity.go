package jactionlint

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Severity is how serious a finding is. The zero value SeverityOff means a rule is disabled; no
// reported Error has this severity.
type Severity int

const (
	// SeverityOff disables a rule. It is only meaningful as a configured level.
	SeverityOff Severity = iota
	// SeverityInfo is for findings which are worth knowing but never fail a run.
	SeverityInfo
	// SeverityWarning is for findings which should be fixed but do not fail a run unless --strict-exit
	// is given.
	SeverityWarning
	// SeverityError is for findings which fail a run.
	SeverityError
)

// severityNames are the spellings accepted in the configuration file and on the command line.
var severityNames = [...]string{"off", "info", "warn", "error"}

// String returns the name used in configuration files and on the command line: "off", "info",
// "warn" or "error".
func (s Severity) String() string {
	if s < SeverityOff || int(s) >= len(severityNames) {
		return fmt.Sprintf("Severity(%d)", int(s))
	}
	return severityNames[s]
}

// ParseSeverity parses "off", "info", "warn" (or "warning") and "error".
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off":
		return SeverityOff, nil
	case "info":
		return SeverityInfo, nil
	case "warn", "warning":
		return SeverityWarning, nil
	case "error":
		return SeverityError, nil
	}
	return SeverityOff, fmt.Errorf("invalid severity %q. available values are \"off\", \"info\", \"warn\" and \"error\"", s)
}

// MarshalText implements encoding.TextMarshaler. JSON output uses the same names as the configuration.
func (s Severity) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler. It accepts the names of ParseSeverity.
func (s *Severity) UnmarshalText(b []byte) error {
	v, err := ParseSeverity(string(b))
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Severity) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("severity must be one of \"off\", \"info\", \"warn\" and \"error\" at line:%d,col:%d", n.Line, n.Column)
	}
	v, err := ParseSeverity(n.Value)
	if err != nil {
		return fmt.Errorf("%w at line:%d,col:%d", err, n.Line, n.Column)
	}
	*s = v
	return nil
}

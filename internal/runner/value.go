package runner

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/jdx/jactionlint/v2"
)

// rawToAny converts a value of the workflow (a matrix value) to the Go value that an expression sees: nil,
// bool, float64, string, []any or map[string]any. A scalar that the YAML parser did not tag as a string
// follows the core schema of YAML 1.2, which is what GitHub uses (so 3.10 is the number 3.1 and "3.10" is a string).
func rawToAny(v jactionlint.RawYAMLValue) any {
	switch v := v.(type) {
	case *jactionlint.RawYAMLString:
		if v.StringTag {
			return v.Value
		}
		return scalarToAny(v.Value)
	case *jactionlint.RawYAMLArray:
		a := make([]any, len(v.Elems))
		for i, e := range v.Elems {
			a[i] = rawToAny(e)
		}
		return a
	case *jactionlint.RawYAMLObject:
		m := make(map[string]any, len(v.Props))
		for k, e := range v.Props {
			m[k] = rawToAny(e)
		}
		return m
	default:
		return nil
	}
}

func scalarToAny(s string) any {
	switch s {
	case "", "~", "null", "Null", "NULL":
		return nil
	case "true", "True", "TRUE":
		return true
	case "false", "False", "FALSE":
		return false
	}
	if n, ok := parseYAMLNumber(s); ok {
		return n
	}
	return s
}

func parseYAMLNumber(s string) (float64, bool) {
	t := strings.ReplaceAll(s, "_", "")
	if t != s {
		return 0, false // YAML 1.2 core has no digit separators
	}
	switch s {
	case ".inf", ".Inf", ".INF", "+.inf", "+.Inf", "+.INF":
		return math.Inf(1), true
	case "-.inf", "-.Inf", "-.INF":
		return math.Inf(-1), true
	case ".nan", ".NaN", ".NAN":
		return math.NaN(), true
	}
	if len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'o') {
		base := 16
		if s[1] == 'o' {
			base = 8
		}
		if n, err := strconv.ParseInt(s[2:], base, 64); err == nil {
			return float64(n), true
		}
		return 0, false
	}
	c := s[0]
	if !(c >= '0' && c <= '9' || c == '-' || c == '+' || c == '.') {
		return 0, false
	}
	if strings.ContainsAny(s, "xXoObBpP") || strings.HasSuffix(s, "e") || strings.HasSuffix(s, ".") && len(s) == 1 {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// formatScalar renders a scalar like GitHub does in a job name or a string conversion: numbers without trailing
// zeros, booleans lower case, null empty.
func formatScalar(v any) (string, bool) {
	switch v := v.(type) {
	case nil:
		return "", true
	case bool:
		return strconv.FormatBool(v), true
	case float64:
		return formatNumber(v), true
	case string:
		return v, true
	}
	return "", false
}

func formatNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	if abs := math.Abs(f); abs >= 1e21 || abs < 1e-6 {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// sameValue reports whether a matrix exclude entry matches a value of a combination: strings compare without
// regard to case, a mapping in the exclude entry matches when each of its keys matches, other kinds compare equal.
func matchValue(have, want any) bool {
	switch want := want.(type) {
	case string:
		h, ok := have.(string)
		return ok && strings.EqualFold(h, want)
	case map[string]any:
		h, ok := have.(map[string]any)
		if !ok {
			return false
		}
		for k, w := range want {
			hv, ok := h[k]
			if !ok || !matchValue(hv, w) {
				return false
			}
		}
		return true
	case []any:
		h, ok := have.([]any)
		if !ok || len(h) != len(want) {
			return false
		}
		for i := range want {
			if !matchValue(h[i], want[i]) {
				return false
			}
		}
		return true
	case nil:
		return have == nil
	case bool:
		h, ok := have.(bool)
		return ok && h == want
	case float64:
		h, ok := have.(float64)
		return ok && (h == want || math.IsNaN(h) && math.IsNaN(want))
	}
	return false
}

// equalValue is strict equality used to decide whether an include entry would overwrite an original value.
func equalValue(a, b any) bool {
	if s, ok := a.(string); ok {
		t, ok := b.(string)
		return ok && s == t
	}
	return matchValue(a, b) && matchValue(b, a)
}

// valueText renders any value for display: scalars as formatScalar does, containers as JSON.
func valueText(v any) string {
	if s, ok := formatScalar(v); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "?"
	}
	return string(b)
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

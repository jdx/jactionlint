// Package eval evaluates the expressions of a workflow (the text inside ${{ }}) with the coercion rules of GitHub
// Actions. A value is nil (null), bool, float64, string, []any or map[string]any.
package eval

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// FormatNumber renders a number the way GitHub does: integers without a fraction, no trailing zeros.
func FormatNumber(f float64) string {
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
	if abs := math.Abs(f); abs >= 1e15 || abs < 1e-5 {
		s := strconv.FormatFloat(f, 'G', -1, 64)
		// .NET writes 1E+21 and 1E-06
		if i := strings.IndexAny(s, "E"); i >= 0 {
			exp := s[i+1:]
			sign := exp[:1]
			digits := strings.TrimLeft(exp[1:], "0")
			for len(digits) < 2 {
				digits = "0" + digits
			}
			return s[:i] + "E" + sign + digits
		}
		return s
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ToNumber converts a value to a number: null is 0, booleans are 0 and 1, a string is parsed (the empty string is 0,
// an invalid one NaN), arrays and objects are NaN.
func ToNumber(v any) float64 {
	switch v := v.(type) {
	case nil:
		return 0
	case bool:
		if v {
			return 1
		}
		return 0
	case float64:
		return v
	case string:
		return parseNumber(v)
	}
	return math.NaN()
}

func parseNumber(s string) float64 {
	s = strings.TrimFunc(s, unicode.IsSpace)
	if s == "" {
		return 0
	}
	switch s {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	body := strings.TrimLeft(s, "+-")
	if len(s)-len(body) > 1 {
		return math.NaN()
	}
	if len(body) > 2 && body[0] == '0' && (body[1] == 'x' || body[1] == 'X' || body[1] == 'o' || body[1] == 'O') {
		base := 16
		if body[1] == 'o' || body[1] == 'O' {
			base = 8
		}
		n, err := strconv.ParseUint(body[2:], base, 64)
		if err != nil {
			return math.NaN()
		}
		if s[0] == '-' {
			return -float64(n)
		}
		return float64(n)
	}
	for _, c := range body {
		if !(c >= '0' && c <= '9' || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-') {
			return math.NaN()
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return f
		}
		return math.NaN()
	}
	return f
}

// Truthy reports whether a value counts as true: false, null, 0, -0, NaN and "" are false, everything else (also
// empty arrays and objects) is true.
func Truthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return !(v == 0 || math.IsNaN(v))
	case string:
		return v != ""
	}
	return true
}

// ToString converts a value to the string that ${{ }} interpolation and format() produce. Arrays and objects become
// indented JSON.
func ToString(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return FormatNumber(v)
	case string:
		return v
	}
	return ToJSON(v)
}

// ToJSON renders a value as the two-space indented JSON of toJSON().
func ToJSON(v any) string {
	b, err := marshalIndent(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func marshalIndent(v any) ([]byte, error) {
	// Numbers that are whole print as integers; json does that for float64 already. NaN and Infinity are not JSON.
	return json.MarshalIndent(sanitize(v), "", "  ")
}

func sanitize(v any) any {
	switch v := v.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = sanitize(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = sanitize(e)
		}
		return out
	}
	return v
}

// Normalize converts the values that encoding/json, YAML and Go code produce (ints, []string, map[string]string, ...)
// to the value kinds above.
func Normalize(v any) any {
	switch v := v.(type) {
	case nil, bool, string, float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case []string:
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = Normalize(e)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(v))
		for k, s := range v {
			out[k] = s
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = Normalize(e)
		}
		return out
	}
	return v
}

func kind(v any) int {
	switch v.(type) {
	case nil:
		return 0
	case bool:
		return 1
	case float64:
		return 2
	case string:
		return 3
	}
	return 4
}

// Equal implements ==: values of one kind compare directly (strings without regard to case), other pairs compare as
// numbers, and an array or an object equals only itself.
func Equal(a, b any) bool {
	ka, kb := kind(a), kind(b)
	if ka == 4 || kb == 4 {
		return ka == 4 && kb == 4 && sameRef(a, b)
	}
	if ka == kb {
		switch a := a.(type) {
		case nil:
			return true
		case bool:
			return a == b.(bool)
		case float64:
			return a == b.(float64)
		case string:
			return strings.EqualFold(a, b.(string))
		}
	}
	return ToNumber(a) == ToNumber(b)
}

// Compare implements < <= > >=: ok is false when the values cannot be ordered (NaN, arrays, objects), and then every
// ordering comparison is false.
func Compare(a, b any) (c int, ok bool) {
	ka, kb := kind(a), kind(b)
	if ka == 4 || kb == 4 {
		return 0, false
	}
	if ka == 3 && kb == 3 {
		return strings.Compare(strings.ToUpper(a.(string)), strings.ToUpper(b.(string))), true
	}
	if ka == kb && ka == 0 {
		return 0, true
	}
	x, y := ToNumber(a), ToNumber(b)
	if math.IsNaN(x) || math.IsNaN(y) {
		return 0, false
	}
	switch {
	case x < y:
		return -1, true
	case x > y:
		return 1, true
	}
	return 0, true
}

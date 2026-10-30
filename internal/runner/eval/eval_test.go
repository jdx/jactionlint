package eval

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

func testEnv() *Env {
	return &Env{Contexts: map[string]any{
		"github": map[string]any{
			"event_name": "push",
			"ref":        "refs/heads/Main",
			"actor":      "Octocat",
			"event": map[string]any{
				"commits": []any{
					map[string]any{"message": "first", "id": "a1"},
					map[string]any{"message": "second", "id": "b2"},
					map[string]any{"id": "c3"},
				},
				"labels": map[string]any{"b": "bug", "a": "area"},
			},
		},
		"env":    map[string]any{"MODE": "fast", "EMPTY": ""},
		"matrix": map[string]any{"os": "ubuntu", "node": float64(20), "cfg": map[string]any{"go": "1.23"}, "list": []any{"x", "y", float64(3)}},
		"needs":  map[string]any{"build": map[string]any{"result": "success", "outputs": map[string]any{"matrix": `{"include":[{"os":"linux"}]}`}}},
	}}
}

func TestEval(t *testing.T) {
	tests := []struct {
		expr string
		want any
	}{
		// literals
		{"true", true}, {"false", false}, {"null", nil}, {"711", 711.0}, {"-9.2", -9.2}, {"0xff", 255.0},
		{"1.5e2", 150.0}, {"'it''s'", "it's"}, {"'Mona the Octocat'", "Mona the Octocat"},
		// contexts and access, case-insensitive
		{"github.event_name", "push"}, {"GITHUB.EVENT_NAME", "push"}, {"github['event_name']", "push"},
		{"github.nothing", nil}, {"nothing.at.all", nil}, {"matrix.cfg.go", "1.23"}, {"matrix.list[1]", "y"},
		{"matrix.list[5]", nil}, {"matrix.list['1']", "y"}, {"env.mode", "fast"},
		// filters
		{"github.event.commits.*.message", []any{"first", "second"}},
		{"github.event.labels.*", []any{"area", "bug"}},
		{"join(github.event.commits.*.message, '+')", "first+second"},
		// ! && ||
		{"!true", false}, {"!''", true}, {"!'x'", false}, {"!null", true}, {"!0", true}, {"!1", false},
		{"1 && 'a'", "a"}, {"0 && 'a'", 0.0}, {"'' || 'b'", "b"}, {"'x' || 'b'", "x"}, {"null || false", false},
		// comparison and coercion
		{"1 == 1", true}, {"'ABC' == 'abc'", true}, {"'abc' != 'abd'", true}, {"null == 0", true}, {"null == ''", true},
		{"true == 1", true}, {"false == ''", true}, {"'1' == 1", true}, {"'abc' == 1", false}, {"'' == 0", true},
		{"' 12 ' == 12", true}, {"'0x10' == 16", true},
		{"matrix.cfg == matrix.cfg", true}, {"matrix.cfg == github.event", false}, {"matrix.list == matrix.list", true},
		{"1 < 2", true}, {"2 <= 2", true}, {"3 > 2", true}, {"2 >= 3", false}, {"'a' < 'B'", true},
		{"'abc' < 1", false}, {"'abc' >= 1", false}, {"null < 1", true}, {"matrix.cfg < 1", false},
		{"'1.0' == 1", true}, {"true > false", true},
		// grouping and precedence
		{"(1 == 2) || (2 == 2)", true}, {"1 == 2 || 2 == 2", true}, {"true || false && false", true},
		{"!(1 == 2) && true", true},
		// functions
		{"contains('Hello world', 'llo')", true}, {"contains('Hello world', 'LLO')", true}, {"contains('abc', 'z')", false},
		{"contains(matrix.list, 'y')", true}, {"contains(matrix.list, 3)", true}, {"contains(matrix.list, 'z')", false},
		{"contains(github.event, 'x')", false}, {"contains(fromJSON('[\"push\",\"pull_request\"]'), github.event_name)", true},
		{"contains(12345, 34)", true},
		{"startsWith('Hello world', 'He')", true}, {"startsWith('Hello', 'hE')", true}, {"startsWith('Hello', 'x')", false},
		{"endsWith('Hello world', 'ld')", true}, {"endsWith(github.ref, 'MAIN')", true},
		{"format('Hello {0} {1} {2}', 'Mona', 'the', 'Octocat')", "Hello Mona the Octocat"},
		{"format('{{Hello {0} {1} {2}!}}', 'Mona', 'the', 'Octocat')", "{Hello Mona the Octocat!}"},
		{"format('{0}{0}', 1.5)", "1.51.5"}, {"format('{0}', null)", ""}, {"format('{0}', true)", "true"},
		{"join(matrix.list, ', ')", "x, y, 3"}, {"join(matrix.list)", "x,y,3"}, {"join('solo', ',')", "solo"},
		{"toJSON(matrix.list)", "[\n  \"x\",\n  \"y\",\n  3\n]"}, {"toJSON('a')", `"a"`}, {"toJSON(null)", "null"},
		{"toJSON(matrix.cfg)", "{\n  \"go\": \"1.23\"\n}"},
		{"fromJSON('{\"a\":[1,2]}').a[1]", 2.0}, {"fromJSON('true')", true}, {"fromJSON('\"s\"')", "s"},
		{"fromJSON('null')", nil}, {"fromJSON('1.5')", 1.5},
		{"fromJSON(needs.build.outputs.matrix).include[0].os", "linux"},
		{"success()", true}, {"always()", true}, {"failure()", false}, {"cancelled()", false},
		{"SUCCESS()", true},
		// interop
		{"matrix.node == 20", true}, {"matrix.node == '20'", true}, {"format('{0}', matrix.node)", "20"},
		{"format('{0}', 3.10)", "3.1"}, {"format('{0}', 1e21)", "1E+21"}, {"format('{0}', 100000)", "100000"},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			got, err := testEnv().Eval(tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			if a, b := ToJSON(got), ToJSON(tc.want); a != b {
				t.Fatalf("got %s (%T), want %s", a, got, b)
			}
		})
	}
}

func TestEvalErrors(t *testing.T) {
	for expr, msg := range map[string]string{
		"nope(1)":                 "unknown function",
		"contains('a')":           "wrong number of arguments",
		"format('{1}', 'a')":      "only 1 argument",
		"format('{', 'a')":        "unterminated",
		"format('}', 'a')":        "unmatched",
		"fromJSON('{bad')":        "invalid JSON",
		"fromJSON('')":            "empty input",
		"fromJSON('1 2')":         "unexpected data",
		"hashFiles('x')":          "not available",
		"1 +":                     "",
		"github.event_name ==":    "",
		"success(1)":              "wrong number of arguments",
		"format('{0}x', 1, 2) )":  "",
		"format('{abc}', 'a')":    "invalid placeholder",
		"toJSON()":                "wrong number of arguments",
		"startsWith('a')":         "wrong number of arguments",
		"join()":                  "wrong number of arguments",
		"join('a', 'b', 'c')":     "wrong number of arguments",
		"fromJSON('[1,2')":        "invalid JSON",
		"contains('a', 'b', 'c')": "wrong number of arguments",
	} {
		_, err := testEnv().Eval(expr)
		if err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("%s: err = %v, want %q", expr, err, msg)
		}
	}
}

func TestNumbers(t *testing.T) {
	for in, want := range map[string]float64{
		"": 0, "  ": 0, "12": 12, " 12\n": 12, "-3.5": -3.5, "+4": 4, "0x1F": 31, "0o17": 15, "1e3": 1000, ".5": 0.5,
		"Infinity": math.Inf(1), "-Infinity": math.Inf(-1),
	} {
		if got := ToNumber(in); got != want {
			t.Errorf("ToNumber(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"abc", "1 2", "--1", "0x", "12abc", "1,5", "NaN", "e5"} {
		if got := ToNumber(in); !math.IsNaN(got) {
			t.Errorf("ToNumber(%q) = %v, want NaN", in, got)
		}
	}
	if !math.IsNaN(ToNumber([]any{})) || !math.IsNaN(ToNumber(map[string]any{})) {
		t.Error("containers are NaN")
	}
	for f, want := range map[float64]string{0: "0", 1: "1", -2: "-2", 3.1: "3.1", 0.1: "0.1", 1e21: "1E+21", 1e-7: "1E-07", 123456789: "123456789", math.Inf(1): "Infinity"} {
		if got := FormatNumber(f); got != want {
			t.Errorf("FormatNumber(%v) = %q, want %q", f, got, want)
		}
	}
}

func TestStatusFunctions(t *testing.T) {
	failed := &Env{Status: func(n string) bool { return n == "failure" }}
	cancelled := &Env{Status: func(n string) bool { return n == "cancelled" || n == "always" }}
	for _, tc := range []struct {
		env  *Env
		cond string
		want bool
	}{
		{testEnv(), "", true},
		{testEnv(), "true", true},
		{testEnv(), "${{ false }}", false},
		{testEnv(), "github.event_name == 'push'", true},
		{testEnv(), "${{ github.event_name == 'push' }}", true},
		{failed, "", false},
		{failed, "true", false}, // implicit success() is false
		{failed, "failure()", true},
		{failed, "failure() && true", true},
		{failed, "${{ always() }}", false},
		{cancelled, "always()", true},
		{cancelled, "cancelled()", true},
		{cancelled, "!cancelled()", false},
		{cancelled, "github.event_name == 'push'", false},
		{testEnv(), "  ${{ !cancelled() }}  ", true},
		{testEnv(), "prefix ${{ 'x' }}", true}, // a string with text around a placeholder is a non-empty string
		{testEnv(), "${{ '' }}", false},
	} {
		got, err := tc.env.Condition(tc.cond)
		if err != nil || got != tc.want {
			t.Errorf("Condition(%q) = %v, %v; want %v", tc.cond, got, err, tc.want)
		}
	}
}

func TestInterpolate(t *testing.T) {
	e := testEnv()
	for in, want := range map[string]string{
		"plain":                             "plain",
		"os: ${{ matrix.os }}":              "os: ubuntu",
		"${{matrix.os}}-${{ matrix.node }}": "ubuntu-20",
		"${{ 'a}}b' }}":                     "a}}b",
		"${{ 'it''s' }}":                    "it's",
		"x${{ null }}y":                     "xy",
		"${{ matrix.cfg }}":                 "{\n  \"go\": \"1.23\"\n}",
		"${{ true }}${{ false }}":           "truefalse",
	} {
		got, err := e.Interpolate(in)
		if err != nil || got != want {
			t.Errorf("Interpolate(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := e.Interpolate("${{ matrix.os "); err == nil {
		t.Error("unterminated placeholder must fail")
	}
	v, err := e.Value("${{ fromJSON(needs.build.outputs.matrix) }}")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(map[string]any); !ok {
		t.Errorf("Value = %T", v)
	}
	if v, _ := e.Value("a-${{ matrix.os }}"); v != "a-ubuntu" {
		t.Errorf("Value mixed = %v", v)
	}
}

func TestContexts(t *testing.T) {
	got, err := StringContexts("${{ github.ref }} ${{ needs.a.outputs.b && matrix.x || github.sha }}")
	if err != nil || strings.Join(got, ",") != "github,needs,matrix" {
		t.Fatalf("%v %v", got, err)
	}
}

func TestHashFiles(t *testing.T) {
	e := testEnv()
	e.HashFiles = func(p []string) (string, error) { return "h:" + strings.Join(p, ","), nil }
	got, err := e.Eval("hashFiles('**/go.sum', 'x')")
	if err != nil || got != "h:**/go.sum,x" {
		t.Fatalf("%v %v", got, err)
	}
}

// corpus.json holds (expression, context) -> result cases. See testdata/README.md for where the values come from.
func TestCorpus(t *testing.T) {
	b, err := os.ReadFile("testdata/corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Expr    string         `json:"expr"`
		Context map[string]any `json:"context"`
		Want    any            `json:"want"`
		Error   bool           `json:"error"`
		Source  string         `json:"source"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 40 {
		t.Fatalf("corpus too small: %d", len(cases))
	}
	for _, c := range cases {
		env := &Env{Contexts: map[string]any{}}
		for k, v := range c.Context {
			env.Contexts[k] = Normalize(v)
		}
		got, err := env.Eval(c.Expr)
		if c.Error {
			if err == nil {
				t.Errorf("%s: want an error, got %v", c.Expr, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.Expr, err)
			continue
		}
		if a, b := ToJSON(got), ToJSON(Normalize(c.Want)); a != b {
			t.Errorf("%s (%s): got %s, want %s", c.Expr, c.Source, a, b)
		}
	}
}

func FuzzEval(f *testing.F) {
	for _, s := range []string{"1 == 1", "github.event.commits.*.message", "format('{0}{1}', 1)", "fromJSON('{')", "a[b[c]]", "'", "((((1))))", "!!!x", "contains(a,b)||c&&d", "x.*.*.y"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		e := testEnv()
		// An expression may fail but never panic or hang.
		v, _ := e.Eval(expr)
		_ = ToString(v)
		_, _ = e.Interpolate(expr)
		_, _ = e.Condition(expr)
	})
}

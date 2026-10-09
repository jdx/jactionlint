package jactionlint

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lintPipefail(t *testing.T, src string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig("pipeline-without-pipefail")
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	var ret []*Error
	for _, e := range errs {
		if e.ID == "pipeline-without-pipefail" {
			ret = append(ret, e)
		}
	}
	return ret
}

func pipefailWorkflow(header, run string) string {
	return "on: push\n" + header + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - " + run
}

func TestRulePipelineWithoutPipefailDetection(t *testing.T) {
	tests := []struct {
		what string
		src  string
		want int
	}{
		{"default shell", pipefailWorkflow("", "run: make | tee out\n"), 1},
		{"status of the first stage read right after", pipefailWorkflow("", "run: |\n          set +e\n          make 2>&1 | tee out.log\n          status=${PIPESTATUS[0]}\n          exit \"$status\"\n"), 0},
		{"status of every stage copied right after", pipefailWorkflow("", "run: |\n          make | tee out\n          rc=(\"${PIPESTATUS[@]}\")\n"), 0},
		{"status read in the next command", pipefailWorkflow("", "run: |\n          make | tee out\n          [ \"${PIPESTATUS[0]}\" -eq 0 ]\n"), 0},
		{"status read only after another command", pipefailWorkflow("", "run: |\n          make | tee out\n          echo done\n          echo \"${PIPESTATUS[0]}\"\n"), 1},
		{"quoted single line", pipefailWorkflow("", "run: 'make | tee out'\n"), 1},
		{"double quoted", pipefailWorkflow("", "run: \"make | tee out\"\n"), 1},
		{"folded", pipefailWorkflow("", "run: >\n          make\n          | tee out\n"), 1},
		{"workflow default shell is bash", pipefailWorkflow("defaults:\n  run:\n    shell: bash\n", "run: make | tee out\n"), 0},
		{"workflow default shell is sh", pipefailWorkflow("defaults:\n  run:\n    shell: sh\n", "run: make | tee out\n"), 1},
		{"step shell overrides workflow shell", pipefailWorkflow("defaults:\n  run:\n    shell: sh\n", "shell: bash\n        run: make | tee out\n"), 0},
		{"step shell sh overrides workflow bash", pipefailWorkflow("defaults:\n  run:\n    shell: bash\n", "shell: sh\n        run: make | tee out\n"), 1},
		{"pwsh", pipefailWorkflow("", "shell: pwsh\n        run: make | tee out\n"), 0},
		{"python", pipefailWorkflow("", "shell: python\n        run: print(1)\n"), 0},
		{"template with pipefail", pipefailWorkflow("", "shell: bash -eo pipefail {0}\n        run: make | tee out\n"), 0},
		{"template with errexit", pipefailWorkflow("", "shell: bash -e {0}\n        run: make | tee out\n"), 1},
		{"template with long errexit", pipefailWorkflow("", "shell: bash -o errexit {0}\n        run: make | tee out\n"), 1},
		{"template without errexit", pipefailWorkflow("", "shell: bash {0}\n        run: make | tee out\n"), 0},
		{"template without errexit but script sets it", pipefailWorkflow("", "shell: bash {0}\n        run: |\n          set -e\n          make | tee out\n"), 1},
		{"non-shell template", pipefailWorkflow("", "shell: python {0}\n        run: print(1)\n"), 0},
		{"bash with arguments is not a name", pipefailWorkflow("", "shell: bash -e\n        run: make | tee out\n"), 0},
		{"while read loop reads to the end", pipefailWorkflow("", "run: |\n          make | while read -r l; do echo \"$l\"; done\n"), 1},
		{"while IFS read loop", pipefailWorkflow("", "run: |\n          make | while IFS= read -r l; do echo \"$l\"; done\n"), 1},
		{"single read takes one line", pipefailWorkflow("", "run: |\n          make | { read -r first; echo \"$first\"; }\n"), 0},
		{"cat of a file feeding a pipe", pipefailWorkflow("", "run: |\n          cat < in.txt | sort\n"), 1},
		{"cat of the previous stage", pipefailWorkflow("", "run: |\n          make | cat | sort\n"), 1},
		{"cat of a here document", pipefailWorkflow("", "run: |\n          cat <<EOF | sort\n          b\n          EOF\n"), 0},
		{"sh template", pipefailWorkflow("", "shell: sh -e {0}\n        run: make | tee out\n"), 1},
		{"nested pipeline in a group", pipefailWorkflow("", "run: |\n          { echo a | echo b | make; } | wc -l\n"), 1},
		{"nested pipeline that ends in head", pipefailWorkflow("", "run: |\n          { echo a | echo b | make; } | head -1\n"), 0},
		{"head in a loop body does not end the stream", pipefailWorkflow("", "run: |\n          make | while read -r l; do echo \"$l\" | head -n 1; done\n"), 1},
		{"head in a nested pipeline of a stage does not end the stream", pipefailWorkflow("", "run: |\n          make | { cat; echo x | head -n 1; }\n"), 1},
		{"group that fails with the left side of &&", pipefailWorkflow("", "run: |\n          { make && echo OK; } | tee out\n"), 1},
		{"later statement overwrites the status of the && list", pipefailWorkflow("", "run: |\n          { make && echo OK; true; } | tee out\n"), 0},
		{"later statement of a subshell overwrites the status of the && list", pipefailWorkflow("", "run: |\n          ( make && echo OK; true ) | tee out\n"), 0},
		{"errexit in a subshell makes the failure the status of the group", pipefailWorkflow("", "run: |\n          ( set -e; make; echo OK ) | tee out\n"), 1},
		{"inner pipeline in a group followed by a statement", pipefailWorkflow("", "run: |\n          { { make && echo OK; } | tee a; true; } | tee out\n"), 2},
		{"nested group followed by a statement", pipefailWorkflow("", "run: |\n          { { make && echo OK; }; true; } | tee out\n"), 0},
		{"substitution in a group followed by a statement", pipefailWorkflow("", "run: |\n          { echo \"$(make && echo OK)\"; true; } | tee out\n"), 0},
		{"group with || true", pipefailWorkflow("", "run: |\n          { make || true; } | tee out\n"), 0},
		{"read in an if takes one line", pipefailWorkflow("", "run: |\n          make | { if read -r x; then echo \"$x\"; fi; }\n"), 0},
		{"read after && takes one line", pipefailWorkflow("", "run: |\n          make | { read -r x && echo \"$x\"; }\n"), 0},
		{"set -o pipefail", pipefailWorkflow("", "run: |\n          set -o pipefail\n          make | tee out\n"), 0},
		{"set -eo pipefail", pipefailWorkflow("", "run: |\n          set -eo pipefail\n          make | tee out\n"), 0},
		{"set -euxo pipefail", pipefailWorkflow("", "run: |\n          set -euxo pipefail\n          make | tee out\n"), 0},
		{"set -o errexit -o pipefail", pipefailWorkflow("", "run: |\n          set -o errexit -o pipefail\n          make | tee out\n"), 0},
		{"set -e -o pipefail", pipefailWorkflow("", "run: |\n          set -e -o pipefail\n          make | tee out\n"), 0},
		{"set -o nounset is not pipefail", pipefailWorkflow("", "run: |\n          set -o nounset\n          make | tee out\n"), 1},
		{"set -eu is not pipefail", pipefailWorkflow("", "run: |\n          set -eu\n          make | tee out\n"), 1},
		{"set with a variable name pipefail", pipefailWorkflow("", "run: |\n          set -- pipefail\n          make | tee out\n"), 1},
		{"pipefail after the pipeline", pipefailWorkflow("", "run: |\n          make | tee out\n          set -o pipefail\n"), 1},
		{"pipefail disabled again", pipefailWorkflow("", "run: |\n          set -o pipefail\n          set +o pipefail\n          make | tee out\n"), 1},
		{"SHELLOPTS", pipefailWorkflow("", "run: |\n          export SHELLOPTS=pipefail\n          make | tee out\n"), 0},
		{"pipefail in a subshell before", pipefailWorkflow("", "run: |\n          ( set -o pipefail; make | tee out )\n"), 0},
		{"two pipelines", pipefailWorkflow("", "run: |\n          make | tee a\n          make | tee b\n"), 2},
		{"three stages", pipefailWorkflow("", "run: make | sort | tee out\n"), 1},
		{"pipeline in command substitution", pipefailWorkflow("", "run: |\n          x=$(make | sort)\n          echo \"$x\"\n"), 1},
		{"pipe all", pipefailWorkflow("", "run: make |& tee out\n"), 1},
		{"expression in command", pipefailWorkflow("", "run: ${{ matrix.cmd }} | tee out\n"), 1},
		{"stderr redirect before pipe", pipefailWorkflow("", "run: make 2>&1 | tee out\n"), 1},
		{"sudo wrapper", pipefailWorkflow("", "run: sudo make | tee out\n"), 1},
		{"group in the first stage", pipefailWorkflow("", "run: |\n          { make; make test; } | tee out\n"), 1},
		{"subshell in the first stage", pipefailWorkflow("", "run: |\n          (cd src && make) | tee out\n"), 1},
		{"first stage echo", pipefailWorkflow("", "run: echo hi | make\n"), 0},
		{"first stage printf", pipefailWorkflow("", "run: printf '%s' hi | make\n"), 0},
		{"first stage cat of a heredoc", pipefailWorkflow("", "run: |\n          cat <<'EOT' | make\n          hi\n          EOT\n"), 0},
		{"first stage cat of a here string", pipefailWorkflow("", "run: cat <<< hi | make\n"), 0},
		{"first stage cat of a file", pipefailWorkflow("", "run: cat file | make\n"), 1},
		{"first stage true", pipefailWorkflow("", "run: true | make\n"), 0},
		{"first stage yes", pipefailWorkflow("", "run: yes | make\n"), 0},
		{"first stage env alone", pipefailWorkflow("", "run: env | sort\n"), 0},
		{"first stage echo in a loop", pipefailWorkflow("", "run: |\n          for i in 1 2; do echo $i; done | make\n"), 0},
		{"filter in the middle", pipefailWorkflow("", "run: echo a | grep a | make\n"), 0},
		{"middle stage which matters", pipefailWorkflow("", "run: echo a | jq . | make\n"), 1},
		{"if condition", pipefailWorkflow("", "run: |\n          if make | grep -q x; then echo y; fi\n"), 0},
		{"if body", pipefailWorkflow("", "run: |\n          if true; then make | tee out; fi\n"), 1},
		{"elif condition", pipefailWorkflow("", "run: |\n          if true; then echo; elif make | grep x; then echo y; fi\n"), 0},
		{"while condition", pipefailWorkflow("", "run: |\n          while make | grep x; do sleep 1; done\n"), 0},
		{"negated", pipefailWorkflow("", "run: |\n          ! make | grep x\n"), 0},
		{"or true", pipefailWorkflow("", "run: make | grep x || true\n"), 0},
		{"and then", pipefailWorkflow("", "run: make | grep x && echo found\n"), 0},
		{"last of an and list", pipefailWorkflow("", "run: |\n          true && make | tee out\n"), 1},
		{"middle of an and list", pipefailWorkflow("", "run: |\n          true && make | tee out && echo ok\n"), 0},
		{"or true in a group", pipefailWorkflow("", "run: |\n          { make || true; } | tee out\n"), 0},
		{"or true in a group next to a command which matters", pipefailWorkflow("", "run: |\n          { make || true; cat file; } | sort\n"), 1},
		{"negated command in a group", pipefailWorkflow("", "run: |\n          { ! make; } | sort\n"), 0},
		{"head", pipefailWorkflow("", "run: make | head -n 1\n"), 0},
		{"head in the middle", pipefailWorkflow("", "run: make | head -n 1 | tr a b\n"), 0},
		{"grep -q", pipefailWorkflow("", "run: make | grep -q ok\n"), 0},
		{"grep clustered quiet", pipefailWorkflow("", "run: make | grep -qE ok\n"), 0},
		{"grep --quiet", pipefailWorkflow("", "run: make | grep --quiet ok\n"), 0},
		{"grep -m1", pipefailWorkflow("", "run: make | grep -m1 ok\n"), 0},
		{"grep --max-count", pipefailWorkflow("", "run: make | grep --max-count=1 ok\n"), 0},
		{"grep without early exit", pipefailWorkflow("", "run: make | grep ok\n"), 1},
		{"grep as the producer", pipefailWorkflow("", "run: grep ok file | cut -d: -f1\n"), 0},
		{"rg as the producer", pipefailWorkflow("", "run: rg ok | sort\n"), 0},
		{"diff as the producer", pipefailWorkflow("", "run: diff a b | sort\n"), 0},
		{"sed as the producer", pipefailWorkflow("", "run: sed s/a/b/ file | sort\n"), 1},
		{"grep -i without early exit", pipefailWorkflow("", "run: make | grep -i ok\n"), 1},
		{"sed q", pipefailWorkflow("", "run: make | sed 1q\n"), 0},
		{"sed -n with q", pipefailWorkflow("", "run: make | sed -n '/x/{p;q}'\n"), 0},
		{"sed without q", pipefailWorkflow("", "run: make | sed 's/a/b/'\n"), 1},
		{"awk exit", pipefailWorkflow("", "run: make | awk '{print; exit}'\n"), 0},
		{"awk without exit", pipefailWorkflow("", "run: make | awk '{print $1}'\n"), 1},
		{"read", pipefailWorkflow("", "run: make | read -r x\n"), 0},
		{"tail is not early", pipefailWorkflow("", "run: make | tail -n 1\n"), 1},
		{"no pipeline", pipefailWorkflow("", "run: make && make test\n"), 0},
		{"or operator", pipefailWorkflow("", "run: make || make test\n"), 0},
		{"pipe in quotes", pipefailWorkflow("", "run: echo 'a | b'\n"), 0},
		{"does not parse", pipefailWorkflow("", "run: if then | fi\n"), 0},
		{"windows", "on: push\njobs:\n  j:\n    runs-on: windows-latest\n    steps:\n      - run: make | tee out\n", 0},
		{"windows explicit sh still reported", "on: push\njobs:\n  j:\n    runs-on: windows-latest\n    steps:\n      - shell: sh\n        run: make | tee out\n", 1},
		{"macos", "on: push\njobs:\n  j:\n    runs-on: macos-latest\n    steps:\n      - run: make | tee out\n", 1},
		{"self-hosted linux", "on: push\njobs:\n  j:\n    runs-on: [self-hosted, linux, x64]\n    steps:\n      - run: make | tee out\n", 1},
		{"self-hosted windows", "on: push\njobs:\n  j:\n    runs-on: [self-hosted, windows]\n    steps:\n      - run: make | tee out\n", 0},
		{"self-hosted without OS", "on: push\njobs:\n  j:\n    runs-on: [self-hosted, x64]\n    steps:\n      - run: make | tee out\n", 0},
		{"custom windows label", "on: push\njobs:\n  j:\n    runs-on: [self-hosted, jdx-win-ci]\n    steps:\n      - run: make | tee out\n", 0},
		{"custom ubuntu label", "on: push\njobs:\n  j:\n    runs-on: blacksmith-4vcpu-ubuntu-2404\n    steps:\n      - run: make | tee out\n", 1},
		{"runner group", "on: push\njobs:\n  j:\n    runs-on:\n      group: mine\n    steps:\n      - run: make | tee out\n", 0},
		{"matrix all unix", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest, macos-latest]\n    steps:\n      - run: make | tee out\n", 1},
		{"matrix with windows", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest, windows-latest]\n    steps:\n      - run: make | tee out\n", 0},
		{"matrix include with windows", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n        include:\n          - os: windows-latest\n    steps:\n      - run: make | tee out\n", 0},
		{"matrix include only", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        include:\n          - os: ubuntu-latest\n          - os: macos-latest\n    steps:\n      - run: make | tee out\n", 1},
		{"matrix from expression", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: ${{ fromJSON(vars.OSES) }}\n    steps:\n      - run: make | tee out\n", 0},
		{"matrix with an unknown label", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest, my-box]\n    steps:\n      - run: make | tee out\n", 0},
		{"expression with a literal linux label next to it", "on: push\njobs:\n  j:\n    runs-on: [self-hosted, linux, '${{ vars.ARCH }}']\n    steps:\n      - run: make | tee out\n", 1},
		{"conditional expression", "on: push\njobs:\n  j:\n    runs-on: ${{ github.event.x && 'ubuntu-latest' || 'windows-latest' }}\n    steps:\n      - run: make | tee out\n", 0},
		{"job shell", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: bash\n    steps:\n      - run: make | tee out\n", 0},
		{"CRLF", strings.ReplaceAll(pipefailWorkflow("", "run: |\n          make | tee out\n"), "\n", "\r\n"), 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := lintPipefail(t, tc.src)
			if len(errs) != tc.want {
				t.Fatalf("want %d errors but got %d: %v", tc.want, len(errs), errs)
			}
		})
	}
}

func TestRulePipelineWithoutPipefailMessage(t *testing.T) {
	errs := lintPipefail(t, pipefailWorkflow("", "run: |\n          echo start\n          gh api repos/o/r | jq .name\n"))
	if len(errs) != 1 {
		t.Fatalf("got %v", errs)
	}
	e := errs[0]
	if e.Line != 8 || e.Column != 11 {
		t.Errorf("position: %d:%d", e.Line, e.Column)
	}
	for _, s := range []string{`"gh"`, `"gh api repos/o/r | jq .name"`, "set -o pipefail", "shell: bash"} {
		if !strings.Contains(e.Message, s) {
			t.Errorf("message %q must contain %q", e.Message, s)
		}
	}
	if e.Severity != SeverityError {
		t.Errorf("severity: %v", e.Severity)
	}
}

func TestRulePipelineWithoutPipefailCanBeTurnedOff(t *testing.T) {
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = mustParseConfig(t, "rules:\n  pipeline-without-pipefail: off\n  missing-timeout: off\n")
	errs, err := l.Lint("test.yaml", []byte(pipefailWorkflow("", "run: make | tee out\n")), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 0 {
		t.Fatalf("want no errors: %v", errs)
	}
}

func TestRulePipelineWithoutPipefailFix(t *testing.T) {
	tests := []struct {
		what    string
		src     string
		want    string // empty: not fixed
		applied int
	}{
		{
			"literal block",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          make | tee out\n",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          set -o pipefail\n          make | tee out\n",
			1,
		},
		{
			"two pipelines in one script get one line, the identical edits are applied once",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo hi\n          make | tee a\n          make | tee b\n",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          set -o pipefail\n          echo hi\n          make | tee a\n          make | tee b\n",
			2,
		},
		{
			"strip chomping and blank first line",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |-\n\n          make | tee out\n",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |-\n\n          set -o pipefail\n          make | tee out\n",
			1,
		},
		{
			"first line is more indented than the block",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          make | tee out\n            continue\n",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          set -o pipefail\n          make | tee out\n            continue\n",
			1,
		},
		{
			"CRLF",
			"on: push\r\njobs:\r\n  j:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - run: |\r\n          make | tee out\r\n",
			"on: push\r\njobs:\r\n  j:\r\n    runs-on: ubuntu-latest\r\n    steps:\r\n      - run: |\r\n          set -o pipefail\r\n          make | tee out\r\n",
			1,
		},
		{
			"custom bash template",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - shell: bash -e {0}\n        run: |\n          make | tee out\n",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - shell: bash -e {0}\n        run: |\n          set -o pipefail\n          make | tee out\n",
			1,
		},
		{
			"keyed before the shell",
			"on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          make | tee out\n        shell: sh\n",
			"",
			0,
		},
		{"single line", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make | tee out\n", "", 0},
		{"folded block", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: >\n          make | tee out\n", "", 0},
		{"quoted", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"make | tee out\"\n", "", 0},
		{"sh has no pipefail in dash", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - shell: sh\n        run: |\n          make | tee out\n", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			root := writeProject(t, map[string]string{".github/workflows/ci.yaml": tc.src})
			path := filepath.Join(root, ".github", "workflows", "ci.yaml")
			newLinter := func() *Linter {
				l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
				if err != nil {
					t.Fatal(err)
				}
				l.defaultConfig = fixtureConfig("pipeline-without-pipefail")
				return l
			}

			// Safe mode does not touch the file
			res, err := newLinter().FixFiles([]string{path}, nil, FixModeSafe)
			if err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(path); string(b) != tc.src || res.Applied != 0 {
				t.Fatalf("safe mode changed the file or applied a fix: %+v\n%s", res, b)
			}

			res, err = newLinter().FixFiles([]string{path}, nil, FixModeUnsafe)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(path)
			if tc.want == "" {
				if string(b) != tc.src || res.Applied != 0 {
					t.Errorf("must not be fixed: %+v\n%s", res, b)
				}
				if len(res.Errors) == 0 {
					t.Errorf("the finding must remain")
				}
				return
			}
			if string(b) != tc.want {
				t.Errorf("fixed file:\n%q\nwant:\n%q", b, tc.want)
			}
			if res.Applied != tc.applied {
				t.Errorf("applied %d fixes, want %d", res.Applied, tc.applied)
			}
			// The fixed file is clean
			if len(res.Errors) != 0 {
				t.Errorf("errors remain after the fix: %v", res.Errors)
			}
			errs := lintPipefail(t, string(b))
			if len(errs) != 0 {
				t.Errorf("the fixed file is not clean: %v", errs)
			}
		})
	}
}

func TestRulePipelineWithoutPipefailFixIsUnsafe(t *testing.T) {
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{Format: FormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig("pipeline-without-pipefail")
	errs, err := l.Lint("test.yaml", []byte(pipefailWorkflow("", "run: |\n          make | tee out\n")), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].Fix == nil || !errs[0].Fix.Unsafe {
		t.Fatalf("want one unsafe fix: %v", errs)
	}
}

// The advice for a template of sh is the one of "shell: sh": dash has no pipefail, so neither "-o pipefail" nor
// "set -o pipefail" works there.
func TestRulePipelineWithoutPipefailAdviceForShTemplate(t *testing.T) {
	errs := lintPipefail(t, pipefailWorkflow("", "shell: sh -e {0}\n        run: make | tee out\n"))
	if len(errs) != 1 {
		t.Fatalf("got %v", errs)
	}
	m := errs[0].Message
	if strings.Contains(m, "-o pipefail") || !strings.Contains(m, `use "shell: bash"`) || errs[0].Fix != nil {
		t.Errorf("unexpected message or fix: %q %v", m, errs[0].Fix)
	}
	bash := lintPipefail(t, pipefailWorkflow("", "shell: bash -e {0}\n        run: make | tee out\n"))
	if len(bash) != 1 || !strings.Contains(bash[0].Message, "-o pipefail") {
		t.Errorf("bash template: %v", bash)
	}
}

// The fix turns pipefail on for the whole script, so a script with a pipeline that quits early is not fixed.
func TestRulePipelineWithoutPipefailNoFixNextToEarlyExit(t *testing.T) {
	both := lintPipefail(t, pipefailWorkflow("", "run: |\n          make | tee out\n          yes | head -1\n"))
	if len(both) != 1 || both[0].Fix != nil {
		t.Errorf("a script with a pipeline that ends in head must not get the fix: %v", both)
	}
	alone := lintPipefail(t, pipefailWorkflow("", "run: |\n          make | tee out\n"))
	if len(alone) != 1 || alone[0].Fix == nil {
		t.Errorf("a script without one gets the fix: %v", alone)
	}
}

package jactionlint

import (
	"strings"
	"testing"
	"time"
)

// TestScanJSCommandSinksRobust cuts a script at every offset and scans a large one: the scanner must neither panic
// nor take long, whatever the input looks like.
func TestScanJSCommandSinksRobust(t *testing.T) {
	env := func(name string) (data, bool) {
		return data{kind: dataUntrusted, input: "github.event.comment.body", via: name}, true
	}
	script := "const { execSync } = require('child_process');\nconst { BODY: b, TITLE } = process.env;\nconst re = /^[a-z]+$/.test(b) ? `x${b}` : \"y'\";\n" +
		"execSync(`echo ${process.env.BODY} ${ {a: `${TITLE}`}.a }`, { env: { ...process.env } }); // comment `\n/* block ${ */ await exec.exec('bash', ['-c', `echo ${b}`]);\nspawnSync(\"sh\", [\"-c\", b + 'x']);\nfoo(((((((\n"
	for i := 0; i <= len(script); i++ {
		scanJSCommandSinks(script[:i], env)
		scanJSCommandSinks(script[i:], env)
	}
	big := "const { execSync } = require('child_process');\n" + strings.Repeat("const a = process.env.BODY; execSync(`echo ${a} ${process.env.BODY}`);\n", 5000)
	start := time.Now()
	hits := scanJSCommandSinks(big, env)
	if len(hits) != 5000 {
		t.Errorf("%d hits, want 5000", len(hits))
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("a script of 5000 lines took %s", d)
	}
}

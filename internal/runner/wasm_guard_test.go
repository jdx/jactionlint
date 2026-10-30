package runner

import (
	"os/exec"
	"strings"
	"testing"
)

// The playground is the root package built for GOOS=js; nothing it links may reach the runner or what it runs
// steps with (the sandbox, the cache service).
func TestPlaygroundDoesNotLinkRunner(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}
	cmd := exec.Command(goBin, "list", "-deps", "-f", "{{.ImportPath}}", "./playground")
	cmd.Dir = "../.."
	cmd.Env = append(cmd.Environ(), "GOOS=js", "GOARCH=wasm")
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("go list for the playground failed, so the guard checked nothing: %v\n%s", err, stderr)
	}
	for _, pkg := range strings.Fields(string(out)) {
		for _, banned := range []string{"/internal/runner", "/internal/sandbox"} {
			if strings.Contains(pkg, banned) {
				t.Errorf("the playground (GOOS=js GOARCH=wasm) links %s", pkg)
			}
		}
	}
}

//go:build darwin

package sandbox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The Seatbelt backend, on a Mac: a command runs, and cannot read the home directory or write outside its allow list.
func TestSeatbeltEscape(t *testing.T) {
	if err := Available(NetOpen); err != nil {
		t.Skip(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	real, err := os.MkdirTemp(home, ".jactionlint-escape-")
	if err != nil {
		t.Skipf("cannot create a directory under %s: %v", home, err)
	}
	defer os.RemoveAll(real)
	work := filepath.Join(real, "run", "work")
	fake := filepath.Join(real, "run", "home")
	tmp := filepath.Join(real, "run", "tmp")
	for _, d := range []string{work, fake, tmp, filepath.Join(real, ".ssh")} {
		os.MkdirAll(d, 0o755)
	}
	secret := filepath.Join(real, ".ssh", "id_rsa")
	os.WriteFile(secret, []byte("TOP-SECRET"), 0o600)

	run := func(script string) (string, int) {
		pol := DefaultPolicy(Options{Workspace: work, Home: fake, Temp: tmp, ToolCache: filepath.Join(real, "run", "tools"), PathDirs: []string{"/usr/bin", "/bin"}, Net: NetOpen})
		pol.Argv = []string{"sh", "-c", script}
		pol.Env = ScrubEnv(os.Environ(), DefaultPassEnv, map[string]string{"HOME": fake, "TMPDIR": tmp, "PATH": "/usr/bin:/bin"})
		pol.Dir = work
		var out bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		code, err := Run(ctx, pol, IO{Stdout: &out, Stderr: &out})
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		return out.String(), code
	}
	if out, code := run(`echo hello > "$PWD/f.txt" && cat f.txt && echo ok > "$HOME/.x" && echo ok > "$TMPDIR/y"`); code != 0 || !strings.Contains(out, "hello") {
		t.Fatalf("an allowed command failed (%d): %s", code, out)
	}
	if out, code := run(`cat ` + secret); code == 0 || strings.Contains(out, "TOP-SECRET") {
		t.Errorf("the real home was readable (%d): %s", code, out)
	}
	if out, code := run(`echo x > ` + filepath.Join(real, "pwned")); code == 0 {
		t.Errorf("a write outside the allow list succeeded: %s", out)
	}
	if _, err := os.Stat(filepath.Join(real, "pwned")); err == nil {
		t.Error("the file was created")
	}
	if out, code := run(`ls ` + filepath.Join(real, ".ssh")); code == 0 {
		t.Errorf("the .ssh directory was listable: %s", out)
	}
}

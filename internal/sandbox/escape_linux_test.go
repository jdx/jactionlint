//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The escape suite runs real commands under the real sandbox and checks that what the plan lists fails the way it
// should. It skips (with the reason) on a kernel without Landlock, and needs `python3` for the system-call probes.

type env struct {
	t         *testing.T
	abi       int
	workspace string
	home      string // the fake home
	realHome  string // stands for the real home of the user: not allowed
	secret    string // a file in realHome
	tmp       string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	abi, err := landlockABI()
	if err != nil {
		t.Skipf("sandbox unavailable: %v", err)
	}
	// Not under /tmp: the sandbox shares /tmp with the host on purpose, so a "real home" there would be writable.
	base, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	real, err := os.MkdirTemp(base, ".jactionlint-escape-")
	if err != nil {
		t.Skipf("cannot create a directory under %s: %v", base, err)
	}
	t.Cleanup(func() { os.RemoveAll(real) })
	e := &env{t: t, abi: abi, realHome: real}
	e.workspace = filepath.Join(real, "run", "work")
	e.home = filepath.Join(real, "run", "home")
	e.tmp = filepath.Join(real, "run", "tmp")
	for _, d := range []string{e.workspace, e.home, e.tmp, filepath.Join(real, ".ssh")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.secret = filepath.Join(real, ".ssh", "id_rsa")
	if err := os.WriteFile(e.secret, []byte("TOP-SECRET-KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	return e
}

// run runs a shell script in the sandbox and returns its output and exit status.
func (e *env) run(net Net, ports []uint16, script string, extra ...string) (string, int, error) {
	e.t.Helper()
	pol := DefaultPolicy(Options{
		Workspace: e.workspace, Home: e.home, Temp: e.tmp, ToolCache: filepath.Join(e.realHome, "run", "tools"),
		PathDirs: []string{"/usr/bin", "/bin"}, Net: net, ConnectPorts: ports, ExtraRead: pythonRoots(),
	})
	// the run directory is under the stand-in home, so only the three directories above are writable
	pol.Argv = []string{"sh", "-c", script}
	pol.Env = ScrubEnv(os.Environ(), DefaultPassEnv, map[string]string{"HOME": e.home, "TMPDIR": e.tmp, "PATH": "/usr/bin:/bin", "CI": "true"})
	pol.Env = append(pol.Env, extra...)
	pol.Dir = e.workspace
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code, err := Run(ctx, pol, IO{Stdout: &out, Stderr: &out})
	return out.String(), code, err
}

// pythonRoots lets the sandbox run the python3 of the PATH when it is a version manager's install outside /usr.
func pythonRoots() []string {
	py, err := exec.LookPath("python3")
	if err != nil {
		return nil
	}
	if r, err := filepath.EvalSymlinks(py); err == nil {
		py = r
	}
	return []string{filepath.Dir(filepath.Dir(py))}
}

// pyCmd writes a python script into the workspace and returns the shell command that runs it.
func (e *env) pyCmd(name, src string) string {
	e.t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		e.t.Skip("python3 is needed")
	}
	path := filepath.Join(e.workspace, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		e.t.Fatal(err)
	}
	return py + " -I " + path
}

func (e *env) mustRun(net Net, script string) string {
	e.t.Helper()
	out, code, err := e.run(net, nil, script)
	if err != nil || code != 0 {
		e.t.Fatalf("script failed (code %d, err %v):\n%s\n%s", code, err, script, out)
	}
	return out
}

func (e *env) mustFail(net Net, script, wantInOutput string) {
	e.t.Helper()
	out, code, err := e.run(net, nil, script)
	if err != nil {
		e.t.Fatalf("setup failed: %v", err)
	}
	if code == 0 {
		e.t.Fatalf("expected the script to fail, but it succeeded:\n%s\n%s", script, out)
	}
	if wantInOutput != "" && !strings.Contains(out, wantInOutput) {
		e.t.Fatalf("output lacks %q:\n%s", wantInOutput, out)
	}
}

func TestEscapeAllowed(t *testing.T) {
	e := newEnv(t)
	out := e.mustRun(NetOpen, `
set -e
echo hello > "$PWD/file.txt"
mkdir -p "$PWD/sub/dir" && cp "$PWD/file.txt" "$PWD/sub/dir/copy.txt"
echo ok > "$HOME/.profile"
echo ok > "$TMPDIR/scratch"
ls /usr/bin >/dev/null
echo to-stdout > /dev/stdout
cat /etc/hostname >/dev/null 2>&1 || true
echo done`)
	if !strings.Contains(out, "to-stdout") || !strings.Contains(out, "done") {
		t.Fatalf("output: %q", out)
	}
	if b, _ := os.ReadFile(filepath.Join(e.workspace, "sub/dir/copy.txt")); string(b) != "hello\n" {
		t.Fatalf("workspace write lost: %q", b)
	}
}

func TestEscapeReadRealHome(t *testing.T) {
	e := newEnv(t)
	e.mustFail(NetOpen, fmt.Sprintf("cat %q", e.secret), "ermission denied")
	e.mustFail(NetOpen, fmt.Sprintf("ls %q", filepath.Dir(e.secret)), "ermission denied")
	e.mustFail(NetOpen, fmt.Sprintf("cat < %q", e.secret), "ermission denied")
	// the prompt example of the plan: ~/.ssh/*
	e.mustFail(NetOpen, `cat "$HOME/../../.ssh/id_rsa"`, "")
}

func TestEscapeWriteOutsideWorkspace(t *testing.T) {
	e := newEnv(t)
	for _, target := range []string{
		filepath.Join(e.realHome, "pwned"),
		filepath.Join(e.realHome, ".ssh", "authorized_keys"),
		filepath.Join(e.realHome, "run", "pwned"),
		filepath.Join(e.workspace, "..", "pwned"),
		"/usr/pwned",
	} {
		e.mustFail(NetOpen, fmt.Sprintf("echo x > %q", target), "")
		if _, err := os.Stat(filepath.Clean(target)); err == nil {
			t.Errorf("%s was created", target)
		}
	}
	// deleting and renaming out of the workspace are writes too
	e.mustFail(NetOpen, fmt.Sprintf("rm %q", e.secret), "")
	e.mustFail(NetOpen, fmt.Sprintf("mv %q ./stolen", e.secret), "")
	e.mustFail(NetOpen, fmt.Sprintf("ln %q ./hardlink", e.secret), "")
	if _, err := os.Stat(e.secret); err != nil {
		t.Fatalf("the secret is gone: %v", err)
	}
}

func TestEscapeSymlinkOut(t *testing.T) {
	e := newEnv(t)
	// A link inside the workspace pointing out of it does not grant the target.
	e.mustFail(NetOpen, fmt.Sprintf("ln -s %q leak && cat leak", e.secret), "ermission denied")
	e.mustFail(NetOpen, fmt.Sprintf("ln -s %q leakdir && ls leakdir/", filepath.Dir(e.secret)), "ermission denied")
	e.mustFail(NetOpen, fmt.Sprintf("ln -s %q leakdir2 && echo x > leakdir2/new", filepath.Join(e.realHome)), "ermission denied")
	if _, err := os.Stat(filepath.Join(e.realHome, "new")); err == nil {
		t.Fatal("wrote through a symlink")
	}
	// a link planted by a previous step, pointing at the stolen file through the workspace path
	if err := os.Symlink(e.secret, filepath.Join(e.workspace, "planted")); err != nil {
		t.Fatal(err)
	}
	e.mustFail(NetOpen, "cat planted", "ermission denied")
}

func TestEscapeEnvScrubbed(t *testing.T) {
	e := newEnv(t)
	t.Setenv("AWS_SECRET_ACCESS_KEY", "sekrit")
	t.Setenv("GITHUB_TOKEN", "ghp_sekrit")
	out := e.mustRun(NetOpen, "env | sort")
	for _, bad := range []string{"sekrit", "AWS_", "SSH_AUTH_SOCK", "GITHUB_TOKEN"} {
		if strings.Contains(out, bad) {
			t.Errorf("the step sees %q:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, "HOME="+e.home) || !strings.Contains(out, "CI=true") {
		t.Errorf("fake home or CI missing:\n%s", out)
	}
}

func TestEscapeOtherProcessEnviron(t *testing.T) {
	e := newEnv(t)
	victim := exec.Command("sleep", "60")
	victim.Env = []string{"VICTIM_SECRET=hunter2"}
	if err := victim.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { victim.Process.Kill(); victim.Wait() }()
	pid := victim.Process.Pid
	// control: the test process itself can read it (once the child has exec'd)
	var b []byte
	var err error
	for i := 0; i < 100; i++ {
		if b, err = os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid)); err == nil && strings.Contains(string(b), "hunter2") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil || !strings.Contains(string(b), "hunter2") {
		t.Skipf("cannot read /proc/%d/environ outside the sandbox: %v", pid, err)
	}
	e.mustFail(NetOpen, fmt.Sprintf("cat /proc/%d/environ", pid), "ermission denied")
	e.mustFail(NetOpen, fmt.Sprintf("cat /proc/%d/cmdline", pid), "ermission denied")
	e.mustFail(NetOpen, fmt.Sprintf("cat /proc/%d/mem", pid), "")
	e.mustFail(NetOpen, fmt.Sprintf("cat /proc/%d/environ", os.Getppid()), "ermission denied")
	e.mustFail(NetOpen, "cat /proc/1/environ", "")
	// the step's own /proc entry still works (for the process that the sandbox started)
	e.mustRun(NetOpen, `read first < /proc/$$/status && test -n "$first" && read c < /proc/cpuinfo && read m < /proc/meminfo`)
}

func TestEscapeNoNewPrivs(t *testing.T) {
	e := newEnv(t)
	out := e.mustRun(NetOpen, `while read k v; do if [ "$k" = "NoNewPrivs:" ]; then echo "nnp=$v"; fi; done < /proc/$$/status`)
	if !strings.Contains(out, "nnp=1") {
		t.Fatalf("no_new_privs is not set: %q", out)
	}
	if _, err := exec.LookPath("sudo"); err == nil {
		e.mustFail(NetOpen, "sudo -n true", "")
	}
}

// probe runs a python snippet that prints the errno of a system call and compares it with a run outside.
func (e *env) probe(name, snippet string, want syscall.Errno) {
	e.t.Helper()
	script := "import ctypes,os\nlibc=ctypes.CDLL(None,use_errno=True)\n" + snippet + "\nprint('errno=%d'%ctypes.get_errno())\n"
	cmdline := e.pyCmd("probe-"+name+".py", script)
	ctl, err := exec.Command("sh", "-c", cmdline).CombinedOutput()
	if err != nil {
		e.t.Fatalf("%s control run failed: %v\n%s", name, err, ctl)
	}
	if strings.Contains(string(ctl), "errno="+strconv.Itoa(int(want))+"\n") {
		e.t.Skipf("%s already fails with %v outside the sandbox (the test environment filters it too)", name, want)
	}
	out := e.mustRun(NetOpen, cmdline)
	if !strings.Contains(out, "errno="+strconv.Itoa(int(want))) {
		e.t.Fatalf("%s: want errno %d (%v) in the sandbox, got %q (outside: %q)", name, want, want, out, ctl)
	}
}

func TestEscapeSyscalls(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		name, snippet string
	}{
		{"ptrace", "libc.ptrace(0,0,0,0)"}, // PTRACE_TRACEME
		{"io_uring_setup", "libc.syscall(%IOURING%,0,0)"},
		{"bpf", "libc.syscall(%BPF%,0,0,0)"},
		{"perf_event_open", "libc.syscall(%PERF%,0,0,-1,-1,0)"},
		{"userfaultfd", "libc.syscall(%UFFD%,0)"},
		{"keyctl", "libc.syscall(%KEYCTL%,0,0,0,0,0)"},
		{"unshare", "libc.unshare(0x10000000)"}, // CLONE_NEWUSER
		{"mount", "libc.mount(b'none',b'/mnt',b'tmpfs',0,None)"},
	}
	nums := nums()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e.t = t
			s := c.snippet
			for k, v := range map[string]int{"%IOURING%": nums["io_uring_setup"], "%BPF%": nums["bpf"], "%PERF%": nums["perf_event_open"], "%UFFD%": nums["userfaultfd"], "%KEYCTL%": nums["keyctl"]} {
				s = strings.ReplaceAll(s, k, strconv.Itoa(v))
			}
			e.probe(c.name, s, syscall.EPERM)
		})
	}
}

func TestEscapeNetworkNone(t *testing.T) {
	e := newEnv(t)
	mk := `import socket,sys
for fam,name in ((socket.AF_INET,'inet'),(socket.AF_INET6,'inet6')):
    try:
        socket.socket(fam, socket.SOCK_STREAM).close(); print(name,'ok')
    except OSError as ex:
        print(name,'denied',ex.errno)
s=socket.socket(socket.AF_UNIX, socket.SOCK_STREAM); s.close(); print('unix ok')
`
	script := e.pyCmd("net.py", mk)
	open := e.mustRun(NetOpen, script)
	if !strings.Contains(open, "inet ok") {
		t.Skipf("the test environment forbids internet sockets even without --net=none: %q", open)
	}
	none := e.mustRun(NetNone, script)
	if !strings.Contains(none, "inet denied 13") || !strings.Contains(none, "inet6 denied 13") || !strings.Contains(none, "unix ok") {
		t.Fatalf("--net=none: %q", none)
	}
}

func TestEscapeNetworkProxy(t *testing.T) {
	e := newEnv(t)
	if e.abi < 4 {
		t.Skipf("--net=proxy needs Landlock ABI 4, this kernel has %d", e.abi)
	}
	allowed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback")
	}
	defer allowed.Close()
	blocked, _ := net.Listen("tcp", "127.0.0.1:0")
	defer blocked.Close()
	port := func(l net.Listener) int { return l.Addr().(*net.TCPAddr).Port }
	go func() {
		for {
			c, err := allowed.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	go func() {
		for {
			c, err := blocked.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	script := e.pyCmd("proxy.py", fmt.Sprintf(`import socket
def conn(p):
    s=socket.socket(); s.settimeout(3)
    try:
        s.connect(('127.0.0.1',p)); return 'ok'
    except OSError as e:
        return 'denied %%d' %% e.errno
print('allowed', conn(%d))
print('blocked', conn(%d))
try:
    socket.socket(socket.AF_INET, socket.SOCK_DGRAM); print('udp ok')
except OSError as e:
    print('udp denied', e.errno)
`, port(allowed), port(blocked)))
	out, code, err := e.run(NetProxy, []uint16{uint16(port(allowed))}, script)
	if err != nil || code != 0 {
		t.Fatalf("code %d err %v: %s", code, err, out)
	}
	if !strings.Contains(out, "allowed ok") || !strings.Contains(out, "blocked denied 13") || !strings.Contains(out, "udp denied 13") {
		t.Fatalf("--net=proxy: %q", out)
	}
}

func TestEscapeUnixSocketOutside(t *testing.T) {
	e := newEnv(t)
	if e.abi < 9 {
		t.Skipf("connecting to a unix socket is only restricted from Landlock ABI 9, this kernel has %d", e.abi)
	}
	sock := filepath.Join(e.realHome, "agent.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	e.mustFail(NetOpen, e.pyCmd("conn-out.py", fmt.Sprintf("import socket\ns=socket.socket(socket.AF_UNIX)\ns.connect('%s')\n", sock)), "")
	// inside the workspace it works
	inner := filepath.Join(e.workspace, "own.sock")
	e.mustRun(NetOpen, e.pyCmd("conn-in.py", fmt.Sprintf(`import socket
srv=socket.socket(socket.AF_UNIX);srv.bind('%s');srv.listen(1)
c=socket.socket(socket.AF_UNIX);c.connect('%s')
`, inner, inner)))
}

func TestEscapeSignalOutside(t *testing.T) {
	e := newEnv(t)
	if e.abi < 6 {
		t.Skipf("signal scoping needs Landlock ABI 6, this kernel has %d", e.abi)
	}
	victim := exec.Command("sleep", "60")
	if err := victim.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { victim.Process.Kill(); victim.Wait() }()
	e.mustFail(NetOpen, fmt.Sprintf("kill -9 %d", victim.Process.Pid), "")
	time.Sleep(100 * time.Millisecond)
	if err := victim.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("the step killed a process outside the sandbox: %v", err)
	}
}

func TestRunExitStatusAndFailClosed(t *testing.T) {
	e := newEnv(t)
	if _, code, err := e.run(NetOpen, nil, "exit 7"); err != nil || code != 7 {
		t.Fatalf("code %d err %v", code, err)
	}
	if _, code, err := e.run(NetOpen, nil, "kill -TERM $$"); err != nil || code != 128+int(syscall.SIGTERM) {
		t.Fatalf("signal: code %d err %v", code, err)
	}
	// a program that is not found, or not allowed to run, is a setup error that names the reason
	pol := DefaultPolicy(Options{Workspace: e.workspace, Home: e.home, Temp: e.tmp, Net: NetOpen})
	pol.Env = []string{"PATH=/usr/bin:/bin"}
	pol.Argv = []string{"definitely-not-installed"}
	var se *SetupError
	if _, err := Run(context.Background(), pol, IO{}); !errors.As(err, &se) || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("not found: %v", err)
	}
	outside := filepath.Join(e.realHome, "evil.sh")
	os.WriteFile(outside, []byte("#!/bin/sh\necho pwned\n"), 0o755)
	pol.Argv = []string{outside}
	var out bytes.Buffer
	if _, err := Run(context.Background(), pol, IO{Stdout: &out}); !errors.As(err, &se) || !strings.Contains(err.Error(), "permission denied") || out.Len() != 0 {
		t.Fatalf("outside binary: err %v out %q", err, out.String())
	}
}

func TestRunCancelKillsTree(t *testing.T) {
	e := newEnv(t)
	killGrace = 500 * time.Millisecond
	pidfile := filepath.Join(e.workspace, "child.pid")
	pol := DefaultPolicy(Options{Workspace: e.workspace, Home: e.home, Temp: e.tmp, PathDirs: []string{"/usr/bin", "/bin"}, Net: NetOpen})
	pol.Env = []string{"PATH=/usr/bin:/bin"}
	pol.Dir = e.workspace
	pol.Argv = []string{"sh", "-c", fmt.Sprintf("(trap '' TERM; exec sleep 60) & echo $! > %s; trap '' TERM; wait", pidfile)}
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	start := time.Now()
	code, err := Run(ctx, pol, IO{})
	if err != nil {
		t.Fatal(err)
	}
	if code != 128+int(syscall.SIGKILL) || time.Since(start) > 10*time.Second {
		t.Fatalf("code %d after %v", code, time.Since(start))
	}
	b, _ := os.ReadFile(pidfile)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	time.Sleep(200 * time.Millisecond)
	if pid == 0 {
		t.Fatalf("no child pid: %q", b)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("the child %d survived the cancel", pid)
	}
}

func TestSeccompProgramShape(t *testing.T) {
	for _, n := range []Net{NetOpen, NetNone, NetProxy} {
		prog, err := buildSeccomp(n)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		if len(prog) < 20 || len(prog) > 200 {
			t.Errorf("%s: odd program length %d", n, len(prog))
		}
		// every jump stays inside the program
		for i, ins := range prog {
			if ins.code&0x07 == 0x05 && ins.code != 0x05 { // BPF_JMP conditional
				if i+1+int(ins.jt) >= len(prog) || i+1+int(ins.jf) >= len(prog) {
					t.Errorf("%s: instruction %d jumps outside", n, i)
				}
			}
		}
		// the last instruction returns
		if prog[len(prog)-1].code != bpfRET|bpfK {
			t.Errorf("%s: program does not end with a return", n)
		}
	}
}

// bpfRun interprets the program for a system call like the kernel does and returns the action.
func bpfRun(t *testing.T, prog []sockFilter, arch, nr uint32, args ...uint32) uint32 {
	t.Helper()
	data := make([]uint32, 16) // struct seccomp_data as 32 bit words: nr, arch, ip(2), args[6](2 each)
	data[0], data[1] = nr, arch
	for i, a := range args {
		data[4+2*i] = a
	}
	var acc uint32
	for pc := 0; pc < len(prog); pc++ {
		in := prog[pc]
		switch {
		case in.code == bpfLD|bpfW|bpfABS:
			acc = data[in.k/4]
		case in.code == bpfALU|bpfAND|bpfK:
			acc &= in.k
		case in.code == bpfJMP|bpfJEQ|bpfK:
			if acc == in.k {
				pc += int(in.jt)
			} else {
				pc += int(in.jf)
			}
		case in.code == bpfJMP|bpfJSET|bpfK:
			if acc&in.k != 0 {
				pc += int(in.jt)
			} else {
				pc += int(in.jf)
			}
		case in.code == bpfRET|bpfK:
			return in.k
		default:
			t.Fatalf("unsupported instruction %#x at %d", in.code, pc)
		}
	}
	t.Fatal("program fell off the end")
	return 0
}

// The kernel-level probes above skip the calls that the machine running the tests already forbids, so this checks the
// filter itself: every denied call gets EPERM, ordinary calls pass, and the socket rules follow the network mode.
func TestSeccompProgramDecisions(t *testing.T) {
	const eperm = seccompRetErrno | uint32(syscall.EPERM)
	const eacces = seccompRetErrno | uint32(syscall.EACCES)
	for _, mode := range []Net{NetOpen, NetNone, NetProxy} {
		prog, err := buildSeccomp(mode)
		if err != nil {
			t.Fatal(err)
		}
		for _, nr := range deniedSyscalls {
			if got := bpfRun(t, prog, auditArch, nr); got != eperm {
				t.Errorf("%s: syscall %d: got %#x, want EPERM", mode, nr, got)
			}
		}
		for _, nr := range []uint32{syscall.SYS_READ, syscall.SYS_WRITE, syscall.SYS_OPENAT, syscall.SYS_EXECVE, syscall.SYS_CLONE, syscall.SYS_MMAP, syscall.SYS_CONNECT, syscall.SYS_KILL, syscall.SYS_EXIT_GROUP} {
			if got := bpfRun(t, prog, auditArch, nr); got != seccompRetAllow {
				t.Errorf("%s: syscall %d: got %#x, want allow", mode, nr, got)
			}
		}
		if got := bpfRun(t, prog, auditArch^1, syscall.SYS_READ); got != seccompRetKill {
			t.Errorf("%s: wrong architecture: got %#x, want kill", mode, got)
		}
		if x32Bit != 0 {
			if got := bpfRun(t, prog, auditArch, x32Bit|syscall.SYS_READ); got != eperm {
				t.Errorf("%s: x32 call: got %#x", mode, got)
			}
		}
		sock := func(domain, typ uint32) uint32 { return bpfRun(t, prog, auditArch, syscall.SYS_SOCKET, domain, typ, 0) }
		const stream, dgram, raw, cloexec = 1, 2, 3, 0o2000000
		want := map[string][]uint32{
			// domain, type, expected
			"unix stream":  {syscall.AF_UNIX, stream, seccompRetAllow},
			"unix dgram":   {syscall.AF_UNIX, dgram, seccompRetAllow},
			"netlink":      {syscall.AF_NETLINK, raw, seccompRetAllow},
			"packet":       {syscall.AF_PACKET, raw, eacces},
			"inet stream":  {syscall.AF_INET, stream, map[Net]uint32{NetOpen: seccompRetAllow, NetNone: eacces, NetProxy: seccompRetAllow}[mode]},
			"inet6 stream": {syscall.AF_INET6, stream | cloexec, map[Net]uint32{NetOpen: seccompRetAllow, NetNone: eacces, NetProxy: seccompRetAllow}[mode]},
			"inet dgram":   {syscall.AF_INET, dgram, map[Net]uint32{NetOpen: seccompRetAllow, NetNone: eacces, NetProxy: eacces}[mode]},
			"inet6 dgram":  {syscall.AF_INET6, dgram | cloexec, map[Net]uint32{NetOpen: seccompRetAllow, NetNone: eacces, NetProxy: eacces}[mode]},
			"inet raw":     {syscall.AF_INET, raw, map[Net]uint32{NetOpen: seccompRetAllow, NetNone: eacces, NetProxy: eacces}[mode]},
		}
		for name, w := range want {
			if got := sock(w[0], w[1]); got != w[2] {
				t.Errorf("%s: socket %s: got %#x, want %#x", mode, name, got, w[2])
			}
		}
	}
}

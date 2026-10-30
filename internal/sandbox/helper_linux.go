package sandbox

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

// HelperArg is the first argument of the re-executed binary that applies a policy and runs the command.
const HelperArg = "__sandbox-exec"

// File descriptors that the runner passes to the helper (os/exec numbers ExtraFiles from 3).
const (
	policyFD = 3
	statusFD = 4
)

// exit status of the helper when it could not apply the policy; the reason is written to the status descriptor.
const helperFailed = 125

// The helper runs before anything else of the program: Go calls init before main, and the restrictions must be
// applied by the thread that execs the command, which LockOSThread guarantees. It only acts when the process was
// started by Run (the first argument, and the two descriptors).
func init() {
	if len(os.Args) == 2 && os.Args[1] == HelperArg {
		runtime.LockOSThread()
		helperMain()
	}
}

func helperMain() {
	status := os.NewFile(statusFD, "status")
	unix.CloseOnExec(statusFD) // closing it at exec tells the runner that the command started
	fail := func(format string, a ...any) {
		fmt.Fprintf(status, format, a...)
		os.Exit(helperFailed)
	}
	pol := os.NewFile(policyFD, "policy")
	raw, err := io.ReadAll(io.LimitReader(pol, 4<<20))
	pol.Close()
	if err != nil {
		fail("could not read the sandbox policy: %v", err)
	}
	p, err := ParsePolicy(raw)
	if err != nil {
		fail("%v", err)
	}
	if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(unix.SIGKILL), 0, 0, 0); err != nil {
		fail("prctl(PR_SET_PDEATHSIG): %v", err)
	}
	if p.Dir != "" {
		if err := os.Chdir(p.Dir); err != nil {
			fail("cannot enter the working directory: %v", err)
		}
	}
	path, err := lookPath(p.Argv[0], p.Env)
	if err != nil {
		fail("%v", err)
	}

	abi, err := landlockABI()
	if err != nil {
		fail("%v", err)
	}
	// The process can read its own /proc entry; entries of other processes (their environ) stay closed.
	p.Read = append(p.Read, fmt.Sprintf("/proc/%d", os.Getpid()))
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		fail("prctl(PR_SET_NO_NEW_PRIVS): %v", err)
	}
	if err := applyLandlock(p, abi); err != nil {
		fail("%v", err)
	}
	if err := applySeccomp(p.Net); err != nil {
		fail("%v", err)
	}
	if err := unix.Exec(path, p.Argv, p.Env); err != nil {
		if err == unix.EACCES {
			fail("cannot execute %s: permission denied (the sandbox only lets a step run programs from the system directories, its PATH and the workspace)", path)
		}
		fail("cannot execute %s: %v", path, err)
	}
}

// lookPath finds the program in the PATH of env (not of this process).
func lookPath(name string, env []string) (string, error) {
	if strings.ContainsRune(name, '/') {
		return name, nil
	}
	pathVar := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			pathVar = v
		}
	}
	for _, dir := range strings.Split(pathVar, ":") {
		if dir == "" || !strings.HasPrefix(dir, "/") {
			continue
		}
		c := dir + "/" + name
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s: command not found in PATH", name)
}

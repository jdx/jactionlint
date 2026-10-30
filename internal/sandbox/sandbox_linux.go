package sandbox

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Available reports whether this machine can apply the sandbox for the network mode, and if not, why.
func Available(net Net) error {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return fmt.Errorf("the sandbox supports Linux on amd64 and arm64 only, not %s", runtime.GOARCH)
	}
	abi, err := landlockABI()
	if err != nil {
		return err
	}
	if net == NetProxy && abi < 4 {
		return fmt.Errorf("--net=proxy needs Landlock ABI 4 (Linux 6.7 or newer); this kernel has ABI %d. Use --net=open or --net=none", abi)
	}
	return nil
}

// Describe says which restrictions are in force on this machine, for diagnostics.
func Describe() string {
	abi, err := landlockABI()
	if err != nil {
		return "Linux: " + err.Error()
	}
	var notes []string
	if abi < 4 {
		notes = append(notes, "no --net=proxy (needs ABI 4)")
	}
	if abi < 9 {
		notes = append(notes, "connecting to unix sockets in allowed directories is not restricted (needs ABI 9)")
	}
	if abi < 6 {
		notes = append(notes, "signals to and abstract unix sockets of other processes are not restricted (needs ABI 6)")
	}
	s := fmt.Sprintf("Linux: Landlock ABI %d, seccomp-bpf", abi)
	if len(notes) > 0 {
		s += "; " + strings.Join(notes, "; ")
	}
	return s
}

func prepare(p *Policy) (*launch, error) {
	if err := Available(p.Net); err != nil {
		return nil, err
	}
	body, err := p.Marshal()
	if err != nil {
		return nil, err
	}
	polR, polW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stR, stW, err := os.Pipe()
	if err != nil {
		polR.Close()
		polW.Close()
		return nil, err
	}
	go func() {
		polW.Write(body)
		polW.Close()
	}()
	cmd := exec.Command("/proc/self/exe", HelperArg)
	cmd.Env = []string{} // the helper needs none; the command gets p.Env from the policy
	cmd.ExtraFiles = []*os.File{polR, stW}
	return &launch{
		cmd:        cmd,
		afterStart: func() { polR.Close(); stW.Close() },
		status: func() string {
			b, _ := io.ReadAll(io.LimitReader(stR, 64<<10))
			return strings.TrimSpace(string(b))
		},
		cleanup: func() { polR.Close(); polW.Close(); stR.Close(); stW.Close() },
	}, nil
}

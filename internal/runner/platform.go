package runner

import (
	"context"
	"fmt"
	"runtime"
	"strings"
)

// platform is where a job's programs run: this machine, or the operating system and architecture of its container. It
// uses Go's names (linux, amd64), as docker --platform and docker image inspect do.
type platform struct {
	os, arch string
}

func hostPlatform() platform { return platform{runtime.GOOS, runtime.GOARCH} }

// parsePlatform reads os/arch[/variant] (linux/amd64, linux/arm64/v8, linux/arm/v7), with the architecture aliases
// that docker accepts (x86_64, aarch64). The variant is dropped: Node.js and the runner context only tell
// architectures apart.
func parsePlatform(s string) (platform, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] == "" {
		return platform{}, fmt.Errorf("the platform %q is not os/arch[/variant], such as linux/amd64", s)
	}
	arch := parts[1]
	if a, ok := archAliases[arch]; ok {
		arch = a
	}
	return platform{parts[0], arch}, nil
}

// archAliases are the other names of architectures that docker normalizes (containerd's platforms package).
var archAliases = map[string]string{
	"x86_64": "amd64", "x86-64": "amd64", "aarch64": "arm64", "armhf": "arm", "armel": "arm", "i386": "386",
}

// String is the name of the platform's tool cache, as the host's is named: linux-amd64.
func (p platform) String() string { return p.os + "-" + p.arch }

// runnerOS is runner.os and RUNNER_OS: Linux, macOS, Windows.
func (p platform) runnerOS() string {
	if s := map[string]string{"linux": "Linux", "darwin": "macOS", "windows": "Windows"}[p.os]; s != "" {
		return s
	}
	return p.os
}

// runnerArch is runner.arch and RUNNER_ARCH: X64, ARM64, X86, ARM.
func (p platform) runnerArch() string {
	if s := map[string]string{"amd64": "X64", "arm64": "ARM64", "386": "X86", "arm": "ARM"}[p.arch]; s != "" {
		return s
	}
	return strings.ToUpper(p.arch)
}

// nodeOS and nodeArch are the names in the file names of nodejs.org (node-v24.0.0-linux-x64.tar.gz).
func (p platform) nodeOS() string { return p.os }

func (p platform) nodeArch() string {
	switch p.arch {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	case "arm":
		return "armv7l"
	}
	return p.arch
}

// optionPlatform is the value of the last --platform in container options, as docker run reads it, or "". The value
// of another option, such as --label --platform=x, is skipped: every docker run option takes a value except the
// boolean ones in runBoolFlags.
func optionPlatform(opts []string) string {
	p := ""
	for i := 0; i < len(opts); i++ {
		w := opts[i]
		if !strings.HasPrefix(w, "-") || w == "-" || w == "--" {
			continue
		}
		if v, ok := strings.CutPrefix(w, "--platform="); ok {
			p = v
			continue
		}
		if strings.Contains(w, "=") || runBoolFlags[w] {
			continue
		}
		if !strings.HasPrefix(w, "--") && len(w) > 2 {
			// short flags put together (-it) or a short flag glued to its value (-eFOO=1): no separate value follows
			continue
		}
		if i+1 < len(opts) {
			if w == "--platform" {
				p = opts[i+1]
			}
			i++ // the value of this option
		}
	}
	return p
}

// runBoolFlags are the options of docker run and podman run that take no value.
var runBoolFlags = map[string]bool{
	"--detach": true, "-d": true, "--init": true, "--interactive": true, "-i": true, "--tty": true, "-t": true,
	"--privileged": true, "--publish-all": true, "-P": true, "--read-only": true, "--rm": true, "--sig-proxy": true,
	"--no-healthcheck": true, "--oom-kill-disable": true, "--disable-content-trust": true, "--quiet": true, "-q": true,
	"--help": true, "--replace": true, "--http-proxy": true, "--read-only-tmpfs": true, "--rmi": true,
	"--env-host": true, "--no-hosts": true, "--systemd": true,
}

// imagePlatform returns the platform of a local image.
func (c *ctrEngine) imagePlatform(ctx context.Context, image string) (platform, error) {
	out, err := c.output(ctx, "image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", image)
	if err != nil {
		return platform{}, err
	}
	return parsePlatform(out)
}

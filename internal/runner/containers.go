package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mattn/go-shellwords"
)

// ctrEngine is the container engine (the docker or podman command line). It is used only for a job or a step that asks
// for a container, or for every Linux job under --container.
type ctrEngine struct {
	bin  string
	kind string // docker or podman
}

// detectEngine finds the engine: the one named by --container-engine, otherwise docker, otherwise podman.
func detectEngine(pref string) (*ctrEngine, error) {
	names := []string{"docker", "podman"}
	if pref != "" {
		names = []string{pref}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			kind := "docker"
			if strings.Contains(filepath.Base(p), "podman") {
				kind = "podman"
			}
			return &ctrEngine{bin: p, kind: kind}, nil
		}
	}
	if pref != "" {
		return nil, fmt.Errorf("the container engine %q is not installed", pref)
	}
	return nil, errors.New("no container engine found (docker or podman)")
}

func (c *ctrEngine) command(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, c.bin, args...)
}

// output runs the engine and returns its standard output; an error carries the standard error.
func (c *ctrEngine) output(ctx context.Context, args ...string) (string, error) {
	cmd := c.command(ctx, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("%s %s: %s", filepath.Base(c.bin), args[0], msg)
	}
	return out.String(), nil
}

// available reports whether the engine answers (the daemon runs).
func (c *ctrEngine) available(ctx context.Context) error {
	_, err := c.output(ctx, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return fmt.Errorf("the container engine does not answer: %w", err)
	}
	return nil
}

// Images that --container uses for the labels of GitHub's Linux runners. They are the multi-architecture indexes of
// the official images, pinned by digest (read from the registry on 2026-10-30).
var defaultImages = map[string]string{
	"ubuntu-latest":    "ubuntu:24.04@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852",
	"ubuntu-24.04":     "ubuntu:24.04@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852",
	"ubuntu-24.04-arm": "ubuntu:24.04@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852",
	"ubuntu-22.04":     "ubuntu:22.04@sha256:076e7bf82319619f28dd0aa9c02ab34b055ad37100395b4f49a2ffba04fb5e11",
	"ubuntu-22.04-arm": "ubuntu:22.04@sha256:076e7bf82319619f28dd0aa9c02ab34b055ad37100395b4f49a2ffba04fb5e11",
}

// imageForLabels picks the image of a job under --container: --container=IMAGE for everything, --image LABEL=IMAGE per
// label, otherwise the default image of the label.
func imageForLabels(labels []string, override string, images map[string]string) (string, error) {
	if override != "" {
		return override, nil
	}
	for _, l := range labels {
		if img, ok := images[strings.ToLower(l)]; ok {
			return img, nil
		}
	}
	for _, l := range labels {
		if img, ok := defaultImages[strings.ToLower(l)]; ok {
			return img, nil
		}
	}
	return "", fmt.Errorf("there is no default image for runs-on %s; pass --image %s=IMAGE or --container=IMAGE", strings.Join(labels, ", "), strings.Join(labels, ","))
}

// pinned reports whether an image reference names a digest.
func pinned(image string) bool { return strings.Contains(image, "@sha256:") }

// checkImageTrust refuses an image that is not pinned by digest, unless the user trusts images.
func checkImageTrust(image string, trust bool) error {
	if pinned(image) || trust {
		return nil
	}
	return fmt.Errorf("the image %s is not pinned by digest. Pulling an image runs someone else's code, so jactionlint run only does it for name@sha256:… references; pass --trust-images (or --trust-actions) to pull %s anyway%s", image, image, explainRef("unpinned-image"))
}

// pull makes the image available locally. A digest-pinned image that is already there is not pulled again.
func (c *ctrEngine) pull(ctx context.Context, image, platform string, say func(string)) error {
	if pinned(image) {
		if _, err := c.output(ctx, "image", "inspect", image); err == nil {
			return nil
		}
	}
	say("pulling " + image)
	args := []string{"pull", "-q"}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	_, err := c.output(ctx, append(args, image)...)
	return err
}

// imageEnv returns the environment of the image (PATH and the like).
func (c *ctrEngine) imageEnv(ctx context.Context, image string) []string {
	out, err := c.output(ctx, "image", "inspect", "--format", "{{json .Config.Env}}", image)
	if err != nil {
		return nil
	}
	var env []string
	json.Unmarshal([]byte(strings.TrimSpace(out)), &env)
	return env
}

// containerOption describes a docker run option that a workflow's `options:` may use.
type containerOption struct {
	hasValue bool
}

// allowedOptions are the options passed through. They limit resources, set health checks and users; none widens what the
// container can reach.
var allowedOptions = map[string]containerOption{
	"--init": {}, "--read-only": {}, "--tty": {}, "-t": {}, "--interactive": {}, "-i": {}, "--no-healthcheck": {},
	"--user": {true}, "-u": {true}, "--memory": {true}, "-m": {true}, "--memory-swap": {true}, "--memory-reservation": {true},
	"--cpus": {true}, "--cpu-shares": {true}, "--cpuset-cpus": {true}, "--shm-size": {true}, "--hostname": {true},
	"--ulimit": {true}, "--stop-signal": {true}, "--stop-timeout": {true}, "--pids-limit": {true}, "--label": {true}, "-l": {true},
	"--health-cmd": {true}, "--health-interval": {true}, "--health-timeout": {true}, "--health-retries": {true},
	"--health-start-period": {true}, "--health-start-interval": {true},
	"--env": {true}, "-e": {true}, "--dns": {true}, "--add-host": {true}, "--entrypoint": {true},
}

// refusedOptions are named so that the message can say why.
var refusedOptions = map[string]string{
	"--privileged":    "it gives the container the capabilities of root on this machine",
	"--cap-add":       "it adds Linux capabilities",
	"--device":        "it gives the container a device of this machine",
	"--network":       "the job network is set by jactionlint",
	"--net":           "the job network is set by jactionlint",
	"--pid":           "it shares the process namespace of this machine",
	"--ipc":           "it shares the IPC namespace of this machine",
	"--uts":           "it shares the host name namespace of this machine",
	"--userns":        "it changes the user namespace",
	"--security-opt":  "it changes the security profile",
	"--sysctl":        "it changes kernel parameters",
	"--volume":        "it mounts a path of this machine",
	"-v":              "it mounts a path of this machine",
	"--mount":         "it mounts a path of this machine",
	"--volumes-from":  "it mounts the volumes of another container",
	"--cgroup-parent": "it changes the cgroup",
	"--publish":       "use the ports: key to publish ports",
	"-p":              "use the ports: key to publish ports",
	"--group-add":     "it adds groups",
	"--name":          "the container name is set by jactionlint",
	"--pull":          "images are pulled by jactionlint under its pinning rule",
	"--runtime":       "it changes the container runtime",
}

// parseOptions turns the `options:` string of a container into arguments of docker run, refusing what could widen the
// access of the container (unless unsafe is set).
func parseOptions(opts string, unsafe bool) ([]string, error) {
	if strings.TrimSpace(opts) == "" {
		return nil, nil
	}
	words, err := shellwords.Parse(opts)
	if err != nil {
		return nil, fmt.Errorf("options %q: %v", opts, err)
	}
	if unsafe {
		return words, nil
	}
	var out []string
	for i := 0; i < len(words); i++ {
		w := words[i]
		name, _, hasEq := strings.Cut(w, "=")
		// short options glued to their value, such as -m512m or -v/a:/b
		short := ""
		if len(w) > 2 && w[0] == '-' && w[1] != '-' {
			short = w[:2]
		}
		if why, bad := refusedOptions[name]; bad {
			return nil, fmt.Errorf("the container option %s is refused: %s (--unsafe-container-options allows it)%s", name, why, explainRef("container-option-refused"))
		}
		if why, bad := refusedOptions[short]; bad && short != "" {
			return nil, fmt.Errorf("the container option %s is refused: %s (--unsafe-container-options allows it)%s", short, why, explainRef("container-option-refused"))
		}
		opt, ok := allowedOptions[name]
		if !ok && short != "" {
			if o2, ok2 := allowedOptions[short]; ok2 && o2.hasValue {
				out = append(out, w)
				continue
			}
		}
		if !ok {
			return nil, fmt.Errorf("the container option %s is not on the list of options that jactionlint passes through (--unsafe-container-options allows any option)", name)
		}
		out = append(out, w)
		if opt.hasValue && !hasEq {
			if i+1 >= len(words) {
				return nil, fmt.Errorf("the container option %s needs a value", name)
			}
			i++
			out = append(out, words[i])
		}
	}
	return out, nil
}

// hardening are the arguments every container of jactionlint gets: no capability beyond what a root userland needs, no new
// privileges, no privileged mode, no host network, no Docker socket, only the mounts jactionlint adds.
func hardening() []string {
	args := []string{"--cap-drop=ALL", "--security-opt", "no-new-privileges"}
	for _, c := range []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL", "SETGID", "SETUID", "SETPCAP", "NET_BIND_SERVICE"} {
		args = append(args, "--cap-add", c)
	}
	return args
}

// remapEnv rewrites the paths of this machine in an environment to the paths inside a container.
func remapEnv(env []string, mounts [][2]string) []string {
	out := make([]string, len(env))
	for i, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			best := -1
			for j, m := range mounts {
				if (v == m[0] || strings.HasPrefix(v, m[0]+"/")) && (best < 0 || len(m[0]) > len(mounts[best][0])) {
					best = j
				}
			}
			if best >= 0 {
				v = mounts[best][1] + strings.TrimPrefix(v, mounts[best][0])
			}
			kv = k + "=" + v
		}
		out[i] = kv
	}
	return out
}

// envArgs passes an environment to docker run or exec without putting values in the process list: single-line values go in
// an env file, values with line breaks (which env files cannot hold) are named with -e and travel in the environment of
// the client. It returns the arguments and the extra environment for the client.
func envArgs(dir, name string, env []string) (args []string, clientEnv []string, err error) {
	var b strings.Builder
	for _, kv := range env {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		if strings.ContainsAny(kv, "\n\r") {
			args = append(args, "-e", k)
			clientEnv = append(clientEnv, kv)
			continue
		}
		b.WriteString(kv + "\n")
	}
	if b.Len() > 0 {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
			return nil, nil, err
		}
		args = append([]string{"--env-file", p}, args...)
	}
	return args, clientEnv, nil
}

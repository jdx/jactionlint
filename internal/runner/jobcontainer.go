package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/eval"
	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

// jobContainer is the container a job runs its steps in (container: of the job, or --container) with the services
// next to it. The container is its own boundary: steps inside it get the hardening of the container, not the Landlock
// sandbox of the host.
type jobContainer struct {
	eng      *ctrEngine
	name     string
	network  string
	image    string
	imageEnv []string
	services []*serviceContainer
	root     bool // --container: a root userland with a sudo that runs the command
	// inContainer is false for a job that runs on this machine and only has services next to it.
	inContainer bool
	files       string
	mounts      [][2]string
}

type serviceContainer struct {
	id    string
	name  string
	image string
	ports map[string]string // container port -> port on this machine
}

func (e *Engine) ctrSay(jr *jobRun) func(string) {
	return func(s string) { jr.pr.Note(1, "%s", s) }
}

// startContainer creates the network, the services and the job container.
func (jr *jobRun) startContainer(ctx context.Context, image string, spec *jactionlint.Container, rootMode bool) (*jobContainer, error) {
	e := jr.e
	cfg := e.cfg
	eng, err := e.containerEngine(ctx)
	if err != nil {
		return nil, err
	}
	say := e.ctrSay(jr)
	seq := e.root().nextContainerSeq()
	prefix := fmt.Sprintf("jactionlint-%d-%d", os.Getpid(), seq)
	jc := &jobContainer{eng: eng, name: prefix + "-job", network: prefix + "-net", image: image, root: rootMode, files: jr.rd.Files, inContainer: image != ""}

	if image != "" {
		if err := checkImageTrust(image, cfg.TrustImages || cfg.TrustActions); err != nil {
			return nil, err
		}
		if err := eng.pull(ctx, image, cfg.Platform, say); err != nil {
			return nil, err
		}
		jc.imageEnv = eng.imageEnv(ctx, image)
	}
	if _, err := eng.output(ctx, "network", "create", "--label", "jactionlint=1", jc.network); err != nil {
		return nil, err
	}

	// services
	if jr.pj.Job.Services != nil {
		ids := make([]string, 0, len(jr.pj.Job.Services.Value))
		for id := range jr.pj.Job.Services.Value {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			svc := jr.pj.Job.Services.Value[id]
			sc, err := jr.startService(ctx, jc, id, svc.Container, prefix)
			if err != nil {
				jc.stop(context.WithoutCancel(ctx))
				return nil, fmt.Errorf("service %s: %w", id, err)
			}
			jc.services = append(jc.services, sc)
		}
	}

	if image == "" {
		return jc, nil
	}
	// the job container: the directories of the job are mounted at the same path as on this machine
	mounts := [][2]string{}
	vol := func(host string, ro bool) []string {
		m := host + ":" + host
		if ro {
			m += ":ro"
		}
		mounts = append(mounts, [2]string{host, host})
		return []string{"-v", m}
	}
	args := []string{"run", "-d", "--init", "--name", jc.name, "--network", jc.network, "--label", "jactionlint=1"}
	if cfg.Platform != "" {
		args = append(args, "--platform", cfg.Platform)
	}
	args = append(args, hardening()...)
	args = append(args, vol(jr.work, false)...)
	args = append(args, vol(jr.rd.Home, false)...)
	args = append(args, vol(jr.rd.Temp, false)...)
	args = append(args, vol(jr.rd.Files, false)...)
	args = append(args, vol(jr.rd.ToolCache, false)...)
	if dirs := jr.actionDirs(); len(dirs) > 0 {
		if err := os.MkdirAll(dirs[0], 0o755); err == nil {
			args = append(args, vol(dirs[0], true)...)
		}
	}
	if rootMode {
		// sudo works as it does on a hosted runner, because the job is root in its own container
		shim := filepath.Join(jr.rd.Root, "sudo")
		if err := os.WriteFile(shim, []byte("#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in -n|-E|-S|-H|-k|-A|--non-interactive|--preserve-env) shift ;; -u) shift 2 ;; --) shift; break ;; -*) shift ;; *) break ;; esac; done\nexec \"$@\"\n"), 0o755); err == nil {
			args = append(args, "-v", shim+":/usr/local/bin/sudo:ro")
		}
	}
	if spec != nil {
		// the container's own env and options
		var envs []string
		if spec.Env != nil {
			m := map[string]string{}
			if err := jr.addEnv(m, spec.Env, jr.env); err != nil {
				return nil, err
			}
			for k, v := range m {
				envs = append(envs, k+"="+v)
			}
		}
		ea, cenv, err := envArgs(jr.rd.Files, "container.env", envs)
		if err != nil {
			return nil, err
		}
		if len(cenv) > 0 {
			return nil, fmt.Errorf("container env values with line breaks are not supported")
		}
		args = append(args, ea...)
		if spec.Options != nil {
			o, err := jr.interp(jr.env, spec.Options.Value)
			if err != nil {
				return nil, err
			}
			opts, err := parseOptions(o, cfg.UnsafeContainerOptions)
			if err != nil {
				return nil, err
			}
			args = append(args, opts...)
		}
		for _, p := range spec.Ports {
			args = append(args, "-p", "127.0.0.1:"+p.Value)
		}
	}
	args = append(args, "-w", jr.work, "--entrypoint", "tail", image, "-f", "/dev/null")
	if _, err := eng.output(ctx, args...); err != nil {
		jc.stop(context.WithoutCancel(ctx))
		return nil, err
	}
	jc.mounts = mounts
	// A remote engine, or one in another mount namespace, cannot see the directories that were just mounted; say so now
	// instead of failing in the first step with "no such file".
	marker := filepath.Join(jr.rd.Temp, ".jactionlint-mount-check")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err == nil {
		if _, err := eng.output(ctx, "exec", jc.name, "test", "-f", marker); err != nil {
			jc.stop(context.WithoutCancel(ctx))
			return nil, fmt.Errorf("the container cannot see the directories of this run (%s): the engine runs on another machine or in another file system namespace, so jactionlint cannot mount the workspace into it%s", jr.rd.Root, explainRef("container-mount-invisible"))
		}
	}
	return jc, nil
}

var hostPortFlag = "127.0.0.1:"

// startService starts one entry of services: and waits until it is healthy when it has a health check.
func (jr *jobRun) startService(ctx context.Context, jc *jobContainer, id string, spec *jactionlint.Container, prefix string) (*serviceContainer, error) {
	cfg := jr.e.cfg
	if spec == nil || spec.Image == nil {
		return nil, fmt.Errorf("it has no image")
	}
	image, err := jr.interp(jr.env, spec.Image.Value)
	if err != nil {
		return nil, err
	}
	if err := checkImageTrust(image, cfg.TrustImages || cfg.TrustActions); err != nil {
		return nil, err
	}
	say := jr.e.ctrSay(jr)
	if err := jc.eng.pull(ctx, image, cfg.Platform, say); err != nil {
		return nil, err
	}
	sc := &serviceContainer{id: id, name: prefix + "-svc-" + sanitizeName(id), image: image, ports: map[string]string{}}
	args := []string{"run", "-d", "--init", "--name", sc.name, "--network", jc.network, "--network-alias", id, "--label", "jactionlint=1"}
	if cfg.Platform != "" {
		args = append(args, "--platform", cfg.Platform)
	}
	args = append(args, hardening()...)
	var envs []string
	if spec.Env != nil {
		m := map[string]string{}
		if err := jr.addEnv(m, spec.Env, jr.env); err != nil {
			return nil, err
		}
		for k, v := range m {
			envs = append(envs, k+"="+v)
		}
	}
	ea, cenv, err := envArgs(jr.rd.Files, "service-"+sanitizeName(id)+".env", envs)
	if err != nil {
		return nil, err
	}
	if len(cenv) > 0 {
		return nil, fmt.Errorf("env values with line breaks are not supported")
	}
	args = append(args, ea...)
	for _, p := range spec.Ports {
		pv, err := jr.interp(jr.env, p.Value)
		if err != nil {
			return nil, err
		}
		if strings.Contains(pv, ":") {
			args = append(args, "-p", hostPortFlag+pv)
		} else {
			args = append(args, "-p", hostPortFlag+":"+pv)
		}
	}
	if spec.Options != nil {
		o, err := jr.interp(jr.env, spec.Options.Value)
		if err != nil {
			return nil, err
		}
		opts, err := parseOptions(o, cfg.UnsafeContainerOptions)
		if err != nil {
			return nil, err
		}
		args = append(args, opts...)
	}
	if spec.Entrypoint != nil {
		args = append(args, "--entrypoint", spec.Entrypoint.Value)
	}
	args = append(args, image)
	if spec.Command != nil {
		w, err := parseOptions("", false)
		_ = w
		_ = err
		args = append(args, strings.Fields(spec.Command.Value)...)
	}
	if _, err := jc.eng.output(ctx, args...); err != nil {
		return nil, err
	}
	// the ports on this machine
	out, _ := jc.eng.output(ctx, "inspect", "--format", "{{json .NetworkSettings.Ports}}", sc.name)
	for cport, hp := range parsePorts(out) {
		sc.ports[cport] = hp
	}
	// wait for the health check, if the image or the options define one
	has, _ := jc.eng.output(ctx, "inspect", "--format", "{{if .State.Health}}yes{{end}}", sc.name)
	if strings.TrimSpace(has) == "yes" {
		deadline := time.Now().Add(5 * time.Minute)
		for {
			st, _ := jc.eng.output(ctx, "inspect", "--format", "{{.State.Health.Status}}", sc.name)
			switch strings.TrimSpace(st) {
			case "healthy":
				return sc, nil
			case "unhealthy":
				return nil, fmt.Errorf("the container became unhealthy")
			}
			if time.Now().After(deadline) || ctx.Err() != nil {
				return nil, fmt.Errorf("it was not healthy after 5 minutes")
			}
			time.Sleep(time.Second)
		}
	}
	return sc, nil
}

// parsePorts reads `{"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"49153"}]}`.
func parsePorts(js string) map[string]string {
	out := map[string]string{}
	var m map[string][]struct{ HostIp, HostPort string }
	if err := jsonUnmarshal(js, &m); err != nil {
		return out
	}
	for k, v := range m {
		if len(v) > 0 {
			out[strings.TrimSuffix(strings.TrimSuffix(k, "/tcp"), "/udp")] = v[0].HostPort
		}
	}
	return out
}

// stop removes the containers and the network, and gives the files back to the user first.
func (jc *jobContainer) stop(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	// files made by root in the mounted directories would otherwise stay owned by root
	if jc.inContainer {
		uidgid := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
		var dirs []string
		for _, m := range jc.mounts {
			dirs = append(dirs, m[0])
		}
		if len(dirs) > 0 && os.Getuid() != 0 {
			jc.eng.output(ctx, append([]string{"exec", jc.name, "chown", "-R", uidgid}, dirs...)...)
		}
	}
	for _, s := range jc.services {
		jc.eng.output(ctx, "rm", "-f", "-v", s.name)
	}
	if jc.inContainer {
		jc.eng.output(ctx, "rm", "-f", "-v", jc.name)
	}
	jc.eng.output(ctx, "network", "rm", jc.network)
}

// jobContext returns the job.container and job.services contexts.
func (jc *jobContainer) jobContext(status string) map[string]any {
	svcs := map[string]any{}
	for _, s := range jc.services {
		ports := map[string]any{}
		for c, h := range s.ports {
			ports[c] = h
		}
		svcs[s.id] = map[string]any{"id": s.name, "network": jc.network, "ports": ports}
	}
	return map[string]any{
		"status":    status,
		"container": map[string]any{"id": jc.name, "network": jc.network},
		"services":  svcs,
	}
}

// execPolicy turns the policy of a step into the engine command that runs it in the job container.
func (jc *jobContainer) execPolicy(pol *sandbox.Policy, dir string) (*sandbox.Policy, error) {
	envArgsList, clientEnv, err := envArgs(jc.files, "exec-"+strconv.FormatInt(time.Now().UnixNano(), 36)+".env", pol.Env)
	if err != nil {
		return nil, err
	}
	argv := []string{jc.eng.bin, "exec", "-i", "-w", dir}
	argv = append(argv, envArgsList...)
	argv = append(argv, jc.name)
	argv = append(argv, pol.Argv...)
	return &sandbox.Policy{Version: sandbox.PolicyVersion, Net: sandbox.NetOpen, Argv: argv, Env: append(os.Environ(), clientEnv...), Dir: ""}, nil
}

var _ = eval.ToString

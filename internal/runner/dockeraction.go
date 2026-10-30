package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/eval"
	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

// The paths that a Docker action sees, as on a hosted runner.
const (
	ctrWorkspace = "/github/workspace"
	ctrHome      = "/github/home"
	ctrTemp      = "/github/runner_temp"
	ctrFiles     = "/github/file_commands"
	ctrEvent     = "/github/workflow/event.json"
	ctrTools     = "/opt/hostedtoolcache"
)

// runDockerAction runs `runs.using: docker` and `uses: docker://image`. skip is set (and nothing ran) when the engine is
// not available.
func (jr *jobRun) runDockerAction(n label, step *jactionlint.Step, ra *resolvedAction, meta *actionMeta, inputs map[string]string, id string, ev *eval.Env, emit func(string), say func(string), ctx context.Context) (res stepResult, eff *effects, skip string) {
	e := jr.e
	cfg := e.cfg
	eff = newEffects()
	failf := func(format string, a ...any) (stepResult, *effects, string) {
		msg := fmt.Sprintf(format, a...)
		emit("error: " + msg)
		return stepResult{Outcome: "failure", Note: msg}, eff, ""
	}
	if cfg.NoContainers {
		return stepResult{}, nil, "it is a Docker action and --no-containers is set"
	}
	eng, err := e.containerEngine(ctx)
	if err != nil {
		return stepResult{}, nil, "it is a Docker action: " + err.Error()
	}

	// the image: a registry image, or a Dockerfile built here
	var image string
	switch {
	case strings.HasPrefix(meta.Image, "docker://"):
		image = strings.TrimPrefix(meta.Image, "docker://")
		if err := checkImageTrust(image, cfg.TrustImages || cfg.TrustActions); err != nil {
			return failf("%v", err)
		}
		if err := eng.pull(ctx, image, cfg.Platform, say); err != nil {
			return failf("%v", err)
		}
	case meta.Image != "":
		dockerfile := filepath.Join(ra.dir, filepath.FromSlash(meta.Image))
		if !inDir(ra.dir, dockerfile) {
			return failf("the Dockerfile %s is outside the action", meta.Image)
		}
		sum := sha256.Sum256([]byte(dockerfile + "|" + ra.sha + "|" + cfg.Platform))
		image = "jactionlint-action:" + hex.EncodeToString(sum[:8])
		say("building the image of " + ra.spec)
		args := []string{"build", "-q", "-t", image, "-f", dockerfile}
		if cfg.Platform != "" {
			args = append(args, "--platform", cfg.Platform)
		}
		if _, err := eng.output(ctx, append(args, ra.dir)...); err != nil {
			return failf("%v", err)
		}
	default:
		return failf("the action has no runs.image")
	}

	pr, err := jr.prepareEnv(n, step, ev, jr.actionEnv())
	if err != nil {
		return failf("%v", err)
	}
	// inputs and the env of action.yml
	aenv := inputEnv(inputs)
	ienv := ev
	if len(meta.Env) > 0 || len(meta.Args) > 0 {
		c := &eval.Env{Contexts: map[string]any{}, Status: ev.Status}
		for k, v := range ev.Contexts {
			c.Contexts[k] = v
		}
		in := map[string]any{}
		for k, v := range inputs {
			in[k] = v
		}
		c.Contexts["inputs"] = in
		ienv = c
	}
	for k, v := range meta.Env {
		s, err := ienv.Interpolate(v)
		if err != nil {
			return failf("%v", err)
		}
		aenv[k] = s
	}
	var args []string
	for _, a := range meta.Args {
		s, err := ienv.Interpolate(a)
		if err != nil {
			return failf("%v", err)
		}
		args = append(args, s)
	}

	// paths of this machine become the paths of a hosted runner inside the container
	filesDir := pr.files.dir
	mounts := [][2]string{
		{pr.files.event, ctrEvent}, {filesDir, ctrFiles}, {jr.work, ctrWorkspace}, {jr.rd.Home, ctrHome},
		{jr.rd.Temp, ctrTemp}, {jr.rd.ToolCache, ctrTools},
	}
	env := remapEnv(pr.env, mounts)
	var kept []string
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") { // the image has its own
			continue
		}
		kept = append(kept, kv)
	}
	for k, v := range aenv {
		kept = append(kept, k+"="+v)
	}
	ea, cenv, err := envArgs(jr.rd.Files, "docker-action-"+strings.ReplaceAll(string(n), " ", "-")+".env", kept)
	if err != nil {
		return failf("%v", err)
	}

	run := []string{"run", "--rm", "--init", "--label", "jactionlint=1", "--workdir", ctrWorkspace}
	if jr.ctr != nil {
		run = append(run, "--network", jr.ctr.network)
	}
	if cfg.Platform != "" {
		run = append(run, "--platform", cfg.Platform)
	}
	run = append(run, hardening()...)
	run = append(run,
		"-v", jr.work+":"+ctrWorkspace, "-v", jr.rd.Home+":"+ctrHome, "-v", jr.rd.Temp+":"+ctrTemp,
		"-v", filesDir+":"+ctrFiles, "-v", pr.files.event+":"+ctrEvent+":ro", "-v", jr.rd.ToolCache+":"+ctrTools)
	run = append(run, ea...)
	if meta.Entrypoint != "" {
		ep, err := ienv.Interpolate(meta.Entrypoint)
		if err != nil {
			return failf("%v", err)
		}
		run = append(run, "--entrypoint", ep)
	}
	run = append(run, image)
	run = append(run, args...)

	direct := &sandbox.Policy{Version: sandbox.PolicyVersion, Net: sandbox.NetOpen, Argv: append([]string{eng.bin}, run...), Env: append(os.Environ(), cenv...)}
	res, eff2 := jr.launchPolicy(n, step, pr, direct.Argv, jr.work, nil, direct, emit, ctx)
	// files that root wrote in the mounted directories go back to the user
	if os.Getuid() != 0 {
		eng.output(context.WithoutCancel(ctx), "run", "--rm", "--entrypoint", "chown", "-v", jr.work+":"+ctrWorkspace, "-v", jr.rd.Home+":"+ctrHome,
			"-v", jr.rd.Temp+":"+ctrTemp, "-v", filesDir+":"+ctrFiles, image, "-R", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), ctrWorkspace, ctrHome, ctrTemp, ctrFiles)
	}
	return res, eff2, ""
}

package runner

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// RunnerBehaviorVersion names the behavior of GitHub Actions that the expression evaluator and the runner follow.
// Bump it when a change in what a workflow does is deliberate (a new expression rule, a command that is now refused).
const RunnerBehaviorVersion = "1 (the Actions behavior of October 2026)"

type reason struct {
	ID    string
	Title string
	Text  string
}

// reasons explain why jactionlint run skips, refuses or changes something. Messages end with a pointer to them.
var reasons = []reason{
	{"sandbox-unavailable", "The sandbox cannot be applied, so nothing ran",
		`Every step runs in a sandbox that the operating system enforces (Landlock and seccomp on Linux, Seatbelt on macOS). When the sandbox cannot be applied on this machine, jactionlint run stops before it runs anything, because running a third-party action unisolated would be a silent downgrade.

Check what the machine supports with:  jactionlint run --check-sandbox

Ways forward: upgrade the kernel (Landlock needs Linux 5.13, --net=proxy needs 6.7), run in WSL2 on Windows, use --container where a container engine is installed, or pass --sandbox=off to run without isolation (the steps then get your environment's files and network).`},
	{"sandbox-denied", "A step failed with a permission error",
		`The sandbox lets a step write only to its workspace copy, its throwaway home, the temp directories, the tool cache and /tmp, and read the system directories, the directories on PATH and the tool installs of mise, asdf, rustup and similar. Everything else, your real home directory first of all (~/.ssh, ~/.aws, ~/.config/gh), is closed.

If the step needs a directory, name it: --allow-read DIR (read and run) or --allow-write DIR (also write). Tools installed under your home that are not on that list, such as a toolchain managed by rustup, usually need --allow-read on their directory and the environment variable that points at it (--pass-env RUSTUP_HOME). --sandbox=off removes the sandbox altogether.`},
	{"runs-on-other-os", "A job needs another operating system",
		`A job runs on this machine when its runs-on label names this machine's operating system (ubuntu-* on Linux, macos-* on macOS, windows-* on Windows) or says nothing about it (self-hosted, custom labels). A cell whose label names another system is skipped, because a Linux script is not a macOS script.

From a Mac or a Windows machine, --container runs the ubuntu-* cells in a Linux container (it needs docker or podman).`},
	{"unpinned-action", "An action is not pinned to a commit",
		`Running an action executes someone else's code with your secrets, and a tag or a branch can move. jactionlint run therefore runs repository actions and reusable workflows only when they are pinned to a full 40-character commit SHA.

--trust-actions resolves the tag or branch now, prints the commit it chose, and runs that commit.`},
	{"unpinned-image", "A container image is not pinned by digest",
		`Pulling an image runs someone else's code, so images must be pinned by digest (name@sha256:...), like actions are pinned by commit.

--trust-images (or --trust-actions) pulls an image by its tag anyway. Each pull is printed before it happens.`},
	{"container-option-refused", "A container option was refused",
		`The options: string of a container or service is not passed to the container engine as it is. It is checked against a list of options that limit resources, set health checks and users. Options that would widen access (--privileged, --cap-add, -v or --mount of a host path, --network host, --pid, --device, --security-opt, -p) are refused, and so are options that are not on the list.

--unsafe-container-options passes the string through unchanged.`},
	{"no-container-engine", "A job or step needs a container engine",
		`container:, services: and Docker actions need docker or podman, and so does --container. Without an engine (or with --no-containers) only the jobs and steps that need one are skipped; everything else runs.

--container-engine picks docker or podman when both are installed.`},
	{"container-mount-invisible", "The container cannot see the run directory",
		`jactionlint mounts the workspace copy, the home directory and the temp directory of the job into the container at the same paths as on this machine. That works when the container engine runs on this machine. A remote engine (DOCKER_HOST over tcp or ssh), or one in another file system namespace, cannot see those directories, and the job stops with this message instead of failing in its first step.

Use an engine on this machine, or run without containers.`},
	{"dynamic-matrix", "A matrix is known only at run time",
		`A matrix that is built from the output of another job (fromJSON(needs.setup.outputs.matrix)) cannot be expanded before that job has run, so --list cannot show its cells. jactionlint run expands it when its turn comes.`},
	{"github-env-refused", "A variable was refused in GITHUB_ENV or GITHUB_PATH",
		`A step can export environment variables and PATH entries to the later steps of its job through the files GITHUB_ENV and GITHUB_PATH. Variables that change what code a program loads (NODE_OPTIONS, LD_PRELOAD, LD_LIBRARY_PATH, DYLD_*, BASH_ENV, ENV, PYTHONPATH, ...) are refused, and a PATH entry must be a directory that a step may run programs from, because both would let one step change what the next one executes.

Set such a variable in the env: of the step that needs it instead.`},
	{"set-env-disabled", "The set-env and add-path commands are disabled",
		`The workflow commands ::set-env:: and ::add-path:: are disabled on GitHub too, because they allowed environment injection. Write to the files $GITHUB_ENV and $GITHUB_PATH instead.`},
	{"secret-unset", "A secret was not supplied",
		`jactionlint run injects no token. secrets.NAME is empty unless you pass --secret NAME=VALUE or --secret-file. Supplied secrets are hidden as *** in all output.`},
	{"net-proxy-refused", "A host was refused by --net=proxy",
		`With --net=proxy a step can connect only to the runner's proxy, which lets through GitHub and the common package registries and refuses everything else. Tools that ignore HTTP_PROXY fail, which is the point.

--learn runs with nothing refused and prints the hosts the steps contacted; --allow-net host,host adds hosts to the list.`},
	{"windows", "Steps do not run natively on Windows yet",
		`The sandbox has no Windows backend yet, so jactionlint run refuses to run steps there. Run it in WSL2, where the Linux sandbox applies, or pass --sandbox=off to run without isolation.`},
}

func reasonByID(id string) *reason {
	for i := range reasons {
		if reasons[i].ID == id {
			return &reasons[i]
		}
	}
	return nil
}

// explainRef is the pointer that messages carry to an explanation.
func explainRef(id string) string { return " (jactionlint run --explain " + id + ")" }

// explain implements `run --explain [ID]`.
func explain(w io.Writer, id string) int {
	if id == "" || id == "list" {
		ids := make([]string, 0, len(reasons))
		for _, r := range reasons {
			ids = append(ids, r.ID)
		}
		sort.Strings(ids)
		fmt.Fprintln(w, "Reasons that jactionlint run explains (jactionlint run --explain ID):")
		for _, id := range ids {
			fmt.Fprintf(w, "  %-28s %s\n", id, reasonByID(id).Title)
		}
		return ExitOK
	}
	r := reasonByID(strings.ToLower(id))
	if r == nil {
		fmt.Fprintf(w, "no explanation named %q; run jactionlint run --explain to list them\n", id)
		return ExitUsage
	}
	fmt.Fprintf(w, "%s: %s\n\n%s\n", r.ID, r.Title, r.Text)
	return ExitOK
}

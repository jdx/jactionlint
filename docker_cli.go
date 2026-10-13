package jactionlint

import (
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// This file finds the images that a script pulls with the docker or podman command line: `docker run IMAGE`,
// `docker create IMAGE` and `docker pull IMAGE` (and the forms `docker container run`, `docker image pull`).
// It reads the options only as far as it knows them: an option it does not know could take the image as its value,
// so the command is then not read at all, and no image is guessed.

// dockerImage is an image that a command of a script uses.
type dockerImage struct {
	// Ref is the image as written: a name with an optional tag or digest.
	Ref string
	// Word is the word of the script which holds it.
	Word *runscript.Word
	// Command is what runs it: "docker run", "podman pull" and so on.
	Command string
}

// dockerRunValueFlags are the options of `docker run` and `docker create` (and of podman, which has the same ones)
// that take a value; the short ones are written with a single dash.
var dockerRunValueFlags = setOf(`--add-host --annotation --attach --blkio-weight --blkio-weight-device --cap-add --cap-drop --cgroup-parent
--cgroupns --cidfile --cpu-period --cpu-quota --cpu-rt-period --cpu-rt-runtime --cpu-shares --cpus --cpuset-cpus --cpuset-mems
--detach-keys --device --device-cgroup-rule --device-read-bps --device-read-iops --device-write-bps --device-write-iops --dns
--dns-option --dns-search --domainname --entrypoint --env --env-file --expose --gpus --group-add --health-cmd --health-interval
--health-retries --health-start-interval --health-start-period --health-timeout --hostname --ip --ip6 --ipc --isolation
--kernel-memory --label --label-file --link --link-local-ip --log-driver --log-opt --mac-address --memory --memory-reservation
--memory-swap --memory-swappiness --mount --name --network --network-alias --oom-score-adj --pid --pids-limit --platform
--publish --pull --restart --runtime --security-opt --shm-size --stop-signal --stop-timeout --storage-opt --sysctl --tmpfs
--ulimit --user --userns --uts --volume --volume-driver --volumes-from --workdir --authfile --pod --secret --systemd --tz
--umask --timeout --cgroups --decryption-key --hostuser --mem-swap-limit --requires
-a -c -e -h -l -m -p -u -v -w`)

// dockerRunBoolFlags are the options without a value.
var dockerRunBoolFlags = setOf(`--detach --disable-content-trust --init --interactive --no-healthcheck --oom-kill-disable --privileged
--publish-all --quiet --read-only --rm --sig-proxy --tty --replace --help --http-proxy --read-only-tmpfs --rdt-class
-d -i -t -P -q`)

// dockerPullValueFlags and dockerPullBoolFlags are the options of `docker pull` and `podman pull`.
var (
	dockerPullValueFlags = setOf(`--platform --authfile --creds --arch --os --variant --cert-dir --decryption-key --signature-policy --retry --retry-delay`)
	dockerPullBoolFlags  = setOf(`--all-tags --disable-content-trust --quiet --all-platforms -a -q`)
)

// dockerGlobalValueFlags are the options in front of the verb of the docker command.
var dockerGlobalValueFlags = setOf(`-H --host --config -c --context -l --log-level --tlscacert --tlscert --tlskey --remote --url --connection --root --runroot --storage-driver`)
var dockerGlobalBoolFlags = setOf(`-D --debug --tls --tlsverify --version -v`)

func setOf(s string) map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(s) {
		m[f] = true
	}
	return m
}

// builtImagesOf reports whether the words of the command can name an image that the repository builds: the commands of
// the repository and of other tools do (`./build.sh app:ci`, `kind load docker-image app:ci`), and so do `docker push` and
// `docker save`. Commands which only use an image from a registry (`docker inspect`, `docker exec`) do not.
func builtImagesOf(c *runscript.Command) bool {
	switch c.Name {
	case "docker", "podman", "nerdctl":
		for _, a := range c.Args {
			if a.Value == "push" || a.Value == "save" {
				return true
			}
		}
		return false
	}
	return true
}

// imageWordRe matches a word which can be a reference to an image with a tag or a registry: "app:ci", "ghcr.io/o/app".
var imageWordRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*(:[A-Za-z0-9._-]+)?(@sha256:[0-9a-f]+)?$`)

// imageIDRe matches the id of an image, which a registry does not serve.
var imageIDRe = regexp.MustCompile(`^(sha256:)?[0-9a-f]{12,64}$`)

// dockerImagesOf returns the image of a `docker run`, `docker create` or `docker pull` command (also of podman and
// nerdctl). It returns false when the command is something else, when it uses an option which is not known, or when
// the image is not a literal.
func dockerImagesOf(c *runscript.Command) (dockerImage, bool) {
	switch c.Name {
	case "docker", "podman", "nerdctl":
	default:
		return dockerImage{}, false
	}
	args := c.Args
	i := 0
	// options in front of the verb
	for i < len(args) && strings.HasPrefix(args[i].Value, "-") && args[i].Value != "--" {
		a := args[i]
		if a.Dynamic() {
			return dockerImage{}, false
		}
		name, _, hasValue := strings.Cut(a.Value, "=")
		switch {
		case hasValue:
			i++
		case dockerGlobalValueFlags[name]:
			i += 2
		case dockerGlobalBoolFlags[name]:
			i++
		default:
			return dockerImage{}, false
		}
	}
	if i >= len(args) || args[i].Dynamic() {
		return dockerImage{}, false
	}
	verb := args[i].Value
	i++
	// docker container run, docker image pull
	if verb == "container" || verb == "image" {
		if i >= len(args) || args[i].Dynamic() {
			return dockerImage{}, false
		}
		verb = args[i].Value
		i++
	}
	var value, boolean map[string]bool
	switch verb {
	case "run", "create":
		value, boolean = dockerRunValueFlags, dockerRunBoolFlags
	case "pull":
		value, boolean = dockerPullValueFlags, dockerPullBoolFlags
	default:
		return dockerImage{}, false
	}
	for ; i < len(args); i++ {
		a := args[i]
		v := a.Value
		if v == "--" {
			i++
			break
		}
		if !strings.HasPrefix(v, "-") || v == "-" {
			break
		}
		if a.Dynamic() && !strings.Contains(v, "=") && value[v] {
			// the value of a known option is the next word, whatever it is
			i++
			continue
		}
		if strings.HasPrefix(v, "--") {
			name, _, hasValue := strings.Cut(v, "=")
			switch {
			case hasValue || boolean[name]:
			case value[name]:
				i++
			default:
				return dockerImage{}, false
			}
			continue
		}
		// short options: a cluster of options without a value which can end in one with a value (-it, -itv /a:/b), or
		// an option with its value attached (-p8080:80, -eA=b)
		for j := 1; j < len(v); j++ {
			flag := "-" + string(v[j])
			if boolean[flag] {
				continue
			}
			if value[flag] {
				if j == len(v)-1 {
					i++ // the value is the next word
				}
				break
			}
			return dockerImage{}, false
		}
	}
	if i >= len(args) {
		return dockerImage{}, false
	}
	img := args[i]
	if img.Dynamic() || img.Glob || img.Value == "" || strings.HasPrefix(img.Value, "-") {
		return dockerImage{}, false
	}
	// something other than an image reference (a path of an archive, an id, scratch)
	ref := img.Value
	if imageIDRe.MatchString(ref) || ref == "scratch" {
		return dockerImage{}, false
	}
	if strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") || strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, ".tar") || strings.HasSuffix(ref, ".tar.gz") {
		return dockerImage{}, false
	}
	// A transport of podman ("docker://alpine", "docker-archive:file", "oci:dir") names where the image is
	if scheme, rest, ok := strings.Cut(ref, "://"); ok {
		if scheme != "docker" || rest == "" {
			return dockerImage{}, false
		}
		ref = rest
	} else if strings.Contains(strings.SplitN(ref, "/", 2)[0], ":") && !strings.Contains(ref, "/") {
		// "name:tag" has a colon too, but "docker-archive:file" and "oci:dir" are known transports
		if t, _, _ := strings.Cut(ref, ":"); t == "docker-archive" || t == "oci" || t == "oci-archive" || t == "dir" || t == "containers-storage" || t == "docker-daemon" {
			return dockerImage{}, false
		}
	}
	return dockerImage{Ref: ref, Word: img, Command: c.Name + " " + verb}, true
}

// localImagesOf returns the names of the images that the commands build or tag in the script
// (`docker build -t name`, `docker tag a name`, `docker commit c name`), and whether the script loads images from an
// archive or builds them in another way, so that the images it runs cannot be told from the ones of a registry.
func localImagesOf(s *runscript.Script) (names []string, unknown bool) {
	for _, c := range s.Commands {
		// The name of an image which another command gets as an argument (`./build.sh playwright:localbuild`,
		// `docker push ghcr.io/o/app:ci`, `kind load docker-image app:ci`) is one that the repository builds: only the
		// command which pulls or runs it is not a sign of that.
		if _, ok := dockerImagesOf(c); !ok && c.Name != "echo" && c.Name != "printf" && builtImagesOf(c) {
			for _, a := range c.Args {
				if a.Literal() && imageWordRe.MatchString(a.Value) {
					names = append(names, a.Value)
				}
			}
		}
		switch c.Name {
		case "docker", "podman", "nerdctl", "buildah", "docker-compose", "kaniko", "buildctl", "earthly":
		default:
			continue
		}
		verb, rest := dockerSubcommand(c)
		switch verb {
		case "load", "import", "compose", "bake", "buildx bake", "image load", "image import":
			unknown = true
		case "build", "buildx build", "builder build", "image build":
			// -t is the tag of a build (and the terminal of a run, which is another subcommand)
			for k, w := range rest {
				switch {
				case (w == "-t" || w == "--tag") && k+1 < len(rest):
					names = append(names, rest[k+1])
				case strings.HasPrefix(w, "--tag="):
					names = append(names, strings.TrimPrefix(w, "--tag="))
				case len(w) > 2 && strings.HasPrefix(w, "-t") && !strings.HasPrefix(w, "--") && c.Name != "buildah":
					names = append(names, w[2:])
				}
			}
		case "tag", "image tag", "commit", "container commit":
			// docker tag SRC DST, docker commit CONTAINER IMAGE
			var pos []string
			for k := 0; k < len(rest); k++ {
				w := rest[k]
				switch {
				case !strings.HasPrefix(w, "-"):
					pos = append(pos, w)
				case !strings.Contains(w, "=") && dockerCommitValueFlags[w] && strings.HasSuffix(verb, "commit"):
					k++ // the value of the option is not an argument
				}
			}
			if len(pos) >= 2 {
				names = append(names, pos[1])
			}
		}
		if c.Name == "docker-compose" || c.Name == "buildah" || c.Name == "kaniko" || c.Name == "buildctl" || c.Name == "earthly" {
			unknown = true
		}
	}
	return names, unknown
}

// dockerSubcommand returns the subcommand of a docker, podman or nerdctl command, after its global options: "run", "build",
// or two words for "buildx build", "image tag", and so on. rest are the words after it. It returns "" for a command whose
// global options it does not know, which the other checks then read as no subcommand at all.
func dockerSubcommand(c *runscript.Command) (string, []string) {
	words := make([]string, 0, len(c.Args))
	for _, a := range c.Args {
		words = append(words, a.Value)
	}
	// skipFlags returns the index of the first word from i which is not an option or the value of one. An option which is not
	// known ends the reading: the subcommand is not known then.
	skipFlags := func(i int, value map[string]bool) (int, bool) {
		for i < len(words) && strings.HasPrefix(words[i], "-") && words[i] != "--" {
			name, _, hasValue := strings.Cut(words[i], "=")
			switch {
			case hasValue || dockerGlobalBoolFlags[name]:
				i++
			case value[name]:
				i += 2
			default:
				return i, false
			}
		}
		return i, true
	}
	i, ok := skipFlags(0, dockerGlobalValueFlags)
	if !ok || i >= len(words) {
		return "", nil
	}
	verb := words[i]
	i++
	if dockerManagementCommands[verb] {
		// docker buildx --builder name build: the options of the management command come before its subcommand
		i, ok = skipFlags(i, dockerManagementValueFlags)
		if !ok || i >= len(words) {
			return "", nil
		}
		verb += " " + words[i]
		i++
	}
	return verb, words[i:]
}

// dockerManagementCommands are the commands which have subcommands of their own (docker image tag, docker buildx build).
var dockerManagementCommands = setOf("container image builder buildx manifest")

// dockerManagementValueFlags are the options with a value that come between a management command and its subcommand.
var dockerManagementValueFlags = setOf("--builder --context -c --config -H --host --log-level -l")

// dockerCommitValueFlags are the options of `docker commit` which take a value.
var dockerCommitValueFlags = setOf("-m --message -a --author -c --change")

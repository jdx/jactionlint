package runscript

// Publish describes a command which publishes a package or a release.
type Publish struct {
	Cmd *Command
	// Tool is the tool of the command, such as twine, cargo, npm, gh.
	Tool string
	// Verb is the subcommand: upload, publish, push, release create, release upload.
	Verb string
	// Kind is "package" for registries (PyPI, crates.io, npm, rubygems) and "release" for GitHub releases.
	Kind string
	// DryRun is whether the command only simulates the publish (`--dry-run`, `-n`, `--no-publish`).
	DryRun bool
	// Registry is the value of the option that selects a registry other than the default
	// (`--repository-url`, `--registry`, `--index-url`), nil if there is none.
	Registry *Word
}

func publishOf(c *Command) *Publish {
	if c.Tool == "" || c.NameWord == nil {
		return nil
	}
	p := &Publish{Cmd: c, Tool: c.Tool, Kind: "package"}
	switch c.Tool {
	case "twine":
		if c.Verb() != "upload" {
			return nil
		}
		p.Verb = "upload"
		p.setRegistry("--repository-url", "-r", "--repository")
	case "cargo":
		if c.Verb() != "publish" {
			return nil
		}
		p.Verb = "publish"
		p.setRegistry("--registry", "--index")
	case "npm", "pnpm", "bun":
		if c.Verb() != "publish" {
			return nil
		}
		p.Verb = "publish"
		p.setRegistry("--registry")
	case "yarn":
		switch {
		case c.Verb() == "npm" && c.Sub(1) == "publish":
			p.Verb = "npm publish"
		case c.Verb() == "publish":
			p.Verb = "publish"
		default:
			return nil
		}
		p.setRegistry("--registry")
	case "gem":
		if c.Verb() != "push" {
			return nil
		}
		p.Verb = "push"
		p.setRegistry("--host")
	case "uv":
		if c.Verb() != "publish" {
			return nil
		}
		p.Verb = "publish"
		p.setRegistry("--publish-url", "--index")
	case "poetry", "flit", "hatch":
		if c.Verb() != "publish" {
			return nil
		}
		p.Verb = "publish"
		p.setRegistry("-r", "--repository", "--repo")
	case "gh":
		if c.Verb() != "release" || (c.Sub(1) != "create" && c.Sub(1) != "upload") {
			return nil
		}
		p.Kind, p.Verb = "release", "release "+c.Sub(1)
	default:
		return nil
	}
	// Only the long flag: `-n` is --notes for gh and --no-interaction for poetry, so it does not mean a dry run.
	p.DryRun = c.HasFlag("--dry-run")
	return p
}

func (p *Publish) setRegistry(names ...string) {
	if vs := p.Cmd.FlagValues(names...); len(vs) > 0 {
		p.Registry = vs[len(vs)-1]
	}
}

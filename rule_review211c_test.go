package jactionlint

import "testing"

// The options of the management commands and the values of the options of commit are not subcommands or arguments (Bugbot on #211).
func TestUnpinnedImagesSubcommandOptions(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   int
	}{
		{"buildx with the builder option", "docker buildx --builder mybuilder build -t alpine:3 .\ndocker run alpine:3", 0},
		{"buildx with the builder option and =", "docker buildx --builder=mybuilder build --tag alpine:3 .\ndocker run alpine:3", 0},
		{"buildx with a global option and a builder", "docker --context ci buildx --builder b build -t alpine:3 .\ndocker run alpine:3", 0},
		{"image build with an option of its own", "docker image --config /c build -t alpine:3 .\ndocker run alpine:3", 0},
		{"buildx bake after the builder option", "docker buildx --builder b bake\ndocker run alpine:3", 0},
		{"buildx with an option which is not known", "docker buildx --unknown x build -t alpine:3 .\ndocker run alpine:3", 1},
		{"commit with a message", "docker commit -m msg container alpine:3\ndocker run alpine:3", 0},
		{"commit with an author and a change", "docker commit --author me -c 'CMD x' container alpine:3\ndocker run alpine:3", 0},
		{"commit with options after the container", "docker commit container alpine:3 -m msg\ndocker run alpine:3", 0},
		{"container commit with a message", "docker container commit --message=msg container alpine:3\ndocker run alpine:3", 0},
		{"commit with a message is not the container", "docker commit -m msg container other:1\ndocker run alpine:3", 1},
		{"tag with a pause flag", "docker commit -p container alpine:3\ndocker run alpine:3", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := imageFindings(t, imagesCfg(t, ""), runWorkflow(tc.script))
			if len(got) != tc.want {
				t.Errorf("want %d findings, got %q", tc.want, got)
			}
		})
	}
}

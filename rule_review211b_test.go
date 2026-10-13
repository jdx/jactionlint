package jactionlint

import "testing"

// Only the subcommand build (and its forms) names an image with -t: the terminal of docker run, or a word "build" in its
// arguments, does not (Bugbot on #211).
func TestUnpinnedImagesBuildSubcommand(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   int // findings: the image of the run is reported unless the script builds it
	}{
		{"run with -t and a build word in the command", "docker run -t alpine:3 npm run build", 1},
		{"run named build with -t", "docker run --name build -t alpine:3 true", 1},
		{"run with -t and the word build as an argument", "docker run -t alpine:3 build", 1},
		{"run -t of an image that another run tags", "docker run -t alpine:3 sh -c 'docker build -t alpine:3 .'", 1},
		{"build", "docker build -t alpine:3 .\ndocker run alpine:3", 0},
		{"buildx build", "docker buildx build --tag alpine:3 .\ndocker run alpine:3", 0},
		{"builder build", "docker builder build -talpine:3 .\ndocker run alpine:3", 0},
		{"image build", "docker image build -t alpine:3 .\ndocker run alpine:3", 0},
		{"build after a global option", "docker -H unix:///x build -t alpine:3 .\ndocker run alpine:3", 0},
		{"tag", "docker tag a alpine:3\ndocker run alpine:3", 0},
		{"image tag", "docker image tag a alpine:3\ndocker run alpine:3", 0},
		{"commit", "docker commit c alpine:3\ndocker run alpine:3", 0},
		{"load", "docker load -i x.tar\ndocker run alpine:3", 0},
		{"a word load in the arguments of a run", "docker run alpine:3 load", 1},
		{"a word tag in the arguments of a run", "docker run alpine:3 tag a b", 1},
		{"buildx bake", "docker buildx bake\ndocker run alpine:3", 0},
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

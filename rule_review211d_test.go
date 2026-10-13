package jactionlint

import "testing"

// The suppression of the images that the workflow builds is for names that are provably built here: literal names only.
func TestUnpinnedImagesLocalSuppressionIsConservative(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"a build tag from a variable suppresses nothing", runWorkflow("docker build -t \"$IMAGE\" .\ndocker run alpine:3"), 1},
		{"a build tag with a variable suppresses nothing else", runWorkflow("docker build -t $REG/app:ci .\ndocker run alpine:3\ndocker pull node:20"), 2},
		{"a build tag with an expression suppresses nothing else", runWorkflow("docker build -t ${{ env.REG }}/app:ci .\ndocker run alpine:3"), 1},
		{"a variable in the tag only: the name is literal", runWorkflow("docker build -t ghcr.io/o/app:$TAG .\ndocker run ghcr.io/o/app:1"), 0},
		{"an expression in the tag only: the name is literal", runWorkflow("docker build -t ghcr.io/o/app:${{ github.sha }} .\ndocker run ghcr.io/o/app:1"), 0},
		{"the name of the image is a variable", runWorkflow("docker build -t \"$IMAGE:ci\" .\ndocker run ghcr.io/o/app:1"), 1},
		{"metadata-action tags are not image names", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/metadata-action@v5\n        with:\n          tags: |\n            type=sha,prefix=${{ github.ref_name }}-\n            type=ref,event=branch\n      - run: docker run alpine:3\n", 1},
		{"metadata-action images are image names", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/metadata-action@v5\n        with:\n          images: ghcr.io/o/app\n      - run: docker run ghcr.io/o/app:1\n", 0},
		{"build-push-action tags are image names", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/build-push-action@v6\n        with:\n          tags: ghcr.io/o/app:v1\n      - run: docker run ghcr.io/o/app:v1\n", 0},
		{"build-push-action tags with an expression suppress nothing else", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/build-push-action@v6\n        with:\n          tags: ${{ steps.meta.outputs.tags }}\n      - run: docker run alpine:3\n", 1},
		{"a registry with a port is pushed", runWorkflow("docker push registry.local:5000/app:ci\ndocker run registry.local:5000/app:ci"), 0},
		{"a registry with a port is an argument of another command", runWorkflow("./publish.sh registry.local:5000/app:ci\ndocker run registry.local:5000/app:ci"), 0},
		{"a registry with a port is built", runWorkflow("docker build -t registry.local:5000/app:ci .\ndocker run registry.local:5000/app:ci"), 0},
		{"another image of the registry with a port is reported", runWorkflow("docker push registry.local:5000/app:ci\ndocker run registry.local:5000/other:ci"), 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := imageFindings(t, imagesCfg(t, ""), tc.src)
			if len(got) != tc.want {
				t.Errorf("want %d findings, got %q", tc.want, got)
			}
		})
	}
}

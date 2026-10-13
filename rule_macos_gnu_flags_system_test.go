package jactionlint

import "testing"

func TestMacosSystemWords(t *testing.T) {
	for script, want := range map[string]bool{
		// A script that tests the system
		`if [ "$(uname -s)" = Darwin ]; then`:            true,
		`[[ "$OSTYPE" == darwin* ]]`:                     true,
		`echo "${RUNNER_OS}"`:                            true,
		`echo ${{ runner.os }}`:                          true,
		`if [ "${{ matrix.os }}" = macos-latest ]; then`: true,
		`sw_vers -productVersion`:                        true,
		// A script that uses the GNU tools
		`gsed -i 's/a/b/' f`: true,
		`gdate -d yesterday`: true,
		`gcp --parents a b`:  true,
		`PATH="$(brew --prefix)/opt/gnu-sed/libexec/gnubin:$PATH"`: true,
		`brew install coreutils`:                                   true,
		`brew install --quiet gnu-tar grep`:                        true,
		`sed --version | grep GNU`:                                 true,
		// Words that only look like them
		`gcloud auth login && echo $GCP_PROJECT`: false,
		`echo GCP`:                               false,
		`export GCPKEY=1`:                        false,
		`ggcp --parents a b`:                     false,
		`echo "$GITHUB_ACTOR is the username"`:   false,
		`echo "${MY_OSTYPE_FILE:-none}"`:         false,
		`brew install jq`:                        false,
		`gsutil cp a gs://b`:                     false,
		`ls docs/gnu_format.md`:                  false,
		`sed -i 's/a/b/' f`:                      false,
	} {
		if have := macosSystemRe.MatchString(script); have != want {
			t.Errorf("%q: have %v, want %v", script, have, want)
		}
	}
}

func TestOptionClusterHasAny(t *testing.T) {
	stat := func(w string) bool { return optionClusterHasAny(w, "FlLnqrsx", "ft", "fxsrFlnq") }
	date := func(w string) bool { return optionClusterHasAny(w, "jnRu", "frvzI", "jfvz") }
	for w, want := range map[string]bool{"-f": true, "-Lf": true, "-nf": true, "-x": true, "-s": true, "-r": true, "-L": false, "-c": false, "-Lc": false, "-cf": false, "-tf": false, "--format": false, "-": false} {
		if stat(w) != want {
			t.Errorf("stat %q: %v, want %v", w, stat(w), want)
		}
	}
	for w, want := range map[string]bool{"-j": true, "-jf": true, "-uj": true, "-v1d": true, "-v-1H": true, "-f": true, "-z": true, "-u": false, "-d": false, "-ud": false, "-rd": false, "--date": false} {
		if date(w) != want {
			t.Errorf("date %q: %v, want %v", w, date(w), want)
		}
	}
}

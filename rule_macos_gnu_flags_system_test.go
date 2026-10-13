package jactionlint

import "testing"

func TestScriptIsForMoreSystems(t *testing.T) {
	for script, want := range map[string]bool{
		// A script that tests the system
		`if [ "$(uname -s)" = Darwin ]; then echo a; fi`: true,
		`[[ "$OSTYPE" == darwin* ]] && echo a`:           true,
		`echo "${RUNNER_OS}"`:                            true,
		`echo ${{ runner.os }}`:                          true,
		`echo ${{ matrix.os }}`:                          true,
		`sw_vers -productVersion`:                        true,
		// A script that uses the GNU tools: the command word, the arguments of brew, the directory of the names
		`gsed -i 's/a/b/' f`:       true,
		`gdate -d yesterday`:       true,
		`gcp --parents a b`:        true,
		`sudo gsed -i x f`:         true,
		`env LC_ALL=C gsed -i x f`: true,
		`PATH="$(brew --prefix)/opt/gnu-sed/libexec/gnubin:$PATH"`: true,
		`brew install coreutils`:                                   true,
		`brew install --quiet gnu-tar grep`:                        true,
		`brew install jq && brew install grep`:                     true,
		`sed --version | grep GNU`:                                 true,
		// A script that cannot be read is not known
		`if [ -z "$X" ]; then`: true,
		// Words that only look like them
		`gcloud auth login; echo $GCP_PROJECT`:         false,
		`echo GCP`:                                     false,
		`export GCPKEY=1`:                              false,
		`ggcp --parents a b`:                           false,
		`echo "$GITHUB_ACTOR is the username"`:         false,
		`echo "${MY_OSTYPE_FILE:-none}"`:               false,
		`brew install jq`:                              false,
		`brew install jq && grep -P x f`:               false,
		`brew install jq; grep -P x f`:                 false,
		`gsutil cp a gs://b`:                           false,
		`ls docs/gnu_format.md`:                        false,
		`sed -i 's/a/b/' f`:                            false,
		`echo macos Darwin`:                            false,
		`echo "gsed is a command"`:                     false,
		`# gsed -i x f`:                                false,
		`grep -P x f # uname`:                          false,
		`echo ${{ github.event_name }} && grep -P x f`: false,
	} {
		if have := scriptIsForMoreSystems(script); have != want {
			t.Errorf("%q: have %v, want %v", script, have, want)
		}
	}
}

func TestStepConditionReadsTheSystem(t *testing.T) {
	for cond, want := range map[string]bool{
		"runner.os == 'Linux'":                 true,
		"${{ runner.os != 'macOS' }}":          true,
		"matrix.os == 'ubuntu-latest'":         true,
		"contains(matrix['os'], 'ubuntu')":     true,
		"github.event_name == 'push'":          false,
		"always()":                             false,
		"steps.a.outputs.x == 'runner.os'":     false,
		"${{ github.ref_name == 'matrix.x' }}": false,
	} {
		if have := stepConditionReadsTheSystem(&String{Value: cond, Pos: &Pos{}}); have != want {
			t.Errorf("%q: have %v, want %v", cond, have, want)
		}
	}
}

func TestOptionClusterHasAny(t *testing.T) {
	stat := func(w string) bool { return optionClusterHasAny(w, "FlLnqrsx", "ft", "fxsrFlnq") }
	date := func(w string) bool { return optionClusterHasAny(w, "jnRu", "frvzI", "jvz") }
	for w, want := range map[string]bool{"-f": true, "-Lf": true, "-nf": true, "-x": true, "-s": true, "-r": true, "-L": false, "-c": false, "-Lc": false, "-cf": false, "-tf": false, "--format": false, "-xyc": true, "-yc": false, "c": false, "-": false} {
		if stat(w) != want {
			t.Errorf("stat %q: %v, want %v", w, stat(w), want)
		}
	}
	// -f is an option of GNU date too (--file), so it does not show the form of macOS
	for w, want := range map[string]bool{"-j": true, "-jf": true, "-uj": true, "-v1d": true, "-v-1H": true, "-z": true, "-f": false, "-fj": false, "-u": false, "-d": false, "-ud": false, "-rd": false, "--date": false} {
		if date(w) != want {
			t.Errorf("date %q: %v, want %v", w, date(w), want)
		}
	}
}

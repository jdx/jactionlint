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
		`brew install coreutils`:                                   false, // keg-only: the BSD tool stays first on the PATH
		`brew install --quiet gnu-tar grep`:                        false,
		`brew install jq && brew install grep`:                     false,
		`sed --version | grep GNU`:                                 true,
		// The options of a wrapper and their values are not the command
		`sudo -u deploy gsed -i x f`:    true,
		`sudo -E -u deploy -g ci uname`: true,
		`env -u FOO gsed -i x f`:        true,
		`env -i FOO=1 gdate -d x`:       true,
		`timeout 5 uname -s`:            true,
		`timeout -k 1 5 gsed -i x f`:    true,
		`nice -n 10 gsed -i x f`:        true,
		`stdbuf -oL gsed -i x f`:        true,
		`ionice -c 2 -n 7 gsed -i x f`:  true,
		`xargs -n 1 gsed -i x`:          true,
		`sudo -u gsed sed -i x f`:       false,
		`env -u uname sed -i x f`:       false,
		`sudo -u deploy sed -i x f`:     false,
		`timeout 5 sed -i x f`:          false,
		// A cluster whose last letter takes a value: the next word is the value
		`sudo -Eu user gsed -i x f`: true,
		`sudo -En gsed -i x f`:      true,
		`sudo -Eu gsed sed -i x f`:  false,
		`sudo -Eu user sed -i x f`:  false,
		`sudo -uuser gsed -i x f`:   true,
		`sudo -Euuser gsed -i x f`:  true,
		// env -S is a command line
		`env -S 'gsed -i x f'`:              true,
		`env -S gsed -i x f`:                true,
		`env --split-string 'gsed -i x f'`:  true,
		`env --split-string=gsed`:           true,
		`env -S'gsed -i x f'`:               true,
		`env -S 'FOO=1 gsed -i x'`:          true,
		`env -S 'sudo -u deploy gsed -i x'`: true,
		`env -S 'sed -i x f'`:               false,
		`env -S "sed -i $X"`:                false,
		// The word GNU is a marker only where a version is searched for it
		`sed --version 2>&1 | grep -q GNU`:  true,
		`echo GNU; grep -P x f`:             false,
		`grep GNU notes.txt`:                false,
		`echo GNU; sed --version | head -1`: false,
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

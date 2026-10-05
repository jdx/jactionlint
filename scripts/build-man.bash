#!/bin/bash

set -e -o pipefail

if [ ! -d .git ]; then
    echo 'This script must be run from root of repository: bash ./scripts/build-man.bash' 1>&2
    exit 1
fi

# mise provides Ruby 3.3 for this script; ronn does not work with Ruby 4.
gem_home="$(mktemp -d)"
trap 'rm -rf "$gem_home"' EXIT
export GEM_HOME="$gem_home"

echo 'Installing ronn-ng (the maintained fork of ronn)'
gem install --no-document ronn-ng

echo 'Generating man/jactionlint.1'
# Only roff. Generating HTML fails inside ronn on current Ruby (undefined method `strip' for an Array).
# Run through the current ruby; the wrapper script in $GEM_HOME/bin expects a ruby next to it.
ruby -e 'load Gem.bin_path("ronn-ng", "ronn")' -- --roff ./man/jactionlint.1.ronn

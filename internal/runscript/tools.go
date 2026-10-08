package runscript

// toolValueFlags lists, per tool, the options which take a value as a separate word (`--index-url URL`).
// Options with `=` or the attached short form (`-rfile`) need no table. The lists are not exhaustive; they
// contain the options which are common in workflows and would otherwise make their value look like a package
// or a URL. They are shared between the subcommands of a tool.
var toolValueFlags = map[string]map[string]bool{
	"pip": set(`-r --requirement -c --constraint -e --editable -i --index-url --extra-index-url -f --find-links -t --target
		--prefix --root --src --platform --python-version --implementation --abi --upgrade-strategy --only-binary --no-binary
		--global-option --install-option --config-settings -C --log --proxy --retries --timeout --exists-action --cert
		--client-cert --trusted-host --cache-dir --python --report --progress-bar --use-feature --use-deprecated
		--keyring-provider`),
	"pipx": set(`--python --pip-args --index-url -i --spec --suffix --preinstall`),
	"uv": set(`-r --requirement -c --constraint -e --editable --index-url -i --extra-index-url -f --find-links --index
		--default-index --python -p --target --prefix --python-version --python-platform --from --with --with-requirements
		--with-editable --extra --group --only-group --no-binary --only-binary --no-build --config-setting -C
		--link-mode --exclude-newer --resolution --prerelease --index-strategy --keyring-provider --cache-dir --directory
		--project --config-file --color --python-preference --python-downloads --reinstall-package -P --upgrade-package
		--no-build-package --no-binary-package --env-file --package --tag --branch --rev --git --marker`),
	"npm": set(`--prefix --registry --tag -w --workspace --cache --loglevel --userconfig --globalconfig --omit --include
		--save-prefix --otp --fetch-retries --fetch-timeout --scope --before --min-release-age --install-strategy
		--package -c --call --node-options --tag-version-prefix --access --auth-type --audit-level --cafile --ca --cert
		--key --proxy --https-proxy --noproxy --script-shell --location`),
	"npx":      set(`--package -p -c --call --registry --cache --userconfig --prefix --node-options`),
	"pnpm":     set(`--filter -F -C --dir --reporter --registry --store-dir --virtual-store-dir --lockfile-dir --modules-dir --tag --workspace-concurrency --network-concurrency --loglevel --config --otp --access --publish-branch --package -p --cpu --os --libc --pnpmfile --global-dir --global-bin-dir --prefix --fetch-timeout --fetch-retries`),
	"pnpm-dlx": set(`--package -p --registry`),
	"yarn":     set(`--cwd --registry --tag --otp --access --mutex --network-timeout --network-concurrency --modules-folder --cache-folder --link-folder --global-folder --prefix --proxy --https-proxy --cpu --os -p --package`),
	"bun":      set(`--cwd --registry --cache-dir --backend --concurrent-scripts --network-concurrency --tag --access --otp --config -c --filter -F --omit --os --cpu --package -p --revision`),
	"bunx":     set(`--package -p --registry --cwd --config -c`),
	"aube":     set(`--filter -F -C --dir --registry --store-dir --tag --reporter --loglevel --config --prefix --cpu --os --lockfile-dir --modules-dir`),
	"cargo": set(`--version --vers --git --branch --tag --rev --path --root --registry --index --features -F --target --bin
		--example --profile -j --jobs --target-dir --config -Z --message-format --color --manifest-path --targets --pkg-url
		--pkg-fmt --bin-dir --install-path --strategies --disable-strategies --timeout --token --package -p
		--lockfile-path --cargo-root`),
	"go":  set(`-C -o -p -mod -modfile -overlay -pkgdir -tags -ldflags -gcflags -asmflags -gccgoflags -buildmode -compiler -installsuffix -pgo -toolexec -covermode -coverpkg`),
	"gem": set(`-v --version -s --source -i --install-dir -n --bindir --platform -P --trust-policy --build-root --config-file -p --http-proxy -C --target-rbconfig`),
	"apt": set(`-o --option -t --target-release -c --config-file -a --host-architecture -T --default-release`),
	"brew": set(`--cc --appdir --colorpicker-dir --fontdir --keyboard-layoutdir --mdimporterdir --prefpanedir --qlplugindir
		--screen-saverdir --servicedir --spelldir --vst-plugindir --vst3-plugindir --audio-unit-plugindir --dictionarydir
		--input-methoddir --language --env`),
	"twine":  set(`-r --repository --repository-url -u --username -p --password -c --comment --sign-with -i --identity --config-file --cert --client-cert`),
	"poetry": set(`-r --repository -u --username -p --password --cert --client-cert -C --directory -P --project`),
	"flit":   set(`--repository --format`),
	"hatch":  set(`-r --repo -u --user -a --auth -p --publisher`),
	"gh": set(`-R --repo -t --title -b --body -F --body-file -f --field -X --method -H --header -q --jq --template -B --base
		--head -a --assignee -l --label -m --milestone --notes -n --notes-file --notes-start-tag --target --discussion-category
		--limit -L --state -s --json --search -S --author -A --branch --workflow -w --ref -r --env -e --name --pattern -p
		--dir -D --hostname --scopes --user --reviewer --cache --input --preview`),
	"curl": set(`-o --output -H --header -d --data --data-raw --data-binary --data-urlencode --data-ascii -X --request -u --user
		-A --user-agent -e --referer -m --max-time --connect-timeout -w --write-out -b --cookie -c --cookie-jar -K --config
		-T --upload-file -F --form --form-string -x --proxy -U --proxy-user --proxy-header -y --speed-time -Y --speed-limit
		-z --time-cond -C --continue-at -r --range -D --dump-header --retry --retry-delay --retry-max-time --proto
		--proto-redir --cacert --capath --cert --key --pass --resolve --connect-to --interface --limit-rate --max-filesize
		--oauth2-bearer --aws-sigv4 --url --url-query --json --max-redirs --tls-max --ciphers --output-dir --variable
		--expand-url --rate --trace --trace-ascii -E --cert-type --key-type --pinnedpubkey --stderr --dns-servers --doh-url
		--unix-socket --abstract-unix-socket -P --ftp-port -Q --quote`),
	"wget": set(`-O --output-document -o --output-file -a --append-output -P --directory-prefix -T --timeout -t --tries
		-U --user-agent --header --post-data --post-file --user --password --http-user --http-password --ca-certificate
		--certificate --private-key --load-cookies --save-cookies --referer --limit-rate -e --execute -i --input-file -B --base
		--bind-address -w --wait --waitretry --dns-timeout --connect-timeout --read-timeout -Q --quota -l --level
		--secure-protocol --method --body-data --body-file --ca-directory --crl-file --proxy-user --proxy-password
		-I --include-directories -X --exclude-directories -A --accept -R --reject --accept-regex --reject-regex
		-D --domains --exclude-domains --restrict-file-names --prefer-family`),
	"tee": set(`--output-error`),
}

func init() {
	// -c takes the command string
	sh := set("-o +o -O +O -c --init-file --rcfile")
	for _, name := range []string{"sh", "bash", "zsh", "dash", "ash", "ksh", "busybox"} {
		toolValueFlags[name] = sh
	}
}

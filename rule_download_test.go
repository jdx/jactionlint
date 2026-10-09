package jactionlint

import (
	"strings"
	"testing"
)

// downloadWorkflow wraps a script in a one step workflow. Line 7 of the result is the first line of the script.
func downloadWorkflow(script string) string {
	var sb strings.Builder
	sb.WriteString("on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n")
	for _, l := range strings.Split(strings.TrimRight(script, "\n"), "\n") {
		sb.WriteString("          " + l + "\n")
	}
	return sb.String()
}

func TestUnverifiedDownload(t *testing.T) {
	tests := []struct {
		what   string
		script string
		lines  []int // the lines of the findings
	}{
		{"curl to sh", "curl -fsSL https://example.com/install.sh | sh", []int{7}},
		{"curl to sudo bash", "curl -fsSL https://example.com/install.sh | sudo bash -s -- -y", []int{7}},
		{"wget to bash", "wget -qO- https://example.com/install.sh | bash", []int{7}},
		{"process substitution", "bash <(curl -fsSL https://example.com/install.sh)", []int{7}},
		{"command substitution", "sh -c \"$(curl -fsSL https://example.com/install.sh)\"", []int{7}},
		{"eval", "eval \"$(curl -fsSL https://example.com/env.sh)\"", []int{7}},
		{"source", "source <(curl -fsSL https://example.com/env.sh)", []int{7}},
		{"python", "curl -sSL https://install.example.com | python3 -", []int{7}},
		{"dynamic url", "curl -fsSL \"$INSTALL_URL\" | sh", []int{7}},
		{"second line", "echo hi\ncurl -fsSL https://example.com/install.sh | sh", []int{8}},
		{"two downloads", "curl https://a.example.com/i | sh\nwget -O- https://b.example.com/i | sh", []int{7, 8}},
		{"tar is not an interpreter", "curl -fsSL https://example.com/x.tar.gz | tar xz", nil},
		{"jq", "curl -fsSL https://example.com/x.json | jq .", nil},
		{"python code", "curl -fsSL https://example.com/x.json | python3 -c 'import sys; print(sys.stdin.read())'", nil},
		{"python module", "curl -fsSL https://example.com/x.json | python3 -m json.tool", nil},
		{"shell reading a file", "curl -fsSL https://example.com/x | bash script.sh", nil},
		{"shell running a command", "curl -fsSL https://example.com/x | sh -c 'cat'", nil},
		{"commit pinned", "curl -fsSL https://raw.githubusercontent.com/o/r/0123456789abcdef0123456789abcdef01234567/install.sh | sh", nil},
		{"branch is not pinned", "curl -fsSL https://raw.githubusercontent.com/o/r/main/install.sh | sh", []int{7}},
		{"short sha is not pinned", "curl -fsSL https://raw.githubusercontent.com/o/r/0123456/install.sh | sh", []int{7}},
		{"localhost", "curl -fsSL http://localhost:8080/install.sh | sh", nil},
		{"private address", "curl -fsSL http://10.0.0.5/install.sh | sh", nil},
		{"archive downloaded to a file", "curl -fsSL https://example.com/x -o x.tgz\ntar xzf x.tgz", nil},

		{"chmod and run", "curl -fsSLo tool https://example.com/tool\nchmod +x tool\n./tool --version", []int{7}},
		{"chmod only", "curl -fsSLo tool https://example.com/tool\nchmod 755 tool", []int{7}},
		{"wget default name", "wget https://example.com/dl/tool.sh\nbash tool.sh", []int{7}},
		{"remote name", "curl -fsSLO https://example.com/dl/tool\nchmod u+x ./tool", []int{7}},
		{"redirect", "curl -fsSL https://example.com/tool > tool\nchmod a+x tool", []int{7}},
		{"tee", "curl -fsSL https://example.com/tool | sudo tee /usr/local/bin/tool >/dev/null\nsudo chmod 0755 /usr/local/bin/tool", []int{7}},
		{"moved before chmod", "curl -fsSLo tool https://example.com/tool\nsudo mv tool /usr/local/bin/tool\nsudo chmod +x /usr/local/bin/tool", []int{7}},
		{"moved into a directory", "curl -fsSLo tool https://example.com/tool\nsudo mv tool /usr/local/bin/\nsudo chmod +x /usr/local/bin/tool", []int{7}},
		{"install -m", "curl -fsSLo tool https://example.com/tool\nsudo install -m 755 tool /usr/local/bin/tool", []int{7}},
		{"dpkg", "curl -fsSLo x.deb https://example.com/x.deb\nsudo dpkg -i x.deb", []int{7}},
		{"apt install of a file", "curl -fsSLo x.deb https://example.com/x.deb\nsudo apt-get install -y ./x.deb", []int{7}},
		{"wget output document", "wget -O /tmp/tool https://example.com/tool\nchmod +x /tmp/tool\n/tmp/tool", []int{7}},
		{"source a file", "curl -fsSLo env.sh https://example.com/env.sh\nsource ./env.sh", []int{7}},
		{"reported once", "curl -fsSLo tool https://example.com/tool\nchmod +x tool\n./tool\n./tool", []int{7}},
		{"checksum", "curl -fsSLo tool https://example.com/tool\necho \"abc  tool\" | sha256sum -c -\nchmod +x tool\n./tool", nil},
		{"shasum", "curl -fsSLo tool https://example.com/tool\nshasum -a 256 -c tool.sha256\nchmod +x tool", nil},
		{"gpg", "curl -fsSLo tool https://example.com/tool\ngpg --verify tool.asc tool\nchmod +x tool", nil},
		{"cosign", "curl -fsSLo tool https://example.com/tool\ncosign verify-blob --signature s tool\nchmod +x tool", nil},
		{"attestation", "curl -fsSLo tool https://example.com/tool\ngh attestation verify tool --repo o/r\nchmod +x tool", nil},
		{"check after the run", "curl -fsSLo tool https://example.com/tool\nchmod +x tool\n./tool\nsha256sum -c sums", []int{7}},
		{"archive is not executed", "curl -fsSLo x.tgz https://example.com/x.tgz\ntar xzf x.tgz", nil},
		{"another file", "curl -fsSLo data.json https://example.com/data.json\nchmod +x build.sh", nil},
		{"gpg import is not a verification", "curl -fsSLo tool https://example.com/tool\ngpg --import key.asc\nchmod +x tool", []int{7}},
		{"stdout is not a file", "curl -fsSL https://example.com/tool -o -\nchmod +x -", nil},
		{"printing the hash is not a check", "curl -fsSLo tool https://example.com/tool\nsha256sum tool\nchmod +x tool", []int{7}},
		{"shasum without -c", "curl -fsSLo tool https://example.com/tool\nshasum -a 256 tool\nchmod +x tool", []int{7}},
		{"openssl dgst prints the hash", "curl -fsSLo tool https://example.com/tool\nopenssl dgst -sha256 tool\nchmod +x tool", []int{7}},
		{"openssl dgst -verify", "curl -fsSLo tool https://example.com/tool\nopenssl dgst -sha256 -verify key.pem -signature tool.sig tool\nchmod +x tool", nil},
		{"sha256sum --check", "curl -fsSLo tool https://example.com/tool\nsha256sum --check sums\nchmod +x tool", nil},
		{"install -t", "curl -fsSLo tool https://example.com/tool\nsudo install -m 755 -t /usr/local/bin tool", []int{7}},
		{"install --target-directory", "curl -fsSLo tool https://example.com/tool\nsudo install -m755 --target-directory=/usr/local/bin tool", []int{7}},
		{"install -Dm755", "curl -fsSLo tool https://example.com/tool\nsudo install -Dm755 tool /usr/local/bin/tool", []int{7}},
		{"install -Dm 755", "curl -fsSLo tool https://example.com/tool\nsudo install -Dm 755 tool /usr/local/bin/tool", []int{7}},
		{"install without a mode", "curl -fsSLo tool https://example.com/tool\nsudo install -t /usr/local/share tool", nil},

		{"python script", "curl -fsSLo install.py https://example.com/install.py\npython3 install.py", []int{7}},
		{"node script", "curl -fsSLo install.js https://example.com/install.js\nnode install.js", []int{7}},
		{"pathed interpreter", "curl -fsSLo install.py https://example.com/install.py\n/usr/bin/python3 install.py", []int{7}},
		{"env interpreter", "curl -fsSLo install.py https://example.com/install.py\n/usr/bin/env python3 install.py", []int{7}},
		{"node -r runs the script", "curl -fsSLo app.js https://example.com/app.js\nnode -r dotenv/config app.js", []int{7}},
		{"ruby -r runs the script", "curl -fsSLo app.rb https://example.com/app.rb\nruby -r json app.rb", []int{7}},
		{"node --check only parses", "curl -fsSLo app.js https://example.com/app.js\nnode --check app.js", nil},
		{"ruby -c only parses", "curl -fsSLo app.rb https://example.com/app.rb\nruby -c app.rb", nil},
		{"python -Werror is no -r", "curl -fsSLo data.json https://example.com/data.json\npython3 -Werror run.py data.json", nil},
		{"php -r is code", "curl -fsSLo data.php https://example.com/data.php\nphp -r 'echo 1;' data.php", nil},
		{"perl script", "wget https://example.com/install.pl\nperl install.pl", []int{7}},
		{"python -c does not run the file", "curl -fsSLo data.json https://example.com/data.json\npython3 -c 'print(1)' data.json", nil},
		{"curl -k", "curl -k https://example.com/x -o x", []int{7}},
		{"curl cluster -k", "curl -fsSLk https://example.com/x -o x", []int{7}},
		{"curl --insecure", "curl --insecure https://example.com/x -o x", []int{7}},
		{"wget no check", "wget --no-check-certificate https://example.com/x", []int{7}},
		{"curl -K is a config file", "curl -K cfg https://example.com/x -o x", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, nil, "ci.yaml", downloadWorkflow(tc.script)), "unverified-download", tc.lines...)
		})
	}
}

func TestUnverifiedDownloadAllow(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  unverified-download:\n    allow: [mise.run, \"https://get.example.com/\"]\n")
	for _, tc := range []struct {
		script string
		lines  []int
	}{
		{"curl https://mise.run | sh", nil},
		{"curl https://get.example.com/install | sh", nil},
		{"curl https://get.example.com.evil.net/install | sh", []int{7}},
		{"curl https://other.example.com/install | sh", []int{7}},
	} {
		wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", downloadWorkflow(tc.script)), "unverified-download", tc.lines...)
	}
}

func TestUnverifiedDownloadShells(t *testing.T) {
	const script = "curl -fsSL https://example.com/i.sh | sh"
	wf := func(job string) string {
		return "on: push\n" + job
	}
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"default shell", wf("jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + script + "\n"), []int{6}},
		{"windows default shell is pwsh", wf("jobs:\n  a:\n    runs-on: windows-latest\n    steps:\n      - run: " + script + "\n"), nil},
		{"windows with bash", wf("jobs:\n  a:\n    runs-on: windows-latest\n    steps:\n      - run: " + script + "\n        shell: bash\n"), []int{6}},
		{"job default bash on windows", wf("jobs:\n  a:\n    runs-on: windows-latest\n    defaults:\n      run:\n        shell: bash\n    steps:\n      - run: " + script + "\n"), []int{9}},
		{"workflow default pwsh", "on: push\ndefaults:\n  run:\n    shell: pwsh\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + script + "\n", nil},
		{"python shell", wf("jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + script + "\n        shell: python\n"), nil},
		{"shell with options", wf("jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + script + "\n        shell: bash --noprofile --norc -eo pipefail {0}\n"), []int{6}},
		{"plain scalar position", wf("jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - name: x\n        run: echo a && " + script + "\n"), []int{7}},
		{"quoted", wf("jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"" + script + "\"\n"), []int{6}},
		{"crlf", strings.ReplaceAll(downloadWorkflow(script), "\n", "\r\n"), []int{7}},
		{"expression in the script", downloadWorkflow("curl -fsSL https://example.com/${{ matrix.v }}/i.sh | sh"), []int{7}},
		{"two steps", wf("jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + script + "\n      - run: echo\n"), []int{6}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, nil, "ci.yaml", tc.src), "unverified-download", tc.lines...)
		})
	}
}

func TestUnverifiedDownloadOff(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  unverified-download: off\n")
	wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", downloadWorkflow("curl https://e.com/i | sh")), "unverified-download")
}

func TestInsecureSSHKeyscan(t *testing.T) {
	tests := []struct {
		what   string
		script string
		lines  []int
	}{
		{"append", "ssh-keyscan example.com >> ~/.ssh/known_hosts", []int{7}},
		{"overwrite", "ssh-keyscan -t ed25519 example.com > $HOME/.ssh/known_hosts", []int{7}},
		{"tee", "ssh-keyscan -H example.com | tee -a ~/.ssh/known_hosts", []int{7}},
		{"sudo tee", "ssh-keyscan example.com | sudo tee -a /etc/ssh/ssh_known_hosts >/dev/null", []int{7}},
		{"through a filter", "ssh-keyscan example.com 2>/dev/null | sort -u >> ~/.ssh/known_hosts", []int{7}},
		{"command substitution", "echo \"$(ssh-keyscan example.com)\" >> ~/.ssh/known_hosts", []int{7}},
		{"group", "{ ssh-keyscan a.example.com; ssh-keyscan b.example.com; } >> ~/.ssh/known_hosts", []int{7, 7}},
		{"secret host", "ssh-keyscan ${{ secrets.DEPLOY_HOST }} >> ~/.ssh/known_hosts", []int{7}},
		{"after mkdir", "mkdir -p ~/.ssh\nssh-keyscan example.com >> ~/.ssh/known_hosts", []int{8}},
		{"fingerprints are compared", "ssh-keyscan example.com > keys\nssh-keygen -lf keys\ncat keys >> ~/.ssh/known_hosts", nil},
		{"printed only", "ssh-keyscan example.com", nil},
		{"another file", "ssh-keyscan example.com > keys.txt", nil},
		{"stderr", "ssh-keyscan example.com 2>> ~/.ssh/known_hosts", nil},
		{"verified key", "echo \"${{ secrets.KNOWN_HOSTS }}\" >> ~/.ssh/known_hosts", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, nil, "ci.yaml", downloadWorkflow(tc.script)), "insecure-ssh-keyscan", tc.lines...)
		})
	}
}

func TestInsecureURLScheme(t *testing.T) {
	tests := []struct {
		what   string
		script string
		lines  []int
	}{
		{"curl", "curl -fsSL http://example.com/x -o x", []int{7}},
		{"wget", "wget http://example.com/x", []int{7}},
		{"ftp", "wget ftp://ftp.example.com/x.tgz", []int{7}},
		{"git clone", "git clone git://github.com/o/r", []int{7}},
		{"git clone http", "git -C dir clone http://example.com/o/r.git", []int{7}},
		{"git remote add", "git remote add up http://example.com/o/r.git", []int{7}},
		{"pip vcs url", "pip install git+http://example.com/o/r.git", []int{7}},
		{"svn vcs url", "pip install svn+http://example.com/o/r", []int{7}},
		{"vcs url over https", "pip install git+https://example.com/o/r.git", nil},
		{"pip index", "pip install --index-url http://pypi.example.com/simple pkg", []int{7}},
		{"pip index equals", "pip install --index-url=http://pypi.example.com/simple pkg", []int{7}},
		{"npm registry", "npm install --registry http://registry.example.com pkg", []int{7}},
		{"pip url", "pip install http://example.com/pkg.whl", []int{7}},
		{"in a pipeline", "curl -s http://example.com/x | tar xz", []int{7}},
		{"command substitution", "X=$(curl -s http://example.com/x)", []int{7}},
		{"upper case", "curl HTTP://example.com/x", []int{7}},
		{"query", "curl 'http://example.com/x?a=b&c=d'", []int{7}},
		{"https", "curl https://example.com/x", nil},
		{"localhost", "curl http://localhost:8080/health", nil},
		{"loopback", "curl http://127.0.0.1:3000/", nil},
		{"loopback v6", "curl 'http://[::1]:3000/'", nil},
		{"private", "curl http://192.168.1.10/x", nil},
		{"metadata", "curl http://169.254.169.254/latest/meta-data", nil},
		{"service name", "curl http://postgres:5432", nil},
		{"container host", "curl http://host.docker.internal:8080", nil},
		{"user info and port", "curl http://user:pw@example.com:8080/x", []int{7}},
		{"user info localhost", "curl http://user:pw@localhost:8080/x", nil},
		{"variable host", "curl http://$HOST/x", nil},
		{"expression host", "curl http://${{ secrets.HOST }}/x", nil},
		{"proxy", "curl -x http://proxy.example.com:3128 https://example.com", nil},
		{"proxy long", "curl --proxy http://proxy.example.com:3128 https://example.com", nil},
		{"header", "curl -H 'Referer: http://example.com' https://example.com", nil},
		{"post data", "curl -d 'url=http://example.com' https://example.com", nil},
		{"echo", "echo see http://example.com/docs", nil},
		{"git config rewrite", "git config --global url.\"https://github.com/\".insteadOf git://github.com/", nil},
		{"sed", "sed -i 's|http://example.com|https://example.com|' file", nil},
		{"comment", "# curl http://example.com/x", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, nil, "ci.yaml", downloadWorkflow(tc.script)), "insecure-url-scheme", tc.lines...)
		})
	}
}

func TestInsecureURLSchemeInputs(t *testing.T) {
	wf := func(with string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: octo/repo@v1\n        with:\n" + with
	}
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"http", wf("          url: http://example.com/x\n"), []int{8}},
		{"quoted", wf("          url: \"http://example.com/x\"\n"), []int{8}},
		{"ftp", wf("          mirror: ftp://ftp.example.com/\n"), []int{8}},
		{"https", wf("          url: https://example.com/x\n"), nil},
		{"localhost", wf("          registry-url: http://localhost:4873\n"), nil},
		{"text with a url", wf("          body: see http://example.com for details\n"), nil},
		{"expression", wf("          url: ${{ inputs.url }}\n"), nil},
		{"literal block", wf("          url: |\n            http://example.com/x\n"), []int{8}},
		{"two inputs", wf("          a: http://example.com/a\n          b: http://example.com/b\n"), []int{8, 9}},
		{"flow style", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: octo/repo@v1\n        with: { url: 'http://example.com/x' }\n", []int{7}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, nil, "ci.yaml", tc.src), "insecure-url-scheme", tc.lines...)
		})
	}
}

func TestCheckoutStaticCredentials(t *testing.T) {
	wf := func(with string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n        with:\n" + with
	}
	cfg := mustParseConfig(t, "rules:\n  checkout-static-credentials:\n    secret-tokens: true\n    allow: [Deploy_Key]\n")
	strict := mustParseConfig(t, "profile: strict\n")
	strictOff := mustParseConfig(t, "profile: strict\nrules:\n  checkout-static-credentials:\n    secret-tokens: false\n")
	tests := []struct {
		what  string
		src   string
		cfg   *Config
		lines []int
	}{
		{"token under the strict profile", wf("          token: ${{ secrets.PAT }}\n"), strict, []int{8}},
		{"token under the strict profile, option off", wf("          token: ${{ secrets.PAT }}\n"), strictOff, nil},
		{"ssh key", wf("          ssh-key: ${{ secrets.SSH_KEY }}\n"), nil, []int{8}},
		{"ssh key literal", wf("          ssh-key: |\n            -----BEGIN OPENSSH PRIVATE KEY-----\n"), nil, []int{8}},
		{"ssh key not a secret", wf("          ssh-key: ${{ steps.keys.outputs.key }}\n"), nil, nil},
		{"token is off by default", wf("          token: ${{ secrets.PAT }}\n"), nil, nil},
		{"token literal", wf("          token: ghp_0123456789\n"), nil, []int{8}},
		{"token", wf("          token: ${{ secrets.PAT }}\n"), cfg, []int{8}},
		{"token index", wf("          token: ${{ secrets['PAT'] }}\n"), cfg, []int{8}},
		{"token default", wf("          token: ${{ secrets.GITHUB_TOKEN }}\n"), cfg, nil},
		{"token github.token", wf("          token: ${{ github.token }}\n"), cfg, nil},
		{"token app", wf("          token: ${{ steps.app.outputs.token }}\n"), cfg, nil},
		{"token input", wf("          token: ${{ inputs.token }}\n"), cfg, nil},
		{"token with fallback", wf("          token: ${{ secrets.PAT || secrets.GITHUB_TOKEN }}\n"), cfg, nil},
		{"token with github.token fallback", wf("          token: ${{ secrets.PAT || github.token }}\n"), cfg, nil},
		{"computed secret", wf("          token: ${{ secrets[inputs.name] }}\n"), cfg, nil},
		{"token allowed", wf("          token: ${{ secrets.DEPLOY_KEY }}\n"), cfg, nil},
		{"empty token", wf("          token: ''\n"), cfg, nil},
		{"both", wf("          ssh-key: ${{ secrets.K }}\n          token: ${{ secrets.T }}\n"), cfg, []int{8, 9}},
		{"other action", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: octo/checkout@v4\n        with:\n          ssh-key: ${{ secrets.K }}\n", nil, nil},
		{"no inputs", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n", cfg, nil},
		{"upper case input", wf("          SSH-KEY: ${{ secrets.K }}\n"), nil, []int{8}},
		{"pinned by sha", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2\n        with:\n          ssh-key: ${{ secrets.K }}\n", nil, []int{8}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, tc.cfg, "ci.yaml", tc.src), "checkout-static-credentials", tc.lines...)
		})
	}
}

func TestURLParts(t *testing.T) {
	tests := []struct {
		in, scheme, host string
		ok               bool
	}{
		{"http://example.com/x", "http", "example.com", true},
		{"HTTP://Example.COM:8080/x?y=z", "http", "example.com", true},
		{"http://user:pw@example.com/x", "http", "example.com", true},
		{"http://[::1]:8080/x", "http", "::1", true},
		{"http://example.com.", "http", "example.com", true},
		{"http://$HOST/x", "http", "", true},
		{"http://${{ secrets.HOST }}/x", "http", "", true},
		{"git://github.com/o/r", "git", "github.com", true},
		{"example.com/x", "", "", false},
		{"://x", "", "", false},
	}
	for _, tc := range tests {
		scheme, host, ok := urlParts(tc.in)
		if scheme != tc.scheme || host != tc.host || ok != tc.ok {
			t.Errorf("urlParts(%q) = %q, %q, %v; want %q, %q, %v", tc.in, scheme, host, ok, tc.scheme, tc.host, tc.ok)
		}
	}
}

func TestIsLocalHost(t *testing.T) {
	for _, h := range []string{"localhost", "127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "192.168.0.1", "169.254.169.254", "0.0.0.0", "db", "a.localhost", "x.local", "x.internal", "x.svc", "x.cluster.local", ""} {
		if !isLocalHost(h) {
			t.Errorf("%q must be local", h)
		}
	}
	for _, h := range []string{"example.com", "8.8.8.8", "172.32.0.1", "localhost.example.com", "github.com"} {
		if isLocalHost(h) {
			t.Errorf("%q must not be local", h)
		}
	}
}

func TestHasExecMode(t *testing.T) {
	for m, want := range map[string]bool{
		"+x": true, "a+x": true, "u+x": true, "755": true, "0755": true, "700": true, "644": false, "600": false, "775": true,
		"u=rwx": true, "+rx": true, "u+rw": false, "a-x": false, "": false, "rwx": false, "0644": false, "111": true, "u+s": false,
	} {
		if got := hasExecMode(m); got != want {
			t.Errorf("hasExecMode(%q) = %v, want %v", m, got, want)
		}
	}
}

func TestMatchesAllowedURLEndsAtAURLBoundary(t *testing.T) {
	allow := []string{"https://github.com", "https://get.example.com/"}
	for url, want := range map[string]bool{
		"https://github.com":                     true,
		"https://github.com/o/r":                 true,
		"https://github.com?x=1":                 true,
		"https://github.com.evil.net/install.sh": false,
		"https://github.comx/o":                  false,
		"https://get.example.com/install":        true,
		"https://get.example.com.evil.net/x":     false,
		"HTTPS://GITHUB.COM/o":                   true,
	} {
		if got := matchesAllowedURL(allow, url); got != want {
			t.Errorf("matchesAllowedURL(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestClusteredInsecureFlagIsNamedAlone(t *testing.T) {
	errs := lintFileWithConfig(t, nil, "ci.yaml", downloadWorkflow("curl -fsSLk https://example.com/x -o x"))
	var msg string
	for _, e := range errs {
		if e.ID == "unverified-download" {
			msg = e.Message
		}
	}
	if !strings.HasPrefix(msg, `"-k" disables`) || !strings.Contains(msg, `remove "-k" and`) {
		t.Errorf("the message must name -k alone: %q", msg)
	}
}

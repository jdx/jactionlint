package sandbox

import (
	"strings"
	"testing"
)

func TestPolicyValidate(t *testing.T) {
	ok := Policy{Version: PolicyVersion, Net: NetOpen, Argv: []string{"/bin/true"}, Exec: []string{"/usr"}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mod := range map[string]func(p *Policy){
		"version":             func(p *Policy) { p.Version = 2 },
		"net":                 func(p *Policy) { p.Net = "wide" },
		"no command":          func(p *Policy) { p.Argv = nil },
		"empty command":       func(p *Policy) { p.Argv = []string{""} },
		"relative exec":       func(p *Policy) { p.Exec = []string{"usr"} },
		"relative write":      func(p *Policy) { p.Write = []string{"./x"} },
		"relative dir":        func(p *Policy) { p.Dir = "x" },
		"nul":                 func(p *Policy) { p.Read = []string{"/a\x00b"} },
		"ports without proxy": func(p *Policy) { p.ConnectPorts = []uint16{80} },
	} {
		p := ok
		mod(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestPolicyRoundTrip(t *testing.T) {
	p := &Policy{Net: NetProxy, ConnectPorts: []uint16{8080, 9000}, Argv: []string{"sh", "-c", "true"}, Env: []string{"A=1"}, Dir: "/work",
		Exec: []string{"/usr"}, Read: []string{"/etc"}, Write: []string{"/work"}}
	b, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePolicy(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Net != NetProxy || len(got.ConnectPorts) != 2 || got.Dir != "/work" || got.Argv[2] != "true" {
		t.Fatalf("%+v", got)
	}
	for _, bad := range []string{``, `{`, `{"version":1,"net":"open","argv":["x"]} {}`, `{"version":1,"net":"open","argv":["x"],"surprise":1}`, `[]`, `null`} {
		if _, err := ParsePolicy([]byte(bad)); err == nil {
			t.Errorf("ParsePolicy(%q) must fail", bad)
		}
	}
}

func FuzzParsePolicy(f *testing.F) {
	f.Add([]byte(`{"version":1,"net":"open","argv":["/bin/true"],"exec":["/usr"]}`))
	f.Add([]byte(`{"version":1,"net":"proxy","connect_ports":[1],"argv":["x"]}`))
	f.Add([]byte(`{"version":1,"net":"none","argv":["x"],"write":["/a","/b"],"env":["A=B"]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := ParsePolicy(b)
		if err != nil {
			return
		}
		// What parses must validate and serialize again.
		if err := p.Validate(); err != nil {
			t.Fatalf("parsed but invalid: %v", err)
		}
		if _, err := p.Marshal(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScrubEnv(t *testing.T) {
	parent := []string{"PATH=/usr/bin", "HOME=/home/me", "AWS_SECRET_ACCESS_KEY=x", "GITHUB_TOKEN=y", "LANG=C", "SSH_AUTH_SOCK=/tmp/a"}
	got := ScrubEnv(parent, DefaultPassEnv, map[string]string{"HOME": "/fake", "CI": "true"})
	want := "CI=true,HOME=/fake,LANG=C,PATH=/usr/bin"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
	if got := ScrubEnv(parent, append([]string{"GITHUB_TOKEN"}, DefaultPassEnv...), nil); !strings.Contains(strings.Join(got, ","), "GITHUB_TOKEN=y") {
		t.Fatalf("--pass-env lost: %v", got)
	}
}

func TestDangerousEnv(t *testing.T) {
	for _, n := range []string{"NODE_OPTIONS", "ld_preload", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "BASH_ENV", "ENV", "LD_ANYTHING"} {
		if !IsDangerousEnv(n) {
			t.Errorf("%s must be dangerous", n)
		}
	}
	for _, n := range []string{"PATH", "FOO", "GITHUB_SHA", "NODE_ENV"} {
		if IsDangerousEnv(n) {
			t.Errorf("%s must be allowed", n)
		}
	}
}

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy(Options{Workspace: "/run/work", Home: "/run/home", Temp: "/run/tmp", ToolCache: "/cache/tools",
		PathDirs: PathDirs("/usr/bin:relative:/opt/x/bin::/usr/local/bin"), RealHome: "/home/me", Net: NetNone,
		ExtraRead: []string{"/data"}, ExtraWrite: []string{"/out"}})
	has := func(list []string, s string) bool {
		for _, e := range list {
			if e == s {
				return true
			}
		}
		return false
	}
	for _, w := range []string{"/run/work", "/run/home", "/run/tmp", "/cache/tools", "/out"} {
		if !has(p.Write, w) {
			t.Errorf("%s must be writable", w)
		}
	}
	for _, e := range []string{"/opt/x/bin", "/usr/local/bin", "/data", "/home/me/.local/share/mise/installs"} {
		if !has(p.Exec, e) {
			t.Errorf("%s must be executable", e)
		}
	}
	for _, list := range [][]string{p.Exec, p.Read, p.Write} {
		for _, s := range list {
			if s == "/home/me" || s == "/home/me/.ssh" || s == "/" || s == "" {
				t.Errorf("the policy must never allow %q", s)
			}
			if strings.Contains(s, "relative") {
				t.Errorf("relative PATH entry leaked: %q", s)
			}
		}
	}
	if err := (&Policy{Version: p.Version, Net: p.Net, Exec: p.Exec, Read: p.Read, Write: p.Write, Argv: []string{"/bin/true"}}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSeatbeltProfile(t *testing.T) {
	p := &Policy{Version: 1, Net: NetProxy, ConnectPorts: []uint16{4000, 4001}, Argv: []string{"x"},
		Exec: []string{"/usr"}, Read: []string{"/private/etc"}, Write: []string{"/work dir", `/we"ird`}}
	s := seatbeltProfile(p)
	for _, want := range []string{
		"(deny default)",
		`(allow file-read* process-exec (subpath "/usr"))`,
		`(allow file-read* file-write* process-exec (subpath "/work dir"))`,
		`(subpath "/we\"ird")`,
		`(allow network-outbound (remote ip "localhost:4000"))`,
		`(allow network-outbound (remote ip "localhost:4001"))`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("profile lacks %s\n%s", want, s)
		}
	}
	if strings.Contains(s, "(allow network*)\n") {
		t.Error("proxy mode must not allow all network")
	}
	p.Net = NetOpen
	if !strings.Contains(seatbeltProfile(p), "(allow network*)") {
		t.Error("open mode allows the network")
	}
	p.Net = NetNone
	if s := seatbeltProfile(p); strings.Contains(s, "network-outbound") || strings.Contains(s, "(allow network*)\n") {
		t.Error("none mode allows no internet")
	}
}

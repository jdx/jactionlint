package runner

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func viaProxy(t *testing.T, p *Proxy, target string) (*http.Response, error) {
	t.Helper()
	pu, _ := url.Parse(p.URL())
	c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	return c.Get(target)
}

func TestProxyAllowListAndLearn(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "origin says hi") }))
	defer origin.Close()
	host := strings.TrimPrefix(origin.URL, "http://")
	name, _, _ := net.SplitHostPort(host)

	// not listed: refused with a message that names the flag
	p, err := StartProxy(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	resp, err := viaProxy(t, p, origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("an unlisted host answered %d", resp.StatusCode)
	}
	if len(p.Denied()) != 1 || p.Denied()[0] != name {
		t.Fatalf("denied = %v", p.Denied())
	}

	// listed by the user: a literal loopback address is allowed because the user named it
	p2, _ := StartProxy([]string{name}, false)
	defer p2.Close()
	resp, err = viaProxy(t, p2, origin.URL)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("an allowed host: %v %v", resp, err)
	}

	// learn mode refuses nothing but still keeps loopback and private addresses out unless named
	p3, _ := StartProxy(nil, true)
	defer p3.Close()
	resp, err = viaProxy(t, p3, origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 502 {
		t.Fatalf("a loopback target must be refused even in learn mode, got %d", resp.StatusCode)
	}
	if hosts := p3.Hosts(); len(hosts) != 1 || hosts[0] != name {
		t.Fatalf("hosts = %v", hosts)
	}
}

func TestProxyConnect(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { fmt.Fprint(c, "tunnel-open\n"); c.Close() }()
		}
	}()
	target := ln.Addr().String()
	name, _, _ := net.SplitHostPort(target)
	for _, tc := range []struct {
		allow []string
		want  string
	}{{nil, "403"}, {[]string{name}, "200"}} {
		p, _ := StartProxy(tc.allow, false)
		c, err := net.Dial("tcp", strings.TrimPrefix(p.URL(), "http://"))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
		br := bufio.NewReader(c)
		status, _ := br.ReadString('\n')
		if !strings.Contains(status, tc.want) {
			t.Errorf("allow %v: %q, want %s", tc.allow, status, tc.want)
		}
		if tc.want == "200" {
			for {
				l, _ := br.ReadString('\n')
				if l == "\r\n" || l == "" {
					break
				}
			}
			if l, _ := br.ReadString('\n'); l != "tunnel-open\n" {
				t.Errorf("through the tunnel: %q", l)
			}
		}
		c.Close()
		p.Close()
	}
}

func TestHostMatches(t *testing.T) {
	for _, tc := range []struct {
		pat, host string
		want      bool
	}{
		{"github.com", "github.com", true}, {"github.com", "api.github.com", false}, {"*.github.com", "api.github.com", true},
		{"*.github.com", "github.com", false}, {"*.github.com", "evilgithub.com", false}, {"*.githubusercontent.com", "objects.githubusercontent.com", true},
	} {
		if hostMatches(tc.pat, tc.host) != tc.want {
			t.Errorf("hostMatches(%q, %q) != %v", tc.pat, tc.host, tc.want)
		}
	}
	p, _ := StartProxy(nil, false)
	defer p.Close()
	for host, want := range map[string]bool{"registry.npmjs.org": true, "Crates.IO": true, "evil.example": false, "github.com.evil.example": false} {
		if got := p.allowed(host); got != want {
			t.Errorf("allowed(%q) = %v, want %v", host, got, want)
		}
	}
}

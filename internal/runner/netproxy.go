package runner

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// builtinHosts are the hosts a workflow needs most: GitHub and the package registries of the common ecosystems.
var builtinHosts = []string{
	"github.com", "api.github.com", "codeload.github.com", "objects.githubusercontent.com", "*.githubusercontent.com",
	"*.github.com", "ghcr.io", "*.pkg.github.com",
	"registry.npmjs.org", "registry.yarnpkg.com", "nodejs.org",
	"pypi.org", "files.pythonhosted.org",
	"crates.io", "index.crates.io", "static.crates.io", "static.rust-lang.org",
	"proxy.golang.org", "sum.golang.org", "go.dev", "dl.google.com", "storage.googleapis.com",
	"rubygems.org", "index.rubygems.org",
	"repo.maven.apache.org", "repo1.maven.org", "plugins.gradle.org", "services.gradle.org",
	"mise.jdx.dev", "mise-versions.jdx.dev",
}

// Proxy is the HTTP proxy that steps use under --net=proxy: it lets CONNECT and plain HTTP requests through to the allowed
// hosts, logs every host, and refuses the rest. It is the only thing the sandbox lets a step connect to (besides the
// cache service), so a tool that ignores HTTP_PROXY fails instead of reaching the network.
type Proxy struct {
	ln       net.Listener
	srv      *http.Server
	allow    []string
	explicit map[string]bool // hosts named by the user, which may be on a private network
	learn    bool

	mu     sync.Mutex
	seen   map[string]bool
	denied map[string]bool
}

// StartProxy listens on a loopback port. With learn, nothing is refused and the contacted hosts are collected.
func StartProxy(extra []string, learn bool) (*Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &Proxy{ln: ln, learn: learn, explicit: map[string]bool{}, seen: map[string]bool{}, denied: map[string]bool{}}
	p.allow = append(p.allow, builtinHosts...)
	for _, h := range extra {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			p.allow = append(p.allow, h)
			p.explicit[h] = true
		}
	}
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	go p.srv.Serve(ln)
	return p, nil
}

func (p *Proxy) Port() uint16 { return uint16(p.ln.Addr().(*net.TCPAddr).Port) }
func (p *Proxy) URL() string  { return "http://" + p.ln.Addr().String() }

func (p *Proxy) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p.srv.Shutdown(ctx)
}

// Env returns the variables that send a step's traffic to the proxy.
func (p *Proxy) Env() map[string]string {
	u := p.URL()
	no := "127.0.0.1,localhost,::1"
	return map[string]string{
		"HTTP_PROXY": u, "HTTPS_PROXY": u, "http_proxy": u, "https_proxy": u, "ALL_PROXY": u, "all_proxy": u,
		"NO_PROXY": no, "no_proxy": no,
		"NODE_USE_ENV_PROXY": "1", // Node.js 24 honours the proxy variables only when asked
	}
}

func hostMatches(pattern, host string) bool {
	if rest, ok := strings.CutPrefix(pattern, "*."); ok {
		return strings.HasSuffix(host, "."+rest)
	}
	return pattern == host
}

func (p *Proxy) allowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	p.mu.Lock()
	p.seen[host] = true
	p.mu.Unlock()
	if p.learn {
		return true
	}
	for _, pat := range p.allow {
		if hostMatches(pat, host) {
			return true
		}
	}
	p.mu.Lock()
	p.denied[host] = true
	p.mu.Unlock()
	return false
}

// Hosts returns the hosts that steps contacted, sorted.
func (p *Proxy) Hosts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return sortedKeys(p.seen)
}

// Denied returns the hosts that were refused, sorted.
func (p *Proxy) Denied() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return sortedKeys(p.denied)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// dialer refuses addresses on this machine and on private networks, whatever name led to them, unless the user named
// the host: a name that resolves to 127.0.0.1 or to a cloud metadata address must not turn the proxy into a way to reach
// local services.
func (p *Proxy) dialer(host string) *net.Dialer {
	d := &net.Dialer{Timeout: 30 * time.Second}
	if p.explicit[strings.ToLower(host)] {
		return d
	}
	d.Control = func(network, address string, c syscall.RawConn) error {
		h, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(h)
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("%s is an address of this machine or of a private network", h)
		}
		return nil
	}
	return d
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			http.Error(w, "bad CONNECT target", http.StatusBadRequest)
			return
		}
		if !p.allowed(host) {
			http.Error(w, "jactionlint: "+host+" is not on the allow-list of --net=proxy (add it with --allow-net "+host+")", http.StatusForbidden)
			return
		}
		up, err := p.dialer(host).DialContext(r.Context(), "tcp", r.Host)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			up.Close()
			http.Error(w, "cannot hijack", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		down, _, err := hj.Hijack()
		if err != nil {
			up.Close()
			return
		}
		go func() {
			io.Copy(up, down)
			up.Close()
		}()
		io.Copy(down, up)
		down.Close()
		return
	}
	if !r.URL.IsAbs() {
		http.Error(w, "jactionlint: this is a proxy; request an absolute URL", http.StatusBadRequest)
		return
	}
	host := r.URL.Hostname()
	if !p.allowed(host) {
		http.Error(w, "jactionlint: "+host+" is not on the allow-list of --net=proxy (add it with --allow-net "+host+")", http.StatusForbidden)
		return
	}
	tr := &http.Transport{DialContext: p.dialer(host).DialContext, Proxy: nil}
	defer tr.CloseIdleConnections()
	out := r.Clone(r.Context())
	out.RequestURI = ""
	for _, h := range []string{"Proxy-Connection", "Proxy-Authorization", "Connection", "Keep-Alive", "Te", "Trailer", "Upgrade"} {
		out.Header.Del(h)
	}
	resp, err := tr.RoundTrip(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

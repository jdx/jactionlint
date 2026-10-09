//go:build !js

package jactionlint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// maxTokenFileBytes bounds how much of a token file is read.
const maxTokenFileBytes = 4096

// ghCLITimeout bounds how long "gh auth token" may take.
const ghCLITimeout = 5 * time.Second

// tokenDiscovery says where to look for the token of the GitHub API.
type tokenDiscovery struct {
	// host is the host of the API. Tokens are looked up for it only.
	host string
	// dotCom is true for api.github.com, false for a GitHub Enterprise Server.
	dotCom bool
	// trusted is false when the host came from the config file of a repository: then no source is
	// consulted, so that the repository cannot make a token reach a server it chose.
	trusted bool

	tokenEnv  string
	tokenFile string
	useGH     bool

	getenv   func(string) string
	readFile func(string) ([]byte, error)
	runGH    func(ctx context.Context, host string) (string, error)
}

// discoveredToken is a token and where it came from. The source names a variable or a file, never the
// token, so it is safe to print.
type discoveredToken struct {
	token  string
	source string
}

// validToken reports whether the text can be a token. It refuses anything that could change the
// meaning of a request header (white space, control characters).
func validToken(s string) bool {
	if s == "" || len(s) > 1024 {
		return false
	}
	for _, r := range s {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// discover returns the first token found, in this order:
//
//  1. the variable named by token-env (-online-token-env),
//  2. the file named by token-file (-online-token-file),
//  3. $GITHUB_TOKEN, $GH_TOKEN (on an Enterprise Server $GITHUB_ENTERPRISE_TOKEN and $GH_ENTERPRISE_TOKEN first),
//  4. the output of "gh auth token --hostname HOST", only when gh is installed and nothing above had a token.
//
// It returns problems (an unreadable file, an empty variable the user named) as notices and goes on
// to the next source. A token found in the wrong shape is a notice too, and never printed.
func (d tokenDiscovery) discover(ctx context.Context) (tok discoveredToken, notices []string) {
	if !d.trusted {
		return discoveredToken{}, []string{"the API URL comes from the config file of the repository, so no token is sent to it. set GITHUB_API_URL or pass -online-api-url to choose the host yourself"}
	}
	if d.tokenEnv != "" {
		v := strings.TrimSpace(d.getenv(d.tokenEnv))
		switch {
		case v == "":
			notices = append(notices, fmt.Sprintf("the variable %s (token-env) is empty or not set", d.tokenEnv))
		case !validToken(v):
			notices = append(notices, fmt.Sprintf("the variable %s (token-env) does not hold a token (it has white space or control characters)", d.tokenEnv))
		default:
			return discoveredToken{v, "$" + d.tokenEnv}, notices
		}
	}
	if d.tokenFile != "" {
		v, err := d.readTokenFile()
		switch {
		case err != nil:
			notices = append(notices, err.Error())
		default:
			return discoveredToken{v, "the file " + d.tokenFile}, notices
		}
	}
	names := []string{"GITHUB_TOKEN", "GH_TOKEN"}
	if !d.dotCom {
		names = append([]string{"GITHUB_ENTERPRISE_TOKEN", "GH_ENTERPRISE_TOKEN"}, names...)
	}
	for _, n := range names {
		v := strings.TrimSpace(d.getenv(n))
		if v == "" {
			continue
		}
		if !validToken(v) {
			notices = append(notices, fmt.Sprintf("the variable %s does not hold a token (it has white space or control characters)", n))
			continue
		}
		return discoveredToken{v, "$" + n}, notices
	}
	if d.useGH && d.runGH != nil {
		out, err := d.runGH(ctx, d.host)
		v := strings.TrimSpace(out)
		switch {
		case err != nil:
			// gh missing or not logged in is the normal case: nothing to say
		case !validToken(v):
		default:
			return discoveredToken{v, "gh auth token"}, notices
		}
	}
	return discoveredToken{}, notices
}

func (d tokenDiscovery) readTokenFile() (string, error) {
	b, err := d.readFile(d.tokenFile)
	if err != nil {
		var pe *os.PathError
		if errors.As(err, &pe) {
			err = pe.Err // Not the path again
		}
		return "", fmt.Errorf("could not read the token file %s: %v", d.tokenFile, err)
	}
	if len(b) > maxTokenFileBytes {
		return "", fmt.Errorf("the token file %s is bigger than %d bytes", d.tokenFile, maxTokenFileBytes)
	}
	v := string(bytes.TrimSpace(b))
	if !validToken(v) {
		return "", fmt.Errorf("the token file %s does not hold exactly one token", d.tokenFile)
	}
	return v, nil
}

// readTokenFileLimited reads at most maxTokenFileBytes+1 bytes of a regular file.
func readTokenFileLimited(name string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	buf := make([]byte, maxTokenFileBytes+1)
	n, _ := f.Read(buf)
	return buf[:n], nil
}

// runGHAuthToken runs "gh auth token --hostname host" and returns its output. gh keeps the token in the
// system keyring on most machines, where no other program can read it. The command gets no stdin
// and cannot prompt, and its stderr is dropped because it may mention the token.
func runGHAuthToken(ctx context.Context, host string) (string, error) {
	path, err := exec.LookPath("gh")
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, ghCLITimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "auth", "token", "--hostname", host)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "NO_COLOR=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// tokenIsSafeToSend reports whether a token may be sent to the API at base: over TLS, or to a loopback
// address (a local test server or proxy).
func tokenIsSafeToSend(base *url.URL) bool {
	if base.Scheme == "https" {
		return true
	}
	h := base.Hostname()
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// redactor removes secrets from text which is printed.
type redactor struct {
	secrets []string
}

const redactedText = "[redacted]"

func (r *redactor) add(secret string) {
	if len(secret) >= 4 {
		r.secrets = append(r.secrets, secret)
	}
}

func (r *redactor) redact(s string) string {
	for _, sec := range r.secrets {
		s = strings.ReplaceAll(s, sec, redactedText)
	}
	return s
}

// redactedError is an error whose message was redacted. It keeps the chain, so errors.Is and
// errors.As (net.Error, context.Canceled) still work on it.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

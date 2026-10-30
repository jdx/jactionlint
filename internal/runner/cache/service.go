package cache

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Service is the cache service of the runner: it listens on a loopback port and speaks both generations of the protocol
// of @actions/cache, so that actions which cache (actions/cache, setup-*, Swatinem/rust-cache, jdx/mise-action) work
// without changes.
//
//   - v1 REST, under <ACTIONS_CACHE_URL>_apis/artifactcache/ (libraries up to 3.x, and the fallback of the newer ones);
//   - v2 Twirp, POST /twirp/github.actions.results.api.v1.CacheService/{CreateCacheEntry,FinalizeCacheEntryUpload,
//     GetCacheEntryDownloadURL}, whose upload goes to a "signed URL" with the three blob operations that the Azure SDK
//     uses (Put Blob, Put Block, Put Block List) and whose download is a plain GET.
//
// Both were checked against @actions/cache 3.3.0, 4.0.5 and 6.3.0 (see docs/run.md).
type Service struct {
	store *Store
	// Scopes are the scopes a restore reads, in order: the current ref, then the default branch. Saves go to Scopes[0].
	scopes []string
	token  string
	ln     net.Listener
	srv    *http.Server
	origin string

	// Log, when set, receives a line for every request (--debug).
	Log func(format string, a ...any)
}

// NewService starts a service on 127.0.0.1 with a random port and a random token.
func NewService(store *Store, scopes []string) (*Service, error) {
	if len(scopes) == 0 {
		return nil, errors.New("no cache scope")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	s := &Service{store: store, scopes: scopes, token: hex.EncodeToString(raw), ln: ln}
	s.origin = "http://" + ln.Addr().String()
	s.srv = &http.Server{Handler: s, ReadHeaderTimeout: 30 * time.Second}
	go s.srv.Serve(ln)
	return s, nil
}

// Origin is the base URL of the service, such as http://127.0.0.1:41233 (no trailing slash).
func (s *Service) Origin() string { return s.origin }

// Port is the TCP port of the service.
func (s *Service) Port() uint16 { return uint16(s.ln.Addr().(*net.TCPAddr).Port) }

// Token is what the client sends as a bearer token (ACTIONS_RUNTIME_TOKEN).
func (s *Service) Token() string { return s.token }

// Close stops the service.
func (s *Service) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.srv.Shutdown(ctx)
}

// Env returns the variables that point a step at the service.
func (s *Service) Env() map[string]string {
	env := map[string]string{
		"ACTIONS_CACHE_URL":        s.origin + "/",
		"ACTIONS_RESULTS_URL":      s.origin + "/",
		"ACTIONS_CACHE_SERVICE_V2": "true",
		"ACTIONS_RUNTIME_TOKEN":    s.token,
	}
	if s.store.ReadOnly {
		env["ACTIONS_CACHE_MODE"] = "read"
	}
	return env
}

func (s *Service) logf(format string, a ...any) {
	if s.Log != nil {
		s.Log(format, a...)
	}
}

func (s *Service) sign(kind string, id int64) string {
	m := hmac.New(sha256.New, []byte(s.token))
	fmt.Fprintf(m, "%s/%d", kind, id)
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Service) authorized(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	want := "Bearer " + s.token
	return len(h) == len(want) && subtle.ConstantTimeCompare([]byte(h), []byte(want)) == 1
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// twirpError answers in the Twirp error format. Clients parse every body as JSON and retry on text, so even errors
// must be JSON.
func twirpError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"code": code, "msg": msg})
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.logf("cache service: %s %s", r.Method, r.URL.Path)
	switch {
	case strings.HasPrefix(r.URL.Path, "/twirp/"):
		s.twirp(w, r)
	case strings.HasPrefix(r.URL.Path, "/_apis/artifactcache/"):
		s.v1(w, r)
	case strings.HasPrefix(r.URL.Path, "/_blobs/"):
		s.blob(w, r)
	default:
		twirpError(w, 404, "bad_route", "unknown route "+r.URL.Path)
	}
}

// ---- v2: Twirp

const twirpPrefix = "/twirp/github.actions.results.api.v1.CacheService/"

func (s *Service) twirp(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		twirpError(w, 401, "unauthenticated", "the runtime token is missing or wrong")
		return
	}
	if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, twirpPrefix) {
		twirpError(w, 404, "bad_route", "unknown route "+r.URL.Path)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		twirpError(w, 400, "malformed", err.Error())
		return
	}
	// The JSON client names fields like the proto does (size_bytes, restore_keys); accept camelCase too.
	var req struct {
		Key          string      `json:"key"`
		Version      string      `json:"version"`
		RestoreKeys  []string    `json:"restore_keys"`
		RestoreKeys2 []string    `json:"restoreKeys"`
		SizeBytes    json.Number `json:"size_bytes"`
		SizeBytes2   json.Number `json:"sizeBytes"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		twirpError(w, 400, "malformed", "invalid JSON: "+err.Error())
		return
	}
	switch strings.TrimPrefix(r.URL.Path, twirpPrefix) {
	case "CreateCacheEntry":
		p, err := s.store.Reserve(s.scopes[0], req.Key, req.Version)
		if err != nil {
			writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "signed_upload_url": s.blobURL("up", p.ID), "signedUploadUrl": s.blobURL("up", p.ID)})
	case "FinalizeCacheEntryUpload":
		var pend *Pending
		s.store.mu.Lock()
		for _, p := range s.store.pending {
			if p.Key == req.Key && p.Version == req.Version && p.Scope == s.scopes[0] {
				pend = p
			}
		}
		s.store.mu.Unlock()
		if pend == nil {
			writeJSON(w, 200, map[string]any{"ok": false, "message": "no upload in progress for this key"})
			return
		}
		size := int64(-1)
		for _, n := range []json.Number{req.SizeBytes, req.SizeBytes2} {
			if v, err := n.Int64(); err == nil && v > 0 {
				size = v
			}
		}
		e, err := s.store.Commit(pend, size)
		if err != nil {
			writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "entry_id": strconv.FormatInt(e.ID, 10), "entryId": strconv.FormatInt(e.ID, 10)})
	case "GetCacheEntryDownloadURL":
		restore := append(req.RestoreKeys, req.RestoreKeys2...)
		e, err := s.store.Lookup(s.scopes, req.Key, restore, req.Version)
		if err != nil {
			twirpError(w, 500, "internal", err.Error())
			return
		}
		if e == nil {
			writeJSON(w, 200, map[string]any{"ok": false})
			return
		}
		u := s.blobURL("dl", e.ID)
		writeJSON(w, 200, map[string]any{"ok": true, "signed_download_url": u, "signedDownloadUrl": u, "matched_key": e.Key, "matchedKey": e.Key})
	default:
		twirpError(w, 404, "bad_route", "unknown method")
	}
}

// blobURL is the "signed URL" of an upload or a download. The Azure SDK keeps the query string on every request, so the
// signature rides along; the path has the shape account/container/blob that the SDK expects of a loopback host.
func (s *Service) blobURL(kind string, id int64) string {
	return fmt.Sprintf("%s/_blobs/%s/%d?sig=%s", s.origin, kind, id, s.sign(kind, id))
}

func (s *Service) blobTarget(r *http.Request) (kind string, id int64, ok bool) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/_blobs/"), "/")
	if len(parts) != 2 {
		return "", 0, false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, false
	}
	sig := r.URL.Query().Get("sig")
	if !hmac.Equal([]byte(sig), []byte(s.sign(parts[0], id))) {
		return "", 0, false
	}
	return parts[0], id, true
}

// blob serves the upload (Put Blob, Put Block, Put Block List) and the download (GET) of v2, and the archive of v1.
func (s *Service) blob(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := s.blobTarget(r)
	if !ok {
		twirpError(w, 403, "permission_denied", "bad signature")
		return
	}
	switch kind {
	case "dl":
		e := s.entryByID(id)
		if e == nil {
			twirpError(w, 404, "not_found", "no such cache")
			return
		}
		f, err := os.Open(s.store.Path(e))
		if err != nil {
			twirpError(w, 404, "not_found", "the archive is gone")
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, "", e.Created, f)
	case "up":
		p := s.store.Pending(id)
		if p == nil {
			twirpError(w, 404, "not_found", "no such upload")
			return
		}
		if r.Method != http.MethodPut {
			twirpError(w, 405, "bad_route", "only PUT")
			return
		}
		s.putBlob(w, r, p)
	default:
		twirpError(w, 404, "not_found", "unknown blob")
	}
}

func (s *Service) entryByID(id int64) *Entry {
	es, err := s.store.List()
	if err != nil {
		return nil
	}
	for _, e := range es {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func azureHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("ETag", `"0x8DC0000000000"`)
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	w.Header().Set("x-ms-request-id", "00000000-0000-0000-0000-000000000000")
	w.Header().Set("x-ms-version", r.Header.Get("x-ms-version"))
	w.Header().Set("x-ms-request-server-encrypted", "true")
}

func (s *Service) putBlob(w http.ResponseWriter, r *http.Request, p *Pending) {
	q := r.URL.Query()
	switch q.Get("comp") {
	case "": // Put Blob: archives up to 128 MiB
		if r.Header.Get("x-ms-blob-type") != "BlockBlob" {
			twirpError(w, 400, "malformed", "x-ms-blob-type must be BlockBlob")
			return
		}
		if err := writeBody(p.Path, r.Body, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0); err != nil {
			twirpError(w, 500, "internal", err.Error())
			return
		}
	case "block": // Put Block: the parts of a larger archive, concurrently and in any order
		id := q.Get("blockid")
		if id == "" || len(id) > 256 {
			twirpError(w, 400, "malformed", "blockid")
			return
		}
		if err := os.MkdirAll(p.Path+".blocks", 0o755); err != nil {
			twirpError(w, 500, "internal", err.Error())
			return
		}
		if err := writeBody(filepath.Join(p.Path+".blocks", url.PathEscape(id)), r.Body, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0); err != nil {
			twirpError(w, 500, "internal", err.Error())
			return
		}
	case "blocklist": // Put Block List: assemble the parts in the order given
		var bl struct {
			Items []struct {
				XMLName xml.Name
				ID      string `xml:",chardata"`
			} `xml:",any"`
		}
		b, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err := xml.Unmarshal(b, &bl); err != nil {
			twirpError(w, 400, "malformed", "block list: "+err.Error())
			return
		}
		out, err := os.OpenFile(p.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			twirpError(w, 500, "internal", err.Error())
			return
		}
		for _, it := range bl.Items {
			part, err := os.Open(filepath.Join(p.Path+".blocks", url.PathEscape(strings.TrimSpace(it.ID))))
			if err != nil {
				out.Close()
				twirpError(w, 400, "malformed", "InvalidBlockList: block "+it.ID+" was not uploaded")
				return
			}
			_, err = io.Copy(out, part)
			part.Close()
			if err != nil {
				out.Close()
				twirpError(w, 500, "internal", err.Error())
				return
			}
		}
		if err := out.Close(); err != nil {
			twirpError(w, 500, "internal", err.Error())
			return
		}
		os.RemoveAll(p.Path + ".blocks")
	default:
		twirpError(w, 400, "malformed", "unsupported blob operation "+q.Get("comp"))
		return
	}
	azureHeaders(w, r)
	w.WriteHeader(http.StatusCreated)
}

// writeBody writes the request body to a file at an offset.
func writeBody(path string, body io.Reader, flag int, off int64) error {
	f, err := os.OpenFile(path, flag, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(io.NewOffsetWriter(f, off), body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ---- v1: REST

func (s *Service) v1(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		twirpError(w, 401, "unauthenticated", "the runtime token is missing or wrong")
		return
	}
	res := strings.TrimPrefix(r.URL.Path, "/_apis/artifactcache/")
	q := r.URL.Query()
	switch {
	case res == "cache" && r.Method == http.MethodGet:
		keys := strings.Split(q.Get("keys"), ",")
		e, err := s.store.Lookup(s.scopes, keys[0], keys[1:], q.Get("version"))
		if err != nil {
			twirpError(w, 500, "internal", err.Error())
			return
		}
		if e == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, 200, map[string]any{"cacheKey": e.Key, "scope": e.Scope, "archiveLocation": s.blobURL("dl", e.ID)})
	case res == "caches" && r.Method == http.MethodGet: // listing, used for diagnostics when debugging
		writeJSON(w, 200, map[string]any{"totalCount": 0, "artifactCaches": []any{}})
	case res == "caches" && r.Method == http.MethodPost:
		var req struct {
			Key       string `json:"key"`
			Version   string `json:"version"`
			CacheSize int64  `json:"cacheSize"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			twirpError(w, 400, "malformed", err.Error())
			return
		}
		p, err := s.store.Reserve(s.scopes[0], req.Key, req.Version)
		if err != nil {
			code := http.StatusConflict
			if errors.Is(err, ErrReadOnly) {
				code = http.StatusForbidden
			}
			twirpError(w, code, "already_exists", err.Error())
			return
		}
		writeJSON(w, 201, map[string]any{"cacheId": p.ID})
	case strings.HasPrefix(res, "caches/") && (r.Method == http.MethodPatch || r.Method == http.MethodPost):
		id, err := strconv.ParseInt(strings.TrimPrefix(res, "caches/"), 10, 64)
		p := s.store.Pending(id)
		if err != nil || p == nil {
			twirpError(w, 404, "not_found", "no such upload")
			return
		}
		if r.Method == http.MethodPatch {
			// chunks arrive concurrently and out of order: write at the offset of Content-Range
			var from, to int64
			if _, err := fmt.Sscanf(r.Header.Get("Content-Range"), "bytes %d-%d/", &from, &to); err != nil || from < 0 || to < from {
				twirpError(w, 400, "malformed", "Content-Range")
				return
			}
			if err := writeBody(p.Path, io.LimitReader(r.Body, to-from+1), os.O_CREATE|os.O_WRONLY, from); err != nil {
				twirpError(w, 500, "internal", err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var req struct {
			Size int64 `json:"size"`
		}
		json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
		if _, err := s.store.Commit(p, req.Size); err != nil {
			twirpError(w, 400, "malformed", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		twirpError(w, 404, "bad_route", "unknown route "+r.URL.Path)
	}
}

package cache

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// These tests speak the protocols the way @actions/cache does (see docs/run.md for the versions checked with the real
// library): JSON bodies with proto field names, Put Blob / Put Block / Put Block List against the signed URL, and
// chunks that arrive out of order with Content-Range.

func newService(t *testing.T, readOnly bool) (*Service, *Store) {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.ReadOnly = readOnly
	svc, err := NewService(st, []string{"refs/heads/main"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	return svc, st
}

func do(t *testing.T, method, u string, token string, body []byte, hdr map[string]string) (int, []byte, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(method, u, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, resp.Header
}

func rpc(t *testing.T, svc *Service, method string, req any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(req)
	code, body, _ := do(t, "POST", svc.Origin()+twirpPrefix+method, svc.Token(), b, map[string]string{"Content-Type": "application/json"})
	if code != 200 {
		t.Fatalf("%s: %d %s", method, code, body)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("%s answered %q, which is not JSON", method, body)
	}
	return out
}

func TestV2RoundTripSmall(t *testing.T) {
	svc, _ := newService(t, false)
	create := rpc(t, svc, "CreateCacheEntry", map[string]string{"key": "k1", "version": "ver"})
	if create["ok"] != true {
		t.Fatalf("%v", create)
	}
	up := create["signedUploadUrl"].(string)
	// the SDK keeps the query string and adds its own
	code, _, _ := do(t, "PUT", up, "", []byte("archive-bytes"), map[string]string{"x-ms-blob-type": "BlockBlob", "x-ms-version": "2024-11-04"})
	if code != 201 {
		t.Fatalf("Put Blob: %d", code)
	}
	fin := rpc(t, svc, "FinalizeCacheEntryUpload", map[string]string{"key": "k1", "size_bytes": "13", "version": "ver"})
	if fin["ok"] != true {
		t.Fatalf("%v", fin)
	}
	// the same key again: the client prints "Unable to reserve cache"
	if again := rpc(t, svc, "CreateCacheEntry", map[string]string{"key": "k1", "version": "ver"}); again["ok"] != false {
		t.Fatalf("a saved key must not be reserved again: %v", again)
	}
	get := rpc(t, svc, "GetCacheEntryDownloadURL", map[string]any{"key": "k1-miss", "restore_keys": []string{"k"}, "version": "ver"})
	if get["ok"] != true || get["matchedKey"] != "k1" {
		t.Fatalf("%v", get)
	}
	code, body, _ := do(t, "GET", get["signedDownloadUrl"].(string), "", nil, nil)
	if code != 200 || string(body) != "archive-bytes" {
		t.Fatalf("download: %d %q", code, body)
	}
	if miss := rpc(t, svc, "GetCacheEntryDownloadURL", map[string]any{"key": "other", "version": "ver"}); miss["ok"] != false {
		t.Fatalf("%v", miss)
	}
	if wrong := rpc(t, svc, "GetCacheEntryDownloadURL", map[string]any{"key": "k1", "version": "other-version"}); wrong["ok"] != false {
		t.Fatalf("a different version must not match: %v", wrong)
	}
}

func TestV2BlocksOutOfOrder(t *testing.T) {
	svc, _ := newService(t, false)
	create := rpc(t, svc, "CreateCacheEntry", map[string]string{"key": "big", "version": "v"})
	up := create["signed_upload_url"].(string)
	parts := map[string]string{"AAA=": "first-", "BBB=": "second-", "CCC=": "third"}
	// uploaded in a different order than the block list
	for _, id := range []string{"CCC=", "AAA=", "BBB="} {
		code, _, _ := do(t, "PUT", up+"&comp=block&blockid="+url.QueryEscape(id), "", []byte(parts[id]), nil)
		if code != 201 {
			t.Fatalf("Put Block %s: %d", id, code)
		}
	}
	list := `<?xml version="1.0" encoding="utf-8"?><BlockList><Latest>AAA=</Latest><Latest>BBB=</Latest><Latest>CCC=</Latest></BlockList>`
	if code, _, _ := do(t, "PUT", up+"&comp=blocklist", "", []byte(list), nil); code != 201 {
		t.Fatalf("Put Block List: %d", code)
	}
	size := len("first-second-third")
	if fin := rpc(t, svc, "FinalizeCacheEntryUpload", map[string]any{"key": "big", "sizeBytes": fmt.Sprint(size), "version": "v"}); fin["ok"] != true {
		t.Fatalf("%v", fin)
	}
	get := rpc(t, svc, "GetCacheEntryDownloadURL", map[string]any{"key": "big", "version": "v"})
	_, body, _ := do(t, "GET", get["signedDownloadUrl"].(string), "", nil, nil)
	if string(body) != "first-second-third" {
		t.Fatalf("assembled %q", body)
	}
	// an unknown block in the list is refused
	create = rpc(t, svc, "CreateCacheEntry", map[string]string{"key": "bad", "version": "v"})
	up = create["signedUploadUrl"].(string)
	if code, _, _ := do(t, "PUT", up+"&comp=blocklist", "", []byte(`<BlockList><Latest>nope</Latest></BlockList>`), nil); code != 400 {
		t.Fatalf("unknown block: %d", code)
	}
	// a size that disagrees with the upload does not commit
	create = rpc(t, svc, "CreateCacheEntry", map[string]string{"key": "short", "version": "v"})
	do(t, "PUT", create["signedUploadUrl"].(string), "", []byte("abc"), map[string]string{"x-ms-blob-type": "BlockBlob"})
	if fin := rpc(t, svc, "FinalizeCacheEntryUpload", map[string]string{"key": "short", "size_bytes": "999", "version": "v"}); fin["ok"] != false {
		t.Fatalf("%v", fin)
	}
}

func TestAuthAndSignatures(t *testing.T) {
	svc, _ := newService(t, false)
	// every answer is JSON, because the client retries on a body that is not
	for _, tc := range []struct {
		method, path, token string
		want                int
	}{
		{"POST", twirpPrefix + "CreateCacheEntry", "", 401},
		{"POST", twirpPrefix + "CreateCacheEntry", "wrong", 401},
		{"POST", "/twirp/other.Service/CreateCacheEntry", svc.Token(), 404},
		{"GET", "/_apis/artifactcache/cache?keys=k&version=v", "", 401},
		{"GET", "/_apis/artifactcache/cache?keys=k&version=v", svc.Token(), 204},
		{"GET", "/nope", "", 404},
		{"GET", "/_blobs/dl/1?sig=bad", "", 403},
		{"PUT", "/_blobs/up/1", "", 403},
	} {
		code, body, hdr := do(t, tc.method, svc.Origin()+tc.path, tc.token, []byte("{}"), nil)
		if code != tc.want {
			t.Errorf("%s %s (token %q): %d, want %d", tc.method, tc.path, tc.token, code, tc.want)
		}
		if code != 204 && !json.Valid(body) {
			t.Errorf("%s %s: the body %q is not JSON (content-type %q)", tc.method, tc.path, body, hdr.Get("Content-Type"))
		}
	}
	// a signature for one id does not work for another, or for the other direction
	if code, _, _ := do(t, "GET", svc.Origin()+"/_blobs/dl/2?sig="+svc.sign("dl", 1), "", nil, nil); code != 403 {
		t.Errorf("a signature of another id was accepted: %d", code)
	}
	if code, _, _ := do(t, "GET", svc.Origin()+"/_blobs/dl/1?sig="+svc.sign("up", 1), "", nil, nil); code != 403 {
		t.Errorf("an upload signature downloaded: %d", code)
	}
	env := svc.Env()
	if !strings.HasSuffix(env["ACTIONS_CACHE_URL"], "/") || env["ACTIONS_CACHE_SERVICE_V2"] != "true" || env["ACTIONS_RUNTIME_TOKEN"] != svc.Token() {
		t.Errorf("env = %v", env)
	}
}

func TestV1RoundTrip(t *testing.T) {
	svc, _ := newService(t, false)
	base := svc.Origin() + "/_apis/artifactcache/"
	code, body, _ := do(t, "POST", base+"caches", svc.Token(), []byte(`{"key":"v1key","version":"ver","cacheSize":10}`), nil)
	if code != 201 {
		t.Fatalf("reserve: %d %s", code, body)
	}
	var r struct{ CacheID int64 }
	json.Unmarshal(body, &r)
	// two chunks, the second one first, as the client sends them concurrently
	patch := func(rng, data string) {
		c, b, _ := do(t, "PATCH", fmt.Sprintf("%scaches/%d", base, r.CacheID), svc.Token(), []byte(data), map[string]string{"Content-Type": "application/octet-stream", "Content-Range": rng})
		if c != 204 {
			t.Fatalf("PATCH %s: %d %s", rng, c, b)
		}
	}
	patch("bytes 5-9/*", "WORLD")
	patch("bytes 0-4/*", "HELLO")
	if c, b, _ := do(t, "POST", fmt.Sprintf("%scaches/%d", base, r.CacheID), svc.Token(), []byte(`{"size":10}`), nil); c != 204 {
		t.Fatalf("commit: %d %s", c, b)
	}
	if c, _, _ := do(t, "POST", base+"caches", svc.Token(), []byte(`{"key":"v1key","version":"ver","cacheSize":10}`), nil); c != 409 {
		t.Fatalf("a saved key must conflict, got %d", c)
	}
	code, body, _ = do(t, "GET", base+"cache?keys="+url.QueryEscape("miss,v1")+"&version=ver", svc.Token(), nil, nil)
	if code != 200 {
		t.Fatalf("lookup: %d %s", code, body)
	}
	var hit struct {
		CacheKey        string `json:"cacheKey"`
		ArchiveLocation string `json:"archiveLocation"`
	}
	json.Unmarshal(body, &hit)
	if hit.CacheKey != "v1key" {
		t.Fatalf("%+v", hit)
	}
	if _, data, _ := do(t, "GET", hit.ArchiveLocation, "", nil, nil); string(data) != "HELLOWORLD" {
		t.Fatalf("download %q", data)
	}
	if code, _, _ := do(t, "GET", base+"cache?keys=none&version=ver", svc.Token(), nil, nil); code != 204 {
		t.Fatalf("a miss must be 204, got %d", code)
	}
}

func TestReadOnlyService(t *testing.T) {
	svc, st := newService(t, true)
	if c, _, _ := do(t, "POST", svc.Origin()+"/_apis/artifactcache/caches", svc.Token(), []byte(`{"key":"k","version":"v","cacheSize":1}`), nil); c != 403 {
		t.Fatalf("v1 reserve on a read-only cache: %d", c)
	}
	if got := rpc(t, svc, "CreateCacheEntry", map[string]string{"key": "k", "version": "v"}); got["ok"] != false {
		t.Fatalf("%v", got)
	}
	if svc.Env()["ACTIONS_CACHE_MODE"] != "read" {
		t.Error("clients are told the cache is read-only")
	}
	_ = st
}

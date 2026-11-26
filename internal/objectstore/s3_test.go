package objectstore

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// s3Server is a server of the tests: it answers the listing, the objects,
// and checks that every request is signed.
func s3Server(t *testing.T, pathStyle bool) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	keys := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AK/") {
			t.Errorf("%s %s: not signed: %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("X-Amz-Date") == "" || r.Header.Get("X-Amz-Content-Sha256") == "" {
			t.Error("the signature's headers are missing")
		}
		mu.Lock()
		keys = append(keys, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
			prefix := r.URL.Query().Get("prefix")
			io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <IsTruncated>false</IsTruncated>
  <Contents><Key>`+prefix+`a.txt</Key><LastModified>2026-01-02T03:04:05Z</LastModified><Size>12</Size></Contents>
  <Contents><Key>`+prefix+`b.txt</Key><LastModified>2026-01-02T03:04:05Z</LastModified><Size>3</Size></Contents>
  <CommonPrefixes><Prefix>`+prefix+`docs/</Prefix></CommonPrefixes>
</ListBucketResult>`)
		case r.Method == http.MethodGet:
			io.WriteString(w, "hello")
		case r.Method == http.MethodPut:
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete, r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &keys
}

// A bucket lists the folders its keys share and the objects under the
// prefix, signs every request, and writes, reads and deletes the keys the
// panel names.
func TestS3Store(t *testing.T) {
	srv, seen := s3Server(t, true)
	s, err := NewS3(S3Config{
		Endpoint: srv.URL, Region: "us-east-1", Bucket: "cache",
		AccessKey: "AK", SecretKey: "SK", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	ctx := context.Background()
	entries, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries %+v", entries)
	}
	if !entries[2].Dir || entries[2].Key != "docs" {
		t.Errorf("the folder %+v", entries[2])
	}
	if entries[0].Key != "a.txt" || entries[0].Size != 12 || entries[0].ModTime.IsZero() {
		t.Errorf("the object %+v", entries[0])
	}
	// The bucket is in the path, as the path style asks.
	if got := (*seen)[0]; !strings.HasPrefix(got, "GET /cache/?") {
		t.Errorf("the listing went to %q", got)
	}

	if err := s.Put(ctx, "dir/a.txt", strings.NewReader("hello"), 5, "text/plain", nil); err != nil {
		t.Fatalf("%v", err)
	}
	r, err := s.Get(ctx, "dir/a.txt")
	if err != nil {
		t.Fatalf("%v", err)
	}
	b, _ := io.ReadAll(r)
	r.Close()
	if string(b) != "hello" {
		t.Errorf("read %q", b)
	}
	if err := s.Delete(ctx, "dir/a.txt", false); err != nil {
		t.Fatalf("%v", err)
	}
	// A folder is a key of no bytes, ending with the slash, as the consoles
	// make one.
	if err := s.Mkdir(ctx, "folder"); err != nil {
		t.Errorf("mkdir: %v", err)
	}
	found := false
	for _, got := range *seen {
		if strings.HasPrefix(got, "PUT /cache/folder/?") {
			found = true
		}
	}
	if !found {
		t.Errorf("the folder's key is not written: %v", *seen)
	}
	for _, got := range *seen {
		if strings.Contains(got, "docs/") && strings.HasPrefix(got, "PUT") {
			t.Errorf("a listing was written: %q", got)
		}
	}
}

// A bucket is in the host name of an endpoint that does not ask for the
// path style, as the clouds' endpoints have it.
func TestS3VirtualHost(t *testing.T) {
	s, err := NewS3(S3Config{
		Endpoint: "https://oss-cn-hangzhou.aliyuncs.com", Bucket: "cache",
		AccessKey: "AK", SecretKey: "SK",
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	req, err := s.request(http.MethodGet, "dir/a b.txt", nil, emptySHA, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if req.URL.Host != "cache.oss-cn-hangzhou.aliyuncs.com" {
		t.Errorf("host %q", req.URL.Host)
	}
	if got := req.URL.EscapedPath(); got != "/dir/a%20b.txt" {
		t.Errorf("path %q", got)
	}
	if !strings.Contains(req.Header.Get("Authorization"), "SignedHeaders=host;x-amz-content-sha256;x-amz-date") {
		t.Errorf("authorization %q", req.Header.Get("Authorization"))
	}
}

// A path-style endpoint puts the bucket in the path, as MinIO wants it.
func TestS3PathStyle(t *testing.T) {
	srv, seen := s3Server(t, true)
	s, err := NewS3(S3Config{
		Endpoint: srv.URL, Region: "us-east-1", Bucket: "cache",
		AccessKey: "AK", SecretKey: "SK", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := s.List(context.Background(), "backups"); err != nil {
		t.Fatalf("%v", err)
	}
	if got := (*seen)[0]; !strings.HasPrefix(got, "GET /cache/?") || !strings.Contains(got, "prefix=backups%2F") {
		t.Errorf("the listing went to %q", got)
	}
}

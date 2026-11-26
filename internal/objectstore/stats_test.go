package objectstore

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// A directory answers a probe when it is there.
func TestLocalProbe(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenLocal(dir)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := s.Probe(context.Background()); err != nil {
		t.Errorf("probe: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err == nil {
		t.Error("the directory has a file it never got")
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := s.Probe(context.Background()); err == nil {
		t.Error("a directory that is gone probes fine")
	}
}

// A bucket answers a probe with a HEAD of it, which the monitor counts.
func TestS3Probe(t *testing.T) {
	var probes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("the probe is a %s", r.Method)
		}
		probes++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	s, err := NewS3(S3Config{
		Endpoint: srv.URL, Region: "us-east-1", Bucket: "cache",
		AccessKey: "AK", SecretKey: "SK", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := s.Probe(context.Background()); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if probes != 1 {
		t.Errorf("the probe was %d HEADs", probes)
	}
}

// A status the server refused with is a StatusError, which the monitor
// counts by code; a request that ran out of time is a timeout.
func TestStatusErrors(t *testing.T) {
	if status, ok := HTTPStatus(&StatusError{Status: 403}); !ok || status != 403 {
		t.Errorf("the status %d, %v", status, ok)
	}
	if status, ok := HTTPStatus(ErrNotFound); !ok || status != 404 {
		t.Errorf("not found: %d, %v", status, ok)
	}
	if _, ok := HTTPStatus(io.EOF); ok {
		t.Error("a local failure has a status")
	}
	if !IsTimeout(context.DeadlineExceeded) {
		t.Error("a request out of time is not a timeout")
	}
	if IsTimeout(io.EOF) {
		t.Error("a local failure is a timeout")
	}
}

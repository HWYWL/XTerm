package objectstore

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A directory lists its folders first, writes and reads a file, makes a
// folder and removes what it holds, as the storages do.
func TestLocalStore(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backups", "2026"), 0o755); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
	s, err := OpenLocal(dir)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ctx := context.Background()
	entries, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(entries) != 2 || !entries[0].Dir || entries[0].Name != "backups" || entries[1].Name != "notes.txt" {
		t.Fatalf("entries %+v", entries)
	}
	if entries[1].Size != 2 {
		t.Errorf("size %d", entries[1].Size)
	}

	// Write, read back, and see it in the listing.
	if err := s.Put(ctx, "backups/2026/a.txt", strings.NewReader("hello"), 5, "text/plain", nil); err != nil {
		t.Fatalf("%v", err)
	}
	r, err := s.Get(ctx, "backups/2026/a.txt")
	if err != nil {
		t.Fatalf("%v", err)
	}
	b, _ := io.ReadAll(r)
	r.Close()
	if string(b) != "hello" {
		t.Errorf("read %q", b)
	}
	if e, err := s.Stat(ctx, "backups/2026/a.txt"); err != nil || e.Size != 5 {
		t.Errorf("stat %+v, %v", e, err)
	}

	// A folder is made, and removing it removes what it holds.
	if err := s.Mkdir(ctx, "logs"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := s.Delete(ctx, "backups", true); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups")); !os.IsNotExist(err) {
		t.Errorf("the folder is still there: %v", err)
	}
}

// A storage seen from a root shows the keys under it and nothing of the
// path to them.
func TestWithRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "team", "docs"), 0o755); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "team", "docs", "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
	inner, err := OpenLocal(dir)
	if err != nil {
		t.Fatalf("%v", err)
	}
	s := WithRoot(inner, "/team/")
	ctx := context.Background()
	entries, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(entries) != 1 || entries[0].Name != "docs" || entries[0].Key != "docs" {
		t.Fatalf("entries %+v", entries)
	}
	if e, err := s.Stat(ctx, "docs/a.md"); err != nil || e.Key != "docs/a.md" {
		t.Errorf("stat %+v, %v", e, err)
	}
	if _, err := s.Get(ctx, "outside.txt"); err == nil {
		t.Error("a key outside the root was read")
	}
}

// A key is cleaned of the slashes around it and of the dots in it, so that
// the panel cannot reach above the root with "..".
func TestCleanKey(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""}, {"/a/b/", "a/b"}, {"a//b", "a/b"}, {"../x", "x"}, {"a/../b", "b"},
	} {
		if got := CleanKey(tc.in); got != tc.want {
			t.Errorf("CleanKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := JoinKey("a", "b"); got != "a/b" {
		t.Errorf("JoinKey %q", got)
	}
	if got := BaseKey("a/b/"); got != "b" {
		t.Errorf("BaseKey %q", got)
	}
}

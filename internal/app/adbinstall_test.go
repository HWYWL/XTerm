package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// A .xapk or .apks bundle is unpacked to its APKs, and the folder goes
// after the install.
func TestExpandAPKs(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "app.xapk")
	f, err := os.Create(bundle)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range []string{"base.apk", "config.arm64_v8a.apk", "manifest.json"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("apk"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	apks, cleanup, err := expandAPKs([]string{bundle})
	if err != nil {
		t.Fatal(err)
	}
	if len(apks) != 2 {
		t.Fatalf("%d APKs: %v", len(apks), apks)
	}
	for _, p := range apks {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	folder := filepath.Dir(apks[0])
	cleanup()
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Errorf("the folder stayed: %v", err)
	}
}

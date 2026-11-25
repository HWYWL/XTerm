package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/internal/record"
	"finallyshell/locales"
)

// TestSessionRecording: starting a transcript writes its header, the
// commands submitted are noted, and stopping it closes the file; the
// window lists it.
func TestSessionRecording(t *testing.T) {
	t.Setenv("FINALLYSHELL_DIR", t.TempDir())
	a, _, s, _, tt := processesApp(t)
	s.Host = &config.Host{Name: "web-1", Addr: "10.0.0.1", Port: 22, User: "root"}
	s.status = statusConnected

	if !a.startRecording(s) {
		t.Fatal("the recording did not start")
	}
	if !a.recording(s) {
		t.Error("not recording")
	}
	a.recordCommand(s, "ls -la")
	a.stopRecording(s, "")
	if a.recording(s) {
		t.Error("still recording")
	}

	root := record.Dir(config.Dir())
	var path string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			path = p
		}
		return nil
	})
	if path == "" {
		t.Fatal("no transcript was written")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if text := string(data); !strings.Contains(text, "[COMMAND] ls -la") || !strings.Contains(text, "Host: root@10.0.0.1") {
		t.Errorf("transcript:\n%s", text)
	}

	// The window lists it.
	a.openRecFiles()
	tt.Frame()
	if len(a.records.files.items) != 1 {
		t.Errorf("%d files", len(a.records.files.items))
	}
	if !tt.HasText(locales.RecordFiles()) {
		t.Errorf("the window is not shown; texts %q", tt.Texts())
	}
	a.records.files.open = false
}

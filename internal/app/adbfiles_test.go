package app

import (
	"io/fs"
	"testing"
	"time"

	"finallyshell/internal/adb"
)

// A device's entry reaches the file panel with its mode and time, so that
// folders sort first and the permissions column reads as the device's.
func TestADBEntry(t *testing.T) {
	at := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	dir := adbEntry(adb.Entry{Name: "sub", Dir: true, Mode: fs.ModeDir | 0o750, ModTime: at, Size: 4096})
	if dir.Name != "sub" || !dir.IsDir() || dir.Mode.Perm() != 0o750 || dir.Size != 4096 || !dir.ModTime.Equal(at) {
		t.Errorf("the folder is %+v", dir)
	}
	link := adbEntry(adb.Entry{Name: "link", Mode: fs.ModeSymlink | 0o777})
	if !link.Link || link.IsDir() || link.Broken {
		t.Errorf("the link is %+v", link)
	}
}

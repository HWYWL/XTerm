package adb

// Android's platform-tools, the archive adb comes in. A device on a cable
// is reached through it on Windows and macOS, so the app fetches it when it
// is missing rather than sending the user to a download page.

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The archives Google publishes, by system.
const (
	toolsWindows = "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
	toolsDarwin  = "https://dl.google.com/android/repository/platform-tools-latest-darwin.zip"
	toolsLinux   = "https://dl.google.com/android/repository/platform-tools-latest-linux.zip"

	// toolsRoot is the folder the archives hold everything in.
	toolsRoot = "platform-tools"
)

// ToolArchive is the platform-tools archive of this system, and the name of
// adb in it.
func ToolArchive() (url, name string, err error) {
	switch runtime.GOOS {
	case "windows":
		return toolsWindows, "adb.exe", nil
	case "darwin":
		return toolsDarwin, "adb", nil
	case "linux":
		return toolsLinux, "adb", nil
	}
	return "", "", fmt.Errorf("adb: no platform-tools for %s", runtime.GOOS)
}

// Stage is part of a download that a progress report is about.
type Stage int

const (
	// StageDownload is the archive coming down the network.
	StageDownload Stage = iota
	// StageUnpack is the archive being unpacked.
	StageUnpack
)

// Progress is how far the download has come: the stage, and the bytes of
// the archive for the stage that counts them.
type Progress struct {
	Stage       Stage
	Done, Total int64
}

// InstallTools fetches platform-tools and unpacks them into dir, under
// their usual platform-tools folder, reporting its progress as it goes. It
// returns the path of the adb in them.
func InstallTools(ctx context.Context, dir string, progress func(Progress)) (string, error) {
	url, name, err := ToolArchive()
	if err != nil {
		return "", err
	}
	if progress == nil {
		progress = func(Progress) {}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("adb: platform-tools: HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "platform-tools-*.zip")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	total, done := resp.ContentLength, int64(0)
	buf := make([]byte, 64<<10)
	last := time.Time{}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := tmp.Write(buf[:n]); err != nil {
				return "", err
			}
			done += int64(n)
			if time.Since(last) >= 100*time.Millisecond || total > 0 && done >= total {
				progress(Progress{Stage: StageDownload, Done: done, Total: total})
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	progress(Progress{Stage: StageDownload, Done: done, Total: total})
	if total > 0 && done != total {
		return "", errors.New("adb: the download of platform-tools is incomplete")
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	progress(Progress{Stage: StageUnpack})
	target := filepath.Join(dir, toolsRoot)
	if err := unzipTools(tmp.Name(), target); err != nil {
		return "", err
	}
	adb := filepath.Join(target, name)
	if st, err := os.Stat(adb); err != nil || st.IsDir() {
		return "", errors.New("adb: the archive holds no adb")
	}
	return adb, nil
}

// unzipTools unpacks an archive of platform-tools into dest, keeping every
// file of it inside dest.
func unzipTools(path, dest string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	root := dest + string(os.PathSeparator)
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, toolsRoot+"/") {
			continue
		}
		rel := strings.TrimPrefix(f.Name, toolsRoot+"/")
		if rel == "" {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if !strings.HasPrefix(target, root) {
			return fmt.Errorf("adb: the archive holds %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if f.Mode()&os.ModeSymlink != 0 {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			link, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}
			if runtime.GOOS == "windows" {
				continue // a link is a copy there, and none is in the archive
			}
			if err := os.Symlink(string(link), target); err != nil {
				return err
			}
			continue
		}
		if err := unzipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

// unzipFile writes one file of the archive, keeping what it says of its
// mode where the system has modes.
func unzipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	mode := f.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	if runtime.GOOS != "windows" && mode&0o111 == 0 {
		mode |= 0o755 // adb is run; the rest does no harm
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

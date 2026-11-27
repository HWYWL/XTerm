package adb

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// toolsZip is an archive of platform-tools that holds files: adb (which a
// system runs) and a folder.
func toolsZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create("platform-tools/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// installFrom serves archive at the address of the system's archive, and
// installs it into a folder of its own. The URL of a system is not a
// variable; the request is answered whatever it asks for.
func installFrom(t *testing.T, archive []byte) (string, []Progress, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", itoa(len(archive)))
		w.WriteHeader(http.StatusOK)
		w.Write(archive)
	}))
	t.Cleanup(srv.Close)
	oldTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = rewriteTransport{base: http.DefaultTransport, url: srv.URL}
	t.Cleanup(func() { http.DefaultClient.Transport = oldTransport })
	junk := t.TempDir()
	var seen []Progress
	path, err := InstallTools(context.Background(), junk, func(p Progress) { seen = append(seen, p) })
	return path, seen, err
}

// rewriteTransport sends every request to url, whichever host the request
// names: the archive's own address is a constant.
type rewriteTransport struct {
	base http.RoundTripper
	url  string
}

func (t rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, t.url+r.URL.Path, r.Body)
	if err != nil {
		return nil, err
	}
	req.Header = r.Header
	return t.base.RoundTrip(req)
}

// The archive is downloaded and unpacked, adb is where the app looks for
// it, and the progress says how far it came.
func TestInstallTools(t *testing.T) {
	name := "adb"
	if runtime.GOOS == "windows" {
		name = "adb.exe"
	}
	archive := toolsZip(t, map[string]string{name: "binary", "source.properties": "x"})
	path, seen, err := installFrom(t, archive)
	if err != nil {
		t.Fatalf("the install: %v", err)
	}
	if filepath.Base(path) != name {
		t.Errorf("adb at %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("adb was not written: %v", err)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode()&0o111 == 0 {
			t.Errorf("adb is not runnable: %v", st.Mode())
		}
	}
	var downloads, unpacks int
	for _, p := range seen {
		switch p.Stage {
		case StageDownload:
			downloads++
			if p.Total != int64(len(archive)) {
				t.Errorf("the download is %d of %d bytes", p.Done, p.Total)
			}
		case StageUnpack:
			unpacks++
		}
	}
	if downloads == 0 || unpacks == 0 {
		t.Errorf("the progress %+v", seen)
	}
	if seen[len(seen)-1].Stage != StageUnpack {
		t.Errorf("the last report is %+v", seen[len(seen)-1])
	}
}

// An archive that would write outside its folder is refused, and nothing of
// it is written.
func TestInstallToolsRejectsEscapes(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("platform-tools/../../evil")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("x"))
	zw.Close()
	_, _, err = installFrom(t, buf.Bytes())
	if err == nil {
		t.Fatal("an archive writing outside its folder was taken")
	}
}

// An archive with no adb in it is not an archive of platform-tools.
func TestInstallToolsNeedsAdb(t *testing.T) {
	archive := toolsZip(t, map[string]string{"readme.txt": "no adb here"})
	if _, _, err := installFrom(t, archive); err == nil {
		t.Fatal("an archive without adb was taken")
	}
}

// Every system the app is built for has an archive.
func TestToolArchive(t *testing.T) {
	url, name, err := ToolArchive()
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		if err != nil || url == "" || name == "" {
			t.Errorf("%s: %q %q %v", runtime.GOOS, url, name, err)
		}
	default:
		if err == nil {
			t.Errorf("%s has an archive after all", runtime.GOOS)
		}
	}
}

// itoa is a small decimal, without the import of strconv in the test.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

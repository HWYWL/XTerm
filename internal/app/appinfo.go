package app

// App labels and icons for the apps tab: an APK's manifest and resources
// carry them, and the device has need of nothing else. A bundle's small
// files are fetched when the device has unzip; otherwise the APK is pulled
// whole, within a size the tab will wait for. What is read is kept beside
// the settings, as an app's APK path changes with each update.

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"finallyshell/internal/adb"
	"finallyshell/internal/config"
	"finallyshell/internal/remotefs"

	"github.com/egoist/mygo/ui"
	"github.com/shogo82148/androidbinary"
	apkfile "github.com/shogo82148/androidbinary/apk"

	_ "golang.org/x/image/webp"
)

// appInfo is what the apps tab shows of an app beside its package: the
// name it calls itself, and its icon.
type appInfo struct {
	Label string
	Icon  *ui.Bitmap
	PNG   []byte // the icon as it is kept on disk
}

// appInfoSem limits how many APKs are read at once: each is a transfer of
// its own, and an app tab of many apps must not swamp the cable.
var appInfoSem = make(chan struct{}, 2)

// appInfoMaxPull is the largest APK pulled whole when the device has no
// unzip: a game of hundreds of megabytes keeps its package name.
const appInfoMaxPull = 80 << 20

// appInfoKey identifies the APK whose label and icon are read.
func appInfoKey(app adbApp) string { return app.Package + "|" + app.Path }

// appInfoFile is the cache's name for a key.
func appInfoFile(key string) string {
	sum := sha1.Sum([]byte(key))
	return hex.EncodeToString(sum[:])
}

// appInfoDir is where the labels and icons read are kept.
func appInfoDir() string { return filepath.Join(config.Dir(), "appinfo") }

// loadAppInfo reads a cached label and icon.
func loadAppInfo(key string) (*appInfo, bool) {
	name := filepath.Join(appInfoDir(), appInfoFile(key))
	data, err := os.ReadFile(name + ".json")
	if err != nil {
		return nil, false
	}
	var v struct {
		Label string `json:"label"`
	}
	if json.Unmarshal(data, &v) != nil {
		return nil, false
	}
	info := &appInfo{Label: v.Label}
	if png, err := os.ReadFile(name + ".png"); err == nil {
		if img, err := ui.DecodeBitmap(png); err == nil {
			info.Icon, info.PNG = img, png
		}
	}
	return info, true
}

// saveAppInfo keeps a label and icon for the next run.
func saveAppInfo(key string, info *appInfo) {
	if info == nil || info.Label == "" && info.PNG == nil {
		return
	}
	dir := appInfoDir()
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	name := filepath.Join(dir, appInfoFile(key))
	data, err := json.Marshal(struct {
		Label string `json:"label"`
	}{info.Label})
	if err == nil {
		_ = os.WriteFile(name+".json", data, 0o600)
	}
	if len(info.PNG) > 0 {
		_ = os.WriteFile(name+".png", info.PNG, 0o600)
	}
}

// fetchAppInfos starts reading the labels and icons of the third-party
// apps of a list that are not read yet. The device's own apps keep their
// package names: there may be hundreds, and the list shows them only when
// asked to.
func (a *App) fetchAppInfos(s *Session) {
	for _, app := range s.adbApps.list {
		if app.System {
			continue
		}
		a.fetchAppInfo(s, app)
	}
}

// fetchAppInfo reads one app's label and icon, on a goroutine, and shows
// them when it has.
func (a *App) fetchAppInfo(s *Session, app adbApp) {
	st := &s.adbApps
	if st.info == nil {
		st.info = map[string]*appInfo{}
	}
	if st.asking == nil {
		st.asking = map[string]bool{}
	}
	if st.failed == nil {
		st.failed = map[string]bool{}
	}
	if _, ok := st.info[app.Package]; ok || st.asking[app.Package] || st.failed[app.Package] {
		return
	}
	conn := a.adbConn(s)
	if conn == nil {
		return
	}
	st.asking[app.Package] = true
	key := appInfoKey(app)
	ctx := adbCtx(s)
	a.redraw()
	go func() {
		appInfoSem <- struct{}{}
		defer func() { <-appInfoSem }()
		if info, ok := loadAppInfo(key); ok {
			a.post(func() {
				delete(st.asking, app.Package)
				st.info[app.Package] = info
				a.redraw()
			})
			return
		}
		info, err := a.readAppInfo(ctx, conn, app)
		a.post(func() {
			delete(st.asking, app.Package)
			if err != nil || info == nil {
				st.failed[app.Package] = true
			} else {
				st.info[app.Package] = info
				saveAppInfo(key, info)
			}
			a.redraw()
		})
	}()
}

// readAppInfo reads app's label and icon: from the small files of its APK
// where the device has unzip, or from the APK itself otherwise.
func (a *App) readAppInfo(ctx context.Context, conn *adb.Conn, app adbApp) (*appInfo, error) {
	if info, err := a.readAppInfoSmall(ctx, conn, app); err == nil && info != nil {
		return info, nil
	}
	return a.readAppInfoWhole(ctx, conn, app)
}

// appInfoConfig asks for the Chinese name, at a density and SDK the icons
// of older releases serve as plain pictures, not adaptive icon XML.
func appInfoConfig(country bool) *androidbinary.ResTableConfig {
	c := &androidbinary.ResTableConfig{
		Language: [2]uint8{'z', 'h'},
		Density:  640,
		SDKVersion: 25,
	}
	if country {
		c.Country = [2]uint8{'C', 'N'}
	}
	return c
}

// appInfoLabel is the app's name in Chinese, else its default name, else
// its package.
func appInfoLabel(apk *apkfile.Apk) string {
	for _, c := range []*androidbinary.ResTableConfig{appInfoConfig(false), appInfoConfig(true), nil} {
		if label, err := apk.Label(c); err == nil {
			if label = strings.TrimSpace(label); label != "" {
				return label
			}
		}
	}
	return apk.PackageName()
}

// appInfoIconPath is the file of the app's icon, when the manifest names a
// picture rather than an adaptive icon.
func appInfoIconPath(apk *apkfile.Apk) string {
	for _, c := range []*androidbinary.ResTableConfig{appInfoConfig(false), appInfoConfig(true)} {
		p, err := apk.Manifest().App.Icon.WithResTableConfig(c).String()
		if err != nil || p == "" {
			continue
		}
		switch strings.ToLower(path.Ext(p)) {
		case ".png", ".webp", ".jpg", ".jpeg":
			return p
		}
	}
	return ""
}

// readAppInfoSmall fetches an APK's manifest, resource table and icon
// alone, with the device's unzip.
func (a *App) readAppInfoSmall(ctx context.Context, conn *adb.Conn, app adbApp) (*appInfo, error) {
	dir := "/data/local/tmp/xterm-info-" + appInfoFile(appInfoKey(app))
	qdir := remotefs.ShellQuote(dir)
	apk := remotefs.ShellQuote(app.Path)
	cmd := fmt.Sprintf("rm -rf %s; mkdir -p %s && unzip -p %s AndroidManifest.xml > %s/m && unzip -p %s resources.arsc > %s/r",
		qdir, qdir, apk, qdir, apk, qdir)
	if _, err := conn.Run(ctx, cmd); err != nil {
		return nil, err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = conn.Exec(c, "rm -rf "+qdir)
	}()

	tmp, err := os.MkdirTemp("", "xterm-appinfo-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	manifest := filepath.Join(tmp, "AndroidManifest.xml")
	table := filepath.Join(tmp, "resources.arsc")
	if err := a.pullADBFile(ctx, conn, dir+"/m", manifest); err != nil {
		return nil, err
	}
	if err := a.pullADBFile(ctx, conn, dir+"/r", table); err != nil {
		return nil, err
	}
	md, err := os.ReadFile(manifest)
	if err != nil {
		return nil, err
	}
	td, err := os.ReadFile(table)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct {
		name string
		data []byte
	}{{"AndroidManifest.xml", md}, {"resources.arsc", td}} {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	apkf, err := apkfile.OpenZipReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return nil, err
	}
	info := &appInfo{Label: appInfoLabel(apkf)}
	if icon := appInfoIconPath(apkf); icon != "" {
		iconCmd := fmt.Sprintf("unzip -p %s %s > %s/i", apk, remotefs.ShellQuote(icon), qdir)
		if _, err := conn.Run(ctx, iconCmd); err == nil {
			local := filepath.Join(tmp, "icon")
			if a.pullADBFile(ctx, conn, dir+"/i", local) == nil {
				if data, err := os.ReadFile(local); err == nil {
					info.setIcon(data)
				}
			}
		}
	}
	return info, nil
}

// readAppInfoWhole pulls an APK and reads its label and icon, for a device
// without unzip: the APK must not be larger than appInfoMaxPull.
func (a *App) readAppInfoWhole(ctx context.Context, conn *adb.Conn, app adbApp) (*appInfo, error) {
	if fi, err := conn.Stat(ctx, app.Path); err != nil {
		return nil, err
	} else if fi.Size > appInfoMaxPull {
		return nil, fmt.Errorf("apk of %d bytes is too large", fi.Size)
	}
	tmp, err := os.MkdirTemp("", "xterm-appinfo-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	local := filepath.Join(tmp, "app.apk")
	if err := a.pullADBFile(ctx, conn, app.Path, local); err != nil {
		return nil, err
	}
	apkf, err := apkfile.OpenFile(local)
	if err != nil {
		return nil, err
	}
	defer apkf.Close()
	info := &appInfo{Label: appInfoLabel(apkf)}
	for _, c := range []*androidbinary.ResTableConfig{appInfoConfig(false), appInfoConfig(true)} {
		if img, err := apkf.Icon(c); err == nil && img != nil {
			info.setImage(img)
			break
		}
	}
	return info, nil
}

// setIcon keeps an icon's picture, for showing and for the cache, as PNG.
func (i *appInfo) setIcon(data []byte) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	i.setImage(img)
}

// setImage is setIcon for an icon already decoded.
func (i *appInfo) setImage(img image.Image) {
	var buf bytes.Buffer
	if png.Encode(&buf, img) == nil {
		i.PNG = buf.Bytes()
	}
	i.Icon = ui.NewBitmap(img)
}


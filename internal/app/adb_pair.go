package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"finallyshell/internal/adb"
	"finallyshell/internal/config"
	"finallyshell/locales"
)

// adbPairTimeout is how long pairing may take: the device waits with its
// pairing dialog open. Tests shorten it.
var adbPairTimeout = 60 * time.Second

// pairADB is adb.Pair, and adbBinary and runADB find and run the adb of
// this computer; tests replace them.
var (
	pairADB   = adb.Pair
	adbBinary = findADB
	runADB    = runADBCommand
)

// adbPair pairs the device with this computer's ADB key, the one the app
// connects with afterwards: the pairing port and the code the device shows
// under "Pair device with pairing code" prove the exchange.
func (a *App) adbPair(ed *connEditor) {
	host := strings.TrimSpace(ed.addr)
	port := strings.TrimSpace(ed.adbPairPort)
	code := strings.TrimSpace(ed.adbPairCode)
	if host == "" || port == "" || code == "" || ed.adbPairing {
		return
	}
	ed.adbPairMsg, ed.adbPairErr = "", false
	ed.adbPairing = true
	a.redraw()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), adbPairTimeout)
		defer cancel()
		msg, failed := a.pairDevice(ctx, host+":"+port, code)
		a.post(func() {
			ed.adbPairing = false
			ed.adbPairMsg, ed.adbPairErr = msg, failed
			a.redraw()
		})
	}()
}

// pairDevice pairs the device, in the app first and with adb after, and
// says how it went.
func (a *App) pairDevice(ctx context.Context, addr, code string) (string, bool) {
	key, err := adb.LoadKey(adbKeyPath())
	if err == nil {
		err = pairADB(ctx, addr, code, key)
	}
	if err == nil {
		return locales.AdbPairOK(), false
	}
	builtin := err
	// The adb of this computer speaks the protocol too: it is tried when
	// the app's own exchange does not get through.
	bin, ok := adbBinary()
	if !ok {
		return locales.AdbPairFailed(builtin.Error()), true
	}
	out, adbErr := runADB(ctx, bin, []string{"pair", addr, code}, code+"\n")
	if adbErr == nil {
		return locales.AdbPairOK(), false
	}
	why := strings.TrimSpace(string(out))
	if why == "" {
		why = adbErr.Error()
	}
	return locales.AdbPairFailed(builtin.Error() + " · adb: " + lastLine(why)), true
}

// findADB is the adb of this computer: the one on PATH, or the
// platform-tools of an Android SDK it knows the place of.
func findADB() (string, bool) {
	name := "adb"
	if runtime.GOOS == "windows" {
		name = "adb.exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	var dirs []string
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			dirs = append(dirs, filepath.Join(v, "platform-tools"))
		}
	}
	for _, env := range []string{"LOCALAPPDATA", "USERPROFILE"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			dirs = append(dirs, filepath.Join(v, "AppData", "Local", "Android", "Sdk", "platform-tools"))
		}
	}
	// The app's own copy, fetched into the settings folder (adbtools.go).
	dirs = append(dirs, filepath.Join(config.Dir(), "platform-tools"))
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Android", "Sdk", "platform-tools"),
			filepath.Join(home, "Library", "Android", "sdk", "platform-tools"))
	}
	for _, d := range dirs {
		p := filepath.Join(d, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
	}
	return "", false
}

// runADBCommand runs adb with args, feeding stdin to a version that asks
// for the pairing code rather than taking it as an argument.
func runADBCommand(ctx context.Context, bin string, args []string, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	hideWindow(cmd)
	return cmd.CombinedOutput()
}

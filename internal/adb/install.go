package adb

// Installing apps: the APKs are copied to the device over the sync service,
// and `pm` installs them through a shell channel, which is what `adb
// install` and `adb install-multiple` do.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Install copies apk to the device and installs it, reporting the bytes it
// sent. It returns what `pm install` printed.
func (c *Conn) Install(ctx context.Context, apk string, progress func(sent, total int64)) (string, error) {
	if c == nil || c.dial == nil {
		return "", errors.New("adb: no connection to install over")
	}
	st, err := os.Stat(apk)
	if err != nil {
		return "", err
	}
	total := st.Size()
	// The device's own folder for it, named after the file, and safe in a
	// shell command.
	remote := path.Join("/data/local/tmp", "xterm-"+safeName(path.Base(apk)))
	err = c.pushAPK(ctx, apk, remote, func(n int64) {
		if progress != nil {
			progress(n, total)
		}
	})
	// Whatever happened, the copy is not needed any more.
	c.removePushed(remote)
	if err != nil {
		return "", err
	}
	out, err := c.Run(ctx, "pm install -r "+shellQuote(remote))
	if err != nil {
		return out, err
	}
	if !strings.Contains(out, "Success") {
		return out, fmt.Errorf("%s", installText(out))
	}
	return out, nil
}

// InstallMultiple copies the APKs of one app - its base APK and its split
// APKs - and installs them together, as `adb install-multiple` does. It
// returns what `pm install-commit` printed.
func (c *Conn) InstallMultiple(ctx context.Context, apks []string, progress func(sent, total int64)) (string, error) {
	if c == nil || c.dial == nil {
		return "", errors.New("adb: no connection to install over")
	}
	switch len(apks) {
	case 0:
		return "", errors.New("adb: no APK to install")
	case 1:
		return c.Install(ctx, apks[0], progress)
	}
	type pushed struct {
		local, remote, name string
		size                int64
	}
	files := make([]pushed, 0, len(apks))
	total := int64(0)
	for i, apk := range apks {
		st, err := os.Stat(apk)
		if err != nil {
			return "", err
		}
		name := safeName(path.Base(apk))
		remote := path.Join("/data/local/tmp", fmt.Sprintf("xterm-split-%d-%s", i+1, name))
		files = append(files, pushed{apk, remote, path.Base(apk), st.Size()})
		total += st.Size()
	}
	defer func() {
		for _, f := range files {
			c.removePushed(f.remote)
		}
	}()

	sent := int64(0)
	for _, f := range files {
		base := sent
		if err := c.pushAPK(ctx, f.local, f.remote, func(n int64) {
			if progress != nil {
				progress(base+n, total)
			}
		}); err != nil {
			return "", err
		}
		sent += f.size
	}

	out, err := c.Run(ctx, fmt.Sprintf("pm install-create -r -S %d", total))
	if err != nil {
		return out, err
	}
	session := installSession(out)
	if session == "" {
		return out, fmt.Errorf("pm install-create: %s", installText(out))
	}
	for _, f := range files {
		wout, werr := c.Run(ctx, fmt.Sprintf("pm install-write -S %d %s %s %s",
			f.size, session, shellQuote(f.name), shellQuote(f.remote)))
		if werr != nil {
			c.abandonInstall(session)
			return wout, werr
		}
		if !strings.Contains(wout, "Success") {
			c.abandonInstall(session)
			return wout, fmt.Errorf("%s", installText(wout))
		}
	}
	cout, cerr := c.Run(ctx, "pm install-commit "+session)
	if cerr != nil {
		c.abandonInstall(session)
		return cout, cerr
	}
	if !strings.Contains(cout, "Success") {
		return cout, fmt.Errorf("%s", installText(cout))
	}
	return cout, nil
}

// pushAPK copies apk to remote over the sync service, reporting the bytes
// sent so far.
func (c *Conn) pushAPK(ctx context.Context, apk, remote string, progress func(sent int64)) error {
	f, err := os.Open(apk)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := c.sync().Create(ctx, remote, 0o644, time.Now())
	if err != nil {
		return err
	}
	c.count.open(ChannelSync)
	sent := int64(0)
	buf := make([]byte, 64<<10)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				w.Close()
				c.count.closed(ChannelSync)
				return werr
			}
			sent += int64(n)
			if progress != nil {
				progress(sent)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			w.Close()
			c.count.closed(ChannelSync)
			return rerr
		}
	}
	err = w.Close()
	c.count.closed(ChannelSync)
	return err
}

// removePushed removes a file copied for an install, however the install
// went.
func (c *Conn) removePushed(remote string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = c.Exec(ctx, "rm -f "+shellQuote(remote))
}

// abandonInstall throws away the session of an install that failed.
func (c *Conn) abandonInstall(session string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = c.Exec(ctx, "pm install-abandon "+session)
}

// installSession is the session `pm install-create` made, from its
// "Success: created install session [1234567]".
var installSessionRE = regexp.MustCompile(`\[(\d+)\]`)

func installSession(out string) string {
	if m := installSessionRE.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	// Some package managers print the id alone.
	for _, f := range strings.Fields(out) {
		if _, err := strconv.Atoi(f); err == nil {
			return f
		}
	}
	return ""
}

// installText is what pm printed, or a word for printing nothing.
func installText(out string) string {
	if msg := strings.TrimSpace(out); msg != "" {
		return msg
	}
	return "pm said nothing"
}

// safeName is a file name the device can take, and a shell can read: the
// letters, digits and signs of one, the rest replaced by dashes.
func safeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "app.apk"
	}
	return b.String()
}

// shellQuote is s as one word of a shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

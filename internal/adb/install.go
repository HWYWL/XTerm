package adb

// Installing an app: the APK is copied to the device over the sync service,
// and `pm install` runs on it through a shell channel, which is what adb
// install does.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pgaskin/go-adb/adblib/adbsync"
)

// Install copies apk to the device and installs it, reporting the bytes it
// sent. It returns what `pm install` printed.
func (c *Conn) Install(ctx context.Context, apk string, progress func(sent, total int64)) (string, error) {
	if c == nil || c.dial == nil {
		return "", errors.New("adb: no connection to install over")
	}
	f, err := os.Open(apk)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	total := st.Size()
	// The device's own folder for it, named after the file, and safe in a
	// shell command.
	remote := path.Join("/data/local/tmp", "xterm-"+safeName(path.Base(apk)))
	sync := &adbsync.Client{Server: c.dial}
	w, err := sync.Create(ctx, remote, 0o644, time.Now())
	if err != nil {
		return "", err
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
				return "", werr
			}
			sent += int64(n)
			if progress != nil {
				progress(sent, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			w.Close()
			c.count.closed(ChannelSync)
			return "", rerr
		}
	}
	if err := w.Close(); err != nil {
		c.count.closed(ChannelSync)
		return "", err
	}
	c.count.closed(ChannelSync)

	out, err := c.Exec(ctx, "pm install -r "+shellQuote(remote))
	// Whatever happened, the copy is not needed any more.
	_, _ = c.Exec(ctx, "rm -f "+shellQuote(remote))
	if err != nil {
		return out, err
	}
	if !strings.Contains(out, "Success") {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = "pm install said nothing"
		}
		return out, fmt.Errorf("%s", msg)
	}
	return out, nil
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

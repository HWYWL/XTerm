package app

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"time"

	"finallyshell/internal/adb"
	"finallyshell/internal/config"
	"finallyshell/locales"
)

// openADB is adb.Dial, which a test replaces with a shell of its own: no
// device is at hand on a builder.
var openADB = func(ctx context.Context, o adb.Options) (io.ReadWriteCloser, error) {
	return adb.Dial(ctx, o)
}

// adbDialTimeout is how long reaching a device may take: the first
// connection asks its screen to allow this computer's key. Tests shorten
// it.
var adbDialTimeout = 60 * time.Second

// adbKeyPath is where the app keeps its own ADB key, beside the settings.
func adbKeyPath() string { return filepath.Join(config.Dir(), "adbkey") }

// dialADB opens a shell of the session's device, over its cable or the
// network (internal/adb). A device on a cable is reached through the ADB
// server of this computer, which is started when adb is there and it was
// not running.
func (a *App) dialADB(ctx context.Context, s *Session) (io.ReadWriteCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, adbDialTimeout)
	defer cancel()
	h := s.Host
	c := h.ADBConfig()
	opts := adb.Options{
		Wireless: c.Wireless(),
		Addr:     h.AddrPort(config.ADBPort),
		Device:   c.Device,
		KeyPath:  adbKeyPath(),
		Term:     h.TermTypeOrDefault(),
	}
	conn, err := openADB(ctx, opts)
	if err == nil {
		a.attachADB(s, conn, opts)
		return conn, nil
	}
	if c.Wireless() || !errors.Is(err, adb.ErrNoServer) {
		return nil, err
	}
	bin, ok := adbBinary()
	if !ok {
		return nil, err
	}
	if _, startErr := runADB(ctx, bin, []string{"start-server"}, ""); startErr != nil {
		return nil, err
	}
	conn, err = openADB(ctx, opts)
	if err == nil {
		a.attachADB(s, conn, opts)
	}
	return conn, err
}

// attachADB gives the session's meter the connection it reports on, with
// how the device is reached.
func (a *App) attachADB(s *Session, conn io.ReadWriteCloser, o adb.Options) {
	c, ok := conn.(*adb.Conn)
	if !ok {
		return
	}
	mode := "USB"
	if o.Wireless {
		mode = "WiFi " + o.Addr
	}
	a.adbMeter(s).attach(c, o.Addr, mode)
}

// describeADB says why reaching a device failed, in the language shown,
// for the two failures of its own that the system's words would not
// explain.
func describeADB(err error) (string, bool) {
	switch {
	case errors.Is(err, adb.ErrNoServer):
		return locales.ErrADBNoServer(), true
	case errors.Is(err, adb.ErrUnauthorized):
		return locales.ErrADBUnauthorized(), true
	case errors.Is(err, adb.ErrRejected):
		return locales.ErrADBRejected(), true
	case errors.Is(err, adb.ErrNoDevice):
		return locales.ErrADBNoDevice(), true
	case errors.Is(err, adb.ErrDeviceNotReady):
		return locales.ErrADBDeviceNotReady(), true
	}
	return "", false
}

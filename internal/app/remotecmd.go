package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/monitor"
	"finallyshell/internal/sshx"

	"golang.org/x/crypto/ssh"
)

// The plugins that run a command once, on a host, share this: a script on a
// new channel of a session's connection, or of a connection of its own made
// from the saved secret. The outputs go back to the UI thread through post,
// as everything the dials and the channel do happens on a goroutine.

// remoteTimeout is how long a one-off command runs before the channel closes.
const remoteTimeout = 20 * time.Second

// errNoSecret is a host whose password or passphrase is not saved: a one-off
// run cannot ask for it without the user, so it says so instead.
var errNoSecret = errors.New("no saved password or passphrase")

// runOnSession runs script on s's connection, off the UI thread, and calls
// then on it with what the command wrote.
func (a *App) runOnSession(s *Session, script string, then func(out string, err error)) {
	client := s.client
	if client == nil {
		then("", errNoClient)
		return
	}
	go func() {
		out, err := runScript(client, script)
		a.post(func() { then(out, err) })
	}()
}

// runOnHost runs script on the host: on a session already connected to it,
// or on a connection of its own, dialed with what the host logs in with and
// closed after. One with no secret this run and none saved fails, as the run
// has no prompt to ask it with; so does a host key ssh does not know, as
// nothing here can ask whether to trust it.
func (a *App) runOnHost(h *config.Host, script string, then func(out string, err error)) {
	if s := a.sessionForHost(h.ID); s != nil {
		a.runOnSession(s, script, then)
		return
	}
	if secret, ok := a.passwords[h.ID]; ok {
		a.dialAndRun(h, secret, script, then)
		return
	}
	if h.Auth == config.AuthAgent {
		a.dialAndRun(h, "", script, then)
		return
	}
	a.secret(loginSecretKey(h), func(secret string, ok bool) {
		if !ok {
			then("", errNoSecret)
			return
		}
		a.dialAndRun(h, secret, script, then)
	})
}

// dialAndRun dials the host with what it logs in with, runs script on the
// connection and closes it. The options read the settings and the jump hosts,
// so they are made on the UI thread; the dial and the channel are not.
func (a *App) dialAndRun(h *config.Host, secret, script string, then func(out string, err error)) {
	opts, err := a.dialOptions(h, secret, nil)
	if err != nil {
		then("", err)
		return
	}
	go func() {
		client, err := sshx.Dial(h, opts)
		if err != nil {
			a.post(func() { then("", err) })
			return
		}
		defer client.Close()
		out, err := runScript(client, script)
		a.post(func() { then(out, err) })
	}()
}

// runScript runs script with monitor's channel: its stderr is added to what
// the command wrote on stdout, as a person reads both in a terminal.
func runScript(client *ssh.Client, script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	defer cancel()
	stdout, stderr, err := monitor.RunScript(ctx, client, script)
	if err != nil && strings.TrimSpace(stdout) == "" {
		return stderr, err
	}
	if strings.TrimSpace(stderr) != "" {
		stdout += stderr
	}
	return stdout, err
}

// sessionForHost is the session connected to the host id, nil for none.
func (a *App) sessionForHost(id string) *Session {
	for _, s := range a.sessions {
		if s.Host != nil && s.Host.ID == id && s.client != nil {
			return s
		}
	}
	return nil
}

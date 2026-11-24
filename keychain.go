package main

import (
	"errors"

	"github.com/egoist/mygo"
)

// secrets is the system's credential store for the hosts' passwords. Its
// service is the app's name, not its identifier, which MyGo would take:
// `mygo dev` (whose identifier ends in ".dev"), `go run` and the
// installed app share the secrets, as they share hosts.json, whose host
// IDs the keys hold. oldSecrets is the service of the name before the
// rename: its secrets are moved over as they are read.
var (
	secrets    = mygo.Secrets.Service("xterm")
	oldSecrets = mygo.Secrets.Service("finallyshell")
)

// SecretGet and the others block while the system asks the user; the app
// calls them on a goroutine, where MyGo calls the store directly.
func (p *platform) SecretGet(key string) (string, bool, error) {
	v, err := secrets.Get(key)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		// Saved before the rename: kept here from now on.
		v, err = oldSecrets.Get(key)
		if err == nil {
			_ = secrets.Set(key, v)
			_ = oldSecrets.Delete(key)
		}
	}
	switch {
	case errors.Is(err, mygo.ErrSecretNotFound):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return string(v), true, nil
}

func (p *platform) SecretSet(key, value string) error { return secrets.Set(key, []byte(value)) }
func (p *platform) SecretDelete(key string) error     { return secrets.Delete(key) }

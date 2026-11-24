package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// The host keys window lists what known_hosts trusts, and forgetting a key
// asks once and rewrites the file, its comments kept.
func TestHostKeysWindow(t *testing.T) {
	a, p := testApp(t)
	path := filepath.Join(config.Dir(), config.KnownHostsFile)
	blob := "AAAAB3NzaC1yc2E=" // enough of a key to fingerprint
	content := "# a note of my own\nhost1 ssh-ed25519 " + blob + "\nhost2,host3 ssh-rsa " + blob + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	a.openPlugin(pluginHostKeys)
	w := p.show(t, WindowHostKeys)
	w.Frame()
	if !w.HasText("host1") || !w.HasText("host2, host3") {
		t.Errorf("hosts missing; texts %q", w.Texts())
	}
	if !w.HasText("ssh-ed25519  " + config.KnownHostEntry{Key: blob}.Fingerprint()) {
		t.Errorf("fingerprint missing; texts %q", w.Texts())
	}

	// Forget host1: the first button asks, the second does it.
	if err := w.Click(locales.HostKeysDelete()); err != nil {
		t.Fatal(err)
	}
	w.Frame()
	if !w.HasText(locales.HostKeysConfirm()) {
		t.Errorf("no confirmation; texts %q", w.Texts())
	}
	if err := w.Click(locales.HostKeysConfirm()); err != nil {
		t.Fatal(err)
	}
	w.Frame()
	entries, err := config.LoadKnownHosts()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || strings.Join(entries[0].Hosts, ",") != "host2,host3" {
		t.Errorf("left %+v", entries)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# a note of my own") {
		t.Errorf("the comment went: %q", data)
	}
	if _, ok := w.Find("host1"); ok {
		t.Error("the row stayed after deleting it")
	}
}

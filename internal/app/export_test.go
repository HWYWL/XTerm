package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// The room past the Connections page's rows imports the connections, and
// exports the live ones as JSON where the user says.
func TestConnectionsMenuImportsAndExports(t *testing.T) {
	a, p, tt := connPageApp(t, "en")
	// The room past the rows.
	a.connTree.tree.Open.Clear()
	tt.Frame()

	tt.RightClickAt(100, 790)
	if err := tt.ChooseMenuItem(locales.Import()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	p.until(t, "the import window", func() bool { return a.windowOpen(WindowImport) && a.imp != nil && !a.imp.busy })
	a.closeWindow(WindowImport)
	tt.Frame()

	path := filepath.Join(t.TempDir(), "connections.json")
	p.pickSave(path, nil)
	tt.RightClickAt(100, 790)
	if err := tt.ChooseMenuItem(locales.ExportConnections()); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	p.until(t, "the file", func() bool {
		_, err := os.Stat(path)
		return err == nil
	})

	// The dialog it asked, and the document written: the folders and the
	// live hosts, as hosts.json holds them.
	p.saveMu.Lock()
	asked := p.saveAsked
	p.saveMu.Unlock()
	if len(asked) != 1 || asked[0].parent != WindowMain || asked[0].title != locales.ExportConnectionsTitle() ||
		asked[0].typeName != locales.JsonFiles() || !slices.Equal(asked[0].extensions, []string{"json"}) ||
		filepath.Base(asked[0].path) != exportName {
		t.Fatalf("asked %+v", asked)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc config.Store
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%v: %s", err, data)
	}
	if doc.Version != config.HostsVersion || len(doc.Hosts) != 5 || len(doc.Folders) != 3 {
		t.Errorf("exported version %d, %d hosts, %d folders: %s", doc.Version, len(doc.Hosts), len(doc.Folders), data)
	}
	if strings.Contains(string(data), "old-box") || strings.Contains(string(data), "password") {
		t.Errorf("a deleted host, or a password: %s", data)
	}
}

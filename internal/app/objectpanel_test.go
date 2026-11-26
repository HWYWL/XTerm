package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/objectstore"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// An object storage of the local file system shows its folders and files
// where a terminal would be: they open, and the tab has no bottom panel.
func TestObjectPanel(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0o755); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backups", "a.tar.gz"), []byte("xx"), 0o644); err != nil {
		t.Fatalf("%v", err)
	}

	a, p := testApp(t)
	p.SetLocale("en")
	h := &config.Host{
		ID: config.NewHostID(), Name: "files", Protocol: config.ProtocolObject,
		Object: &config.ObjectSettings{Provider: "local", LocalPath: dir},
	}
	a.hosts.Add(h)
	tt := ui.NewTester(func(c *ui.Context) { a.View(c) }, 1280, 820)
	tt.SetLocale("en")
	p.tt = tt
	a.setLeftPage(leftConnections)
	a.openSessionIn(nil, h)
	p.until(t, "the storage", func() bool {
		s := a.current()
		return s != nil && s.isObject() && s.status == statusConnected
	})
	// Connected, the left column shows the storage's System page: whether
	// its endpoint answers, and what the tab did with it.
	p.until(t, "the system page", func() bool { return a.left.page == leftSystem })
	p.until(t, "the probe", func() bool {
		return a.current().objectMeter.snapshot().probeState != ""
	})
	tt.Frame()
	for _, want := range []string{
		"files", "notes.txt", "backups/",
		locales.ObjectOnline(), locales.ObjectSuccess(), locales.ObjectTraffic(),
		locales.ObjectRequests(), locales.ObjectLatency(), locales.ObjectCodes(),
		"Put", "Get", "List", "P50",
	} {
		if !tt.HasText(want) {
			t.Errorf("%q is not shown; texts %q", want, tt.Texts())
		}
	}
	if p50, _, _ := objectPercentiles(a.current().objectMeter.snapshot().latency("List")); p50 <= 0 {
		t.Errorf("the listing's latency %v", p50)
	}
	saveShot(t, tt, "object-system-en.png")

	// A transfer lists its file under the panel, and the traffic moves as
	// its bytes come, rather than when it ends.
	s := a.current()
	tr := &objectTransfer{
		upload: false, name: "backup.zip", where: filepath.Join(dir, "backup.zip"),
		total: 400 << 10, startAt: time.Now(),
	}
	s.addObjectTransfer(tr)
	time.Sleep(20 * time.Millisecond) // the frame clock is coarse
	tr.report(s, 200<<10)
	if r := tr.rate(); r <= 0 {
		t.Fatalf("rate %v, since %v", r, time.Since(tr.startAt))
	}
	tt.Frame()
	for _, want := range []string{
		fmt.Sprintf(locales.ObjectTransfersRunning(), 1), "backup.zip",
	} {
		if !tt.HasText(want) {
			t.Errorf("%q is not shown; texts %q", want, tt.Texts())
		}
	}
	before := s.objectMeter.snapshot().in
	tr.report(s, 300<<10)
	if got := s.objectMeter.snapshot().in; got <= before {
		t.Errorf("the traffic did not move: %d, then %d", before, got)
	}
	saveShot(t, tt, "object-transfers-en.png")
	tr.finish(s, nil)
	tt.Frame()
	if !tt.HasText(locales.ObjectClearFinished()) {
		t.Errorf("no button to clear the finished; texts %q", tt.Texts())
	}

	// A folder opens, and shows what it holds.
	a.objectOpen(s, "backups")
	p.until(t, "the folder", func() bool { return s.objects.cwd == "backups" })
	tt.Frame()
	if !tt.HasText("a.tar.gz") || tt.HasText("notes.txt") {
		t.Errorf("the folder's entries; texts %q", tt.Texts())
	}
	saveShot(t, tt, "object-panel-folder-en.png")

	// The tab has no bottom panel: asking for it opens no row.
	a.openBottomTab(0)
	if a.current().isPanelTab() {
		t.Error("the panel opened")
	}
}

// The page of an object storage saves what it holds: the provider, where
// the storage is, and the directory it shows.
func TestEditorSavesObjectHost(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click(locales.ProtocolObject()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	if err := tt.Click(locales.EditorPageObject()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.editor.protocol != editorObject {
		t.Fatalf("the protocol %q; texts %q", a.editor.protocol, tt.Texts())
	}
	if a.editor.nav.Page != editorObject {
		t.Fatalf("the page %q; texts %q", a.editor.nav.Page, tt.Texts())
	}
	choose(t, tt, locales.FieldObjectProvider(), locales.ObjectProviderLocal())
	tt.Frame()
	if a.editor.objectProvider != config.ObjectLocal {
		t.Fatalf("the provider %q; texts %q", a.editor.objectProvider, tt.Texts())
	}
	dir := t.TempDir()
	typeInto(t, tt, locales.FieldLocalPath(), dir)
	tt.Frame()
	saveShot(t, tt, "editor-object-en.png")
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if len(a.hosts.Hosts) != 1 {
		t.Fatalf("hosts %v", a.hosts.Hosts)
	}
	h := a.hosts.Hosts[0]
	if !h.IsObject() {
		t.Fatalf("protocol %q", h.Protocol)
	}
	if h.Object == nil || h.Object.Provider != config.ObjectLocal || h.Object.LocalPath != dir {
		t.Errorf("object %+v", h.Object)
	}
}

// A cloud's region is chosen from its list, and the endpoint follows it: a
// region the list does not have is typed, from the last item.
func TestEditorObjectRegion(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click(locales.ProtocolObject()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if a.editor.region != "cn-hangzhou" || a.editor.endpoint != "oss-cn-hangzhou.aliyuncs.com" {
		t.Fatalf("Aliyun's default: region %q, endpoint %q", a.editor.region, a.editor.endpoint)
	}
	choose(t, tt, locales.FieldRegion(), "cn-shanghai")
	tt.Frame()
	if a.editor.region != "cn-shanghai" || a.editor.endpoint != "oss-cn-shanghai.aliyuncs.com" {
		t.Errorf("another region: region %q, endpoint %q", a.editor.region, a.editor.endpoint)
	}
	saveShot(t, tt, "editor-object-cloud-en.png")

	// AWS's default endpoint is the one for every region.
	choose(t, tt, locales.FieldObjectProvider(), "AWS S3")
	tt.Frame()
	if a.editor.region != "us-east-1" || a.editor.endpoint != "s3.amazonaws.com" {
		t.Errorf("AWS's default: region %q, endpoint %q", a.editor.region, a.editor.endpoint)
	}
	choose(t, tt, locales.FieldRegion(), "eu-west-1")
	tt.Frame()
	if a.editor.endpoint != "s3.eu-west-1.amazonaws.com" {
		t.Errorf("AWS's region: endpoint %q", a.editor.endpoint)
	}

	// A region the list does not have is typed: MinIO's list is short.
	choose(t, tt, locales.FieldObjectProvider(), "MinIO")
	tt.Frame()
	// A server of one's own says what its region is: what it signs with.
	if !tt.HasText(locales.ObjectRegionSelfHosted()) {
		t.Errorf("no note on a self hosted region; texts %q", tt.Texts())
	}
	choose(t, tt, locales.FieldRegion(), locales.ObjectRegionCustom())
	tt.Frame()
	if !a.editor.regionCustom {
		t.Fatalf("the region is not typed")
	}
	typeInto(t, tt, locales.FieldRegion(), "cn-qingdao")
	tt.Frame()
	if a.editor.region != "cn-qingdao" {
		t.Errorf("the typed region %q", a.editor.region)
	}
}

// The folders of a key are its prefixes: the panel goes up through them,
// and a folder is a key that ends with the slash.
func TestObjectKeys(t *testing.T) {
	if got := objectUp("a/b/c"); got != "a/b" {
		t.Errorf("up %q", got)
	}
	if got := objectUp("a"); got != "" {
		t.Errorf("up %q", got)
	}
	if got := objectPath("a/b"); got != "/a/b" {
		t.Errorf("path %q", got)
	}
	dir := t.TempDir()
	s, err := objectstore.OpenLocal(dir)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := s.Mkdir(context.Background(), "x/y"); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x", "y")); err != nil {
		t.Errorf("the folder %v", err)
	}
}

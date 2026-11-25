package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/record"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// recordState is this run's session transcripts: the recorders by session
// ID, and the window that lists the files.
type recordState struct {
	bySession map[int64]*record.Recorder
	files     recFiles
}

// recFiles is the window over the recorded transcripts.
type recFiles struct {
	open  bool
	items []recFile
	err   error
	msg   string
}

// recFile is a transcript in the window.
type recFile struct {
	path string
	rel  string
	size int64
	mod  time.Time
}

func (r *recordState) init() { r.bySession = map[int64]*record.Recorder{} }

// recorder is s's recorder, nil when it is not being recorded.
func (a *App) recorder(s *Session) *record.Recorder {
	if a.records.bySession == nil || s == nil {
		return nil
	}
	return a.records.bySession[s.ID]
}

// recording tells whether s is being recorded.
func (a *App) recording(s *Session) bool { return a.recorder(s) != nil }

// startRecording starts s's transcript, and says whether it did.
func (a *App) startRecording(s *Session) bool {
	if s == nil || a.recording(s) || s.status != statusConnected || s.Host == nil {
		return false
	}
	name := s.name
	if name == "" {
		name = s.Host.Name
	}
	if name == "" {
		name = s.Host.Addr
	}
	path := record.File(record.Dir(config.Dir()), name, time.Now())
	header := []string{
		"Session: " + name,
		"Host: " + s.Host.User + "@" + s.Host.Addr,
		"Protocol: SSH",
		"Started: " + time.Now().Format("2006-01-02 15:04:05"),
	}
	rec, err := record.Start(path, header)
	if err != nil {
		a.notify(locales.RecordFailed(describeError(err)))
		return false
	}
	a.records.bySession[s.ID] = rec
	a.notify(locales.RecordStarted(filepath.Base(path)))
	return true
}

// stopRecording ends s's transcript, with reason as its last note.
func (a *App) stopRecording(s *Session, reason string) {
	rec := a.recorder(s)
	if rec == nil {
		return
	}
	delete(a.records.bySession, s.ID)
	rec.Stop(reason)
	a.notify(locales.RecordSaved(filepath.Base(rec.Path())))
}

// toggleRecording starts or stops s's transcript.
func (a *App) toggleRecording(s *Session) {
	if a.recording(s) {
		a.stopRecording(s, locales.RecordStoppedByUser())
		return
	}
	a.startRecording(s)
}

// autoRecord starts s's transcript when the settings say the sessions are
// recorded, as a connection just came up.
func (a *App) autoRecord(s *Session) {
	if a.settings.RecordAuto {
		a.startRecording(s)
	}
}

// recordCommand writes a command submitted in s's terminal.
func (a *App) recordCommand(s *Session, line string) {
	if rec := a.recorder(s); rec != nil {
		rec.Command(line)
	}
}

// recordNote writes what the app did with s, as a connection ended.
func (a *App) recordNote(s *Session, text string) {
	if rec := a.recorder(s); rec != nil {
		rec.Note(text)
	}
}

// recordConn is a terminal's connection with a transcript beside it: what
// the server sends is written to s's recorder as it is read. The recorder
// is looked up each time, so a session may start and stop recording
// without the connection being made again.
type recordConn struct {
	io.ReadWriteCloser
	app *App
	s   *Session
}

func (c *recordConn) Read(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Read(p)
	if n > 0 {
		if rec := c.app.recorder(c.s); rec != nil {
			rec.Output(p[:n])
		}
	}
	return n, err
}

// Resize passes the terminal's size to the connection, as the terminal
// tells the server through the wrapped one.
func (c *recordConn) Resize(cols, rows int) error {
	if r, ok := c.ReadWriteCloser.(interface{ Resize(int, int) error }); ok {
		return r.Resize(cols, rows)
	}
	return nil
}

// recordMenu is the recording items of a session's context menu.
func (a *App) recordMenu(m *ui.Menu, s *Session) {
	if s.isLocal() || s.status != statusConnected {
		return
	}
	if a.recording(s) {
		if m.Item(locales.RecordStop()).Chosen() {
			a.stopRecording(s, locales.RecordStoppedByUser())
		}
	} else if m.Item(locales.RecordStart()).Chosen() {
		a.startRecording(s)
	}
	if m.Item(locales.RecordFiles()).Chosen() {
		a.openRecFiles()
	}
}

// recordSub is what a session row's second line adds while it records.
func (a *App) recordSub(s *Session) string {
	rec := a.recorder(s)
	if rec == nil {
		return ""
	}
	return " · " + locales.RecordingNow()
}

// openRecFiles opens the window over the transcripts, reading the folder.
func (a *App) openRecFiles() {
	a.records.files.open = true
	a.scanRecFiles()
}

// scanRecFiles reads the transcripts' folder again.
func (a *App) scanRecFiles() {
	f := &a.records.files
	f.err, f.msg = nil, ""
	root := record.Dir(config.Dir())
	f.items = nil
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".log") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		f.items = append(f.items, recFile{path: path, rel: rel, size: info.Size(), mod: info.ModTime()})
		return nil
	})
	slices.SortFunc(f.items, func(x, y recFile) int { return y.mod.Compare(x.mod) })
}

// recFilesDialog draws the window over the transcripts, when it is open.
func (a *App) recFilesDialog(c *ui.Context) {
	f := &a.records.files
	if !f.open {
		return
	}
	open := true
	ui.Modal(c, &open, func() { a.recFilesView(c) })
	if !open {
		f.open = false
	}
}

func (a *App) recFilesView(c *ui.Context) {
	f := &a.records.files
	ui.Column(c).Width(680).MaxHeight(560).Children(func() {
		ui.Row(c).Padding(12, 16).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			ui.Box(c).Size(3, 18).Radius(2).Background(theme.Accent).Shrink(0)
			ui.Text(c, locales.RecordFiles()).FontSize(15).Bold()
			widgets.Muted(c, fmt.Sprintf("%d", len(f.items))).Shrink(0)
			ui.Spacer(c)
			if ui.Button(c, locales.OpenFolder()).Clicked() {
				openPath(record.Dir(config.Dir()))
			}
			if ui.Button(c, locales.Refresh()).Clicked() {
				a.scanRecFiles()
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(10, 16, 16, 16).Gap(6).Children(func() {
				if f.err != nil {
					ui.Text(c, describeError(f.err)).TextColor(theme.Danger).FontSize(12).Wrap()
				}
				if len(f.items) == 0 {
					widgets.Muted(c, locales.RecordNone())
				}
				for i, it := range f.items {
					file := it
					ui.Row(c).Key(i).Gap(10).AlignItems(ui.Center).Padding(6, 8).Radius(6).
						Background(colors.sidebar).Children(func() {
							ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
								ui.Text(c, file.rel).FontSize(12.5).SingleLine().Tooltip(file.path)
								widgets.Muted(c, formatSize(file.size)+" · "+file.mod.Format("2006-01-02 15:04")).
									FontSize(11.5).SingleLine()
							})
							if ui.Button(c, locales.OpenFolder()).Clicked() {
								openPath(file.path)
							}
							if ui.Button(c, locales.Delete()).Clicked() {
								if err := os.Remove(file.path); err != nil {
									f.msg = describeError(err)
								} else {
									f.msg = locales.RecordDeleted()
									a.scanRecFiles()
								}
							}
						})
				}
				if f.msg != "" {
					widgets.Muted(c, f.msg).FontSize(12)
				}
			})
		})
	})
}

// recordPage is the Recording page of the terminal's options.
func (a *App) recordPage(c *ui.Context) {
	ui.Column(c).Gap(8).Children(func() {
		on := a.settings.RecordAuto
		if ui.Checkbox(c, &on, locales.RecordAuto()).Changed() {
			a.changeSettings(func() { a.settings.RecordAuto = on })
		}
		widgets.Muted(c, locales.RecordAutoNote()).FontSize(12).PaddingStart(18)
	})
	ui.Column(c).Gap(8).Padding(12, 0, 0, 0).Children(func() {
		ui.Text(c, locales.RecordFolder()).Bold()
		widgets.Muted(c, record.Dir(config.Dir())).FontSize(12).Selectable()
		if ui.Button(c, locales.OpenFolder()).Clicked() {
			openPath(record.Dir(config.Dir()))
		}
		widgets.Muted(c, locales.RecordFolderNote()).FontSize(12)
	})
}

// openPath opens a file or a folder with the system's program.
func openPath(path string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("cmd", "/c", "start", "", path).Start()
	case "darwin":
		_ = exec.Command("open", path).Start()
	default:
		_ = exec.Command("xdg-open", path).Start()
	}
}

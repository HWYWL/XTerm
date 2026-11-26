package app

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/sshx"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// WindowEditor is the connection editor, FinalShell's "New connection"
// window: one at a time, for a new host or an existing one.
const WindowEditor WindowKind = "editor"

// editorSpec is how the connection editor opens, for a new host or, with
// editing, one saved. Opened from the main window, it belongs to it and
// does not block it: a modal window would be a sheet covering the main
// window on macOS, and would block every window of the app on Linux (see
// docs/zh-CN/development.md). Opened from the connection manager, openEditor makes it modal
// to the manager, as FinalShell's is.
func editorSpec(editing bool) WindowSpec {
	title := locales.NewConnection
	if editing {
		title = locales.EditConnection
	}
	return WindowSpec{
		Kind:  WindowEditor,
		Title: title,
		Width: 680, Height: 600,
		MinWidth: 600, MinHeight: 440,
	}
}

// The editor's pages.
const (
	editorSSH      = "ssh"
	editorRedis    = "redis"
	editorTerminal = "terminal"
	editorProxy    = "proxy"
	editorTunnels  = "tunnels"
	editorJump     = "jump"
)

// editorProtocols are the protocols the editor offers at its top. SSH and
// Redis are the ones it configures; the others are there for the protocols
// to come, and say they are not supported yet.
var editorProtocols = []struct {
	id   string
	name func() string
}{
	{editorSSH, func() string { return "SSH" }},
	{editorRedis, locales.ProtocolRedis},
	{"s3", func() string { return "S3" }},
	{"rdp", func() string { return "RDP" }},
	{"vnc", func() string { return "VNC" }},
	{"serial", locales.ProtocolSerial},
	{"telnet", func() string { return "Telnet" }},
}

// editorField names a field of the editor that can be wrong.
type editorField int

const (
	fieldAddr editorField = iota
	fieldPort
	fieldUser
	fieldKeyPath
	fieldTermType
	fieldSeeds
	fieldDB
)

// page is the editor's page that shows f.
func (f editorField) page() string {
	switch {
	case f == fieldTermType:
		return editorTerminal
	case f == fieldJumpHost:
		return editorJump
	case slices.Contains(proxyFieldOrder, f):
		return editorProxy
	case f == fieldSeeds || f == fieldDB:
		return editorRedis
	}
	return editorSSH
}

// connEditor is what the connection editor shows: the fields of a host,
// as typed, until OK saves them into the host.
type connEditor struct {
	id  string           // the host edited; "" for a new one
	nav widgets.NavState // the page shown

	// protocol is the protocol of the host, editorSSH or editorRedis,
	// chosen by the row at the top of the window.
	protocol string

	name, addr, port, notes string
	auth                    config.AuthKind
	user                    string
	password                string
	keyPath, passphrase     string
	remember                bool // in the system's credential store
	monitor                 bool

	termType  string
	backspace config.BackspaceKey
	delete    config.DeleteKey
	encoding  config.Encoding
	proxy     proxyFields  // the Proxy page (editor_proxy.go)
	tunnels   tunnelEditor // see editor_tunnels.go
	folder    string       // a new host's, from the manager
	// tree is the tree of hosts that opened the editor, which shows the
	// host once saved: the manager's when nil.
	tree *managerState

	// The Redis page's fields: whether the host is a cluster, its seeds
	// one a line, and its database as typed (editor_redis.go).
	cluster bool
	seeds   string
	db      string

	// readingSecret is set while the host's saved password, or
	// passphrase, is read from the credential store into the fields.
	readingSecret bool
	// readingProxySecret is the same of its proxy's password.
	readingProxySecret bool

	// agentCheck is what the SSH agent's Check found (agent.go).
	agentCheck agentCheck

	// checked is set once OK was pressed: from then on the errors show,
	// and follow the fields as they change.
	checked bool
	// saveErr says why saving failed, in the language shown.
	saveErr func() string
}

// newConnection opens the connection editor for a new host, or brings
// the open editor to the front. OK adds the host to the saved ones.
func (a *App) newConnection() {
	a.openEditor(nil, WindowMain)
}

// editConnection opens the connection editor on h, a saved host, or
// brings the open editor to the front, whichever host it edits: there is
// one editor at a time, and what it holds is not thrown away. OK saves
// the changes into h, which sessions open to it use the next time they
// connect.
func (a *App) editConnection(h *config.Host) {
	a.openEditor(h, WindowMain)
}

// openEditor opens the editor on h, nil for a new host, from the window of
// kind from: modal to the connection manager when from is it.
func (a *App) openEditor(h *config.Host, from WindowKind) {
	if a.windowOpen(WindowEditor) {
		a.plat.FocusWindow(WindowEditor)
		return
	}
	ed := &connEditor{protocol: editorSSH, port: "22", user: "root", auth: config.AuthPassword, remember: true, monitor: true, db: "0"}
	if h != nil {
		ed.id = h.ID
		ed.name, ed.addr, ed.notes = h.Name, h.Addr, h.Notes
		ed.remember = h.RemembersPassword()
		ed.protocol = h.ProtocolOrDefault()
		if h.IsRedis() {
			ed.port = "6379"
			if h.Port != 0 {
				ed.port = strconv.Itoa(h.Port)
			}
			if h.Redis != nil {
				ed.cluster = h.Redis.Cluster
				ed.seeds = strings.Join(h.Redis.Seeds, "\n")
				ed.db = strconv.Itoa(h.Redis.DB)
			}
			ed.password = a.passwords[h.ID]
			ed.auth = config.AuthPassword
		} else {
			ed.port = strconv.Itoa(h.PortOr22())
			ed.auth, ed.user, ed.keyPath = h.Auth, h.User, h.KeyPath
			switch ed.auth {
			case config.AuthAgent:
			case config.AuthKey:
				ed.passphrase = a.passwords[h.ID]
			default:
				ed.auth = config.AuthPassword
				ed.password = a.passwords[h.ID]
			}
			ed.monitor = h.MonitorEnabled()
		}
		ed.termType, ed.backspace, ed.delete, ed.encoding = h.TermType, h.Backspace, h.Delete, h.Encoding
	}
	ed.proxy = a.proxyFieldsOf(h)
	ed.tunnels = newTunnelEditor(h)
	a.editor = ed
	a.readEditorSecret(ed, h)
	spec := editorSpec(h != nil)
	if from == WindowManager && a.windowOpen(WindowManager) {
		// A sheet of the manager on macOS, showing its title inside;
		// on Linux GTK blocks the main window too while it is open, as
		// FinalShell's own dialogs do. Saving shows in the manager.
		spec.Parent, spec.Modal = WindowManager, true
	}
	a.openWindow(spec, a.editorView)
}

// validTermType tells whether s can be a terminal type, a terminfo
// name: letters, digits and a few signs.
func validTermType(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-.+_", r))
	})
}

// problems are what is wrong with the fields, said in the language shown.
func (ed *connEditor) problems() map[editorField]string {
	errs := map[editorField]string{}
	if ed.protocol == editorRedis {
		ed.redisProblems(errs)
		if !validTermType(strings.TrimSpace(ed.termType)) {
			errs[fieldTermType] = locales.ErrTermTypeInvalid()
		}
		ed.proxyProblems(errs)
		return errs
	}
	if strings.TrimSpace(ed.addr) == "" {
		errs[fieldAddr] = locales.ErrHostRequired()
	}
	if port, err := strconv.Atoi(strings.TrimSpace(ed.port)); err != nil || port < 1 || port > 65535 {
		errs[fieldPort] = locales.ErrPortInvalid()
	}
	if strings.TrimSpace(ed.user) == "" {
		errs[fieldUser] = locales.ErrUserRequired()
	}
	if ed.auth == config.AuthKey && strings.TrimSpace(ed.keyPath) == "" {
		errs[fieldKeyPath] = locales.ErrKeyFileRequired()
	}
	if !validTermType(strings.TrimSpace(ed.termType)) {
		errs[fieldTermType] = locales.ErrTermTypeInvalid()
	}
	ed.proxyProblems(errs)
	return errs
}

// saveEditor checks the editor's fields and saves them, closing the
// editor; when something is wrong it shows the first page with an error,
// and keeps the editor open.
func (a *App) saveEditor() {
	ed := a.editor
	ed.checked = true
	if errs := ed.problems(); len(errs) > 0 {
		first, here := "", false
		for _, f := range append([]editorField{fieldAddr, fieldPort, fieldUser, fieldKeyPath, fieldTermType, fieldSeeds, fieldDB}, proxyFieldOrder...) {
			if errs[f] != "" {
				first = cmp.Or(first, f.page())
				here = here || f.page() == ed.nav.Page
			}
		}
		if !here {
			ed.nav.Page = first
		}
		return
	}
	var h *config.Host
	if ed.id != "" {
		// Deleted in the manager while the editor was open: not saved back.
		if h = a.hosts.Host(ed.id); h == nil || h.Deleted() {
			ed.saveErr = locales.ErrHostDeleted
			return
		}
	} else {
		h = &config.Host{Folder: ed.folder, CreatedAt: time.Now()}
		a.hosts.Add(h)
		ed.id = h.ID
	}
	if ed.protocol == editorRedis {
		a.saveRedis(h, ed)
	} else {
		h.Protocol, h.Redis = "", nil
		port, _ := strconv.Atoi(strings.TrimSpace(ed.port))
		h.Name, h.Addr, h.Port, h.Notes = strings.TrimSpace(ed.name), strings.TrimSpace(ed.addr), port, ed.notes
		h.User, h.Auth = strings.TrimSpace(ed.user), ed.auth
		secret := ed.password
		h.KeyPath = ""
		switch ed.auth {
		case config.AuthKey:
			h.KeyPath, secret = strings.TrimSpace(ed.keyPath), ed.passphrase
		case config.AuthAgent:
			secret = ""
		}
		a.keepLogin(h, secret, ed.remember, ed.readingSecret)
		h.NoMonitor = !ed.monitor
		h.TermType, h.Backspace, h.Delete, h.Encoding = strings.TrimSpace(ed.termType), ed.backspace, ed.delete, ed.encoding
		if h.TermType == config.DefaultTermType {
			h.TermType = ""
		}
		a.saveProxy(h, &ed.proxy)
		h.Tunnels = ed.tunnels.result()
		a.keepProxySecret(h, ed.remember, ed.readingProxySecret)
	}
	tree := ed.tree
	if tree == nil {
		tree = &a.manager
	}
	tree.reveal(managerItem{host: h.ID})
	a.redraw() // the manager, the Connections page and the sessions show the host

	if err := a.saveHosts(a.hosts); err != nil {
		ed.saveErr = func() string { return locales.ErrSaveFailed(err.Error()) }
		return
	}
	ed.saveErr = nil
	a.closeWindow(WindowEditor)
}

// browseKey asks for the private key file, starting where the one typed
// is, else in ~/.ssh.
func (a *App) browseKey() {
	ed := a.editor
	dir := ""
	if p := strings.TrimSpace(ed.keyPath); p != "" {
		dir = filepath.Dir(sshx.ExpandHome(p))
	} else if home, err := os.UserHomeDir(); err == nil {
		if fi, err := os.Stat(filepath.Join(home, ".ssh")); err == nil && fi.IsDir() {
			dir = filepath.Join(home, ".ssh")
		}
	}
	title := locales.ChooseKeyFile()
	go func() {
		path, err := a.plat.OpenFile(WindowEditor, title, dir)
		if err != nil || path == "" {
			return
		}
		a.post(func() {
			if a.editor == ed {
				ed.keyPath = path
			}
		})
	}()
}

// termTypes are the terminal types the editor suggests.
var termTypes = []string{"xterm-256color", "xterm", "xterm-color", "vt100", "vt220", "linux", "screen-256color", "tmux-256color", "ansi"}

// protocolRow is the row of protocols at the top of the editor: SSH and
// Redis, either of which it configures, chosen; and the others, which are
// not supported yet, muted and saying so.
func (a *App) protocolRow(c *ui.Context, ed *connEditor) {
	ui.Row(c).Padding(10, 14).Gap(6).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		for _, p := range editorProtocols {
			active := p.id == ed.protocol
			supported := p.id == editorSSH || p.id == editorRedis
			pill := ui.Row(c).Height(26).Padding(0, 14).Radius(13).
				AlignItems(ui.Center).Justify(ui.Center)
			if active {
				pill.Background(theme.Accent)
			} else {
				pill.Background(theme.Surface).Border(1, theme.Border)
				if !supported {
					pill.Tooltip(locales.ProtocolUnsupported())
				}
			}
			pill.Children(func() {
				text := ui.Text(c, p.name()).FontSize(12).SingleLine()
				if active {
					text.TextColor(theme.AccentText).FontWeight(600)
				} else if supported {
					text.TextColor(theme.Text)
				} else {
					text.TextColor(theme.TextMuted)
				}
			})
			if supported && pill.Cursor(ui.CursorPointer).Clicked() {
				a.chooseProtocol(ed, p.id)
			}
		}
	})
}

// chooseProtocol shows the page of the protocol chosen, with the fields it
// has: SSH's address and login, or Redis's mode, nodes and password.
func (a *App) chooseProtocol(ed *connEditor, id string) {
	if ed.protocol == id {
		ed.nav.Page = id
		return
	}
	ed.protocol = id
	ed.nav.Page = id
	// The ports of the two protocols differ: the one typed is kept only
	// when it is not the other protocol's.
	if id == editorRedis && strings.TrimSpace(ed.port) == "22" {
		ed.port = "6379"
	}
	if id == editorSSH && strings.TrimSpace(ed.port) == "6379" {
		ed.port = "22"
	}
}

// editorView is the connection editor's window: the pages on the left,
// OK and Cancel below. Enter is OK, but in the notes, where it breaks
// the line, and Escape is Cancel.
func (a *App) editorView(c *ui.Context) {
	ed := a.editor
	if ed == nil {
		return
	}
	// Enter where no field takes it, and Enter in a text input, which
	// takes it as submitting.
	submit := c.Shortcut(0, ui.KeyEnter)
	input := func(e ui.Element) ui.Element {
		submit = e.Submitted() || submit
		return e
	}
	var errs map[editorField]string
	if ed.checked {
		errs = ed.problems()
	}
	// The protocol's page is not an entry of the tree: the row of protocols
	// above chooses it, and the tree holds the pages beside it, which both
	// protocols have: a Redis server too is reached through a proxy or a
	// jump host, and its command line is a terminal.
	if ed.nav.Page == "" {
		ed.nav.Page = ed.protocol
	}
	main := widgets.NavPage{ID: editorSSH, Hidden: true, Title: locales.EditorPageSSH, View: func(c *ui.Context) { a.sshPage(c, ed, errs, input) }}
	if ed.protocol == editorRedis {
		main = widgets.NavPage{ID: editorRedis, Hidden: true, Title: locales.EditorPageRedis, View: func(c *ui.Context) { a.redisPage(c, ed, errs, input) }}
	}
	pages := []widgets.NavPage{
		main,
		{ID: editorTerminal, Title: locales.EditorPageTerminal, View: func(c *ui.Context) { a.terminalPage(c, ed, errs, input) }},
		{ID: editorProxy, Title: locales.EditorPageProxy, View: func(c *ui.Context) { a.proxyPage(c, ed, errs, input) }},
		{ID: editorTunnels, Title: locales.EditorPageTunnels, View: func(c *ui.Context) { a.tunnelsPage(c, ed) }},
		{ID: editorJump, Title: locales.EditorPageJumpHost, View: func(c *ui.Context) { a.jumpPage(c, ed, errs) }},
	}
	ui.Column(c).Fill().Children(func() {
		a.protocolRow(c, ed)
		widgets.NavPages(c, &ed.nav, pages).Grow(1).MinHeight(0)
		ui.Row(c).Gap(8).Padding(10, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
			if ed.saveErr != nil {
				ui.Text(c, ed.saveErr()).TextColor(theme.Danger).Grow(1).MinWidth(0)
			} else {
				ui.Spacer(c)
			}
			if ui.Button(c, locales.Cancel()).Clicked() {
				a.closeWindow(WindowEditor)
			}
			if ui.PrimaryButton(c, locales.Ok()).Clicked() {
				submit = true
			}
		})
	})
	if submit && a.editor == ed {
		a.saveEditor()
	}
}

// sshPage is the editor's first page: the address, how to log in, and
// whether to monitor the server. input marks a text input whose Enter is
// OK.
func (a *App) sshPage(c *ui.Context, ed *connEditor, errs map[editorField]string, input func(ui.Element) ui.Element) {
	ui.Form(c, func() {
		ui.Fieldset(c, locales.SectionGeneral(), func() {
			ui.Field(c, locales.FieldName(), func() {
				input(ui.TextInput(c, &ed.name).Placeholder(locales.FieldNamePlaceholder()))
			})
			ui.Field(c, locales.FieldHost(), func() {
				input(ui.TextInput(c, &ed.addr).Placeholder(locales.FieldHostPlaceholder()).AutoFocus())
			}).Error(errs[fieldAddr])
			ui.Field(c, locales.FieldPort(), func() {
				input(ui.TextInput(c, &ed.port).Width(90))
			}).Error(errs[fieldPort])
			ui.Field(c, locales.FieldNotes(), func() {
				ui.TextArea(c, &ed.notes).Lines(2, 4)
			})
		})
		ui.Fieldset(c, locales.SectionAuth(), func() {
			ui.Field(c, locales.FieldAuth(), func() {
				kinds := []config.AuthKind{config.AuthPassword, config.AuthKey, config.AuthAgent}
				names := []string{locales.AuthPassword(), locales.AuthKey(), locales.AuthAgent()}
				chosen := names[max(slices.Index(kinds, ed.auth), 0)]
				if ui.Select(c, &chosen, names).Width(160).Changed() {
					ed.auth = kinds[slices.Index(names, chosen)]
				}
			})
			ui.Field(c, locales.FieldUser(), func() {
				input(ui.TextInput(c, &ed.user))
			}).Error(errs[fieldUser])
			switch ed.auth {
			case config.AuthAgent:
				a.agentField(c, ed)
			case config.AuthKey:
				ui.Field(c, locales.FieldKeyFile(), func() {
					ui.Row(c).Gap(8).Children(func() {
						input(ui.TextInput(c, &ed.keyPath).Placeholder("~/.ssh/id_ed25519").Grow(1).MinWidth(0))
						if ui.Button(c, locales.Browse()).Clicked() {
							a.browseKey()
						}
					})
				}).Error(errs[fieldKeyPath])
				ui.Field(c, locales.FieldPassphrase(), func() {
					input(ui.TextInput(c, &ed.passphrase).Password().Placeholder(locales.PassphrasePlaceholder()))
				})
			default:
				ui.Field(c, locales.FieldPassword(), func() {
					input(ui.TextInput(c, &ed.password).Password())
				})
			}
			if ed.auth != config.AuthAgent {
				a.rememberField(c, &ed.remember, ed.auth == config.AuthKey)
			}
		})
		ui.Fieldset(c, locales.SectionAdvanced(), func() {
			ui.Field(c, "", func() {
				ui.Checkbox(c, &ed.monitor, locales.MonitorChannel())
			}).Description(locales.MonitorChannelHint())
		})
	})
}

// agentField is the SSH agent's line of the editor, for a host that logs
// in with one: the address is the options', and Check lists its keys.
func (a *App) agentField(c *ui.Context, ed *connEditor) {
	ui.Field(c, locales.FieldAgent(), func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.AgentUsesOptions())
				if ui.Button(c, locales.AgentCheck()).Label(locales.AgentCheckLabel()).Disabled(ed.agentCheck.running).Clicked() {
					a.checkAgent(&ed.agentCheck)
				}
			})
			agentCheckView(c, &ed.agentCheck)
		})
	}).Description(locales.AgentEditorNote())
}

// terminalPage is the editor's page of the terminal's options.
func (a *App) terminalPage(c *ui.Context, ed *connEditor, errs map[editorField]string, input func(ui.Element) ui.Element) {
	ui.Form(c, func() {
		ui.Field(c, locales.FieldTermType(), func() {
			input(ui.Autocomplete(c, &ed.termType, termTypes).Placeholder(config.DefaultTermType).Width(240))
		}).Error(errs[fieldTermType])
		ui.Field(c, locales.FieldEncoding(), func() {
			def := a.settings.TermEncodingName()
			choiceSelect(c, locales.FieldEncoding(), &ed.encoding, encodingChoices, &def)
		}).Description(locales.EncodingHint())
		ui.Field(c, locales.FieldBackspace(), func() {
			def := a.settings.TermBackspaceKey()
			choiceSelect(c, locales.FieldBackspace(), &ed.backspace, backspaceChoices, &def)
		})
		ui.Field(c, locales.FieldDelete(), func() {
			def := a.settings.TermDeleteKey()
			choiceSelect(c, locales.FieldDelete(), &ed.delete, deleteChoices, &def)
		})
	})
	widgets.Muted(c, locales.TerminalNextConnect()).FontSize(12)
}

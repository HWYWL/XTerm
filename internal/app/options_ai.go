package app

import (
	"context"
	"strings"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// aiPage is Options › AI: the assistant of the sessions' AI tabs, where it
// talks to, what it says, and what it may do on the session's host.
func (a *App) aiPage(c *ui.Context) {
	st := &a.ai
	a.ensureAIKey()
	ui.Column(c).Gap(12).Children(func() {
		widgets.Muted(c, locales.AiIntro()).FontSize(12).Wrap()
		enabled := a.settings.AI.Enabled
		if ui.Checkbox(c, &enabled, locales.AiEnable()).Changed() {
			a.changeSettings(func() { a.settings.AI.Enabled = enabled })
		}
		a.aiServiceFields(c)
		a.aiModelFields(c)
		a.aiRequestFields(c)
		a.aiExecFields(c)
		if st.test != "" {
			ui.Text(c, st.test).Wrap().FontSize(11).TextColor(theme.TextMuted)
		}
	})
}

// aiServiceFields are the provider the assistant talks to, its address and
// key, and the test of the connection.
func (a *App) aiServiceFields(c *ui.Context) {
	cfg := &a.settings.AI
	st := &a.ai
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, locales.AiService()).Bold()
		a.aiServiceSelect(c)
		p, ok := cfg.ChosenProvider()
		if !ok {
			return
		}
		ui.Field(c, locales.AiBaseURL(), func() {
			v := p.BaseURL
			if ui.TextInput(c, &v).Width(340).Label(locales.AiBaseURL()).Changed() {
				url := strings.TrimSpace(v)
				a.changeSettings(func() { a.settings.AI.SetProviderURL(p.ID, url) })
			}
		})
		a.aiKeyField(c, p)
		ui.Field(c, "", func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, locales.AiTestConnection()).Disabled(st.testing).Clicked() {
					a.aiTestConnection()
				}
				if st.testing {
					widgets.Muted(c, locales.AiTesting()).FontSize(11)
				}
				ui.Spacer(c)
				if ui.Button(c, locales.AiDeleteProvider()).Disabled(len(cfg.Providers) <= 1).Clicked() {
					a.changeSettings(func() { a.settings.AI.DeleteProvider(p.ID) })
				}
				if ui.Button(c, locales.AiAddProvider()).Clicked() {
					a.changeSettings(func() {
						a.settings.AI.AddProvider(locales.AiCustomProvider(), "https://")
					})
				}
			})
		})
	})
}

// aiServiceSelect chooses the provider of the list.
func (a *App) aiServiceSelect(c *ui.Context) {
	cfg := &a.settings.AI
	names := make([]string, len(cfg.Providers))
	chosen := ""
	cur, hasCur := cfg.ChosenProvider()
	for i, p := range cfg.Providers {
		names[i] = p.Name
		if hasCur && p.ID == cur.ID {
			chosen = p.Name
		}
	}
	ui.Field(c, locales.AiService(), func() {
		if ui.Select(c, &chosen, names).Width(240).Label(locales.AiService()).Changed() {
			for _, p := range cfg.Providers {
				if p.Name == chosen {
					a.changeSettings(func() {
						a.settings.AI.Provider = p.ID
						a.ai.keyLoaded = false
					})
				}
			}
		}
	})
}

// aiKeyField is the provider's key, kept in the credential store as it is
// typed.
func (a *App) aiKeyField(c *ui.Context, p config.AIProvider) {
	st := &a.ai
	f := ui.Field(c, locales.AiAPIKey(), func() {
		v := ""
		if st.keyFor == p.ID && st.keyLoaded {
			v = st.key
		}
		if ui.TextInput(c, &v).Password().Width(340).Label(locales.AiAPIKey()).Changed() {
			v = strings.TrimSpace(v)
			st.key, st.keyFor, st.keyLoaded = v, p.ID, true
			key := config.AIKeyName(p.ID)
			if v == "" {
				a.deleteSecret(key)
			} else {
				a.setSecret(key, v)
			}
		}
	})
	f.Description(locales.AiAPIKeyHint())
}

// aiModelFields are the model the assistant asks, and the list's models.
func (a *App) aiModelFields(c *ui.Context) {
	cfg := &a.settings.AI
	p, ok := cfg.ChosenProvider()
	if !ok {
		return
	}
	models := cfg.ModelsOf(p.ID)
	names := make([]string, len(models))
	chosen := ""
	cur, hasCur := cfg.ChosenModel()
	for i, m := range models {
		names[i] = m.Name
		if hasCur && m.ID == cur.ID {
			chosen = m.Name
		}
	}
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, locales.AiModel()).Bold()
		ui.Field(c, locales.AiModel(), func() {
			if ui.Select(c, &chosen, names).Width(240).Label(locales.AiModel()).Changed() {
				for _, m := range models {
					if m.Name == chosen {
						a.changeSettings(func() { a.settings.AI.Model = m.ID })
					}
				}
			}
		})
		ui.Field(c, "", func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.ai.newModel).Width(200).Placeholder(locales.AiNewModel()).Label(locales.AiModel())
				if ui.Button(c, locales.AiAdd()).Disabled(strings.TrimSpace(a.ai.newModel) == "").Clicked() {
					name := strings.TrimSpace(a.ai.newModel)
					a.ai.newModel = ""
					a.changeSettings(func() { a.settings.AI.AddModel(p.ID, name) })
				}
				if hasCur && ui.Button(c, locales.AiDeleteModel()).Clicked() {
					a.changeSettings(func() { a.settings.AI.DeleteModel(cur.ID) })
				}
			})
		})
	})
}

// aiRequestFields are how much of the terminal a question carries, how long
// a request may take, and what the assistant is told.
func (a *App) aiRequestFields(c *ui.Context) {
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, locales.AiRequest()).Bold()
		ui.Field(c, locales.AiContextLines(), func() {
			n := float64(a.settings.AI.ContextLineCount())
			if ui.NumberInput(c, &n, 20, 2000, 20).Width(120).Label(locales.AiContextLines()).Changed() {
				lines := int(n)
				a.changeSettings(func() { a.settings.AI.ContextLines = lines })
			}
		})
		ui.Field(c, locales.AiTimeout(), func() {
			sec := float64(a.settings.AI.Timeout().Seconds())
			if ui.NumberInput(c, &sec, 10, 300, 10).Width(120).Label(locales.AiTimeout()).Changed() {
				seconds := int(sec)
				a.changeSettings(func() { a.settings.AI.TimeoutSeconds = seconds })
			}
		})
		ui.Field(c, locales.AiSystemPrompt(), func() {
			if ui.TextArea(c, &a.settings.AI.SystemPrompt).Lines(3, 8).Width(420).
				Placeholder(locales.AiDefaultSystemPrompt()).Changed() {
				a.saveSettings(a.settings)
				a.redraw()
			}
		})
	})
}

// aiExecFields are what the assistant may do on the session's host.
func (a *App) aiExecFields(c *ui.Context) {
	cfg := &a.settings.AI
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, locales.AiExec()).Bold()
		allow := cfg.ExecTool()
		if ui.Checkbox(c, &allow, locales.AiAllowExec()).Changed() {
			a.changeSettings(func() { a.settings.AI.NoExecTool = !allow })
		}
		confirm := cfg.ConfirmExec()
		if ui.Checkbox(c, &confirm, locales.AiConfirmExec()).Disabled(!allow).Changed() {
			a.changeSettings(func() { a.settings.AI.NoExecConfirm = !confirm })
		}
		screen := cfg.ScreenContext
		if ui.Checkbox(c, &screen, locales.AiScreenContext()).Changed() {
			a.changeSettings(func() { a.settings.AI.ScreenContext = screen })
		}
		widgets.Muted(c, locales.AiExecHint()).FontSize(11).PaddingStart(18).Wrap()
	})
}

// aiTestConnection asks the provider whether it answers with this key at
// all, and says so on the page.
func (a *App) aiTestConnection() {
	st := &a.ai
	client, why := a.aiClient()
	if client == nil {
		st.test = why
		a.redraw()
		return
	}
	st.testing, st.test = true, ""
	a.redraw()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := client.Test(ctx)
		a.post(func() {
			st.testing = false
			if err != nil {
				st.test = locales.AiTestFailed(err.Error())
			} else {
				st.test = locales.AiTestOK()
			}
		})
	}()
}

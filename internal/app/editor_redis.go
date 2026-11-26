package app

import (
	"net"
	"strconv"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// redisPage is the editor's page of a Redis host: whether it is a
// standalone server or a cluster, where it is, and how to log in. Its
// command line is the terminal's, so it has no page of its own here.
func (a *App) redisPage(c *ui.Context, ed *connEditor, errs map[editorField]string, input func(ui.Element) ui.Element) {
	ui.Form(c, func() {
		ui.Fieldset(c, locales.SectionGeneral(), func() {
			ui.Field(c, locales.FieldName(), func() {
				input(ui.TextInput(c, &ed.name).Placeholder(locales.FieldNamePlaceholder()))
			})
			ui.Field(c, locales.FieldRedisMode(), func() {
				modes := []string{locales.RedisStandalone(), locales.RedisCluster()}
				chosen := modes[0]
				if ed.cluster {
					chosen = modes[1]
				}
				if ui.Select(c, &chosen, modes).Width(160).Changed() {
					ed.cluster = chosen == modes[1]
				}
			})
			if ed.cluster {
				ui.Field(c, locales.FieldRedisSeeds(), func() {
					ui.TextArea(c, &ed.seeds).Lines(3, 6).Placeholder(locales.RedisSeedsPlaceholder())
				}).Error(errs[fieldSeeds]).Description(locales.RedisSeedsHint())
			} else {
				ui.Field(c, locales.FieldHost(), func() {
					input(ui.TextInput(c, &ed.addr).Placeholder(locales.FieldHostPlaceholder()).AutoFocus())
				}).Error(errs[fieldAddr])
				ui.Field(c, locales.FieldPort(), func() {
					input(ui.TextInput(c, &ed.port).Width(90))
				}).Error(errs[fieldPort])
				ui.Field(c, locales.FieldRedisDB(), func() {
					input(ui.TextInput(c, &ed.db).Width(90))
				}).Error(errs[fieldDB])
			}
			ui.Field(c, locales.FieldNotes(), func() {
				ui.TextArea(c, &ed.notes).Lines(2, 4)
			})
		})
		ui.Fieldset(c, locales.SectionAuth(), func() {
			ui.Field(c, locales.FieldPassword(), func() {
				input(ui.TextInput(c, &ed.password).Password())
			})
			a.rememberField(c, &ed.remember, false)
		})
	})
}

// redisProblems are what is wrong with the Redis page's fields: the
// address of a standalone server, the seeds of a cluster, and the database,
// which Redis keeps from 0 to 15.
func (ed *connEditor) redisProblems(errs map[editorField]string) {
	if ed.cluster {
		seeds := redisSeedLines(ed.seeds)
		if len(seeds) == 0 {
			errs[fieldSeeds] = locales.ErrRedisSeedsRequired()
			return
		}
		for _, s := range seeds {
			if !validRedisAddr(s) {
				errs[fieldSeeds] = locales.ErrRedisSeedInvalid(s)
				return
			}
		}
		return
	}
	if strings.TrimSpace(ed.addr) == "" {
		errs[fieldAddr] = locales.ErrHostRequired()
	}
	if port, err := strconv.Atoi(strings.TrimSpace(ed.port)); err != nil || port < 1 || port > 65535 {
		errs[fieldPort] = locales.ErrPortInvalid()
	}
	if db, err := strconv.Atoi(strings.TrimSpace(ed.db)); err != nil || db < 0 || db > 15 {
		errs[fieldDB] = locales.ErrRedisDBInvalid()
	}
}

// saveRedis saves the Redis page into h: its protocol, address or seeds,
// database, and the password, which is the credential store's as any other
// host's.
func (a *App) saveRedis(h *config.Host, ed *connEditor) {
	h.Protocol = config.ProtocolRedis
	h.Object = nil
	if h.Redis == nil {
		h.Redis = &config.RedisSettings{}
	}
	h.Name, h.Addr, h.Notes = strings.TrimSpace(ed.name), strings.TrimSpace(ed.addr), ed.notes
	h.User, h.Auth, h.KeyPath = "", config.AuthPassword, ""
	h.TermType, h.Backspace, h.Delete, h.Encoding = strings.TrimSpace(ed.termType), ed.backspace, ed.delete, ed.encoding
	if h.TermType == config.DefaultTermType {
		h.TermType = ""
	}
	h.NoMonitor = true
	// The way to it — a proxy, a jump host, tunnels on the jump host's
	// connection — is its own pages' (redis_session.go).
	a.saveProxy(h, &ed.proxy)
	h.Tunnels = ed.tunnels.result()
	a.keepProxySecret(h, ed.remember, ed.readingProxySecret)
	h.Redis.Cluster = ed.cluster
	if ed.cluster {
		h.Redis.Seeds, h.Redis.DB = redisSeedLines(ed.seeds), 0
		h.Port = 0
	} else {
		h.Redis.Seeds = nil
		h.Port, _ = strconv.Atoi(strings.TrimSpace(ed.port))
		h.Redis.DB, _ = strconv.Atoi(strings.TrimSpace(ed.db))
	}
	a.keepLogin(h, ed.password, ed.remember, ed.readingSecret)
}

// redisSeedLines are the seed nodes typed, one a line, without the empty
// lines and the spaces around them.
func redisSeedLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// validRedisAddr tells whether s is a host and port, as a seed is.
func validRedisAddr(s string) bool {
	host, port, err := net.SplitHostPort(s)
	if err != nil || strings.TrimSpace(host) == "" {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

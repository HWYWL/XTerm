package app

import (
	"slices"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The object storage page's fields, for the errors of its own.
const (
	fieldEndpoint editorField = fieldDB + 1 + iota
	fieldBucket
	fieldAccessKey
	fieldLocalPath
)

// objectPage is the editor's page of an object storage: which storage it
// is, where it is, the credential, and the folder it opens on.
func (a *App) objectPage(c *ui.Context, ed *connEditor, errs map[editorField]string, input func(ui.Element) ui.Element) {
	p := config.ObjectProviderOf(ed.objectProvider)
	ui.Form(c, func() {
		ui.Fieldset(c, locales.SectionGeneral(), func() {
			ui.Field(c, locales.FieldName(), func() {
				input(ui.TextInput(c, &ed.name).Placeholder(locales.FieldNamePlaceholder()))
			})
			ui.Field(c, locales.FieldObjectProvider(), func() {
				names := make([]string, len(config.ObjectProviders))
				chosen := names[0]
				for i, prov := range config.ObjectProviders {
					names[i] = objectProviderName(prov)
					if prov.ID == p.ID {
						chosen = names[i]
					}
				}
				if ui.Select(c, &chosen, names).Width(220).Changed() {
					for _, prov := range config.ObjectProviders {
						if objectProviderName(prov) == chosen {
							ed.setObjectProvider(prov)
						}
					}
				}
			})
			if p.Local {
				ui.Field(c, locales.FieldLocalPath(), func() {
					input(ui.TextInput(c, &ed.localPath).Placeholder(locales.FieldLocalPathHint()).AutoFocus())
				}).Error(errs[fieldLocalPath])
			} else {
				ui.Field(c, locales.FieldEndpoint(), func() {
					input(ui.TextInput(c, &ed.endpoint).Placeholder("s3.amazonaws.com").AutoFocus())
				}).Error(errs[fieldEndpoint])
				if p.S3 {
					region := ui.Field(c, locales.FieldRegion(), func() {
						// The regions the cloud serves, as a list; a region
						// it does not list is typed, from the last item.
						if len(p.Regions) == 0 || ed.regionCustom {
							input(ui.TextInput(c, &ed.region).Width(180))
							return
						}
						names := append(slices.Clone(p.Regions), locales.ObjectRegionCustom())
						chosen := ed.region
						if chosen == "" {
							chosen = p.Regions[0]
						}
						if ui.Select(c, &chosen, names).Width(220).Changed() {
							if chosen == locales.ObjectRegionCustom() {
								ed.regionCustom = true
								return
							}
							ed.region = chosen
							if ep := p.EndpointForRegion(chosen); ep != "" {
								ed.endpoint = ep
							}
						}
					})
					// A server of one's own has no regions of the clouds':
					// its "region" is only what the request is signed with.
					if p.IsSelfHosted() {
						region.Description(locales.ObjectRegionSelfHosted())
					}
					ui.Field(c, locales.FieldBucket(), func() {
						input(ui.TextInput(c, &ed.bucket).Placeholder("my-bucket"))
					}).Error(errs[fieldBucket])
					ui.Checkbox(c, &ed.pathStyle, locales.ObjectPathStyle())
				} else {
					ui.Field(c, locales.FieldBucket(), func() {
						input(ui.TextInput(c, &ed.bucket).Placeholder(locales.FieldContainerPlaceholder()))
					}).Error(errs[fieldBucket])
				}
				ui.Field(c, locales.FieldAccessKey(), func() {
					input(ui.TextInput(c, &ed.accessKey))
				}).Error(errs[fieldAccessKey])
				ui.Field(c, locales.FieldSecretKey(), func() {
					input(ui.TextInput(c, &ed.password).Password())
				})
				a.rememberField(c, &ed.remember, false)
			}
			ui.Field(c, locales.FieldObjectRoot(), func() {
				input(ui.TextInput(c, &ed.objectRoot).Placeholder(locales.FieldObjectRootHint()))
			})
			ui.Field(c, locales.FieldNotes(), func() {
				ui.TextArea(c, &ed.notes).Lines(2, 4).Placeholder("")
			})
		})
	})
}

// objectProviderName is the storage's name in the app's language: the
// clouds keep theirs, and a directory's says what it is.
func objectProviderName(p config.ObjectProvider) string {
	if p.Local {
		return locales.ObjectProviderLocal()
	}
	return p.Name
}

// setObjectProvider puts the provider's endpoint into the fields, and its
// path style, as the editor chooses it.
func (ed *connEditor) setObjectProvider(p config.ObjectProvider) {
	if ed.objectProvider == p.ID {
		return
	}
	ed.objectProvider = p.ID
	ed.endpoint, ed.region = p.Endpoint, p.Region
	if ed.region == "" && len(p.Regions) > 0 {
		ed.region = p.Regions[0]
	}
	ed.regionCustom = !slices.Contains(p.Regions, ed.region)
	ed.pathStyle = p.PathStyle
}

// objectProblems are what is wrong with the page's fields: where the
// storage is, the bucket or the directory, and the access key.
func (ed *connEditor) objectProblems(errs map[editorField]string) {
	p := config.ObjectProviderOf(ed.objectProvider)
	if p.Local {
		if strings.TrimSpace(ed.localPath) == "" {
			errs[fieldLocalPath] = locales.ErrObjectLocalRequired()
		}
		return
	}
	if strings.TrimSpace(ed.endpoint) == "" {
		errs[fieldEndpoint] = locales.ErrObjectEndpointRequired()
	}
	if strings.TrimSpace(ed.bucket) == "" {
		errs[fieldBucket] = locales.ErrObjectBucketRequired()
	}
	if strings.TrimSpace(ed.accessKey) == "" {
		errs[fieldAccessKey] = locales.ErrObjectKeyRequired()
	}
}

// saveObject saves the page into h: its protocol, the storage's fields,
// and the secret key, which the credential store keeps as any password.
func (a *App) saveObject(h *config.Host, ed *connEditor) {
	p := config.ObjectProviderOf(ed.objectProvider)
	h.Protocol = config.ProtocolObject
	h.Redis, h.Serial, h.ADB = nil, nil, nil
	h.Name, h.Notes = strings.TrimSpace(ed.name), ed.notes
	h.Addr, h.Port, h.User, h.KeyPath = "", 0, "", ""
	h.Auth = config.AuthPassword
	h.NoMonitor = true
	h.Redis, h.Proxy, h.Tunnels = nil, nil, nil
	if h.Name == "" {
		if p.Local {
			h.Name = strings.TrimSpace(ed.localPath)
		} else {
			h.Name = strings.TrimSpace(ed.bucket)
		}
	}
	if h.Object == nil {
		h.Object = &config.ObjectSettings{}
	}
	h.Object.Provider = p.ID
	h.Object.PathStyle = ed.pathStyle && p.S3
	h.Object.Insecure = strings.HasPrefix(strings.TrimSpace(ed.endpoint), "http://")
	if p.Local {
		h.Object.LocalPath = strings.TrimSpace(ed.localPath)
		h.Object.Endpoint, h.Object.Bucket, h.Object.AccessKey = "", "", ""
	} else {
		h.Object.Endpoint = strings.TrimSpace(ed.endpoint)
		h.Object.Region = strings.TrimSpace(ed.region)
		h.Object.Bucket = strings.TrimSpace(ed.bucket)
		h.Object.AccessKey = strings.TrimSpace(ed.accessKey)
	}
	h.Object.Root = strings.Trim(strings.TrimSpace(ed.objectRoot), "/")
	a.keepLogin(h, ed.password, ed.remember, ed.readingSecret)
}

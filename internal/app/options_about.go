package app

import (
	_ "embed"

	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// upstreamURL is the open source project XTerm is a secondary development
// of; aboutIntro names it, and the page links to it.
const upstreamURL = "https://github.com/TooonyChen/finallyshell"

// projectURL is this project's open source home, as the page links to it.
const projectURL = "https://github.com/HWYWL/XTerm"

// aboutLogo is the app's icon (docs/icon.svg), drawn at any size.
//
//go:embed logo.svg
var logoData []byte

var aboutLogo = ui.MustParseSVG(logoData)

// aboutPage is Options › About: what the app is, the project it comes from,
// and its updates.
func (a *App) aboutPage(c *ui.Context) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Image(c, aboutLogo).Size(56, 56).Shrink(0)
			ui.Text(c, locales.AppTitle()).Bold().FontSize(18)
		})
		ui.Text(c, locales.AboutIntro())
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.AboutSource()).FontSize(12)
			ui.Link(c, "HWYWL/XTerm", projectURL)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.AboutUpstream()).FontSize(12)
			ui.Link(c, "TooonyChen/finallyshell", upstreamURL)
		})
	})
	a.updatesSection(c)
}

package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// The app has a light and a dark look, and shows the one of the system's
// appearance, or the one the user chose in Options › General, which
// forces the app's appearance (options_appearance.go). Each window picks
// its look at the start of each frame (useTheme), from the appearance
// MyGo gives the frame.

// newTheme is MyGo's theme t made compact, with fonts that cover Chinese,
// in the glass look the app shows: a deep blue night, and a soft blue-white
// day, of translucent surfaces, hairline strokes and a cyan accent.
func newTheme(t *ui.Theme) *ui.Theme {
	t.Spacing = 3
	t.FontSize = 13
	t.Font = "Microsoft YaHei UI, PingFang SC, Noto Sans CJK SC, WenQuanYi Zen Hei, system-ui"
	t.Radius = 10
	if t.Dark {
		t.Background = ui.Hex("#070a13")
		t.Surface = ui.RGBA(255, 255, 255, 0.055)
		t.SurfaceHover = ui.RGBA(255, 255, 255, 0.085)
		t.SurfacePressed = ui.RGBA(255, 255, 255, 0.115)
		t.Border = ui.RGBA(255, 255, 255, 0.115)
		t.Text = ui.Hex("#e9eefb")
		t.TextMuted = ui.RGBA(233, 238, 251, 0.64)
		t.Accent = ui.Hex("#63c8ff")
		t.AccentHover = ui.Hex("#86d6ff")
		t.AccentPressed = ui.Hex("#3aa9e0")
		t.AccentText = ui.Hex("#04202e")
		t.Danger = ui.Hex("#ff6b6b")
		t.Warning = ui.Hex("#ffc861")
		t.Success = ui.Hex("#3ddc97")
		t.Selection = ui.RGBA(99, 200, 255, 0.30)
		t.Focus = ui.Hex("#63c8ff")
		t.Inverse = ui.Hex("#101422")
		t.InverseText = ui.Hex("#e9eefb")
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.18)
		return t
	}
	t.Background = ui.Hex("#eef2fa")
	t.Surface = ui.RGBA(255, 255, 255, 0.62)
	t.SurfaceHover = ui.RGBA(255, 255, 255, 0.78)
	t.SurfacePressed = ui.RGBA(255, 255, 255, 0.90)
	t.Border = ui.RGBA(13, 23, 40, 0.10)
	t.Text = ui.Hex("#0d1728")
	t.TextMuted = ui.RGBA(13, 23, 40, 0.64)
	t.Accent = ui.Hex("#0a7cff")
	t.AccentHover = ui.Hex("#2b8dff")
	t.AccentPressed = ui.Hex("#0060d0")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#e0453f")
	t.Warning = ui.Hex("#b8790a")
	t.Success = ui.Hex("#12a878")
	t.Selection = ui.RGBA(10, 124, 255, 0.22)
	t.Focus = ui.Hex("#0a7cff")
	t.Inverse = ui.Hex("#0d1728")
	t.InverseText = ui.Hex("#ffffff")
	t.Scrollbar = ui.RGBA(13, 23, 40, 0.22)
	return t
}

// The app's two themes, made once: the light one and the dark one.
var (
	lightTheme = newTheme(ui.LightTheme())
	darkTheme  = newTheme(ui.DarkTheme())
)

// theme and colors are the theme and the palette of the window being
// built. useTheme sets them at the start of each window's frame; every
// window builds on the UI thread, one at a time, and all show the same
// appearance.
var (
	theme  = lightTheme
	colors = &lightColors
)

// useTheme gives the frame c builds the app's theme for its appearance,
// and makes it, with its palette, theme and colors.
func useTheme(c *ui.Context) {
	theme, colors = lightTheme, &lightColors
	if c.Theme().Dark {
		theme, colors = darkTheme, &darkColors
	}
	c.SetTheme(theme)
}

// palette is the app's colors besides its theme's, for one appearance.
type palette struct {
	// sidebar is the gray of the side panel, as FinalShell's, and of the
	// bars around the file editor's text.
	sidebar ui.Color
	// line divides parts of a window: under tab strips and toolbars,
	// between toolbar groups, around the file editor's text.
	line ui.Color
	// rowLine is the fainter line between the rows of a list.
	rowLine ui.Color
	// frame outlines the side panel's tables and divides its halves.
	frame ui.Color
	// procTable and diskTable fill the side panel's tables; procHead
	// and procHeadText are the process table's header.
	procTable, diskTable   ui.Color
	procHead, procHeadText ui.Color
	// cpu, mem and swap fill the side panel's usage bars, with the
	// theme's text over them. cpuRing, memRing and diskRing stroke the
	// monitor's ring gauges, which need a color that reads on its own.
	cpu, mem, swap         ui.Color
	cpuRing, memRing, diskRing ui.Color
	// tx and rx are the upload and download speeds.
	tx, rx ui.Color
	// tableHead is the header row of the start page's table; note is the
	// box of details in a dialog.
	tableHead, note ui.Color
	// dangerBanner and warningBanner fill the file editor's notes above
	// the text.
	dangerBanner, warningBanner ui.Color
	// problemText and warningText are the shortcuts page's notes on a
	// key: one that cannot be used, and a conflict.
	problemText, warningText ui.Color
	// activeLine is the line with the caret in the file editor, and
	// lineNumber its line numbers.
	activeLine, lineNumber ui.Color
}

var lightColors = palette{
	sidebar:       ui.RGBA(255, 255, 255, 0.55),
	line:          ui.RGBA(13, 23, 40, 0.07),
	rowLine:       ui.RGBA(13, 23, 40, 0.04),
	frame:         ui.RGBA(13, 23, 40, 0.10),
	procTable:     ui.RGBA(255, 255, 255, 0.45),
	diskTable:     ui.RGBA(255, 255, 255, 0.55),
	procHead:      ui.RGBA(10, 124, 255, 0.16),
	procHeadText:  ui.Hex("#0d1728"),
	cpu:           ui.RGBA(18, 168, 120, 0.45),
	mem:           ui.RGBA(184, 121, 10, 0.35),
	swap:          ui.RGBA(10, 124, 255, 0.35),
	cpuRing:       ui.Hex("#12a878"),
	memRing:       ui.Hex("#b8790a"),
	diskRing:      ui.Hex("#0a7cff"),
	tx:            ui.Hex("#c2580a"),
	rx:            ui.Hex("#0b8256"),
	tableHead:     ui.RGBA(255, 255, 255, 0.60),
	note:          ui.RGBA(13, 23, 40, 0.05),
	dangerBanner:  ui.RGBA(224, 69, 63, 0.10),
	warningBanner: ui.RGBA(184, 121, 10, 0.12),
	problemText:   ui.Hex("#b3261e"),
	warningText:   ui.Hex("#8a5f00"),
	activeLine:    ui.RGBA(10, 124, 255, 0.07),
	lineNumber:    ui.RGBA(13, 23, 40, 0.40),
}

// darkColors are lightColors for the dark theme's background (#18181b):
// the panels a little lighter than it, the tables in them darker, and
// the fills under text deep enough for the theme's light text.
var darkColors = palette{
	sidebar:       ui.RGBA(255, 255, 255, 0.020),
	line:          ui.RGBA(255, 255, 255, 0.060),
	rowLine:       ui.RGBA(255, 255, 255, 0.035),
	frame:         ui.RGBA(255, 255, 255, 0.115),
	procTable:     ui.RGBA(255, 255, 255, 0.030),
	diskTable:     ui.RGBA(255, 255, 255, 0.030),
	procHead:      ui.RGBA(99, 200, 255, 0.20),
	procHeadText:  ui.Hex("#e9eefb"),
	cpu:           ui.RGBA(61, 220, 151, 0.55),
	mem:           ui.RGBA(255, 200, 97, 0.55),
	swap:          ui.RGBA(99, 200, 255, 0.55),
	cpuRing:       ui.Hex("#3ddc97"),
	memRing:       ui.Hex("#ffc861"),
	diskRing:      ui.Hex("#63c8ff"),
	tx:            ui.Hex("#ffb37a"),
	rx:            ui.Hex("#7ee0c0"),
	tableHead:     ui.RGBA(255, 255, 255, 0.045),
	note:          ui.RGBA(255, 255, 255, 0.055),
	dangerBanner:  ui.RGBA(255, 107, 107, 0.13),
	warningBanner: ui.RGBA(255, 200, 97, 0.13),
	problemText:   ui.Hex("#ff8a8a"),
	warningText:   ui.Hex("#ffcb6b"),
	activeLine:    ui.RGBA(255, 255, 255, 0.045),
	lineNumber:    ui.RGBA(233, 238, 251, 0.38),
}

// Colors that read on both backgrounds.
var (
	// folderColor is the app's own folder icon.
	folderColor = ui.Hex("#eab308")
	// The side panel's charts: upload and download, and latency.
	txChart, rxChart, latencyChart = ui.Hex("#fb923c"), ui.Hex("#4ade80"), ui.Hex("#60a5fa")
)

// codeThemes are the file editor's colors on each theme: the editor's
// own, with the app's accent, and the line with the caret faint.
var codeThemes = [2]*editor.Theme{newCodeTheme(lightTheme, &lightColors), newCodeTheme(darkTheme, &darkColors)}

func newCodeTheme(t *ui.Theme, p *palette) *editor.Theme {
	th := editor.DefaultTheme(t)
	th.ActiveLine = p.activeLine
	th.LineNumber = p.lineNumber
	sel := float32(0.18)
	if t.Dark {
		sel = 0.35
	}
	th.Selection = t.Accent.Alpha(sel)
	th.InactiveSelection = t.Accent.Alpha(sel * 0.55)
	return th
}

// codeTheme is the file editor's colors in the window being built.
func codeTheme() *editor.Theme {
	if theme.Dark {
		return codeThemes[1]
	}
	return codeThemes[0]
}

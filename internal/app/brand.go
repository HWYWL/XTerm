package app

import (
	"finallyshell/internal/config"
	"finallyshell/internal/icons"

	"github.com/egoist/mygo/ui"
)

// hostBrand is the logo that says what a host connects to — the protocol's
// distribution, or the cloud of the bucket — and the tint to draw a logo
// that is one color in, "" when the server's icon serves instead.
func hostBrand(h *config.Host) (name string, tint ui.Color) {
	if h == nil {
		return "", ui.Color{}
	}
	if h.IsObject() {
		switch h.ObjectProvider().ID {
		case config.ObjectAliyunOSS:
			return "alibabacloud", ui.Color{}
		case config.ObjectTencentCOS:
			return "tencentcloud", ui.Color{}
		case config.ObjectHuaweiOBS:
			return "huaweicloud", ui.Color{}
		case config.ObjectQiniu:
			return "qiniu", ui.Color{}
		case config.ObjectAWS:
			return "aws", ui.Color{}
		case config.ObjectGCS:
			return "googlecloud", ui.Color{}
		case config.ObjectAzure:
			return "azure", ui.Color{}
		case config.ObjectMinIO:
			return "minio", ui.RGBA(199, 46, 73, 1)
		}
		// RustFS, a directory: their logos are not ones an icon set has.
		return "", ui.Color{}
	}
	switch {
	case h.IsRedis():
		return "redis", ui.RGBA(255, 68, 56, 1)
	case h.Protocol == "" || h.Protocol == config.ProtocolSSH:
		return "linux", ui.Color{} // the distribution every host of the app's starts as
	}
	return "", ui.Color{}
}

// hostIcon draws what the host connects to, at size, in the row's color.
func (a *App) hostIcon(c *ui.Context, h *config.Host, size float32, color ui.Color) {
	name, tint := hostBrand(h)
	if name == "" {
		// A directory of this machine has no cloud's logo — a folder says
		// where it is — and a port or a Telnet server no vendor's: a cable
		// and a network do.
		glyph := "server"
		switch {
		case h != nil && h.IsObject():
			glyph = "folder"
		case h != nil && h.IsSerial():
			glyph = "cable"
		case h != nil && h.IsTelnet():
			glyph = "network"
		case h != nil && h.IsADB():
			glyph = "plug"
		}
		ui.Icon(c, icons.Get(glyph)).FontSize(size).TextColor(color.Alpha(0.75))
		return
	}
	icon := ui.Icon(c, icons.Brand(name)).FontSize(size)
	if tint.A > 0 {
		icon.TextColor(tint)
	} else {
		icon.TextColor(color.Alpha(0.75))
	}
}

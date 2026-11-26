package app

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"finallyshell/internal/icons"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// objectTransfer is one upload or download of the object panel, shown as
// the transfers page shows the shell's: the file, how far it is, how fast,
// and the button that stops it.
type objectTransfer struct {
	id       int
	upload   bool
	name     string
	where    string // the local file, or the key
	total    int64
	done     atomic.Int64
	reported atomic.Int64 // the bytes the meter has heard of
	startAt  time.Time
	ended    time.Time
	err      error
	cancel   context.CancelFunc
	removed  bool
}

// addObjectTransfer puts t in the panel's list, under the next id.
func (s *Session) addObjectTransfer(t *objectTransfer) {
	f := &s.objects
	f.transferSeq++
	t.id = f.transferSeq
	f.transfers = append(f.transfers, t)
}

// report records how far the transfer is, and tells the meter of the bytes
// since the last report: the traffic moves as the file does.
func (t *objectTransfer) report(s *Session, done int64) {
	t.done.Store(done)
	for {
		last := t.reported.Load()
		if done-last < 64<<10 {
			return
		}
		if t.reported.CompareAndSwap(last, done) {
			t.send(s, done-last)
			return
		}
	}
}

// send is the meter's note of n bytes.
func (t *objectTransfer) send(s *Session, n int64) {
	if n <= 0 {
		return
	}
	if t.upload {
		s.noteObjectWritten(n)
	} else {
		s.noteObjectRead(n)
	}
}

// finish ends the transfer, with the bytes it still had to report.
func (t *objectTransfer) finish(s *Session, err error) {
	t.err = err
	t.ended = time.Now()
	if err == nil {
		t.done.Store(t.total)
	}
	done := t.done.Load()
	if rest := done - t.reported.Load(); rest > 0 && t.reported.CompareAndSwap(t.reported.Load(), done) {
		t.send(s, rest)
	}
}

// running tells whether the transfer is still under way.
func (t *objectTransfer) running() bool { return t.ended.IsZero() }

// ratio is how much of the file has gone, 0 to 1.
func (t *objectTransfer) ratio() float64 {
	if t.total <= 0 {
		if t.ended.IsZero() {
			return 0
		}
		return 1
	}
	return min(1, float64(t.done.Load())/float64(t.total))
}

// rate is the average bytes a second since it began.
func (t *objectTransfer) rate() float64 {
	elapsed := t.ended.Sub(t.startAt).Seconds()
	if t.ended.IsZero() {
		elapsed = time.Since(t.startAt).Seconds()
	}
	if elapsed <= 0 {
		return 0
	}
	return float64(t.done.Load()) / elapsed
}

// remaining is how long the rest takes at the average rate.
func (t *objectTransfer) remaining() time.Duration {
	if !t.running() || t.total <= t.done.Load() {
		return 0
	}
	rate := t.rate()
	if rate <= 0 {
		return 0
	}
	return time.Duration(float64(t.total-t.done.Load()) / rate * float64(time.Second))
}

// progressReader is the reader of a download, which reports what it read.
type progressReader struct {
	r io.Reader
	s *Session
	t *objectTransfer
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.t.report(p.s, p.t.done.Load()+int64(n))
	}
	return n, err
}

// objectTransferTray is the list under the file panel: what is going where,
// how far, and how fast, with a button to stop a running one and to clear
// the finished ones.
func (a *App) objectTransferTray(c *ui.Context, s *Session) {
	f := &s.objects
	kept := f.transfers[:0]
	for _, t := range f.transfers {
		if !t.removed {
			kept = append(kept, t)
		}
	}
	f.transfers = kept
	if len(f.transfers) == 0 {
		return
	}
	running := 0
	finished := 0
	for _, t := range f.transfers {
		if t.running() {
			running++
		} else {
			finished++
		}
	}
	ui.Column(c).BorderWidth(1, 0, 0, 0).BorderColor(colors.line).Children(func() {
		ui.Row(c).Padding(3, 10).Gap(6).AlignItems(ui.Center).Children(func() {
			head := fmt.Sprintf(locales.ObjectTransfersRunning(), running)
			if running == 0 {
				head = locales.ObjectTransfersDone()
			}
			ui.Text(c, head).FontSize(11)
			ui.Spacer(c)
			if finished > 0 {
				if ui.Button(c, locales.ObjectClearFinished()).Clicked() {
					kept := f.transfers[:0]
					for _, t := range f.transfers {
						if t.running() {
							kept = append(kept, t)
						}
					}
					f.transfers = kept
				}
			}
		})
		rows := 0
		for _, t := range f.transfers {
			if rows == 4 {
				break
			}
			rows++
			a.objectTransferRow(c, s, t)
		}
	})
}

// objectTransferRow is one transfer: its file, its bar, and its numbers.
func (a *App) objectTransferRow(c *ui.Context, s *Session, t *objectTransfer) {
	color := colors.rx
	icon := "download"
	if t.upload {
		color, icon = colors.tx, "upload"
	}
	if t.err != nil {
		color = theme.Danger
	}
	ui.Column(c).Gap(2).Padding(2, 10).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Get(icon)).FontSize(11).TextColor(color)
			ui.Text(c, t.name).FontSize(11.5).SingleLine().Shrink(1).MinWidth(0).Tooltip(t.where)
			ui.Spacer(c)
			if t.running() {
				if widgets.IconButton(c, "x", locales.Cancel()).Clicked() {
					if t.cancel != nil {
						t.cancel()
					}
					return
				}
			} else if widgets.IconButton(c, "x", locales.ObjectClear()).Clicked() {
				t.removed = true
			}
		})
		ui.Box(c).FillWidth().Height(5).Radius(3).Background(colors.line).Clip().Children(func() {
			ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).
				WidthPercent(float32(t.ratio() * 100)).Background(color)
		})
		text := fmt.Sprintf("%s / %s · %s", formatSize(t.done.Load()), formatSize(t.total), formatRate(t.rate()))
		if rem := t.remaining(); rem > 0 {
			text += " · " + fmt.Sprintf(locales.ObjectTransferRemaining(), clockText(rem))
		}
		if t.err != nil {
			text = describeError(t.err)
		}
		line := ui.Text(c, text).FontSize(10.5).SingleLine().Tooltip(text)
		if t.err != nil {
			line.TextColor(theme.Danger)
		} else {
			line.TextColor(theme.TextMuted)
		}
	})
}

// clockText is d as minutes and seconds, as a transfer's rest.
func clockText(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

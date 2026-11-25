package widgets

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// Curve draws series as smooth lines, the newest sample on the right of
// a box n samples wide, each line filled under with a faint shade of its
// color, as FinalShell's network and latency graphs. A series shorter
// than n starts at the right, as the others end there. The scale is the
// largest value of the series, a flat line along the bottom when there is
// none.
func Curve(c *ui.Context, n int, series [][]float64, colors []ui.Color, height float32) {
	if n < 2 {
		n = 2
	}
	t := c.Theme()
	ui.Box(c).Height(height).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		base := r.Y + r.H - 2
		p.Line(r.X, base, r.X+r.W, base, 1, chartBorder(t).Alpha(0.55))
		peak := 0.0
		for _, s := range series {
			for _, v := range s {
				peak = math.Max(peak, v)
			}
		}
		if peak <= 0 {
			return
		}
		step := r.W / float32(n-1)
		for si, s := range series {
			if len(s) == 0 {
				continue
			}
			pts := make([][2]float32, len(s))
			for i, v := range s {
				x := r.X + float32(n-len(s)+i)*step
				y := base - float32(v/peak)*(r.H-6)
				pts[i] = [2]float32{x, y}
			}
			smooth := smoothPoints(pts)
			var fill ui.Path
			fill.MoveTo(smooth[0][0], base)
			for _, q := range smooth {
				fill.LineTo(q[0], q[1])
			}
			fill.LineTo(smooth[len(smooth)-1][0], base)
			fill.Close()
			p.FillPath(&fill, colors[si].Alpha(0.14))
			p.StrokePolyline(smooth, 1.6, colors[si])
		}
	})
}

// smoothPoints is a line's points with the corners rounded: a
// Catmull-Rom spline through them, sampled so the line reads as a curve
// at the panel's size.
func smoothPoints(pts [][2]float32) [][2]float32 {
	if len(pts) < 3 {
		return pts
	}
	out := make([][2]float32, 0, len(pts)*8)
	out = append(out, pts[0])
	for i := 0; i < len(pts)-1; i++ {
		p0 := pts[i]
		if i > 0 {
			p0 = pts[i-1]
		}
		p1, p2 := pts[i], pts[i+1]
		p3 := p2
		if i+2 < len(pts) {
			p3 = pts[i+2]
		}
		for j := 1; j <= 8; j++ {
			t := float32(j) / 8
			out = append(out, [2]float32{
				catmullRom(p0[0], p1[0], p2[0], p3[0], t),
				catmullRom(p0[1], p1[1], p2[1], p3[1], t),
			})
		}
	}
	return out
}

// catmullRom is one axis of a point between b and c, with a and d as the
// neighbors that set the curve.
func catmullRom(a, b, c, d, t float32) float32 {
	return 0.5 * (2*b +
		(-a+c)*t +
		(2*a-5*b+4*c-d)*t*t +
		(-a+3*b-3*c+d)*t*t*t)
}

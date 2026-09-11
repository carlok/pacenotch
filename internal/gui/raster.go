package gui

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/carlok/pacenotch/internal/pace"
)

// NotchWidth is the notch width for a bar h pixels tall; never less than 1 px.
func NotchWidth(h int) int { return max(1, h/8) }

// NotchX is the notch's left column, relative to the bar's left edge, in a bar w×h pixels.
// The notch is centered on the pace position and kept inside the bar.
func NotchX(w, h, expBP int) int {
	nw := NotchWidth(h)
	return min(max(w*expBP/10000-nw/2, 0), max(w-nw, 0))
}

// DrawBar is the one bar rasterizer, used by the window bars and the tray icon. Bars are
// always horizontal: the track fills r minus a small vertical inset, the fill grows from
// the left to the used percentage, and the pace notch crosses the whole height of r
// vertically, so it stays taller than the bar. Where the notch does not contrast with its
// neighbour, a 1 px gap separates them. Idle rows are hollow and have no notch.
func DrawBar(img draw.Image, r image.Rectangle, row pace.Row, th BarTheme) {
	if r.Empty() {
		return
	}
	w, h := r.Dx(), r.Dy()
	inset := h / 6
	track := image.Rect(r.Min.X, r.Min.Y+inset, r.Max.X, r.Max.Y-inset)
	fillW := min(max((w*row.UsedBP+5000)/10000, 0), w)
	fill := th.Fill[row.Class]
	filled := image.Rect(track.Min.X, track.Min.Y, track.Min.X+fillW, track.Max.Y)

	if row.Class == pace.Idle {
		outline(img, track, th.IdleEdge)
		paint(img, filled, fill)
		return
	}
	paint(img, track, th.Track)
	paint(img, filled, fill)
	if row.ExpBP < 0 {
		return
	}

	nw, nx := NotchWidth(h), NotchX(w, h, row.ExpBP)
	neighbour := func(x int) color.NRGBA {
		if x < fillW {
			return fill
		}
		return th.Track
	}
	for _, x := range []int{nx - 1, nx + nw} {
		if x >= 0 && x < w && Contrast(th.Notch, neighbour(x)) < MinContrast {
			paint(img, image.Rect(r.Min.X+x, track.Min.Y, r.Min.X+x+1, track.Max.Y), th.Gap)
		}
	}
	paint(img, image.Rect(r.Min.X+nx, r.Min.Y, r.Min.X+nx+nw, r.Max.Y), th.Notch)
}

func paint(img draw.Image, r image.Rectangle, c color.NRGBA) {
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
}

func outline(img draw.Image, r image.Rectangle, c color.NRGBA) {
	paint(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), c)
	paint(img, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), c)
	paint(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), c)
	paint(img, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), c)
}

package gui

import (
	"bytes"
	"image"
	"image/png"

	"github.com/carlok/pacenotch/internal/pace"
)

// IconState adds a warning badge to the tray icon.
type IconState struct {
	Stale     bool // stale data or no data
	AuthError bool
}

func (s IconState) warn() bool { return s.Stale || s.AuthError }

// IconLayout returns the icon size and where its two bars and the badge go, for an icon h
// pixels tall. wide makes it 2:1 (the macOS menu bar); otherwise it is square. The bars
// shrink to make room for the badge instead of letting it cover a notch.
func IconLayout(h int, wide, warn bool) (size image.Point, bars [2]image.Rectangle, badge image.Rectangle) {
	h = max(h, 8)
	w := h
	if wide {
		w = 2 * h
	}
	pad, gap := max(1, h/16), max(1, h/8)
	barH := (h - 2*pad - gap) / 2
	right := w - pad
	if warn {
		b := max(4, h/4)
		badge = image.Rect(w-pad-b, (h-b)/2, w-pad, (h-b)/2+b)
		right = badge.Min.X - 1
	}
	bars[0] = image.Rect(pad, pad, right, pad+barH)
	bars[1] = image.Rect(pad, h-pad-barH, right, h-pad)
	return image.Pt(w, h), bars, badge
}

// TrayIcon renders the tray / menu bar icon: two horizontal bars stacked, 5h on top and 7d
// below, each filled to its used percentage with its vertical pace notch. A window missing
// from the data is drawn hollow, like an idle one.
func TrayIcon(rows []pace.Row, st IconState, h int, wide bool, th BarTheme) *image.NRGBA {
	size, bars, badge := IconLayout(h, wide, st.warn())
	img := image.NewNRGBA(image.Rectangle{Max: size})
	for i, key := range []string{"five_hour", "seven_day"} {
		DrawBar(img, bars[i], rowFor(rows, key), th)
	}
	if st.warn() {
		paint(img, badge, th.Warn)
		if b := badge.Dx(); b >= 8 { // an exclamation mark when there is room for one
			x := badge.Min.X + b/2 - b/8
			paint(img, image.Rect(x, badge.Min.Y+b/6, x+max(1, b/4), badge.Max.Y-b/3-1), th.Gap)
			paint(img, image.Rect(x, badge.Max.Y-b/3+b/12, x+max(1, b/4), badge.Max.Y-b/6), th.Gap)
		}
	}
	return img
}

func rowFor(rows []pace.Row, key string) pace.Row {
	for _, r := range rows {
		if r.Key == key {
			return r
		}
	}
	return pace.Row{Key: key, Class: pace.Idle, ExpBP: -1, Left: -1, Proj: -1, Hit: -1}
}

// EncodePNG encodes an in-memory image, which cannot fail.
func EncodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

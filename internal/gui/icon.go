package gui

import (
	"bytes"
	"image"
	"image/color"
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

// AppIconRows are the sample windows on the application icon: 5h on pace, 7d ahead.
var AppIconRows = [2]pace.Row{
	{Key: "five_hour", Class: pace.On, UsedBP: 4700, ExpBP: 5000},
	{Key: "seven_day", Class: pace.Ahead, UsedBP: 7200, ExpBP: 6339},
}

// AppIconBars returns where the two bars go on a size×size application icon.
func AppIconBars(size int) [2]image.Rectangle {
	m := size / 10
	inner := size - 2*m
	pad, barH, gap := inner*3/20, max(2, inner/5), max(1, inner/10)
	top := m + (inner-2*barH-gap)/2
	x0, x1 := m+pad, size-m-pad
	return [2]image.Rectangle{
		image.Rect(x0, top, x1, top+barH),
		image.Rect(x0, top+barH+gap, x1, top+2*barH+gap),
	}
}

// AppIcon is the application icon: the two notched bars on a dark rounded square, inside
// the transparent margin macOS icons expect.
func AppIcon(size int) *image.NRGBA {
	size = max(size, 16)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	m := size / 10
	roundedRect(img, image.Rect(m, m, size-m, size-m), (size-2*m)*9/40, rgb(0x1C1C1E))
	th := DarkBars()
	for i, r := range AppIconBars(size) {
		DrawBar(img, r, AppIconRows[i], th)
	}
	return img
}

func roundedRect(img *image.NRGBA, r image.Rectangle, radius int, c color.NRGBA) {
	left, right := r.Min.X+radius, r.Max.X-1-radius
	top, bottom := r.Min.Y+radius, r.Max.Y-1-radius
	for y := r.Min.Y; y < r.Max.Y; y++ {
		dy := max(top-y, y-bottom, 0)
		for x := r.Min.X; x < r.Max.X; x++ {
			if dx := max(left-x, x-right, 0); dx*dx+dy*dy <= radius*radius {
				img.SetNRGBA(x, y, c)
			}
		}
	}
}

// EncodePNG encodes an in-memory image, which cannot fail.
func EncodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

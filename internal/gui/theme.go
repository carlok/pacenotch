// Package gui is the desktop window and tray icon. The files ending in _fyne.go are the
// thin Fyne glue, built only with the "gui" tag. Everything else (view-model, bar
// rasterizer, tray icon, refresh controller) is plain Go and fully unit-tested.
package gui

import (
	"image/color"
	"math"

	"github.com/carlok/pacenotch/internal/pace"
)

// MinContrast is the WCAG contrast ratio the notch keeps against every pixel that touches it.
const MinContrast = 3.0

// BarTheme is the palette of the bar rasterizer and the window text.
type BarTheme struct {
	Dark     bool
	Fill     map[pace.Class]color.NRGBA
	Track    color.NRGBA // the empty part of an active bar
	IdleEdge color.NRGBA // outline of a hollow idle bar
	Notch    color.NRGBA
	Gap      color.NRGBA // 1 px separator between the notch and a fill it does not contrast with
	Warn     color.NRGBA // stale / auth badge
	Text     color.NRGBA
	Dim      color.NRGBA
}

func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 0xff}
}

// DarkBars is the dark palette: white notch, black gap.
func DarkBars() BarTheme {
	return BarTheme{
		Dark: true,
		Fill: map[pace.Class]color.NRGBA{
			pace.Ahead: rgb(0xFF453A), pace.On: rgb(0xFFD60A), pace.Under: rgb(0x32D74B), pace.Idle: rgb(0x64D2FF),
		},
		Track: rgb(0x48484A), IdleEdge: rgb(0x8E8E93),
		Notch: rgb(0xFFFFFF), Gap: rgb(0x000000), Warn: rgb(0xFF9F0A),
		Text: rgb(0xF2F2F7), Dim: rgb(0x98989D),
	}
}

// LightBars is the light palette: black notch, white gap.
func LightBars() BarTheme {
	return BarTheme{
		Fill: map[pace.Class]color.NRGBA{
			pace.Ahead: rgb(0xD70015), pace.On: rgb(0xB25000), pace.Under: rgb(0x248A3D), pace.Idle: rgb(0x0071A4),
		},
		Track: rgb(0xD1D1D6), IdleEdge: rgb(0x8E8E93),
		Notch: rgb(0x000000), Gap: rgb(0xFFFFFF), Warn: rgb(0xC93400),
		Text: rgb(0x1C1C1E), Dim: rgb(0x6C6C70),
	}
}

// Bars returns the dark or light palette.
func Bars(dark bool) BarTheme {
	if dark {
		return DarkBars()
	}
	return LightBars()
}

// Contrast is the WCAG 2 contrast ratio of two opaque colors, from 1 to 21.
func Contrast(a, b color.NRGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c color.NRGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

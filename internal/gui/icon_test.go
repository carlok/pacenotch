package gui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/carlok/pacenotch/internal/pace"
)

func iconRows() []pace.Row {
	return []pace.Row{
		{Key: "five_hour", Class: pace.Ahead, UsedBP: 7000, ExpBP: 5000},
		{Key: "seven_day", Class: pace.Under, UsedBP: 2000, ExpBP: 6000},
		{Key: "seven_day_opus", Class: pace.Ahead, UsedBP: 9000, ExpBP: 1000},
	}
}

func TestTrayIcon(t *testing.T) {
	for name, th := range themes {
		for _, h := range []int{16, 22, 32, 44} {
			for _, wide := range []bool{false, true} {
				for _, st := range []IconState{{}, {Stale: true}, {AuthError: true}} {
					img := TrayIcon(iconRows(), st, h, wide, th)
					size, bars, badge := IconLayout(h, wide, st.Stale || st.AuthError)
					wantW := h
					if wide {
						wantW = 2 * h
					}
					if img.Bounds().Dx() != wantW || img.Bounds().Dy() != h || size != image.Pt(wantW, h) {
						t.Fatalf("%s h=%d wide=%v: size %v", name, h, wide, img.Bounds())
					}
					if bars[0].Max.Y > bars[1].Min.Y || bars[0].Dx() < 4 {
						t.Fatalf("h=%d: 5h bar %v must sit above the 7d bar %v", h, bars[0], bars[1])
					}
					checkBar(t, img, bars[0], iconRows()[0], th)
					checkBar(t, img, bars[1], iconRows()[1], th)
					hasBadge := img.NRGBAAt(badge.Min.X, badge.Min.Y) == th.Warn
					if hasBadge != (st.Stale || st.AuthError) || (badge.Empty() == hasBadge) {
						t.Fatalf("h=%d %+v: badge %v at %v", h, st, hasBadge, badge)
					}
					if hasBadge && badge.Min.X <= bars[0].Max.X {
						t.Fatal("the badge must not cover the bars")
					}
				}
			}
		}
	}
}

func TestTrayIconMissingWindowsAreHollow(t *testing.T) {
	th := LightBars()
	img := TrayIcon(nil, IconState{}, 4, false, th) // clamped to 8 px
	if img.Bounds().Dx() != 8 {
		t.Fatalf("size %v", img.Bounds())
	}
	img = TrayIcon(nil, IconState{}, 32, true, th)
	notch, edge := 0, 0
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			switch img.NRGBAAt(x, y) {
			case th.Notch:
				notch++
			case th.IdleEdge:
				edge++
			}
		}
	}
	if notch != 0 || edge == 0 {
		t.Errorf("hollow bars: %d notch pixels, %d outline pixels", notch, edge)
	}
}

func TestEncodePNG(t *testing.T) {
	img := TrayIcon(iconRows(), IconState{}, 22, true, DarkBars())
	decoded, err := png.Decode(bytes.NewReader(EncodePNG(img)))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(decoded.At(10, 5)); got != img.At(10, 5) {
		t.Errorf("round trip %v != %v", got, img.At(10, 5))
	}
}

package gui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/carlok/pacenotch/internal/pace"
)

var themes = map[string]BarTheme{"dark": DarkBars(), "light": LightBars()}

func TestPalettes(t *testing.T) {
	for name, th := range themes {
		if c := Contrast(th.Notch, th.Track); c < MinContrast {
			t.Errorf("%s: notch/track contrast %.2f", name, c)
		}
		if c := Contrast(th.Notch, th.Gap); c < MinContrast {
			t.Errorf("%s: notch/gap contrast %.2f", name, c)
		}
		for cls, fill := range th.Fill {
			// either the notch stands out from the fill, or the gap drawn between them does
			if Contrast(th.Notch, fill) < MinContrast && Contrast(th.Gap, fill) < MinContrast {
				t.Errorf("%s %s: neither the notch nor the gap contrasts with the fill", name, cls)
			}
			if fill == th.Notch || fill == th.Track {
				t.Errorf("%s %s: fill color collides", name, cls)
			}
		}
	}
	if Bars(true).Dark != true || Bars(false).Dark != false {
		t.Error("Bars(dark)")
	}
	if c := Contrast(rgb(0xFFFFFF), rgb(0x000000)); math.Abs(c-21) > 1e-9 {
		t.Errorf("white/black contrast %v", c)
	}
	if c := Contrast(rgb(0x777777), rgb(0x777777)); c != 1 {
		t.Errorf("same color contrast %v", c)
	}
}

// checkBar verifies an active bar drawn into r: the notch is vertical (full height, narrower
// than tall), at least 1 px wide, at the pace position, and every pixel beside it contrasts
// with it; the fill edge and the empty part have the right colors.
func checkBar(t *testing.T, img *image.NRGBA, r image.Rectangle, row pace.Row, th BarTheme) {
	t.Helper()
	w, h := r.Dx(), r.Dy()
	nw, nx := NotchWidth(h), NotchX(w, h, row.ExpBP)
	midY := r.Min.Y + h/2
	if nw < 1 || nw >= h {
		t.Fatalf("notch %d px wide in a %d px bar is not a vertical line", nw, h)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := nx; x < nx+nw; x++ {
			if got := img.NRGBAAt(r.Min.X+x, y); got != th.Notch {
				t.Fatalf("h=%d %+v: pixel (%d,%d) = %v, want notch", h, row, x, y-r.Min.Y, got)
			}
		}
	}
	if center, want := float64(nx)+float64(nw)/2, float64(w*row.ExpBP)/10000; math.Abs(center-want) > float64(nw) {
		t.Fatalf("h=%d %+v: notch center %.1f, pace at %.1f", h, row, center, want)
	}
	for _, x := range []int{nx - 1, nx + nw} {
		if x < 0 || x >= w {
			continue
		}
		if c := Contrast(img.NRGBAAt(r.Min.X+x, midY), th.Notch); c < MinContrast {
			t.Fatalf("h=%d %+v: neighbour at %d contrast %.2f", h, row, x, c)
		}
	}
	fillW := (w*row.UsedBP + 5000) / 10000
	near := func(x int) bool { return x >= nx-1 && x <= nx+nw }
	if fillW >= 1 && !near(fillW-1) {
		if got := img.NRGBAAt(r.Min.X+fillW-1, midY); got != th.Fill[row.Class] {
			t.Fatalf("h=%d %+v: fill edge %v", h, row, got)
		}
	}
	if fillW < w && !near(fillW) {
		if got := img.NRGBAAt(r.Min.X+fillW, midY); got != th.Track {
			t.Fatalf("h=%d %+v: empty part %v", h, row, got)
		}
	}
}

func TestDrawBar(t *testing.T) {
	cases := []struct{ ubp, ebp int }{
		{7200, 6339}, {7200, 3000}, {3300, 5000}, {0, 0}, {10000, 10000}, {5000, 5000}, {100, 9999}, {10000, 0}, {2000, 8000},
	}
	for name, th := range themes {
		for _, h := range []int{16, 22, 32, 44} {
			for _, cls := range []pace.Class{pace.Ahead, pace.On, pace.Under} {
				for _, c := range cases {
					w := 4 * h
					img := image.NewNRGBA(image.Rect(0, 0, w+10, h+4))
					r := image.Rect(5, 2, 5+w, 2+h)
					row := pace.Row{Class: cls, UsedBP: c.ubp, ExpBP: c.ebp}
					DrawBar(img, r, row, th)
					t.Run("", func(t *testing.T) { checkBar(t, img, r, row, th) })
					if img.NRGBAAt(4, 2+h/2) != (color.NRGBA{}) || img.NRGBAAt(5+w, 2+h/2) != (color.NRGBA{}) {
						t.Fatalf("%s: drew outside the rectangle", name)
					}
				}
			}
		}
	}
}

func TestDrawBarIdleAndEdgeCases(t *testing.T) {
	th := DarkBars()
	img := image.NewNRGBA(image.Rect(0, 0, 100, 24))
	DrawBar(img, img.Bounds(), pace.Row{Class: pace.Idle, UsedBP: 2500, ExpBP: -1}, th)
	track := 24 / 6
	for x := 0; x < 100; x++ {
		for y := 0; y < 24; y++ {
			if img.NRGBAAt(x, y) == th.Notch {
				t.Fatal("idle bars have no notch")
			}
		}
	}
	if img.NRGBAAt(10, 12) != th.Fill[pace.Idle] || img.NRGBAAt(99, 12) != th.IdleEdge ||
		img.NRGBAAt(60, track) != th.IdleEdge || img.NRGBAAt(60, 12) != (color.NRGBA{}) {
		t.Error("idle bars are hollow with an idle fill")
	}

	img = image.NewNRGBA(image.Rect(0, 0, 50, 10))
	DrawBar(img, img.Bounds(), pace.Row{Class: pace.On, UsedBP: 5000, ExpBP: -1}, th)
	for x := 0; x < 50; x++ {
		if img.NRGBAAt(x, 0) == th.Notch {
			t.Fatal("no notch without a pace")
		}
	}
	DrawBar(img, image.Rectangle{}, pace.Row{Class: pace.On}, th)                                       // empty: no-op
	DrawBar(img, image.Rect(40, 5, 80, 20), pace.Row{Class: pace.Ahead, UsedBP: 9000, ExpBP: 9000}, th) // clipped
	if NotchX(3, 32, 10000) != 0 || NotchX(1, 8, 5000) != 0 {
		t.Error("the notch must stay inside tiny bars")
	}
}

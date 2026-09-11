//go:build gui

package gui

import (
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/carlok/pacenotch/internal/usage"
)

// TestScreenshots renders the README images with Fyne's software painter:
//
//	PACENOTCH_SCREENSHOTS=docs go test -tags gui ./internal/gui -run Screenshots
func TestScreenshots(t *testing.T) {
	dir := os.Getenv("PACENOTCH_SCREENSHOTS")
	if dir == "" {
		t.Skip("set PACENOTCH_SCREENSHOTS=DIR to render the README images")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("..", "..", dir)
	}
	os.MkdirAll(dir, 0o755)
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "screenshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	v := BuildView(usage.Result{Data: data, Age: 24}, nil, now, 5)

	for _, name := range []string{"dark", "light"} {
		a := test.NewTempApp(t)
		u := NewUI(a, "v0.1.0")
		if name == "light" {
			// "System" with the OS in light mode; the test driver has no OS variant of its own
			u.SetThemeChoice(ThemeSystem)
			a.Settings().SetTheme(forcedVariant{theme.DefaultTheme(), theme.VariantLight})
		}
		u.Show(v)
		capture(t, u.Window, fyne.NewSize(620, 400), filepath.Join(dir, "window-"+name+".png"))
		u.ShowAbout()
		capture(t, u.about, fyne.NewSize(480, 380), filepath.Join(dir, "about-"+name+".png"))

		dark := name == "dark"
		menubar := rgb(0xF2F2F2)
		if dark {
			menubar = rgb(0x1E1E1E)
		}
		for suffix, img := range map[string]*image.NRGBA{
			"wide":   TrayIcon(v.PaceRows(), IconState{}, 32, true, Bars(dark)),
			"square": TrayIcon(v.PaceRows(), IconState{}, 32, false, Bars(dark)),
			"stale":  TrayIcon(v.PaceRows(), IconState{Stale: true}, 32, true, Bars(dark)),
		} {
			write(t, filepath.Join(dir, "tray-"+name+"-"+suffix+".png"), scale(onBackground(img, menubar), 4))
		}
	}
}

func capture(t *testing.T, w fyne.Window, size fyne.Size, path string) {
	t.Helper()
	w.Resize(size)
	if sc, ok := w.Canvas().(interface{ SetScale(float32) }); ok {
		sc.SetScale(2)
	}
	write(t, path, w.Canvas().Capture())
}

func write(t *testing.T, path string, img image.Image) {
	t.Helper()
	if err := os.WriteFile(path, EncodePNG(img), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func onBackground(src *image.NRGBA, bg color.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Over)
	return dst
}

// scale enlarges pixel art without smoothing, so single-pixel notches stay visible.
func scale(src *image.NRGBA, k int) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx()*k, b.Dy()*k))
	for y := 0; y < b.Dy()*k; y++ {
		for x := 0; x < b.Dx()*k; x++ {
			dst.SetNRGBA(x, y, src.NRGBAAt(x/k, y/k))
		}
	}
	return dst
}

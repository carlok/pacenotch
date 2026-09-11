package tui

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTerminalScreenshot renders the README terminal images: the real colored output of
// the CLI, converted to SVG. Bars and the notch are drawn as rectangles, so they look the
// same whatever font the browser picks.
//
//	PACENOTCH_SCREENSHOTS=docs go test ./internal/tui -run TerminalScreenshot
func TestTerminalScreenshot(t *testing.T) {
	dir := os.Getenv("PACENOTCH_SCREENSHOTS")
	if dir == "" {
		t.Skip("set PACENOTCH_SCREENSHOTS=DIR to render the README terminal images")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("..", "..", dir)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "screenshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct {
		name string
		cols int
	}{{"terminal", 84}, {"terminal-compact", 52}} {
		h := newHarness(t, "--color", "always", "--width", strconv.Itoa(s.cols))
		h.src.body = string(data)
		h.run()
		path := filepath.Join(dir, s.name+".svg")
		if err := os.WriteFile(path, []byte(ansiSVG(h.stdout.String(), s.cols)), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}
}

type sgr struct {
	fg, bg    string
	bold, dim bool
}

type cell struct {
	r  rune
	st sgr
}

var palette = map[string]string{
	"31": "#ff6159", "32": "#3ad55a", "33": "#ffcc2e", "36": "#5ac8fa", "90": "#6b6b70", "97": "#ffffff",
	"41": "#ff6159", "42": "#3ad55a", "43": "#ffcc2e", "46": "#5ac8fa",
}

var eighths = map[rune]float64{'█': 1, '▏': 1. / 8, '▎': 2. / 8, '▍': 3. / 8, '▌': 4. / 8, '▋': 5. / 8, '▊': 6. / 8, '▉': 7. / 8}

func isBlock(r rune) bool { _, ok := eighths[r]; return ok || r == '░' || r == '┃' }

func ansiSVG(out string, cols int) string {
	const cw, lh, fs, pad, titleBar = 8.4, 20.0, 14.0, 16.0, 26.0
	var grid [][]cell
	var line []cell
	var st sgr
	for i := 0; i < len(out); {
		if strings.HasPrefix(out[i:], "\033[") {
			end := strings.IndexByte(out[i:], 'm')
			for _, p := range strings.Split(out[i+2:i+end], ";") {
				switch {
				case p == "0":
					st = sgr{}
				case p == "1":
					st.bold = true
				case p == "2":
					st.dim = true
				case strings.HasPrefix(p, "4"):
					st.bg = palette[p]
				default:
					st.fg = palette[p]
				}
			}
			i += end + 1
			continue
		}
		r, n := utf8.DecodeRuneInString(out[i:])
		i += n
		if r == '\n' {
			grid, line = append(grid, line), nil
			continue
		}
		line = append(line, cell{r, st})
	}
	for len(grid) > 0 && len(grid[len(grid)-1]) == 0 {
		grid = grid[:len(grid)-1]
	}

	w, h := 2*pad+float64(cols)*cw, titleBar+2*pad+float64(len(grid))*lh
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`+"\n", w, h, w, h)
	b.WriteString(`<rect width="100%" height="100%" rx="10" fill="#1e1e1e"/>` + "\n")
	for i, c := range []string{"#ff5f57", "#febc2e", "#28c840"} {
		fmt.Fprintf(&b, `<circle cx="%d" cy="15" r="6" fill="%s"/>`+"\n", 20+i*20, c)
	}
	b.WriteString(`<g font-family="ui-monospace,SFMono-Regular,Menlo,Consolas,'DejaVu Sans Mono',monospace" font-size="14">` + "\n")
	for row, cells := range grid {
		y := titleBar + pad + float64(row)*lh
		for col := 0; col < len(cells); {
			c := cells[col]
			x := pad + float64(col)*cw
			fg := c.st.fg
			if fg == "" {
				fg = "#d4d4d4"
			}
			if c.st.bg != "" {
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`, x, y+4, cw+0.4, lh-8, c.st.bg)
			}
			if frac, ok := eighths[c.r]; ok {
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.2f" height="%.1f" fill="%s"/>`, x, y+4, cw*frac+0.4, lh-8, fg)
				col++
				continue
			}
			switch c.r {
			case '░': // one rectangle per run, so no seams between cells
				j := col
				for j < len(cells) && cells[j] == c {
					j++
				}
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" opacity="0.55"/>`, x, y+4, float64(j-col)*cw, lh-8, fg)
				col = j
			case '┃':
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`, x+cw*0.3, y, cw*0.4, lh, fg)
				col++
			case ' ':
				col++
			default:
				j := col
				var text strings.Builder
				for j < len(cells) && cells[j].st == c.st && !isBlock(cells[j].r) {
					text.WriteRune(cells[j].r)
					j++
				}
				s := strings.TrimRight(text.String(), " ")
				n := utf8.RuneCountInString(s)
				attrs := ""
				if c.st.bold {
					attrs += ` font-weight="bold"`
				}
				if c.st.dim {
					attrs += ` opacity="0.6"`
				}
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s"%s textLength="%.1f" lengthAdjust="spacingAndGlyphs" xml:space="preserve">%s</text>`,
					x, y+lh*0.7, fg, attrs, float64(n)*cw, html.EscapeString(s))
				col = j
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("</g>\n</svg>\n")
	return b.String()
}

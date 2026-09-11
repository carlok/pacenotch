package tui

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/carlok/pacenotch/internal/pace"
	"github.com/carlok/pacenotch/internal/usage"
)

const (
	hideCursor  = "\033[?25l"
	showCursor  = "\033[?25h"
	clearScreen = "\033[H\033[2J"
)

// watch redraws on every interval, even without new data, because the pace notch moves
// with time. The loader only fetches when the TTL has expired. A resize redraws at once.
// Each frame is built completely and written with a single Write, so it does not flicker.
func watch(d Deps, opts Options, loader *usage.Loader, now func() time.Time, st Style) int {
	ctx, cancel := context.WithCancel(d.Ctx)
	defer cancel()
	resized := d.Resize(ctx, d.Size)

	io.WriteString(d.Stdout, hideCursor)
	defer io.WriteString(d.Stdout, showCursor+"\n")

	interval := time.Duration(opts.Interval) * time.Second
	for {
		var out string
		res, err := loader.Load(ctx, false)
		if err == nil {
			out, _, err = frame(d, opts, res, now(), st)
		}
		if err != nil {
			out = st.FG[pace.On] + " " + Program + ": " + err.Error() + st.Reset + "\n" +
				st.Dim + " retrying every " + interval.String() + st.Reset + "\n"
		}
		io.WriteString(d.Stdout, clearScreen+strings.TrimRight(out, "\n")+"\n")

		select {
		case <-ctx.Done():
			return 0
		case <-resized:
		case <-d.After(interval):
		}
	}
}

// pollResize reports width changes by polling, for consoles without SIGWINCH (Windows).
func pollResize(ctx context.Context, size func() (int, error), every time.Duration) <-chan struct{} {
	out := make(chan struct{}, 1)
	last, _ := size() // baseline at call time, so an immediate resize is not missed
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if w, _ := size(); w != last {
					last = w
					notify(out)
				}
			}
		}
	}()
	return out
}

// notify sends without blocking; one pending event is enough to trigger a redraw.
func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
